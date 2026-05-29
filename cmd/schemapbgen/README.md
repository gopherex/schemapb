# schemapbgen — Generate typed Go structs from schemapb schemas

`schemapbgen` is a code generator that reads a `schemapb.Schema` and emits a
typed Go struct with full roundtrip/validation/sugar, losing no dynamic rule.

## Installation

```bash
go install github.com/stroppy-io/schemapb/cmd/schemapbgen@latest
```

Or download a prebuilt binary for your platform from the
[latest release](https://github.com/stroppy-io/schemapb/releases) (archives named
`schemapbgen_<tag>_<os>_<arch>`).

This CLI is a **separate Go module** pinned to a published `schemapb` library
version, so `go install ...@latest` builds cleanly (and `...@vX.Y.Z` once a
release tags `cmd/schemapbgen/vX.Y.Z`). The library and CLI are tagged together
by `make release`.

**Developing on both at once?** The CLI is pinned to a *released* library, so it
won't see unreleased library changes by default. Create a local (gitignored)
workspace:

```bash
make dev-workspace      # go work init . ./cmd/schemapbgen
```

## Phase 1: Generate from protojson files

Read a schema from a protojson file and emit typed Go structs:

```bash
schemapbgen --in schema.json --out config_gen.go --pkg myconfig
```

Flags:
- `-in` (repeatable): input protojson schema file(s), or a directory of `.json` files
- `-out`: output Go file (single input only); if omitted or multiple inputs provided, output filename derives from the schema identity (e.g., `InfraDiskV1_gen.go`)
- `-pkg`: package name of generated code (required)

Example:

```bash
schemapbgen --in configs/disk.json --pkg gen
# → outputs configs/InfraDiskV1_gen.go
```

## Phase 2: Generate from Go builder code

Write **provider functions** — exported, zero-arg, returning `*schemapb.Schema` or
`[]*schemapb.Schema` — and let the CLI discover and run them. One `//go:generate`
line per package generates for every provider in it:

```go
//go:generate go run github.com/stroppy-io/schemapb/cmd/schemapbgen@latest --from-go-code .

package databases

import "github.com/stroppy-io/schemapb/schemapb"

func PostgresSchema() *schemapb.Schema {
	return schemapb.NewSchema("databases", "postgres", "v1").Fields(
		schemapb.Int64("port").Default(5432),
	).MustBuild()
}

func MysqlSchema() *schemapb.Schema {
	return schemapb.NewSchema("databases", "mysql", "v1").Fields(
		schemapb.Int64("port").Default(3306),
	).MustBuild()
}
```

`go generate ./...` then writes one `<source>_gen.go` next to each source file (here
`postgres_gen.go`, `mysql_gen.go` if the funcs were in separate files, or both in one
file if they share a source file).

Flags (cobra long flags use **two** dashes):
- `--from-go-code <dir>`: the package directory to scan. May be any package (including a
  nested `internal/...` one), not a module root — the CLI walks up to the enclosing
  `go.mod` and imports packages by their full path. Providers may import other packages
  (e.g. shared schema helpers); they resolve normally.
- `--symbol <Func>`: (optional) generate for one specific provider; omit to **auto-discover**
  every provider in the package.
- `--recursive`: also scan and generate for sub-packages (each gets its own files, in its
  own package).
- `--names func|identity` (default `func`): how to name the generated Go type —
  `func` = provider name + `Schema` (e.g. `PostgresSchema` → type `PostgresSchemaSchema`;
  name the func `Postgres` to get `PostgresSchema`), or `identity` = from the schema
  identity (e.g. `DatabasesPostgresV1`).
- `--pkg <name>`: (optional) package clause of the generated files. Defaults to the source
  package, so generated code joins the same package.
- `--out <dir>`: (optional) output directory; defaults to the source directory.

**Per-function markers** (doc comments on the provider):
- `//schemapbgen:skip` — exclude this function from auto-discovery.
- `//schemapbgen:name <GoTypeName>` — set an explicit generated type name, overriding `--names`.

Provider code must compile; the CLI builds and runs it to capture the schemas. Generated
files (`*_gen.go`) and test files are ignored by discovery.

## Naming scheme

The root struct name is derived from `SchemaIdentity` (namespace + name + version),
with each segment PascalCased and concatenated:

```
{namespace: "infra", name: "disk", version: "v1"}       → InfraDiskV1
{namespace: "",      name: "user", version: "v2"}       → UserV2
{namespace: "infra", name: "disk_config", version: "1.2.0"} → InfraDiskConfigV1_2_0
```

Nested types use protobuf-style `_`-separated naming:

```
InfraDiskV1                 (root type)
InfraDiskV1_Wal             (nested Object field "wal")
InfraDiskV1_Storage         (OneOf field "storage")
InfraDiskV1_Storage_Local   (OneOf variant "local")
InfraDiskV1_Node            (named def "node")
InfraDiskV1_TagsItem        (list element type of field "tags")
```

## Generated API surface

### Roundtrip serialization

- `func (c *InfraDiskV1) ToValues() (*structpb.Struct, error)` — marshal struct to protobuf Values
- `func FromValuesInfraDiskV1(st *structpb.Struct) (*InfraDiskV1, error)` — unmarshal from Values
- `func (c *InfraDiskV1) ToFilled() (*schemapb.Filled, error)` — struct wrapped in a Filled envelope
- `func (c *InfraDiskV1) ToBaked() (*schemapb.Baked, error)` — struct + schema wrapped in a Baked envelope

### Validation

- `func (c *InfraDiskV1) Validate() []*schemapb.FieldError` — validates struct against the embedded schema
- `func (c *InfraDiskV1) Schema() *schemapb.Schema` — returns the original schema
- `var InfraDiskV1Identity = &schemapb.SchemaIdentity{...}` — the schema identity constant

### Constructors & defaults

- `func NewInfraDiskV1() *InfraDiskV1` — new empty instance
- `func DefaultInfraDiskV1() *InfraDiskV1` — new instance with schema defaults applied
- `func (c *InfraDiskV1) Default()` — fills defaults into empty fields of existing instance

### Getters (nil-safe, protobuf-style)

- `func (c *InfraDiskV1) GetSharedBuffers() int64` — returns zero value if receiver or field is nil
- `func (c *InfraDiskV1) GetWalLevel() string` — dereferences `*string`, returns empty string if nil

### Builder sugar

- `func (c *InfraDiskV1) WithSharedBuffers(v int64) *InfraDiskV1` — chainable field setter
- `func (c *InfraDiskV1) Clone() *InfraDiskV1` — deep copy via JSON roundtrip

### JSON/protobuf serialization

- `func (c *InfraDiskV1) MarshalJSON() ([]byte, error)` — standard `encoding/json` compat
- `func (c *InfraDiskV1) UnmarshalJSON([]byte) error` — standard `encoding/json` compat

## Dynamic logic preservation

The generator preserves all dynamic rules from the schema as both:

1. **Comments** — `// when: <expr>`, `// rule: <expr>`, `// computed: <expr>` on fields
2. **Embedded schema** — the original schema is stored as protobuf wire bytes and decoded at runtime, so all validation runs through the schemapb engine

The struct layer is simple (typed). The conditions live at runtime where they belong.

## Example usage

```go
package main

import (
	"log"
	"myconfig"
)

func main() {
	// Build
	cfg := myconfig.NewInfraDiskV1().
		WithSharedBuffers(16000).
		WithWalLevel("replica")

	// Validate
	errs := cfg.Validate()
	if len(errs) > 0 {
		log.Fatal(errs)
	}

	// Roundtrip to protobuf Values
	vals, err := cfg.ToValues()
	if err != nil {
		log.Fatal(err)
	}

	// ... submit vals to API, get back a Baked result ...

	// Restore struct from Values
	restored, err := myconfig.FromValuesInfraDiskV1(vals)
	if err != nil {
		log.Fatal(err)
	}

	// Clone for safe mutation
	copy := restored.Clone()
	copy.WithSharedBuffers(32000)
}
```

## Type mapping

| schema kind  | Go type |
|--------------|---------|
| Float/Double | `float32`/`float64` |
| Int32/Int64  | `int32`/`int64` |
| UInt32/UInt64 | `uint32`/`uint64` |
| Bool         | `bool` |
| String       | `string`; with `.In(...)` → a `type <Root>_<Field> = string` alias + a const per value (string enum) |
| Enum         | named `int32` type + const block + `String()` |
| Duration     | `time.Duration` |
| Timestamp    | `time.Time` |
| List         | `[]T` (element type from item schema) |
| Object       | nested generated struct |
| OneOf        | interface + per-variant struct types |
| Ref(name)    | `*T` (pointer to named def) |
| Computed     | normal field; omitted on `ToValues`, included on `FromValues` |

**Pointer rules:**
- Required, non-nullable → value type (must be set)
- Optional (not required) → pointer type (nil = absent)
- Nullable → pointer type (nil = explicit null)
- List / OneOf → no extra pointer (already nil-able)
- Ref → always pointer (recursion / reuse)

## List elements

A list's element type is derived from `items[0]` ONLY — the engine validates
every array element against the first item definition and ignores the rest. The
element follows the same kind mapping as any field:

| `items[0]` kind | element type |
|-----------------|--------------|
| scalar / enum / duration / timestamp | that Go type (e.g. `[]string`, `[]time.Duration`) |
| Object | `[]<Parent>_<Field>Item` (generated struct) |
| Ref | `[]<Def>` (the def type) |
| OneOf | `[]<Parent>_<Field>Item` (the element interface; per-element discriminator JSON) |

## Engine-compatible JSON

The generated `MarshalJSON`/`UnmarshalJSON` translate two kinds into the form the
schemapb engine expects, while keeping ergonomic Go field types:

- **Duration** (`time.Duration`, `*time.Duration`, `[]time.Duration`) serializes
  as a string parseable by `time.ParseDuration` (e.g. `"5m0s"`), not nanoseconds.
- **OneOf** (field or list element) serializes as a single object carrying the
  discriminator property alongside the active variant's fields, and decodes back
  into the concrete variant type.

`time.Time` already round-trips as RFC3339, which the engine accepts, so it needs
no special handling.
