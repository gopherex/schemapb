/**
 * Typed protobuf messages validated against a schemapb Schema: the runtime
 * support of the code protoc-gen-ts-schemapb generates. Mirrors the Go
 * protomsg package; the value mapping is identical:
 *
 *   field name        proto field name (not the JSON name)
 *   implicit presence always present, zero value included
 *   explicit presence present only when set (optional, message, oneof member)
 *   repeated / map    always present, possibly empty; map keys as strings
 *   enum              value name (string); an unknown number stays an integer
 *   integer kinds     bigint       float / double  number
 *   bytes             Uint8Array
 *   Timestamp / Duration          the WKT message
 *   wrappers          the wrapped scalar, absent when unset
 *   Struct / Value / ListValue    their JSON-shaped value
 *   Empty             empty object  FieldMask  list of path strings
 *   Any               "@type" plus the fields of the packed message when its
 *                     type is listed for the validator; only "@type" otherwise
 *
 * Error paths name proto fields; key segments that name a proto field carry
 * their wire field number, and the result carries the schema identity.
 */

import {
  clone,
  create,
  createRegistry,
  type DescField,
  type DescMessage,
  fromBinary,
  fromJson,
  type JsonValue,
  type Message,
  type MessageShape,
  type Registry,
  ScalarType,
  toBinary,
} from "@bufbuild/protobuf";
import { type ReflectMessage, reflect } from "@bufbuild/protobuf/reflect";
import { base64Decode } from "@bufbuild/protobuf/wire";
import {
  type Any,
  anyUnpack,
  DurationSchema,
  FeatureSet_FieldPresence,
  ListValueSchema,
  StructSchema,
  TimestampSchema,
  ValueSchema,
} from "@bufbuild/protobuf/wkt";
import { Engine } from "./engine.js";
import type { ValidationResult } from "./gen/schemapb/errors_pb.js";
import { type Schema, SchemaSchema } from "./gen/schemapb/schema_pb.js";
import type { PathSegment } from "./gen/schemapb/value_pb.js";
import { resultBlocking } from "./validate.js";
import type { Native, NativeStruct } from "./value.js";

/** A message field the value mapping cannot express. */
export class UnsupportedFieldError extends Error {
  constructor(what: string) {
    super(`protomsg: unsupported field type: ${what}`);
    this.name = "UnsupportedFieldError";
  }
}

/** A resolved value that does not fit its proto field. */
export class WriteBackError extends Error {
  constructor(what: string) {
    super(`protomsg: resolved value does not fit the field: ${what}`);
    this.name = "WriteBackError";
  }
}

/** The type URL prefix protobuf runtimes write. */
const anyUrlPrefix = "type.googleapis.com/";

/** The property naming an Any's type URL in the value tree. */
export const anyTypeKey = "@type";

/** A blocking validation outcome, thrown by Validator.validate. */
export class ValidationFailure extends Error {
  readonly result: ValidationResult;

  constructor(result: ValidationResult) {
    const text = result.errors
      .map((e) => (e.path === "" ? e.message : `${e.path}: ${e.message}`))
      .join("; ");
    super(`schemapb: validation failed; ${text}`);
    this.name = "ValidationFailure";
    this.result = result;
  }
}

/**
 * Sets an own enumerable property. Plain assignment would treat a map key
 * or Struct key "__proto__" as the prototype setter: the entry would vanish
 * from the value tree and escape validation.
 */
function setOwn(out: NativeStruct, key: string, value: Native): void {
  Object.defineProperty(out, key, { value, enumerable: true, writable: true, configurable: true });
}

/**
 * Presents a message as the value tree the schemapb engine validates.
 * anyTypes lists the message types an Any may hold and that are unpacked.
 */
export function messageValues<Desc extends DescMessage>(
  desc: Desc,
  msg: MessageShape<Desc>,
  anyTypes: readonly DescMessage[] = [],
): NativeStruct {
  return new Mapping(createRegistry(...anyTypes)).reflected(reflect(desc, msg));
}

const integral = new Set<ScalarType>([
  ScalarType.INT64,
  ScalarType.UINT64,
  ScalarType.INT32,
  ScalarType.FIXED64,
  ScalarType.FIXED32,
  ScalarType.UINT32,
  ScalarType.SFIXED32,
  ScalarType.SFIXED64,
  ScalarType.SINT32,
  ScalarType.SINT64,
]);

const int32Kinds = new Set<ScalarType>([
  ScalarType.INT32,
  ScalarType.SINT32,
  ScalarType.SFIXED32,
  ScalarType.UINT32,
  ScalarType.FIXED32,
]);

const wrappers = new Set([
  "google.protobuf.DoubleValue",
  "google.protobuf.FloatValue",
  "google.protobuf.Int64Value",
  "google.protobuf.UInt64Value",
  "google.protobuf.Int32Value",
  "google.protobuf.UInt32Value",
  "google.protobuf.BoolValue",
  "google.protobuf.StringValue",
  "google.protobuf.BytesValue",
]);

function scalarValue(v: unknown): Native {
  switch (typeof v) {
    case "number":
    case "bigint":
    case "boolean":
    case "string":
      return v;
    default:
      if (v instanceof Uint8Array) {
        return v;
      }
      throw new UnsupportedFieldError(typeof v);
  }
}

function fieldByName(r: ReflectMessage, name: string): DescField {
  const field = r.desc.fields.find((f) => f.name === name);
  if (field === undefined) {
    throw new UnsupportedFieldError(`${r.desc.typeName}.${name}`);
  }
  return field;
}

function structValue(r: ReflectMessage): NativeStruct {
  const field = fieldByName(r, "fields");
  const out: NativeStruct = {};
  if (field.fieldKind === "map") {
    for (const [k, v] of r.get(field)) {
      setOwn(out, String(k), jsonValue(v as ReflectMessage));
    }
  }
  return out;
}

function listValue(r: ReflectMessage): Native[] {
  const field = fieldByName(r, "values");
  return field.fieldKind === "list"
    ? Array.from(r.get(field), (v) => jsonValue(v as ReflectMessage))
    : [];
}

/** google.protobuf.Value; an unset kind is null, as in protoJSON. */
function jsonValue(r: ReflectMessage): Native {
  const kind = r.desc.oneofs[0];
  const field = kind?.fields.find((f) => r.isSet(f));
  if (field === undefined) {
    return null;
  }
  const v = r.get(field);
  switch (field.name) {
    case "number_value":
    case "string_value":
    case "bool_value":
      return v as number | string | boolean;
    case "struct_value":
      return structValue(v as ReflectMessage);
    case "list_value":
      return listValue(v as ReflectMessage);
    default:
      return null;
  }
}

/** One conversion: the registry holds the Any types to unpack. */
class Mapping {
  readonly #anyTypes: Registry;

  constructor(anyTypes: Registry) {
    this.#anyTypes = anyTypes;
  }

  reflected(r: ReflectMessage): NativeStruct {
    const out: NativeStruct = {};
    for (const field of r.fields) {
      const always =
        field.fieldKind === "list" ||
        field.fieldKind === "map" ||
        (field.presence === FeatureSet_FieldPresence.IMPLICIT && field.oneof === undefined);
      if (!always && !r.isSet(field)) {
        continue;
      }
      setOwn(out, field.name, this.#field(field, r));
    }
    return out;
  }

  #field(field: DescField, r: ReflectMessage): Native {
    switch (field.fieldKind) {
      case "list":
        return Array.from(r.get(field), (v) => this.#element(field, v));
      case "map": {
        const out: NativeStruct = {};
        for (const [k, v] of r.get(field)) {
          setOwn(out, String(k), this.#element(field, v));
        }
        return out;
      }
      default:
        return this.#element(field, r.get(field));
    }
  }

  #element(field: DescField, v: unknown): Native {
    if (field.enum !== undefined) {
      const num = v as number;
      return field.enum.value[num]?.name ?? BigInt(num);
    }
    if (field.message !== undefined) {
      return this.#message(v as ReflectMessage);
    }
    // Integer kinds are bigint in the engine's native form; protobuf-es yields
    // number for 32-bit kinds and, with jstype=JS_STRING, string for 64-bit.
    if (field.scalar !== undefined && integral.has(field.scalar)) {
      return BigInt(v as number | bigint | string);
    }
    return scalarValue(v);
  }

  #message(r: ReflectMessage): Native {
    const name = r.desc.typeName;
    const msg = r.message as Message & Record<string, unknown>;
    if (name === "google.protobuf.Timestamp") {
      return create(TimestampSchema, {
        seconds: msg["seconds"] as bigint,
        nanos: msg["nanos"] as number,
      });
    }
    if (name === "google.protobuf.Duration") {
      return create(DurationSchema, {
        seconds: msg["seconds"] as bigint,
        nanos: msg["nanos"] as number,
      });
    }
    if (wrappers.has(name)) {
      const field = r.desc.fields[0] as DescField;
      return this.#element(field, r.get(field));
    }
    switch (name) {
      case "google.protobuf.Struct":
        return structValue(r);
      case "google.protobuf.Value":
        return jsonValue(r);
      case "google.protobuf.ListValue":
        return listValue(r);
      case "google.protobuf.FieldMask":
        return [...(msg["paths"] as string[])];
      case "google.protobuf.Any":
        return this.#any(r.message as Any);
      default:
        return this.reflected(r);
    }
  }

  /** Unpacks an Any of a listed type into its fields plus "@type". */
  #any(any: Any): NativeStruct {
    const out: NativeStruct = {};
    const name = any.typeUrl.startsWith(anyUrlPrefix) ? any.typeUrl.slice(anyUrlPrefix.length) : "";
    const desc = name === "" ? undefined : this.#anyTypes.getMessage(name);
    if (desc !== undefined) {
      const inner = anyUnpack(any, desc);
      if (inner !== undefined) {
        Object.assign(out, this.reflected(reflect(desc, inner)));
      }
    }
    setOwn(out, anyTypeKey, any.typeUrl);
    return out;
  }

  setMessage(r: ReflectMessage, values: NativeStruct): void {
    for (const field of r.fields) {
      if (!Object.hasOwn(values, field.name)) {
        continue;
      }
      const x = values[field.name] ?? null;
      if (x === null) {
        r.clear(field);
        continue;
      }
      switch (field.fieldKind) {
        case "list": {
          if (!Array.isArray(x)) throw misfit(field, x);
          const list = r.get(field);
          const old = Array.from(list);
          list.clear();
          x.forEach((item, i) => {
            if (isObjectMessage(field.message)) {
              const el = (old[i] as ReflectMessage | undefined) ?? reflect(field.message);
              this.#setObject(el, asStruct(field, item));
              list.add(el);
            } else {
              list.add(toProto(field, item));
            }
          });
          break;
        }
        case "map": {
          const entries = asStruct(field, x);
          const mp = r.get(field);
          const old = new Map(mp);
          mp.clear();
          for (const [k, item] of Object.entries(entries)) {
            const key = mapKey(field, k);
            if (field.mapKind === "message" && isObjectMessage(field.message)) {
              const el = (old.get(key) as ReflectMessage | undefined) ?? reflect(field.message);
              this.#setObject(el, asStruct(field, item));
              mp.set(key, el);
            } else {
              mp.set(key, toProto(field, item));
            }
          }
          break;
        }
        case "message":
          if (isObjectMessage(field.message)) {
            const el = r.isSet(field) ? r.get(field) : reflect(field.message);
            this.#setObject(el, asStruct(field, x));
            r.set(field, el);
          } else {
            r.set(field, toProto(field, x));
          }
          break;
        default:
          r.set(field, toProto(field, x));
      }
    }
  }

  /** Writes obj into a plain message, or into the message a listed Any packs. */
  #setObject(el: ReflectMessage, obj: NativeStruct): void {
    if (el.desc.typeName !== anyName) {
      this.setMessage(el, obj);
      return;
    }
    const any = el.message as Any;
    const name = any.typeUrl.startsWith(anyUrlPrefix) ? any.typeUrl.slice(anyUrlPrefix.length) : "";
    const desc = name === "" ? undefined : this.#anyTypes.getMessage(name);
    const inner = desc === undefined ? undefined : anyUnpack(any, desc);
    if (desc === undefined || inner === undefined) {
      return; // only "@type" was seen: nothing resolved
    }
    this.setMessage(reflect(desc, inner), obj);
    any.value = toBinary(desc, inner);
  }
}

// =============================================================================
// Write-back: the resolved value tree into a message
// =============================================================================

/**
 * Stores the resolved values into msg, all or nothing: writes a clone and
 * copies it over msg only when every field converted. Keys absent from
 * values leave their field unchanged. An Any of a listed type is unpacked,
 * written and packed again, so what was validated is what is stored; an Any
 * of any other type was seen only by its "@type" and is left as is.
 */
function writeBack<Desc extends DescMessage>(
  mapping: Mapping,
  desc: Desc,
  msg: MessageShape<Desc>,
  values: NativeStruct,
): void {
  const copy = clone(desc, msg);
  mapping.setMessage(reflect(desc, copy), values);
  Object.assign(msg, copy);
}

const anyName = "google.protobuf.Any";

/** Message kinds written field by field: plain messages, and Any (repacked). */
function isObjectMessage(desc: DescMessage | undefined): desc is DescMessage {
  return (
    desc !== undefined &&
    (desc.file.proto.package !== "google.protobuf" || desc.typeName === anyName)
  );
}

function asStruct(field: DescField, x: Native): NativeStruct {
  if (
    x === null ||
    typeof x !== "object" ||
    Array.isArray(x) ||
    x instanceof Uint8Array ||
    isWkt(x)
  ) {
    throw misfit(field, x);
  }
  return x as NativeStruct;
}

function isWkt(x: object): boolean {
  return "$typeName" in x;
}

function mapKey(field: DescField, k: string): unknown {
  if (field.fieldKind !== "map") return k;
  switch (field.mapKey) {
    case ScalarType.STRING:
      return k;
    case ScalarType.BOOL:
      if (k === "true" || k === "false") return k === "true";
      break;
    default:
      if (/^-?[0-9]+$/.test(k)) {
        return int32Kinds.has(field.mapKey) ? Number(k) : BigInt(k);
      }
  }
  throw new WriteBackError(`${field.toString()} key ${JSON.stringify(k)}`);
}

/** Converts one native value to what ReflectMessage.set/add expects for field. */
function toProto(field: DescField, x: Native): unknown {
  if (field.enum !== undefined) {
    if (typeof x === "string") {
      const v = field.enum.values.find((ev) => ev.name === x);
      if (v !== undefined) return v.number;
    } else if (typeof x === "bigint") {
      return Number(x);
    }
    throw misfit(field, x);
  }
  if (field.message !== undefined) {
    return reflect(field.message, wellKnown(field, field.message, x));
  }
  return scalarToProto(field, field.scalar, x);
}

function scalarToProto(field: DescField, scalar: ScalarType | undefined, x: Native): unknown {
  switch (scalar) {
    case ScalarType.BOOL:
      if (typeof x === "boolean") return x;
      break;
    case ScalarType.STRING:
      if (typeof x === "string") return x;
      break;
    case ScalarType.BYTES:
      if (x instanceof Uint8Array) return x;
      break;
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT:
      if (typeof x === "number") return x;
      if (typeof x === "bigint") return Number(x);
      break;
    default: {
      if (scalar === undefined) break;
      const n =
        typeof x === "bigint"
          ? x
          : typeof x === "number" && Number.isInteger(x)
            ? BigInt(x)
            : undefined;
      if (n === undefined || !fitsInteger(scalar, n)) break;
      return int32Kinds.has(scalar) ? Number(n) : n;
    }
  }
  throw misfit(field, x);
}

function fitsInteger(scalar: ScalarType, n: bigint): boolean {
  switch (scalar) {
    case ScalarType.INT32:
    case ScalarType.SINT32:
    case ScalarType.SFIXED32:
      return n >= -(1n << 31n) && n < 1n << 31n;
    case ScalarType.UINT32:
    case ScalarType.FIXED32:
      return n >= 0n && n < 1n << 32n;
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
      return n >= 0n && n < 1n << 64n;
    default:
      return n >= -(1n << 63n) && n < 1n << 63n;
  }
}

/** Builds a well-known-type message from its natural value. */
function wellKnown(field: DescField, desc: DescMessage, x: Native): Message {
  const name = desc.typeName;
  const isMsg = (want: DescMessage) =>
    x !== null && typeof x === "object" && "$typeName" in x && x.$typeName === want.typeName;
  if (name === "google.protobuf.Timestamp" && isMsg(TimestampSchema)) return x as Message;
  if (name === "google.protobuf.Duration" && isMsg(DurationSchema)) return x as Message;
  if (name === "google.protobuf.Empty") return create(desc);
  if (
    name === "google.protobuf.FieldMask" &&
    Array.isArray(x) &&
    x.every((p) => typeof p === "string")
  ) {
    return create(desc, { paths: x as string[] } as never);
  }
  if (wrappers.has(name)) {
    const inner = desc.fields[0] as DescField;
    return create(desc, { value: scalarToProto(inner, inner.scalar, x) } as never);
  }
  const json = toJsonValue(x);
  switch (name) {
    case "google.protobuf.Struct":
      if (json !== null && typeof json === "object" && !Array.isArray(json))
        return fromJson(StructSchema, json);
      break;
    case "google.protobuf.ListValue":
      if (Array.isArray(json)) return fromJson(ListValueSchema, json);
      break;
    case "google.protobuf.Value":
      return fromJson(ValueSchema, json);
  }
  throw misfit(field, x);
}

/** A JSON-shaped native value as JsonValue (Struct/Value/ListValue). */
function toJsonValue(x: Native): JsonValue {
  if (x === null || typeof x === "string" || typeof x === "boolean" || typeof x === "number")
    return x;
  if (typeof x === "bigint") return Number(x);
  if (Array.isArray(x)) return x.map(toJsonValue);
  if (x instanceof Uint8Array || "$typeName" in x) {
    throw new WriteBackError("non-JSON value in a JSON field");
  }
  const out: { [k: string]: JsonValue } = {};
  for (const [k, v] of Object.entries(x)) {
    Object.defineProperty(out, k, {
      value: toJsonValue(v),
      enumerable: true,
      writable: true,
      configurable: true,
    });
  }
  return out;
}

function misfit(field: DescField, x: Native): WriteBackError {
  const what = x === null ? "null" : Array.isArray(x) ? "list" : typeof x;
  return new WriteBackError(`${field.toString()} cannot hold ${what}`);
}

/** Records the wire field number on each key segment that names a proto field. */
function numberPath(desc: DescMessage, segments: PathSegment[]): void {
  let cur: DescMessage | undefined = desc;
  let container: DescField | undefined;
  for (const seg of segments) {
    if (container !== undefined) {
      cur = elementMessage(container);
      container = undefined;
      continue;
    }
    if (seg.segment.case !== "key" || cur === undefined) {
      return;
    }
    const key: string = seg.segment.value;
    const field: DescField | undefined = cur.fields.find((f) => f.name === key);
    if (field === undefined) {
      return;
    }
    seg.fieldNumber = field.number;
    if (field.fieldKind === "list" || field.fieldKind === "map") {
      container = field;
      cur = undefined;
    } else {
      cur = objectMessage(field.message);
    }
  }
}

function elementMessage(field: DescField): DescMessage | undefined {
  if (field.fieldKind === "map") {
    return field.mapKind === "message" ? objectMessage(field.message) : undefined;
  }
  return objectMessage(field.message);
}

/** Messages that map to objects of their fields: everything but google.protobuf. */
function objectMessage(desc: DescMessage | undefined): DescMessage | undefined {
  return desc === undefined || desc.file.proto.package === "google.protobuf" ? undefined : desc;
}

/**
 * Validates messages of one type against the Schema embedded by the
 * generator. The schema compiles once, on first use.
 */
export class Validator<Desc extends DescMessage> {
  readonly desc: Desc;
  readonly #raw: string;
  readonly #mapping: Mapping;
  #engine: Engine | undefined;

  /**
   * rawSchema is the base64 binary encoding of the Schema; anyTypes the
   * message types an Any field may hold.
   */
  constructor(desc: Desc, rawSchema: string, anyTypes: readonly DescMessage[] = []) {
    this.desc = desc;
    this.#raw = rawSchema;
    this.#mapping = new Mapping(createRegistry(...anyTypes));
  }

  #compiled(): Engine {
    this.#engine ??= Engine.compile(fromBinary(SchemaSchema, base64Decode(this.#raw)));
    return this.#engine;
  }

  /** The embedded schema. Throws SchemaError if it does not compile. */
  get schema(): Schema {
    return this.#compiled().schema;
  }

  /**
   * Validates msg and returns every failure, warnings included. Throws only
   * on programmatic failure: SchemaError, UnsupportedFieldError.
   */
  check(msg: MessageShape<Desc>): ValidationResult {
    return this.#check(msg).res;
  }

  /** Validates msg; the engine resolves the returned values in place. */
  #check(msg: MessageShape<Desc>): { res: ValidationResult; values: NativeStruct } {
    const engine = this.#compiled();
    const values = this.#mapping.reflected(reflect(this.desc, msg));
    const res = engine.validate(values);
    if (engine.schema.id !== undefined) {
      res.schema = engine.schema.id;
    }
    for (const err of res.errors) {
      numberPath(this.desc, err.pathSegments);
    }
    return { res, values };
  }

  /**
   * Validates msg and, when nothing blocks, writes the resolved values
   * (defaults, normalize, computed) back into it. On a blocking result msg
   * is left untouched. Throws WriteBackError (msg unchanged) when a
   * resolved value does not fit its field.
   */
  resolve(msg: MessageShape<Desc>): ValidationResult {
    const { res, values } = this.#check(msg);
    if (!resultBlocking(res)) {
      writeBack(this.#mapping, this.desc, msg, values);
    }
    return res;
  }

  /**
   * Validates msg and throws ValidationFailure when a failure blocks
   * (warnings alone do not). Returns the full result otherwise.
   */
  validate(msg: MessageShape<Desc>): ValidationResult {
    const res = this.check(msg);
    if (resultBlocking(res)) {
      throw new ValidationFailure(res);
    }
    return res;
  }
}
