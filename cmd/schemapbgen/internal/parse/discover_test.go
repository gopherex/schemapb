package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	src := `package databases

import (
	sp "github.com/stroppy-io/schemapb/schemapb"
)

// PostgresInputSchema is a provider.
func PostgresInputSchema() *sp.Schema { return nil }

// Many returns several schemas.
func Many() []*sp.Schema { return nil }

//schemapbgen:skip
func Helper() *sp.Schema { return nil }

//schemapbgen:name CustomName
func WithMarker() *sp.Schema { return nil }

// not a provider: has params
func WithParams(x int) *sp.Schema { return nil }

// not a provider: wrong return
func WrongReturn() int { return 0 }

// unexported
func provider() *sp.Schema { return nil }
`
	if err := os.WriteFile(filepath.Join(dir, "postgres.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// A generated file must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "x_gen.go"), []byte("package databases\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pkg, providers, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "databases" {
		t.Errorf("pkg=%q want databases", pkg)
	}

	got := map[string]Provider{}
	for _, p := range providers {
		got[p.Func] = p
	}
	if len(got) != 3 {
		t.Fatalf("want 3 providers (Many, PostgresInputSchema, WithMarker), got %d: %v", len(got), keys(got))
	}
	if _, ok := got["Helper"]; ok {
		t.Error("Helper should be skipped (//schemapbgen:skip)")
	}
	if _, ok := got["WithParams"]; ok {
		t.Error("WithParams (has params) must not be a provider")
	}
	if _, ok := got["WrongReturn"]; ok {
		t.Error("WrongReturn must not be a provider")
	}
	if p := got["Many"]; !p.Slice {
		t.Error("Many should be detected as slice-returning")
	}
	if p := got["PostgresInputSchema"]; p.Slice || p.File != "postgres.go" {
		t.Errorf("PostgresInputSchema: slice=%v file=%q", p.Slice, p.File)
	}
	if p := got["WithMarker"]; p.TypeName != "CustomName" {
		t.Errorf("WithMarker type name override = %q want CustomName", p.TypeName)
	}
}

func keys(m map[string]Provider) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
