/**
 * Display formatting (the spec's display forms) and Mustache rendering with
 * the contract render context — see the Go reference render.go. The context
 * is part of the cross-language contract: fields / groups / values with
 * precomputed helper forms; inactive fields are excluded.
 */

import { isMessage } from "@bufbuild/protobuf";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import { formatGoDuration, formatRfc3339 } from "./duration.js";
import type { Schema_Field } from "./gen/schemapb/schema_pb.js";
import { asBigInt, isNativeStruct, type Native, type NativeStruct, toNative } from "./value.js";

/**
 * Renders a native value in the spec's display form: integers in decimal,
 * doubles in JSON number form, bool as true/false, durations in Go form
 * ("5m0s"), timestamps as RFC3339, bytes as std base64, lists and objects as
 * compact JSON with the same leaf forms.
 */
export function displayString(v: Native): string {
  if (v === null) {
    return "";
  }
  if (typeof v === "string") {
    return v;
  }
  if (typeof v === "boolean") {
    return v ? "true" : "false";
  }
  if (typeof v === "bigint") {
    return v.toString(10);
  }
  if (typeof v === "number") {
    return JSON.stringify(v);
  }
  if (v instanceof Uint8Array) {
    return base64(v);
  }
  if (isMessage(v, DurationSchema)) {
    return formatGoDuration(v);
  }
  if (isMessage(v, TimestampSchema)) {
    return formatRfc3339(v);
  }
  return JSON.stringify(displayJson(v));
}

/**
 * Converts non-JSON leaves (duration, timestamp, bytes, bigint) to their
 * display forms so container JSON stays in the spec's leaf forms. Object
 * keys are sorted — Go marshals maps sorted.
 */
function displayJson(v: Native): unknown {
  if (v === null || typeof v === "boolean" || typeof v === "number" || typeof v === "string") {
    return v;
  }
  if (typeof v === "bigint") {
    // Go marshals int64 as a JSON number; stay exact within double range.
    return v >= BigInt(Number.MIN_SAFE_INTEGER) && v <= BigInt(Number.MAX_SAFE_INTEGER)
      ? Number(v)
      : v.toString(10);
  }
  if (v instanceof Uint8Array || isMessage(v, DurationSchema) || isMessage(v, TimestampSchema)) {
    return displayString(v);
  }
  if (Array.isArray(v)) {
    return v.map(displayJson);
  }
  const out: Record<string, unknown> = {};
  for (const key of Object.keys(v).sort()) {
    const el = v[key];
    if (el !== undefined) {
      out[key] = displayJson(el);
    }
  }
  return out;
}

const BASE64_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/** Std base64 without Node Buffer (portable to browsers). */
export function base64(bytes: Uint8Array): string {
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i] ?? 0;
    const b1 = bytes[i + 1];
    const b2 = bytes[i + 2];
    out += BASE64_CHARS[b0 >> 2];
    out += BASE64_CHARS[((b0 & 3) << 4) | ((b1 ?? 0) >> 4)];
    out += b1 === undefined ? "=" : BASE64_CHARS[((b1 & 15) << 2) | ((b2 ?? 0) >> 6)];
    out += b2 === undefined ? "=" : BASE64_CHARS[b2 & 63];
  }
  return out;
}

/** Decodes std base64; undefined on invalid input. */
export function base64Decode(s: string): Uint8Array | undefined {
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(s) || s.length % 4 !== 0) {
    return undefined;
  }
  const clean = s.replace(/=+$/, "");
  const out: number[] = [];
  let bits = 0;
  let acc = 0;
  for (const ch of clean) {
    acc = (acc << 6) | BASE64_CHARS.indexOf(ch);
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      out.push((acc >> bits) & 0xff);
    }
  }
  return new Uint8Array(out);
}

// ASCII-only casing: locale-dependent Unicode casing would break
// cross-implementation determinism.
export function asciiUpper(s: string): string {
  return s.replace(/[a-z]/g, (c) => String.fromCharCode(c.charCodeAt(0) - 32));
}

export function asciiLower(s: string): string {
  return s.replace(/[A-Z]/g, (c) => String.fromCharCode(c.charCodeAt(0) + 32));
}

/** Go strconv.Quote-compatible quoting for the render context. */
export function goQuote(s: string): string {
  return JSON.stringify(s);
}

/** The short kind name used in render contexts. */
export function kindName(f: Schema_Field): string {
  switch (f.kind.case) {
    case "float":
      return "float";
    case "double":
      return "double";
    case "int32":
      return "int32";
    case "int64":
      return "int64";
    case "uint32":
      return "uint32";
    case "uint64":
      return "uint64";
    case "bool":
      return "bool";
    case "string":
      return "string";
    case "bytes":
      return "bytes";
    case "choice":
      return "choice";
    case "duration":
      return "duration";
    case "timestamp":
      return "timestamp";
    case "list":
      return "list";
    case "object":
      return "object";
    case "map":
      return "map";
    case "oneOf":
      return "oneof";
    case "ref":
      return "ref";
    case "computed":
      return "computed";
    case "json":
      return "json";
    default:
      return "";
  }
}

/** One field entry of the contract render context. */
export interface RenderField {
  name: string;
  title: string;
  description: string;
  unit: string;
  group: string;
  kind: string;
  label: string;
  set: boolean;
  computed: boolean;
  secret: boolean;
  immutable: boolean;
  deprecated: boolean;
  value: string;
  onoff: string;
  yesno: string;
  quoted: string;
  upper: string;
  lower: string;
}

export interface RenderContext {
  fields: RenderField[];
  groups: { name: string; fields: RenderField[] }[];
  values: Record<string, string>;
}

/** Builds one render-context field entry (label from a matched choice option). */
export function renderField(f: Schema_Field, values: NativeStruct): RenderField {
  const val = values[f.name];
  const set = f.name in values && val !== null && val !== undefined;
  const display = set ? displayString(val ?? null) : "";
  let label = "";
  if (f.kind.case === "choice" && set) {
    for (const o of f.kind.value.options) {
      if (nativeEquals(val ?? null, toNative(o.value))) {
        label = o.label;
        break;
      }
    }
  }
  const b = typeof val === "boolean" ? val : false;
  return {
    name: f.name,
    title: f.title ?? "",
    description: f.description ?? "",
    unit: f.unit ?? "",
    group: f.group ?? "",
    kind: kindName(f),
    label,
    set,
    computed: f.kind.case === "computed",
    secret: f.secret,
    immutable: f.immutable,
    deprecated: f.deprecated,
    value: display,
    onoff: b ? "on" : "off",
    yesno: b ? "yes" : "no",
    quoted: goQuote(display),
    upper: asciiUpper(display),
    lower: asciiLower(display),
  };
}

/** Structural native equality with cross-numeric comparison (spec). */
export function nativeEquals(a: Native, b: Native): boolean {
  const an = numericOf(a);
  if (an !== undefined) {
    const bn = numericOf(b);
    return bn !== undefined && an === bn;
  }
  if (a === null || b === null) {
    return a === b;
  }
  if (typeof a === "boolean" || typeof a === "string") {
    return a === b;
  }
  if (a instanceof Uint8Array) {
    return b instanceof Uint8Array && a.length === b.length && a.every((x, i) => b[i] === x);
  }
  if (isMessage(a, DurationSchema)) {
    return isMessage(b, DurationSchema) && a.seconds === b.seconds && a.nanos === b.nanos;
  }
  if (isMessage(a, TimestampSchema)) {
    return isMessage(b, TimestampSchema) && a.seconds === b.seconds && a.nanos === b.nanos;
  }
  if (Array.isArray(a)) {
    return (
      Array.isArray(b) && a.length === b.length && a.every((x, i) => nativeEquals(x, b[i] ?? null))
    );
  }
  if (isNativeStruct(a) && isNativeStruct(b)) {
    const ak = Object.keys(a);
    const bk = Object.keys(b);
    return (
      ak.length === bk.length && ak.every((k) => k in b && nativeEquals(a[k] ?? null, b[k] ?? null))
    );
  }
  return false;
}

/** Numeric view for cross-type equality: exact when both are integral. */
function numericOf(v: Native): number | bigint | undefined {
  if (typeof v === "bigint") {
    return v;
  }
  if (typeof v === "number") {
    const i = asBigInt(v);
    return i ?? v;
  }
  return undefined;
}
