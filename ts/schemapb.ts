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

import { toJson } from "@bufbuild/protobuf";
import { SchemaSchema, type Schema, type FieldErrorJson } from "./schemapb/schema_pb.ts";

export type { Schema, FieldErrorJson };

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
