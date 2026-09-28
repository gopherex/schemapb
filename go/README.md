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
		spb.Computed("memory_mb", "root.replicas * 256"),
	).
	Template("conf", "{{values.name}}: {{values.memory_mb}}MB").
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

`Reflect` / `ReflectType[T]` turn an existing Go type into a Schema (json
tags for names, the go-playground/validator vocabulary for constraints,
`WithType` for domain-type overrides) — see the package docs.

`example_test.go` walks the entire public API (builders, registry + `Link`,
`Choice`, `OneOf`, `Ref`, tuples, secrets, merge) in one runnable example.

## Reflection attributes

The `schemapb` struct tag applies protobuf attributes to the reflected field:

```go
type Config struct {
    Token   string        `json:"token" schemapb:"secret=true;default=token"`
    Workers int32         `json:"workers" schemapb:"default=4;gte=1;lte=64"`
    Timeout time.Duration `json:"timeout" schemapb:"default=30s"`
    Enabled bool          `json:"enabled" schemapb:"default=false"`
}
```

Attributes are discovered from protobuf descriptors. Common attributes such
as `secret`, `required`, `description`, `normalize` and `rules` address the
field; attributes such as `default`, `gte`, `pattern` and `min_len` address
its active kind. Both protobuf snake_case and JSON lowerCamelCase names are
accepted. Unknown attributes, duplicate names, invalid values, and attempts
to change `name` or the field kind fail reflection. List/Map defaults
remain unsupported. Object/Ref accept only `default={}` to create an absent
active section and apply child defaults. Nonempty object defaults are rejected.

Assignments are separated by `;`. Strings can be bare (`default=` is a
present empty string) or JSON quoted. Quote strings containing separators or
unbalanced brackets: `schemapb:"default=\"a;b\""`. Bytes use base64; enums use
protobuf names. Duration attributes accept Go durations such as `1m30s` or
protobuf duration strings; timestamps use RFC 3339. Repeated fields, maps
and message attributes use protoJSON, for example:

```go
Mode string `json:"mode" validate:"oneof=fast safe" schemapb:"default={\"stringValue\":\"safe\"}"`
Name string `json:"name" schemapb:"rules=[{\"expr\":\"this != ''\",\"message\":\"required\"}]"`
```

Precedence is type inference / `WithType`, existing `json` / `validate` /
`desc` / `pattern` tags, the `schemapb` tag, then `WithFieldTags` callbacks in
option order. Callbacks receive the original `reflect.StructField` and an
owned mutable `*Schema_Field`, including for nested struct fields. Skipped
fields and flattened embedded wrappers do not invoke callbacks. Type
override templates are cloned before decoration.

Project tag conventions can be added without changing this library:

```go
spb.WithFieldTags(func(sf reflect.StructField, field *spb.Schema_Field) error {
    if value, present := sf.Tag.Lookup("default"); present {
        return spb.SetFieldAttribute(field, "default", value)
    }
    return nil
})
```

`SetFieldAttribute` uses the same descriptor-driven conversion as the tag.
Setting one attribute replaces it and preserves sibling attributes; failure
leaves the field unchanged. Defaults apply to absent keys, not explicit
zero/false/empty values. Preserve key presence while merging input layers.

## Resolution and reports

Coercion and normalization apply recursively to scalar and container values
in lists, tuples and maps, including Ref and OneOf. Object and Ref builders
support `DefaultEmpty()`; the equivalent reflection tag is
`schemapb:"default={}"`. Missing sections are created only with this explicit
default. Explicit null is preserved and checked against nullable.

```go
baked, validation, report, err := schema.BakeDetailed(input)
// report.Events carries Path, PathSegments and Operation, without values.
```

BakeDetailed executes normalize once, like Bake, and returns a partial
report even when validation fails. Validation errors also expose
`PathSegments`; map keys containing dots use quoted display paths such as
`tenants["customer.a"].timeout`.

Numeric fields accept `json.Number` without enabling Coerce, including
nested collection entries. Use `json.Decoder.UseNumber()` to retain JSON
integer precision. Integer conversion is exact and range checked; float
parsing rounds directly to the target precision and rejects overflow.

See [the resolution contract](../docs/RESOLUTION.md) for presence, reports,
path escaping and numeric conversion details.

## Decoding typed snapshots

```go
var cfg Config
if err := baked.Decode(&cfg); err != nil {
    return err
}
// A standalone *StructValue has the same Decode method.
```

Decode reads typed protobuf values directly into structs, string-keyed maps
or empty interfaces. It follows Reflect's JSON names and embedded-field
mapping, supports pointers, named scalars, nested lists/maps, fixed arrays,
bytes, `time.Duration`, and `time.Time`. Integers remain exact; numeric
conversions follow the shared `value-as.json` contract. Strings are not
implicitly parsed. Unknown or ambiguous destination fields, wrong kinds,
wrong array lengths, overflow, and precision loss return `*DecodeError`
with a field/index path. Null requires a pointer, map, slice or interface.

The target must be a non-nil pointer. Decode builds a fresh value and replaces
the target only on success, without aliasing wire buffers or existing target
containers. Missing fields become zero; defaults and validation belong to
Resolve/Bake. Nil snapshots are errors. Custom `json.Unmarshaler` and
`encoding.TextUnmarshaler` implementations run on fresh destination values
(text hooks require a wire string). Only explicit JSON hooks/RawMessage use
JSON for their subtree. Native time types are handled before custom hooks.
Decode errors omit source values and custom-hook error payloads.

`List(Ref(...))` resolves each present element before its validation rules:
defaults, inherited coercion, normalization and computed fields apply inside
the referenced schema. Bake preserves declared wire kinds throughout nested
refs, including when re-baking a snapshot.

Resolve and `StructValue.ToGo()` use native Go values: durations are
`time.Duration` and timestamps are `time.Time`. Bake stores typed protobuf
values. An application exposing a plain JSON map should format durations at
its DTO boundary; `encoding/json` encodes a native `time.Duration` as a
number of nanoseconds.

## Development

From the repository root: `make configure` once, then `make lint-go` /
`make test-go`. Regenerate goldens after intentional behaviour changes with
`go test ./schemapb -run Golden -update` (run from `go/`) — every other
language's conformance suite depends on them.

## Masked display values

`baked.Masked()` returns an independent `*StructValue` with present secret
fields replaced by the string `***`. It descends through objects, collections,
Ref and OneOf without rerunning resolution or CEL; the original snapshot stays
intact. Secret containers are hidden as a whole, and inactive secrets are also
masked. Public values retain their wire types and precision.

```go
visible := baked.Masked() // values for instance state or UI cards
```

The result is for display and may no longer conform to the execution schema.
See [the masking contract](../docs/MASKING.md) for edge cases and other languages.
