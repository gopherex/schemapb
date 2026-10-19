import { describe, expect, it } from "vitest";
import { bake } from "../src/bake.js";
import { ErrorCode } from "../src/gen/schemapb/errors_pb.js";
import * as b from "../src/new.js";
import { id, Version } from "../src/typed.js";
import { validate } from "../src/validate.js";
import { structFromNative, structToNative } from "../src/value.js";

// "__proto__" is an ordinary key on the wire and in JSON.parse output; a
// plain `obj[key] = v` on a fresh object would set the prototype instead,
// dropping the entry from whatever the engine builds next.
describe('"__proto__" keys', () => {
  const values = () => JSON.parse('{"m": {"__proto__": "BAD", "ok": "fine"}}');

  const { engine } = b
    .newSchema(id("t", "t", Version.zero))
    .fields(b.mapOf("m", b.str("value")))
    .rules(b.rule('root.m.all(k, root.m[k] != "BAD")', "no BAD values").id("no-bad"))
    .build();

  it("CEL rules see the entry the field checks see", () => {
    const codes = validate(engine, values()).errors.map((e) => ErrorCode[e.code]);
    expect(codes).toContain("RULE_VIOLATED");
  });

  it("survives native <-> wire conversion and baking", () => {
    const wire = structFromNative(values());
    expect(Object.keys(wire.fields["m"]?.kind.value as object)).toBeDefined();
    const back = structToNative(wire) as Record<string, Record<string, unknown>>;
    expect(Object.keys(back["m"] ?? {})).toEqual(["__proto__", "ok"]);

    const { engine: plain } = b
      .newSchema(id("t", "t", Version.zero))
      .fields(b.mapOf("m", b.str("value")))
      .build();
    const out = bake(plain, values());
    const m = out.baked?.values?.fields["m"]?.kind;
    if (m?.case !== "structValue") {
      throw new Error(`baked m is ${m?.case}`);
    }
    expect(Object.keys(m.value.fields)).toEqual(["__proto__", "ok"]);
  });
});
