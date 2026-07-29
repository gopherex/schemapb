package schemapb_test

import (
	"testing"
	"time"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

func TestSmokeResolve(t *testing.T) {
	s := schemapb.NewSchema("infra", "pg", "v1").
		Fields(
			schemapb.Int64("shared_buffers").Gte(16).Default(128),
			schemapb.Computed("cache", "root.shared_buffers * 3").Result(schemapb.ResultInt64),
			schemapb.Str("mode").Default("fast").Normalize("this.upperAscii()"),
			schemapb.Duration("timeout").Default(5*time.Second),
			schemapb.Int64("hidden").When("root.shared_buffers > 1000").Default(1),
		).
		MustBuild()

	out, res, err := s.Resolve(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetErrors()) != 0 {
		t.Fatalf("resolve errors: %v", res.GetErrors())
	}
	if out["shared_buffers"] != int64(128) {
		t.Errorf("default: %v (%T)", out["shared_buffers"], out["shared_buffers"])
	}
	if out["cache"] != int64(384) {
		t.Errorf("computed: %v (%T)", out["cache"], out["cache"])
	}
	if out["mode"] != "FAST" {
		t.Errorf("normalize: %v", out["mode"])
	}
	if out["timeout"] != 5*time.Second {
		t.Errorf("duration default: %v (%T)", out["timeout"], out["timeout"])
	}
	if _, ok := out["hidden"]; ok {
		t.Errorf("when-gated field seeded: %v", out["hidden"])
	}
}

func TestSmokeValidate(t *testing.T) {
	s := schemapb.NewSchema("infra", "pg", "v1").
		Strict().
		Fields(
			schemapb.Int64("conns").Gte(16).Lte(100).Required(),
			schemapb.Str("pass").MinLen(8).Secret(),
			schemapb.Str("mail").Format(schemapb.FormatEmail),
			schemapb.Str("weird").Format("k8s.quantity"),
		).
		Rules(schemapb.Rule("int(root.conns) != 42", "not 42").ID("no42").Warn()).
		MustBuild()

	res, err := s.Validate(map[string]any{
		"conns": int64(8),
		"pass":  "short",
		"mail":  "not-an-email",
		"weird": "500Mi",
		"junk":  int64(1),
		
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[schemapb.ErrorCode]int{}
	for _, e := range res.GetErrors() {
		got[e.GetCode()]++
		if e.GetPath() == "pass" && e.GetActual() != nil {
			t.Errorf("secret actual not masked: %v", e)
		}
	}
	want := []schemapb.ErrorCode{
		schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED,
		schemapb.ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED,
		schemapb.ErrorCode_ERROR_CODE_FORMAT_MISMATCH,
		schemapb.ErrorCode_ERROR_CODE_UNSUPPORTED_FORMAT,
		schemapb.ErrorCode_ERROR_CODE_UNKNOWN_FIELD,
	}
	for _, w := range want {
		if got[w] == 0 {
			t.Errorf("missing code %v in %v", w, res.GetErrors())
		}
	}
	if !res.Blocking() {
		t.Error("expected blocking result")
	}

	// Valid input: only the warning rule fires; Bake succeeds. The custom
	// format is registered on an explicitly compiled engine.
	eng, err := schemapb.Compile(s, schemapb.WithFormats(schemapb.FormatRegistry{
		"k8s.quantity": func(v string) bool { return v != "" },
	}))
	if err != nil {
		t.Fatal(err)
	}
	baked, res2, err := eng.Bake(map[string]any{"conns": int64(42), "pass": "longenough", "mail": "a@b.co", "weird": "1Gi"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Blocking() {
		t.Fatalf("unexpected blocking: %v", res2.GetErrors())
	}
	if len(res2.GetErrors()) != 1 || res2.GetErrors()[0].GetCode() != schemapb.ErrorCode_ERROR_CODE_RULE_VIOLATED {
		t.Errorf("want single warning rule violation, got %v", res2.GetErrors())
	}
	if baked.GetValues().GetFields()["conns"].GetInt64Value() != 42 {
		t.Errorf("canonical int64 lost: %v", baked.GetValues())
	}
	if !baked.Matches(s) {
		t.Error("baked must match its schema")
	}
}

func TestSmokeRender(t *testing.T) {
	s := schemapb.NewSchema("infra", "pg", "v1").
		Fields(
			schemapb.Int64("shared_buffers").Default(128).Unit("MB").Group("Memory"),
			schemapb.Bool("autovacuum").Default(true).Group("Vacuum"),
			schemapb.Enum("wal_level").Values(map[int32]string{0: "minimal", 1: "replica"}).Default(1).Group("WAL"),
		).
		Template("conf", `{{#fields}}{{#set}}{{name}} = {{value}}{{#label}} # {{label}}{{/label}}
{{/set}}{{/fields}}`).
		MustBuild()

	baked, res, err := s.Bake(map[string]any{})
	if err != nil || res.Blocking() {
		t.Fatalf("bake: %v %v", err, res.GetErrors())
	}
	out, err := baked.Render("conf")
	if err != nil {
		t.Fatal(err)
	}
	want := "shared_buffers = 128\nautovacuum = true\nwal_level = 1 # replica\n"
	if out != want {
		t.Errorf("render:\n%q\nwant:\n%q", out, want)
	}
}
