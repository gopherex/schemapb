# schemapb (TypeScript)

Native TypeScript implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

## Install

```sh
npm install @gopherex/schemapb   # or: yarn add @gopherex/schemapb
```

ESM-only, ships type declarations. Runtime dependencies:
`@bufbuild/protobuf`, `@bufbuild/cel`, `mustache`, `semver`.

## Quickstart

```ts
import * as spb from "@gopherex/schemapb";

// build() fully compiles: every schema defect surfaces here as SchemaError.
const { schema, engine } = spb
  .newSchema(spb.id("shared", "service", spb.Version.of(1, 0, 0)))
  .fields(
    spb.str("name").required().minLen(1n),
    spb.int64("replicas").default(1n).gte(1n).lte(9n), // int64 = bigint, honestly
    spb.computed("memory_mb", "replicas * 256"),
  )
  .template("conf", "{{name}}: {{values.memory_mb}}MB")
  .build(); // schema is a plain proto message — ship it anywhere

const result = spb.validate(engine, values); // ValidationResult — errors as data
const outcome = spb.bake(engine, values);    // canonical Baked snapshot
const text = spb.render(engine, "conf", values); // Mustache from the schema
```

64-bit integers are `bigint` end to end; identifier domains (`FieldName`,
`Namespace`, `TemplateName`, …) are branded types.

## Development

From the repository root: `make configure` once, then `make lint-ts` /
`make test-ts` (Biome + tsc + vitest; yarn 4 pinned via `yarnPath`).

Known deviations (tracked):

- no CEL evaluation cost limit: `@bufbuild/cel` does not expose one (the Go
  reference has `WithCostLimit`).
