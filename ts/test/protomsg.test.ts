import {
  create,
  createFileRegistry,
  type DescMessage,
  fromJson,
  toBinary,
} from "@bufbuild/protobuf";
import { base64Encode } from "@bufbuild/protobuf/wire";
import {
  AnySchema,
  DurationSchema,
  FieldMaskSchema,
  FileDescriptorProtoSchema,
  file_google_protobuf_any,
  file_google_protobuf_duration,
  file_google_protobuf_field_mask,
  file_google_protobuf_struct,
  file_google_protobuf_timestamp,
  file_google_protobuf_wrappers,
  TimestampSchema,
} from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";
import { SchemaSchema } from "../src/gen/schemapb/schema_pb.js";
import * as b from "../src/new.js";
import {
  messageValues,
  UnsupportedFieldError,
  ValidationFailure,
  Validator,
} from "../src/protomsg.js";
import { id, Version } from "../src/typed.js";
import { structFromNative } from "../src/value.js";

const opt = "LABEL_OPTIONAL";
const rep = "LABEL_REPEATED";

const fileJson = {
  name: "test/v1/test.proto",
  package: "test.v1",
  syntax: "proto3",
  dependency: [
    "google/protobuf/timestamp.proto",
    "google/protobuf/duration.proto",
    "google/protobuf/wrappers.proto",
    "google/protobuf/struct.proto",
    "google/protobuf/field_mask.proto",
    "google/protobuf/any.proto",
  ],
  enumType: [
    {
      name: "Tier",
      value: [
        { name: "TIER_UNSPECIFIED", number: 0 },
        { name: "TIER_GOLD", number: 1 },
      ],
    },
  ],
  messageType: [
    {
      name: "Item",
      field: [{ name: "sku", number: 1, label: opt, type: "TYPE_STRING", jsonName: "sku" }],
    },
    {
      name: "Order",
      field: [
        { name: "id", number: 1, label: opt, type: "TYPE_INT32" },
        {
          name: "note",
          number: 2,
          label: opt,
          type: "TYPE_STRING",
          proto3Optional: true,
          oneofIndex: 0,
        },
        { name: "tier", number: 3, label: opt, type: "TYPE_ENUM", typeName: ".test.v1.Tier" },
        { name: "items", number: 4, label: rep, type: "TYPE_MESSAGE", typeName: ".test.v1.Item" },
        {
          name: "by_sku",
          number: 5,
          label: rep,
          type: "TYPE_MESSAGE",
          typeName: ".test.v1.Order.BySkuEntry",
        },
        {
          name: "at",
          number: 6,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.Timestamp",
        },
        {
          name: "ttl",
          number: 7,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.Duration",
        },
        {
          name: "limit",
          number: 8,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.UInt32Value",
        },
        {
          name: "meta",
          number: 9,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.Struct",
        },
        {
          name: "mask",
          number: 10,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.FieldMask",
        },
        { name: "blob", number: 11, label: opt, type: "TYPE_BYTES" },
        {
          name: "any",
          number: 12,
          label: opt,
          type: "TYPE_MESSAGE",
          typeName: ".google.protobuf.Any",
        },
        { name: "item", number: 13, label: opt, type: "TYPE_MESSAGE", typeName: ".test.v1.Item" },
      ],
      nestedType: [
        {
          name: "BySkuEntry",
          field: [
            { name: "key", number: 1, label: opt, type: "TYPE_STRING" },
            {
              name: "value",
              number: 2,
              label: opt,
              type: "TYPE_MESSAGE",
              typeName: ".test.v1.Item",
            },
          ],
          options: { mapEntry: true },
        },
      ],
      oneofDecl: [{ name: "_note" }],
    },
  ],
};

const registry = createFileRegistry(fromJson(FileDescriptorProtoSchema, fileJson), (name) =>
  [
    file_google_protobuf_timestamp,
    file_google_protobuf_duration,
    file_google_protobuf_wrappers,
    file_google_protobuf_struct,
    file_google_protobuf_field_mask,
    file_google_protobuf_any,
  ].find((f) => f.proto.name === name),
);

const orderDesc = registry.getMessage("test.v1.Order") as DescMessage;
const itemDesc = registry.getMessage("test.v1.Item") as DescMessage;

// The engine-facing shape is a dynamic message here; generated code passes
// typed protobuf-es messages of the same shape.
// biome-ignore lint/suspicious/noExplicitAny: dynamic test messages
type Dyn = any;

const item = (sku: string): Dyn => create(itemDesc, { sku } as Dyn);

/** What the generator emits for Order (subset), as base64 binary. */
function orderSchema(): string {
  const { schema } = b
    .newSchema(id("test.v1", "Order", Version.zero))
    .fields(
      b.int32("id").gte(1),
      b.str("note").minLen(3n),
      b.list("items", b.ref("item", "test.v1.Item")).minItems(2n),
      b.mapOf("by_sku", b.ref("value", "test.v1.Item")),
      b.ref("item", "test.v1.Item"),
    )
    .def("test.v1.Item", b.str("sku").minLen(2n))
    .build();
  return base64Encode(toBinary(SchemaSchema, schema));
}

describe("protomsg", () => {
  it("maps a message to engine values like the Go bridge", () => {
    const meta = { a: 1.5, b: null, c: ["x"] };
    const order: Dyn = create(orderDesc, {
      tier: 1,
      items: [item("a")],
      bySku: { k: item("b") },
      at: create(TimestampSchema, { seconds: 10n, nanos: 5 }),
      ttl: create(DurationSchema, { seconds: 1n, nanos: 500_000_000 }),
      limit: 7,
      meta,
      mask: create(FieldMaskSchema, { paths: ["a.b"] }),
    } as Dyn);

    const got = messageValues(orderDesc, order);
    const want = {
      id: 0n,
      tier: "TIER_GOLD",
      items: [{ sku: "a" }],
      by_sku: { k: { sku: "b" } },
      at: create(TimestampSchema, { seconds: 10n, nanos: 5 }),
      ttl: create(DurationSchema, { seconds: 1n, nanos: 500_000_000 }),
      limit: 7n,
      meta: { a: 1.5, b: null, c: ["x"] },
      mask: ["a.b"],
      blob: new Uint8Array(),
    };
    expect(structFromNative(got)).toEqual(structFromNative(want));

    order.any = create(AnySchema, {});
    expect(() => messageValues(orderDesc, order)).toThrow(UnsupportedFieldError);
  });

  it("validates with field numbers and schema identity", () => {
    const order: Dyn = create(orderDesc, {
      note: "x",
      items: [item("a")],
      bySku: { sku: item("b") },
      item: item("c"),
    } as Dyn);

    const v = new Validator(orderDesc, orderSchema());
    let failure: ValidationFailure | undefined;
    try {
      v.validate(order);
    } catch (e) {
      failure = e as ValidationFailure;
    }
    expect(failure).toBeInstanceOf(ValidationFailure);
    const res = failure?.result;
    expect(res?.schema?.namespace).toBe("test.v1");
    expect(res?.schema?.name).toBe("Order");

    const got = Object.fromEntries(
      (res?.errors ?? []).map((e) => [e.path, e.pathSegments.map((s) => s.fieldNumber ?? 0)]),
    );
    expect(got).toMatchObject({
      id: [1],
      note: [2],
      items: [4],
      "items[0].sku": [4, 0, 1],
      "by_sku.sku.sku": [5, 0, 1],
      "item.sku": [13, 1],
    });
  });

  it("passes a valid message", () => {
    const order: Dyn = create(orderDesc, { id: 1, items: [item("aa"), item("bb")] } as Dyn);
    const res = new Validator(orderDesc, orderSchema()).validate(order);
    expect(res.errors).toEqual([]);
  });
});
