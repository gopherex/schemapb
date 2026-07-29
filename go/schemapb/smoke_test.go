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
