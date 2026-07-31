/**
 * Bake: validate + resolve, then seal the values with the schema into a
 * Baked snapshot in canonical wire form. Mirrors the Go reference bake.go.
 */

import { create, equals } from "@bufbuild/protobuf";
import Mustache from "mustache";
import { fieldIsActive, refDefKey, selectVariant } from "./compute.js";
import type { Engine } from "./engine.js";
import type { ValidationResult } from "./gen/schemapb/errors_pb.js";
import type { Baked, Filled } from "./gen/schemapb/runtime_pb.js";
import { BakedSchema } from "./gen/schemapb/runtime_pb.js";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import { SchemaSchema } from "./gen/schemapb/schema_pb.js";
import type { StructValue, Value } from "./gen/schemapb/value_pb.js";
import { StructValueSchema } from "./gen/schemapb/value_pb.js";
import { displayString, type RenderContext, type RenderField, renderField } from "./render.js";
import type { TemplateName } from "./typed.js";
import { resultBlocking, validate } from "./validate.js";
import {
  canonicalStruct,
  canonicalValue,
  fromNative,
  isNativeStruct,
  type Native,
  type NativeStruct,
  structToNative,
} from "./value.js";

export interface BakeOutcome {
  baked?: Baked;
  result: ValidationResult;
}

/**
 * Validates and resolves values, then seals them (canonical wire variants).
 * On a blocking failure `baked` is absent; warnings do not block.
 */
export function bake(e: Engine, values: NativeStruct): BakeOutcome {
  const result = validate(e, values);
  if (resultBlocking(result)) {
    return { result };
  }
  const baked = create(BakedSchema, {
    schema: e.schema,
    values: canonicalEngineStruct(e, values),
  });
  return { baked, result };
}

/** Projects a resolved native form into canonical wire variants. */
function canonicalEngineStruct(e: Engine, values: NativeStruct): StructValue {
  const fields: Record<string, Value> = {};
  for (const [name, val] of Object.entries(values)) {
    const f = e.schema.fields.find((x) => x.name === name);
    fields[name] = canonicalTop(e, f, val);
  }
  return create(StructValueSchema, { fields });
}

function canonicalTop(e: Engine, f: Schema_Field | undefined, val: Native): Value {
  if (f === undefined) {
    return fromNative(val);
  }
  if (f.kind.case === "ref") {
    const def = e.schema.defs[refDefKey(f.kind.value)];
    if (def !== undefined && isNativeStruct(val)) {
      return canonicalStruct(def, val);
    }
    return fromNative(val);
  }
  if (f.kind.case === "oneOf") {
    const sel = selectVariant(f.kind.value, val);
    if (sel !== undefined) {
      return canonicalStruct(sel[0], sel[1]);
    }
    return fromNative(val);
  }
  try {
    return canonicalValue(f, val);
  } catch {
    return fromNative(val);
  }
}

/**
 * Layers overrides onto a baked form (objects merge recursively, lists
 * append unless replaceLists, scalars overwrite) and re-seals on this
 * engine.
 */
export function merge(
  e: Engine,
  baked: Baked,
  overrides: StructValue,
  replaceLists: boolean,
): BakeOutcome {
  const base = structToNative(baked.values);
  const over = structToNative(overrides);
  return bake(e, mergeStructs(base, over, replaceLists));
}

function mergeStructs(dst: NativeStruct, src: NativeStruct, replaceLists: boolean): NativeStruct {
  const out: NativeStruct = { ...dst };
  for (const [k, sv] of Object.entries(src)) {
    const dv = out[k];
    if (dv !== undefined) {
      if (isNativeStruct(dv) && isNativeStruct(sv)) {
        out[k] = mergeStructs(dv, sv, replaceLists);
        continue;
      }
      if (!replaceLists && Array.isArray(dv) && Array.isArray(sv)) {
        out[k] = [...dv, ...sv];
        continue;
      }
    }
    out[k] = sv;
  }
  return out;
}

/** Whether the baked schema is identical in content to s. */
export function bakedMatches(baked: Baked, s: Schema): boolean {
  return baked.schema !== undefined && equals(SchemaSchema, baked.schema, s);
}

/**
 * Validates and seals an inline Filled. Throws when the Filled references a
 * schema by id (a registry must resolve it first).
 */
export function filledBake(filled: Filled, compile: (s: Schema) => Engine): BakeOutcome {
  const source = filled.schema?.source;
  if (source?.case !== "schema") {
    throw new Error(
      "schemapb: Filled bake requires an inline schema (id refs resolve via a registry)",
    );
  }
  const e = compile(source.value);
  return bake(e, structToNative(filled.values));
}

/**
 * Renders a schema-carried Mustache template against resolved values.
 * Returns undefined when the template does not exist.
 */
export function render(e: Engine, name: TemplateName, values: NativeStruct): string | undefined {
  const tmpl = e.schema.templates[name as string];
  if (tmpl === undefined) {
    return undefined;
  }
  return Mustache.render(tmpl, buildRenderContext(e, values));
}

/**
 * Builds the contract render context (fields / groups / values with
 * precomputed display forms); inactive fields are excluded entirely.
 */
export function buildRenderContext(e: Engine, values: NativeStruct): RenderContext {
  const fields: RenderField[] = [];
  const groups: { name: string; fields: RenderField[] }[] = [];
  const groupIdx = new Map<string, number>();

  for (const f of e.schema.fields) {
    if ((f.when ?? "") !== "" && !fieldIsActive(e, f, values, f.name, undefined)) {
      continue;
    }
    const rf = renderField(f, values);
    fields.push(rf);
    const g = f.group ?? "";
    let i = groupIdx.get(g);
    if (i === undefined) {
      i = groups.length;
      groupIdx.set(g, i);
      groups.push({ name: g, fields: [] });
    }
    groups[i]?.fields.push(rf);
  }

  const display: Record<string, string> = {};
  for (const [name, val] of Object.entries(values)) {
    display[name] = val === null ? "" : displayString(val);
  }
  return { fields, groups, values: display };
}

/** Renders a Baked snapshot with a template of its embedded schema. */
export function renderBaked(e: Engine, baked: Baked, name: TemplateName): string | undefined {
  return render(e, name, structToNative(baked.values));
}
