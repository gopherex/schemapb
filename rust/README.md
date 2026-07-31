# schemapb (Rust)

Native Rust implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

```rust
use schemapb::{engine, validate, bake, formats};

let eng = engine::compile(schema, formats::FormatRegistry::new())?; // SchemaError on defects
let result = validate::validate(&eng, &mut values); // ValidationResult (data, not panics)
let outcome = bake::bake(&eng, &mut values);        // canonical Baked snapshot
let text = bake::render(&eng, "conf", &values);     // Mustache template from the schema
```

Schemas are authored as plain `prost` struct literals of the generated
types (with `..Default::default()`) — the idiomatic Rust equivalent of the
builder APIs in the other ports.

Runtime dependencies: `prost`/`pbjson` (protobuf + protoJSON),
`cel-interpreter`/`cel-parser` (CEL evaluation), `mustache`, `chrono`,
`semver`, `regex`.

Known deviations (tracked):
- no CEL evaluation cost limit: `cel-interpreter` does not expose one
  (the Go reference has `WithCostLimit`).
- timestamps flow through `chrono::DateTime` inside CEL expressions;
  the conformance goldens only exercise whole-second RFC 3339 values.
