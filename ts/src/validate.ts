/**
 * The validation engine: immutable check -> resolve -> structured constraints
 * + CEL rules + form-wide rules, all against the fully resolved form.
 * Mirrors the Go reference validate.go: identical codes, deterministic
 * order, typed expected/actual, secret masking (values AND messages).
 */

import { create, isMessage } from "@bufbuild/protobuf";
import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import {
  defaultValue,
  exprErr,
  fieldIsActive,
  isTuple,
  listItemDef,
  refDefKey,
  resolve,
  selectVariant,
} from "./compute.js";
import { joinPath } from "./descriptor.js";
import { durationNanos, parseGoDuration, parseRfc3339, timestampNanos } from "./duration.js";
import type { Engine } from "./engine.js";
import type { ValidationError, ValidationResult } from "./gen/schemapb/errors_pb.js";
import {
  ErrorCode,
  ValidationErrorSchema,
  ValidationResultSchema,
} from "./gen/schemapb/errors_pb.js";
import type {
  Schema,
  Schema_Field,
  Schema_Field_Bytes,
  Schema_Field_Choice,
  Schema_Field_Duration,
  Schema_Field_List,
  Schema_Field_Map,
  Schema_Field_OneOf,
  Schema_Field_Ref,
  Schema_Field_Rule,
  Schema_Field_String,
  Schema_Field_Timestamp,
} from "./gen/schemapb/schema_pb.js";
import { Schema_Field_Severity } from "./gen/schemapb/schema_pb.js";
import type { Value } from "./gen/schemapb/value_pb.js";
import { messageTemplates, renderMessage } from "./messages.js";
import { displayString, nativeEquals } from "./render.js";
import type { Format } from "./typed.js";
import {
  asBigInt,
  asFloat,
  asUnsigned,
  bytesV,
  canonicalValue,
  doubleV,
  durationV,
  fromNative,
  int64V,
  isNativeStruct,
  listV,
  type Native,
  type NativeStruct,
  nullV,
  strV,
  timestampV,
  toNative,
  uint64V,
} from "./value.js";

const INT32_MIN = -2147483648n;
const INT32_MAX = 2147483647n;
const INT64_MIN = -9223372036854775808n;
const INT64_MAX = 9223372036854775807n;
const UINT32_MAX = 4294967295n;
const UINT64_MAX = 18446744073709551615n;

/**
 * Validates form values against the compiled schema. values is mutated in
 * place by the resolve step. Every outcome lives in the ValidationResult.
 */
export function validate(e: Engine, values: NativeStruct): ValidationResult {
  const errs: ValidationError[] = [];
  checkImmutable(e, e.schema.fields, values, "", values, errs);
  errs.push(...resolve(e, values));
  validateFields(e, e.schema, values, values, "", errs);
  for (const r of e.schema.rules) {
    evalRule(e, r, r.id ?? "", null, values, undefined, errs);
  }
  return create(ValidationResultSchema, { errors: errs });
}

/** No failures at all. */
export function resultOk(r: ValidationResult): boolean {
  return r.errors.length === 0;
}

/** At least one ERROR-severity failure (warnings alone do not block). */
export function resultBlocking(r: ValidationResult): boolean {
  return r.errors.some((e) => e.severity !== Schema_Field_Severity.WARNING);
}

// =============================================================================
// Error construction
// =============================================================================

function verr(
  path: string,
  code: ErrorCode,
  constraint: string,
  expected: Value | undefined,
  actual: Value | undefined,
): ValidationError {
  const err = create(ValidationErrorSchema, {
    path,
    code,
    constraint,
    severity: Schema_Field_Severity.ERROR,
    message: renderMessage(code, expected, actual),
  });
  if (expected !== undefined) {
    err.expected = expected;
  }
  if (actual !== undefined) {
    err.actual = actual;
  }
  return err;
}

function typeErr(path: string, want: string, val: Native): ValidationError {
  return verr(path, ErrorCode.TYPE_MISMATCH, "", strV(want), fromNative(val));
}

/** Masks secret fields: drops actual and re-renders the message. */
function mask(errs: ValidationError[], secret: boolean): ValidationError[] {
  if (!secret) {
    return errs;
  }
  for (const e of errs) {
    delete e.actual;
    if (messageTemplates.has(e.code)) {
      e.message = renderMessage(e.code, e.expected, undefined);
    }
  }
  return errs;
}

// =============================================================================
// Traversal
// =============================================================================

function checkImmutable(
  e: Engine,
  fields: Schema_Field[],
  scope: NativeStruct,
  prefix: string,
  root: NativeStruct,
  errs: ValidationError[],
): void {
  for (const f of fields) {
    const path = joinPath(prefix, f.name);
    if (!fieldIsActive(e, f, root, path, undefined)) {
      continue;
    }
    if (f.immutable) {
      if (f.name in scope) {
        const dv = defaultOf(f);
        const cur = scope[f.name] ?? null;
        if (dv !== undefined && !nativeEquals(cur, dv)) {
          const err = verr(
            path,
            ErrorCode.IMMUTABLE_MODIFIED,
            "immutable",
            canonicalOrBestFit(f, dv),
            fromNative(cur),
          );
          errs.push(...mask([err], f.secret));
        }
      }
      continue;
    }
    const kind = f.kind;
    const cur = scope[f.name];
    if (kind.case === "object" && kind.value.schema !== undefined) {
      if (cur !== undefined && isNativeStruct(cur)) {
        checkImmutable(e, kind.value.schema.fields, cur, path, root, errs);
      }
    }
    if (kind.case === "list" && kind.value.items.length >= 1 && Array.isArray(cur)) {
      cur.forEach((el, i) => {
        const it = listItemDef(kind.value, i);
        if (it?.kind.case === "object" && it.kind.value.schema !== undefined) {
          if (isNativeStruct(el)) {
            checkImmutable(e, it.kind.value.schema.fields, el, `${path}[${i}]`, root, errs);
          }
        }
      });
    }
    if (kind.case === "map" && kind.value.valueSchema !== undefined) {
      if (cur !== undefined && isNativeStruct(cur)) {
        for (const k of Object.keys(cur).sort()) {
          const el = cur[k];
          if (el !== undefined && isNativeStruct(el)) {
            checkImmutable(e, kind.value.valueSchema.fields, el, joinPath(path, k), root, errs);
          }
        }
      }
    }
  }
}

function defaultOf(f: Schema_Field): Native | undefined {
  return defaultValue(f);
}

function canonicalOrBestFit(f: Schema_Field, x: Native): Value {
  try {
    return canonicalValue(f, x);
  } catch {
    return fromNative(x);
  }
}

function validateFields(
  e: Engine,
  schema: Schema,
  scope: NativeStruct,
  root: NativeStruct,
  prefix: string,
  errs: ValidationError[],
): void {
  const inactive = new Set<string>();
  const declared = new Set<string>();
  for (const f of schema.fields) {
    declared.add(f.name);
    if ((f.when ?? "") !== "" && !fieldIsActive(e, f, root, f.name, undefined)) {
      inactive.add(f.name);
    }
  }

  if (schema.strict) {
    for (const key of Object.keys(scope).sort()) {
      if (!declared.has(key)) {
        errs.push(
          verr(
            joinPath(prefix, key),
            ErrorCode.UNKNOWN_FIELD,
            "strict",
            undefined,
            fromNative(scope[key] ?? null),
          ),
        );
      }
    }
  }

  let present = 0n;
  for (const key of Object.keys(scope)) {
    if (!inactive.has(key)) {
      present += 1n;
    }
  }
  const minProps = schema.minProperties;
  if (minProps !== undefined && present < minProps) {
    errs.push(
      verr(
        prefix,
        ErrorCode.MIN_PROPERTIES_VIOLATED,
        "min_properties",
        uint64V(minProps),
        uint64V(present),
      ),
    );
  }
  const maxProps = schema.maxProperties;
  if (maxProps !== undefined && present > maxProps) {
    errs.push(
      verr(
        prefix,
        ErrorCode.MAX_PROPERTIES_VIOLATED,
        "max_properties",
        uint64V(maxProps),
        uint64V(present),
      ),
    );
  }

  for (const f of schema.fields) {
    if (inactive.has(f.name)) {
      continue;
    }
    validateOne(
      e,
      f,
      scope[f.name] ?? null,
      f.name in scope,
      joinPath(prefix, f.name),
      root,
      undefined,
      errs,
    );
  }
}

function validateOne(
  e: Engine,
  f: Schema_Field,
  val: Native,
  exists: boolean,
  path: string,
  root: NativeStruct,
  index: bigint | undefined,
  errs: ValidationError[],
): void {
  if (!exists) {
    if (f.required) {
      errs.push(verr(path, ErrorCode.REQUIRED_MISSING, "required", undefined, undefined));
    }
    return;
  }
  if (val === null) {
    if (f.required) {
      errs.push(verr(path, ErrorCode.REQUIRED_MISSING, "required", undefined, undefined));
    } else if (!f.nullable) {
      errs.push(verr(path, ErrorCode.NOT_NULLABLE, "nullable", undefined, nullActual()));
    }
    return;
  }

  errs.push(...mask(checkKind(e, f, val, path, root), f.secret));
  for (const r of f.rules) {
    evalRule(e, r, path, val, root, index, errs);
  }
}

function nullActual(): Value {
  return nullV();
}

function evalRule(
  e: Engine,
  r: Schema_Field_Rule,
  path: string,
  thisVal: Native,
  root: NativeStruct,
  index: bigint | undefined,
  errs: ValidationError[],
): void {
  const vars = index === undefined ? { this: thisVal, root } : { this: thisVal, root, index };
  const out = e.eval(r.expr, vars);
  if (!out.ok) {
    const ve = exprErr(path, r.expr, `rule: ${out.error}`);
    if (r.id !== undefined) {
      ve.ruleId = r.id;
    }
    errs.push(ve);
    return;
  }
  if (out.value === true) {
    return;
  }
  const sev =
    r.severity === undefined || r.severity === Schema_Field_Severity.UNSPECIFIED
      ? Schema_Field_Severity.ERROR
      : r.severity;
  const err = create(ValidationErrorSchema, {
    path,
    code: ErrorCode.RULE_VIOLATED,
    expr: r.expr,
    severity: sev,
    message: r.message,
  });
  if (r.id !== undefined) {
    err.ruleId = r.id;
  }
  errs.push(err);
}

// =============================================================================
// Kind dispatch
// =============================================================================

function checkKind(
  e: Engine,
  f: Schema_Field,
  val: Native,
  path: string,
  root: NativeStruct,
): ValidationError[] {
  const kind = f.kind;
  switch (kind.case) {
    case "float": {
      const k = kind.value;
      return checkNumber(path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multipleOf, k.in, k.notIn);
    }
    case "double": {
      const k = kind.value;
      return checkNumber(path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multipleOf, k.in, k.notIn);
    }
    case "int32": {
      const k = kind.value;
      return checkInt(
        path,
        val,
        bigints(k.const),
        bigints(k.gt),
        bigints(k.gte),
        bigints(k.lt),
        bigints(k.lte),
        bigints(k.multipleOf),
        k.in.map(BigInt),
        k.notIn.map(BigInt),
        INT32_MIN,
        INT32_MAX,
      );
    }
    case "int64": {
      const k = kind.value;
      return checkInt(
        path,
        val,
        k.const,
        k.gt,
        k.gte,
        k.lt,
        k.lte,
        k.multipleOf,
        k.in,
        k.notIn,
        INT64_MIN,
        INT64_MAX,
      );
    }
    case "uint32": {
      const k = kind.value;
      return checkUint(
        path,
        val,
        bigints(k.const),
        bigints(k.gt),
        bigints(k.gte),
        bigints(k.lt),
        bigints(k.lte),
        bigints(k.multipleOf),
        k.in.map(BigInt),
        k.notIn.map(BigInt),
        UINT32_MAX,
      );
    }
    case "uint64": {
      const k = kind.value;
      return checkUint(
        path,
        val,
        k.const,
        k.gt,
        k.gte,
        k.lt,
        k.lte,
        k.multipleOf,
        k.in,
        k.notIn,
        UINT64_MAX,
      );
    }
    case "bool": {
      if (typeof val !== "boolean") {
        return [typeErr(path, "bool", val)];
      }
      const c = kind.value.const;
      if (c !== undefined && val !== c) {
        return [verr(path, ErrorCode.CONST_MISMATCH, "const", fromNative(c), fromNative(val))];
      }
      return [];
    }
    case "string":
      return typeof val === "string"
        ? checkString(e, path, val, kind.value)
        : [typeErr(path, "string", val)];
    case "bytes":
      return val instanceof Uint8Array
        ? checkBytes(path, val, kind.value)
        : [typeErr(path, "bytes", val)];
    case "choice":
      return checkChoice(e, path, val, kind.value, root);
    case "duration":
      return checkDuration(path, val, kind.value);
    case "timestamp":
      return checkTimestamp(path, val, kind.value);
    case "list":
      return Array.isArray(val)
        ? checkList(e, path, val, kind.value, root)
        : [typeErr(path, "list", val)];
    case "object": {
      if (!isNativeStruct(val)) {
        return [typeErr(path, "object", val)];
      }
      const sub: ValidationError[] = [];
      const s = kind.value.schema;
      if (s !== undefined) {
        validateFields(e, s, val, root, path, sub);
        for (const r of s.rules) {
          evalRule(e, r, path, val, root, undefined, sub);
        }
      }
      return sub;
    }
    case "map":
      return isNativeStruct(val)
        ? checkMap(e, path, val, kind.value, root)
        : [typeErr(path, "map", val)];
    case "oneOf":
      return isNativeStruct(val)
        ? checkOneOf(e, path, val, kind.value, root)
        : [typeErr(path, "object", val)];
    case "ref":
      return checkRef(e, path, val, kind.value, root);
    default:
      // Computed (derived, rules ran above) and Json (free-form).
      return [];
  }
}

function bigints(v: number | undefined): bigint | undefined {
  return v === undefined ? undefined : BigInt(v);
}

// =============================================================================
// Numeric checks (honest 64-bit)
// =============================================================================

function checkNumber(
  path: string,
  val: Native,
  cst: number | undefined,
  gt: number | undefined,
  gte: number | undefined,
  lt: number | undefined,
  lte: number | undefined,
  mul: number | undefined,
  inSet: number[],
  notIn: number[],
): ValidationError[] {
  const n = asFloat(val);
  if (n === undefined) {
    return [typeErr(path, "number", val)];
  }
  const out: ValidationError[] = [];
  const add = (code: ErrorCode, constraint: string, expected: Value): void => {
    out.push(verr(path, code, constraint, expected, doubleV(n)));
  };
  if (cst !== undefined && n !== cst) {
    add(ErrorCode.CONST_MISMATCH, "const", doubleV(cst));
  }
  if (gt !== undefined && n <= gt) {
    add(ErrorCode.GT_VIOLATED, "gt", doubleV(gt));
  }
  if (gte !== undefined && n < gte) {
    add(ErrorCode.GTE_VIOLATED, "gte", doubleV(gte));
  }
  if (lt !== undefined && n >= lt) {
    add(ErrorCode.LT_VIOLATED, "lt", doubleV(lt));
  }
  if (lte !== undefined && n > lte) {
    add(ErrorCode.LTE_VIOLATED, "lte", doubleV(lte));
  }
  if (inSet.length > 0 && !inSet.includes(n)) {
    add(ErrorCode.NOT_IN_ALLOWED_SET, "in", listV(...inSet.map(doubleV)));
  }
  if (notIn.length > 0 && notIn.includes(n)) {
    add(ErrorCode.IN_FORBIDDEN_SET, "not_in", listV(...notIn.map(doubleV)));
  }
  if (mul !== undefined && mul !== 0 && n % mul !== 0) {
    add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", doubleV(mul));
  }
  return out;
}

function checkInt(
  path: string,
  val: Native,
  cst: bigint | undefined,
  gt: bigint | undefined,
  gte: bigint | undefined,
  lt: bigint | undefined,
  lte: bigint | undefined,
  mul: bigint | undefined,
  inSet: bigint[],
  notIn: bigint[],
  minV: bigint,
  maxV: bigint,
): ValidationError[] {
  const n = asBigInt(val);
  if (n === undefined) {
    return [typeErr(path, "integer", val)];
  }
  if (n < minV || n > maxV) {
    return [typeErr(path, `integer in [${minV}, ${maxV}]`, val)];
  }
  const out: ValidationError[] = [];
  const add = (code: ErrorCode, constraint: string, expected: Value): void => {
    out.push(verr(path, code, constraint, expected, int64V(n)));
  };
  if (cst !== undefined && n !== cst) {
    add(ErrorCode.CONST_MISMATCH, "const", int64V(cst));
  }
  if (gt !== undefined && n <= gt) {
    add(ErrorCode.GT_VIOLATED, "gt", int64V(gt));
  }
  if (gte !== undefined && n < gte) {
    add(ErrorCode.GTE_VIOLATED, "gte", int64V(gte));
  }
  if (lt !== undefined && n >= lt) {
    add(ErrorCode.LT_VIOLATED, "lt", int64V(lt));
  }
  if (lte !== undefined && n > lte) {
    add(ErrorCode.LTE_VIOLATED, "lte", int64V(lte));
  }
  if (inSet.length > 0 && !inSet.includes(n)) {
    add(ErrorCode.NOT_IN_ALLOWED_SET, "in", listV(...inSet.map(int64V)));
  }
  if (notIn.length > 0 && notIn.includes(n)) {
    add(ErrorCode.IN_FORBIDDEN_SET, "not_in", listV(...notIn.map(int64V)));
  }
  if (mul !== undefined && mul !== 0n && n % mul !== 0n) {
    add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", int64V(mul));
  }
  return out;
}

function checkUint(
  path: string,
  val: Native,
  cst: bigint | undefined,
  gt: bigint | undefined,
  gte: bigint | undefined,
  lt: bigint | undefined,
  lte: bigint | undefined,
  mul: bigint | undefined,
  inSet: bigint[],
  notIn: bigint[],
  maxV: bigint,
): ValidationError[] {
  const n = asUnsigned(val);
  if (n === undefined) {
    return [typeErr(path, "unsigned integer", val)];
  }
  if (n > maxV) {
    return [typeErr(path, `unsigned integer <= ${maxV}`, val)];
  }
  const out: ValidationError[] = [];
  const add = (code: ErrorCode, constraint: string, expected: Value): void => {
    out.push(verr(path, code, constraint, expected, uint64V(n)));
  };
  if (cst !== undefined && n !== cst) {
    add(ErrorCode.CONST_MISMATCH, "const", uint64V(cst));
  }
  if (gt !== undefined && n <= gt) {
    add(ErrorCode.GT_VIOLATED, "gt", uint64V(gt));
  }
  if (gte !== undefined && n < gte) {
    add(ErrorCode.GTE_VIOLATED, "gte", uint64V(gte));
  }
  if (lt !== undefined && n >= lt) {
    add(ErrorCode.LT_VIOLATED, "lt", uint64V(lt));
  }
  if (lte !== undefined && n > lte) {
    add(ErrorCode.LTE_VIOLATED, "lte", uint64V(lte));
  }
  if (inSet.length > 0 && !inSet.includes(n)) {
    add(ErrorCode.NOT_IN_ALLOWED_SET, "in", listV(...inSet.map(uint64V)));
  }
  if (notIn.length > 0 && notIn.includes(n)) {
    add(ErrorCode.IN_FORBIDDEN_SET, "not_in", listV(...notIn.map(uint64V)));
  }
  if (mul !== undefined && mul !== 0n && n % mul !== 0n) {
    add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", uint64V(mul));
  }
  return out;
}

// =============================================================================
// String / bytes / choice / time checks
// =============================================================================

function checkString(
  e: Engine,
  path: string,
  s: string,
  k: Schema_Field_String,
): ValidationError[] {
  const out: ValidationError[] = [];
  const add = (code: ErrorCode, constraint: string, expected: Value): void => {
    out.push(verr(path, code, constraint, expected, strV(s)));
  };
  const n = BigInt([...s].length); // rune count

  if (k.const !== undefined && s !== k.const) {
    add(ErrorCode.CONST_MISMATCH, "const", strV(k.const));
  }
  if (k.len !== undefined && n !== k.len) {
    add(ErrorCode.LEN_MISMATCH, "len", uint64V(k.len));
  }
  if (k.minLen !== undefined && n < k.minLen) {
    add(ErrorCode.MIN_LEN_VIOLATED, "min_len", uint64V(k.minLen));
  }
  if (k.maxLen !== undefined && n > k.maxLen) {
    add(ErrorCode.MAX_LEN_VIOLATED, "max_len", uint64V(k.maxLen));
  }
  if (k.pattern !== undefined) {
    const re = e.regexps.get(k.pattern);
    if (re !== undefined && !re.test(s)) {
      add(ErrorCode.PATTERN_MISMATCH, "pattern", strV(k.pattern));
    }
  }
  if (k.in.length > 0 && !k.in.includes(s)) {
    add(ErrorCode.NOT_IN_ALLOWED_SET, "in", listV(...k.in.map(strV)));
  }
  if (k.notIn.length > 0 && k.notIn.includes(s)) {
    add(ErrorCode.IN_FORBIDDEN_SET, "not_in", listV(...k.notIn.map(strV)));
  }
  const format = k.format ?? "";
  if (format !== "") {
    const check = e.formats.get(format as Format);
    if (check === undefined) {
      add(ErrorCode.UNSUPPORTED_FORMAT, "format", strV(format));
    } else if (!check(s)) {
      add(ErrorCode.FORMAT_MISMATCH, "format", strV(format));
    }
  }
  return out;
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((x, i) => b[i] === x);
}

function checkBytes(path: string, b: Uint8Array, k: Schema_Field_Bytes): ValidationError[] {
  const out: ValidationError[] = [];
  const add = (code: ErrorCode, constraint: string, expected: Value): void => {
    out.push(verr(path, code, constraint, expected, bytesV(b)));
  };
  const n = BigInt(b.length);

  if (k.const !== undefined && !bytesEqual(b, k.const)) {
    add(ErrorCode.CONST_MISMATCH, "const", bytesV(k.const));
  }
  if (k.len !== undefined && n !== k.len) {
    add(ErrorCode.LEN_MISMATCH, "len", uint64V(k.len));
  }
  if (k.minLen !== undefined && n < k.minLen) {
    add(ErrorCode.MIN_LEN_VIOLATED, "min_len", uint64V(k.minLen));
  }
  if (k.maxLen !== undefined && n > k.maxLen) {
    add(ErrorCode.MAX_LEN_VIOLATED, "max_len", uint64V(k.maxLen));
  }
  const prefix = k.prefix;
  if (prefix !== undefined && prefix.length > 0 && !bytesEqual(b.slice(0, prefix.length), prefix)) {
    add(ErrorCode.PREFIX_MISMATCH, "prefix", bytesV(prefix));
  }
  const suffix = k.suffix;
  if (
    suffix !== undefined &&
    suffix.length > 0 &&
    !bytesEqual(b.slice(Math.max(0, b.length - suffix.length)), suffix)
  ) {
    add(ErrorCode.SUFFIX_MISMATCH, "suffix", bytesV(suffix));
  }
  if (k.in.length > 0 && !k.in.some((x) => bytesEqual(x, b))) {
    add(ErrorCode.NOT_IN_ALLOWED_SET, "in", listV(...k.in.map(bytesV)));
  }
  if (k.notIn.length > 0 && k.notIn.some((x) => bytesEqual(x, b))) {
    add(ErrorCode.IN_FORBIDDEN_SET, "not_in", listV(...k.notIn.map(bytesV)));
  }
  return out;
}

function checkChoice(
  e: Engine,
  path: string,
  val: Native,
  k: Schema_Field_Choice,
  root: NativeStruct,
): ValidationError[] {
  if (k.open) {
    return [];
  }
  const actual = fromNative(val);
  const src = k.optionsExpr ?? "";
  if (src !== "") {
    const res = e.eval(src, { root });
    if (!res.ok) {
      return [exprErr(path, src, `options_expr: ${res.error}`)];
    }
    const allowed = Array.isArray(res.value) ? res.value : [];
    for (const a of allowed) {
      if (nativeEquals(val, a)) {
        return [];
      }
    }
    return [
      verr(
        path,
        ErrorCode.CHOICE_NOT_ALLOWED,
        "options_expr",
        listV(...allowed.map(fromNative)),
        actual,
      ),
    ];
  }
  const expected: Value[] = [];
  for (const o of k.options) {
    if (nativeEquals(val, toNative(o.value))) {
      return [];
    }
    if (o.value !== undefined) {
      expected.push(o.value);
    }
  }
  return [verr(path, ErrorCode.CHOICE_NOT_ALLOWED, "options", listV(...expected), actual)];
}

function asDurationNative(val: Native): Duration | undefined {
  if (isMessage(val, DurationSchema)) {
    return val;
  }
  if (typeof val === "string") {
    return parseGoDuration(val);
  }
  return undefined;
}

function checkDuration(path: string, val: Native, k: Schema_Field_Duration): ValidationError[] {
  const d = asDurationNative(val);
  if (d === undefined) {
    return [typeErr(path, "duration", val)];
  }
  const out: ValidationError[] = [];
  const n = durationNanos(d);
  const add = (code: ErrorCode, constraint: string, bound: Duration): void => {
    out.push(verr(path, code, constraint, durationV(bound), durationV(d)));
  };
  if (k.gt !== undefined && n <= durationNanos(k.gt)) {
    add(ErrorCode.GT_VIOLATED, "gt", k.gt);
  }
  if (k.gte !== undefined && n < durationNanos(k.gte)) {
    add(ErrorCode.GTE_VIOLATED, "gte", k.gte);
  }
  if (k.lt !== undefined && n >= durationNanos(k.lt)) {
    add(ErrorCode.LT_VIOLATED, "lt", k.lt);
  }
  if (k.lte !== undefined && n > durationNanos(k.lte)) {
    add(ErrorCode.LTE_VIOLATED, "lte", k.lte);
  }
  return out;
}

function asTimestampNative(val: Native): Timestamp | undefined {
  if (isMessage(val, TimestampSchema)) {
    return val;
  }
  if (typeof val === "string") {
    return parseRfc3339(val);
  }
  return undefined;
}

function checkTimestamp(path: string, val: Native, k: Schema_Field_Timestamp): ValidationError[] {
  const ts = asTimestampNative(val);
  if (ts === undefined) {
    return [typeErr(path, "timestamp (RFC3339)", val)];
  }
  const out: ValidationError[] = [];
  const n = timestampNanos(ts);
  const add = (code: ErrorCode, constraint: string, bound: Timestamp): void => {
    out.push(verr(path, code, constraint, timestampV(bound), timestampV(ts)));
  };
  if (k.gt !== undefined && n <= timestampNanos(k.gt)) {
    add(ErrorCode.GT_VIOLATED, "gt", k.gt);
  }
  if (k.gte !== undefined && n < timestampNanos(k.gte)) {
    add(ErrorCode.GTE_VIOLATED, "gte", k.gte);
  }
  if (k.lt !== undefined && n >= timestampNanos(k.lt)) {
    add(ErrorCode.LT_VIOLATED, "lt", k.lt);
  }
  if (k.lte !== undefined && n > timestampNanos(k.lte)) {
    add(ErrorCode.LTE_VIOLATED, "lte", k.lte);
  }
  return out;
}

// =============================================================================
// Container checks
// =============================================================================

function checkList(
  e: Engine,
  path: string,
  arr: Native[],
  l: Schema_Field_List,
  root: NativeStruct,
): ValidationError[] {
  if (isTuple(l)) {
    const out: ValidationError[] = [];
    const want = l.items.length;
    if (arr.length !== want) {
      out.push(
        verr(
          path,
          ErrorCode.LIST_COUNT_MISMATCH,
          "tuple",
          int64V(BigInt(want)),
          int64V(BigInt(arr.length)),
        ),
      );
    }
    const sub: ValidationError[] = [];
    for (let i = 0; i < Math.min(arr.length, want); i++) {
      const item = l.items[i];
      if (item !== undefined) {
        validateOne(e, item, arr[i] ?? null, true, `${path}[${i}]`, root, BigInt(i), sub);
      }
    }
    return [...out, ...sub];
  }

  const out: ValidationError[] = [];
  const n = BigInt(arr.length);
  if (l.minItems !== undefined && n < l.minItems) {
    out.push(
      verr(path, ErrorCode.MIN_ITEMS_VIOLATED, "min_items", uint64V(l.minItems), uint64V(n)),
    );
  }
  if (l.maxItems !== undefined && n > l.maxItems) {
    out.push(
      verr(path, ErrorCode.MAX_ITEMS_VIOLATED, "max_items", uint64V(l.maxItems), uint64V(n)),
    );
  }
  const ce = l.countExpr ?? "";
  if (ce !== "") {
    const res = e.eval(ce, { root });
    const want = res.ok ? asBigInt(res.value) : undefined;
    if (!res.ok) {
      out.push(exprErr(path, ce, `count_expr: ${res.error}`));
    } else if (want === undefined || want < 0n) {
      out.push(exprErr(path, ce, "count_expr: want a non-negative int"));
    } else if (want !== n) {
      out.push(verr(path, ErrorCode.LIST_COUNT_MISMATCH, "count_expr", int64V(want), uint64V(n)));
    }
  }
  if (l.unique) {
    const seen = new Set<string>();
    arr.forEach((el, i) => {
      const key = uniqueKey(el);
      if (seen.has(key)) {
        out.push(verr(`${path}[${i}]`, ErrorCode.NOT_UNIQUE, "unique", undefined, fromNative(el)));
      }
      seen.add(key);
    });
  }
  const item = l.items[0];
  if (l.items.length >= 1 && item !== undefined) {
    const sub: ValidationError[] = [];
    arr.forEach((el, i) => {
      validateOne(e, item, el, true, `${path}[${i}]`, root, BigInt(i), sub);
    });
    out.push(...sub);
  }
  return out;
}

/** Serializes a native value for uniqueness comparison. */
function uniqueKey(v: Native): string {
  return JSON.stringify(v, (_k, x: unknown) => {
    if (typeof x === "bigint") {
      return x <= BigInt(Number.MAX_SAFE_INTEGER) && x >= BigInt(Number.MIN_SAFE_INTEGER)
        ? Number(x)
        : x.toString(10);
    }
    if (x instanceof Uint8Array || isMessage(x, DurationSchema) || isMessage(x, TimestampSchema)) {
      return displayString(x as Native);
    }
    return x;
  });
}

function checkMap(
  e: Engine,
  path: string,
  m: NativeStruct,
  k: Schema_Field_Map,
  root: NativeStruct,
): ValidationError[] {
  const out: ValidationError[] = [];
  const n = BigInt(Object.keys(m).length);
  if (k.minEntries !== undefined && n < k.minEntries) {
    out.push(
      verr(path, ErrorCode.MIN_ENTRIES_VIOLATED, "min_entries", uint64V(k.minEntries), uint64V(n)),
    );
  }
  if (k.maxEntries !== undefined && n > k.maxEntries) {
    out.push(
      verr(path, ErrorCode.MAX_ENTRIES_VIOLATED, "max_entries", uint64V(k.maxEntries), uint64V(n)),
    );
  }
  const vs = k.valueSchema;
  if (vs === undefined) {
    return out;
  }
  for (const key of Object.keys(m).sort()) {
    const vpath = joinPath(path, key);
    const el = m[key] ?? null;
    if (!isNativeStruct(el)) {
      out.push(typeErr(vpath, "object", el));
      continue;
    }
    const sub: ValidationError[] = [];
    validateFields(e, vs, el, root, vpath, sub);
    for (const r of vs.rules) {
      evalRule(e, r, vpath, el, root, undefined, sub);
    }
    out.push(...sub);
  }
  return out;
}

function checkOneOf(
  e: Engine,
  path: string,
  m: NativeStruct,
  oo: Schema_Field_OneOf,
  root: NativeStruct,
): ValidationError[] {
  const disc = m[oo.discriminator];
  if (typeof disc !== "string" || disc === "") {
    return [
      verr(
        path,
        ErrorCode.DISCRIMINATOR_MISSING,
        "discriminator",
        strV(oo.discriminator),
        undefined,
      ),
    ];
  }
  const variant = oo.variants[disc];
  if (variant === undefined) {
    const keys = Object.keys(oo.variants).sort();
    return [
      verr(path, ErrorCode.UNKNOWN_VARIANT, "variants", listV(...keys.map(strV)), strV(disc)),
    ];
  }
  const sub: ValidationError[] = [];
  validateFields(e, variant, m, root, path, sub);
  for (const r of variant.rules) {
    evalRule(e, r, path, m, root, undefined, sub);
  }
  return sub;
}

function checkRef(
  e: Engine,
  path: string,
  val: Native,
  ref: Schema_Field_Ref,
  root: NativeStruct,
): ValidationError[] {
  const key = refDefKey(ref);
  const def = e.schema.defs[key];
  if (def === undefined) {
    let label = key;
    if (ref.target.case === "id") {
      const idv = ref.target.value;
      const ns = idv.namespace !== "" ? `${idv.namespace}/` : "";
      const ver = idv.version !== "" ? `@${idv.version}` : "";
      label = `${ns}${idv.name}${ver} (unlinked identity-ref — call link)`;
    }
    return [verr(path, ErrorCode.UNKNOWN_REF, "ref", strV(label), undefined)];
  }
  if (!isNativeStruct(val)) {
    return [typeErr(path, "object", val)];
  }
  const sub: ValidationError[] = [];
  validateFields(e, def, val, root, path, sub);
  for (const r of def.rules) {
    evalRule(e, r, path, val, root, undefined, sub);
  }
  return sub;
}
