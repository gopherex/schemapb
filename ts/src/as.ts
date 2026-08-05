/**
 * Typed extraction from a wire Value: valueAs reads one value as a native
 * type, keyed by the spec kind string (the type token TypeScript's erased
 * types can offer).
 *
 * One rule, shared by every implementation and pinned by the conformance
 * golden value-as.json: a conversion succeeds iff the value is represented
 * in the target EXACTLY (lossless round-trip). Numeric values convert
 * across kinds under that rule; non-numeric targets are strict — no string
 * parsing, no truncation, no coercion.
 */

import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import type { Value } from "./gen/schemapb/value_pb.js";

const INT64_MAX = 2n ** 63n - 1n;
const UINT64_MAX = 2n ** 64n - 1n;
const INT32_MIN = -(2n ** 31n);
const INT32_MAX = 2n ** 31n - 1n;
const UINT32_MAX = 2n ** 32n - 1n;

/** The extraction targets: the spec kind strings. */
export type ValueAsTarget =
  | "bool"
  | "int32"
  | "int64"
  | "uint32"
  | "uint64"
  | "float"
  | "double"
  | "string"
  | "bytes"
  | "duration"
  | "timestamp"
  | "list"
  | "struct";

/** The exact int64 view of a numeric value. */
function valueInt(v: Value): bigint | undefined {
  const kind = v.kind;
  switch (kind.case) {
    case "int32Value":
    case "uint32Value":
      return BigInt(kind.value);
    case "int64Value":
      return kind.value;
    case "uint64Value":
      return kind.value <= INT64_MAX ? kind.value : undefined;
    case "floatValue":
    case "doubleValue": {
      const f = kind.value;
      if (!Number.isInteger(f) || f < -(2 ** 63) || f >= 2 ** 63) {
        return undefined;
      }
      return BigInt(f);
    }
    default:
      return undefined;
  }
}

/** The exact uint64 view of a numeric value. */
function valueUint(v: Value): bigint | undefined {
  const n = valueIntLike(v);
  return n !== undefined && n >= 0n && n <= UINT64_MAX ? n : undefined;
}

/** Integer view without the int64 clamp (uint64 fits). */
function valueIntLike(v: Value): bigint | undefined {
  const kind = v.kind;
  switch (kind.case) {
    case "int32Value":
    case "uint32Value":
      return BigInt(kind.value);
    case "int64Value":
    case "uint64Value":
      return kind.value;
    case "floatValue":
    case "doubleValue": {
      const f = kind.value;
      if (!Number.isInteger(f) || f < 0 || f >= 2 ** 64) {
        return undefined;
      }
      return BigInt(f);
    }
    default:
      return undefined;
  }
}

/** The exact float64 view of a numeric value. */
function valueDouble(v: Value): number | undefined {
  const kind = v.kind;
  switch (kind.case) {
    case "int32Value":
    case "uint32Value":
    case "floatValue":
    case "doubleValue":
      return kind.value;
    case "int64Value":
    case "uint64Value": {
      const f = Number(kind.value);
      return Number.isFinite(f) && BigInt(f) === kind.value ? f : undefined;
    }
    default:
      return undefined;
  }
}

export function valueAs(v: Value, target: "bool"): boolean | undefined;
export function valueAs(v: Value, target: "int32"): number | undefined;
export function valueAs(v: Value, target: "int64"): bigint | undefined;
export function valueAs(v: Value, target: "uint32"): number | undefined;
export function valueAs(v: Value, target: "uint64"): bigint | undefined;
export function valueAs(v: Value, target: "float"): number | undefined;
export function valueAs(v: Value, target: "double"): number | undefined;
export function valueAs(v: Value, target: "string"): string | undefined;
export function valueAs(v: Value, target: "bytes"): Uint8Array | undefined;
export function valueAs(v: Value, target: "duration"): Duration | undefined;
export function valueAs(v: Value, target: "timestamp"): Timestamp | undefined;
export function valueAs(v: Value, target: "list"): Value[] | undefined;
export function valueAs(v: Value, target: "struct"): Record<string, Value> | undefined;
/**
 * Reads the value as the target kind, `undefined` unless the value is
 * represented in the target exactly.
 */
export function valueAs(v: Value, target: ValueAsTarget): unknown {
  const kind = v.kind;
  switch (target) {
    case "bool":
      return kind.case === "boolValue" ? kind.value : undefined;
    case "int32": {
      const n = valueInt(v);
      return n !== undefined && n >= INT32_MIN && n <= INT32_MAX ? Number(n) : undefined;
    }
    case "int64":
      return valueInt(v);
    case "uint32": {
      const n = valueUint(v);
      return n !== undefined && n <= UINT32_MAX ? Number(n) : undefined;
    }
    case "uint64":
      return valueUint(v);
    case "float": {
      const f = valueDouble(v);
      return f !== undefined && Math.fround(f) === f ? f : undefined;
    }
    case "double":
      return valueDouble(v);
    case "string":
      return kind.case === "stringValue" ? kind.value : undefined;
    case "bytes":
      return kind.case === "bytesValue" ? kind.value : undefined;
    case "duration":
      return kind.case === "durationValue" ? kind.value : undefined;
    case "timestamp":
      return kind.case === "timestampValue" ? kind.value : undefined;
    case "list":
      return kind.case === "listValue" ? kind.value.items : undefined;
    case "struct":
      return kind.case === "structValue" ? kind.value.fields : undefined;
    default:
      return undefined;
  }
}

/** The wire kind of a value as its spec short name. */
export function valueKindName(v: Value): string {
  switch (v.kind.case) {
    case "nullValue":
      return "null";
    case "boolValue":
      return "bool";
    case "int32Value":
      return "int32";
    case "int64Value":
      return "int64";
    case "uint32Value":
      return "uint32";
    case "uint64Value":
      return "uint64";
    case "floatValue":
      return "float";
    case "doubleValue":
      return "double";
    case "stringValue":
      return "string";
    case "bytesValue":
      return "bytes";
    case "durationValue":
      return "duration";
    case "timestampValue":
      return "timestamp";
    case "listValue":
      return "list";
    case "structValue":
      return "struct";
    default:
      return "";
  }
}
