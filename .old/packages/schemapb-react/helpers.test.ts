import { describe, expect, it } from "vitest";
import { errorsByField, getPath, setPath, splitPath } from "./helpers.ts";

describe("splitPath", () => {
  it("splits dotted and bracketed paths", () => {
    expect(splitPath("a.b.c")).toEqual(["a", "b", "c"]);
    expect(splitPath("a.b[0].c")).toEqual(["a", "b", "0", "c"]);
    expect(splitPath("tags.2")).toEqual(["tags", "2"]);
    expect(splitPath("")).toEqual([]);
  });
});

describe("getPath", () => {
  const obj = { a: { b: [{ c: 1 }, { c: 2 }] }, x: 5 };
  it("reads nested + indexed values", () => {
    expect(getPath(obj, "x")).toBe(5);
    expect(getPath(obj, "a.b[1].c")).toBe(2);
    expect(getPath(obj, "a.b.0.c")).toBe(1);
  });
  it("returns undefined for absent paths", () => {
    expect(getPath(obj, "a.z")).toBeUndefined();
    expect(getPath(obj, "nope.deep")).toBeUndefined();
  });
});

describe("setPath", () => {
  it("sets a top-level value immutably", () => {
    const o = { a: 1 };
    const n = setPath(o, "a", 2);
    expect(n.a).toBe(2);
    expect(o.a).toBe(1); // original untouched
  });
  it("creates nested objects", () => {
    const n = setPath({}, "a.b.c", 9);
    expect(getPath(n, "a.b.c")).toBe(9);
  });
  it("creates arrays for numeric segments", () => {
    const n = setPath({}, "tags.0", "x");
    expect(Array.isArray((n as Record<string, unknown>).tags)).toBe(true);
    expect(getPath(n, "tags.0")).toBe("x");
  });
  it("does not mutate nested branches of the source", () => {
    const o = { a: { b: 1 } };
    const n = setPath(o, "a.c", 2);
    expect(o.a).toEqual({ b: 1 });
    expect(getPath(n, "a.b")).toBe(1);
    expect(getPath(n, "a.c")).toBe(2);
  });
});

describe("errorsByField", () => {
  it("indexes by field, first error wins", () => {
    const m = errorsByField([
      { field: "a", message: "m1", code: "required", severity: "ERROR" },
      { field: "a", message: "m2", code: "type", severity: "ERROR" },
      { field: "b", message: "m3", code: "min_len", severity: "ERROR" },
    ] as never);
    expect(m.a.code).toBe("required");
    expect(m.b.code).toBe("min_len");
  });
});
