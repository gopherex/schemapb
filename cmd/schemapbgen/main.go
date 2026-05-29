package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/emit"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/parse"
	"github.com/stroppy-io/schemapb/schemapb"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		in         []string
		out        string
		pkg        string
		fromGoCode string
		symbol     string
		recursive  bool
		names      string
	)
	cmd := &cobra.Command{
		Use:   "schemapbgen",
		Short: "Generate typed Go structs from schemapb schemas",
		RunE: func(_ *cobra.Command, _ []string) error {
			switch names {
			case "", "func", "identity":
			default:
				return fmt.Errorf("--names must be 'func' or 'identity', got %q", names)
			}
			if fromGoCode != "" {
				return runFromGoCode(fromGoCode, symbol, pkg, out, recursive, names)
			}
			return runFromJSON(in, pkg, out)
		},
	}
	cmd.Flags().StringSliceVar(&in, "in", nil, "input schema JSON file(s)")
	cmd.Flags().StringVar(&out, "out", "", "output file (JSON, single input) or directory (Go-code mode)")
	cmd.Flags().StringVar(&pkg, "pkg", "", "package name of generated code (JSON mode: required; Go-code mode: defaults to the source package)")
	cmd.Flags().StringVar(&fromGoCode, "from-go-code", "", "package directory holding schema provider func(s)")
	cmd.Flags().StringVar(&symbol, "symbol", "", "a specific provider func; omit to auto-discover all providers in the package")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "with --from-go-code, also generate for sub-packages")
	cmd.Flags().StringVar(&names, "names", "func", "Go-code mode type naming: 'func' (provider name + Schema) or 'identity' (schema identity)")
	return cmd
}

// runFromJSON implements the protojson input path (-in).
func runFromJSON(in []string, pkg, out string) error {
	if pkg == "" {
		return fmt.Errorf("--pkg is required")
	}
	if len(in) == 0 {
		return fmt.Errorf("--in is required when --from-go-code is not set")
	}
	for _, path := range in {
		s, err := parse.FromJSONFile(path)
		if err != nil {
			return err
		}
		f, err := model.Build(s, pkg)
		if err != nil {
			return err
		}
		src, err := emit.Emit(f)
		if err != nil {
			return err
		}
		dst := out
		if dst == "" || len(in) > 1 {
			dst = filepath.Join(filepath.Dir(path), f.Root+"_gen.go")
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", dst)
	}
	return nil
}

// runFromGoCode implements the Go-builder path (--from-go-code): discover (or
// take) provider funcs, run them, and emit one <source>_gen.go per source file.
func runFromGoCode(dir, symbol, pkgOverride, out string, recursive bool, names string) error {
	dirs := []string{dir}
	if recursive {
		extra, err := goDirs(dir)
		if err != nil {
			return err
		}
		dirs = extra
	}
	var generated int
	for _, d := range dirs {
		n, err := generateDir(d, symbol, pkgOverride, out, names, recursive)
		if err != nil {
			return err
		}
		generated += n
	}
	if generated == 0 {
		return fmt.Errorf("no schema providers found (exported func() *schemapb.Schema or []*schemapb.Schema)")
	}
	return nil
}

// generateDir generates for one package directory; returns the schema count.
func generateDir(dir, symbol, pkgOverride, out, names string, recursive bool) (int, error) {
	pkgName, providers, err := parse.Discover(dir)
	if err != nil {
		return 0, err
	}
	if symbol != "" {
		providers = filterSymbol(providers, symbol)
	}
	if len(providers) == 0 {
		return 0, nil
	}

	funcs := make([]string, len(providers))
	for i, p := range providers {
		funcs[i] = p.Func
	}
	byFunc, err := parse.RunProviders(dir, funcs)
	if err != nil {
		return 0, err
	}

	pkg := pkgName
	if pkgOverride != "" && !recursive {
		pkg = pkgOverride
	}

	// Group generated files by output path (one file per source file).
	type group struct {
		files []*model.File
		dst   string
	}
	groups := map[string]*group{}
	var order []string
	count := 0
	for _, p := range providers {
		schemas := byFunc[p.Func]
		multiple := len(schemas) > 1
		for i, s := range schemas {
			root := resolveRoot(p, s, names, multiple)
			f, err := model.BuildNamed(s, pkg, root)
			if err != nil {
				return 0, fmt.Errorf("%s: %w", p.Func, err)
			}
			dst := filepath.Join(dir, outBase(p.File, f.Root)+"_gen.go")
			g := groups[dst]
			if g == nil {
				g = &group{dst: dst}
				groups[dst] = g
				order = append(order, dst)
			}
			g.files = append(g.files, f)
			count++
			_ = i
		}
	}

	for _, dst := range order {
		g := groups[dst]
		src, err := emit.EmitMulti(g.files)
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			return 0, err
		}
		fmt.Printf("wrote %s\n", dst)
	}
	return count, nil
}

// resolveRoot picks the generated root type name for a schema.
func resolveRoot(p parse.Provider, s *schemapb.Schema, names string, multiple bool) string {
	if p.TypeName != "" {
		base := p.TypeName
		if multiple {
			base += model.RootName(s.GetId())
		}
		return base
	}
	if names == "identity" {
		return "" // model derives from the schema identity
	}
	if multiple {
		return p.Func + model.RootName(s.GetId())
	}
	return p.Func + "Schema"
}

// outBase returns the generated file's base name: the source file's name
// without .go (e.g. "postgres"), or the root type name when the source file is
// unknown (explicit --symbol that was not discovered).
func outBase(srcFile, root string) string {
	if srcFile == "" {
		return root
	}
	return strings.TrimSuffix(srcFile, ".go")
}

// filterSymbol keeps only the named provider, or synthesizes one if discovery
// missed it (so an explicit --symbol still runs).
func filterSymbol(providers []parse.Provider, symbol string) []parse.Provider {
	for _, p := range providers {
		if p.Func == symbol {
			return []parse.Provider{p}
		}
	}
	return []parse.Provider{{Func: symbol}}
}

// goDirs returns root and every subdirectory containing Go files, skipping
// vendor, hidden, and testdata directories.
func goDirs(root string) ([]string, error) {
	seen := map[string]bool{}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") && !strings.HasSuffix(d.Name(), "_gen.go") {
			dir := filepath.Dir(path)
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
		return nil
	})
	return dirs, err
}
