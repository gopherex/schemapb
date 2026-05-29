import { beforeAll, describe, expect, it } from "vitest";
import { create, fromJson, toJson } from "@bufbuild/protobuf";
import {
  schemapb,
  type Schemapb,
  SchemaSchema,
  BakedSchema,
  Schema_Filed_ResultType as RT,
  Schema_Filed_Severity as Sev,
  Schema_Filed_String_StringFormat as SF,
  type Schema,
} from "./index.ts";

let sp: Schemapb;

// Zero-config: schemapb() auto-loads the wasm and runs wasm_exec.js itself.
beforeAll(async () => {
  sp = await schemapb();
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
              items: [{ name: "child", kind: { case: "ref", value: { target: { case: "name", value: "node" } } } }],
            },
          },
        },
      ],
    });
    return create(SchemaSchema, {
      id: { namespace: "t", name: "tree", version: "v1" },
      defs: { node: nodeSchema },
      fields: [{ name: "root", required: true, kind: { case: "ref", value: { target: { case: "name", value: "node" } } } }],
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

// ---------------------------------------------------------------------------
// Feature: when (conditional field gate)
// ---------------------------------------------------------------------------

describe("when (conditional gate)", () => {
  // tls_cert required + range-checked only when tls is on.
  const whenSchema = create(SchemaSchema, {
    id: { namespace: "t", name: "when", version: "v1" },
    fields: [
      { name: "tls", kind: { case: "bool", value: {} } },
      {
        name: "tls_cert",
        required: true,
        when: "root.tls == true",
        kind: { case: "string", value: { minLen: 3n } },
      },
    ],
  });

  it("skips required for an inactive field", () => {
    expect(sp.validate(whenSchema, { tls: false }).ok).toBe(true);
  });

  it("enforces required for an active field", () => {
    const r = sp.validate(whenSchema, { tls: true });
    expect(r.ok).toBe(false);
    expect(r.errors.find((e) => e.field === "tls_cert")?.code).toBe("required");
  });

  it("does not validate an inactive field's present value", () => {
    // too short, but inactive => ignored.
    expect(sp.validate(whenSchema, { tls: false, tls_cert: "x" }).ok).toBe(true);
  });

  it("validates an active field's value", () => {
    expect(sp.validate(whenSchema, { tls: true, tls_cert: "x" }).ok).toBe(false);
    expect(sp.validate(whenSchema, { tls: true, tls_cert: "pem" }).ok).toBe(true);
  });

  it("gates a whole container subtree", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "whenobj", version: "v1" },
      fields: [
        { name: "backup", kind: { case: "bool", value: {} } },
        {
          name: "backup_cfg",
          when: "root.backup == true",
          kind: {
            case: "object",
            value: { schema: { fields: [{ name: "bucket", required: true, kind: { case: "string", value: {} } }] } },
          },
        },
      ],
    });
    expect(sp.validate(s, { backup: false }).ok).toBe(true);
    const r = sp.validate(s, { backup: true, backup_cfg: {} });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "backup_cfg.bucket")).toBe(true);
  });

  it("skips compute/normalize for an inactive field", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "t", name: "whencompute", version: "v1" },
      fields: [
        { name: "on", kind: { case: "bool", value: {} } },
        { name: "tag", when: "root.on == true", kind: { case: "string", value: {}, }, normalize: "lower(this)" },
        { name: "derived", when: "root.on == true", kind: { case: "computed", value: { expr: "root.tag", result: RT.STRING } } },
      ],
    });
    const off = sp.compute(s, { on: false, tag: "HELLO" });
    expect(off.values.tag).toBe("HELLO"); // normalize skipped
    expect(off.values.derived).toBeUndefined(); // computed not seeded
    const on = sp.compute(s, { on: true, tag: "HELLO" });
    expect(on.values.tag).toBe("hello");
    expect(on.values.derived).toBe("hello");
  });
});

// ---------------------------------------------------------------------------
// Feature: options_expr (dynamic enum options)
// ---------------------------------------------------------------------------

describe("options_expr (dynamic enum options)", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "opts", version: "v1" },
    fields: [
      { name: "edition", kind: { case: "string", value: {} } },
      {
        name: "version",
        kind: {
          case: "enum",
          value: {
            values: { 13: "13", 14: "14", 15: "15", 16: "16" },
            optionsExpr: 'root.edition == "lts" ? [14, 16] : [15]',
          },
        },
      },
    ],
  });

  it("accepts a value in the dynamic set", () => {
    expect(sp.validate(s, { edition: "lts", version: 16 }).ok).toBe(true);
    expect(sp.validate(s, { edition: "std", version: 15 }).ok).toBe(true);
  });

  it("rejects a value outside the dynamic set with code enum_not_allowed", () => {
    const r = sp.validate(s, { edition: "lts", version: 15 });
    expect(r.ok).toBe(false);
    expect(r.errors.find((e) => e.field === "version")?.code).toBe("enum_not_allowed");
  });
});

// ---------------------------------------------------------------------------
// Feature: count_expr (dynamic list length)
// ---------------------------------------------------------------------------

describe("count_expr (dynamic list length)", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "cnt", version: "v1" },
    fields: [
      { name: "replicas", kind: { case: "int32", value: { gte: 0 } } },
      {
        name: "machines",
        kind: {
          case: "list",
          value: { countExpr: "root.replicas + 1", items: [{ name: "host", kind: { case: "string", value: {} } }] },
        },
      },
    ],
  });

  it("accepts a list of the required length", () => {
    expect(sp.validate(s, { replicas: 2, machines: ["a", "b", "c"] }).ok).toBe(true);
  });

  it("rejects a wrong length with code list_count_mismatch", () => {
    const r = sp.validate(s, { replicas: 2, machines: ["a", "b"] });
    expect(r.ok).toBe(false);
    expect(r.errors.find((e) => e.field === "machines")?.code).toBe("list_count_mismatch");
  });

  it("binds the item index inside item rules", () => {
    const idx = create(SchemaSchema, {
      id: { namespace: "t", name: "cntidx", version: "v1" },
      fields: [
        { name: "n", kind: { case: "int32", value: { gte: 0 } } },
        {
          name: "seq",
          kind: {
            case: "list",
            value: {
              countExpr: "root.n",
              items: [{ name: "v", kind: { case: "int32", value: {} }, rules: [{ expr: "this == index", message: "must equal its index", id: "rng" }] }],
            },
          },
        },
      ],
    });
    expect(sp.validate(idx, { n: 3, seq: [0, 1, 2] }).ok).toBe(true);
    const r = sp.validate(idx, { n: 3, seq: [0, 9, 2] });
    expect(r.ok).toBe(false);
    expect(r.errors.some((e) => e.field === "seq[1]")).toBe(true);
  });
});

// Identity-ref (Ref by SchemaIdentity). Linking is server-side (Go Schema.Link);
// the browser receives an already self-contained schema. An UNLINKED id-ref
// surfaces a "ref" error — proving the new Ref oneof shape flows through WASM.
describe("identity ref (RefID)", () => {
  it("reports a 'ref' error for an unlinked identity ref", () => {
    const s = create(SchemaSchema, {
      id: { namespace: "app", name: "cfg", version: "v1" },
      fields: [
        {
          name: "primary",
          required: true,
          kind: {
            case: "ref",
            value: { target: { case: "id", value: { namespace: "infra", name: "db", version: "v1" } } },
          },
        },
      ],
    });
    const r = sp.validate(s, { primary: { host: "h" } });
    expect(r.ok).toBe(false);
    expect(r.errors.find((e) => e.field === "primary")?.code).toBe("ref");
  });
});

// ===========================================================================
// SDK parity: mirror the Go scenarios that exercise the shared WASM engine, so
// both SDKs are confirmed to behave identically (and protojson round-trips).
// ===========================================================================

describe("parity: numeric constraints", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "num", version: "v1" },
    fields: [
      { name: "a", kind: { case: "int32", value: { gt: 0, lte: 10 } } },
      { name: "b", kind: { case: "double", value: { multipleOf: 0.5 } } },
      { name: "c", kind: { case: "int64", value: { in: [1n, 2n, 3n] } } },
    ],
  });
  it("accepts values within all bounds", () => {
    expect(sp.validate(s, { a: 5, b: 1.5, c: 2 }).ok).toBe(true);
  });
  it.each([
    [{ a: 0 }, "a"],
    [{ a: 11 }, "a"],
    [{ b: 1.3 }, "b"],
    [{ c: 9 }, "c"],
  ])("rejects %o on field %s", (vals, field) => {
    expect(sp.validate(s, vals).errors.some((e) => e.field === field)).toBe(true);
  });
});

describe("parity: string constraints", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "str", version: "v1" },
    fields: [
      { name: "s", kind: { case: "string", value: { minLen: 2n, maxLen: 4n, pattern: "^[a-z]+$" } } },
      { name: "e", kind: { case: "string", value: { in: ["x", "y"] } } },
    ],
  });
  it("accepts a valid string", () => {
    expect(sp.validate(s, { s: "abc", e: "x" }).ok).toBe(true);
  });
  it.each([
    [{ s: "a" }],     // too short
    [{ s: "abcde" }], // too long
    [{ s: "AB" }],    // pattern
    [{ e: "z" }],     // not in allowlist
  ])("rejects %o", (vals) => {
    expect(sp.validate(s, vals).ok).toBe(false);
  });
});

describe("parity: list constraints", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "lst", version: "v1" },
    fields: [
      {
        name: "tags",
        kind: {
          case: "list",
          value: { minItems: 1n, maxItems: 3n, unique: true, items: [{ name: "t", kind: { case: "string", value: { minLen: 1n } } }] },
        },
      },
    ],
  });
  it("accepts a valid unique list", () => {
    expect(sp.validate(s, { tags: ["a", "b"] }).ok).toBe(true);
  });
  it("rejects too few / too many / duplicate / bad item", () => {
    expect(sp.validate(s, { tags: [] }).ok).toBe(false);
    expect(sp.validate(s, { tags: ["a", "b", "c", "d"] }).ok).toBe(false);
    expect(sp.validate(s, { tags: ["a", "a"] }).ok).toBe(false);
    expect(sp.validate(s, { tags: [""] }).ok).toBe(false);
  });
});

describe("parity: bool + enum", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "be", version: "v1" },
    fields: [
      { name: "flag", kind: { case: "bool", value: { const: true } } },
      { name: "role", kind: { case: "enum", value: { values: { 1: "user", 2: "admin" }, definedOnly: true } } },
    ],
  });
  it("accepts const bool + defined enum", () => {
    expect(sp.validate(s, { flag: true, role: 2 }).ok).toBe(true);
  });
  it("rejects wrong const + undefined enum", () => {
    expect(sp.validate(s, { flag: false }).ok).toBe(false);
    expect(sp.validate(s, { flag: true, role: 9 }).ok).toBe(false);
  });
});

describe("parity: duration + timestamp", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "dt", version: "v1" },
    fields: [
      // create() takes the message-init form; toJson emits "1s" / RFC3339.
      { name: "d", kind: { case: "duration", value: { gte: { seconds: 1n }, lte: { seconds: 60n } } } },
      { name: "ts", kind: { case: "timestamp", value: { gte: { seconds: 1577836800n } } } }, // 2020-01-01Z
    ],
  });
  it("accepts in-range duration + timestamp", () => {
    expect(sp.validate(s, { d: "30s", ts: "2021-01-01T00:00:00Z" }).ok).toBe(true);
  });
  it("rejects out-of-range and unparseable", () => {
    expect(sp.validate(s, { d: "90s" }).ok).toBe(false);
    expect(sp.validate(s, { ts: "2019-01-01T00:00:00Z" }).ok).toBe(false);
    expect(sp.validate(s, { d: "nope" }).ok).toBe(false);
  });
});

describe("parity: immutable", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "imm", version: "v1" },
    fields: [{ name: "id", immutable: true, kind: { case: "int32", value: { default: 7 } } }],
  });
  it("accepts the fixed default", () => {
    expect(sp.validate(s, { id: 7 }).ok).toBe(true);
  });
  it("rejects a changed immutable value", () => {
    expect(sp.validate(s, { id: 8 }).ok).toBe(false);
  });
});

describe("parity: nullable + required", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "nr", version: "v1" },
    fields: [
      { name: "req", required: true, kind: { case: "string", value: {} } },
      { name: "opt", nullable: true, kind: { case: "string", value: {} } },
      { name: "nn", kind: { case: "string", value: {} } },
    ],
  });
  it("allows explicit null on a nullable field", () => {
    expect(sp.validate(s, { req: "x", opt: null }).ok).toBe(true);
  });
  it("requires a missing required field", () => {
    expect(sp.validate(s, {}).errors.find((e) => e.field === "req")?.code).toBe("required");
  });
  it("rejects null on a non-nullable field with code not_null", () => {
    expect(sp.validate(s, { req: "x", nn: null }).errors.find((e) => e.field === "nn")?.code).toBe("not_null");
  });
});

describe("parity: compute defaults + nested", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "comp", version: "v1" },
    fields: [
      { name: "qty", kind: { case: "int32", value: { default: 3 } } },
      { name: "price", kind: { case: "double", value: { default: 2.0 } } },
      { name: "total", kind: { case: "computed", value: { expr: "root.qty * root.price", result: RT.DOUBLE } } },
      {
        name: "box",
        kind: {
          case: "object",
          value: {
            schema: {
              fields: [
                { name: "w", kind: { case: "int32", value: { default: 4 } } },
                { name: "area", kind: { case: "computed", value: { expr: "root.box.w * 2", result: RT.INT64 } } },
              ],
            },
          },
        },
      },
    ],
  });
  it("fills defaults and evaluates computed (incl. nested)", () => {
    const r = sp.compute(s, { box: {} });
    expect(r.errors).toHaveLength(0);
    expect(r.values.qty).toBe(3);
    expect(r.values.total).toBe(6); // 3 * 2.0
    expect((r.values.box as Record<string, unknown>).area).toBe(8); // 4 * 2
  });
});

describe("parity: Link (identity composition via WASM)", () => {
  const db = create(SchemaSchema, {
    id: { namespace: "infra", name: "db", version: "v1" },
    fields: [
      { name: "host", required: true, kind: { case: "string", value: {} } },
      { name: "port", kind: { case: "int32", value: { gte: 1, lte: 65535, default: 5432 } } },
    ],
  });
  const idRef = (name: string, id: { namespace: string; name: string; version: string }) => ({
    name,
    required: true,
    kind: { case: "ref" as const, value: { target: { case: "id" as const, value: id } } },
  });

  it("resolves an identity ref then validates standalone", () => {
    const cfg = create(SchemaSchema, {
      id: { namespace: "app", name: "cfg", version: "v1" },
      fields: [idRef("primary", { namespace: "infra", name: "db", version: "v1" })],
    });
    const linked = sp.link(cfg, [db]);
    expect(sp.validate(linked, { primary: { host: "h" } }).ok).toBe(true);
    expect(sp.validate(linked, { primary: {} }).ok).toBe(false); // host required
    // identity preserved on the node post-link
    const ref = linked.fields[0].kind.value as { target: { case: string; value: { name: string } } };
    expect(ref.target.case).toBe("id");
    expect(ref.target.value.name).toBe("db");
  });

  it("resolves transitively (A -> B -> C)", () => {
    const c = create(SchemaSchema, {
      id: { namespace: "x", name: "c", version: "v1" },
      fields: [{ name: "leaf", required: true, kind: { case: "string", value: {} } }],
    });
    const b = create(SchemaSchema, {
      id: { namespace: "x", name: "b", version: "v1" },
      fields: [idRef("c", { namespace: "x", name: "c", version: "v1" })],
    });
    const a = create(SchemaSchema, {
      id: { namespace: "x", name: "a", version: "v1" },
      fields: [idRef("b", { namespace: "x", name: "b", version: "v1" })],
    });
    const linked = sp.link(a, [b, c]);
    expect(sp.validate(linked, { b: { c: { leaf: "y" } } }).ok).toBe(true);
    expect(sp.validate(linked, { b: { c: {} } }).ok).toBe(false);
  });

  it("throws when an identity cannot be resolved", () => {
    const cfg = create(SchemaSchema, {
      id: { namespace: "app", name: "cfg", version: "v1" },
      fields: [idRef("x", { namespace: "infra", name: "missing", version: "v1" })],
    });
    expect(() => sp.link(cfg, [])).toThrow();
  });
});

describe("parity: renderer helpers (FieldActive / EnumOptions / ListCount)", () => {
  const s = create(SchemaSchema, {
    id: { namespace: "t", name: "rh", version: "v1" },
    fields: [
      { name: "flag", kind: { case: "bool", value: {} } },
      { name: "gated", when: "root.flag == true", kind: { case: "string", value: {} } },
      { name: "always", kind: { case: "string", value: {} } },
      {
        name: "version",
        kind: {
          case: "enum",
          value: { values: { 13: "13", 14: "14", 15: "15" }, optionsExpr: 'root.flag == true ? [14, 15] : [13]' },
        },
      },
      { name: "n", kind: { case: "int32", value: {} } },
      {
        name: "machines",
        kind: { case: "list", value: { countExpr: "root.n + 1", items: [{ name: "h", kind: { case: "string", value: {} } }] } },
      },
    ],
  });

  it("FieldActive reflects the when gate", () => {
    expect(sp.fieldActive(s, "always", {})).toBe(true);
    expect(sp.fieldActive(s, "gated", { flag: true })).toBe(true);
    expect(sp.fieldActive(s, "gated", { flag: false })).toBe(false);
    expect(() => sp.fieldActive(s, "nope", {})).toThrow();
  });

  it("EnumOptions returns the dynamic set", () => {
    expect(sp.enumOptions(s, "version", { flag: true }).sort()).toEqual([14, 15]);
    expect(sp.enumOptions(s, "version", { flag: false })).toEqual([13]);
  });

  it("ListCount derives the required length", () => {
    expect(sp.listCount(s, "machines", { n: 4 })).toBe(5);
  });
});

describe("render (Go text/template via WASM)", () => {
  const tmpl =
    "{{- range .Groups }}\n# === {{ .Name }} ===\n{{- range .Fields }}\n{{ .Name }} = {{ .Value }}{{ .Unit }}\n{{- end }}\n{{ end -}}";

  function confSchema(): Schema {
    return create(SchemaSchema, {
      id: { namespace: "pg", name: "postgresql", version: "16" },
      templates: { conf: tmpl },
      fields: [
        { name: "shared_buffers", kind: { case: "int64", value: { default: 128n } }, unit: "MB", group: "Resource Usage" },
        { name: "wal_level", kind: { case: "string", value: { in: ["minimal", "replica"], default: "replica" } }, group: "WAL" },
      ],
    });
  }

  it("renders the schema's named template identically to Go", () => {
    const s = confSchema();
    const { values } = sp.compute(s, {});
    const out = sp.render(s, values, "conf");
    expect(out).toContain("# === Resource Usage ===");
    expect(out).toContain("shared_buffers = 128MB");
    expect(out).toContain("# === WAL ===");
    expect(out).toContain("wal_level = replica");
  });

  it("throws on an unknown template", () => {
    expect(() => sp.render(confSchema(), {}, "nope")).toThrow();
  });
});
