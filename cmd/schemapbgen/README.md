# schemapbgen — Generate typed Go structs from schemapb schemas

`schemapbgen` is a code generator that reads a `schemapb.Schema` and emits a
typed Go struct with full roundtrip/validation/sugar, losing no dynamic rule.

## Installation

Download a prebuilt binary for your platform from the
[latest release](https://github.com/stroppy-io/schemapb/releases) (archives named
`schemapbgen_<tag>_<os>_<arch>`).

Or build from a clone:

```bash
git clone https://github.com/stroppy-io/schemapb
cd schemapb
go build -o schemapbgen ./cmd/schemapbgen
# or run directly:
go run ./cmd/schemapbgen -in schema.json -out config_gen.go -pkg myconfig
```

> `go install github.com/stroppy-io/schemapb/cmd/schemapbgen@latest` is **not**
> supported: this is a separate module that builds against the in-tree library
> through the repo's `go.work`, and `go install` ignores workspaces.

## Phase 1: Generate from protojson files

Read a schema from a protojson file and emit typed Go structs:

```bash
schemapbgen -in schema.json -out config_gen.go -pkg myconfig
```

Flags:
- `-in` (repeatable): input protojson schema file(s), or a directory of `.json` files
- `-out`: output Go file (single input only); if omitted or multiple inputs provided, output filename derives from the schema identity (e.g., `InfraDiskV1_gen.go`)
- `-pkg`: package name of generated code (required)

Example:

```bash
schemapbgen -in configs/disk.json -pkg gen
# → outputs configs/InfraDiskV1_gen.go
```

## Phase 2: Generate from Go builder code

Use a `//go:generate` directive to capture schemas built with the Go builder API:

```go
//go:generate go run github.com/stroppy-io/schemapb/cmd/schemapbgen -from-go-code . -symbol BuildDiskSchema -pkg myconfig

package config

import "github.com/stroppy-io/schemapb/schemapb"

// BuildDiskSchema is called by schemapbgen during code generation.
func BuildDiskSchema() *schemapb.Schema {
	return schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
	).MustBuild()
}
```

Then run:

```bash
go generate ./...
```

Flags:
- `-from-go-code`: directory of a Go module exposing a schema provider function
- `-symbol`: exported provider function returning `*schemapb.Schema` or `[]*schemapb.Schema`
- `-pkg`: package name of generated code (required)
- `-out`: (optional) output directory; defaults to the current directory

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
| String       | `string` |
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
