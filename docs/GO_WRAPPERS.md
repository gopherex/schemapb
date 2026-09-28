# Go type wrappers and field annotations

Schemapb supports application-owned types without importing their runtime
packages. A wrapper can describe its wire type, decorate its schema field,
and expose private storage to the typed decoder. Schemapb does not provide
Live/Secret types, subscriptions, instance bindings or reload publication.

## Annotations

`Schema.Field.annotations` is a `map<string, Value>`. Applications should use
namespaced keys, for example `backplate.live` with `BoolV(true)`. Keys have no
reserved engine meanings. Unknown keys, false, null, containers and precise
numeric wire kinds survive JSON/binary round trips and the embedded schema
of a Baked snapshot in Go, TypeScript, Python and Rust.

Annotations do not change resolution, validation, computed expressions or
masking. In particular, an annotation named `secret` does not set the field's
`secret` flag. Annotations are schema metadata, not instance values; Masked
returns only masked values and does not sanitize schema metadata.

Go's existing descriptor-driven attribute API supports annotations without
special tag parsing:

```go
Count int32 `json:"count" schemapb:"annotations={\"backplate.live\":{\"boolValue\":true}}"`
```

A type hook can set `field.Annotations` directly. Other languages use the
same generated field map; their schema engines treat it as opaque data.

## Reflect interfaces

```go
type SchemaWrapper interface {
    SchemaInner() reflect.Type
}

type SchemaFieldConfigurer interface {
    SchemaField(field *Schema_Field) error
}
```

The interfaces are independent. `SchemaWrapper` supplies the **immediate**
inner type; wrappers can nest. A type that only implements
`SchemaFieldConfigurer` retains normal type inference and decorates its field.
Both value and pointer receiver methods are recognized on fresh zero
instances. Hooks must be deterministic, independent of live instance state,
and must not retain the mutable descriptor or perform external side effects.
Nil inner types, wrapper cycles and excessively deep wrapper chains fail.

For each field, including collection elements, the order is:

1. Unwrap types and infer the inner kind, including native time/bytes handling.
2. Apply legacy tags, the `schemapb` tag, and `WithFieldTags` handlers.
3. Apply type decorators from the innermost type to the outermost wrapper.
4. Check the completed schema with the ordinary descriptor validator.

An exact `WithType` override stops unwrapping at that type and bypasses its
hooks; any already encountered outer wrapper decorators still apply. The
returned override descriptor is cloned. Overrides affect reflection only,
not the storage type accepted by Decode.

Decorators can enforce a type invariant after tags, for example always setting
`Secret = true`. The library does not recognize type names or special-case
Live/Secret. A decorator may reject a final field configuration with an error.

List items and map values use the same type handling. The bytes shortcut
for slices/arrays of uint8-like types applies only when the element has no
schema hooks; annotated/wrapped elements use a List to retain their metadata.
Struct-valued map
entries with hooks use `value_field` so their field-level metadata is retained.
Normal embedded structs still flatten, and explicit JSON names keep nested
objects. Go method promotion also applies to these interfaces: embedding a
hook-bearing type can make the containing type implement the hook. Prefer a
named field unless that forwarding is intentional.

Like an overridden/scalar root, a root type with schema hooks is represented
by a single `value` field. Its annotations belong to that field. Decode does
not implicitly remove this envelope; normally put wrappers in named fields
of the application config.

Pointer layers retain optional/nullable behavior, including pointers inside
wrappers. `omitempty` and explicit `required` tags retain their existing role.
A recursive ordinary struct keeps the existing JSON fallback; that is distinct
from an invalid cycle consisting solely of wrapper declarations.

## Native decoding hook

```go
type SchemaDecodeTarget interface {
    SchemaDecodeTarget() any
}
```

The receiver must also implement `SchemaWrapper`. Return a non-nil pointer
whose element type is exactly `SchemaInner()`. The decoder feeds the original
wire value into that destination using its existing type checks. It does not
convert through JSON, revalidate, apply defaults or execute CEL. Existing
JSON/Text unmarshaler hooks remain a fallback for types without this native
hook. Native time and numeric behavior remain unchanged.

A minimal application wrapper can implement it as follows (this is storage,
not an implementation of a live runtime):

```go
type Wrapped[T any] struct {
    value T
}

func (Wrapped[T]) SchemaInner() reflect.Type {
    return reflect.TypeFor[T]()
}

func (w *Wrapped[T]) SchemaDecodeTarget() any {
    return &w.value
}

func (*Wrapped[T]) SchemaField(f *schemapb.Schema_Field) error {
    if f.Annotations == nil {
        f.Annotations = make(map[string]*schemapb.Value)
    }
    f.Annotations["example.marker"] = schemapb.BoolV(true)
    return nil
}
```

For an inner pointer `*T`, the target is `**T`. Decode runs on a fresh wrapper,
so allocating a private cell inside the receiver is also allowed. The storage
must belong to that receiver: do not expose an already published live instance,
register subscriptions, invoke updates or write shared/global state. External
side effects cannot be rolled back by the decoder.

| Destination | Explicit null |
| --- | --- |
| `Wrapped[string]` | Error |
| `Wrapped[*string]` | Wrapper with an inner nil pointer |
| `*Wrapped[string]` | Outer nil pointer; hook is not called |

An absent field does not invoke its hook and becomes zero, matching ordinary
Decode. The full destination is replaced only after successful decoding;
errors keep the existing destination intact and report the original field,
map-key or list-index path without including source values. Nested wrapper
chains count toward the decoder's existing nesting limit.
