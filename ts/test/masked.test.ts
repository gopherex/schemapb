import { readFileSync } from "node:fs";
import { join } from "node:path";
import { clone, fromJson, type JsonValue } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { BakedSchema, masked, StructValueSchema, type Value } from "../src/index.js";

const cases = JSON.parse(
  readFileSync(join(__dirname, "../../conformance/golden/masked.json"), "utf8"),
) as { name: string; baked: JsonValue; masked: JsonValue }[];
for (const c of cases) {
  it(`masks ${c.name} according to the shared contract`, () => {
    const baked = fromJson(BakedSchema, c.baked);
    const before = clone(BakedSchema, baked);
    const want = fromJson(StructValueSchema, c.masked);
    const out = masked(baked);
    expect(out).toStrictEqual(want);
    expect(baked).toStrictEqual(before);
    for (const v of Object.values(out.fields)) mutate(v);
    expect(baked).toStrictEqual(before);
    expect(masked(baked)).toStrictEqual(want);
  });
}
function mutate(value: Value): void {
  const k = value.kind;
  if (k.case === "structValue") for (const v of Object.values(k.value.fields)) mutate(v);
  if (k.case === "listValue") for (const v of k.value.items) mutate(v);
  if (k.case === "bytesValue" && k.value.length) k.value[0] = 255;
  if (k.case === "durationValue" || k.case === "timestampValue") k.value.seconds++;
  value.kind = { case: "stringValue", value: "changed" };
}
