/**
 * Schema path lookup: resolving a dot path ("a.b.c") to the field it
 * addresses. Paths address FIELDS, not values, so they carry no list
 * indices or map keys: lookup descends through Object fields and resolves
 * Refs against root $defs; every other kind is terminal — a path may END
 * on a list/map/oneof field but cannot continue through one.
 *
 * Failures point at the exact segment that broke ("no field b in a",
 * never "a.b.c not found").
 */

import { refDefKey } from "./compute.js";
import { joinPath } from "./descriptor.js";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import { kindName } from "./render.js";

/** Stable spec strings shared by all implementations (conformance). */
export type LookupReason =
  | "empty_path"
  | "not_found"
  | "not_traversable"
  | "ambiguous_oneof"
  | "unknown_ref";

/**
 * Pinpoints the failing segment of a schema path: `at` is the resolved
 * parent path ("" for root), `segment` the name that failed, `kind` the
 * kind of the offending field (set for the traversal reasons).
 */
export class LookupError extends Error {
  readonly at: string;
  readonly segment: string;
  readonly reason: LookupReason;
  readonly kind: string;

  constructor(reason: LookupReason, at = "", segment = "", kind = "") {
    super(lookupMessage(reason, at, segment, kind));
    this.name = "LookupError";
    this.at = at;
    this.segment = segment;
    this.reason = reason;
    this.kind = kind;
  }
}

function lookupMessage(reason: LookupReason, at: string, segment: string, kind: string): string {
  const where = at === "" ? "root" : JSON.stringify(at);
  switch (reason) {
    case "empty_path":
      return "schemapb: lookup: empty path";
    case "not_found":
      return `schemapb: lookup: no field ${JSON.stringify(segment)} in ${where}`;
    case "ambiguous_oneof":
      return `schemapb: lookup: cannot descend into oneof ${JSON.stringify(segment)} in ${where}: the variant depends on a discriminator value`;
    case "unknown_ref":
      return `schemapb: lookup: ref ${JSON.stringify(segment)} in ${where} points to a def that does not exist`;
    default:
      return `schemapb: lookup: cannot descend into ${JSON.stringify(segment)} in ${where} (kind ${kind})`;
  }
}

/**
 * Resolves a field path within the schema, one segment per field name.
 * Returns the addressed field or throws a LookupError naming the exact
 * segment that failed.
 */
export function lookup(s: Schema, ...segments: string[]): Schema_Field {
  if (segments.length === 0) {
    throw new LookupError("empty_path");
  }

  let cur: Schema | undefined = s;
  let parent = "";

  for (const [i, seg] of segments.entries()) {
    const f: Schema_Field | undefined = cur?.fields.find((c) => c.name === seg);
    if (f === undefined) {
      throw new LookupError("not_found", parent, seg);
    }
    if (i === segments.length - 1) {
      return f;
    }

    switch (f.kind.case) {
      case "object":
        cur = f.kind.value.schema;
        break;
      case "ref": {
        const def: Schema | undefined = s.defs[refDefKey(f.kind.value)];
        if (def === undefined) {
          throw new LookupError("unknown_ref", parent, seg, "ref");
        }
        cur = def;
        break;
      }
      case "oneOf":
        throw new LookupError("ambiguous_oneof", parent, seg, "oneof");
      default:
        throw new LookupError("not_traversable", parent, seg, kindName(f));
    }

    parent = joinPath(parent, seg);
  }

  throw new Error("unreachable");
}

/**
 * Lookup over a dot-separated path ("a.b.c"). Field names are identifiers
 * (enforced by descriptor validation), so the dot is never part of a name.
 */
export function lookupPath(s: Schema, path: string): Schema_Field {
  if (path === "") {
    throw new LookupError("empty_path");
  }
  return lookup(s, ...path.split("."));
}
