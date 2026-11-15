import { runInNewContext } from "node:vm";
import { equals } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { bake } from "../src/bake.js";
import { isBytes } from "../src/bytes.js";
import { StructValueSchema } from "../src/gen/schemapb/value_pb.js";
import * as b from "../src/new.js";
import { displayString, nativeEquals } from "../src/render.js";
import { id, Version } from "../src/typed.js";
import { validate } from "../src/validate.js";
import { structFromNative } from "../src/value.js";

/** A Uint8Array from another realm (jsdom, an iframe, a worker, node:vm). */
function foreignBytes(...values: number[]): Uint8Array {
  return runInNewContext(`new Uint8Array([${values.join(",")}])`);
}

describe("bytes from another realm", () => {
  it("is foreign: instanceof alone would reject it", () => {
    expect(foreignBytes(1) instanceof Uint8Array).toBe(false);
  });

  it.concurrent.each([
    ["local Uint8Array", new Uint8Array([1]), true],
    ["foreign Uint8Array", foreignBytes(1), true],
    ["Int8Array", new Int8Array([1]), false],
    ["DataView", new DataView(new ArrayBuffer(1)), false],
    ["ArrayBuffer", new ArrayBuffer(1), false],
    ["array", [1], false],
    ["look-alike", { [Symbol.toStringTag]: "Uint8Array" }, false],
    ["string", "AQ==", false],
  ] as const)("isBytes(%s)", async (_name, value, want) => {
    expect(isBytes(value)).toBe(want);
  });

  const { engine } = b
    .newSchema(id("t", "bytes", Version.of(1, 0, 0)))
    .fields(
      b
        .bytes("blob")
        .minLen(1n)
        .default(new Uint8Array([9])),
    )
    .build();

  it("validates, bakes, displays and compares like a local array", () => {
    const foreign = foreignBytes(1, 2);
    expect(validate(engine, { blob: foreign }).errors).toEqual([]);
    expect(validate(engine, { blob: foreignBytes() }).errors.map((e) => e.constraint)).toEqual([
      "min_len",
    ]);
    expect(bake(engine, { blob: foreign }).baked).toBeDefined();
    expect(displayString(foreign)).toBe("AQI=");
    expect(nativeEquals(foreign, new Uint8Array([1, 2]))).toBe(true);
    // toEqual tells realms apart; protobuf equality compares the bytes.
    expect(
      equals(
        StructValueSchema,
        structFromNative({ blob: foreign }),
        structFromNative({ blob: new Uint8Array([1, 2]) }),
      ),
    ).toBe(true);
  });
});
