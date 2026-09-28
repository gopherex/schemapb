import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fromBinary, fromJson, toBinary, toJson } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import {
  BakedSchema,
  Engine,
  masked,
  ResolveReportSchema,
  SchemaSchema,
  StructValueSchema,
  structToNative,
} from "../src/index.js";

it("preserves opaque annotations through JSON, binary, Bake and masking", () => {
  const doc = JSON.parse(
    readFileSync(join(__dirname, "../../conformance/golden/annotations.json"), "utf8"),
  );
  const schema = fromJson(SchemaSchema, doc.schema);
  expect(fromBinary(SchemaSchema, toBinary(SchemaSchema, schema))).toStrictEqual(schema);
  expect(fromJson(SchemaSchema, toJson(SchemaSchema, schema))).toStrictEqual(schema);
  const engine = Engine.compile(schema);
  const outcome = engine.bakeDetailed(structToNative(fromJson(StructValueSchema, doc.input)));
  expect(outcome.result.errors).toEqual([]);
  expect(outcome.baked).toStrictEqual(fromJson(BakedSchema, doc.baked));
  expect(outcome.report).toStrictEqual(fromJson(ResolveReportSchema, doc.report));
  if (!outcome.baked) throw new Error("missing baked snapshot");
  expect(masked(outcome.baked)).toStrictEqual(fromJson(StructValueSchema, doc.masked));
  expect(schema).toStrictEqual(fromJson(SchemaSchema, doc.schema));
});
