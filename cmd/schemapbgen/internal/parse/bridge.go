package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
)

// FromGoCode runs the exported provider symbol in the package at dir (a func
// returning *schemapb.Schema or []*schemapb.Schema) and returns the schemas.
//
// dir may be any package directory, not a module root — it is resolved to its
// enclosing module (the nearest ancestor with a go.mod) and to the package's
// full import path. A temporary runner is written under the module root (so it
// can import internal/ packages) and executed with `go run`, resolving deps via
// the user module's own go.mod. The user's code must compile.
func FromGoCode(dir, symbol string) ([]*schemapb.Schema, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if fi, serr := os.Stat(abs); serr != nil || !fi.IsDir() {
		return nil, fmt.Errorf("-from-go-code: %q is not a directory (check the path; it must point at the package holding the provider func)", dir)
	}
	modRoot, modPath, err := findModule(abs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(modRoot, abs)
	if err != nil {
		return nil, err
	}
	pkgImport := modPath
	if rel != "." {
		pkgImport = modPath + "/" + filepath.ToSlash(rel)
	}

	runnerDir, err := os.MkdirTemp(modRoot, ".schemapbgen-runner-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(runnerDir)

	runner := fmt.Sprintf(`package main

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
	userpkg %q
)

func main() {
	out := []*schemapb.Schema{}
	switch v := any(userpkg.%s()).(type) {
	case *schemapb.Schema:
		out = append(out, v)
	case []*schemapb.Schema:
		out = append(out, v...)
	default:
		panic("symbol must return *schemapb.Schema or []*schemapb.Schema")
	}
	for _, s := range out {
		b, err := protojson.Marshal(s)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%%s\n", b)
	}
}
`, pkgImport, symbol)

	if err := os.WriteFile(filepath.Join(runnerDir, "main.go"), []byte(runner), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = runnerDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run provider %s.%s: %w\n%s", pkgImport, symbol, err, stderr.String())
	}

	var schemas []*schemapb.Schema
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		var s schemapb.Schema
		if err := protojson.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		schemas = append(schemas, &s)
	}
	return schemas, nil
}

// findModule walks up from dir to the nearest go.mod, returning its directory
// and declared module path.
func findModule(dir string) (root, modPath string, err error) {
	for d := dir; ; {
		b, rerr := os.ReadFile(filepath.Join(d, "go.mod"))
		if rerr == nil {
			for _, line := range bytes.Split(b, []byte("\n")) {
				line = bytes.TrimSpace(line)
				if bytes.HasPrefix(line, []byte("module ")) {
					return d, string(bytes.TrimSpace(line[len("module "):])), nil
				}
			}
			return "", "", fmt.Errorf("no module path in %s", filepath.Join(d, "go.mod"))
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("no go.mod found in %s or any parent directory", dir)
		}
		d = parent
	}
}
