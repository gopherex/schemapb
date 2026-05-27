import { readFile } from "node:fs/promises";
import { beforeAll, describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";
import "./wasm_exec.js"; // sets globalThis.Go
import {
  Schemapb,
  SchemaSchema,
  Schema_Filed_ResultType as RT,
  Schema_Filed_Severity as Sev,
  type Schema,
} from "./index.ts";

let sp: Schemapb;

beforeAll(async () => {
  const bytes = await readFile(new URL("./schemapb.wasm", import.meta.url));
  sp = await Schemapb.load(bytes);
});

// Disk: derived iops/bandwidth/score (score depends on the other two) + the
// conditional size rules.
function diskSchema(): Schema {
  return create(SchemaSchema, {
    id: { namespace: "infra", name: "disk", version: "v1" },
    fields: [
      {
        name: "disk_type",
        required: true,
        kind: { case: "enum", value: { values: { 1: "ssd", 2: "hdd" }, definedOnly: true } },
      },
      {
        name: "disk_size",
        required: true,
        kind: { case: "int32", value: { gte: 1 } },
        rules: [
          { expr: "int(root.disk_type) != 1 || (int(this) >= 20 && int(this) <= 8192)", message: "t1", id: "disk1" },
          {
            expr: "int(root.disk_type) != 2 || (int(this) >= 93 && int(this) <= 262074 && int(this) % 93 == 0)",
            message: "t2",
            id: "disk2",
          },
        ],
      },
      {
        name: "iops",
        kind: {
          case: "computed",
          value: {
            expr:
              "root.disk_type == 1 ? min(max(root.disk_size * 50, 3000), 16000) : min(max(root.disk_size * 5, 100), 3000)",
            result: RT.INT64,
          },
        },
      },
      {
        name: "bandwidth_mbps",
        kind: {
          case: "computed",
          value: {
            expr:
              "root.disk_type == 1 ? min(max(root.disk_size / 4, 125), 1000) : min(max(root.disk_size / 10, 40), 500)",
            result: RT.INT64,
          },
        },
      },
      {
        name: "score",
        kind: { case: "computed", value: { expr: "root.iops + root.bandwidth_mbps * 10", result: RT.INT64 } },
      },
    ],
  });
}

// Validation surface across kinds.
function richSchema(): Schema {
  return create(SchemaSchema, {
    id: { namespace: "t", name: "rich", version: "v1" },
    fields: [
      { name: "name", required: true, kind: { case: "string", value: { minLen: 2n, maxLen: 8n, pattern: "^[a-z]+$" } } },
      { name: "role", kind: { case: "enum", value: { values: { 1: "user", 2: "admin" }, definedOnly: true, in: [1, 2] } } },
      {
        name: "tags",
        kind: {
          case: "list",
          value: { minItems: 1n, maxItems: 3n, unique: true, items: [{ name: "tag", kind: { case: "string", value: { minLen: 1n } } }] },
        },
      },
      {
        name: "addr",
        kind: {
          case: "object",
          value: { schema: { fields: [{ name: "zip", required: true, kind: { case: "string", value: { len: 5n } } }] } },
        },
      },
      { name: "opt", nullable: true, kind: { case: "string", value: {} } },
    ],
  });
}

describe("compute (derived values, dependency order)", () => {
  it.each([
    [1, 100, 5000, 125, 6250],
    [1, 1000, 16000, 250, 18500],
    [2, 1000, 3000, 100, 4000],
    [2, 10, 100, 40, 500],
  ])("type %i size %i -> iops/bw/score", (typ, size, iops, bw, score) => {
    const r = sp.compute(diskSchema(), { disk_type: typ, disk_size: size });
    expect(r.errors).toHaveLength(0);
    expect(r.values.iops).toBe(iops);
    expect(r.values.bandwidth_mbps).toBe(bw);
    expect(r.values.score).toBe(score); // depends on iops + bandwidth
  });
});

describe("disk conditional bounds", () => {
  it.each([
    [1, 20, true],
    [1, 19, false],
    [1, 8193, false],
    [2, 93, true],
    [2, 262074, true],
    [2, 100, false],
    [2, 92, false],
  ])("type %i size %i ok=%o", (typ, size, ok) => {
    const r = sp.validate(diskSchema(), { disk_type: typ, disk_size: size });
    expect(r.ok).toBe(ok);
  });
});

describe("kind validation", () => {
  it("accepts a fully valid object", () => {
    const r = sp.validate(richSchema(), {
      name: "bob",
      role: 1,
      tags: ["a", "b"],
      addr: { zip: "12345" },
      opt: null, // nullable
    });
    expect(r.ok).toBe(true);
    expect(r.errors).toHaveLength(0);
  });

  it("flags string/enum/list/object failures with paths", () => {
    const r = sp.validate(richSchema(), {
      name: "Bo!", // pattern fail
      role: 9, // undefined + not in
      tags: ["x", "x", "x", "x"], // max + unique
      addr: {}, // zip required
    });
    expect(r.ok).toBe(false);
    const fields = new Set(r.errors.map((e) => e.field));
    expect(fields.has("name")).toBe(true);
    expect(fields.has("role")).toBe(true);
    expect(fields.has("tags")).toBe(true);
    expect(fields.has("addr.zip")).toBe(true);
  });

  it("requires required fields", () => {
    const r = sp.validate(richSchema(), { role: 1 });
    expect(r.errors.find((e) => e.field === "name")?.message).toBe("required");
  });
});

describe("rules", () => {
  it("evaluates a form-wide rule over root", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "form", version: "v1" },
      fields: [
        { name: "a", kind: { case: "int32", value: {} } },
        { name: "b", kind: { case: "int32", value: {} } },
      ],
      rules: [{ expr: "root.a < root.b", message: "a<b", id: "ab" }],
    });
    expect(sp.validate(s, { a: 1, b: 2 }).ok).toBe(true);
    const bad = sp.validate(s, { a: 5, b: 1 });
    expect(bad.ok).toBe(false);
    expect(bad.errors.some((e) => e.ruleId === "ab")).toBe(true);
  });

  it("carries WARNING severity", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "warn", version: "v1" },
      fields: [
        {
          name: "w",
          kind: { case: "int32", value: {} },
          rules: [{ expr: "this <= 100", message: "soft", id: "cap", severity: Sev.WARNING }],
        },
      ],
    });
    const r = sp.validate(s, { w: 200 });
    const cap = r.errors.find((e) => e.ruleId === "cap");
    expect(cap?.severity).toBe("WARNING");
  });
});

describe("schema errors", () => {
  it("throws on a schema without identity", () => {
    const bad = create(SchemaSchema, { fields: [{ name: "x", kind: { case: "bool", value: {} } }] });
    expect(() => sp.validate(bad, {})).toThrow();
  });

  it("throws on a computed cycle", () => {
    const bad = create(SchemaSchema, {
      id: { namespace: "t", name: "cyc", version: "v1" },
      fields: [
        { name: "a", kind: { case: "computed", value: { expr: "root.b + 1" } } },
        { name: "b", kind: { case: "computed", value: { expr: "root.a + 1" } } },
      ],
    });
    expect(() => sp.compute(bad, {})).toThrow();
  });
});
