/**
 * The resolve pipeline: seed defaults (immutable forced), coerce string
 * inputs, apply normalize expressions, evaluate Computed fields in
 * dependency order. Mirrors the Go reference compute.go.
 */

import { create, isMessage } from "@bufbuild/protobuf";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import { joinPath } from "./descriptor.js";
import { parseGoDuration, parseRfc3339 } from "./duration.js";
import type { Engine } from "./engine.js";
import type { ValidationError } from "./gen/schemapb/errors_pb.js";
import { ErrorCode, ValidationErrorSchema } from "./gen/schemapb/errors_pb.js";
import type {
  Schema,
  Schema_Field,
  Schema_Field_List,
  Schema_Field_OneOf,
  Schema_Field_Ref,
} from "./gen/schemapb/schema_pb.js";
import { Schema_Field_ResultType, Schema_Field_Severity } from "./gen/schemapb/schema_pb.js";
import { base64Decode } from "./render.js";
import {
  asBigInt,
  asFloat,
  asUnsigned,
  isNativeStruct,
  type Native,
  type NativeStruct,
  toNative,
} from "./value.js";

/** Builds a runtime expression-failure ValidationError. */
export function exprErr(path: string, expr: string, msg: string): ValidationError {
  return create(ValidationErrorSchema, {
    path,
    code: ErrorCode.EXPR_ERROR,
    expr,
    severity: Schema_Field_Severity.ERROR,
    message: msg,
  });
}

/** The root-defs lookup key for a Ref. */
export function refDefKey(ref: Schema_Field_Ref): string {
  const target = ref.target;
  if (target.case === "id") {
    const id = target.value;
    return [id.namespace, id.name, id.version].join("\u0000");
  }
  return target.case === "name" ? target.value : "";
}

/** Tuple semantics: several item definitions validate positionally. */
export function isTuple(l: Schema_Field_List): boolean {
  return l.items.length > 1;
}

/** The item definition for element i (homogeneous or positional). */
export function listItemDef(l: Schema_Field_List, i: number): Schema_Field | undefined {
  if (l.items.length === 1) {
    return l.items[0];
  }
  return l.items[i];
}

/** Picks the OneOf variant schema for a value by its discriminator. */
export function selectVariant(
  oo: Schema_Field_OneOf,
  val: Native,
): [Schema, NativeStruct] | undefined {
  if (!isNativeStruct(val)) {
    return undefined;
  }
  const disc = val[oo.discriminator];
  if (typeof disc !== "string" || disc === "") {
    return undefined;
  }
  const variant = oo.variants[disc];
  return variant === undefined ? undefined : [variant, val];
}

interface ComputeTask {
  field: Schema_Field;
  scope: NativeStruct;
  path: string;
}

/**
 * Resolves values in place: defaults, coercion, normalize, computed. Returns
 * the expression failures (empty = clean).
 */
export function resolve(e: Engine, values: NativeStruct): ValidationError[] {
  const errs: ValidationError[] = [];
  const tasks: ComputeTask[] = [];
  seed(e, e.schema, values, "", tasks, values, errs);
  runNormalize(e, e.schema, values, values, errs);
  runCompute(e, values, tasks, errs);
  return errs;
}

/** Evaluates a field's `when` gate; an evaluation error deactivates. */
export function fieldIsActive(
  e: Engine,
  f: Schema_Field,
  root: NativeStruct,
  path: string,
  errs: ValidationError[] | undefined,
): boolean {
  const when = f.when ?? "";
  if (when === "") {
    return true;
  }
  const res = e.evalBool(when, { root });
  if (res.error !== undefined) {
    errs?.push(exprErr(path, when, `when: ${res.error}`));
    return false;
  }
  return res.ok;
}

function seed(
  e: Engine,
  schema: Schema,
  scope: NativeStruct,
  prefix: string,
  tasks: ComputeTask[],
  root: NativeStruct,
  errs: ValidationError[],
): void {
  const coerce = schema.coerce;
  for (const f of schema.fields) {
    const name = f.name;
    const path = joinPath(prefix, name);
    if (!fieldIsActive(e, f, root, path, errs)) {
      continue;
    }
    if (coerce && name in scope) {
      const coerced = coerceInput(f, scope[name] ?? null);
      if (coerced !== undefined) {
        scope[name] = coerced;
      }
    }
    if (f.immutable) {
      const dv = defaultValue(f);
      if (dv !== undefined) {
        scope[name] = dv;
      }
    } else if (!(name in scope)) {
      const dv = defaultValue(f);
      if (dv !== undefined) {
        scope[name] = dv;
      }
    }

    const kind = f.kind;
    const cur = scope[name];
    switch (kind.case) {
      case "computed":
        tasks.push({ field: f, scope, path });
        break;
      case "object": {
        const sub = kind.value.schema;
        if (sub !== undefined && cur !== undefined && isNativeStruct(cur)) {
          seed(e, sub, cur, path, tasks, root, errs);
        }
        break;
      }
      case "list": {
        if (Array.isArray(cur)) {
          cur.forEach((el, i) => {
            const it = listItemDef(kind.value, i);
            if (it?.kind.case === "object" && it.kind.value.schema !== undefined) {
              if (isNativeStruct(el)) {
                seed(e, it.kind.value.schema, el, `${path}[${i}]`, tasks, root, errs);
              }
            }
          });
        }
        break;
      }
      case "map": {
        const vs = kind.value.valueSchema;
        if (vs !== undefined && cur !== undefined && isNativeStruct(cur)) {
          for (const [k, el] of Object.entries(cur)) {
            if (isNativeStruct(el)) {
              seed(e, vs, el, joinPath(path, k), tasks, root, errs);
            }
          }
        }
        break;
      }
      case "oneOf": {
        const sel = selectVariant(kind.value, cur ?? null);
        if (sel !== undefined) {
          seed(e, sel[0], sel[1], path, tasks, root, errs);
        }
        break;
      }
      case "ref": {
        const def = e.schema.defs[refDefKey(kind.value)];
        if (def !== undefined && cur !== undefined && isNativeStruct(cur)) {
          seed(e, def, cur, path, tasks, root, errs);
        }
        break;
      }
      default:
        break;
    }
  }
}

function runNormalize(
  e: Engine,
  schema: Schema,
  scope: NativeStruct,
  root: NativeStruct,
  errs: ValidationError[],
): void {
  for (const f of schema.fields) {
    const name = f.name;
    let cur = scope[name];
    if (cur === undefined || cur === null) {
      continue;
    }
    if (!fieldIsActive(e, f, root, name, undefined)) {
      continue;
    }
    const norm = f.normalize ?? "";
    if (norm !== "") {
      const res = e.eval(norm, { this: cur, root });
      if (!res.ok) {
        errs.push(exprErr(name, norm, `normalize: ${res.error}`));
      } else {
        scope[name] = res.value;
        cur = res.value;
      }
    }
    const kind = f.kind;
    switch (kind.case) {
      case "object":
        if (kind.value.schema !== undefined && isNativeStruct(cur)) {
          runNormalize(e, kind.value.schema, cur, root, errs);
        }
        break;
      case "list":
        if (Array.isArray(cur)) {
          cur.forEach((el, i) => {
            const it = listItemDef(kind.value, i);
            if (it?.kind.case === "object" && it.kind.value.schema !== undefined) {
              if (isNativeStruct(el)) {
                runNormalize(e, it.kind.value.schema, el, root, errs);
              }
            }
          });
        }
        break;
      case "map":
        if (kind.value.valueSchema !== undefined && isNativeStruct(cur)) {
          for (const el of Object.values(cur)) {
            if (isNativeStruct(el)) {
              runNormalize(e, kind.value.valueSchema, el, root, errs);
            }
          }
        }
        break;
      case "oneOf": {
        const sel = selectVariant(kind.value, cur);
        if (sel !== undefined) {
          runNormalize(e, sel[0], sel[1], root, errs);
        }
        break;
      }
      case "ref": {
        const def = e.schema.defs[refDefKey(kind.value)];
        if (def !== undefined && isNativeStruct(cur)) {
          runNormalize(e, def, cur, root, errs);
        }
        break;
      }
      default:
        break;
    }
  }
}

function runCompute(
  e: Engine,
  root: NativeStruct,
  tasks: ComputeTask[],
  errs: ValidationError[],
): void {
  if (tasks.length === 0) {
    return;
  }
  const byPath = new Map(tasks.map((t) => [t.path, t]));
  const deps = new Map<string, string[]>();
  for (const t of tasks) {
    if (t.field.kind.case !== "computed") {
      continue;
    }
    deps.set(
      t.path,
      e.exprDeps(t.field.kind.value.expr).filter((d) => d !== t.path && byPath.has(d)),
    );
  }

  const color = new Map<string, number>();
  const order: ComputeTask[] = [];
  const visit = (p: string): boolean => {
    const c = color.get(p) ?? 0;
    if (c === 1) {
      return false;
    }
    if (c === 2) {
      return true;
    }
    color.set(p, 1);
    for (const d of deps.get(p) ?? []) {
      if (!visit(d)) {
        return false;
      }
    }
    color.set(p, 2);
    const task = byPath.get(p);
    if (task !== undefined) {
      order.push(task);
    }
    return true;
  };
  for (const t of tasks) {
    if (color.get(t.path) !== 2 && !visit(t.path)) {
      errs.push(
        create(ValidationErrorSchema, {
          path: t.path,
          code: ErrorCode.INVALID_SCHEMA,
          severity: Schema_Field_Severity.ERROR,
          message: "computed field cycle",
        }),
      );
    }
  }

  for (const t of order) {
    if (t.field.kind.case !== "computed") {
      continue;
    }
    const c = t.field.kind.value;
    const res = e.eval(c.expr, { root });
    if (!res.ok) {
      errs.push(exprErr(t.path, c.expr, `compute: ${res.error}`));
      continue;
    }
    const shaped = shapeResult(c.result, res.value);
    if (shaped === undefined) {
      errs.push(exprErr(t.path, c.expr, `compute: result does not match declared type`));
      continue;
    }
    t.scope[t.field.name] = shaped;
  }
}

/** Converts a computed result to its declared ResultType's native form. */
export function shapeResult(
  rt: Schema_Field_ResultType | undefined,
  x: Native,
): Native | undefined {
  if (x === null) {
    return null;
  }
  switch (rt) {
    case undefined:
    case Schema_Field_ResultType.UNSPECIFIED:
    case Schema_Field_ResultType.JSON:
      return x;
    case Schema_Field_ResultType.DOUBLE:
      return asFloat(x);
    case Schema_Field_ResultType.INT64:
      return asBigInt(x);
    case Schema_Field_ResultType.UINT64:
      return asUnsigned(x);
    case Schema_Field_ResultType.BOOL:
      return typeof x === "boolean" ? x : undefined;
    case Schema_Field_ResultType.STRING:
      return typeof x === "string" ? x : undefined;
    case Schema_Field_ResultType.DURATION:
      return isMessage(x, DurationSchema) ? x : undefined;
    case Schema_Field_ResultType.TIMESTAMP:
      return isMessage(x, TimestampSchema) ? x : undefined;
    case Schema_Field_ResultType.BYTES:
      return x instanceof Uint8Array ? x : undefined;
    default:
      return undefined;
  }
}

/** Coerces a string input to the field's native type (undefined = unchanged). */
export function coerceInput(f: Schema_Field, val: Native): Native | undefined {
  if (typeof val !== "string") {
    return undefined;
  }
  switch (f.kind.case) {
    case "int32":
    case "int64": {
      if (!/^-?\d+$/.test(val)) {
        return undefined;
      }
      try {
        return BigInt(val);
      } catch {
        return undefined;
      }
    }
    case "uint32":
    case "uint64": {
      if (!/^\d+$/.test(val)) {
        return undefined;
      }
      try {
        return BigInt(val);
      } catch {
        return undefined;
      }
    }
    case "float":
    case "double": {
      const n = Number(val);
      return val.trim() !== "" && !Number.isNaN(n) ? n : undefined;
    }
    case "bool":
      return val === "true" ? true : val === "false" ? false : undefined;
    case "bytes":
      return base64Decode(val);
    case "duration":
      return parseGoDuration(val);
    case "timestamp":
      return parseRfc3339(val);
    default:
      return undefined;
  }
}

/**
 * The allowed values for a top-level choice field given the form: the
 * options_expr result when set, the static option values otherwise.
 */
export function choiceOptions(e: Engine, name: string, root: NativeStruct): Native[] | undefined {
  const f = e.schema.fields.find((x) => x.name === name);
  if (f?.kind.case !== "choice") {
    return undefined;
  }
  const src = f.kind.value.optionsExpr ?? "";
  if (src === "") {
    return f.kind.value.options.map((o) => toNative(o.value));
  }
  const res = e.eval(src, { root });
  return res.ok && Array.isArray(res.value) ? res.value : undefined;
}

/** The required length of a top-level list field per its count_expr. */
export function listCount(e: Engine, name: string, root: NativeStruct): bigint | undefined {
  const f = e.schema.fields.find((x) => x.name === name);
  if (f?.kind.case !== "list") {
    return undefined;
  }
  const ce = f.kind.value.countExpr ?? "";
  if (ce === "") {
    return undefined;
  }
  const res = e.eval(ce, { root });
  if (!res.ok) {
    return undefined;
  }
  const n = asBigInt(res.value);
  return n !== undefined && n >= 0n ? n : undefined;
}

/** A field's default in the native value model (undefined = none). */
export function defaultValue(f: Schema_Field): Native | undefined {
  const kind = f.kind;
  switch (kind.case) {
    case "float":
      return kind.value.default !== undefined ? Math.fround(kind.value.default) : undefined;
    case "double":
      return kind.value.default;
    case "int32":
    case "int64":
      return kind.value.default !== undefined ? BigInt(kind.value.default) : undefined;
    case "uint32":
    case "uint64":
      return kind.value.default !== undefined ? BigInt(kind.value.default) : undefined;
    case "bool":
      return kind.value.default;
    case "string":
      return kind.value.default;
    case "bytes": {
      const d = kind.value.default;
      return d !== undefined && d.length > 0 ? d : undefined;
    }
    case "choice":
      return kind.value.default !== undefined ? toNative(kind.value.default) : undefined;
    case "duration":
      return kind.value.default;
    case "timestamp":
      return kind.value.default;
    case "json":
      return kind.value.default !== undefined ? toNative(kind.value.default) : undefined;
    default:
      return undefined;
  }
}
