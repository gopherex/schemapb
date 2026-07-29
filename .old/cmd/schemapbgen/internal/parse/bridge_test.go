package parse

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// tidy populates the synthetic module's go.sum so the read-only bridge can
// resolve deps (a real consumer project already has a complete go.sum).
func tidy(t *testing.T, modRoot string) {
	t.Helper()
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = modRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
}

func TestBridgeRunsProvider(t *testing.T) {
	dir := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..")
	pkg := `package prov

import "github.com/stroppy-io/schemapb/schemapb"

func Provide() *schemapb.Schema {
	return schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
}
`
	os.WriteFile(filepath.Join(dir, "prov.go"), []byte(pkg), 0o644)
	gomod := "module prov\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)
	tidy(t, dir)

	schemas, err := FromGoCode(dir, "Provide")
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 || schemas[0].GetId().GetName() != "disk" {
		t.Fatalf("got %+v", schemas)
	}
}

// TestBridgeSubpackage locks resolving a provider that lives in a nested
// internal/ package, not at the module root — the bridge must walk up to the
// go.mod and import the package by its full path.
func TestBridgeSubpackage(t *testing.T) {
	root := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..")
	gomod := "module example.com/app\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(root, "internal", "databases")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prov := `package databases

import "github.com/stroppy-io/schemapb/schemapb"

func PostgresInputSchema() *schemapb.Schema {
	return schemapb.NewSchema("databases", "postgres", "v1").Fields(
		schemapb.Int64("port").Default(5432),
	).MustBuild()
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "postgres.go"), []byte(prov), 0o644); err != nil {
		t.Fatal(err)
	}
	tidy(t, root)

	schemas, err := FromGoCode(pkgDir, "PostgresInputSchema")
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 || schemas[0].GetId().GetName() != "postgres" {
		t.Fatalf("got %+v", schemas)
	}
}
