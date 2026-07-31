/**
 * The conformance runner: parse the golden kitchen-sink schema, run the two
 * canonical inputs through this implementation, and require results
 * IDENTICAL to the Go reference goldens (proto equality for messages, byte
 * equality for rendered text, map equality for message templates).
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fromJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { bake, renderBaked } from "../src/bake.js";
import { Engine } from "../src/engine.js";
import { ErrorCode, ValidationResultSchema } from "../src/gen/schemapb/errors_pb.js";
import { SchemaSchema } from "../src/gen/schemapb/schema_pb.js";
import { StructValueSchema } from "../src/gen/schemapb/value_pb.js";
import { messageTemplates } from "../src/messages.js";
import { validate } from "../src/validate.js";
import type { NativeStruct } from "../src/value.js";

const goldenDir = join(__dirname, "..", "..", "conformance", "golden");

function golden(name: string): string {
  return readFileSync(join(goldenDir, name), "utf8");
}

function goldenEngine(): Engine {
  const schema = fromJson(SchemaSchema, JSON.parse(golden("full-schema.json")));
  return Engine.compile(schema, {
    formats: { "x.nonempty": (v: string) => v !== "" },
  });
}

/** validInput mirrors go/schemapb/golden_test.go exactly. */
function validInput(): NativeStruct {
  return {
    i64: "256", // coerced
    mail: "dba@corp.io",
    token: "s3cret-token",
    magic: new Uint8Array([0xde, 0xad]),
    replica_count: 1n,
    replicas: [{ name: "r1" }],
    tablespaces: {
      main: { location: "/var/lib/ts" },
    },
    backup: { type: "s3", bucket: "backups" },
    data_volume: { path: "/data" },
    region: "somewhere-else",
    endpoint_pair: ["db1", 5432n],
  };
}

/** brokenInput mirrors go/schemapb/golden_test.go exactly. */
function brokenInput(): NativeStruct {
  return {
    f32: 0.25,
    f64: 2.0,
    i32: 5n,
    i64: 8n,
    u32: 3n,
    u64: 0n,
    pinned: false,
    name: "Bad Name!",
    mode: "legacy",
    exact: "abcde",
    mail: "not-an-email",
    token: "short",
    license: new TextEncoder().encode("XX"),
    magic: new Uint8Array([0x00]),
    wal_level: "extreme",
    cpu: 3n,
    timeout: "3h",
    not_before: "2020-01-01T00:00:00Z",
    replica_count: 2n,
    replicas: [{ name: "r1" }, { name: "r1" }, { weight: 2n }],
    logging: { collector: true, junk: 1n },
    tablespaces: { bad: {} },
    backup: { type: "tape" },
    data_volume: { path: "/data", size_gb: 0n },
    garbage: 1n,
    endpoint_pair: ["", "not-a-port"],
  };
}

describe("conformance against Go goldens", () => {
  it("bakes the valid input into full-baked.json", () => {
    const e = goldenEngine();
    const outcome = bake(e, validInput());
    expect(outcome.result.errors.filter((x) => x.code !== ErrorCode.RULE_VIOLATED)).toEqual([]);
    expect(outcome.baked).toBeDefined();
    const want = fromJson(StructValueSchema, JSON.parse(golden("full-baked.json")));
    expect(outcome.baked?.values).toStrictEqual(want);
  });

  it("produces full-errors.json for the broken input", () => {
    const e = goldenEngine();
    const got = validate(e, brokenInput());
    const want = fromJson(ValidationResultSchema, JSON.parse(golden("full-errors.json")));
    expect(got.errors.map(errKey)).toEqual(want.errors.map(errKey));
    expect(got).toStrictEqual(want);
  });

  it("renders full-rendered.txt", () => {
    const e = goldenEngine();
    const outcome = bake(e, validInput());
    expect(outcome.baked).toBeDefined();
    if (outcome.baked === undefined) {
      return;
    }
    const conf = renderBaked(e, outcome.baked, "conf" as never);
    const report = renderBaked(e, outcome.baked, "report" as never);
    expect(`${conf}---\n${report}`).toBe(golden("full-rendered.txt"));
  });

  it("message templates match messages.json", () => {
    const want = JSON.parse(golden("messages.json")) as Record<string, string>;
    const got: Record<string, string> = {};
    for (const [code, tpl] of messageTemplates) {
      got[`ERROR_CODE_${ErrorCode[code]}`] = tpl;
    }
    expect(got).toEqual(want);
  });
});

function errKey(e: { path: string; code: ErrorCode }): string {
  return `${e.path}:${ErrorCode[e.code]}`;
}
