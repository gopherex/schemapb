# schemapb (Rust)

Native Rust implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

## Install

The crate is not published to crates.io; depend on a release tag of the
repository:

```toml
[dependencies]
schemapb = { git = "https://github.com/gopherex/schemapb", tag = "v1.13.0", features = ["derive"] }
```

Features: `derive` (`#[derive(Validate)]`, `#[derive(Reflect)]`), `uuid`
(`uuid::Uuid` fields, format `uuid`), `url` (`url::Url` fields, format
`url`). chrono types are always supported.

## Validating structs (feature `derive`)

`#[derive(Validate)]` is the `validator`-crate workflow on the whole
schemapb engine: bounds, lengths, patterns, formats, CEL rules on fields
and on the struct, defaults, normalization and computed fields. No proto
in sight:

```rust
use schemapb::{Email, Reflect, Validate};
use serde::{Deserialize, Serialize};
use std::time::Duration;

#[derive(Validate, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
#[schemapb(rule("root.minReplicas <= root.maxReplicas", message = "min above max", id = "range"))]
struct Deployment {
    #[schemapb(min = 1, max = 32, pattern = "^[a-z][a-z0-9-]*$")]
    name: String,
    owner: Email,                                  // format from the type
    #[schemapb(gte = 1, lte = 64, default = 4)]
    workers: Option<u32>,                          // None takes the default
    min_replicas: u32,
    max_replicas: u32,
    #[schemapb(min = 1, unique, items(pattern = "^[a-z]+$"))]
    tags: Vec<String>,
    #[schemapb(lte = Duration::from_secs(3600), default = Duration::from_secs(30))]
    timeout: Option<Duration>,
    #[schemapb(default = Tier::Standard)]
    tier: Option<Tier>,
}

#[derive(Reflect, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum Tier { Standard, HighMemory }

deployment.validate()?;             // Result<Report, Invalid>: warnings or the violations
let report = deployment.check();    // every error and warning: path, code, message
let full = deployment.resolved()?;  // defaults, normalization, computed fields filled in
```

Attribute values are Rust expressions typed by the field: `gte` on a
`String`, `pattern` on a number or `lte = 300` on a `u8` do not compile,
with the reason at the attribute. Field attributes: `required`, `secret`,
`skip`, `unique`, `finite`, `deprecated`, `immutable`, `nullable`; `gt`,
`gte`, `lt`, `lte`, `min`, `max`, `len`, `default`, `example`; `pattern`,
`format`, `desc`, `title`, `unit`, `group`, `normalize`, `when`, `rename`;
`rule("expr", message = "..", id = "..", warn)`, `oneof(..)`,
`annotation("key", value)`; `items(..)`, `keys(..)`, `values(..)` with the
constraints of list items, map keys and map values. Struct attributes:
`rule(..)`, `strict`, and the schema identity `namespace`, `name`,
`version` (defaults: the crate name, the type name).

serde shapes the schema like it shapes the data: `rename`, `rename_all`,
`skip`, `transparent`, and `tag` for internally tagged enums. A unit enum
is a choice of its wire names; an internally tagged enum
(`#[serde(tag = "kind")]`) is a discriminated union whose variants validate
their own fields. `flatten`, `untagged` and adjacent tagging do not compile.

Rust types keep their honest kinds and ranges:

| Rust type | Schema |
|---|---|
| `i8`, `i16`, `u8`, `u16` | int32 / uint32 with the type's range |
| `i32`, `i64`, `isize`, `u32`, `u64`, `usize` | int32 / int64 / uint32 / uint64 |
| `NonZero*` | the integer, zero excluded |
| `f32`, `f64` | float / double |
| `String`, `&str`, `Cow<str>`, `PathBuf` | string |
| `char` | string of one code point |
| `Email`, `Hostname`, `IpAddr`, `Ipv4Addr`, `Ipv6Addr`, `Uuid`, `Url`, `NaiveDate`, `NaiveTime` | string with the format |
| `Vec<u8>`, `[u8; N]` | bytes (`[u8; N]` of length N) |
| `Vec<T>`, `VecDeque<T>`, `[T; N]`, tuples | list (arrays of exactly N, tuples positional) |
| `HashSet<T>`, `BTreeSet<T>` | list of unique items |
| `HashMap<String, T>`, `BTreeMap<String, T>` | map |
| `std::time::Duration`, `pbjson_types::Duration` | duration (std: never negative) |
| `SystemTime`, `chrono::DateTime<Tz>`, `pbjson_types::Timestamp` | timestamp |
| `Option<T>` | optional and nullable; `None` takes the default |
| `Box<T>`, `Rc<T>`, `Arc<T>` | the inner type |
| a derived struct | an object (an optional section with its defaults) |
| `serde_json::Value`, `Values` | JSON |

The schema compiles once per type on first use (`Type::validator()`); a
CEL rule or a pattern with an error panics there with the type's name.

## Dynamic schemas: fields defined at run time

A tracker's custom fields, a form built by its users: the schema is data,
built in code or loaded from storage, and the values live in your own
types as `Values`:

```rust
use schemapb::build::*;
use schemapb::{Validator, Values};

let project = schema("tracker", "project-42")
    .field(string("title").min_len(1).required())
    .field(int64("estimate").gte(0).unit("h"))
    .field(choice("status").str_opts(["open", "done"]).default("open"))
    .field(duration("spent"))
    .field(list("labels", string("label").pattern("^[a-z]+$")).unique())
    .rule(rule("root.estimate < 1000", "estimate too large").id("estimate-cap"))
    .build()?;                                      // Validator

let stored = project.schema_json();                 // keep the schema in the database
let project = Validator::from_json(&stored)?;       // and load it back

#[derive(Serialize, Deserialize)]
struct Issue { id: u64, custom: Values }            // custom fields in a jsonb column

let mut custom = Values::new().with("title", "crash").with("estimate", 8_i64);
project.resolve(&mut custom)?;                      // defaults and kinds, only when valid
let status: Option<String> = custom.get("status")?; // Some("open")
let report = project.check(&custom);                // violations as plain data
```

`Values` serializes as plain JSON (`{"estimate":8,"spent":"1h30m0s"}`),
queryable in the database: integers are exact, bytes standard base64,
durations Go form, timestamps RFC 3339. The builder's schemas coerce by
default, so values read back from that JSON regain their kinds when they
are resolved. The builder speaks the Go, TypeScript and Python builders'
vocabulary; the kitchen-sink schema built with it equals the Go golden.

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

Schemas come from the builder (`schemapb::build`), from a derived type,
from storage (`Validator::from_json`) or as plain `prost` struct literals
of the generated types.

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

`#[derive(Reflect)]` produces the schema only; `#[derive(Validate)]` adds
validation on top (see above). Both take the same attributes.

Nested types recurse through the `ReflectField` trait — implementing it
for your own type IS the override mechanism (serde-style). `Option<T>` is
optional+nullable, `Vec<u8>` is Bytes, `[T; N]` a fixed-length list,
string-keyed maps become Map, `serde_json::Value` JSON, `pbjson_types`
Duration/Timestamp their kinds, and a type cycle degrades to JSON. The
derive mirror model is pinned by conformance/golden/reflect.json.

`#[schemapb(finite)]` on an `f32`/`f64` rejects NaN and the infinities;
bounds (`gte`, `lt`, ...) always reject NaN.

Struct sections are optional by default. A field whose type is a derived
struct (not `Option<T>`) reflects as an Object with an implicit empty
default, the same as Go's `schemapb:"default={}"`: an absent section is
materialized and its inner defaults apply, and a missing required inner
field is reported at its own path (`db.host`), not at the section. Add
`#[schemapb(required)]` to require the section itself (Go's
`validate:"required"`). `Option<T>` fields, hand-written `ReflectField`
impls (overrides), list items and map values are unchanged.

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

Runtime dependencies: `prost`/`pbjson` (protobuf + protoJSON), `cel` (CEL
parsing and evaluation; a malformed expression is a schema error, never a
crash), `mustache`, `chrono`, `semver`, `regex`.

## Development

From the repository root: `make configure` once, then `make lint-rust`
(clippy pedantic+nursery as errors + rustfmt) / `make test-rust`
(conformance suite).

Known deviations (tracked):
- no CEL evaluation cost limit: `cel` offers a comprehension iteration
  budget, not cel-go's cost model (the Go reference has `WithCostLimit`).
- timestamps flow through `chrono::DateTime` inside CEL expressions;
  the conformance goldens only exercise whole-second RFC 3339 values.
- no typed-message bridge (Go `protomsg`, TS `Validator`) and no generator:
  `ValidationResult.schema` and `PathSegment.field_number` are never set
  here, and ErrorCode specs come from the template table (checked against
  `messages.json`) rather than from the `error_spec` options.
