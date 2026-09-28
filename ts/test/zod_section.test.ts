/**
 * Sections: a member that is a plain z.object (not optional, not nullable)
 * is optional with an implicit empty object default, so an absent section
 * still resolves its inner defaults. Mirrors the Go reference
 * (go/schemapb/reflect_section_test.go).
 */

import { describe, expect, it } from "vitest";
import { z } from "zod";
import { Engine } from "../src/engine.js";
import { ErrorCode, type ValidationResult } from "../src/gen/schemapb/errors_pb.js";
import type { Schema, Schema_Field } from "../src/gen/schemapb/schema_pb.js";
import { lookupPath } from "../src/lookup.js";
import { int64, json, str } from "../src/new.js";
import { fieldName, id, Version } from "../src/typed.js";
import { structToNative } from "../src/value.js";
import { reflectZod, required } from "../src/zod.js";

// zod defaults are not reflected; the inner defaults come from the native
// override mechanism, like Go's `schemapb:"default=..."` tags.
const Host = z.string();
const Port = z.bigint();
const Realm = z.string();
const overrides = new Map<unknown, (name: string) => Schema_Field>([
  [Host, (n) => str(fieldName(n)).default("localhost").done()],
  [Port, (n) => int64(fieldName(n)).default(5432n).done()],
  [Realm, (n) => str(fieldName(n)).default("main").done()],
]);

const SectionDB = z.object({ host: Host, port: Port });
const SectionAuth = z.object({ token: z.string(), realm: Realm });

function reflect(model: unknown): Schema {
  return reflectZod(model, id("t", "section", Version.of(1, 0, 0)), { overrides });
}

function hasError(res: ValidationResult, path: string, code: ErrorCode): boolean {
  return res.errors.some((e) => e.path === path && e.code === code);
}

function objectDefault(f: Schema_Field) {
  return f.kind.case === "object" ? f.kind.value.default : undefined;
}

describe("reflectZod sections", () => {
  it("absent section is valid with inner defaults applied", () => {
    const schema = reflect(z.object({ db: SectionDB }));
    const f = lookupPath(schema, "db");
    expect(f.required).toBe(false);
    expect(objectDefault(f)).toBeDefined();

    const outcome = Engine.compile(schema).bake({});
    expect(outcome.result.errors).toEqual([]);
    expect(structToNative(outcome.baked?.values)).toEqual({
      db: { host: "localhost", port: 5432n },
    });
  });

  it("missing required inner field is reported inside the section", () => {
    const res = Engine.compile(reflect(z.object({ auth: SectionAuth }))).bake({}).result;
    expect(hasError(res, "auth.token", ErrorCode.REQUIRED_MISSING)).toBe(true);
    expect(hasError(res, "auth", ErrorCode.REQUIRED_MISSING)).toBe(false);
  });

  it("required() keeps the section required without a default", () => {
    const model = z.object({ db: required(SectionDB), other: SectionDB });
    const schema = reflect(model);
    const f = lookupPath(schema, "db");
    expect(f.required).toBe(true);
    expect(objectDefault(f)).toBeUndefined();

    // The shared schema itself stays unmarked, and zod still parses.
    expect(lookupPath(schema, "other").required).toBe(false);
    expect(model.parse({ db: { host: "h", port: 1n }, other: { host: "h", port: 1n } })).toEqual({
      db: { host: "h", port: 1n },
      other: { host: "h", port: 1n },
    });

    const res = Engine.compile(schema).bake({}).result;
    expect(hasError(res, "db", ErrorCode.REQUIRED_MISSING)).toBe(true);
  });

  it("required() forces an optional member required", () => {
    const f = lookupPath(reflect(z.object({ db: required(SectionDB.nullish()) })), "db");
    expect(f.required).toBe(true);
    expect(f.nullable).toBe(true);
    expect(objectDefault(f)).toBeUndefined();
  });

  it("optional and nullable sections are unchanged", () => {
    const schema = reflect(z.object({ opt: SectionDB.optional(), nul: SectionDB.nullable() }));
    const opt = lookupPath(schema, "opt");
    expect(opt.required).toBe(false);
    expect(objectDefault(opt)).toBeUndefined();
    const nul = lookupPath(schema, "nul");
    expect(nul.required).toBe(true);
    expect(nul.nullable).toBe(true);
    expect(objectDefault(nul)).toBeUndefined();

    const outcome = Engine.compile(schema).bake({ nul: null });
    expect(outcome.result.errors).toEqual([]);
    expect(structToNative(outcome.baked?.values)).toEqual({ nul: null });
  });

  it("overrides, list items and map values are not sections", () => {
    const schema = reflectZod(
      z.object({ db: SectionDB, dbs: z.array(SectionAuth) }),
      id("t", "section", Version.of(1, 0, 0)),
      { overrides: new Map([[SectionDB, (n: string) => json(fieldName(n)).done()]]) },
    );
    const db = lookupPath(schema, "db");
    expect(db.required).toBe(true);
    expect(db.kind.case).toBe("json");
    const dbs = lookupPath(schema, "dbs");
    const item = dbs.kind.case === "list" ? dbs.kind.value.items[0] : undefined;
    expect(item?.required).toBe(true);
    expect(item && objectDefault(item)).toBeUndefined();
  });
});
