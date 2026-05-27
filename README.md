# schemapb

A **runtime, dynamic form/schema descriptor** with a validation and derived-value computation engine that runs the **same expression-language engine on both the Go server and in the browser via WebAssembly**. Build your schema once in a fluent API, and server and client agree on validation rules, computed fields, and defaults—no reimplementation, no drift. Ideal for configuration UIs (e.g., `postgresql.conf`-style settings), dynamic form generation, and cross-platform data validation.

## Install

### Go

```bash
go get github.com/stroppy-io/schemapb/schemapb
```

### TypeScript / npm

Configure GitHub Packages access in `.npmrc`:

```
@stroppy-io:registry=https://npm.pkg.github.com
```

Then install:

```bash
npm install @stroppy-io/schemapb
```

The package includes compiled WebAssembly (`schemapb.wasm`) and the Go runtime loader (`wasm_exec.js`).

## Quickstart (Go)

Build a schema with the fluent API, then validate, compute, and bake values:

```go
package main

import (
	"fmt"
	"github.com/stroppy-io/schemapb/schemapb"
)

func main() {
	// Define a disk configuration schema with enum, constraints, immutable field,
	// grouping, unit metadata, and a derived field.
	s := schemapb.NewSchema("infra", "disk", "v1").
		Descr("Disk configuration").
		Fields(
			schemapb.Enum("disk_type").
				Values(map[int32]string{0: "ssd", 1: "hdd"}).
				Default(0).
				Immutable().
				Group("Hardware"),
			schemapb.Int64("disk_size").
				Gte(10).
				Lte(1000).
				Default(100).
				Unit("GB").
				Group("Hardware").
				Desc("Size in GB"),
			schemapb.Int64("iops_limit").
				Gte(100).
				Default(3000).
				Unit("ops/sec").
				Group("Performance").
				Rules(
					schemapb.Rule("root.iops_limit <= root.disk_size * 100", "IOPS cannot exceed disk_size * 100").
						ID("iops_constraint").
						Warn(),
				),
			schemapb.Computed("calculated_throughput", "root.iops_limit / 1000").
				Result(schemapb.ResultDouble).
				Group("Performance").
				Desc("Derived throughput in MB/s"),
		).
		MustBuild()

	// Validate: check immutable, required, structured rules, and expr rules.
	values := map[string]any{
		"disk_type": 0.0,
		"disk_size": 500.0,
		"iops_limit": 45000.0,
	}
	errs := s.Validate(values)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Printf("Validation error at %s: %s\n", err.Field, err.Message)
		}
		return
	}

	// Compute: fill defaults and evaluate Computed fields in dependency order.
	resolved, errs := s.Compute(values)
	if len(errs) > 0 {
		fmt.Printf("Compute failed: %v\n", errs)
		return
	}
	fmt.Printf("Resolved: %v\n", resolved)

	// Bake: validate + resolve, then seal into a snapshot with the schema.
	baked, errs := s.Bake(values)
	if len(errs) > 0 {
		fmt.Printf("Bake failed: %v\n", errs)
		return
	}
	fmt.Printf("Baked schema hash: %x\n", schemapb.Hash(baked.GetSchema()))

	// Merge: layer overrides onto a baked form and re-seal against the same schema.
	overrides := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"iops_limit": {Kind: &structpb.Value_NumberValue{NumberValue: 5000}},
		},
	}
	merged, errs := baked.Merge(overrides, false)
	if len(errs) > 0 {
		fmt.Printf("Merge failed: %v\n", errs)
		return
	}
	fmt.Printf("Merged: %v\n", merged.GetValues().AsMap())
}
```

## Concepts

### Field Kinds

Each field has exactly one kind, with type-specific structured constraints:

- **Numeric** (`Int32`, `Int64`, `UInt32`, `UInt64`, `Float`, `Double`): `Gt`, `Gte`, `Lt`, `Lte`, `In`, `NotIn`, `MultipleOf`, `Default`, `Const`
- **Bool**: `Default`, `Const`
- **String**: `MinLen`, `MaxLen`, `Len`, `Pattern` (regex), `In`, `NotIn`, `Default`, `Const`
- **Enum**: integer-keyed enumeration; `Values` map, `DefinedOnly` (restrict to known keys), `In`, `NotIn`, `Default`
- **Duration**: time.Duration; `Gt`, `Gte`, `Lt`, `Lte`, `Default`
- **Timestamp**: RFC3339 time; `Gt`, `Gte`, `Lt`, `Lte`, `Default`
- **List**: homogeneous array; `MinItems`, `MaxItems`, `Unique`, items define the element schema
- **Object**: nested object; fields define the child schema
- **Computed**: derived, read-only field; `Expr` (expression) and `Result` (type hint for marshaling)

### Structured Constraints

Single-field constraints are checked before rules. They are ergonomic to author and introspectable by UI renderers (e.g., slider bounds, step).

```go
schemapb.Int64("age").Gte(18).Lte(120)
schemapb.Str("email").Pattern(`^[^@]+@[^@]+$`)
schemapb.List("tags", schemapb.Str("tag")).MinItems(1).MaxItems(5).Unique()
```

### Cross-Field Rules (Expr-Lang)

Rules are CEL (Common Expression Language) expressions that read `this` (the field value or scope) and `root` (all form inputs + defaults + computed fields) and evaluate to `bool`. `true` = valid, `false` = invalid. Attach them to fields or to the schema itself.

```go
schemapb.Rule("root.password.len >= 8", "password must be at least 8 characters").ID("pw_len"),
schemapb.Rule("root.password == root.confirm_password", "passwords must match").ID("pw_match"),
schemapb.Rule("root.work_mem * root.max_connections <= 4096", "budget exceeded").ID("mem_budget"),
```

Rules can be marked with `.Warn()` to issue warnings instead of errors—warnings do not block validation.

### Computed (Derived) Fields

Computed fields are read-only and evaluated in dependency order. They read from `root` and produce a value of a specified type.

```go
schemapb.Computed("total_cost", "root.price * root.quantity").Result(schemapb.ResultDouble)
schemapb.Computed("formatted_date", "root.created_at").Result(schemapb.ResultString)
```

The expression engine detects cycles and reports them at schema validation time.

### Defaults and Immutable Fields

**Defaults** are seeded when a field is unset during validation and compute. **Immutable** fields are system-fixed—they always revert to their default, and any attempt to change them is rejected.

```go
schemapb.Str("api_key").Default("auto-generated-key").Immutable()
```

### Group and Unit (Metadata)

**Group** and **Unit** are informational metadata for UI rendering:

- `Group("Hardware")` groups related fields visually
- `Unit("GB")` labels the unit of measurement

```go
schemapb.Int64("disk_size").Unit("GB").Group("Storage")
```

### Filled vs. Baked

- **Filled**: A form and its schema (identity or inline). The schema may or may not be resolved. Used for interchange.
- **Baked**: A resolved, validated, sealed snapshot: schema + final values. It carries the authority of the schema at the moment it was sealed (hashable by content). Result of `Schema.Bake()` or `Filled.Bake()`.

## TypeScript / Browser Usage

Load the WASM module and use the fluent API on the client side with protobuf-es:

```typescript
import { Schemapb, SchemaSchema, create } from "@stroppy-io/schemapb";
import type { Schema } from "@stroppy-io/schemapb";

// Load the wasm module once.
const wasmBytes = await fetch("/@stroppy-io/schemapb/schemapb.wasm").then(r => r.arrayBuffer());
const sp = await Schemapb.load(wasmBytes);

// Build a schema using protobuf-es constructors (or fetch from the server).
const schema: Schema = create(SchemaSchema, {
  id: { namespace: "config", name: "app", version: "v1" },
  description: "Application configuration",
  fields: [
    {
      name: "debug",
      kind: { case: "bool", value: { default: false } },
    },
    {
      name: "max_connections",
      kind: { case: "int64", value: { default: 100, gte: 1 } },
    },
    {
      name: "timeout_ms",
      kind: {
        case: "computed",
        value: { expr: "root.max_connections * 10" },
      },
    },
  ],
});

// Validate against the schema.
const values = { debug: true, max_connections: 500 };
const validation = sp.validate(schema, values);
console.log(validation.ok ? "✓ Valid" : "✗ Errors:", validation.errors);

// Compute derived fields.
const computed = sp.compute(schema, values);
console.log("Resolved:", computed.values); // { debug: true, max_connections: 500, timeout_ms: 5000 }

// Bake: validate + resolve + seal.
const result = sp.bake(schema, values);
if (result.baked) {
  console.log("Sealed schema:", result.baked.schema);
  console.log("Final values:", result.baked.values);
} else {
  console.log("Bake failed:", result.errors);
}

// Merge: layer overrides.
const merged = sp.merge(result.baked!, { max_connections: 200 });
console.log("After merge:", merged.baked?.values);
```

The same expression-language engine runs server-side and client-side: no drift, no reimplementation.

## gRPC SchemaService

The service provides schema registration, validation, and computation over gRPC:

### RPCs

- **RegisterSchema(Schema)** → RegisterSchemaResponse: Register and validate a schema; returns its identity.
- **GetSchema(SchemaIdentity)** → Schema: Fetch a registered schema by namespace/name/version.
- **ListSchemas(Filter)** → ListSchemasResponse: List schemas, optionally filtered by name, namespace, version, or substring.
- **ValidateSchema(Schema)** → ValidateSchemaResponse: Check that a descriptor is well-formed.
- **Validate(Filled)** → ValidateResponse: Validate form values against a schema (by id or inline).
- **Compute(Filled)** → ComputeResponse: Evaluate Computed fields.
- **Bake(Filled)** → BakeResponse: Validate + resolve + seal.
- **Merge(MergeRequest)** → BakeResponse: Layer overrides onto a Baked and re-seal.

### Registry Interface

Implement the `Registry` interface to persist or fetch schemas:

```go
type Registry interface {
	Put(ctx context.Context, s *Schema) error
	Get(ctx context.Context, id *SchemaIdentity) (*Schema, error)
	List(ctx context.Context, filter *Filter) ([]*Schema, error)
}
```

The default is `InMemoryRegistry`, which stores schemas in memory (suitable for dev/test).

### Config

```go
type Config struct {
	Registry          Registry
	AllowRegister     bool  // permit the RegisterSchema RPC
	AllowInlineSchema bool  // permit inline schemas in requests
}
```

Use `DefaultConfig()` for a permissive setup, then tighten as needed:

```go
cfg := schemapb.DefaultConfig()
cfg.AllowRegister = false  // lock down registration
server := schemapb.NewServer(cfg)
```

## Development

### Regenerate Protobuf Code

```bash
make proto
```

Runs `easyp generate`, which invokes:
- `protoc-gen-go` for Go message types
- `go-grpc` for server/client stubs
- `go-hashpb` for content-hash support
- `protobuf-es` for TypeScript types

Configuration is in `easyp.yaml`.

### Build WASM

```bash
make wasm
```

Builds Go's WASM target (`GOOS=js GOARCH=wasm`) and copies Go's loader (`wasm_exec.js`).

### Run Tests

```bash
make test
```

Runs all Go tests. TypeScript tests are in `ts/` and run via `npm test`.

### Directory Structure

- **schemapb/** — Core Go package:
  - `new.go` — Fluent builder API
  - `validate.go` — Validation and error types
  - `compute.go` — Computed field evaluation
  - `server.go` — gRPC server implementation
  - `registry.go` — Schema storage interface
  - `schema.proto`, `service.proto` — Protobuf definitions
  - Generated: `schema.pb.go`, `service.pb.go`, `service_grpc.pb.go`
- **ts/** — TypeScript package:
  - `schemapb.ts` — WASM wrapper
  - `index.ts` — Package entry point
  - `schemapb/schema_pb.ts` — Generated protobuf-es types
  - `schemapb.wasm` — Compiled WASM module
  - `wasm_exec.js` — Go runtime loader
  - `package.json` — npm configuration
- **wasm/** — Go WASM entry point (`main.go`) registering the validation/compute/bake/merge functions.

### Module

- Go: `github.com/stroppy-io/schemapb`
- npm: `@stroppy-io/schemapb` (GitHub Packages)
