// Pure, framework-agnostic helpers used by the hook. Kept separate so the
// tricky bits (path get/set, error indexing) are unit-tested without React.

import type { FieldErrorJson } from "@stroppy-io/schemapb";

type Dict = Record<string, unknown>;

// splitPath turns "a.b[0].c" / "a.b.0.c" into ["a","b","0","c"].
export function splitPath(path: string): string[] {
  return path
    .replace(/\[(\w+)\]/g, ".$1")
    .split(".")
    .filter((s) => s.length > 0);
}

// getPath reads a nested value by dotted/indexed path; undefined if absent.
export function getPath(obj: unknown, path: string): unknown {
  let cur: unknown = obj;
  for (const key of splitPath(path)) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = (cur as Dict)[key];
  }
  return cur;
}

// setPath returns a shallow-cloned copy of obj with value written at path.
// Numeric segments create/extend arrays; string segments create objects.
export function setPath<T extends Dict>(obj: T, path: string, value: unknown): T {
  const keys = splitPath(path);
  if (keys.length === 0) return obj;

  const root: Dict = Array.isArray(obj) ? [...(obj as unknown[])] as unknown as Dict : { ...obj };
  let cur: Dict = root;
  for (let i = 0; i < keys.length - 1; i++) {
    const key = keys[i];
    const nextIsIndex = /^\d+$/.test(keys[i + 1]);
    const existing = cur[key];
    const clone: Dict = Array.isArray(existing)
      ? ([...(existing as unknown[])] as unknown as Dict)
      : existing && typeof existing === "object"
        ? { ...(existing as Dict) }
        : nextIsIndex
          ? ([] as unknown as Dict)
          : {};
    cur[key] = clone;
    cur = clone;
  }
  cur[keys[keys.length - 1]] = value;
  return root as T;
}

// errorsByField indexes a flat FieldError list by field path (first wins).
export function errorsByField(errors: FieldErrorJson[]): Record<string, FieldErrorJson> {
  const out: Record<string, FieldErrorJson> = {};
  for (const e of errors) {
    const field = e.field ?? "";
    if (!(field in out)) out[field] = e;
  }
  return out;
}
