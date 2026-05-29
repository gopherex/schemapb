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

// FromGoCode compiles+runs the user's module at dir, calling the exported symbol
// (a func returning *schemapb.Schema or []*schemapb.Schema), and returns the
// schemas. The user's code must compile.
func FromGoCode(dir, symbol string) ([]*schemapb.Schema, error) {
	pkgPath, err := modulePath(dir)
	if err != nil {
		return nil, err
	}
	runnerDir, err := os.MkdirTemp(dir, "schemapbgen-runner-")
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
`, pkgPath, symbol)

	if err := os.WriteFile(filepath.Join(runnerDir, "main.go"), []byte(runner), 0o644); err != nil {
		return nil, err
	}

	// Run `go mod tidy` in the parent (user) directory so the runner can resolve deps.
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = runnerDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run provider: %w\n%s", err, stderr.String())
	}

	var schemas []*schemapb.Schema
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var rawObj json.RawMessage
		if err := dec.Decode(&rawObj); err != nil {
			return nil, err
		}
		var s schemapb.Schema
		if err := protojson.Unmarshal(rawObj, &s); err != nil {
			return nil, err
		}
		schemas = append(schemas, &s)
	}
	return schemas, nil
}

// modulePath returns the module path declared in dir/go.mod so the runner can
// import the user package.
func modulePath(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("module ")) {
			return string(bytes.TrimSpace(line[len("module "):])), nil
		}
	}
	return "", fmt.Errorf("no module path in %s/go.mod", dir)
}
