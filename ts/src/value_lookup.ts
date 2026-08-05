/**
 * Value path lookup: resolving a path in the ValidationError dialect
 * ("replicas[0].name", "tablespaces.main.location") to the value it
 * addresses inside a StructValue — the path from a validation error
 * fetches the offending value directly.
 *
 * The string form cannot address a map key containing '.' or '[' (the
 * same ambiguity error paths carry); the valueField/valueIndex steppers
 * cover arbitrary keys without parsing.
 */

import { joinPath } from "./descriptor.js";
import type { StructValue, Value } from "./gen/schemapb/value_pb.js";
import { structV } from "./value.js";

/** Stable spec strings shared by all implementations (conformance). */
export type ValueLookupReason =
  | "empty_path"
  | "bad_path"
  | "not_found"
  | "index_out_of_range"
  | "not_a_struct"
  | "not_a_list";

/**
 * Pinpoints the failing segment of a value path: `at` is the resolved
 * parent path ("" for root), `segment` the key or "[i]" index that failed.
 */
export class ValueLookupError extends Error {
  readonly at: string;
  readonly segment: string;
  readonly reason: ValueLookupReason;

  constructor(reason: ValueLookupReason, at = "", segment = "") {
    super(valueLookupMessage(reason, at, segment));
    this.name = "ValueLookupError";
    this.at = at;
    this.segment = segment;
    this.reason = reason;
  }
}

function valueLookupMessage(reason: ValueLookupReason, at: string, segment: string): string {
  const where = at === "" ? "root" : JSON.stringify(at);
  switch (reason) {
    case "empty_path":
      return "schemapb: value lookup: empty path";
    case "bad_path":
      return `schemapb: value lookup: malformed path ${JSON.stringify(segment)}`;
    case "not_found":
      return `schemapb: value lookup: no field ${JSON.stringify(segment)} in ${where}`;
    case "index_out_of_range":
      return `schemapb: value lookup: index ${segment} out of range in ${where}`;
    case "not_a_struct":
      return `schemapb: value lookup: ${where} is not a struct, cannot read field ${JSON.stringify(segment)}`;
    default:
      return `schemapb: value lookup: ${where} is not a list, cannot index ${segment}`;
  }
}

/**
 * Steps into a struct value member; `undefined` when the value is not a
 * struct or has no such field. Handles keys the string path cannot spell.
 */
export function valueField(v: Value, name: string): Value | undefined {
  return v.kind.case === "structValue" ? v.kind.value.fields[name] : undefined;
}

/**
 * Steps into a list value element; `undefined` when the value is not a
 * list or the index is out of range.
 */
export function valueIndex(v: Value, i: number): Value | undefined {
  return v.kind.case === "listValue" ? v.kind.value.items[i] : undefined;
}

type PathToken = { key: string } | { index: number };

/** Tokenizes the error-path dialect: key ('.' key | '[' int ']')*. */
function parseValuePath(path: string): PathToken[] | undefined {
  const tokens: PathToken[] = [];
  let rest = path;

  while (rest !== "") {
    if (rest.startsWith("[")) {
      if (tokens.length === 0) {
        return undefined; // paths start with a key, not an index
      }
      const end = rest.indexOf("]");
      // digits only: no "+3", "-0", "[]"
      if (end < 2 || !/^\d+$/.test(rest.slice(1, end))) {
        return undefined;
      }
      tokens.push({ index: Number(rest.slice(1, end)) });
      rest = rest.slice(end + 1);
      // After "]" only ".", "[" or the end may follow.
      if (rest !== "" && !rest.startsWith(".") && !rest.startsWith("[")) {
        return undefined;
      }
      continue;
    }

    if (rest.startsWith(".")) {
      if (tokens.length === 0) {
        return undefined; // leading dot
      }
      rest = rest.slice(1);
      // A dot must be followed by a key: no trailing dot, "a..b", "a.[0]".
      if (rest === "" || rest.startsWith(".") || rest.startsWith("[")) {
        return undefined;
      }
      continue;
    }

    let end = rest.length;
    for (let i = 0; i < rest.length; i++) {
      if (rest[i] === "." || rest[i] === "[") {
        end = i;
        break;
      }
    }
    tokens.push({ key: rest.slice(0, end) });
    rest = rest.slice(end);
  }

  return tokens.length > 0 ? tokens : undefined;
}

/**
 * Resolves a path in the ValidationError dialect against the struct's
 * values. Returns the addressed value or throws a ValueLookupError naming
 * the exact segment that failed.
 */
export function lookupValue(sv: StructValue, path: string): Value {
  if (path === "") {
    throw new ValueLookupError("empty_path");
  }

  const tokens = parseValuePath(path);
  if (tokens === undefined) {
    throw new ValueLookupError("bad_path", "", path);
  }

  let cur = structV(sv.fields);
  let parent = "";

  for (const tok of tokens) {
    if ("key" in tok) {
      if (cur.kind.case !== "structValue") {
        throw new ValueLookupError("not_a_struct", parent, tok.key);
      }
      const next: Value | undefined = cur.kind.value.fields[tok.key];
      if (next === undefined) {
        throw new ValueLookupError("not_found", parent, tok.key);
      }
      cur = next;
      parent = joinPath(parent, tok.key);
      continue;
    }

    const seg = `[${tok.index}]`;
    if (cur.kind.case !== "listValue") {
      throw new ValueLookupError("not_a_list", parent, seg);
    }
    const next: Value | undefined = cur.kind.value.items[tok.index];
    if (next === undefined) {
      throw new ValueLookupError("index_out_of_range", parent, seg);
    }
    cur = next;
    parent += seg;
  }

  return cur;
}
