/**
 * Reflect: a zod schema -> a Schema. zod's runtime schemas ARE
 * TypeScript's reflection for erased types, so its own vocabulary becomes
 * the schema: object shapes to Objects, `.optional()` to not-required,
 * `.nullable()`/`.nullish()` to nullable, min/max/length/regex checks to
 * constraints, `z.enum` and literal unions to Choice, `z.email()` and
 * friends to formats, `z.record` to Map (`value_schema` for objects,
 * `value_field` otherwise), `z.lazy` cycles to JSON, `.default(v)` to the
 * field default.
 *
 * TypeScript lacks sized numerics and some schema notions, so this module
 * exports markers to close the gap — applied to the fully-built inner
 * schema: `i32(z.number().int())`, `u32(...)`, `u64(z.bigint())`,
 * `f32(z.number())`, and the leaf helpers `bytesField(...)`,
 * `durationField()`, `timestampField()`.
 *
 * zod is an OPTIONAL peer dependency (only its published `_zod.def` shape
 * is read; nothing from the zod runtime is imported). Everything
 * unrepresentable fails LOUDLY. Supported: zod 4.
 */

import { equals } from "@bufbuild/protobuf";
import type { Schema_Field, SchemaIdentity } from "./gen/schemapb/schema_pb.js";
import { type Schema, Schema_FieldSchema } from "./gen/schemapb/schema_pb.js";
import {
  bool,
  bytes,
  choice,
  double,
  duration,
  FieldB,
  float,
  int32,
  int64,
  json,
  list,
  map,
  mapOf,
  newSchema,
  object,
  str,
  timestamp,
  uint32,
  uint64,
} from "./new.js";

// The minimal structural slice of a zod 4 schema this module reads.
interface ZodLike {
  _zod: { def: ZodDef };
  description?: string;
}

interface ZodDef {
  type: string;
  checks?: ZodCheckLike[];
  format?: string;
  pattern?: RegExp;
  shape?: Record<string, ZodLike>;
  entries?: Record<string, string>;
  options?: ZodLike[];
  values?: unknown[];
  element?: ZodLike;
  items?: ZodLike[];
  keyType?: ZodLike;
  valueType?: ZodLike;
  innerType?: ZodLike;
  getter?: () => ZodLike;
  defaultValue?: unknown;
}

interface ZodCheckLike {
  _zod: {
    def: {
      check: string;
      minimum?: number;
      maximum?: number;
      length?: number;
      value?: number | bigint;
      inclusive?: boolean;
      format?: string;
      pattern?: RegExp;
    };
  };
}

const KIND = Symbol("schemapb.kind");
const BYTES = Symbol("schemapb.bytes");

type Marked = ZodLike & { [KIND]?: string; [BYTES]?: { min?: number; max?: number; len?: number } };

function mark<T>(s: T, kind: string): T {
  (s as Marked)[KIND] = kind;
  return s;
}

/** Marks a fully-built numeric schema as int32. Apply LAST. */
export function i32<T>(s: T): T {
  return mark(s, "int32");
}
/** Marks a fully-built numeric schema as uint32. Apply LAST. */
export function u32<T>(s: T): T {
  return mark(s, "uint32");
}
/** Marks a fully-built bigint schema as uint64. Apply LAST. */
export function u64<T>(s: T): T {
  return mark(s, "uint64");
}
/** Marks a fully-built numeric schema as float32. Apply LAST. */
export function f32<T>(s: T): T {
  return mark(s, "float");
}

/** A bytes leaf (zod has no bytes type). */
export function bytesField(c: { min?: number; max?: number; len?: number } = {}): ZodLike {
  const s = leaf();
  (s as Marked)[KIND] = "bytes";
  (s as Marked)[BYTES] = c;
  return s;
}

/** A duration leaf (google.protobuf.Duration semantics). */
export function durationField(): ZodLike {
  return mark(leaf(), "duration");
}

/** A timestamp leaf (google.protobuf.Timestamp semantics). */
export function timestampField(): ZodLike {
  return mark(leaf(), "timestamp");
}

function leaf(): ZodLike {
  return { _zod: { def: { type: "any" } } };
}

export interface ReflectZodOptions {
  /**
   * Re-describes exact schema INSTANCES (compare by reference) and binds
   * before every default branch, markers included.
   */
  overrides?: Map<unknown, (name: string) => Schema_Field>;
}

class ZodReflectError extends Error {
  constructor(where: string, msg: string) {
    super(`schemapb: reflect zod: field ${where}: ${msg}`);
    this.name = "ZodReflectError";
  }
}

/** Builds a Schema from a zod object schema (coercion enabled on the root). */
export function reflectZod(
  schema: unknown,
  id: SchemaIdentity,
  opts: ReflectZodOptions = {},
): Schema {
  const r = new Reflector(opts.overrides ?? new Map());
  const root = schema as ZodLike;
  if (root._zod?.def?.type !== "object") {
    throw new ZodReflectError("(root)", `root must be a z.object, got ${root._zod?.def?.type}`);
  }
  const fields = r.fieldsOf(root, new Set());
  return newSchema(id)
    .coerce()
    .fields(...fields)
    .build().schema;
}

class Reflector {
  readonly #overrides: Map<unknown, (name: string) => Schema_Field>;

  constructor(overrides: Map<unknown, (name: string) => Schema_Field>) {
    this.#overrides = overrides;
  }

  fieldsOf(obj: ZodLike, visited: Set<ZodLike>): FieldB[] {
    visited = new Set(visited).add(obj);
    const out: FieldB[] = [];
    for (const [name, sub] of Object.entries(obj._zod.def.shape ?? {})) {
      out.push(this.fieldOf(name, sub, visited));
    }
    return out;
  }

  // Unwraps optional/nullable wrappers, then dispatches on kind.
  fieldOf(name: string, s: ZodLike, visited: Set<ZodLike>): FieldB {
    let required = true;
    let nullable = false;

    let cur = s;
    for (;;) {
      const t = cur._zod.def.type;
      if (t === "optional") {
        required = false;
      } else if (t === "nullable") {
        nullable = true;
      } else if (t === "default") {
        // Field defaults live inside each proto kind; a generic value
        // cannot be placed without guessing. Loud, not silent.
        throw new ZodReflectError(
          name,
          "z.default() is not representable; set defaults via the builder",
        );
      } else {
        break;
      }
      const inner = cur._zod.def.innerType;
      if (inner === undefined) {
        break;
      }
      cur = inner;
    }

    const fb = this.baseField(name, cur, visited);
    const done = fb.done();
    if (required) {
      done.required = true;
    }
    if (nullable) {
      done.nullable = true;
    }
    const desc = (s as { description?: string }).description ?? cur.description;
    if (desc !== undefined && desc !== "") {
      done.description = desc;
    }
    return fb;
  }

  baseField(name: string, s: ZodLike, visited: Set<ZodLike>): FieldB {
    const override = this.#overrides.get(s);
    if (override !== undefined) {
      return new Prebuilt(override(name));
    }

    const m = s as Marked;
    const def = s._zod.def;

    if (m[KIND] === "bytes") {
      let bb = bytes(name);
      const c = m[BYTES] ?? {};
      if (c.min !== undefined) {
        bb = bb.minLen(BigInt(c.min));
      }
      if (c.max !== undefined) {
        bb = bb.maxLen(BigInt(c.max));
      }
      if (c.len !== undefined) {
        bb = bb.len(BigInt(c.len));
      }
      return bb;
    }
    if (m[KIND] === "duration") {
      return duration(name);
    }
    if (m[KIND] === "timestamp") {
      return timestamp(name);
    }

    switch (def.type) {
      case "string":
        return this.stringField(name, def);
      case "boolean":
        return bool(name);
      case "number":
        return this.numberField(name, def, m[KIND]);
      case "bigint":
        return this.bigintField(name, def, m[KIND]);
      case "enum":
        return choice(name).strOpts(...Object.values(def.entries ?? {}));
      case "union":
        return this.literalUnion(name, def);
      case "object":
        if (visited.has(s)) {
          return json(name); // a cycle: the shape is not finite
        }
        return object(name, ...this.fieldsOf(s, visited));
      case "array":
        return this.arrayField(name, def, visited);
      case "tuple":
        return this.tupleField(name, def, visited);
      case "record":
        return this.recordField(name, def, visited);
      case "lazy": {
        const resolved = def.getter?.();
        if (resolved === undefined) {
          throw new ZodReflectError(name, "unresolvable z.lazy");
        }
        if (visited.has(resolved)) {
          return json(name);
        }
        return this.baseField(name, resolved, new Set(visited).add(resolved));
      }
      case "any":
      case "unknown":
        return json(name);
      default:
        throw new ZodReflectError(name, `unsupported zod type ${def.type}`);
    }
  }

  stringField(name: string, def: ZodDef): FieldB {
    // z.email() & co. carry the format on the def itself.
    const formats: Record<string, string> = {
      email: "email",
      url: "url",
      uuid: "uuid",
      guid: "uuid",
      ipv4: "ipv4",
      ipv6: "ipv6",
      hostname: "hostname",
    };
    let sb = str(name);
    if (def.format !== undefined && def.format !== "regex") {
      const mapped = formats[def.format];
      if (mapped === undefined) {
        throw new ZodReflectError(name, `unsupported string format ${def.format}`);
      }
      sb = sb.format(mapped);
    }
    for (const c of def.checks ?? []) {
      const cd = c._zod.def;
      switch (cd.check) {
        case "min_length":
          sb = sb.minLen(BigInt(cd.minimum ?? 0));
          break;
        case "max_length":
          sb = sb.maxLen(BigInt(cd.maximum ?? 0));
          break;
        case "length_equals":
          sb = sb.len(BigInt(cd.length ?? 0));
          break;
        case "string_format":
          if (cd.format === "regex" && cd.pattern !== undefined) {
            sb = sb.pattern(cd.pattern.source);
          } else {
            throw new ZodReflectError(name, `unsupported string check format ${cd.format}`);
          }
          break;
        default:
          throw new ZodReflectError(name, `unsupported string check ${cd.check}`);
      }
    }
    return sb;
  }

  numberField(name: string, def: ZodDef, kind: string | undefined): FieldB {
    let isInt = false;
    const bounds: { check: string; value: number; inclusive: boolean }[] = [];
    for (const c of def.checks ?? []) {
      const cd = c._zod.def;
      if (cd.check === "number_format") {
        isInt = true;
      } else if (cd.check === "greater_than" || cd.check === "less_than") {
        bounds.push({ check: cd.check, value: Number(cd.value), inclusive: cd.inclusive ?? false });
      } else {
        throw new ZodReflectError(name, `unsupported number check ${cd.check}`);
      }
    }

    if (kind === "int32" || kind === "uint32") {
      let nb = kind === "int32" ? int32(name) : uint32(name);
      for (const b of bounds) {
        nb = applyBound(nb, b.check, b.value, b.inclusive);
      }
      return nb;
    }

    if (isInt) {
      let nb = int64(name);
      for (const b of bounds) {
        nb = applyBound(nb, b.check, BigInt(b.value), b.inclusive);
      }
      return nb;
    }

    const mk = kind === "float" ? float : double;
    let nb = mk(name);
    for (const b of bounds) {
      nb = applyBound(nb, b.check, b.value, b.inclusive);
    }
    return nb;
  }

  bigintField(name: string, def: ZodDef, kind: string | undefined): FieldB {
    let nb = (kind === "uint64" ? uint64(name) : int64(name)) as ReturnType<typeof int64>;
    for (const c of def.checks ?? []) {
      const cd = c._zod.def;
      if (cd.check === "greater_than" || cd.check === "less_than") {
        nb = applyBound(nb, cd.check, BigInt(cd.value ?? 0), cd.inclusive ?? false);
      } else {
        throw new ZodReflectError(name, `unsupported bigint check ${cd.check}`);
      }
    }
    return nb;
  }

  literalUnion(name: string, def: ZodDef): FieldB {
    const values: unknown[] = [];
    for (const opt of def.options ?? []) {
      const od = opt._zod.def;
      if (od.type !== "literal") {
        throw new ZodReflectError(name, "only literal unions map to Choice");
      }
      values.push(...(od.values ?? []));
    }
    if (values.every((v) => typeof v === "string")) {
      return choice(name).strOpts(...(values as string[]));
    }
    if (values.every((v) => typeof v === "number" && Number.isInteger(v))) {
      return choice(name).intOpts(...values.map((v) => BigInt(v as number)));
    }
    throw new ZodReflectError(name, "Literal options must be all-str or all-int");
  }

  arrayField(name: string, def: ZodDef, visited: Set<ZodLike>): FieldB {
    if (def.element === undefined) {
      throw new ZodReflectError(name, "array without an element type");
    }
    const item = this.fieldOf("item", def.element, visited);
    item.done().required = true;
    let lb = list(name, item);
    for (const c of def.checks ?? []) {
      const cd = c._zod.def;
      if (cd.check === "min_length") {
        lb = lb.minItems(BigInt(cd.minimum ?? 0));
      } else if (cd.check === "max_length") {
        lb = lb.maxItems(BigInt(cd.maximum ?? 0));
      } else {
        throw new ZodReflectError(name, `unsupported array check ${cd.check}`);
      }
    }
    return lb;
  }

  tupleField(name: string, def: ZodDef, visited: Set<ZodLike>): FieldB {
    const items = (def.items ?? []).map((it) => {
      const fb = this.fieldOf("item", it, visited);
      fb.done().required = true;
      return fb;
    });
    if (items.length === 0) {
      throw new ZodReflectError(name, "empty tuple");
    }
    const first = items[0] as FieldB;
    const homogeneous = items.every((it) => equals(Schema_FieldSchema, it.done(), first.done()));
    if (homogeneous && items.length > 0) {
      const n = BigInt(items.length);
      return list(name, first).minItems(n).maxItems(n);
    }
    return list(name, ...items);
  }

  recordField(name: string, def: ZodDef, visited: Set<ZodLike>): FieldB {
    if (def.keyType?._zod.def.type !== "string") {
      throw new ZodReflectError(name, "record keys must be strings (JSON object keys)");
    }
    const val = def.valueType;
    if (val === undefined) {
      throw new ZodReflectError(name, "record without a value type");
    }
    if (val._zod.def.type === "object" && !visited.has(val) && !this.#overrides.has(val)) {
      return map(name, ...this.fieldsOf(val, visited));
    }
    const vf = this.fieldOf("value", val, visited);
    vf.done().required = true;
    return mapOf(name, vf);
  }
}

class Prebuilt extends FieldB {
  constructor(f: Schema_Field) {
    super(f.name);
    Object.assign(this.f, f);
  }
}

function applyBound<
  T extends { gte(v: never): T; gt(v: never): T; lte(v: never): T; lt(v: never): T },
>(nb: T, check: string, value: unknown, inclusive: boolean): T {
  if (check === "greater_than") {
    return inclusive ? nb.gte(value as never) : nb.gt(value as never);
  }
  return inclusive ? nb.lte(value as never) : nb.lt(value as never);
}
