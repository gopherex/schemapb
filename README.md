# schemapb

A runtime, proto-defined form/config schema descriptor with a validation and
derived-value engine — implemented natively in **Go, TypeScript, Python and
Rust**, with no shared runtime (no WASM, no FFI).

A producer builds a `Schema` and ships it as a protobuf message (binary on
the wire, protoJSON for debugging). Any consumer — in any of the four
languages — then:

- **validates** values against it (typed errors as data: `path`, `ErrorCode`,
  expected/actual, severity — in deterministic order),
- **resolves** defaults, coercion, normalization and `Computed` fields
  ([CEL](https://cel.dev) expressions),
- **bakes** the result into a canonical, typed `Baked` snapshot,
- **renders** Mustache templates carried by the schema.

Cross-implementation agreement is pinned by a conformance suite
(`conformance/golden`), not by sharing code: the Go implementation is the
reference and writes the goldens; every other implementation must reproduce
them byte-for-byte. See [docs/CONFORMANCE.md](docs/CONFORMANCE.md).

## Feature highlights

- **Typed `Value` container** — a oneof per kind (int32/64, uint32/64,
  float/double, bool, string, bytes, duration, timestamp, list, struct,
  json), so schema defaults, options and error payloads are never stringly
  typed.
- **Full field toolbox** — numeric/string/bytes constraints, `Choice`
  (typed options with labels, open/closed, CEL `options_expr`), lists
  (including fixed-shape tuples), maps, objects, discriminated `OneOf`,
  `Ref` into `$defs`, `Computed`, `when` activation, `immutable`, `secret`
  (masked everywhere), severity levels.
- **CEL everywhere logic lives** — computed fields, validation rules,
  activation conditions, dynamic choice options and list counts share one
  expression language across all four implementations (strings extension
  included).
- **Open format registry** — ten mandatory core string formats (`email`,
  `url`, `uuid`, `ipv4`, `ipv6`, `ip`, `hostname`, `date`, `time`,
  `datetime`); custom formats are registered per engine; an unknown format
  fails loudly (`UNSUPPORTED_FORMAT`), never silently.
- **Identity-based registry** — schemas are declared once with a typed
  identity (`namespace/name@vX.Y.Z`, opaque semver), stored in a strict
  registry (`Put` refuses overwrites) and composed via `Link`.
- **Errors as data** — validation returns a `ValidationResult` proto, not
  exceptions; message texts come from a spec-owned template set identical in
  all languages.

## Installation

| Language | Package | Install |
|---|---|---|
| Go | `github.com/gopherex/schemapb/go` | `go get github.com/gopherex/schemapb/go@latest` |
| TypeScript | [`@gopherex/schemapb`](https://www.npmjs.com/package/@gopherex/schemapb) | `npm install @gopherex/schemapb` (or `yarn add`) |
| Python | [`schemapb`](https://pypi.org/project/schemapb/) | `pip install schemapb` |
| Rust | [`schemapb`](https://crates.io/crates/schemapb) | `cargo add schemapb` |

Each implementation ships only light native dependencies (a CEL evaluator, a
Mustache renderer, protobuf codegen output). Per-language details, quickstarts
and tracked deviations: [go/](go/README.md) · [ts/](ts/README.md) ·
[py/](py/README.md) · [rust/](rust/README.md).

## A taste (Go, the reference)

```go
import spb "github.com/gopherex/schemapb/go/schemapb"

schema := spb.NewSchema(spb.ID("shared", "service", spb.Ver(1, 0, 0))).
	Fields(
		spb.Str("name").Required().MinLen(1),
		spb.Int64("replicas").Default(1).Gte(1).Lte(9),
		spb.Computed("memory_mb", "replicas * 256"),
	).
	Template("conf", "{{name}}: {{values.memory_mb}}MB").
	MustBuild() // *Schema — a plain proto message, ship it anywhere

engine, err := spb.Compile(schema)          // compile once
res := engine.Validate(values)              // errors as data
baked, res, err := engine.Bake(values)      // canonical snapshot
text, err := engine.Render("conf", values)  // Mustache
```

The same schema — the same bytes — validates, computes and renders
identically from TypeScript, Python and Rust.

## Repository layout

```
schemapb/     the contract: value.proto, schema.proto, errors.proto,
              runtime.proto + easyp.<lang>.yaml codegen configs
go/           Go reference implementation (writes the goldens)
ts/           TypeScript implementation
py/           Python implementation
rust/         Rust implementation
conformance/  golden fixtures every implementation must reproduce
docs/         PRINCIPLES.md, CONFORMANCE.md, RELEASING.md
```

## Development

Prerequisites: `go`, `node`/`npm`, `cargo`, `python3`. Everything else is
version-pinned and installed into `./bin` — nothing global:

```sh
make configure   # fetch all pinned tools (easyp, protoc plugins, linters…)
make gen         # regenerate protobuf code for all 4 languages
make lint        # proto lint; lint-go / lint-ts / lint-py / lint-rust per language
make test-go test-ts test-py test-rust
```

`make help` lists every target. CI runs the same gates
(`.github/workflows/ci.yml`). Releasing is `make release`: one `vX.Y.Z` tag
publishes all four languages in lockstep
([docs/RELEASING.md](docs/RELEASING.md)).

Design rules that hold across all four implementations (typed identifier
domains, honest 64-bit integers, determinism, secrets never leak, …):
[docs/PRINCIPLES.md](docs/PRINCIPLES.md).

## License

[MIT](LICENSE)
