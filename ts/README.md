# @stroppy-io/schemapb (TypeScript / WASM)

The validator and the derived-value (`Computed`) engine are implemented once, in
Go, and compiled to WebAssembly. The browser/Node client runs the **same**
expr-lang engine as the server, so validation and computation agree exactly —
nothing is reimplemented in TypeScript.

This directory is the npm package. (The Go side installs separately with
`go get github.com/stroppy-io/schemapb/schema`.)

## Build

From the repo root you need the Go toolchain (for the wasm) and Node:

```sh
cd ts
npm install
npm run build        # runs `make wasm` then bundles with tsup → dist/ + *.wasm
```

`build` produces:

- `dist/index.js` + `dist/index.d.ts` — the wrapper and re-exported protobuf-es types
- `schemapb.wasm` — the Go engine
- `wasm_exec.js` — Go's wasm loader glue

All three are listed in `package.json#files`, so they ship in the published
tarball.

## Install from another project

The consumer machine does **not** need Go — the prebuilt `schemapb.wasm` ships
in the package. Three ways:

```sh
# 1. published registry
npm install @stroppy-io/schemapb

# 2. local path (monorepo / sibling checkout)
npm install file:../schemapb/ts

# 3. git — NOTE: only works if dist/ + schemapb.wasm are committed (the consumer
#    can't run the Go build). Prefer (1) or (2), or commit the artifacts.
npm install github:stroppy-io/schemapb#main --workspace ts
```

## Use (browser / Vite)

```ts
import "@stroppy-io/schemapb/wasm_exec.js";          // defines global `Go`
import wasmUrl from "@stroppy-io/schemapb/schemapb.wasm?url";
import { create } from "@bufbuild/protobuf";
import { Schemapb, SchemaSchema, Schema_Filed_ResultType as ResultType } from "@stroppy-io/schemapb";

const bytes = await (await fetch(wasmUrl)).arrayBuffer();
const sp = await Schemapb.load(bytes);

const schema = create(SchemaSchema, {
  id: { namespace: "infra", name: "disk", version: "v1" },
  fields: [
    { name: "disk_type", required: true,
      kind: { case: "enum", value: { values: { 1: "ssd", 2: "hdd" }, definedOnly: true } } },
    { name: "disk_size", required: true,
      kind: { case: "int32", value: { gte: 1 } } },
    { name: "iops",
      kind: { case: "computed", value: {
        expr: "root.disk_type == 1 ? min(max(root.disk_size * 50, 3000), 16000) : min(max(root.disk_size * 5, 100), 3000)",
        result: ResultType.INT64 } } },
  ],
});

sp.compute(schema, { disk_type: 1, disk_size: 100 });
// { values: { disk_type:1, disk_size:100, iops:5000, ... }, errors: [] }
sp.validate(schema, { disk_type: 3, disk_size: 0 });
// { ok: false, errors: [ { field: "disk_type", ... }, ... ] }
```

The form `values` are a plain object (not a proto message); they're passed
through as-is. Build the `schema` with `create(SchemaSchema, …)` for full type
safety.

## Node

`wasm_exec.js` defines a global `Go`. In Node, evaluate it once (e.g. via
`node --experimental-wasm-modules`, or run the file to set `globalThis.Go`),
read the wasm with `fs.readFile`, then call `Schemapb.load(bytes)` the same way.

## Notes

- `Schemapb.load` runs the Go program, which registers the global functions and
  blocks (`select {}`) to stay alive. Load once, reuse the instance.
- Errors are returned as protobuf-es `FieldErrorJson[]` (`field`, `message`,
  `ruleId?`, `severity`).
