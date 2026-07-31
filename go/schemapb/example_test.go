package schemapb_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

// Example walks the complete public API in one continuous story: authoring
// (every field kind and attribute), registry + identity linking, explicit
// compilation with a custom format, descriptor errors, validation with typed
// error codes, UI helpers, resolve, bake / merge / render, and the typed
// Value layer.
func Example() {
	ctx := context.Background()

	// --- 1. A shared schema, registered by identity -------------------------

	endpointID := schemapb.ID("shared", "endpoint", schemapb.Ver(1, 0, 0))
	endpoint := schemapb.NewSchema(endpointID).
		Descr("reusable network endpoint").
		Fields(
			schemapb.Str("host").Required().Format(schemapb.FormatHostname),
			schemapb.Int64("port").Default(5432).Gte(1).Lte(65535),
		).
		MustBuild()

	reg := schemapb.NewInMemoryRegistry()
	if err := reg.Put(ctx, endpoint); err != nil {
		panic(err)
	}

	got, err := reg.Get(ctx, endpointID)
	if err != nil {
		panic(err)
	}
	// A different schema under a taken identity is rejected; replacing is
	// explicit.
	dupe := schemapb.NewSchema(endpointID).Fields(schemapb.Bool("other")).MustBuild()
	dupErr := reg.Put(ctx, dupe)

	if err := reg.PutReplace(ctx, endpoint); err != nil {
		panic(err)
	}

	listed, _ := reg.List(ctx, &schemapb.ListFilter{Namespace: "shared", NameContains: "END"})
	fmt.Printf("registry: got %s@%s, listed %d, dupe rejected: %v\n",
		got.GetId().SchemaName(), got.GetId().Ver(), len(listed), errors.Is(dupErr, schemapb.ErrAlreadyRegistered))

	// --- 2. The main schema: every kind, every attribute --------------------

	pg := schemapb.NewSchema(schemapb.ID("infra", "postgres", schemapb.MustVersion("v1"))).
		Descr("PostgreSQL instance configuration").
		Strict().
		Coerce().
		MinProps(1).
		MaxProps(50).
		Def("volume",
			schemapb.Str("path").Required(),
			schemapb.Int64("size_gb").Default(10).Gt(0),
		).
		DefSchema("endpoint_copy", endpoint).
		Fields(
			// Numeric kinds share one generic builder.
			schemapb.Int64("shared_buffers").Gte(16).Lte(1<<40).Default(128).
				Unit("MB").Group("Memory").Title("Shared buffers").
				Desc("shared memory for page cache"),
			schemapb.Int32("max_workers").Default(8).MultipleOf(2),
			schemapb.UInt32("shards").In(1, 2, 4),
			schemapb.UInt64("max_bytes").NotIn(0),
			schemapb.Float("sample_rate").Gt(0).Lt(1).Default(0.1),
			schemapb.Double("cost_factor").Const(1.5).Default(1.5).Deprecated(),

			schemapb.Bool("autovacuum").Default(true).Group("Vacuum"),
			schemapb.Str("cluster_name").MinLen(1).MaxLen(32).Pattern(`^[a-z][a-z0-9-]*$`).
				Default("main").Examples(schemapb.StrV("main"), schemapb.StrV("analytics")),
			schemapb.Str("admin_email").Format(schemapb.FormatEmail).Required(),
			schemapb.Str("disk").Format("k8s.quantity").Default("10Gi"),
			schemapb.Str("password").MinLen(8).Secret().Required(),
			schemapb.Bytes("license").MinLen(4).MaxLen(64).Prefix([]byte("LIC-")),
			schemapb.JSON("annotations"),

			schemapb.Choice("wal_level").
				Opt(schemapb.StrV("minimal"), "Minimal").
				Opt(schemapb.StrV("replica"), "Replica").
				Opt(schemapb.StrV("logical"), "Logical").
				Default(schemapb.StrV("replica")).Group("WAL"),
			schemapb.Choice("compression").
				Options(`root.wal_level == "logical" ? ["lz4", "zstd"] : ["off"]`),

			schemapb.Duration("checkpoint_timeout").Default(5*time.Minute).
				Gte(30*time.Second).Lte(time.Hour).Unit("duration"),
			schemapb.Timestamp("maintenance_start").
				Gte(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),

			schemapb.Int64("replica_count").Default(2).Gte(0).Lte(10),
			schemapb.List("replica_names",
				schemapb.Str("").MinLen(1).Rules(
					schemapb.Rule(`this != "primary"`, "replicas cannot be named primary"),
				),
			).Unique().MinItems(0).MaxItems(10).Count("int(root.replica_count)"),

			schemapb.Object("logging",
				schemapb.Bool("collector").Default(false),
				schemapb.Str("directory").Default("log"),
			).Strict(),

			schemapb.Map("tablespaces",
				schemapb.Str("location").Required(),
			).Strict().MinEntries(0).MaxEntries(16),

			schemapb.OneOf("backup", "type").
				Variant("s3", schemapb.Str("bucket").Required()).
				VariantOf("endpoint", endpoint),

			schemapb.Ref("data_volume", "volume"),
			schemapb.RefID("primary_endpoint", endpointID),

			schemapb.Computed("effective_cache", "root.shared_buffers * 3").
				Result(schemapb.ResultInt64).Group("Memory"),
			schemapb.Str("mode").Default("Fast").Normalize("this.lowerAscii()").In("fast", "slow"),
			schemapb.Int64("debug_level").When("root.mode == 'slow'").Default(1).Immutable(),
		).
		Rules(
			schemapb.Rule("int(root.shared_buffers) <= 1024 || root.autovacuum == true",
				"large buffers need autovacuum").ID("buf-vacuum"),
			schemapb.Rule("int(root.replica_count) < 8", "many replicas").Warn(),
			schemapb.Rule("true", "informational").Severity(schemapb.SeverityWarning).Done().Done(),
		).
		RequiredWhen("annotations", `root.mode == "slow"`).
		ForbiddenWhen("max_bytes", `root.mode == "fast" && false`).
		Template("conf", "shared_buffers = {{values.shared_buffers}}\nwal_level = {{values.wal_level}}\n").
		MustBuild()

	// --- 3. Identity linking + explicit compilation -------------------------

	linked, err := pg.Link(ctx, reg)
	if err != nil {
		panic(err)
	}

	if err := linked.CheckDescriptor(); err != nil {
		panic(err)
	}

	eng, err := schemapb.Compile(linked, schemapb.WithFormats(schemapb.FormatRegistry{
		"k8s.quantity": func(s string) bool { return s != "" && s != "0" },
	}))
	if err != nil {
		panic(err)
	}

	fmt.Println("compiled:", eng.Schema().GetId().GetName())

	// --- 4. A malformed schema fails Build with a SchemaError ---------------

	_, err = schemapb.NewSchema(schemapb.ID("t", "broken", schemapb.Ver(0, 1, 0))).Fields(
		schemapb.Computed("a", "root.b + 1"),
		schemapb.Computed("b", "root.a + 1"),
	).Build()

	var se *schemapb.SchemaError

	if ok := errorsAs(err, &se); ok {
		fmt.Println("schema errors:", len(se.Result.GetErrors()))
	}

	// --- 5. Validation: typed codes, masking, severity ----------------------

	bad := map[string]any{
		"shared_buffers": int64(8),                                        // GTE violated
		"admin_email":    "not-an-email",                                  // FORMAT_MISMATCH
		"password":       "short",                                         // MIN_LEN, masked
		"license":        []byte("XX"),                                    // MIN_LEN + PREFIX
		"wal_level":      int64(7),                                        // ENUM_NOT_DEFINED
		"replica_count":  int64(3),                                        //
		"replica_names":  []any{"a", "a"},                                 // COUNT + UNIQUE
		"tablespaces":    map[string]any{"t1": map[string]any{"junk": 1}}, // REQUIRED + UNKNOWN
		"backup":         map[string]any{"type": "tape"},                  // UNKNOWN_VARIANT
		"debug_level":    int64(9),                                        // inactive: ignored
	}
	res := eng.Validate(bad)
	fmt.Println("valid:", res.Ok(), "blocking:", res.Blocking())

	lines := make([]string, 0, len(res.GetErrors()))

	for _, e := range res.GetErrors() {
		masked := ""
		if e.GetPath() == "password" && e.GetActual() == nil {
			masked = " (masked)"
		}

		lines = append(lines, fmt.Sprintf("%s: %s%s", e.GetPath(),
			e.GetCode().String()[len("ERROR_CODE_"):], masked))
	}

	slices.Sort(lines)

	for _, l := range lines {
		fmt.Println(" ", l)
	}

	// --- 6. UI helpers ------------------------------------------------------

	active, _ := eng.FieldActive("debug_level", map[string]any{"mode": "slow"})
	optVals, _ := eng.ChoiceOptions("compression", map[string]any{"wal_level": "logical"})
	opts := make([]string, len(optVals))

	for i, v := range optVals {
		opts[i] = v.GetStringValue()
	}

	count, _ := eng.ListCount("replica_names", map[string]any{"replica_count": int64(4)})
	fmt.Printf("active=%v options=%v count=%d\n", active, opts, count)

	// --- 7. Resolve: defaults, coercion, normalize, computed ----------------

	form := map[string]any{
		"shared_buffers": "256", // coerced from string
		"admin_email":    "dba@corp.io",
		"password":       "s3cret-pass",
		"mode":           "Fast", // normalized to "fast"
		"replica_count":  int64(0),
	}
	form, resolveRes := eng.Resolve(form)
	fmt.Println("resolve ok:", resolveRes.Ok(),
		"buffers:", form["shared_buffers"],
		"cache:", form["effective_cache"],
		"mode:", form["mode"],
		"timeout:", form["checkpoint_timeout"])

	// --- 8. Bake, render, merge, Filled ------------------------------------

	baked, bakeRes, err := eng.Bake(form)
	if err != nil || bakeRes.Blocking() {
		panic(fmt.Sprint(err, bakeRes.GetErrors()))
	}

	text, err := baked.Render("conf")
	if err != nil {
		panic(err)
	}

	fmt.Print(text)

	merged, _, err := eng.Merge(baked, schemapb.MustStructFromGo(map[string]any{"autovacuum": false}), false)
	if err != nil {
		panic(err)
	}

	fmt.Println("merged autovacuum:",
		merged.GetValues().GetFields()["autovacuum"].GetBoolValue(),
		"matches:", merged.Matches(linked))

	filled := &schemapb.Filled{
		Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: endpoint}},
		Values: schemapb.MustStructFromGo(map[string]any{"host": "db.corp.io"}),
	}

	fromFilled, _, err := filled.Bake()
	if err != nil {
		panic(err)
	}

	fmt.Println("filled port:", fromFilled.GetValues().GetFields()["port"].GetInt64Value())

	// --- 9. The typed Value layer ------------------------------------------

	wire := schemapb.ListV(
		schemapb.NullV(), schemapb.BoolV(true),
		schemapb.Int32V(1), schemapb.Int64V(2),
		schemapb.UInt32V(3), schemapb.UInt64V(4),
		schemapb.FloatV(1.5), schemapb.DoubleV(2.5),
		schemapb.StrV("s"), schemapb.BytesV([]byte{0xFF}),
		schemapb.DurationV(time.Second), schemapb.TimestampV(time.Unix(0, 0).UTC()),
		schemapb.StructV(map[string]*schemapb.Value{"k": schemapb.Int64V(9)}),
	)
	native := wire.ToGo().([]any)

	back, err := schemapb.FromGo(native)
	if err != nil {
		panic(err)
	}

	canon, err := schemapb.CanonicalValue(
		schemapb.Int32("n").Done(), float64(42)) // integral float -> int32 kind
	if err != nil {
		panic(err)
	}

	fmt.Printf("values: native=%d roundtrip=%d canonical=%T\n",
		len(native), len(back.GetListValue().GetItems()), canon.GetKind())

	// Output:
	// registry: got endpoint@v1.0.0, listed 1, dupe rejected: true
	// compiled: postgres
	// schema errors: 2
	// valid: false blocking: true
	//   admin_email: FORMAT_MISMATCH
	//   backup: UNKNOWN_VARIANT
	//   license: MIN_LEN_VIOLATED
	//   license: PREFIX_MISMATCH
	//   password: MIN_LEN_VIOLATED (masked)
	//   replica_names: LIST_COUNT_MISMATCH
	//   replica_names[1]: NOT_UNIQUE
	//   shared_buffers: GTE_VIOLATED
	//   tablespaces.t1.junk: UNKNOWN_FIELD
	//   tablespaces.t1.location: REQUIRED_MISSING
	//   wal_level: CHOICE_NOT_ALLOWED
	// active=true options=[lz4 zstd] count=4
	// resolve ok: true buffers: 256 cache: 768 mode: fast timeout: 5m0s
	// shared_buffers = 256
	// wal_level = replica
	// merged autovacuum: false matches: true
	// filled port: 5432
	// values: native=13 roundtrip=13 canonical=*schemapb.Value_Int32Value
}
