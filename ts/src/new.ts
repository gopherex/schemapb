/**
 * The fluent authoring API — TypeScript surface of the Go builder (new.go):
 * kind-first constructors, chainable attribute methods (polymorphic `this`),
 * one generic numeric builder for all six numeric kinds. build() fully
 * compiles the schema: every defect surfaces as SchemaError.
 */

import { create } from "@bufbuild/protobuf";
import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import { Engine } from "./engine.js";
import type { CompileOptions } from "./format.js";
import type {
  Schema,
  Schema_Field,
  Schema_Field_Bytes,
  Schema_Field_Choice,
  Schema_Field_Duration,
  Schema_Field_List,
  Schema_Field_Map,
  Schema_Field_Object,
  Schema_Field_OneOf,
  Schema_Field_Rule,
  Schema_Field_String,
  Schema_Field_Timestamp,
  SchemaIdentity,
} from "./gen/schemapb/schema_pb.js";
import {
  Schema_Field_BoolSchema,
  Schema_Field_BytesSchema,
  Schema_Field_Choice_OptionSchema,
  Schema_Field_ChoiceSchema,
  Schema_Field_ComputedSchema,
  Schema_Field_DoubleSchema,
  Schema_Field_DurationSchema,
  Schema_Field_FloatSchema,
  Schema_Field_Int32Schema,
  Schema_Field_Int64Schema,
  Schema_Field_JsonSchema,
  Schema_Field_ListSchema,
  Schema_Field_MapSchema,
  Schema_Field_ObjectSchema,
  Schema_Field_OneOfSchema,
  Schema_Field_RefSchema,
  Schema_Field_ResultType,
  Schema_Field_RuleSchema,
  Schema_Field_Severity,
  Schema_Field_StringSchema,
  Schema_Field_TimestampSchema,
  Schema_Field_UInt32Schema,
  Schema_Field_UInt64Schema,
  Schema_FieldSchema,
  SchemaSchema,
} from "./gen/schemapb/schema_pb.js";
import type { Value } from "./gen/schemapb/value_pb.js";
import { int64V, strV } from "./value.js";

export const ResultType = Schema_Field_ResultType;
export const Severity = Schema_Field_Severity;

// =============================================================================
// Rule builder
// =============================================================================

export class RuleB {
  readonly r: Schema_Field_Rule;

  constructor(expr: string, message: string) {
    this.r = create(Schema_Field_RuleSchema, { expr, message });
  }

  id(id: string): this {
    this.r.id = id;
    return this;
  }

  warn(): this {
    this.r.severity = Schema_Field_Severity.WARNING;
    return this;
  }

  severity(s: Schema_Field_Severity): this {
    this.r.severity = s;
    return this;
  }

  done(): Schema_Field_Rule {
    return this.r;
  }
}

/** A CEL validation rule; expr must evaluate to bool, true = valid. */
export function rule(expr: string, message: string): RuleB {
  return new RuleB(expr, message);
}

// =============================================================================
// Field base (chainable via polymorphic `this`)
// =============================================================================

export class FieldB {
  readonly f: Schema_Field;

  constructor(name: string) {
    this.f = create(Schema_FieldSchema, { name });
  }

  required(): this {
    this.f.required = true;
    return this;
  }

  nullable(): this {
    this.f.nullable = true;
    return this;
  }

  immutable(): this {
    this.f.immutable = true;
    return this;
  }

  group(g: string): this {
    this.f.group = g;
    return this;
  }

  unit(u: string): this {
    this.f.unit = u;
    return this;
  }

  desc(d: string): this {
    this.f.description = d;
    return this;
  }

  title(t: string): this {
    this.f.title = t;
    return this;
  }

  deprecated(): this {
    this.f.deprecated = true;
    return this;
  }

  secret(): this {
    this.f.secret = true;
    return this;
  }

  examples(...vals: Value[]): this {
    this.f.examples.push(...vals);
    return this;
  }

  rules(...rs: RuleB[]): this {
    this.f.rules.push(...rs.map((r) => r.done()));
    return this;
  }

  normalize(e: string): this {
    this.f.normalize = e;
    return this;
  }

  when(e: string): this {
    this.f.when = e;
    return this;
  }

  done(): Schema_Field {
    return this.f;
  }
}

// =============================================================================
// Numeric kinds — one generic builder for all six
// =============================================================================

/** The shared structural shape of the six numeric kind messages. */
interface NumKind<T> {
  default?: T;
  const?: T;
  gt?: T;
  gte?: T;
  lt?: T;
  lte?: T;
  multipleOf?: T;
  in: T[];
  notIn: T[];
}

export class NumB<T extends number | bigint> extends FieldB {
  readonly #k: NumKind<T>;

  constructor(name: string, k: NumKind<T>) {
    super(name);
    this.#k = k;
  }

  default(v: T): this {
    this.#k.default = v;
    return this;
  }

  const(v: T): this {
    this.#k.const = v;
    return this;
  }

  gt(v: T): this {
    this.#k.gt = v;
    return this;
  }

  gte(v: T): this {
    this.#k.gte = v;
    return this;
  }

  lt(v: T): this {
    this.#k.lt = v;
    return this;
  }

  lte(v: T): this {
    this.#k.lte = v;
    return this;
  }

  in(...vs: T[]): this {
    this.#k.in = vs;
    return this;
  }

  notIn(...vs: T[]): this {
    this.#k.notIn = vs;
    return this;
  }

  multipleOf(v: T): this {
    this.#k.multipleOf = v;
    return this;
  }
}

export function float(name: string): NumB<number> {
  const k = create(Schema_Field_FloatSchema);
  const b = new NumB<number>(name, k);
  b.f.kind = { case: "float", value: k };
  return b;
}

export function double(name: string): NumB<number> {
  const k = create(Schema_Field_DoubleSchema);
  const b = new NumB<number>(name, k);
  b.f.kind = { case: "double", value: k };
  return b;
}

export function int32(name: string): NumB<number> {
  const k = create(Schema_Field_Int32Schema);
  const b = new NumB<number>(name, k);
  b.f.kind = { case: "int32", value: k };
  return b;
}

export function int64(name: string): NumB<bigint> {
  const k = create(Schema_Field_Int64Schema);
  const b = new NumB<bigint>(name, k);
  b.f.kind = { case: "int64", value: k };
  return b;
}

export function uint32(name: string): NumB<number> {
  const k = create(Schema_Field_UInt32Schema);
  const b = new NumB<number>(name, k);
  b.f.kind = { case: "uint32", value: k };
  return b;
}

export function uint64(name: string): NumB<bigint> {
  const k = create(Schema_Field_UInt64Schema);
  const b = new NumB<bigint>(name, k);
  b.f.kind = { case: "uint64", value: k };
  return b;
}

// =============================================================================
// Bool / String / Bytes
// =============================================================================

export class BoolB extends FieldB {
  readonly #k: { default?: boolean; const?: boolean };

  constructor(name: string, k: { default?: boolean; const?: boolean }) {
    super(name);
    this.#k = k;
  }

  default(v: boolean): this {
    this.#k.default = v;
    return this;
  }

  const(v: boolean): this {
    this.#k.const = v;
    return this;
  }
}

export function bool(name: string): BoolB {
  const k = create(Schema_Field_BoolSchema);
  const b = new BoolB(name, k);
  b.f.kind = { case: "bool", value: k };
  return b;
}

export class StrB extends FieldB {
  readonly #k: Schema_Field_String;

  constructor(name: string, k: Schema_Field_String) {
    super(name);
    this.#k = k;
  }

  default(v: string): this {
    this.#k.default = v;
    return this;
  }

  const(v: string): this {
    this.#k.const = v;
    return this;
  }

  len(v: bigint): this {
    this.#k.len = v;
    return this;
  }

  minLen(v: bigint): this {
    this.#k.minLen = v;
    return this;
  }

  maxLen(v: bigint): this {
    this.#k.maxLen = v;
    return this;
  }

  pattern(v: string): this {
    this.#k.pattern = v;
    return this;
  }

  in(...vs: string[]): this {
    this.#k.in = vs;
    return this;
  }

  notIn(...vs: string[]): this {
    this.#k.notIn = vs;
    return this;
  }

  /** A format-registry identifier; unknown formats fail validation loudly. */
  format(v: string): this {
    this.#k.format = v;
    return this;
  }
}

export function str(name: string): StrB {
  const k = create(Schema_Field_StringSchema);
  const b = new StrB(name, k);
  b.f.kind = { case: "string", value: k };
  return b;
}

export class BytesB extends FieldB {
  readonly #k: Schema_Field_Bytes;

  constructor(name: string, k: Schema_Field_Bytes) {
    super(name);
    this.#k = k;
  }

  default(v: Uint8Array): this {
    this.#k.default = v;
    return this;
  }

  const(v: Uint8Array): this {
    this.#k.const = v;
    return this;
  }

  len(v: bigint): this {
    this.#k.len = v;
    return this;
  }

  minLen(v: bigint): this {
    this.#k.minLen = v;
    return this;
  }

  maxLen(v: bigint): this {
    this.#k.maxLen = v;
    return this;
  }

  prefix(v: Uint8Array): this {
    this.#k.prefix = v;
    return this;
  }

  suffix(v: Uint8Array): this {
    this.#k.suffix = v;
    return this;
  }

  in(...vs: Uint8Array[]): this {
    this.#k.in = vs;
    return this;
  }

  notIn(...vs: Uint8Array[]): this {
    this.#k.notIn = vs;
    return this;
  }
}

export function bytes(name: string): BytesB {
  const k = create(Schema_Field_BytesSchema);
  const b = new BytesB(name, k);
  b.f.kind = { case: "bytes", value: k };
  return b;
}

// =============================================================================
// Choice / Duration / Timestamp / Json
// =============================================================================

export class ChoiceB extends FieldB {
  readonly #k: Schema_Field_Choice;

  constructor(name: string, k: Schema_Field_Choice) {
    super(name);
    this.#k = k;
  }

  /** One option: a typed value with a human label (empty = show the value). */
  opt(value: Value, label = ""): this {
    this.#k.options.push(create(Schema_Field_Choice_OptionSchema, { value, label }));
    return this;
  }

  /** String options (value doubles as label). */
  strOpts(...vs: string[]): this {
    for (const v of vs) {
      this.opt(strV(v));
    }
    return this;
  }

  /** Int64 options (value doubles as label). */
  intOpts(...vs: bigint[]): this {
    for (const v of vs) {
      this.opt(int64V(v));
    }
    return this;
  }

  default(v: Value): this {
    this.#k.default = v;
    return this;
  }

  /** Advisory set: values outside it validate fine. */
  open(): this {
    this.#k.open = true;
    return this;
  }

  /** CEL over `root` returning the allowed values (replaces options). */
  options(e: string): this {
    this.#k.optionsExpr = e;
    return this;
  }
}

export function choice(name: string): ChoiceB {
  const k = create(Schema_Field_ChoiceSchema);
  const b = new ChoiceB(name, k);
  b.f.kind = { case: "choice", value: k };
  return b;
}

export class DurationB extends FieldB {
  readonly #k: Schema_Field_Duration;

  constructor(name: string, k: Schema_Field_Duration) {
    super(name);
    this.#k = k;
  }

  default(d: Duration): this {
    this.#k.default = d;
    return this;
  }

  gt(d: Duration): this {
    this.#k.gt = d;
    return this;
  }

  gte(d: Duration): this {
    this.#k.gte = d;
    return this;
  }

  lt(d: Duration): this {
    this.#k.lt = d;
    return this;
  }

  lte(d: Duration): this {
    this.#k.lte = d;
    return this;
  }
}

export function duration(name: string): DurationB {
  const k = create(Schema_Field_DurationSchema);
  const b = new DurationB(name, k);
  b.f.kind = { case: "duration", value: k };
  return b;
}

export class TimestampB extends FieldB {
  readonly #k: Schema_Field_Timestamp;

  constructor(name: string, k: Schema_Field_Timestamp) {
    super(name);
    this.#k = k;
  }

  default(t: Timestamp): this {
    this.#k.default = t;
    return this;
  }

  gt(t: Timestamp): this {
    this.#k.gt = t;
    return this;
  }

  gte(t: Timestamp): this {
    this.#k.gte = t;
    return this;
  }

  lt(t: Timestamp): this {
    this.#k.lt = t;
    return this;
  }

  lte(t: Timestamp): this {
    this.#k.lte = t;
    return this;
  }
}

export function timestamp(name: string): TimestampB {
  const k = create(Schema_Field_TimestampSchema);
  const b = new TimestampB(name, k);
  b.f.kind = { case: "timestamp", value: k };
  return b;
}

export class JsonB extends FieldB {
  readonly #k: { default?: Value };

  constructor(name: string, k: { default?: Value }) {
    super(name);
    this.#k = k;
  }

  default(v: Value): this {
    this.#k.default = v;
    return this;
  }
}

/** Free-form field: any value passes (rules still apply). */
export function json(name: string): JsonB {
  const k = create(Schema_Field_JsonSchema);
  const b = new JsonB(name, k);
  b.f.kind = { case: "json", value: k };
  return b;
}

// =============================================================================
// Containers
// =============================================================================

export class ListB extends FieldB {
  readonly #k: Schema_Field_List;

  constructor(name: string, k: Schema_Field_List) {
    super(name);
    this.#k = k;
  }

  minItems(v: bigint): this {
    this.#k.minItems = v;
    return this;
  }

  maxItems(v: bigint): this {
    this.#k.maxItems = v;
    return this;
  }

  unique(): this {
    this.#k.unique = true;
    return this;
  }

  /** CEL over `root` returning the exact length (`index` binds in items). */
  count(e: string): this {
    this.#k.countExpr = e;
    return this;
  }
}

/**
 * A list field; one item def = homogeneous list, several = positional tuple.
 */
export function list(name: string, ...items: FieldB[]): ListB {
  const k = create(Schema_Field_ListSchema, { items: items.map((i) => i.done()) });
  const b = new ListB(name, k);
  b.f.kind = { case: "list", value: k };
  return b;
}

export class ObjectB extends FieldB {
  readonly #sub: Schema;

  constructor(name: string, k: Schema_Field_Object, sub: Schema) {
    super(name);
    this.#sub = sub;
    k.schema = sub;
  }

  strict(): this {
    this.#sub.strict = true;
    return this;
  }

  minProps(n: bigint): this {
    this.#sub.minProperties = n;
    return this;
  }

  maxProps(n: bigint): this {
    this.#sub.maxProperties = n;
    return this;
  }

  rule(...rs: RuleB[]): this {
    this.#sub.rules.push(...rs.map((r) => r.done()));
    return this;
  }
}

/** A nested object field from its child fields (inline schema). */
export function object(name: string, ...fields: FieldB[]): ObjectB {
  const sub = create(SchemaSchema, { fields: fields.map((f) => f.done()) });
  const k = create(Schema_Field_ObjectSchema);
  const b = new ObjectB(name, k, sub);
  b.f.kind = { case: "object", value: k };
  return b;
}

export class MapB extends FieldB {
  readonly #k: Schema_Field_Map;
  readonly #sub: Schema;

  constructor(name: string, k: Schema_Field_Map, sub: Schema) {
    super(name);
    this.#k = k;
    this.#sub = sub;
    k.valueSchema = sub;
  }

  strict(): this {
    this.#sub.strict = true;
    return this;
  }

  minEntries(n: bigint): this {
    this.#k.minEntries = n;
    return this;
  }

  maxEntries(n: bigint): this {
    this.#k.maxEntries = n;
    return this;
  }

  rule(...rs: RuleB[]): this {
    this.#sub.rules.push(...rs.map((r) => r.done()));
    return this;
  }
}

/** Free-key map field; valueFields describe the shared value schema. */
export function map(name: string, ...valueFields: FieldB[]): MapB {
  const sub = create(SchemaSchema, { fields: valueFields.map((f) => f.done()) });
  const k = create(Schema_Field_MapSchema);
  const b = new MapB(name, k, sub);
  b.f.kind = { case: "map", value: k };
  return b;
}

export class OneOfB extends FieldB {
  readonly #k: Schema_Field_OneOf;

  constructor(name: string, k: Schema_Field_OneOf) {
    super(name);
    this.#k = k;
  }

  variant(key: string, ...fields: FieldB[]): this {
    this.#k.variants[key] = create(SchemaSchema, { fields: fields.map((f) => f.done()) });
    return this;
  }

  variantOf(key: string, s: Schema): this {
    this.#k.variants[key] = s;
    return this;
  }
}

/** A discriminated-union field. */
export function oneOf(name: string, discriminator: string): OneOfB {
  const k = create(Schema_Field_OneOfSchema, { discriminator });
  const b = new OneOfB(name, k);
  b.f.kind = { case: "oneOf", value: k };
  return b;
}

export class ComputedB extends FieldB {
  result(rt: Schema_Field_ResultType): this {
    if (this.f.kind.case === "computed") {
      this.f.kind.value.result = rt;
    }
    return this;
  }
}

/** A derived field; expr reads root (inputs + computed). */
export function computed(name: string, expr: string): ComputedB {
  const b = new ComputedB(name);
  b.f.kind = { case: "computed", value: create(Schema_Field_ComputedSchema, { expr }) };
  return b;
}

/** A Ref field resolving against a local def. */
export function ref(name: string, defName: string): FieldB {
  const b = new FieldB(name);
  b.f.kind = {
    case: "ref",
    value: create(Schema_Field_RefSchema, { target: { case: "name", value: defName } }),
  };
  return b;
}

/** A Ref field targeting a registered schema by identity handle. */
export function refId(name: string, id: SchemaIdentity): FieldB {
  const b = new FieldB(name);
  b.f.kind = {
    case: "ref",
    value: create(Schema_Field_RefSchema, { target: { case: "id", value: id } }),
  };
  return b;
}

// =============================================================================
// Schema builder
// =============================================================================

export class SchemaB {
  readonly s: Schema;

  constructor(id: SchemaIdentity) {
    this.s = create(SchemaSchema, { id });
  }

  descr(d: string): this {
    this.s.description = d;
    return this;
  }

  strict(): this {
    this.s.strict = true;
    return this;
  }

  coerce(): this {
    this.s.coerce = true;
    return this;
  }

  minProps(n: bigint): this {
    this.s.minProperties = n;
    return this;
  }

  maxProps(n: bigint): this {
    this.s.maxProperties = n;
    return this;
  }

  template(name: string, tmpl: string): this {
    this.s.templates[name] = tmpl;
    return this;
  }

  fields(...defs: FieldB[]): this {
    this.s.fields.push(...defs.map((d) => d.done()));
    return this;
  }

  rules(...rs: RuleB[]): this {
    this.s.rules.push(...rs.map((r) => r.done()));
    return this;
  }

  def(name: string, ...fields: FieldB[]): this {
    this.s.defs[name] = create(SchemaSchema, { fields: fields.map((f) => f.done()) });
    return this;
  }

  defSchema(name: string, s: Schema): this {
    this.s.defs[name] = s;
    return this;
  }

  requiredWhen(field: string, cond: string): this {
    return this.rules(
      rule(
        `!(${cond}) || (${JSON.stringify(field)} in root)`,
        `${field} is required when: ${cond}`,
      ).id(field),
    );
  }

  requiredUnless(field: string, cond: string): this {
    return this.rules(
      rule(
        `(${cond}) || (${JSON.stringify(field)} in root)`,
        `${field} is required unless: ${cond}`,
      ).id(field),
    );
  }

  forbiddenWhen(field: string, cond: string): this {
    return this.rules(
      rule(
        `!(${cond}) || !(${JSON.stringify(field)} in root)`,
        `${field} must be absent when: ${cond}`,
      ).id(field),
    );
  }

  /**
   * Fully compiles the schema (descriptor, CEL, patterns, templates,
   * cycles): every defect surfaces here as SchemaError. Returns both the
   * schema and its compiled engine.
   */
  build(opts?: CompileOptions): { schema: Schema; engine: Engine } {
    const engine = Engine.compile(this.s, opts);
    return { schema: this.s, engine };
  }
}

/** Starts a schema with an identity handle (declare the id once — see id()). */
export function newSchema(id: SchemaIdentity): SchemaB {
  return new SchemaB(id);
}
