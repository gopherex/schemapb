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
 *   Any               unsupported (UnsupportedFieldError)
 *
 * Error paths name proto fields; key segments that name a proto field carry
 * their wire field number, and the result carries the schema identity.
 */

import {
  create,
  type DescField,
  type DescMessage,
  fromBinary,
  type Message,
  type MessageShape,
  ScalarType,
} from "@bufbuild/protobuf";
import { type ReflectMessage, reflect } from "@bufbuild/protobuf/reflect";
import { base64Decode } from "@bufbuild/protobuf/wire";
import { DurationSchema, FeatureSet_FieldPresence, TimestampSchema } from "@bufbuild/protobuf/wkt";
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

/** Presents a message as the value tree the schemapb engine validates. */
export function messageValues<Desc extends DescMessage>(
  desc: Desc,
  msg: MessageShape<Desc>,
): NativeStruct {
  return reflectedValues(reflect(desc, msg));
}

function reflectedValues(r: ReflectMessage): NativeStruct {
  const out: NativeStruct = {};
  for (const field of r.fields) {
    const always =
      field.fieldKind === "list" ||
      field.fieldKind === "map" ||
      (field.presence === FeatureSet_FieldPresence.IMPLICIT && field.oneof === undefined);
    if (!always && !r.isSet(field)) {
      continue;
    }
    out[field.name] = fieldValue(field, r);
  }
  return out;
}

function fieldValue(field: DescField, r: ReflectMessage): Native {
  switch (field.fieldKind) {
    case "list": {
      const list = r.get(field);
      return Array.from(list, (v) => elementValue(field, v));
    }
    case "map": {
      const out: NativeStruct = {};
      for (const [k, v] of r.get(field)) {
        out[String(k)] = elementValue(field, v);
      }
      return out;
    }
    default:
      return elementValue(field, r.get(field));
  }
}

function elementValue(field: DescField, v: unknown): Native {
  if (field.enum !== undefined) {
    const num = v as number;
    return field.enum.value[num]?.name ?? BigInt(num);
  }
  if (field.message !== undefined) {
    return messageValue(v as ReflectMessage);
  }
  // Integer kinds are bigint in the engine's native form; protobuf-es yields
  // number for 32-bit kinds and, with jstype=JS_STRING, string for 64-bit.
  if (field.scalar !== undefined && integral.has(field.scalar)) {
    return BigInt(v as number | bigint | string);
  }
  return scalarValue(v);
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

function scalarValue(v: unknown): Native {
  switch (typeof v) {
    case "number":
    case "bigint":
    case "boolean":
      return v;
    case "string":
      return v;
    default:
      if (v instanceof Uint8Array) {
        return v;
      }
      throw new UnsupportedFieldError(typeof v);
  }
}

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

function messageValue(r: ReflectMessage): Native {
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
    return elementValue(field, r.get(field));
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
      throw new UnsupportedFieldError(name);
    default:
      return reflectedValues(r);
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
      out[String(k)] = jsonValue(v as ReflectMessage);
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
  #engine: Engine | undefined;

  /** rawSchema is the base64 binary encoding of the Schema. */
  constructor(desc: Desc, rawSchema: string) {
    this.desc = desc;
    this.#raw = rawSchema;
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
    const engine = this.#compiled();
    const values = messageValues(this.desc, msg);
    const res = engine.validate(values);
    if (engine.schema.id !== undefined) {
      res.schema = engine.schema.id;
    }
    for (const err of res.errors) {
      numberPath(this.desc, err.pathSegments);
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
