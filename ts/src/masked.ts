import { clone, create } from "@bufbuild/protobuf";
import { listItemDef, refDefKey } from "./compute.js";
import type { Baked } from "./gen/schemapb/runtime_pb.js";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import {
  type StructValue,
  StructValueSchema,
  type Value,
  ValueSchema,
} from "./gen/schemapb/value_pb.js";

/** Independent display values; no resolution/CEL. Present secrets become "***". */
export function masked(baked: Baked): StructValue {
  const out = baked.values ? clone(StructValueSchema, baked.values) : create(StructValueSchema);
  maskStruct(baked.schema, out, baked.schema?.defs ?? {});
  return out;
}

function own<T>(values: Record<string, T>, key: string): T | undefined {
  return Object.hasOwn(values, key) ? values[key] : undefined;
}

function hidden(): Value {
  return create(ValueSchema, { kind: { case: "stringValue", value: "***" } });
}

function maskStruct(
  schema: Schema | undefined,
  values: StructValue,
  defs: Record<string, Schema>,
): void {
  if (!schema) {
    for (const key of Object.keys(values.fields)) values.fields[key] = hidden();
    return;
  }
  for (const field of schema.fields) {
    const value = values.fields[field.name];
    if (Object.hasOwn(values.fields, field.name) && value)
      values.fields[field.name] = maskValue(field, value, defs);
  }
}

function maskObject(schema: Schema | undefined, value: Value, defs: Record<string, Schema>): Value {
  if (!schema || value.kind.case !== "structValue") return hidden();
  maskStruct(schema, value.kind.value, defs);
  return value;
}

function maskValue(field: Schema_Field, value: Value, defs: Record<string, Schema>): Value {
  if (field.secret) return hidden();
  if (value.kind.case === "nullValue" || value.kind.case === undefined) return value;
  const k = field.kind;
  switch (k.case) {
    case "object":
      return maskObject(k.value.schema, value, defs);
    case "ref":
      return maskObject(own(defs, refDefKey(k.value)), value, defs);
    case "oneOf": {
      const disc =
        value.kind.case === "structValue"
          ? own(value.kind.value.fields, k.value.discriminator)
          : undefined;
      const schema =
        disc?.kind.case === "stringValue" ? own(k.value.variants, disc.kind.value) : undefined;
      return maskObject(schema, value, defs);
    }
    case "list":
      if (value.kind.case !== "listValue") return hidden();
      value.kind.value.items = value.kind.value.items.map((item, i) => {
        const def = listItemDef(k.value, i);
        return def ? maskValue(def, item, defs) : item;
      });
      break;
    case "map":
      if (value.kind.case !== "structValue") return hidden();
      for (const [key, item] of Object.entries(value.kind.value.fields)) {
        if (k.value.valueField)
          value.kind.value.fields[key] = maskValue(k.value.valueField, item, defs);
        else if (k.value.valueSchema && item.kind.case !== "nullValue")
          value.kind.value.fields[key] = maskObject(k.value.valueSchema, item, defs);
      }
      break;
  }
  return value;
}
