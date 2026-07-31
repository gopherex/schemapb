package schemapb_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The golden files under conformance/golden/ are the cross-language contract
// seed: the Go reference implementation WRITES them (go test -update), every
// other implementation READS them and must produce byte-identical results.
//
//	full-schema.json       — a kitchen-sink Schema exercising every kind and
//	                         every attribute the contract has
//	full-baked.json        — the Baked snapshot of a valid form against it
//	                         (canonical Value variants, defaults, computed)
//	full-errors.json       — the ValidationResult for a deliberately broken
//	                         form (codes, typed expected/actual, order)
//	full-rendered.txt      — Mustache render output of the baked form
var update = flag.Bool("update", false, "rewrite golden files")

const goldenDir = "../../conformance/golden"

// goldenSchemaID is declared once, identity-handle doctrine.
var goldenSchemaID = schemapb.ID("conformance", "kitchen_sink", schemapb.Ver(1, 0, 0))

// goldenSchema builds the kitchen-sink schema: every field kind, every
// constraint field, every field attribute, every schema attribute appears at
// least once.
func goldenSchema(t *testing.T) *schemapb.Schema {
	t.Helper()
	endpointID := schemapb.ID("conformance", "endpoint", schemapb.Ver(2, 1, 0))
	endpoint := schemapb.NewSchema(endpointID).Fields(
		schemapb.Str("host").Required(),
		schemapb.Int64("port").Default(5432),
	).MustBuild()

	s, err := schemapb.NewSchema(goldenSchemaID).
		Descr("conformance kitchen sink: every kind, every attribute").
		Strict().
		Coerce().
		MinProps(1).
		MaxProps(64).
		Def("volume",
			schemapb.Str("path").Required(),
			schemapb.Int64("size_gb").Default(10).Gt(0),
		).
		DefSchema("endpoint", endpoint).
		Fields(
			schemapb.Float("f32").Default(0.5).Gt(0).Lt(1).NotIn(0.25),
			schemapb.Double("f64").Const(1.5).Default(1.5).MultipleOf(0.5),
			schemapb.Int32("i32").Default(8).Gte(0).Lte(64).MultipleOf(2).In(2, 4, 8, 16, 32, 64),
			schemapb.Int64("i64").Default(128).Gte(16).Lte(1<<40).
				Unit("MB").Group("numbers").Title("Big number").
				Desc("an int64 with everything"),
			schemapb.UInt32("u32").In(1, 2, 4).Default(2),
			schemapb.UInt64("u64").NotIn(0).Default(1),

			schemapb.Bool("flag").Default(true).Group("flags"),
			schemapb.Bool("pinned").Const(true).Default(true),

			schemapb.Str("name").Default("main").MinLen(1).MaxLen(32).
				Pattern(`^[a-z][a-z0-9-]*$`).
				Examples(schemapb.StrV("main"), schemapb.StrV("analytics")),
			schemapb.Str("mode").In("fast", "slow").NotIn("legacy").Default("Fast").
				Normalize("this.lowerAscii()"),
			schemapb.Str("exact").Len(4).Default("abcd"),
			schemapb.Str("mail").Format(schemapb.FormatEmail).Required(),
			schemapb.Str("token").MinLen(8).Secret().Required().Deprecated(),

			schemapb.Bytes("license").MinLen(4).MaxLen(64).
				Prefix([]byte("LIC-")).Suffix([]byte("-END")).
				Default([]byte("LIC-x-END")),
			schemapb.Bytes("magic").Const([]byte{0xDE, 0xAD}).
				In([]byte{0xDE, 0xAD}, []byte{0xBE, 0xEF}).NotIn([]byte{0x00}),

			schemapb.Choice("wal_level").
				OptFull(&schemapb.Schema_Field_Choice_Option{
					Value: schemapb.StrV("minimal"), Label: "Minimal",
					Description: "bare minimum WAL", Deprecated: true,
				}).
				Opt(schemapb.StrV("replica"), "Replica").
				Opt(schemapb.StrV("logical"), "Logical").
				Default(schemapb.StrV("replica")).Group("choices"),
			schemapb.Choice("cpu").IntOpts(2, 4, 8).Default(schemapb.Int64V(4)),
			schemapb.Choice("region").StrOpts("eu", "us").Open(),
			schemapb.Choice("compression").
				Options(`root.wal_level == "logical" ? ["lz4", "zstd"] : ["off"]`),

			schemapb.Duration("timeout").Default(5*time.Minute).
				Gt(0).Gte(time.Second).Lt(2*time.Hour).Lte(time.Hour),
			schemapb.Timestamp("not_before").
				Gt(time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)).
				Gte(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).
				Lt(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)).
				Lte(time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC)).
				Default(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)),

			schemapb.List("endpoint_pair",
				schemapb.Str("host").MinLen(1),
				schemapb.Int64("port").Gte(1).Lte(65535),
			),

			schemapb.Int64("replica_count").Default(2).Gte(0).Lte(10),
			schemapb.List("replicas",
				schemapb.Object("",
					schemapb.Str("name").Required(),
					schemapb.Int64("weight").Default(1),
				).Rules(schemapb.Rule("true", "always fine")),
			).MinItems(0).MaxItems(10).Unique().Count("int(root.replica_count)"),

			schemapb.Object("logging",
				schemapb.Bool("collector").Default(false),
				schemapb.Str("directory").Default("log"),
			).Strict().MinProps(0).MaxProps(8).
				Rule(schemapb.Rule(`root.logging.directory != ""`, "dir required").ID("log-dir")),

			schemapb.Map("tablespaces",
				schemapb.Str("location").Required(),
			).Strict().MinEntries(0).MaxEntries(16).
				Rule(schemapb.Rule("true", "map value rule")),

			schemapb.OneOf("backup", "type").
				Variant("s3", schemapb.Str("bucket").Required()).
				VariantOf("endpoint", endpoint),

			schemapb.Ref("data_volume", "volume"),
			schemapb.RefID("primary_endpoint", endpointID),

			schemapb.Computed("cache", "root.i64 * 3").Result(schemapb.ResultInt64).
				Group("numbers"),
			schemapb.Json("annotations").Default(schemapb.MustFromGo(map[string]any{
				"team": "storage", "tier": int64(1),
			})),
			schemapb.Int64("debug_level").When(`root.mode == "slow"`).
				Default(1).Immutable().Nullable(),
		).
		Rules(
			schemapb.Rule("int(root.i64) >= int(root.i32)", "i64 must cover i32").ID("cross"),
			schemapb.Rule("int(root.replica_count) < 8", "many replicas").Warn(),
			schemapb.Rule("true", "explicit severity").Severity(schemapb.SeverityError),
		).
		RequiredWhen("annotations", `root.mode == "slow"`).
		RequiredUnless("mail", "false").
		ForbiddenWhen("u64", `root.mode == "legacy-off"`).
		Template("conf", "name = {{values.name}}\nwal_level = {{values.wal_level}}\ncache = {{values.cache}}\n").
		Template("report", "{{#groups}}[{{name}}]\n{{#fields}}{{#set}}{{name}}={{{value}}}{{#label}} ({{label}}){{/label}}\n{{/set}}{{/fields}}{{/groups}}").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// goldenEngine compiles the schema with the conformance format extension.
func goldenEngine(t *testing.T, s *schemapb.Schema) *schemapb.Engine {
	t.Helper()
	eng, err := schemapb.Compile(s, schemapb.WithFormats(schemapb.FormatRegistry{
		"x.nonempty": func(v string) bool { return v != "" },
	}))
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

// validInput is the valid form baked into full-baked.json.
func validInput() map[string]any {
	return map[string]any{
		"i64":           "256", // coerced
		"mail":          "dba@corp.io",
		"token":         "s3cret-token",
		"magic":         []byte{0xDE, 0xAD},
		"replica_count": int64(1),
		"replicas":      []any{map[string]any{"name": "r1"}},
		"tablespaces": map[string]any{
			"main": map[string]any{"location": "/var/lib/ts"},
		},
		"backup":        map[string]any{"type": "s3", "bucket": "backups"},
		"data_volume":   map[string]any{"path": "/data"},
		"region":        "somewhere-else", // open choice: fine
		"endpoint_pair": []any{"db1", int64(5432)},
	}
}

// brokenInput trips every category of validation failure deterministically.
func brokenInput() map[string]any {
	return map[string]any{
		"f32":           0.25,                   // NOT_IN + (gt/lt ok)
		"f64":           2.0,                    // CONST + MULTIPLE_OF ok
		"i32":           int64(5),               // MULTIPLE_OF + NOT_IN_ALLOWED_SET
		"i64":           int64(8),               // GTE
		"u32":           uint64(3),              // NOT_IN_ALLOWED_SET
		"u64":           uint64(0),              // IN_FORBIDDEN_SET
		"pinned":        false,                  // CONST (bool) + IMMUTABLE? (no: not immutable)
		"name":          "Bad Name!",            // PATTERN
		"mode":          "legacy",               // IN + NOT_IN (after normalize no-op)
		"exact":         "abcde",                // LEN
		"mail":          "not-an-email",         // FORMAT
		"token":         "short",                // MIN_LEN (masked)
		"license":       []byte("XX"),           // MIN_LEN + PREFIX + SUFFIX
		"magic":         []byte{0x00},           // CONST + IN + NOT_IN
		"wal_level":     "extreme",              // CHOICE_NOT_ALLOWED
		"cpu":           int64(3),               // CHOICE_NOT_ALLOWED
		"timeout":       "3h",                   // LT + LTE
		"not_before":    "2020-01-01T00:00:00Z", // GT + GTE
		"replica_count": int64(2),
		"replicas": []any{ // COUNT mismatch (2 wanted, 3 given) + UNIQUE + nested REQUIRED
			map[string]any{"name": "r1"},
			map[string]any{"name": "r1"},
			map[string]any{"weight": int64(2)},
		},
		"logging":       map[string]any{"collector": true, "junk": int64(1)},  // UNKNOWN_FIELD (strict object)
		"tablespaces":   map[string]any{"bad": map[string]any{}},              // nested REQUIRED
		"backup":        map[string]any{"type": "tape"},                       // UNKNOWN_VARIANT
		"data_volume":   map[string]any{"path": "/data", "size_gb": int64(0)}, // GT via def
		"garbage":       int64(1),                                             // UNKNOWN_FIELD (strict root)
		"endpoint_pair": []any{"", "not-a-port"},                              // tuple: MIN_LEN + TYPE_MISMATCH
	}
}

// stableJSON reformats protojson output deterministically (protojson inserts
// random whitespace by design).
func stableJSON(t *testing.T, m proto.Message) []byte {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

// checkGolden writes (with -update) or compares a golden file.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(goldenDir, name)
	if *update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing (run: go test -run Golden -update ./...): %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden %s drifted; run go test -run Golden -update ./... and review the diff", name)
	}
}

func TestGoldenFullSchema(t *testing.T) {
	s := goldenSchema(t)
	checkGolden(t, "full-schema.json", stableJSON(t, s))

	// The schema must survive a wire round-trip unchanged.
	wire, err := proto.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back schemapb.Schema
	if err := proto.Unmarshal(wire, &back); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(s, &back) {
		t.Fatal("kitchen-sink schema does not round-trip")
	}
}

func TestGoldenBaked(t *testing.T) {
	s := goldenSchema(t)
	eng := goldenEngine(t, s)
	baked, res, err := eng.Bake(validInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocking() {
		t.Fatalf("valid input must bake: %v", res.GetErrors())
	}
	checkGolden(t, "full-baked.json", stableJSON(t, baked.GetValues()))

	rendered, err := baked.Render("conf")
	if err != nil {
		t.Fatal(err)
	}
	report, err := baked.Render("report")
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "full-rendered.txt", []byte(rendered+"---\n"+report))
}

func TestGoldenMessages(t *testing.T) {
	tmpl := schemapb.MessageTemplates()
	names := make(map[string]string, len(tmpl))
	for code, text := range tmpl {
		names[code.String()] = text
	}
	raw, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "messages.json", append(raw, '\n'))
}

func TestGoldenValidationErrors(t *testing.T) {
	s := goldenSchema(t)
	eng := goldenEngine(t, s)
	res := eng.Validate(brokenInput())
	if !res.Blocking() {
		t.Fatal("broken input must block")
	}
	checkGolden(t, "full-errors.json", stableJSON(t, res))
}
