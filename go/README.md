# schemapb (Go)

Go **reference implementation** of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. This implementation writes the cross-language
conformance goldens (`conformance/golden` in the repository root) that the
TypeScript, Python and Rust implementations must reproduce.

## Install

```sh
go get github.com/gopherex/schemapb/go@latest
```

Requires Go ≥ the version in `go.mod`. Runtime dependencies:
`google.golang.org/protobuf`, `github.com/google/cel-go`,
`github.com/cbroglie/mustache`, `golang.org/x/mod/semver`.

## Quickstart

```go
import spb "github.com/gopherex/schemapb/go/schemapb"

// Identity is declared once and reused (typed domains, opaque semver).
id := spb.ID("shared", "service", spb.Ver(1, 0, 0))

schema := spb.NewSchema(id).
	Fields(
		spb.Str("name").Required().MinLen(1),
		spb.Int64("replicas").Default(1).Gte(1).Lte(9),
		spb.Computed("memory_mb", "replicas * 256"),
	).
	Template("conf", "{{name}}: {{values.memory_mb}}MB").
	MustBuild()

engine, err := spb.Compile(schema,
	spb.WithFormats(myFormats),   // extend the core format registry
	spb.WithCostLimit(1_000_000), // cap CEL evaluation cost
)

res := engine.Validate(values)             // *ValidationResult — errors as data
values, res = engine.Resolve(values)       // defaults, coercion, computed
baked, res, err := engine.Bake(values)     // canonical *Baked snapshot
text, err := engine.Render("conf", values) // Mustache from the schema
```

`example_test.go` walks the entire public API (builders, registry + `Link`,
`Choice`, `OneOf`, `Ref`, tuples, secrets, merge) in one runnable example.

## Development

From the repository root: `make configure` once, then `make lint-go` /
`make test-go`. Regenerate goldens after intentional behaviour changes with
`go test ./schemapb -run Golden -update` (run from `go/`) — every other
language's conformance suite depends on them.
