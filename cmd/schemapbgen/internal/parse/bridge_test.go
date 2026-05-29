package parse

import (
	"os"
	"path/filepath"
	"testing"
)

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

	schemas, err := FromGoCode(dir, "Provide")
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 || schemas[0].GetId().GetName() != "disk" {
		t.Fatalf("got %+v", schemas)
	}
}
