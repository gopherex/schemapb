package schemapb_test

import (
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/schemapb"
)

const pgConfTmpl = `{{- range .Groups }}
# === {{ .Name }} ===
{{- range .Fields }}
{{- if .Description }}
# {{ .Description }}{{ end }}
{{ .Name }} = {{ .Value }}{{ .Unit }}
{{- end }}
{{ end -}}`

func TestRenderConf(t *testing.T) {
	s := schemapb.NewSchema("pg", "postgresql", "16").
		Template("conf", pgConfTmpl).
		Fields(
			schemapb.Int64("shared_buffers").Default(128).Unit("MB").
				Group("Resource Usage").Desc("shared memory for buffers"),
			schemapb.Int64("work_mem").Default(4).Unit("MB").Group("Resource Usage"),
			schemapb.Str("wal_level").In("minimal", "replica", "logical").Default("replica").Group("WAL"),
		).MustBuild()

	resolved, errs := s.Compute(map[string]any{})
	if len(errs) != 0 {
		t.Fatalf("compute: %v", errs)
	}

	out, err := s.Render("conf", resolved)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# === Resource Usage ===",
		"# shared memory for buffers",
		"shared_buffers = 128MB",
		"work_mem = 4MB",
		"# === WAL ===",
		"wal_level = replica",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered conf missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderBakedAndErrors(t *testing.T) {
	s := schemapb.NewSchema("t", "s", "v1").
		Template("kv", `{{ range .Fields }}{{ .Name }}={{ .Value }}
{{ end }}`).
		Fields(schemapb.Str("a").Default("x")).
		MustBuild()

	// unknown template -> error
	if _, err := s.Render("nope", nil); err == nil {
		t.Error("want error for unknown template")
	}

	baked, errs := s.Bake(map[string]any{"a": "y"})
	if len(errs) != 0 {
		t.Fatalf("bake: %v", errs)
	}
	out, err := baked.Render("kv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a=y") {
		t.Errorf("baked render: %q", out)
	}
}
