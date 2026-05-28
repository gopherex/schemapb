import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import "@stroppy-io/schemapb/wasm_exec.js"; // sets globalThis.Go (static; avoids the
// dynamic import.meta.url resolution vitest+jsdom can't do)
import { act, renderHook, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { beforeAll, describe, expect, it } from "vitest";
import { Schemapb, SchemaSchema, Schema_Filed_ResultType as RT } from "@stroppy-io/schemapb";
import { useSchemaForm } from "./index.ts";

// Load the engine from the built wasm via the filesystem (vitest+jsdom rewrites
// import.meta.url to http:, so the SDK's auto-loader can't run here — we inject
// the engine instead, exactly as an app would for SSR / a shared instance).
let engine: Schemapb;
beforeAll(async () => {
  const bytes = await readFile(resolve(process.cwd(), "../schemapb/dist/schemapb.wasm"));
  engine = await Schemapb.load(bytes);
});

const schema = create(SchemaSchema, {
  id: { namespace: "t", name: "form", version: "v1" },
  fields: [
    { name: "name", required: true, kind: { case: "string", value: { minLen: 2n } } },
    { name: "qty", kind: { case: "int32", value: { default: 3 } } },
    { name: "total", kind: { case: "computed", value: { expr: "root.qty * 10", result: RT.INT64 } } },
    // gated: only active when advanced is on
    { name: "advanced", kind: { case: "bool", value: {} } },
    { name: "threads", when: "root.advanced == true", kind: { case: "int32", value: { gte: 1 } } },
  ],
});

describe("useSchemaForm", () => {
  it("loads the engine and reports ready", async () => {
    const { result } = renderHook(() => useSchemaForm({ engine, schema }));
    await waitFor(() => expect(result.current.ready).toBe(true));
  });

  it("validates on change and surfaces field errors by path", async () => {
    const { result } = renderHook(() => useSchemaForm({ engine, schema, initialValues: {} }));
    await waitFor(() => expect(result.current.ready).toBe(true));

    // name is required and missing.
    await waitFor(() => expect(result.current.errors.name?.code).toBe("required"));
    expect(result.current.isValid).toBe(false);

    // set a too-short name -> min_len.
    act(() => result.current.setValue("name", "x"));
    await waitFor(() => expect(result.current.errors.name?.code).toBe("min_len"));

    // valid name -> no errors.
    act(() => result.current.setValue("name", "alice"));
    await waitFor(() => expect(result.current.isValid).toBe(true));
  });

  it("exposes computed defaults + derived values", async () => {
    const { result } = renderHook(() => useSchemaForm({ engine, schema, initialValues: { name: "ok" } }));
    await waitFor(() => expect(result.current.ready).toBe(true));
    await waitFor(() => {
      expect(result.current.computed.qty).toBe(3); // default
      expect(result.current.computed.total).toBe(30); // 3 * 10
    });
  });

  it("reflects the `when` gate via fieldActive", async () => {
    const { result } = renderHook(() => useSchemaForm({ engine, schema, initialValues: { name: "ok" } }));
    await waitFor(() => expect(result.current.ready).toBe(true));

    expect(result.current.fieldActive("threads")).toBe(false);
    act(() => result.current.setValue("advanced", true));
    await waitFor(() => expect(result.current.fieldActive("threads")).toBe(true));
  });

  it("register() wires a controlled input and handleSubmit bakes when valid", async () => {
    const { result } = renderHook(() => useSchemaForm({ engine, schema, initialValues: { name: "alice" } }));
    await waitFor(() => expect(result.current.ready).toBe(true));

    // register exposes value + onChange.
    expect(result.current.register("name").value).toBe("alice");
    act(() => result.current.register("name").onChange({ target: { value: "bob" } }));
    await waitFor(() => expect(result.current.values.name).toBe("bob"));

    // handleSubmit: valid form -> onValid(baked) with resolved values.
    let baked: unknown;
    act(() => result.current.handleSubmit((b) => (baked = b))());
    await waitFor(() => expect(baked).toBeTruthy());
    expect((baked as { values: Record<string, unknown> }).values.total).toBe(30);

    // invalid form -> onInvalid.
    act(() => result.current.setValue("name", "x")); // too short
    let invalidCalled = false;
    act(() =>
      result.current.handleSubmit(
        () => {},
        () => (invalidCalled = true),
      )(),
    );
    await waitFor(() => expect(invalidCalled).toBe(true));
  });
});
