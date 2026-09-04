/**
 * Reflect conformance: the zod mirror model must produce the exact Schema
 * the Go reference reflected into conformance/golden/reflect.json (field
 * order included).
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fromJson, toJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { z } from "zod";
import { SchemaSchema } from "../src/gen/schemapb/schema_pb.js";
import { int64, str } from "../src/new.js";
import { id, Version } from "../src/typed.js";
import {
  bytesField,
  durationField,
  f32,
  i32,
  reflectZod,
  timestampField,
  u32,
  u64,
} from "../src/zod.js";

const goldenDir = join(__dirname, "..", "..", "conformance", "golden");

const MirrorNested = z.object({ on: z.boolean() });

interface MirrorNodeT {
  next?: MirrorNodeT | null | undefined;
}
const MirrorNode: z.ZodType<MirrorNodeT> = z.object({
  next: z.lazy(() => MirrorNode).nullish(),
});

const MirrorBase = z.object({ base: z.string() });

const MirrorModel = MirrorBase.extend({
  name: z.string().min(1).max(64).describe("display name"),
  mail: z.email(),
  slug: z.string().regex(/^[a-z-]+$/),
  mode: z.enum(["fast", "safe"]),
  level: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  count: i32(z.number().int().gte(0).lte(100)),
  big: z.bigint().gt(-10n).lt(10n),
  port: u32(z.number().int().lte(65535)),
  total: u64(z.bigint()),
  ratio: f32(z.number().gte(0)),
  score: z.number().lt(1),
  flag: z.boolean(),
  opt: z.string().nullish(),
  must: z.bigint().nullable(),
  tags: z.array(z.string()).min(1).max(5),
  pair: z.tuple([z.string(), z.string()]),
  blob: bytesField({ max: 16 }),
  magic: bytesField({ len: 4 }),
  limits: z.record(z.string(), z.bigint()),
  extra: z.record(z.string(), MirrorNested),
  nested: MirrorNested,
  when: timestampField(),
  wait: durationField(),
  raw: z.any(),
  anything: z.any(),
  chain: MirrorNode,
});

describe("reflectZod", () => {
  it("mirror model matches the golden", () => {
    const got = reflectZod(MirrorModel, id("conformance", "mirror", Version.of(1, 0, 0)));
    const want = fromJson(
      SchemaSchema,
      JSON.parse(readFileSync(join(goldenDir, "reflect.json"), "utf8")),
    );
    expect(toJson(SchemaSchema, got)).toEqual(toJson(SchemaSchema, want));
  });

  it("fails loudly", () => {
    const badKey = z.object({ x: z.record(z.number(), z.string()) });
    expect(() => reflectZod(badKey, id("t", "r", Version.of(1, 0, 0)))).toThrow(/record keys/);

    const dflt = z.object({ x: z.string().default("v") });
    expect(() => reflectZod(dflt, id("t", "r", Version.of(1, 0, 0)))).toThrow(/z\.default/);
  });

  it("overrides beat every default branch", () => {
    const SecretName = z.string();
    const Wait = durationField();
    const params = z.object({ token: SecretName, wait: Wait });

    const schema = reflectZod(params, id("t", "r", Version.of(1, 0, 0)), {
      overrides: new Map<unknown, (n: string) => never>([
        [SecretName, ((n: string) => str(n).secret().done()) as never],
        [Wait, ((n: string) => int64(n).gte(0n).done()) as never],
      ]),
    });

    const token = schema.fields.find((f) => f.name === "token");
    expect(token?.secret).toBe(true);
    const wait = schema.fields.find((f) => f.name === "wait");
    expect(wait?.kind.case).toBe("int64");
  });
});
