package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

// TestGoldenCompiles generates code from disk.json, drops it into a temp module
// that requires the library via replace, and runs `go build` to prove the
// generated code compiles against the real runtime.
func TestGoldenCompiles(t *testing.T) {
	raw, err := os.ReadFile("testdata/disk.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	f, err := model.Build(&s, "gen")
	if err != nil {
		t.Fatal(err)
	}
	src, err := Emit(f)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..") // cmd/schemapbgen/internal/emit -> repo root
	must(t, os.WriteFile(filepath.Join(dir, "gen.go"), src, 0o644))
	gomod := "module gen\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

	run(t, dir, "go", "mod", "tidy")
	run(t, dir, "go", "build", "./...")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
