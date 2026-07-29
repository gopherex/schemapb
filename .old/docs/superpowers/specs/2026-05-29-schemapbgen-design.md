# schemapbgen — Go struct codegen from schemapb schemas

Date: 2026-05-29
Status: Design (approved-in-conversation, pending written review)

## 1. Goal

A CLI tool that reads a `schemapb.Schema` and emits a typed Go struct mirror of
the form's **data shape**, plus a full sugar layer for roundtripping, defaults,
validation, and serialization. The user codes against `cfg.SharedBuffers`
instead of `map[string]any`, while no dynamic rule from the schema is lost.

Use case: **roundtrip** — struct → values (submit/validate) and values → struct
(read results), both directions typed.

## 2. The hard boundary (why this is feasible)

A `Schema` is two things in one descriptor:

1. **Data shape** — kinds, nesting, lists, refs, nullable/required. Maps cleanly
   to Go types. This is what becomes structs.
2. **Dynamic logic** — `when` gates, `Computed.expr`, `Rule.expr`,
   `options_expr`, `count_expr`, `normalize`, `coerce`. These are **expr-lang
   expressions evaluated at runtime** (NOT CEL — the proto doc comments saying
   cel-go/cel-es are stale).

Go's type system cannot express "field X is required when root.Y > 3". So
dynamic logic does NOT become types. It is preserved two ways:

- as `// when: <expr>`, `// rule: <expr>`, `// computed: <expr>` comments on the
  generated fields/types, and
- by embedding the **entire original schema as protobuf wire bytes** in the
  generated file and routing validation through the existing schemapb engine.

The struct layer stays simple; the conditions live at runtime where they belong.

## 3. Type mapping (data shape)

| schema kind | Go type |
|-------------|---------|
| Float / Double | `float32` / `float64` |
| Int32 / Int64 | `int32` / `int64` |
| UInt32 / UInt64 | `uint32` / `uint64` |
| Bool | `bool` |
| String | `string` |
| Enum | named `int32` type + const block + `String()` + `Parse…()` |
| Duration | `time.Duration` |
| Timestamp | `time.Time` |
| List | `[]T` (element type from item schema) |
| Object | nested generated struct |
| OneOf | interface + per-variant struct types (see §5) |
| Ref(name) | `*T` pointing at the generated def type (pointer always — recursion / reuse) |
| Computed | normal field carrying the derived output (see §6) |

### Pointer vs value rules

| field attribute | Go field |
|-----------------|----------|
| `required` && !`nullable` | `T` (value, must be set) |
| !`required` | `*T` (nil = absent) |
| `nullable` | `*T` (nil = explicit null) |
| List / OneOf | `[]T` / interface — already nil-able, no extra pointer |
| Ref | `*T` always |

JSON tags: `json:"name,omitempty"` for `*T`/slices, `json:"name"` for required
values.

**Defaults on FromValues:** a nil `*T` stays nil. The generator does NOT
back-fill `default` from the schema during `FromValues`; the schemapb engine
applies defaults at validation time. Back-filling here would desync from the
expr evaluation. (Explicit defaults are available via `Default()` — see §7.)

## 4. Naming — protobuf-style, `_`-separated nesting levels

Root name derived from `SchemaIdentity` = `namespace + name + version`, each
segment PascalCased, concatenated (no separator between identity parts), empty
parts skipped:

```
{namespace:"infra", name:"disk",        version:"v1"} → InfraDiskV1
{namespace:"",       name:"user",        version:"v2"} → UserV2
{namespace:"infra", name:"disk_config", version:"1.2.0"} → InfraDiskConfigV1_2_0
```

Normalization: `_ - .` separators split words → PascalCase; in a version, `.` →
`_` to stay a valid identifier (`1.2.0` → `V1_2_0`).

Nested types append a `_`-separated PascalCase segment per nesting level, exactly
like protobuf-gen-go (`Schema_Filed_Float`):

```
InfraDiskV1
InfraDiskV1_Wal                 (Object field "wal")
InfraDiskV1_Wal_Segment         (Object inside wal)
InfraDiskV1_Storage             (OneOf type for field "storage")
InfraDiskV1_Storage_Local       (variant "local")
InfraDiskV1_Storage_S3          (variant "s3")
InfraDiskV1_Node                (def "node"; ref → *InfraDiskV1_Node)
InfraDiskV1_TagsItem            (element type of list field "tags")
```

- **Defs** are named from the def key under the root identity (`InfraDiskV1_Node`),
  not from any referencing field — they are reusable/recursive.
- **List element** name is the literal field name + `Item` (no singularization):
  `tags` → `InfraDiskV1_TagsItem`. Predictable over clever.
- **Collisions** are impossible by construction (root identity is unique). If one
  is ever detected, the generator **fails with an explicit error** rather than
  auto-suffixing.

## 5. OneOf representation — interface (protobuf style)

```go
type InfraDiskV1_Storage interface{ isInfraDiskV1_Storage() }

type InfraDiskV1_Storage_Local struct { /* variant fields */ }
func (*InfraDiskV1_Storage_Local) isInfraDiskV1_Storage() {}

type InfraDiskV1_Storage_S3 struct { /* variant fields */ }
func (*InfraDiskV1_Storage_S3) isInfraDiskV1_Storage() {}
```

The parent struct holds `Storage InfraDiskV1_Storage`. Read with a type switch.
The discriminator value is mapped to/from the concrete variant type during
`ToValues`/`FromValues`.

## 6. Computed fields

Generated as a normal struct field with a `// computed: <expr>` comment and
`json:"name,omitempty"`. On `ToValues` the computed field is **omitted** (the
engine recomputes it authoritatively). On `FromValues` it is **read** (so a
`Baked` result carries the resolved value back into the struct).

## 7. Generated sugar (per schema)

Constructors / defaults:
- `func DefaultInfraDiskV1() *InfraDiskV1` — fresh instance with schema `default`
  values filled recursively.
- `func (c *InfraDiskV1) Default()` — fills defaults into already-empty fields of
  an existing instance.

Getters (protobuf-style, nil-safe):
- `func (c *InfraDiskV1) GetSharedBuffers() int64` — returns zero value / schema
  default if receiver or field is nil.

Builder sugar:
- `func NewInfraDiskV1() *InfraDiskV1` + chainable `WithSharedBuffers(v) *InfraDiskV1` …

Roundtrip / dump-undump:
- `func (c *InfraDiskV1) ToValues() (*structpb.Struct, error)`
- `func FromValuesInfraDiskV1(v *structpb.Struct) (*InfraDiskV1, error)`
- `func (c *InfraDiskV1) ToFilled() *schemapb.Filled` / `ToBaked() *schemapb.Baked`

Serialization:
- `MarshalJSON` / `UnmarshalJSON` routed through ToValues/FromValues so omitempty
  and computed-omission behave consistently.

Clone:
- `func (c *InfraDiskV1) Clone() *InfraDiskV1` — deep copy.

Schema wrapping / sugar around the schema:
- `var _schemaInfraDiskV1 []byte` — the original schema as **protobuf wire bytes**,
  decoded lazily once via `sync.Once` + `proto.Unmarshal`.
- `func (c *InfraDiskV1) Schema() *schemapb.Schema` — the decoded schema.
- `func (c *InfraDiskV1) Validate() []*schemapb.FieldError` — struct → values →
  existing schemapb engine → field errors.
- `var InfraDiskV1Identity = &schemapb.SchemaIdentity{…}` — the identity constant.

Enum sugar:
- `type InfraDiskV1_WalLevel int32` + const block + `String()` (from the schema's
  label map) + `ParseInfraDiskV1_WalLevel(s string) (…, bool)`.

## 8. Runtime dependency

Generated code imports `github.com/stroppy-io/schemapb/schemapb` for `_schema`
decoding, `FieldError`, the validation engine, and the proto well-known types.
This is an accepted hard dependency.

## 9. CLI — `cmd/schemapbgen`

Lives at `cmd/schemapbgen` as **its own Go module** (`go.mod`) so its CLI deps
(cobra) do not pollute the library module. It depends on the library module.

### Phase 1 — generate from JSON

```
schemapbgen -in schema.json -out schema_gen.go -pkg myconfig
```

- `-in` — a schemapb JSON file (protojson of one `Schema`). Repeatable, or a
  directory.
- One **output file per schema** (`-out` names it for a single input; for a
  directory the output filename derives from the schema identity).
- `-pkg` — package name of the generated file.

### Phase 2 — generate from Go builder code (`-from-go-code`)

```go
//go:generate go run github.com/stroppy-io/schemapb/cmd/schemapbgen -from-go-code . -symbol BuildDiskSchema -out schema_gen.go -pkg myconfig
```

The user exposes a provider function `func() *schemapb.Schema` (or
`[]*schemapb.Schema`). The generator:

1. Writes a temporary `main.go` that imports the user package and calls the named
   `-symbol`.
2. `go run`s it to dump the schema (protobuf wire / protojson) to stdout. **The
   user's Go code must compile.**
3. Reads the dump and runs the Phase-1 generation path.

## 10. Components / isolation

- **parse**: load schema from protojson (Phase 1) or via the builder-dump bridge
  (Phase 2) → `*schemapb.Schema`.
- **model**: walk the schema into a naming-resolved IR (types, fields, enums,
  oneofs, defs, collision check). One pure function, fully testable.
- **emit**: render the IR to Go source (structs, sugar, embedded wire bytes),
  `go/format` the output.
- **cli**: cobra wiring, flags, file/dir IO, the Phase-2 go-run bridge.

Each unit testable independently: parse from fixtures, model from a `*Schema`,
emit from an IR snapshot (golden files), cli via integration tests.

## 11. Testing

- Golden-file tests: fixture schemas (covering every kind, nesting, oneof, ref
  recursion, computed, enum) → expected generated `.go`, compiled + vet'd in CI.
- Roundtrip property tests: struct → ToValues → FromValues → struct is identity
  (modulo computed/defaults).
- Validation integration: generated `Validate()` agrees with calling the engine
  directly on the same values.
- Collision test: two schemas / fields that would collide → generator errors.

## 12. Out of scope (YAGNI)

- Name override annotations / side mapping files (identity uniqueness is enough).
- Generating from a registry or binary `.pb` directly (JSON + builder cover it).
- TS/other-language output (separate effort).
