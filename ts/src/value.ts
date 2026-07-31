/**
 * The single conversion point between the value worlds:
 *
 *   wire    — generated Value / StructValue (the typed protobuf contract)
 *   native  — natural TS runtime values the engine and CEL operate on
 *   schema  — the canonical wire variant dictated by a field's declared kind
 *
 * Native model:
 *
 *   Null       -> null                Bool      -> boolean
 *   Int32/64   -> bigint              UInt32/64 -> bigint (non-negative)
 *   Float/Double -> number            String    -> string
 *   Bytes      -> Uint8Array          Choice    -> the option's native type
 *   Duration   -> wkt Duration        Timestamp -> wkt Timestamp
 *   List       -> Native[]            Object/Map -> { [key]: Native }
 *
 * 64-bit integers are honest bigints — no float64 flattening. The declared
 * field kind narrows a native value back to the exact wire variant at the
 * boundary (canonicalValue).
 */

import { create, isMessage } from "@bufbuild/protobuf";
import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import type { StructValue, Value } from "./gen/schemapb/value_pb.js";
import {
  ListValueSchema,
  NullValue,
  StructValueSchema,
  ValueSchema,
} from "./gen/schemapb/value_pb.js";

export type Native =
  | null
  | boolean
  | bigint
  | number
  | string
  | Uint8Array
  | Duration
  | Timestamp
  | Native[]
  | NativeStruct;

export interface NativeStruct {
  [key: string]: Native;
}

const INT32_MIN = -2147483648n;
const INT32_MAX = 2147483647n;
const UINT32_MAX = 4294967295n;

// =============================================================================
// Constructors (wire values)
// =============================================================================

export function nullV(): Value {
  return create(ValueSchema, { kind: { case: "nullValue", value: NullValue.NULL_VALUE } });
}

export function boolV(v: boolean): Value {
  return create(ValueSchema, { kind: { case: "boolValue", value: v } });
}

export function int32V(v: number): Value {
  return create(ValueSchema, { kind: { case: "int32Value", value: v } });
}

export function int64V(v: bigint): Value {
  return create(ValueSchema, { kind: { case: "int64Value", value: v } });
}

export function uint32V(v: number): Value {
  return create(ValueSchema, { kind: { case: "uint32Value", value: v } });
}

export function uint64V(v: bigint): Value {
  return create(ValueSchema, { kind: { case: "uint64Value", value: v } });
}

export function floatV(v: number): Value {
  return create(ValueSchema, { kind: { case: "floatValue", value: v } });
}

export function doubleV(v: number): Value {
  return create(ValueSchema, { kind: { case: "doubleValue", value: v } });
}

export function strV(v: string): Value {
  return create(ValueSchema, { kind: { case: "stringValue", value: v } });
}

export function bytesV(v: Uint8Array): Value {
  return create(ValueSchema, { kind: { case: "bytesValue", value: v } });
}

export function durationV(v: Duration): Value {
  return create(ValueSchema, { kind: { case: "durationValue", value: v } });
}

export function timestampV(v: Timestamp): Value {
  return create(ValueSchema, { kind: { case: "timestampValue", value: v } });
}

export function listV(...items: Value[]): Value {
  return create(ValueSchema, {
    kind: { case: "listValue", value: create(ListValueSchema, { items }) },
  });
}

export function structV(fields: Record<string, Value>): Value {
  return create(ValueSchema, {
    kind: { case: "structValue", value: create(StructValueSchema, { fields }) },
  });
}

// =============================================================================
// Wire -> native
// =============================================================================

/** Converts a wire value to its native representation. */
export function toNative(v: Value | undefined): Native {
  const kind = v?.kind;
  if (kind === undefined) {
    return null;
  }
  switch (kind.case) {
    case "nullValue":
    case undefined:
      return null;
    case "boolValue":
      return kind.value;
    case "int32Value":
      return BigInt(kind.value);
    case "int64Value":
      return kind.value;
    case "uint32Value":
      return BigInt(kind.value);
    case "uint64Value":
      return kind.value;
    case "floatValue":
      return kind.value;
    case "doubleValue":
      return kind.value;
    case "stringValue":
      return kind.value;
    case "bytesValue":
      return kind.value;
    case "durationValue":
      return kind.value;
    case "timestampValue":
      return kind.value;
    case "listValue":
      return kind.value.items.map(toNative);
    case "structValue":
      return structToNative(kind.value);
  }
}

/** Converts a wire struct to a native map. Absent yields an empty map. */
export function structToNative(s: StructValue | undefined): NativeStruct {
  const out: NativeStruct = {};
  for (const [name, v] of Object.entries(s?.fields ?? {})) {
    out[name] = toNative(v);
  }
  return out;
}

// =============================================================================
// Native -> wire (best fit, no schema)
// =============================================================================

/**
 * Converts a native value to a wire value using the best-fitting variant
 * (int64 for bigints, double for numbers). Use canonicalValue when the field
 * kind is known — it picks the exact contract variant.
 */
export function fromNative(x: Native): Value {
  if (x === null) {
    return nullV();
  }
  if (typeof x === "boolean") {
    return boolV(x);
  }
  if (typeof x === "bigint") {
    return int64V(x);
  }
  if (typeof x === "number") {
    return doubleV(x);
  }
  if (typeof x === "string") {
    return strV(x);
  }
  if (x instanceof Uint8Array) {
    return bytesV(x);
  }
  if (isMessage(x, DurationSchema)) {
    return durationV(x);
  }
  if (isMessage(x, TimestampSchema)) {
    return timestampV(x);
  }
  if (Array.isArray(x)) {
    return listV(...x.map(fromNative));
  }
  const fields: Record<string, Value> = {};
  for (const [name, v] of Object.entries(x)) {
    fields[name] = fromNative(v);
  }
  return structV(fields);
}

/** Converts a native map to a wire struct. */
export function structFromNative(m: NativeStruct): StructValue {
  const fields: Record<string, Value> = {};
  for (const [name, v] of Object.entries(m)) {
    fields[name] = fromNative(v);
  }
  return create(StructValueSchema, { fields });
}

// =============================================================================
// Native numeric coercion helpers (shared by validate/compute)
// =============================================================================

/**
 * Extracts a signed 64-bit integer from any native numeric representation.
 * Numbers convert only when integral.
 */
export function asBigInt(x: Native): bigint | undefined {
  if (typeof x === "bigint") {
    return x;
  }
  if (typeof x === "number" && Number.isInteger(x)) {
    return BigInt(x);
  }
  return undefined;
}

/** Extracts an unsigned integer (non-negative bigint). */
export function asUnsigned(x: Native): bigint | undefined {
  const n = asBigInt(x);
  return n !== undefined && n >= 0n ? n : undefined;
}

/** Extracts a float from any native numeric representation. */
export function asFloat(x: Native): number | undefined {
  if (typeof x === "number") {
    return x;
  }
  if (typeof x === "bigint") {
    return Number(x);
  }
  return undefined;
}

// =============================================================================
// Canonical form (field kind -> exact wire variant)
// =============================================================================

class CanonicalError extends Error {}

function fail(msg: string): never {
  throw new CanonicalError(msg);
}

/**
 * Converts a native value to the exact wire variant the field's declared
 * kind mandates (the contract's canonical form). Throws on values that
 * cannot represent the kind — the validator reports those as TYPE_MISMATCH
 * before this point.
 */
export function canonicalValue(f: Schema_Field, x: Native): Value {
  if (x === null) {
    return nullV();
  }
  const kind = f.kind;
  switch (kind.case) {
    case "float": {
      const n = asFloat(x);
      return n === undefined ? fail(`field ${f.name}: not numeric`) : floatV(Math.fround(n));
    }
    case "double": {
      const n = asFloat(x);
      return n === undefined ? fail(`field ${f.name}: not numeric`) : doubleV(n);
    }
    case "int32": {
      const n = asBigInt(x);
      if (n === undefined || n < INT32_MIN || n > INT32_MAX) {
        fail(`field ${f.name}: does not fit int32`);
      }
      return int32V(Number(n));
    }
    case "int64": {
      const n = asBigInt(x);
      return n === undefined ? fail(`field ${f.name}: not an integer`) : int64V(n);
    }
    case "uint32": {
      const n = asUnsigned(x);
      if (n === undefined || n > UINT32_MAX) {
        fail(`field ${f.name}: does not fit uint32`);
      }
      return uint32V(Number(n));
    }
    case "uint64": {
      const n = asUnsigned(x);
      return n === undefined ? fail(`field ${f.name}: not an unsigned integer`) : uint64V(n);
    }
    case "bool":
      return typeof x === "boolean" ? boolV(x) : fail(`field ${f.name}: not bool`);
    case "string":
      return typeof x === "string" ? strV(x) : fail(`field ${f.name}: not string`);
    case "bytes":
      return x instanceof Uint8Array ? bytesV(x) : fail(`field ${f.name}: not bytes`);
    case "choice":
      return fromNative(x);
    case "duration":
      return isMessage(x, DurationSchema) ? durationV(x) : fail(`field ${f.name}: not a duration`);
    case "timestamp":
      return isMessage(x, TimestampSchema)
        ? timestampV(x)
        : fail(`field ${f.name}: not a timestamp`);
    case "list": {
      if (!Array.isArray(x)) {
        fail(`field ${f.name}: not a list`);
      }
      const items = kind.value.items;
      return listV(
        ...x.map((el, i) => {
          const item = items.length === 1 ? items[0] : items[i];
          return item === undefined ? fromNative(el) : canonicalValue(item, el);
        }),
      );
    }
    case "object": {
      if (!isNativeStruct(x)) {
        fail(`field ${f.name}: not an object`);
      }
      const schema = kind.value.schema;
      return schema === undefined ? fromNative(x) : canonicalStruct(schema, x);
    }
    case "map": {
      if (!isNativeStruct(x)) {
        fail(`field ${f.name}: not a map`);
      }
      const vs = kind.value.valueSchema;
      const fields: Record<string, Value> = {};
      for (const [key, el] of Object.entries(x)) {
        if (vs !== undefined && isNativeStruct(el)) {
          fields[key] = canonicalStruct(vs, el);
        } else {
          fields[key] = fromNative(el);
        }
      }
      return structV(fields);
    }
    case "json":
      return fromNative(x);
    default:
      // Computed / OneOf / Ref values canonicalize structurally; the engine
      // resolves through their target schemas instead.
      return fromNative(x);
  }
}

/** Canonicalizes a native map against a schema's declared fields. */
export function canonicalStruct(s: Schema, m: NativeStruct): Value {
  const fields: Record<string, Value> = {};
  for (const [key, el] of Object.entries(m)) {
    const fld = s.fields.find((f) => f.name === key);
    fields[key] = fld === undefined ? fromNative(el) : canonicalValue(fld, el);
  }
  return structV(fields);
}

/** Narrow helper: a plain native object (not bytes/messages/arrays). */
export function isNativeStruct(x: Native): x is NativeStruct {
  return (
    typeof x === "object" &&
    x !== null &&
    !Array.isArray(x) &&
    !(x instanceof Uint8Array) &&
    !isMessage(x)
  );
}
