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

// FromGoCode runs a single provider symbol and returns its schemas.
func FromGoCode(dir, symbol string) ([]*schemapb.Schema, error) {
	byFunc, err := RunProviders(dir, []string{symbol})
	if err != nil {
		return nil, err
	}
	return byFunc[symbol], nil
}

// RunProviders compiles and runs the named provider functions in the package at
// dir, returning each function's schemas keyed by function name. Order within a
// function is preserved.
//
// dir may be any package directory, not a module root — it is resolved to its
// enclosing module (the nearest ancestor with a go.mod) and to the package's
// full import path. A temporary runner is written under the module root (so it
// can import internal/ packages) and executed with `go run`, resolving deps via
// the user module's own go.mod. The user's code must compile.
func RunProviders(dir string, funcs []string) (map[string][]*schemapb.Schema, error) {
	if len(funcs) == 0 {
		return map[string][]*schemapb.Schema{}, nil
	}
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

	var calls bytes.Buffer
	for _, fn := range funcs {
		fmt.Fprintf(&calls, "\t\t{%q, func() []*schemapb.Schema { return norm(userpkg.%s()) }},\n", fn, fn)
	}

	runner := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
	userpkg %q
)

func norm(v any) []*schemapb.Schema {
	switch x := v.(type) {
	case *schemapb.Schema:
		return []*schemapb.Schema{x}
	case []*schemapb.Schema:
		return x
	default:
		panic("provider must return *schemapb.Schema or []*schemapb.Schema")
	}
}

func main() {
	providers := []struct {
		name string
		fn   func() []*schemapb.Schema
	}{
%s	}
	for _, p := range providers {
		for _, s := range p.fn() {
			b, err := protojson.Marshal(s)
			if err != nil {
				panic(err)
			}
			line, err := json.Marshal(struct {
				F string          %sjson:"f"%s
				S json.RawMessage %sjson:"s"%s
			}{p.name, b})
			if err != nil {
				panic(err)
			}
			fmt.Println(string(line))
		}
	}
}
`, pkgImport, calls.String(), "`", "`", "`", "`")

	if err := os.WriteFile(filepath.Join(runnerDir, "main.go"), []byte(runner), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = runnerDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run providers in %s: %w\n%s", pkgImport, err, stderr.String())
	}

	out := map[string][]*schemapb.Schema{}
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var rec struct {
			F string          `json:"f"`
			S json.RawMessage `json:"s"`
		}
		if err := dec.Decode(&rec); err != nil {
			return nil, err
		}
		var s schemapb.Schema
		if err := protojson.Unmarshal(rec.S, &s); err != nil {
			return nil, err
		}
		out[rec.F] = append(out[rec.F], &s)
	}
	return out, nil
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
