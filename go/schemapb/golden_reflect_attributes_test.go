package schemapb_test

import (
	"testing"
	"time"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

// Other ports express these same attributes through native type overrides.
// The Go tag grammar is an adapter to the existing schema contract.
type attributeMirror struct {
	Token   string        `json:"token"   schemapb:"secret=true;default=token;min_len=1"`
	Workers int32         `json:"workers" schemapb:"default=4;gte=1;lte=64"`
	Enabled bool          `json:"enabled" schemapb:"default=false"`
	Timeout time.Duration `json:"timeout" schemapb:"default=30s"`
	Empty   *string       `json:"empty"   schemapb:"default="`
}

func TestGoldenReflectAttributes(t *testing.T) {
	t.Parallel()

	s, err := sp.ReflectType[attributeMirror](sp.ID("conformance", "reflect_attributes", sp.Ver(1, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}

	b, r, err := s.Bake(map[string]any{})
	if err != nil || !r.Ok() {
		t.Fatalf("bake: %v %v", err, r)
	}

	var got attributeMirror
	if err := b.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Token != "token" || got.Workers != 4 || got.Enabled || got.Timeout != 30*time.Second || got.Empty == nil || *got.Empty != "" {
		t.Fatal("default values differ")
	}

	checkGolden(t, "reflect-attributes.json", stableJSON(t, s))
	checkGolden(t, "reflect-attributes-baked.json", stableJSON(t, b.GetValues()))
}
