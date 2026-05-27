// Public entry point for the @stroppy-io/schemapb package.
export * from "./schemapb.ts";
// Re-export the generated protobuf-es types so consumers build schemas with
// `create(SchemaSchema, ...)` without a second import path.
export * from "./schemapb/schema_pb.ts";
