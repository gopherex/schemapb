# schemapb (Rust)

Native Rust implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

## Install

```sh
cargo add schemapb
```

## Quickstart

```rust
use schemapb::engine::Engine;
use schemapb::formats::FormatRegistry;

let engine = Engine::compile(schema, FormatRegistry::new())?; // SchemaError on defects
let result = engine.validate(&mut values); // ValidationResult (data, not panics)
let outcome = engine.bake(&mut values);    // canonical Baked snapshot
let text = engine.render("conf", &values); // Mustache template from the schema

let field = schema.lookup_path("logging.collector")?; // schema path lookup
let n: i64 = i64::try_from(baked_value)?;             // typed extraction (TryFrom<&Value>)
```

Schemas are authored as plain `prost` struct literals of the generated
types (with `..Default::default()`) — the idiomatic Rust equivalent of the
builder APIs in the other ports.

## Deriving a Schema from a struct (feature `derive`)

```toml
schemapb = { version = "...", features = ["derive"] }
```

```rust
use schemapb::Reflect;
use schemapb::reflect::reflect_schema;

#[derive(Reflect)]
struct Params {
    #[schemapb(desc = "display name", min = 1, max = 64)]
    name: String,
    #[schemapb(gte = 1, lte = 9)]
    replicas: i64,
    opt: Option<String>, // optional + nullable
}

let schema = reflect_schema::<Params>(id)?;
```

Nested types recurse through the `ReflectField` trait — implementing it
for your own type IS the override mechanism (serde-style). `Option<T>` is
optional+nullable, `Vec<u8>` is Bytes, `[T; N]` a fixed-length list,
string-keyed maps become Map, `serde_json::Value` JSON, `pbjson_types`
Duration/Timestamp their kinds, and a type cycle degrades to JSON. The
derive mirror model is pinned by conformance/golden/reflect.json.

## Using schemapb types from your own protos

When your `.proto` files embed schemapb messages (`schemapb.Schema`,
`schemapb.Value`, …), point prost at this crate instead of generating a
second copy:

```rust
// build.rs
prost_build::Config::new()
    .extern_path(".schemapb", "::schemapb::gen::schemapb")
    .compile_protos(&["proto/my_service.proto"], &["proto"])?;
```

Your generated code then references this crate's types directly — no
conversion layer. The path `schemapb::gen::schemapb` is a stability
promise.

Runtime dependencies: `prost`/`pbjson` (protobuf + protoJSON),
`cel-interpreter`/`cel-parser` (CEL evaluation), `mustache`, `chrono`,
`semver`, `regex`.

## Development

From the repository root: `make configure` once, then `make lint-rust`
(clippy pedantic+nursery as errors + rustfmt) / `make test-rust`
(conformance suite).

Known deviations (tracked):
- no CEL evaluation cost limit: `cel-interpreter` does not expose one
  (the Go reference has `WithCostLimit`).
- timestamps flow through `chrono::DateTime` inside CEL expressions;
  the conformance goldens only exercise whole-second RFC 3339 values.
