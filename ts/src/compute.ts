/**
 * The resolve pipeline: seed defaults (immutable forced), coerce string
 * inputs, apply normalize expressions, evaluate Computed fields in
 * dependency order. Mirrors the Go reference compute.go.
 */

import { clone, create, isMessage } from "@bufbuild/protobuf";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import { joinPath } from "./descriptor.js";
import { parseGoDuration, parseRfc3339 } from "./duration.js";
import type { Engine } from "./engine.js";
import type { ValidationError } from "./gen/schemapb/errors_pb.js";
import { ErrorCode, ValidationErrorSchema } from "./gen/schemapb/errors_pb.js";
import {
  ResolveEventSchema,
  ResolveOperation,
  type ResolveReport,
} from "./gen/schemapb/runtime_pb.js";
import type {
  Schema,
  Schema_Field,
  Schema_Field_List,
  Schema_Field_OneOf,
  Schema_Field_Ref,
} from "./gen/schemapb/schema_pb.js";
import { Schema_Field_ResultType, Schema_Field_Severity } from "./gen/schemapb/schema_pb.js";
import { ValueSchema } from "./gen/schemapb/value_pb.js";
import { pathSegments, sortedKeys } from "./path.js";
import { base64Decode, nativeEquals } from "./render.js";
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

/** The schema of a present Object, Ref or selected OneOf (also list/tuple items). */
export function objectSchema(
  f: Schema_Field,
  val: Native,
  defs: Record<string, Schema>,
): [Schema, NativeStruct] | undefined {
  if (!isNativeStruct(val)) return undefined;
  const k = f.kind;
  if (k.case === "oneOf") return selectVariant(k.value, val);
  const sub =
    k.case === "object" ? k.value.schema : k.case === "ref" ? defs[refDefKey(k.value)] : undefined;
  return sub === undefined ? undefined : [sub, val];
}

interface ComputeTask {
  field: Schema_Field;
  set: (value: Native) => void;
  path: string;
}

/**
 * Resolves values in place: defaults, coercion, normalize, computed. Returns
 * the expression failures (empty = clean).
 */
export function resolve(
  e: Engine,
  values: NativeStruct,
  report?: ResolveReport,
): ValidationError[] {
  const state = new Resolver(e, values, report);
  state.object(e.schema, values, "", false, false);
  state.object(e.schema, values, "", false, true);
  runCompute(e, values, state.tasks, state.errs, report);
  for (const err of state.errs) err.pathSegments = pathSegments(err.path);
  return state.errs;
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

class Resolver {
  readonly errs: ValidationError[] = [];
  readonly tasks: ComputeTask[] = [];
  readonly gates = new Map<string, boolean>();
  readonly gateErrors = new Set<string>();
  readonly inactive = new Set<string>();
  constructor(
    readonly engine: Engine,
    readonly root: NativeStruct,
    readonly report?: ResolveReport,
  ) {}

  object(
    schema: Schema,
    scope: NativeStruct,
    path: string,
    inherited: boolean,
    normalize: boolean,
  ): void {
    const coerce = inherited || schema.coerce;
    for (const f of schema.fields)
      this.field(
        f,
        scope[f.name] ?? null,
        Object.hasOwn(scope, f.name),
        (value) => {
          Object.defineProperty(scope, f.name, {
            value,
            writable: true,
            enumerable: true,
            configurable: true,
          });
        },
        joinPath(path, f.name),
        coerce,
        normalize,
      );
  }
  field(
    f: Schema_Field,
    cur: Native,
    present: boolean,
    set: (value: Native) => void,
    path: string,
    coerce: boolean,
    normalize: boolean,
  ): void {
    const seeded = this.gates.get(path);
    if (!this.active(f, path)) return;
    if (normalize && !seeded) {
      this.field(
        f,
        cur,
        present,
        (v) => {
          cur = v;
          present = true;
          set(v);
        },
        path,
        coerce,
        false,
      );
    }
    if (!normalize) {
      if (present && coerce) {
        const out = coerceInput(f, cur);
        if (out !== undefined) {
          cur = out;
          set(cur);
          recordResolve(this.report, path, ResolveOperation.COERCED);
        }
      }
      if (!present || f.immutable) {
        const out = defaultValue(f);
        if (out !== undefined) {
          const changed = !present || !nativeEquals(cur, out);
          cur = out;
          present = true;
          set(cur);
          if (changed) recordResolve(this.report, path, ResolveOperation.DEFAULT_APPLIED);
        }
      }
    } else {
      if (present && cur !== null && f.normalize) {
        const out = this.engine.eval(f.normalize, { this: cur, root: this.root });
        if (!out.ok) this.errs.push(exprErr(path, f.normalize, `normalize: ${out.error}`));
        else {
          const changed = !nativeEquals(cur, out.value);
          cur = out.value;
          set(cur);
          if (changed) {
            recordResolve(this.report, path, ResolveOperation.NORMALIZED);
            this.children(f, cur, path, coerce, false);
          }
        }
      }
      if (f.kind.case === "computed") {
        this.tasks.push({ field: f, set, path });
        return;
      }
    }
    if (present && cur !== null) this.children(f, cur, path, coerce, normalize);
  }
  active(f: Schema_Field, path: string): boolean {
    const errors: ValidationError[] = [];
    const active = fieldIsActive(this.engine, f, this.root, path, errors);
    const key = JSON.stringify([path, f.when]);
    if (errors.length && !this.gateErrors.has(key)) {
      this.errs.push(...errors);
      this.gateErrors.add(key);
    }
    this.gates.set(path, active);
    if (!active && !this.inactive.has(path)) {
      recordResolve(this.report, path, ResolveOperation.INACTIVE);
      this.inactive.add(path);
    }
    return active;
  }
  children(f: Schema_Field, cur: Native, path: string, coerce: boolean, normalize: boolean): void {
    const sub = objectSchema(f, cur, this.engine.schema.defs);
    if (sub) {
      this.object(sub[0], sub[1], path, coerce, normalize);
      return;
    }
    const k = f.kind;
    if (k.case === "list" && Array.isArray(cur))
      cur.forEach((value, i) => {
        const item = listItemDef(k.value, i);
        if (item)
          this.field(
            item,
            value,
            true,
            (v) => {
              cur[i] = v;
            },
            `${path}[${i}]`,
            coerce,
            normalize,
          );
      });
    if (k.case === "map" && isNativeStruct(cur))
      for (const key of sortedKeys(cur)) {
        const value = cur[key] ?? null,
          childPath = joinPath(path, key);
        if (k.value.valueField)
          this.field(
            k.value.valueField,
            value,
            true,
            (v) => {
              cur[key] = v;
            },
            childPath,
            coerce,
            normalize,
          );
        else if (k.value.valueSchema && isNativeStruct(value))
          this.object(k.value.valueSchema, value, childPath, coerce, normalize);
      }
  }
}

function recordResolve(
  report: ResolveReport | undefined,
  path: string,
  operation: ResolveOperation,
): void {
  report?.events.push(
    create(ResolveEventSchema, { path, pathSegments: pathSegments(path), operation }),
  );
}

function runCompute(
  e: Engine,
  root: NativeStruct,
  tasks: ComputeTask[],
  errs: ValidationError[],
  report?: ResolveReport,
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
    t.set(shaped);
    recordResolve(report, t.path, ResolveOperation.COMPUTED);
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
    case "object":
    case "ref":
      return kind.value.default === undefined ? undefined : {};
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
      return d !== undefined && d.length > 0 ? d.slice() : undefined;
    }
    case "choice":
      return kind.value.default !== undefined
        ? toNative(clone(ValueSchema, kind.value.default))
        : undefined;
    case "duration":
      return kind.value.default === undefined
        ? undefined
        : clone(DurationSchema, kind.value.default);
    case "timestamp":
      return kind.value.default === undefined
        ? undefined
        : clone(TimestampSchema, kind.value.default);
    case "json":
      return kind.value.default !== undefined
        ? toNative(clone(ValueSchema, kind.value.default))
        : undefined;
    default:
      return undefined;
  }
}
