import { create } from "@bufbuild/protobuf";
import { DurationSchema } from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";
import { bake, renderBaked } from "../src/bake.js";
import { choiceOptions } from "../src/compute.js";
import { SchemaError } from "../src/descriptor.js";
import { ErrorCode } from "../src/gen/schemapb/errors_pb.js";
import * as b from "../src/new.js";
import { id, Version } from "../src/typed.js";
import { validate } from "../src/validate.js";
import { strV } from "../src/value.js";

describe("builder", () => {
  it("builds, validates, bakes and renders like the Go smoke test", () => {
    const { engine } = b
      .newSchema(id("infra", "pg", Version.of(1, 0, 0)))
      .fields(
        b.int64("shared_buffers").gte(16n).default(128n).unit("MB").group("Memory"),
        b.bool("autovacuum").default(true).group("Vacuum"),
        b
          .choice("wal_level")
          .opt(strV("minimal"), "Minimal")
          .opt(strV("replica"), "Replica")
          .default(strV("replica"))
          .group("WAL"),
        b.computed("cache", "root.shared_buffers * 3").result(b.ResultType.INT64),
        b.duration("timeout").default(create(DurationSchema, { seconds: 300n })),
        b.str("mode").default("Fast").normalize("this.lowerAscii()").in("fast", "slow"),
      )
      .rules(b.rule("int(root.shared_buffers) < 100000", "too big").warn())
      .build();

    const res = validate(engine, { shared_buffers: 8n });
    expect(res.errors.map((e) => `${e.path}:${ErrorCode[e.code]}`)).toContain(
      "shared_buffers:GTE_VIOLATED",
    );

    const outcome = bake(engine, {});
    expect(outcome.baked).toBeDefined();
    if (outcome.baked === undefined) {
      return;
    }
    const values = outcome.baked.values?.fields ?? {};
    expect(values["shared_buffers"]?.kind).toEqual({ case: "int64Value", value: 128n });
    expect(values["cache"]?.kind).toEqual({ case: "int64Value", value: 384n });
    expect(values["mode"]?.kind).toEqual({ case: "stringValue", value: "fast" });

    expect(choiceOptions(engine, "wal_level", {})).toEqual(["minimal", "replica"]);
  });

  it("rejects a broken schema at build time", () => {
    expect(() =>
      b
        .newSchema(id("t", "cycle", Version.of(0, 1, 0)))
        .fields(b.computed("a", "root.b + 1"), b.computed("b", "root.a + 1"))
        .build(),
    ).toThrow(SchemaError);
  });

  it("renders through a builder-made template", () => {
    const { engine } = b
      .newSchema(id("t", "tpl", Version.of(1, 0, 0)))
      .fields(b.str("name").default("main"))
      .template("out", "name = {{values.name}}\n")
      .build();
    const outcome = bake(engine, {});
    expect(outcome.baked).toBeDefined();
    if (outcome.baked !== undefined) {
      expect(renderBaked(engine, outcome.baked, "out" as never)).toBe("name = main\n");
    }
  });
});
