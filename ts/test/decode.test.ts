import { readFileSync } from "node:fs";
import { join } from "node:path";
import { equals, fromBinary, fromJson, toBinary, toJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { ValidationResultSchema } from "../src/gen/schemapb/errors_pb.js";
import { BakedSchema, FilledSchema } from "../src/gen/schemapb/runtime_pb.js";
import { SchemaSchema } from "../src/gen/schemapb/schema_pb.js";
import { ListValueSchema, StructValueSchema } from "../src/gen/schemapb/value_pb.js";

const goldenDir = join(__dirname, "..", "..", "conformance", "golden");

function golden(name: string): string {
  return readFileSync(join(goldenDir, name), "utf8");
}

describe("golden decoding", () => {
  it("decodes and round-trips full-schema.json", () => {
    const schema = fromJson(SchemaSchema, JSON.parse(golden("full-schema.json")));
    expect(schema.id?.name).toBe("kitchen_sink");
    const back = fromBinary(SchemaSchema, toBinary(SchemaSchema, schema));
    expect(equals(SchemaSchema, schema, back)).toBe(true);
  });

  it("decodes full-baked.json with honest int64", () => {
    const baked = fromJson(StructValueSchema, JSON.parse(golden("full-baked.json")));
    const i64 = baked.fields["i64"];
    expect(i64?.kind.case).toBe("int64Value");
    expect(i64?.kind.value).toBe(256n);
  });

  it("decodes full-errors.json", () => {
    const res = fromJson(ValidationResultSchema, JSON.parse(golden("full-errors.json")));
    expect(res.errors.length).toBeGreaterThan(20);
  });

  it("decodes every message of full-coverage.json and round-trips", () => {
    const doc = JSON.parse(golden("full-coverage.json"));
    const schemas = {
      schema: SchemaSchema,
      allValues: ListValueSchema,
      filledInline: FilledSchema,
      filledById: FilledSchema,
      baked: BakedSchema,
      validationResult: ValidationResultSchema,
    } as const;
    for (const [key, desc] of Object.entries(schemas)) {
      const msg = fromJson(desc, doc[key]);
      const back = fromBinary(desc, toBinary(desc, msg));
      expect(equals(desc, msg, back), key).toBe(true);
      // protoJSON round-trip must not lose fields either.
      const again = fromJson(desc, toJson(desc, msg));
      expect(equals(desc, msg, again), `${key} json`).toBe(true);
    }
  });
});
