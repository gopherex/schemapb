# schemapb — cross-implementation code principles

Every implementation (Go, TypeScript, Python, Rust) follows these rules. The
Go implementation is the reference; behaviour is pinned by
`conformance/golden`. A port that cannot satisfy a principle idiomatically
must document the deviation in its README — silent divergence is forbidden.

## 1. Typed identifier domains

Every identifier-like string gets a distinct nominal type where the language
can express one; plain literals must stay ergonomic.

| Domain | Go | Rust | TypeScript | Python |
|---|---|---|---|---|
| Namespace, SchemaName, FieldName, DefName, TemplateName, RuleID, GroupName, VariantKey, Format | defined string types | newtype `struct X(String)` | branded types (`string & { __brand }`) | `NewType(str)` |

A variable of one domain must not silently cross into another. Where the type
system cannot enforce it (Python at runtime), the type checker (mypy/pyright
strict) must.

## 2. Invalid values are unrepresentable

Constructors validate; the constructed value is always valid. `Version` is an
opaque validated semver (zero = unversioned); an identity is built once via
`ID(...)` and reused as a handle — never re-spelled as string triples at call
sites. Where privacy exists (Go unexported field, Rust private field, TS
`#private`/module scope), broken values cannot be constructed at all.

## 3. Two value layers, one conversion point

- **wire** — the generated protobuf types (`Value`, `StructValue`): the
  contract.
- **native** — the language's natural runtime values the engine and CEL
  operate on: Go `any`, TS `NativeValue` union (with `bigint` for 64-bit
  ints), Python natives (`int`, `float`, `bytes`, `datetime`, `timedelta`),
  Rust a `Native` enum.

All conversion lives in exactly one module per implementation (`value.*`),
including canonicalisation: the declared field kind — not the runtime type —
picks the wire variant (an Int64 field's value is always `int64_value`).

## 4. Integers are honest

64-bit integers never pass through a lossy double. TS uses `bigint`, Python
`int`, Rust `i64`/`u64`, Go `int64`/`uint64`. The float64 flattening of the
WASM era is dead; a conformance case with a >2^53 value guards this.

## 5. Errors are data, exceptions are for bugs

Every validation outcome — including warnings — is a `ValidationResult`
value. Exceptions / panics / `Err` are reserved for programmatic failure
(the schema itself does not compile → `SchemaError` carrying the same proto
error shape). No control flow through exceptions inside the engine.

## 6. Explicit compilation, cached sugar

`compile(schema, options)` produces an immutable Engine: every CEL
expression, regex and Mustache template compiled up front; a bad schema
fails at compile time, never at evaluation time. Convenience methods on the
schema go through a cache keyed by schema identity/pointer. Options
(format extensions, cost limits) are compile options, not eval flags.

## 7. Generics against copy-paste

One generic implementation per family: the six numeric kinds share one
constraint checker and one builder (Go type parameters, Rust generics with
an ops trait, TS a generic function over `number | bigint`). Python may rely
on dynamic typing internally but keeps the family in one function. Rule of
thumb: adding a seventh numeric kind must be O(constructor), not O(new
checker).

## 8. Idiomatic surface, identical behaviour

Naming and mechanics follow the language (camelCase TS / snake_case Python /
snake_case Rust; iterators, context managers, `Result`, builders as the
language likes them). Behaviour follows only the spec: evaluation order,
error codes and ordering, canonical forms, display formatting, message
templates (`conformance/golden/messages.json`). No port grows features the
others lack.

## 9. Determinism is spec, not accident

Map iteration is sorted wherever order reaches output; string casing helpers
are ASCII-only; display forms (numbers, durations RFC3339, base64, compact
JSON) follow the spec exactly. If the language's default differs, the
default loses.

## 10. Minimal, spec-compatible dependencies

Per implementation: one proto runtime, one CEL evaluator, one spec-compliant
Mustache renderer, (Go: x/mod/semver). Nothing else — no util/helper
libraries. A CEL/Mustache library that falls short of the spec subset is
patched or vendored; the gap is tracked, never silently absorbed.

## 11. Conformance first

A port starts with the golden runner (`conformance/golden/*`): parse
`full-schema.json`, validate the two canonical inputs, byte-compare
`full-baked.json`, `full-errors.json`, `full-rendered.txt`, render messages
from `messages.json`. A feature does not exist until its golden passes.
Unsupported anything fails loudly (`UNSUPPORTED_FORMAT` doctrine applies to
everything).

## 12. Every implementation has its own linter — pinned, in ./bin

Each language ships a strict lint configuration in its directory (Go:
golangci-lint, TypeScript/Python/Rust: chosen per port) and a `lint-<lang>`
Make target. Linter binaries are NEVER system-global: `make configure`
installs the exact pinned version into `./bin`, same doctrine as the code
generators. CI runs the linters; a suppression always carries a written
reason (`nolint:<linter> // why`).

## 13. Secrets never leak

Masking strips the actual value AND re-renders the message from templates.
Any new error path must prove (test) that a secret field's value appears
nowhere in the result.
