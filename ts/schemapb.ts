// Thin TypeScript wrapper around the schemapb WASM module.
//
// The schema and validation/compute logic live in Go (one source of truth) and
// run in the browser (or Node) via WebAssembly, so the client uses the *same*
// expr-lang engine as the server — no reimplementation, no drift.
//
// Build the wasm first (see ts/README.md):
//   GOOS=js GOARCH=wasm go build -o ts/schemapb.wasm ./wasm
// and copy Go's loader:
//   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ts/wasm_exec.js

import { fromJson, toJson } from "@bufbuild/protobuf";
import {
  SchemaSchema,
  BakedSchema,
  type Schema,
  type Baked,
  type BakedJson,
  type FieldErrorJson,
} from "./schemapb/schema_pb.ts";

export type { Schema, Baked, BakedJson, FieldErrorJson };

export interface ValidateResult {
  ok: boolean;
  errors: FieldErrorJson[];
}

export interface ComputeResult {
  // Fully resolved form: provided inputs + values filled from schema defaults +
  // evaluated Computed (derived) fields.
  values: Record<string, unknown>;
  errors: FieldErrorJson[];
}

export interface BakeResult {
  // The sealed Baked (schema + final values); absent when errors blocked sealing.
  baked?: BakedJson;
  errors: FieldErrorJson[];
}

// The Go wasm glue (wasm_exec.js) defines a global `Go` class.
declare const Go: new () => {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): void;
};

declare global {
  // Registered by wasm/main.go.
  // eslint-disable-next-line no-var
  var schemapbValidate: (schemaJSON: string, valuesJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbCompute: (schemaJSON: string, valuesJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbBake: (schemaJSON: string, valuesJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbMerge: (bakedJSON: string, overridesJSON: string, replaceLists: boolean) => string;
  // eslint-disable-next-line no-var
  var schemapbLink: (schemaJSON: string, registrySchemasJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbFieldActive: (schemaJSON: string, field: string, rootJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbEnumOptions: (schemaJSON: string, field: string, rootJSON: string) => string;
  // eslint-disable-next-line no-var
  var schemapbListCount: (schemaJSON: string, field: string, rootJSON: string) => string;
}

export class Schemapb {
  private constructor() {}

  /** Instantiate the wasm module. `wasmBytes` is the compiled schemapb.wasm. */
  static async load(wasmBytes: BufferSource): Promise<Schemapb> {
    const go = new Go();
    const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
    go.run(instance); // registers the global functions; runs until program exit
    return new Schemapb();
  }

  /** Validate `values` (a plain form object) against `schema`. */
  validate(schema: Schema, values: unknown): ValidateResult {
    return this.call(globalThis.schemapbValidate, schema, values) as ValidateResult;
  }

  /** Evaluate the schema's derived (Computed) fields for `values`. */
  compute(schema: Schema, values: unknown): ComputeResult {
    return this.call(globalThis.schemapbCompute, schema, values) as ComputeResult;
  }

  /** Validate + resolve `values` and seal them with `schema` into a Baked. */
  bake(schema: Schema, values: unknown): BakeResult {
    return this.call(globalThis.schemapbBake, schema, values) as BakeResult;
  }

  /** Whether the named top-level field is active for `root` (its `when` gate).
   *  A field with no `when` is always active. Mirrors Go's `Schema.FieldActive`. */
  fieldActive(schema: Schema, field: string, root: unknown): boolean {
    return (this.helper(globalThis.schemapbFieldActive, schema, field, root) as { active: boolean }).active;
  }

  /** Allowed integer values for the named enum field given `root` (dynamic via
   *  `options_expr`, else the static values). Mirrors Go's `Schema.EnumOptions`. */
  enumOptions(schema: Schema, field: string, root: unknown): number[] {
    return (this.helper(globalThis.schemapbEnumOptions, schema, field, root) as { options: number[] }).options;
  }

  /** Required length of the named list field derived from its `count_expr` over
   *  `root`. Mirrors Go's `Schema.ListCount`. */
  listCount(schema: Schema, field: string, root: unknown): number {
    return (this.helper(globalThis.schemapbListCount, schema, field, root) as { count: number }).count;
  }

  private helper(
    fn: (s: string, field: string, root: string) => string,
    schema: Schema,
    field: string,
    root: unknown,
  ): unknown {
    const out = JSON.parse(fn(JSON.stringify(toJson(SchemaSchema, schema)), field, JSON.stringify(root ?? {})));
    if (out.error) throw new Error(out.error);
    return out;
  }

  /** Resolve every identity-Ref in `schema` against `registry` (a pool of
   *  built schemas), folding the referenced schemas into the result so it is
   *  self-contained and validates standalone. The id-ref nodes keep their
   *  identity. Mirrors Go's `Schema.Link`. */
  link(schema: Schema, registry: Schema[]): Schema {
    const schemaJSON = JSON.stringify(toJson(SchemaSchema, schema));
    const regJSON = JSON.stringify(registry.map((s) => toJson(SchemaSchema, s)));
    const out = JSON.parse(globalThis.schemapbLink(schemaJSON, regJSON));
    if (out.error) throw new Error(out.error);
    return fromJson(SchemaSchema, out.schema);
  }

  /** Layer `overrides` onto a `baked` snapshot and re-seal. Lists append unless
   *  `replaceLists` is set. */
  merge(baked: Baked, overrides: unknown, replaceLists = false): BakeResult {
    const bakedJSON = JSON.stringify(toJson(BakedSchema, baked));
    const out = JSON.parse(globalThis.schemapbMerge(bakedJSON, JSON.stringify(overrides ?? {}), replaceLists));
    if (out.error) throw new Error(out.error);
    return out as BakeResult;
  }

  private call(
    fn: (s: string, v: string) => string,
    schema: Schema,
    values: unknown,
  ): unknown {
    // toJson produces protojson, which the Go side reads with protojson.Unmarshal.
    const schemaJSON = JSON.stringify(toJson(SchemaSchema, schema));
    const out = JSON.parse(fn(schemaJSON, JSON.stringify(values ?? {})));
    if (out.error) throw new Error(out.error);
    return out;
  }
}
