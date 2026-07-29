package schemapb_test

import (
	"context"
	"fmt"
	"sort"

	schemapb "github.com/stroppy-io/schemapb/schemapb"
)

// dumpErrs prints field errors as sorted "field: code" lines, so example output
// is deterministic regardless of validation order.
func dumpErrs(errs []*schemapb.FieldError) {
	lines := make([]string, 0, len(errs))
	for _, e := range errs {
		lines = append(lines, e.GetField()+": "+e.GetCode())
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		fmt.Println("(valid)")
		return
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}

// Build a schema with the fluent API and validate a form against it.
func Example() {
	s := schemapb.NewSchema("infra", "server", "v1").Fields(
		schemapb.Str("name").Required().MinLen(2),
		schemapb.Int32("port").Gte(1).Lte(65535).Default(8080),
		schemapb.Bool("tls"),
	).MustBuild()

	ok, _ := s.ValidateJSON([]byte(`{"name":"api","port":443}`))
	fmt.Print("valid form -> ")
	dumpErrs(ok)

	bad, _ := s.ValidateJSON([]byte(`{"name":"x","port":99999}`))
	fmt.Print("bad form   -> ")
	dumpErrs(bad)

	// Output:
	// valid form -> (valid)
	// bad form   -> name: min_len
	// port: lte
}

// Compute seeds defaults for unset fields and evaluates Computed (derived)
// fields, in dependency order, over the whole form (`root`).
func ExampleSchema_Compute() {
	s := schemapb.NewSchema("db", "tuning", "v1").Fields(
		schemapb.Int64("max_connections").Gte(1).Default(100),
		// effective_cache derives from another input.
		schemapb.Computed("effective_cache_mb", "root.max_connections * 4").
			Result(schemapb.ResultInt64),
	).MustBuild()

	out, _ := s.Compute(map[string]any{}) // nothing supplied
	fmt.Println("max_connections:", out["max_connections"])
	fmt.Println("effective_cache_mb:", out["effective_cache_mb"])

	// Output:
	// max_connections: 100
	// effective_cache_mb: 400
}

// A cross-field Rule validates an invariant over the whole form. WARNING rules
// surface without blocking.
func ExampleRule() {
	s := schemapb.NewSchema("db", "mem", "v1").
		Fields(
			schemapb.Int64("work_mem").Gte(1).Default(4),
			schemapb.Int64("max_connections").Gte(1).Default(100),
		).
		Rules(
			schemapb.Rule("root.work_mem * root.max_connections <= 1024",
				"memory budget exceeded").ID("mem_budget"),
		).
		MustBuild()

	over, _ := s.ValidateJSON([]byte(`{"work_mem":64,"max_connections":100}`))
	dumpErrs(over)

	// Output:
	// mem_budget: rule
}

// When gates a field's existence; RequiredWhen makes a visible field
// conditionally required.
func ExampleSchema_when() {
	s := schemapb.NewSchema("net", "tls", "v1").
		Fields(
			schemapb.Bool("tls"),
			// cert is hidden + ignored unless tls is on, and required when it is.
			schemapb.Str("cert").Required().When("root.tls == true"),
		).
		MustBuild()

	off, _ := s.ValidateJSON([]byte(`{"tls":false}`))
	fmt.Print("tls off -> ")
	dumpErrs(off)

	on, _ := s.ValidateJSON([]byte(`{"tls":true}`))
	fmt.Print("tls on  -> ")
	dumpErrs(on)

	// Output:
	// tls off -> (valid)
	// tls on  -> cert: required
}

// OneOf validates a discriminated union: the discriminator selects the variant
// schema to apply.
func ExampleOneOf() {
	s := schemapb.NewSchema("app", "storage", "v1").Fields(
		schemapb.OneOf("backend", "type").
			Variant("s3", schemapb.Str("bucket").Required()).
			Variant("disk", schemapb.Int32("size_gb").Required().Gte(1)).
			Required(),
	).MustBuild()

	s3, _ := s.ValidateJSON([]byte(`{"backend":{"type":"s3","bucket":"data"}}`))
	fmt.Print("s3   -> ")
	dumpErrs(s3)

	bad, _ := s.ValidateJSON([]byte(`{"backend":{"type":"disk"}}`))
	fmt.Print("disk -> ")
	dumpErrs(bad)

	// Output:
	// s3   -> (valid)
	// disk -> backend.size_gb: required
}

// Ref + $defs reuse a named sub-schema; a def may reference itself for recursive
// data (the recursion follows the finite value).
func ExampleRef() {
	s := schemapb.NewSchema("app", "tree", "v1").
		Def("node",
			schemapb.Str("label").Required(),
			schemapb.List("children", schemapb.Ref("child", "node")),
		).
		Fields(schemapb.Ref("root", "node").Required()).
		MustBuild()

	errs, _ := s.ValidateJSON([]byte(`{"root":{"label":"a","children":[{"label":"b"}]}}`))
	dumpErrs(errs)

	// Output:
	// (valid)
}

// RefID references another schema by identity; Link resolves it (transitively)
// against a registry so the result validates standalone while keeping identity.
func ExampleSchema_Link() {
	ctx := context.Background()
	reg := schemapb.NewInMemoryRegistry()
	_ = reg.Put(ctx, schemapb.NewSchema("infra", "db", "v1").Fields(
		schemapb.Str("host").Required(),
		schemapb.Int32("port").Gte(1).Lte(65535).Default(5432),
	).MustBuild())

	cfg := schemapb.NewSchema("app", "cfg", "v1").Fields(
		schemapb.RefID("primary", &schemapb.SchemaIdentity{
			Namespace: "infra", Name: "db", Version: "v1",
		}).Required(),
	).MustBuild()

	linked, err := cfg.Link(ctx, reg)
	if err != nil {
		fmt.Println("link error:", err)
		return
	}
	errs, _ := linked.ValidateJSON([]byte(`{"primary":{"host":"db-1"}}`))
	dumpErrs(errs)
	// The id-ref node still carries its identity (renderer-friendly).
	fmt.Println("ref ->", linked.GetFields()[0].GetRef().GetId().GetName())

	// Output:
	// (valid)
	// ref -> db
}

// Bake seals a validated + resolved form with its schema into a portable
// snapshot.
func ExampleSchema_Bake() {
	s := schemapb.NewSchema("infra", "sizing", "v1").Fields(
		schemapb.Int32("replicas").Gte(1).Default(3),
		schemapb.Computed("quorum", "(root.replicas + 1) / 2").Result(schemapb.ResultInt64),
	).MustBuild()

	baked, errs := s.Bake(map[string]any{}) // defaults applied
	if len(errs) != 0 {
		dumpErrs(errs)
		return
	}
	vals := baked.GetValues().AsMap()
	fmt.Println("replicas:", vals["replicas"])
	fmt.Println("quorum:", vals["quorum"])

	// Output:
	// replicas: 3
	// quorum: 2
}

// EnumOptions returns the allowed values for an enum — dynamic when the field
// carries an options expression over `root`.
func ExampleSchema_EnumOptions() {
	s := schemapb.NewSchema("db", "ver", "v1").Fields(
		schemapb.Str("edition"),
		schemapb.Enum("version").
			Values(map[int32]string{13: "13", 14: "14", 15: "15", 16: "16"}).
			Options(`root.edition == "lts" ? [14, 16] : [15]`),
	).MustBuild()

	lts, _ := s.EnumOptions("version", map[string]any{"edition": "lts"})
	fmt.Println("lts options:", lts)

	// Output:
	// lts options: [14 16]
}
