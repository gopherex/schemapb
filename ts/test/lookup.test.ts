/**
 * Lookup conformance: every case in conformance/golden/lookup.json must
 * resolve to the same kind, or fail with the same (at, segment, reason)
 * triple, as the Go reference. Plus port-local cases the golden cannot
 * host (dangling ref, name rules).
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { create, fromJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { checkDescriptor } from "../src/descriptor.js";
import { SchemaSchema } from "../src/gen/schemapb/schema_pb.js";
import { LookupError, listItems, lookupPath } from "../src/lookup.js";
import { kindName } from "../src/render.js";

const goldenDir = join(__dirname, "..", "..", "conformance", "golden");

interface LookupCase {
  path: string;
  kind?: string;
  items?: string[];
  error?: { at: string; segment: string; reason: string };
}

describe("lookup conformance", () => {
  const schema = fromJson(
    SchemaSchema,
    JSON.parse(readFileSync(join(goldenDir, "full-schema.json"), "utf8")),
  );
  const cases: LookupCase[] = JSON.parse(
    readFileSync(join(goldenDir, "lookup.json"), "utf8"),
  ).cases;

  it.each(cases)("$path", (c) => {
    if (c.error !== undefined) {
      try {
        lookupPath(schema, c.path);
        expect.fail(`lookupPath(${c.path}) resolved, want ${c.error.reason}`);
      } catch (err) {
        expect(err).toBeInstanceOf(LookupError);
        const le = err as LookupError;
        expect({ at: le.at, segment: le.segment, reason: le.reason }).toEqual(c.error);
      }
      return;
    }
    const f = lookupPath(schema, c.path);
    expect(kindName(f)).toBe(c.kind);
    expect(listItems(f).map(kindName)).toEqual(c.items ?? []);
  });
});

describe("field name rules", () => {
  const schemaWithName = (name: string) =>
    create(SchemaSchema, {
      id: { namespace: "t", name: "names" },
      fields: [{ name, kind: { case: "string", value: {} } }],
    });

  it.each(["a.b", "my-field", "1st", "in", "true", "while"])("rejects %s", (bad) => {
    expect(checkDescriptor(schemaWithName(bad)).length).toBeGreaterThan(0);
  });

  it.each(["snake_case", "camelCase", "_x", "a1"])("accepts %s", (good) => {
    expect(checkDescriptor(schemaWithName(good))).toEqual([]);
  });
});
