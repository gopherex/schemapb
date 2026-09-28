# Resolution and diagnostics

Resolution operates on values and their field descriptors. Object properties,
list/tuple elements and `Map.value_field` entries share the same pipeline.
`Map.value_schema`, Object, Ref (local or linked identity) and the selected
OneOf variant supply child schemas. Root coercion is inherited through all
container boundaries. No absent list positions or map entries are created.

The phases are defaults/coercion, normalization, dependency-ordered computed
values, then validation and canonicalization. Normalization sees seeded
values; computed expressions see normalized inputs. A container normalizer
may replace its value: the replacement's children receive defaults/coercion
before their own normalization, and computed results target the replacement.
A normalizer is not repeated to produce diagnostics.

Gates are evaluated against the current root before seeding and before
normalizing each node. If normalization of an earlier field activates a
previously skipped node, that node is seeded before its normalization. Gates
are not cached across phases or replaced OneOf variants. Resolution is an
ordered pass, not a fixed-point iteration: dependencies on later normalization
or computed output do not restart earlier nodes. Repeated failures of the same
gate are reported once.

A false `when` gates the whole subtree. Supplied inactive values are retained,
without coercion, normalization or kind-based canonicalization. Collection
positions are not removed or renumbered. Error and report ordering uses schema
field order, ascending list indices and UTF-8 lexical map-key order; computed
operations additionally follow their dependencies. Literal string-key and
integer-index references in CEL participate in dependency ordering.

## Presence and object defaults

Missing, explicit null, and a present empty object are distinct:

| Input | Object/Ref has `default: {}` | Behavior |
| --- | --- | --- |
| Key absent, active | Yes | Create a fresh object and resolve children |
| Key absent, active | No | Keep absent; required may fail |
| Key present as null | Either | Preserve null; check nullable |
| Key present as an object | Either | Resolve supplied children |
| Inactive | Either | Skip the subtree and preserve supplied data |

`required` concerns key presence; `nullable` concerns an explicitly supplied
null. A required nullable field accepts null, but does not accept a missing
key. This applies to scalar and container fields alike.

Object and Ref descriptors accept an optional `StructValue default`. Only an
empty value is valid; nonempty object defaults are schema errors. Defaults on
Refs belong to the reference use site, not the shared definition. The
combination of an object default with `immutable` is rejected: materializing
an absent section is not a fixed whole-object value. Each application creates
independent mutable data. Resolving an already present section does not
replace it with another empty object.

Go builders expose `Object(...).DefaultEmpty()` and `Ref(...).DefaultEmpty()`;
reflection accepts `schemapb:"default={}"` and applies it implicitly to
value (non-pointer) struct fields unless they carry `validate:"required"` or
are immutable. TypeScript builders expose
`defaultEmpty()`, Python builders `default_empty()`. Rust uses the generated
Object/Ref descriptor's `default: Some(StructValue::default())`.

## Paths

`ValidationError.path_segments` and `ResolveEvent.path_segments` contain
`PathSegment` messages with exactly one of `key` or `index`. Numeric-looking
map keys are keys, never indices. An empty segment array denotes the root.
The `path` string is retained for display:

```text
db.value
servers[2].timeout
tenants["customer.a"].timeout
```

Dots, brackets, quotes, backslashes and empty keys use JSON-quoted bracket
notation. The engine escapes keys at each descent, so diagnostics never try
to recover structure from an ambiguous concatenation of raw keys. Simple
existing paths retain their spelling. CEL error wording remains informational;
paths, segment types and error codes are the machine-readable contract.

## Bake reports

`BakeDetailed` (Go), `bakeDetailed` (TypeScript), and `bake_detailed`
(Python/Rust) execute the same pipeline as ordinary Bake, collecting a
`ResolveReport`. Go returns `(baked, validation, report, error)`; the other
ports expose `report` on the bake outcome. Ordinary Bake does not collect
report events. Reports are local to the invocation, including concurrent
calls using the same compiled engine.

| Operation | Meaning |
| --- | --- |
| `DEFAULT_APPLIED` | An absent value was supplied, or a differing immutable scalar was restored |
| `COERCED` | A string was converted to the field's native type |
| `NORMALIZED` | A successful normalize expression changed the value |
| `COMPUTED` | A computed result was evaluated and written |
| `INACTIVE` | The node's gate skipped processing in a phase; emitted at most once per path, with no descendant events during that skip |

Events contain only paths and operations, never source/result values or
expression text, including for secret fields. Unchanged normalizations do
not emit events. A failed bake returns errors and the operations already
performed, without a successful Baked snapshot. Reports describe schema
operations; external source merging and provenance remain the caller's job.

## Go JSON numbers

Native numeric inputs may be `encoding/json.Number`, for example from
`json.Decoder.UseNumber()`. Resolve, Validate, Bake and CanonicalValue accept
them recursively in numeric fields independently of string coercion.

Integers are parsed exactly: the mathematical value must be integral and fit
the declared signed/unsigned range. `1.5e1` and `1000.0` are valid integers;
`15e-1` is not. No intermediate float is used. Very large exponents are
checked without allocating their expanded decimal representation.

Float32/float64 use round-to-nearest, ties-to-even at the target precision;
float32 is parsed directly at 32-bit precision. Overflow to infinity and
invalid JSON-number syntax fail. Ordinary rounding, including underflow to
signed zero, is permitted. Conversion of an already numeric JSON token does
not emit a string-coercion event. This adapter does not change the lossless
contract for extracting already-typed wire values with Decode/As.
