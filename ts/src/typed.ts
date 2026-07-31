/**
 * Distinct string domains of the public API get distinct branded types.
 * Untyped string literals still convert at call sites via the constructor
 * helpers, but a value of one domain cannot silently cross into another.
 */

import { create } from "@bufbuild/protobuf";
import semver from "semver";
import type { SchemaIdentity } from "./gen/schemapb/schema_pb.js";
import { SchemaIdentitySchema } from "./gen/schemapb/schema_pb.js";

declare const brand: unique symbol;

type Branded<B extends string> = string & { readonly [brand]: B };

export type Namespace = Branded<"Namespace">;
export type SchemaName = Branded<"SchemaName">;
export type FieldName = Branded<"FieldName">;
export type DefName = Branded<"DefName">;
export type TemplateName = Branded<"TemplateName">;
export type RuleId = Branded<"RuleId">;
export type GroupName = Branded<"GroupName">;
export type VariantKey = Branded<"VariantKey">;
export type Format = Branded<"Format">;

/** Core string formats of the spec's format registry. */
export const FORMATS = {
  email: "email" as Format,
  url: "url" as Format,
  uuid: "uuid" as Format,
  ipv4: "ipv4" as Format,
  ipv6: "ipv6" as Format,
  ip: "ip" as Format,
  hostname: "hostname" as Format,
  date: "date" as Format,
  time: "time" as Format,
  datetime: "datetime" as Format,
} as const;

/**
 * Version is an opaque, always-valid semver value: either zero
 * ("unversioned", serializes to "") or a canonical semver produced by
 * parseVersion / mustVersion / ver. Construction outside this module is
 * impossible — the class constructor is private.
 */
export class Version {
  static readonly zero = new Version("");

  readonly #s: string;

  private constructor(s: string) {
    this.#s = s;
  }

  /** Builds a release version ("v1.2.3"). */
  static of(major: number, minor: number, patch: number): Version {
    return new Version(`v${major}.${minor}.${patch}`);
  }

  /**
   * Parses a semver string ("v1.2.3", "1.2.3-rc.1"; shorthands
   * canonicalize). An empty string is the zero Version.
   */
  static parse(s: string): Version {
    if (s === "") {
      return Version.zero;
    }
    const normalized = s.startsWith("v") ? s.slice(1) : s;
    const parsed = semver.parse(
      semver.coerce(normalized, { includePrerelease: true }) ?? normalized,
    );
    if (parsed === null) {
      throw new Error(`schemapb: invalid version ${JSON.stringify(s)}`);
    }
    return new Version(`v${parsed.version}`);
  }

  get isZero(): boolean {
    return this.#s === "";
  }

  /** Canonical wire form ("" when unversioned). */
  toString(): string {
    return this.#s;
  }

  /** Semver precedence (-1, 0, 1); the zero Version sorts first. */
  compare(o: Version): number {
    if (this.#s === o.#s) {
      return 0;
    }
    if (this.#s === "") {
      return -1;
    }
    if (o.#s === "") {
      return 1;
    }
    return semver.compare(this.#s.slice(1), o.#s.slice(1));
  }
}

/**
 * Builds a schema identity. Declare it ONCE next to the schema and reuse the
 * same value everywhere the identity is needed (newSchema, refId,
 * registry.get) — a typo then cannot happen twice.
 */
export function id(ns: string, name: string, ver: Version): SchemaIdentity {
  return create(SchemaIdentitySchema, {
    namespace: ns,
    name,
    version: ver.toString(),
  });
}
