# schemapb

[![CI](https://github.com/stroppy-io/schemapb/actions/workflows/ci.yml/badge.svg)](https://github.com/stroppy-io/schemapb/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/stroppy-io/schemapb/schemapb.svg)](https://pkg.go.dev/github.com/stroppy-io/schemapb/schemapb)
[![Go Report Card](https://goreportcard.com/badge/github.com/stroppy-io/schemapb)](https://goreportcard.com/report/github.com/stroppy-io/schemapb)
![Go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)
![npm](https://img.shields.io/badge/npm-%40stroppy--io%2Fschemapb-CB3837?logo=npm&logoColor=white)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A **runtime, proto-defined form/config schema descriptor** with a validation and derived-value computation engine. The **same [expr-lang](https://github.com/expr-lang/expr) engine** runs on the Go server and in the browser via WebAssembly, so server and client agree on validation rules, computed fields, and defaults — no reimplementation, no drift.

Typical use cases: `postgresql.conf`-style configuration UIs, dynamic form generation, cross-platform data validation. Build the schema once with the fluent Go API; ship it to the browser as a protobuf message; the browser validates and computes live using the embedded WASM engine.

## Contents

1. [Install](#install)
2. [Quickstart (Go)](#quickstart-go)
3. [Field kinds & constraints](#field-kinds--constraints)
4. [Validation model](#validation-model)
5. [Computed / resolve / Bake](#computed--resolve--bake)
6. [Composition — OneOf and Ref/$defs](#composition--oneof-and-refdefs)
7. [Dynamic fields — when / options_expr / count_expr](#dynamic-fields--when--options_expr--count_expr)
8. [Code generation — typed Go structs](#code-generation--typed-go-structs)
9. [TypeScript / browser](#typescript--browser)
10. [gRPC SchemaService](#grpc-schemaservice)
11. [Development](#development)

---

## Install

### Go

```sh
go get github.com/stroppy-io/schemapb/schemapb
```

### Code generator CLI (`schemapbgen`)

Download a prebuilt binary for your OS/arch from the [latest release](https://github.com/stroppy-io/schemapb/releases), or install from a clone (the repo's `go.work` makes the CLI build against the in-tree library):

```sh
git clone https://github.com/stroppy-io/schemapb
cd schemapb
go install ./cmd/schemapbgen     # installs `schemapbgen` to $GOBIN
# or run without installing:
go run ./cmd/schemapbgen -in schema.json -out config_gen.go -pkg myconfig
```

Standalone `go install <module>/cmd/schemapbgen@latest` is **not** supported: the `@version` form ignores the workspace and pins the library to an older release tag, so the build fails. Generates typed Go structs from schemas — see [Code generation](#code-generation--typed-go-structs).

### TypeScript / npm

The package is published to GitHub Packages. Configure the scope in `.npmrc`:

```
@stroppy-io:registry=https://npm.pkg.github.com
```

Then install:

```sh
npm install @stroppy-io/schemapb
```

The package ships compiled WebAssembly (`schemapb.wasm`) and the Go runtime loader (`wasm_exec.js`).

---

## Quickstart (Go)

```go
package main

import (
	"fmt"

	"github.com/stroppy-io/schemapb/schemapb"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	// Build a schema with the fluent chain API.
	// NewSchema(namespace, name, version)
	s := schemapb.NewSchema("infra", "postgres", "v1").
		Descr("PostgreSQL instance configuration").
		Strict(). // reject unknown keys
		Fields(
			schemapb.Enum("wal_level").
				Values(map[int32]string{0: "minimal", 1: "replica", 2: "logical"}).
				Default(1).
				DefinedOnly().
				Group("WAL").
				Title("WAL level"),

			schemapb.Int64("shared_buffers").
				Gte(16).
				Lte(65536).
				Default(128).
				Unit("MB").
				Group("Resource Usage").
				Desc("Size of shared memory buffers"),

			schemapb.Int64("max_connections").
				Gte(1).
				Lte(10000).
				Default(100).
				Group("Resource Usage"),

			// Immutable: always reverts to its default; changing it is rejected.
			schemapb.Str("cluster_id").
				Default("auto").
				Immutable().
				Secret().
				Group("Identity"),

			// Cross-field rule: warns when the memory budget is tight.
			schemapb.Int64("work_mem").
				Gte(1).
				Default(4).
				Unit("MB").
				Group("Resource Usage").
				Rules(
					schemapb.Rule(
						"root.work_mem * root.max_connections <= root.shared_buffers * 4",
						"work_mem * max_connections may exceed shared_buffers budget",
					).ID("mem_budget").Warn(),
				),

			// Computed: derived from inputs in dependency order, read-only.
			schemapb.Computed("effective_cache", "root.shared_buffers * 3").
				Result(schemapb.ResultInt64).
				Unit("MB").
				Group("Resource Usage").
				Desc("Estimated effective cache size"),
		).
		MustBuild()

	values := map[string]any{
		"wal_level":       1.0,
		"shared_buffers":  256.0,
		"max_connections": 200.0,
		"work_mem":        8.0,
	}

	// Validate: structured checks + expr rules. Empty result = valid.
	errs := s.Validate(values)
	for _, e := range errs {
		fmt.Printf("[%s] %s: %s\n", e.Severity, e.Field, e.Message)
	}

	// Compute: fill defaults + evaluate Computed fields in dependency order.
	resolved, errs := s.Compute(values)
	fmt.Println("effective_cache:", resolved["effective_cache"]) // 768

	// Bake: validate + resolve, then seal into a self-contained snapshot.
	baked, errs := s.Bake(values)
	if len(errs) == 0 {
		fmt.Printf("baked schema hash: %x\n", schemapb.Hash(baked.GetSchema()))
	}

	// Merge: layer partial overrides onto a Baked and re-seal.
	overrides, _ := structpb.NewStruct(map[string]any{"work_mem": 16.0})
	merged, errs := baked.Merge(overrides, false /* replaceLists=append */)
	_ = merged
}
```

---

## Field kinds & constraints

Every field has exactly one *kind*. Kind constructors take the field name and return a typed builder; constraints are kind-specific methods on that builder. Common field metadata methods are available on every kind via the shared `fieldBase`.

### Kind constructors

| Constructor | Go type | Notes |
|---|---|---|
| `Float(name)` | float32 | |
| `Double(name)` | float64 | |
| `Int32(name)` | int32 | |
| `Int64(name)` | int64 | |
| `UInt32(name)` | uint32 | |
| `UInt64(name)` | uint64 | |
| `Bool(name)` | bool | |
| `Str(name)` | string | |
| `Enum(name)` | int32 with labels | |
| `Duration(name)` | Go duration string | e.g. `"1h30m"` |
| `Timestamp(name)` | RFC3339 string | |
| `List(name, items...)` | array | items = element field def |
| `Object(name, fields...)` | nested object | inline sub-schema |
| `Computed(name, expr)` | derived value | read-only, evaluated from `root` |
| `OneOf(name, discriminator)` | discriminated union | `.Variant(key, fields...)` / `.VariantOf(key, *Schema)` |
| `Ref(name, defName)` | reference to local `$defs` | validates object against named def |
| `RefID(name, *SchemaIdentity)` | reference by identity | external; resolved via `Link` (see Composition) |
| `ObjectOf(name, *Schema)` | embed a built schema | inline object (clone) |

### Per-kind constraint methods

**Numeric** (`Float`, `Double`, `Int32`, `Int64`, `UInt32`, `UInt64`):

```go
.Gt(v)         // exclusive minimum: value > v
.Gte(v)        // inclusive minimum: value >= v
.Lt(v)         // exclusive maximum
.Lte(v)        // inclusive maximum
.In(v...)      // value must be one of
.NotIn(v...)   // value must not be one of
.MultipleOf(v) // divisibility constraint
.Const(v)      // value must equal exactly v
.Default(v)    // seeded when unset
```

**Bool**: `.Default(v bool)`, `.Const(v bool)`

**String**:

```go
.MinLen(n)       // minimum character count (Unicode runes)
.MaxLen(n)       // maximum character count
.Len(n)          // exact character count
.Pattern(re)     // RE2 regular expression (compiled and cached)
.In(v...)        // allowlist
.NotIn(v...)     // denylist
.Const(v)        // must equal exactly
.Default(v)      // seeded when unset
.Format(f)       // semantic format (see below)
```

String format constants (use with `.Format(...)`):

| Constant | Validates |
|---|---|
| `FormatEmail` | RFC 5322 email address |
| `FormatURL` | parseable request URI |
| `FormatUUID` | canonical UUID (case-insensitive) |
| `FormatIPv4` | IPv4 address |
| `FormatIPv6` | IPv6 address (not 4-in-6) |
| `FormatIP` | any IP address |
| `FormatHostname` | RFC 1123 hostname |
| `FormatDate` | `2006-01-02` |
| `FormatTime` | `15:04:05` |
| `FormatDatetime` | RFC3339 |

**Enum**:

```go
.Values(map[int32]string{...}) // integer → human label mapping
.DefinedOnly()                 // value must be a key in Values
.In(v...)                      // allowlist of integer values
.NotIn(v...)                   // denylist
.Default(v int32)
.Options(expr)                 // dynamic allowed set over `root` (see Dynamic fields)
```

**Duration** and **Timestamp**: `.Default(...)`, `.Gt(...)`, `.Gte(...)`, `.Lt(...)`, `.Lte(...)`
- `Duration` accepts `time.Duration`; values in the form are Go duration strings (`"1h30m"`).
- `Timestamp` accepts `time.Time`; values in the form are RFC3339 strings.

**List**:

```go
List("tags", Str("tag")).
    MinItems(1).
    MaxItems(10).
    Unique().
    Count(expr)   // dynamic exact length over `root` (see Dynamic fields)
```

**Computed**:

```go
Computed("total", "root.price * root.qty").Result(schemapb.ResultDouble)
```

Result type constants: `ResultDouble`, `ResultInt64`, `ResultUint64`, `ResultBool`, `ResultString`, `ResultDuration`.

### Common field metadata (every kind)

```go
.Required()          // value must be present (missing → error code "required")
.Nullable()          // nil/null is allowed even when required
.Immutable()         // system-fixed: forced to Default, change rejected
.Group("WAL")        // UI grouping label (informative only)
.Unit("MB")          // unit of measurement label (informative only)
.Title("Buffer size")// human title (informative only)
.Desc("...")         // human description (informative only)
.Deprecated()        // flag for renderers (informative only)
.Secret()            // sensitive value, e.g. password (informative only)
.Examples(vals...)   // example values (*structpb.Value; informative only)
.Normalize(expr)     // transform the value before validation (see below)
.When(expr)          // conditional gate: field is active only if expr is true (see Dynamic fields)
.Rules(rules...)     // attach cross-field rules to this field
```

### Schema-level builder chain

```go
schemapb.NewSchema(namespace, name, version string) *SchemaB
    .Descr(d string) *SchemaB
    .Strict() *SchemaB          // reject unknown keys (code "unknown_field")
    .MinProps(n uint64) *SchemaB
    .MaxProps(n uint64) *SchemaB
    .Coerce() *SchemaB          // coerce string inputs to the field's kind
    .Def(name string, fields ...FieldDef) *SchemaB  // register a reusable def
    .Fields(defs ...FieldDef) *SchemaB
    .Rules(rules ...RuleDef) *SchemaB
    .Build() (*Schema, error)
    .MustBuild() *Schema        // panics on a malformed schema
```

### Rule builder

```go
schemapb.Rule(expr, msg string) *RuleB
    .ID(id string) *RuleB
    .Warn() *RuleB                        // severity WARNING (does not block)
    .Severity(s Schema_Filed_Severity) *RuleB
```

Severity constants: `SeverityError`, `SeverityWarning`, `SeverityUnspecified`.

### Utility

```go
schemapb.Ptr[T any](v T) *T  // returns a pointer to v (useful for optional proto fields)
```

---

## Validation model

Validation runs in two passes after seeding defaults:

### 1. Structured constraints

Per-field, kind-specific checks (`Gte`, `Pattern`, `MinLen`, `DefinedOnly`, etc.). These run before expr rules and are introspectable by UI renderers (e.g. derive slider bounds from `Gte`/`Lte`).

### 2. Expr-lang rules

CEL boolean expressions attached to fields or to the schema itself. The engine binds:
- `this` — the field's current value (field-level rules only)
- `root` — the fully resolved form (all inputs + defaults + Computed values)

`true` = valid, `false` = invalid.

```go
// Field-level rule: `this` is the field value.
schemapb.Int64("port").
    Rules(schemapb.Rule("this >= 1 && this <= 65535", "must be a valid port").ID("port_range"))

// Schema-level cross-field rule: `root` only.
schemapb.NewSchema("app", "config", "v1").
    Rules(
        schemapb.Rule(
            "root.work_mem * root.max_connections <= 4096",
            "memory budget exceeded",
        ).ID("mem_budget"),
    )
```

### `Schema.IsValid` — descriptor self-check

`IsValid() []*FieldError` validates the schema descriptor itself: field names present and unique, exactly one kind set, expressions compilable, patterns valid, no Computed cycles, all Ref names resolve. Called automatically by `Build()` / `MustBuild()`.

### Entry points

```go
s.Validate(values map[string]any) []*FieldError
s.ValidateStruct(st *structpb.Struct) []*FieldError
s.ValidateJSON(raw json.RawMessage) ([]*FieldError, error)
```

Compiled programs (rules + Computed expressions) are cached by schema content hash (`schemapb.Hash(s)`) — no per-call compilation overhead after the first call.

### FieldError

```go
type FieldError struct {
    Field    string                 // dotted path, e.g. "disk.size" or "tags[0]"
    Message  string                 // human-readable failure message
    Severity Schema_Filed_Severity  // ERROR (blocks submit) or WARNING
    Code     string                 // stable machine code, e.g. "gte", "pattern", "required", "rule"
    Params   map[string]string      // template params for client i18n, e.g. {"gte": "16"}
    RuleId   *string                // set when a named Rule failed
}
```

`Code` + `Params` enable client-side message localisation without reparsing the human `Message`. Common codes: `required`, `not_null`, `type`, `const`, `gt`, `gte`, `lt`, `lte`, `in`, `not_in`, `multiple_of`, `min_len`, `max_len`, `len`, `pattern`, `format`, `min_items`, `max_items`, `unique`, `min_properties`, `max_properties`, `unknown_field`, `immutable`, `enum_defined`, `enum_in`, `enum_not_in`, `rule`, `ref`.

`hasBlockingError` (used internally) treats `SeverityWarning` as non-blocking; anything else (including `SEVERITY_UNSPECIFIED`) blocks.

---

## Computed / resolve / Bake

### Resolve pipeline

When `Compute` or `Validate` runs, the following happens in order:

1. **Coerce** — if `schema.Coerce` is set, scalar string inputs are parsed to the field's kind (numbers, booleans, enum integers).
2. **Seed defaults** — unset fields receive their `Default` value. Immutable fields are *forced* to their default regardless of what was submitted.
3. **Normalize** — each field's `Normalize` expression (if set) is evaluated with `this = currentValue, root = wholeForm`; the result replaces the field value.
4. **Compute** — Computed fields are evaluated in dependency order (topological sort of `root.*` references). A cycle is detected statically by `IsValid` and at runtime surfaces as a `FieldError`.

### Compute entry points

```go
s.Compute(values map[string]any) (map[string]any, []*FieldError)
s.ComputeStruct(st *structpb.Struct) (map[string]any, []*FieldError)
s.ComputeJSON(raw json.RawMessage) (map[string]any, []*FieldError, error)
```

The returned map is the full resolved form: original inputs + seeded defaults + evaluated Computed fields.

### Filled vs. Baked

**`Filled`** is a mutable input: a `SchemaRef` (by identity or inline) plus form values. It represents "what the user is currently editing."

**`Baked`** is a sealed, self-contained snapshot: an embedded `Schema` plus the final resolved values. It carries the authority of the schema at the moment of sealing and is content-hashable.

```go
// Bake: validate + resolve, seal into a Baked.
// Warnings do not block; a Baked is returned alongside any warnings.
baked, errs := s.Bake(values map[string]any) (*Baked, []*FieldError)

// Merge: deep-merge overrides onto a Baked and re-seal.
// Objects merge recursively; lists append (or replace when replaceLists=true).
// Immutable fields keep their baked value; a changed immutable is rejected.
merged, errs := baked.Merge(overrides *structpb.Struct, replaceLists bool) (*Baked, []*FieldError)

// Matches: compare by content hash.
baked.Matches(s *Schema) bool

// Hash: SHA-256 of any message via its generated HashPB method.
schemapb.Hash(s *Schema) [32]byte
```

### Filled convenience methods

```go
// Bake a Filled that carries an inline schema.
// Returns an error if the Filled references a schema by id (id refs resolve server-side).
baked, errs, err := filled.Bake() (*Baked, []*FieldError, error)

// IntoBaked copies values into a Baked WITHOUT validating or resolving.
// WARNING — UNSAFE ESCAPE HATCH. Skips defaults, Computed evaluation,
// rules, and immutable enforcement. Only use when re-wrapping values that
// were already baked and validated elsewhere.
baked, err := filled.IntoBaked() (*Baked, error)
```

---

## Composition — OneOf and Ref/$defs

### OneOf (discriminated union)

`OneOf` validates the value as an object; the discriminator property selects which variant schema to apply.

```go
schemapb.NewSchema("app", "storage", "v1").
    Fields(
        schemapb.OneOf("backend", "type").
            Variant("s3",
                schemapb.Str("bucket").Required(),
                schemapb.Str("region").Default("us-east-1"),
            ).
            Variant("gcs",
                schemapb.Str("bucket").Required(),
                schemapb.Str("project").Required(),
            ),
    ).
    MustBuild()

// Valid value: {"backend": {"type": "s3", "bucket": "my-bucket", "region": "eu-west-1"}}
```

The discriminator field must be a non-empty string. An unknown variant key produces a `FieldError` with code `"oneof_variant"`.

### Ref and `$defs` (reusable / recursive schemas)

Named sub-schemas registered with `.Def(name, fields...)` can be referenced by any `Ref` field in the same root schema. Refs enable both reuse and recursive data structures (a def may contain a `Ref` back to itself; recursion terminates because it follows the actual data).

```go
schemapb.NewSchema("app", "tree", "v1").
    Def("node",
        schemapb.Str("label").Required(),
        // Recursive: a node may have child nodes.
        schemapb.List("children", schemapb.Ref("child", "node")),
    ).
    Fields(
        schemapb.Ref("root_node", "node"),
    ).
    MustBuild()
```

`IsValid` checks that every local `Ref.name` resolves to a key in `schema.defs`. An unresolved ref produces a schema error at build time.

### Embedding an already-built `*Schema`

To reuse a separately-built `*Schema` (instead of re-declaring its fields inline) there are two models.

**By value (inline)** — the embedded schema's fields are *cloned* into the host. Its identity is not used (validation is structural); its own `$defs` are hoisted into the root at `Build()` so internal refs still resolve. Self-contained, validates offline, good for snapshots / `Baked`.

```go
db := schemapb.NewSchema("infra", "db", "v1").Fields(
    schemapb.Str("host").Required(),
    schemapb.Int32("port").Gte(1).Lte(65535).Default(5432),
).MustBuild()

schemapb.NewSchema("app", "cfg", "v1").
    DefSchema("db", db).                          // register a built schema as a def
    Fields(
        schemapb.ObjectOf("primary", db),         // embed as a nested object
        schemapb.Ref("replica", "db"),            // or reference the registered def
        schemapb.OneOf("backend", "kind").
            VariantOf("db", db).                  // a built schema as a oneof variant
            Variant("cache", schemapb.Int32("ttl").Required()),
    ).
    MustBuild()
```

| Builder | Embeds a `*Schema` as |
|---|---|
| `ObjectOf(name, s)` | a nested object field |
| `(*SchemaB).DefSchema(name, s)` | a named def (pair with `Ref`) |
| `(*OneOfB).VariantOf(key, s)` | a oneof variant |

All three clone the source — later mutation of the original never leaks into the composite.

**By identity (reference)** — a `Ref` carries the *target's `SchemaIdentity`* instead of a copy. The identity is **preserved on the node**, so a renderer can see exactly which schema (and version) a branch points at — useful for lazy-loading, linking, and version-aware UIs. The referenced schema isn't copied, so a source update propagates.

```go
dbID := &schemapb.SchemaIdentity{Namespace: "infra", Name: "db", Version: "v1"}

cfg := schemapb.NewSchema("app", "cfg", "v1").Fields(
    schemapb.RefID("primary", dbID).Required(),         // refers to infra/db/v1 by identity
    // or: schemapb.RefIdentity("primary", "infra", "db", "v1")
).MustBuild()
```

An identity-ref is **external**: it does not have to resolve at build time. Before validation the target must be pulled in with `Link`, which resolves every identity-ref (transitively) against a `Registry` and folds the referenced schemas into the root defs — keeping the identity on the node, but making the schema self-contained so it validates standalone:

```go
reg := schemapb.NewInMemoryRegistry()
_ = reg.Put(ctx, db)                 // db has identity infra/db/v1

linked, err := cfg.Link(ctx, reg)    // resolves all id-refs; returns a clone
// linked validates standalone; linked's id-ref nodes still report their identity.
```

`Registry` is the same interface the gRPC `SchemaService` uses, so the core needs no extra resolver. An unlinked identity-ref that reaches validation produces a `FieldError` with code `"ref"` (message hints to call `Link`). Typical flow: the **server** links and ships the self-contained schema; the **browser** receives it and validates with the embedded WASM engine (no registry needed client-side).

| Mode | Builder | Identity kept | Needs registry | Self-contained |
|---|---|---|---|---|
| inline (value) | `ObjectOf` / `DefSchema` / `VariantOf` | no | no | yes |
| reference (identity) | `RefID` / `RefIdentity` + `Link` | **yes** | to `Link` | after `Link` |

---

## Dynamic fields — when / options_expr / count_expr

Three field-level expressions make a field's shape depend on the rest of the form (`root`). Each is an expr-lang expression evaluated over `root` (the whole form as `map<string, dyn>`); they compile at `Build()` and run during validation/compute.

### `When(expr)` — conditional gate

`When` is a boolean expression. When it is **false** the field is *inactive*: the validator skips it entirely (no required/nullable, no rules, no kind constraints, no Computed/normalize) and treats it as **absent** regardless of any value present. For a container kind (Object/OneOf/List/Ref) the whole subtree is gated. Inactive fields do not count toward `min/max_properties` and their key never trips `Strict` "unknown_field". The value is not deleted, so it reappears if the field becomes active again. `this` is **not** bound (a field's own value must not gate its existence). Empty/absent ⇒ always active; a non-bool result is a runtime error (code `when`).

```go
schemapb.Bool("tls"),
schemapb.Str("tls_cert").Required().When("root.tls == true"), // required only when tls is on
```

Renderers decide visibility with `Schema.FieldActive(name, root) (bool, error)` (TS: `sp.fieldActive`).

### `Options(expr)` on Enum — dynamic allowed set

An expr returning a list of allowed integer values. When set it **replaces** the static allowed set (`Values`/`In`/`NotIn`/`DefinedOnly`) for validation and supplies the option list to renderers. A submitted value not in the result fails with code `enum_not_allowed`. Empty/absent ⇒ use the static values; a non-list result is a runtime error.

```go
schemapb.Enum("version").
    Values(map[int32]string{13: "13", 14: "14", 15: "15", 16: "16"}).
    Options(`root.edition == "lts" ? [14, 16] : [15]`)
```

Renderers fetch options with `Schema.EnumOptions(name, root) ([]int32, error)` (falls back to the static keys when no `Options`; TS: `sp.enumOptions`).

### `Count(expr)` on List — dynamic exact length

An expr returning a non-negative integer: the exact number of items the list must have. The length must equal the result, else code `list_count_mismatch`. Empty/absent ⇒ length bounded only by `MinItems`/`MaxItems`; a non-int or negative result is a runtime error. Each item is still validated by the item schema, and inside an item's rules the item's zero-based position is bound as `index`.

```go
schemapb.Int32("replicas").Gte(0),
schemapb.List("machines", schemapb.Str("host")).Count("root.replicas + 1"),
```

Renderers fetch the count with `Schema.ListCount(name, root) (int64, error)` (TS: `sp.listCount`).

### Conditional presence sugar (schema-level)

`When` gates a field's **existence** (inactive ⇒ hidden, value ignored). When a field should stay active/visible but its **presence** is conditional, use these schema-level helpers — they emit a form-wide rule (so they fire even when the field is absent) and report the error on `field`:

```go
schemapb.NewSchema("t","s","v1").Fields(
    schemapb.Bool("tls"),
    schemapb.Str("cert"),               // NOT .Required()
).
    RequiredWhen("cert", "root.tls == true").       // required only when tls on
    RequiredUnless("user", "root.anon == true").    // required unless anon
    ForbiddenWhen("manual_host", "root.managed == true") // must be ABSENT when managed (hard error)
```

| Helper | Meaning |
|---|---|
| `RequiredWhen(field, cond)` | `field` required iff `cond` true |
| `RequiredUnless(field, cond)` | `field` required unless `cond` true |
| `ForbiddenWhen(field, cond)` | `field` must be absent when `cond` true (vs `When`, which silently ignores) |

Presence is tested with `"field" in root`, so these target top-level fields by name. They are pure sugar over `Rules(...)`, so the behavior ships through WASM to TS unchanged (TS users write the equivalent rule directly).

---

## Code generation — typed Go structs

`schemapbgen` reads a schema and emits a typed Go struct mirror of the form's
data shape, with full roundtrip/validation/sugar and no loss of dynamic rules.
Code against `cfg.SharedBuffers` instead of `map[string]any`.

```sh
# from a protojson schema file
schemapbgen -in disk.json -out disk_gen.go -pkg myconfig

# or from a Go builder provider, via go:generate
//go:generate go run github.com/stroppy-io/schemapb/cmd/schemapbgen -from-go-code . -symbol BuildDiskSchema -pkg myconfig
```

Generated per schema: identity-named structs (protobuf-style `_`-nested), enums
with `String()`, OneOf as interface + variants, `ToValues`/`FromValues`,
`ToFilled`/`ToBaked`, `Validate()` (through the embedded schema), `Default()`,
nil-safe getters, builders, and `Clone()`. The struct layer stays simple; dynamic
logic (`when`/`expr` rules/computed) is preserved as comments plus the embedded
schema wire bytes and runs through the schemapb engine at runtime.

Full docs, naming scheme, and type mapping: **[`cmd/schemapbgen/README.md`](cmd/schemapbgen/README.md)**.

---

## TypeScript / browser

The package exposes a thin TypeScript wrapper around the WASM-compiled Go engine. The schema is expressed using **protobuf-es** `create(SchemaSchema, ...)` and passed to the `Schemapb` instance.

```typescript
import { schemapb, SchemaSchema } from "@stroppy-io/schemapb";
import { create } from "@bufbuild/protobuf";
import type { Schema } from "@stroppy-io/schemapb";

// Zero-config: auto-loads the bundled schemapb.wasm (readFile on Node, fetch in
// the browser) and runs wasm_exec.js for you. Cached, so call it anywhere.
const sp = await schemapb();

// Advanced: supply the module yourself (custom path / CDN / cache):
//   import { Schemapb } from "@stroppy-io/schemapb";
//   const sp = await Schemapb.load(await fetch(wasmUrl).then(r => r.arrayBuffer()));

// Build a schema using protobuf-es (matches the Go proto definition exactly).
const schema: Schema = create(SchemaSchema, {
  id: { namespace: "config", name: "app", version: "v1" },
  description: "Application configuration",
  strict: true,
  fields: [
    {
      name: "debug",
      kind: { case: "bool", value: { default: false } },
    },
    {
      name: "max_connections",
      kind: { case: "int64", value: { default: 100n, gte: 1n } },
    },
    {
      name: "effective_timeout",
      kind: {
        case: "computed",
        value: { expr: "root.max_connections * 10" },
      },
    },
  ],
});

// Validate: returns { ok: boolean; errors: FieldErrorJson[] }
const validation = sp.validate(schema, { debug: true, max_connections: 500 });
if (!validation.ok) {
  console.error("Errors:", validation.errors);
}

// Compute: seeds defaults + evaluates Computed fields.
// Returns { values: Record<string, unknown>; errors: FieldErrorJson[] }
const computed = sp.compute(schema, { max_connections: 200 });
console.log(computed.values); // { debug: false, max_connections: 200, effective_timeout: 2000 }

// Bake: validate + resolve + seal.
// Returns { baked?: BakedJson; errors: FieldErrorJson[] }
const result = sp.bake(schema, { max_connections: 50 });
if (result.baked) {
  console.log("Sealed values:", result.baked.values);
}

// Merge: layer overrides onto a Baked and re-seal.
// replaceLists=false → lists append; true → lists replace.
const merged = sp.merge(result.baked!, { max_connections: 300 });
console.log("Merged:", merged.baked?.values);

// Link: resolve identity-Refs against a pool of built schemas (mirrors Go's
// Schema.Link). Returns a self-contained Schema; id-ref nodes keep their identity.
const linked = sp.link(cfgSchema, [dbSchema]);
const ok = sp.validate(linked, { primary: { host: "h" } }).ok;
```

The TS wrapper also exposes the renderer helpers `sp.fieldActive(schema, field, root)`, `sp.enumOptions(schema, field, root)` and `sp.listCount(schema, field, root)` — the exact counterparts of Go's `Schema.FieldActive` / `EnumOptions` / `ListCount`.

`schemapb()` (or `Schemapb.load()`) calls `WebAssembly.instantiate` and runs the Go binary, which registers eight globals: `schemapbValidate`, `schemapbCompute`, `schemapbBake`, `schemapbMerge`, `schemapbLink`, `schemapbFieldActive`, `schemapbEnumOptions`, `schemapbListCount`. These accept and return JSON strings; the TypeScript wrapper handles serialisation via protobuf-es `toJson`/`fromJson`. The engine is the *same* Go code compiled to WASM, so the TS and Go SDKs validate, compute, bake, merge, link, and render identically.

Schemas may also be fetched from a running `SchemaService` with `GetSchema` and deserialised with protobuf-es — no need to build them client-side.

### React

[`@stroppy-io/schemapb-react`](packages/schemapb-react) is a **headless** hook on top of this SDK: it owns the form logic (state + validate/compute/bake + `when`/`options`/`count` helpers) and leaves rendering to your own components.

```tsx
const form = useSchemaForm({ schema, initialValues: {} });
form.register("name");          // { name, value, error, onChange }
form.fieldActive("tls_cert");   // when-gate → show/hide
form.enumOptions("version");    // dynamic options for a <select>
form.handleSubmit(baked => save(baked.values));
```

See the [package README](packages/schemapb-react/README.md) for the full API.

---

## gRPC SchemaService

### RPCs

| RPC | Request | Response | Notes |
|---|---|---|---|
| `RegisterSchema` | `Schema` | `RegisterSchemaResponse` | Validates and stores; returns identity + errors |
| `GetSchema` | `SchemaIdentity` | `Schema` | Fetch by namespace/name/version |
| `ListSchemas` | `Filter` | `ListSchemasResponse` | Filter by namespace, name, version, name_contains (substring) |
| `ValidateSchema` | `Schema` | `ValidateSchemaResponse` | Descriptor well-formedness only |
| `Validate` | `Filled` | `ValidateResponse` | Form value validation (inline or id-ref schema) |
| `Compute` | `Filled` | `ComputeResponse` | Seed defaults + evaluate Computed fields |
| `Bake` | `Filled` | `BakeResponse` | Validate + resolve + seal |
| `Merge` | `MergeRequest` | `BakeResponse` | Layer overrides onto a `Baked`; `ListMerge_LIST_MERGE_APPEND` or `LIST_MERGE_REPLACE` |

A `Filled` carries a `SchemaRef`, which is either an inline `Schema` or a `SchemaIdentity` (resolved from the registry). `BakeResponse.baked` is unset when there are blocking errors.

### Server setup

```go
import (
    "google.golang.org/grpc"
    "github.com/stroppy-io/schemapb/schemapb"
)

// DefaultConfig: AllowRegister=true, AllowInlineSchema=true, InMemoryRegistry.
cfg := schemapb.DefaultConfig()

// Tighten for production: lock down registration, require registry-based schemas.
cfg.AllowRegister = false
cfg.AllowInlineSchema = false

// Optionally inject a custom registry (e.g. backed by a database).
cfg.Registry = myDBRegistry{}

server := schemapb.NewServer(cfg)

grpcServer := grpc.NewServer()
schemapb.RegisterSchemaServiceServer(grpcServer, server)
```

### Config

```go
type Config struct {
    Registry          Registry // nil → fresh InMemoryRegistry
    AllowRegister     bool     // permit RegisterSchema RPC
    AllowInlineSchema bool     // permit inline schemas in requests
}

func DefaultConfig() Config  // AllowRegister=true, AllowInlineSchema=true
```

### Registry interface

```go
type Registry interface {
    Put(ctx context.Context, s *Schema) error
    Get(ctx context.Context, id *SchemaIdentity) (*Schema, error)
    List(ctx context.Context, filter *Filter) ([]*Schema, error)
}
```

Implementations must return `schemapb.ErrNotFound` from `Get` when a schema is absent so the server maps it to a gRPC `NOT_FOUND` status. The built-in `InMemoryRegistry` is suitable for development and testing.

```go
reg := schemapb.NewInMemoryRegistry()
```

---

## Development

### Regenerate protobuf code

```sh
make proto
```

Runs `easyp generate` (`easyp.yaml`), which invokes four plugins from the `schemapb/` input directory:

| Plugin | Output | Purpose |
|---|---|---|
| `protoc-gen-go` (`go`) | `.` (source-relative) | Go message types |
| `go-grpc` | `.` | gRPC server/client stubs |
| `go-hashpb` | `.` | Content-hash support used by `schemapb.Hash` |
| `protobuf-es` (`es`) | `packages/schemapb/` | TypeScript types with JSON types and `create()`/`toJson()` |

The hand-written Go files (`new.go`, `validate.go`, `compute.go`, `server.go`, `registry.go`) live alongside the generated files in `schemapb/` and add methods directly to the generated types.

### Build WASM

```sh
make wasm
```

Compiles the `wasm/` entry point (`GOOS=js GOARCH=wasm`) and copies Go's runtime loader:

```sh
GOOS=js GOARCH=wasm go build -o packages/schemapb/schemapb.wasm ./wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" packages/schemapb/wasm_exec.js
```

### Run tests

```sh
make test                          # Go tests: go test ./...
npm install                        # once, from the repo root (npm workspaces)
npm test --workspaces --if-present # all TS packages via vitest (build WASM first)
```

### Validator cache

Compiled expr-lang programs (rules, Computed expressions, normalize expressions) are cached globally in a `sync.Map` keyed by the schema's SHA-256 content hash (`schemapb.Hash`). Equal schemas compile once — there is no per-server or per-request compilation after the first call for a given schema.

### Release / publish

```sh
make release    # interactive: bump version or recreate the latest tag
```

Pushing a `vX.Y.Z` tag triggers `.github/workflows/publish-npm.yml`, which builds the WASM, runs `tsup`, and publishes the npm package to GitHub Packages.

### Directory layout

```
schemapb/           Core Go package
  schema.proto      Schema + FieldError + Baked/Filled/SchemaRef messages
  service.proto     SchemaService RPCs + request/response messages
  schema.pb.go      Generated (protoc-gen-go)
  service.pb.go     Generated
  service_grpc.pb.go Generated (go-grpc)
  new.go            Fluent chain builder API (hand-written)
  validate.go       Validation engine + IsValid + Hash (hand-written)
  compute.go        Compute/Bake/Merge engine (hand-written)
  server.go         gRPC server implementation (hand-written)
  registry.go       Registry interface + InMemoryRegistry (hand-written)

packages/           npm workspaces (TypeScript)
  schemapb/         @stroppy-io/schemapb — WASM SDK
    schemapb.ts     WASM wrapper (Schemapb class + schemapb() loader)
    index.ts        Package entry point
    schemapb/
      schema_pb.ts  Generated protobuf-es types
      service_pb.ts Generated protobuf-es service types
    schemapb.wasm   Compiled WASM module (not in git; built by make wasm)
    wasm_exec.js    Go runtime loader (not in git; copied by make wasm)
  schemapb-react/   @stroppy-io/schemapb-react — headless useSchemaForm hook
    useSchemaForm.ts  The hook (state + WASM engine, bring your own UI)
    helpers.ts        Pure path/error utilities

wasm/               Go WASM entry point (main.go)
  main.go           Registers schemapbValidate/Compute/Bake/Merge/Link/... globals

cmd/schemapbgen/    Code generator CLI (own go.mod; cobra)
  main.go           Flags + protojson/go-builder input -> model -> emit
  internal/parse/   protojson loader + go:generate builder bridge
  internal/model/   schema -> naming-resolved IR (types, enums, oneofs)
  internal/emit/    IR -> gofmt'd Go (structs, sugar, engine-compatible JSON)

go.mod              module github.com/stroppy-io/schemapb
easyp.yaml          easyp code-generation configuration
Makefile            proto / wasm / test / release targets
```

### Module coordinates

- Go: `github.com/stroppy-io/schemapb` — import path `github.com/stroppy-io/schemapb/schemapb`
- Go CLI: `github.com/stroppy-io/schemapb/cmd/schemapbgen` (separate module)
- npm: `@stroppy-io/schemapb` (GitHub Packages, `https://npm.pkg.github.com`)
