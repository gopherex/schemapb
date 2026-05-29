package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/emit"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/parse"
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
	)
	cmd := &cobra.Command{
		Use:   "schemapbgen",
		Short: "Generate typed Go structs from schemapb schemas",
		RunE: func(_ *cobra.Command, _ []string) error {
			if pkg == "" {
				return fmt.Errorf("-pkg is required")
			}
			if fromGoCode != "" {
				schemas, err := parse.FromGoCode(fromGoCode, symbol)
				if err != nil {
					return err
				}
				for _, s := range schemas {
					f, err := model.Build(s, pkg)
					if err != nil {
						return err
					}
					src, err := emit.Emit(f)
					if err != nil {
						return err
					}
					// Default: write next to the provider (same package dir), so
					// the generated type lives in the provider's package. -out, if
					// given, is treated as the output directory.
					outDir := fromGoCode
					if out != "" {
						outDir = out
					}
					dst := filepath.Join(outDir, f.Root+"_gen.go")
					if err := os.WriteFile(dst, src, 0o644); err != nil {
						return err
					}
					fmt.Printf("wrote %s\n", dst)
				}
				return nil
			}
			if len(in) == 0 {
				return fmt.Errorf("-in is required when -from-go-code is not set")
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
		},
	}
	cmd.Flags().StringSliceVar(&in, "in", nil, "input schema JSON file(s)")
	cmd.Flags().StringVar(&out, "out", "", "output Go file (single input only)")
	cmd.Flags().StringVar(&pkg, "pkg", "", "package name of generated code")
	cmd.Flags().StringVar(&fromGoCode, "from-go-code", "", "dir of a Go module exposing a schema provider func")
	cmd.Flags().StringVar(&symbol, "symbol", "", "exported provider func returning *schemapb.Schema or []*schemapb.Schema")
	return cmd
}
