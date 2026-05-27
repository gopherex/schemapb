import { readFile } from "node:fs/promises";
import { beforeAll, describe, expect, it } from "vitest";
import { create, fromJson, toJson } from "@bufbuild/protobuf";
import "./wasm_exec.js"; // sets globalThis.Go
import {
  Schemapb,
  SchemaSchema,
  BakedSchema,
  Schema_Filed_ResultType as RT,
  Schema_Filed_Severity as Sev,
  Schema_Filed_String_StringFormat as SF,
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

describe("bake / merge", () => {
  function sizing(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "infra", name: "sizing", version: "v1" },
      fields: [
        { name: "size", kind: { case: "int32", value: { gte: 1, default: 20 } } },
        { name: "iops", kind: { case: "computed", value: { expr: "root.size * 50", result: RT.INT64 } } },
      ],
    });
  }

  it("bakes defaults + computed into a sealed Baked", () => {
    const r = sp.bake(sizing(), {});
    expect(r.errors).toHaveLength(0);
    expect(r.baked).toBeDefined();
    const v = r.baked!.values as Record<string, number>;
    expect(v.size).toBe(20);
    expect(v.iops).toBe(1000); // 20 * 50
  });

  it("merges overrides and re-bakes (computed recomputed)", () => {
    const base = fromJson(BakedSchema, sp.bake(sizing(), {}).baked!);
    const r = sp.merge(base, { size: 100 });
    expect(r.errors).toHaveLength(0);
    const v = r.baked!.values as Record<string, number>;
    expect(v.size).toBe(100);
    expect(v.iops).toBe(5000); // 100 * 50
  });

  it("reports errors instead of sealing when a merge violates a constraint", () => {
    const base = fromJson(BakedSchema, sp.bake(sizing(), {}).baked!);
    const r = sp.merge(base, { size: 0 }); // gte 1
    expect(r.baked).toBeUndefined();
    expect(r.errors.some((e) => e.field === "size")).toBe(true);
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

// ---------------------------------------------------------------------------
// String format validation
// ---------------------------------------------------------------------------

describe("string format", () => {
  function fmtSchema(fmt: SF): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "fmt", version: "v1" },
      fields: [{ name: "val", kind: { case: "string", value: { format: fmt } } }],
    });
  }

  it.each([
    [SF.EMAIL, "user@example.com", true],
    [SF.EMAIL, "not-an-email", false],
    [SF.UUID, "550e8400-e29b-41d4-a716-446655440000", true],
    [SF.UUID, "not-a-uuid", false],
    [SF.URL, "https://example.com/path", true],
    [SF.URL, "not a url", false],
    [SF.IPV4, "192.168.1.1", true],
    [SF.IPV4, "::1", false],
    [SF.IPV6, "2001:db8::1", true],
    [SF.IPV6, "1.2.3.4", false],
    [SF.DATE, "2024-03-15", true],
    [SF.DATE, "not-a-date", false],
  ])("format %i: value=%s valid=%o", (fmt, value, valid) => {
    const r = sp.validate(fmtSchema(fmt), { val: value });
    expect(r.ok).toBe(valid);
  });
});

// ---------------------------------------------------------------------------
// strict mode
// ---------------------------------------------------------------------------

describe("strict mode", () => {
  function strictSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "strict", version: "v1" },
      strict: true,
      fields: [{ name: "name", kind: { case: "string", value: {} } }],
    });
  }

  it("accepts a known key", () => {
    expect(sp.validate(strictSchema(), { name: "alice" }).ok).toBe(true);
  });

  it("rejects an unknown key", () => {
    const r = sp.validate(strictSchema(), { name: "alice", extra: "field" });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "extra")).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// minProperties / maxProperties
// ---------------------------------------------------------------------------

describe("minProperties / maxProperties", () => {
  function propSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "props", version: "v1" },
      minProperties: 2n,
      maxProperties: 3n,
      fields: [
        { name: "a", kind: { case: "string", value: {} } },
        { name: "b", kind: { case: "string", value: {} } },
        { name: "c", kind: { case: "string", value: {} } },
      ],
    });
  }

  it("accepts exactly minProperties", () => {
    expect(sp.validate(propSchema(), { a: "x", b: "y" }).ok).toBe(true);
  });

  it("rejects fewer than minProperties", () => {
    const r = sp.validate(propSchema(), { a: "x" });
    expect(r.ok).toBe(false);
  });

  it("accepts exactly maxProperties", () => {
    expect(sp.validate(propSchema(), { a: "x", b: "y", c: "z" }).ok).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// coerce
// ---------------------------------------------------------------------------

describe("coerce", () => {
  function coerceSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "coerce", version: "v1" },
      coerce: true,
      fields: [
        { name: "n", kind: { case: "int32", value: { gte: 0, lte: 100 } } },
        { name: "flag", kind: { case: "bool", value: {} } },
        { name: "kind", kind: { case: "enum", value: { values: { 1: "a", 2: "b" }, definedOnly: true } } },
      ],
    });
  }

  it("coerces string '5' to int and passes constraints", () => {
    expect(sp.validate(coerceSchema(), { n: "5", flag: "true", kind: "1" }).ok).toBe(true);
  });

  it("reports error when string is unparseable", () => {
    const r = sp.validate(coerceSchema(), { n: "abc" });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "n")).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// normalize
// ---------------------------------------------------------------------------

describe("normalize", () => {
  function normalizeSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "norm", version: "v1" },
      fields: [
        {
          name: "tag",
          normalize: "lower(this)",
          kind: { case: "string", value: { pattern: "^[a-z]+$" } },
        },
      ],
    });
  }

  it("lowercases the value before the pattern check", () => {
    // "Alice" -> lower -> "alice" -> matches ^[a-z]+$ -> valid
    expect(sp.validate(normalizeSchema(), { tag: "Alice" }).ok).toBe(true);
  });

  it("already lowercase passes too", () => {
    expect(sp.validate(normalizeSchema(), { tag: "bob" }).ok).toBe(true);
  });

  it("absent field is not forced present", () => {
    expect(sp.validate(normalizeSchema(), {}).ok).toBe(true);
  });

  it("normalized value is reflected in computed output", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "norm2", version: "v1" },
      fields: [
        { name: "tag", normalize: "lower(this)", kind: { case: "string", value: {} } },
        { name: "tag_up", kind: { case: "computed", value: { expr: "upper(root.tag)", result: RT.STRING } } },
      ],
    });
    const r = sp.compute(s, { tag: "Hello" });
    expect(r.errors).toHaveLength(0);
    expect(r.values.tag).toBe("hello");
    expect(r.values.tag_up).toBe("HELLO");
  });
});

// ---------------------------------------------------------------------------
// OneOf (discriminated union)
// ---------------------------------------------------------------------------

describe("OneOf discriminated union", () => {
  function oneOfSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "t", name: "oneof", version: "v1" },
      fields: [
        {
          name: "target",
          required: true,
          kind: {
            case: "oneOf",
            value: {
              discriminator: "kind",
              variants: {
                disk: create(SchemaSchema, {
                  fields: [{ name: "size", required: true, kind: { case: "int32", value: { gte: 1 } } }],
                }),
                net: create(SchemaSchema, {
                  fields: [{ name: "cidr", required: true, kind: { case: "string", value: {} } }],
                }),
              },
            },
          },
        },
      ],
    });
  }

  it("accepts a valid disk variant", () => {
    expect(sp.validate(oneOfSchema(), { target: { kind: "disk", size: 100 } }).ok).toBe(true);
  });

  it("accepts a valid net variant", () => {
    expect(sp.validate(oneOfSchema(), { target: { kind: "net", cidr: "10.0.0.0/8" } }).ok).toBe(true);
  });

  it("rejects missing discriminator", () => {
    const r = sp.validate(oneOfSchema(), { target: { size: 10 } });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "target")).toBe(true);
  });

  it("rejects unknown variant", () => {
    const r = sp.validate(oneOfSchema(), { target: { kind: "usb", size: 10 } });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "target")).toBe(true);
  });

  it("reports missing required field inside chosen variant", () => {
    // disk chosen but size absent
    const r = sp.validate(oneOfSchema(), { target: { kind: "disk" } });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "target.size")).toBe(true);
  });

  it("reports required field for wrong variant (net without cidr)", () => {
    const r = sp.validate(oneOfSchema(), { target: { kind: "net" } });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "target.cidr")).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Ref + defs (recursive tree schema)
// ---------------------------------------------------------------------------

describe("Ref + defs", () => {
  // Recursive tree: each node has a label (required) and an optional children
  // list, each element validated against the "node" def again.
  function treeSchema(): Schema {
    const nodeSchema = create(SchemaSchema, {
      fields: [
        { name: "label", required: true, kind: { case: "string", value: {} } },
        {
          name: "children",
          kind: {
            case: "list",
            value: {
              items: [{ name: "child", kind: { case: "ref", value: { name: "node" } } }],
            },
          },
        },
      ],
    });
    return create(SchemaSchema, {
      id: { namespace: "t", name: "tree", version: "v1" },
      defs: { node: nodeSchema },
      fields: [{ name: "root", required: true, kind: { case: "ref", value: { name: "node" } } }],
    });
  }

  it("validates a two-level tree", () => {
    const data = {
      root: {
        label: "root",
        children: [{ label: "child", children: [{ label: "leaf" }] }],
      },
    };
    expect(sp.validate(treeSchema(), data).ok).toBe(true);
  });

  it("reports a deep missing label with a path containing 'label'", () => {
    const data = {
      root: {
        label: "root",
        children: [{ label: "child", children: [{}] }], // grandchild missing label
      },
    };
    const r = sp.validate(treeSchema(), data);
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => (e.field ?? "").includes("label"))).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Field metadata round-trip through toJson
// ---------------------------------------------------------------------------

describe("field metadata round-trip", () => {
  it("preserves title, secret, deprecated in toJson output", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "meta", version: "v1" },
      fields: [
        {
          name: "email",
          title: "Email address",
          deprecated: true,
          secret: true,
          kind: { case: "string", value: { format: SF.EMAIL } },
        },
      ],
    });
    const j = toJson(SchemaSchema, s) as Record<string, unknown>;
    const fields = j.fields as Array<Record<string, unknown>>;
    expect(fields[0].title).toBe("Email address");
    expect(fields[0].deprecated).toBe(true);
    expect(fields[0].secret).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// FieldError.code
// ---------------------------------------------------------------------------

describe("FieldError.code", () => {
  const codeSchema = create(SchemaSchema, {
    id: { namespace: "t", name: "codes", version: "v1" },
    fields: [
      { name: "name", required: true, kind: { case: "string", value: {} } },
      { name: "age", kind: { case: "int32", value: { gte: 0 } } },
      { name: "email", kind: { case: "string", value: { format: SF.EMAIL } } },
    ],
  });

  it("carries code='required' for a missing required field", () => {
    const r = sp.validate(codeSchema, {});
    const e = r.errors.find((x) => x.field === "name");
    expect(e?.code).toBe("required");
  });

  it("carries code='type' for a wrong-type value", () => {
    const r = sp.validate(codeSchema, { name: "x", age: "notanumber" });
    const e = r.errors.find((x) => x.field === "age");
    expect(e?.code).toBe("type");
  });

  it("carries code='format' for a bad format value", () => {
    const r = sp.validate(codeSchema, { name: "x", email: "notanemail" });
    const e = r.errors.find((x) => x.field === "email");
    expect(e?.code).toBe("format");
  });
});
