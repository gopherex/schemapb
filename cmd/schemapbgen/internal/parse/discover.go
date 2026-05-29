package parse

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const schemapbImport = "github.com/stroppy-io/schemapb/schemapb"

// Provider is a discovered schema-provider function.
type Provider struct {
	Func     string // function name, e.g. PostgresInputSchema
	File     string // base name of the source file, e.g. postgres.go
	TypeName string // explicit type name from a //schemapbgen:name marker, else ""
	Slice    bool   // returns []*schemapb.Schema (vs *schemapb.Schema)
}

// Discover scans the Go package in dir (non-recursively) for exported, zero-arg
// functions returning *schemapb.Schema or []*schemapb.Schema. It also returns
// the package name (for the generated file's package clause). Functions marked
// with a //schemapbgen:skip doc comment are excluded; //schemapbgen:name <Type>
// sets an explicit generated type name. Generated files (*_gen.go) and test
// files are ignored. Results are sorted by function name for stable output.
func Discover(dir string) (pkgName string, providers []Provider, err error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		n := fi.Name()
		return !strings.HasSuffix(n, "_test.go") && !strings.HasSuffix(n, "_gen.go")
	}, parser.ParseComments)
	if err != nil {
		return "", nil, err
	}

	var out []Provider
	for _, pkg := range pkgs {
		pkgName = pkg.Name
		for fileName, file := range pkg.Files {
			local := schemapbLocalName(file)
			if local == "" {
				continue // file does not import the schemapb package
			}
			base := filepath.Base(fileName)
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !fn.Name.IsExported() {
					continue
				}
				if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
					continue
				}
				slice, ok := returnsSchema(fn.Type.Results, local)
				if !ok {
					continue
				}
				skip, name := markers(fn.Doc)
				if skip {
					continue
				}
				out = append(out, Provider{Func: fn.Name.Name, File: base, TypeName: name, Slice: slice})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Func < out[j].Func })
	return pkgName, out, nil
}

// schemapbLocalName returns the local import name for the schemapb package in
// file (the alias, or "schemapb" by default), or "" if not imported.
func schemapbLocalName(file *ast.File) string {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != schemapbImport {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "schemapb"
	}
	return ""
}

// returnsSchema reports whether results is exactly one *local.Schema (slice
// false) or []*local.Schema (slice true).
func returnsSchema(results *ast.FieldList, local string) (slice, ok bool) {
	if results == nil || len(results.List) != 1 || len(results.List[0].Names) > 1 {
		return false, false
	}
	switch t := results.List[0].Type.(type) {
	case *ast.StarExpr:
		return false, isSchemaSelector(t.X, local)
	case *ast.ArrayType:
		if t.Len != nil {
			return false, false
		}
		if star, isStar := t.Elt.(*ast.StarExpr); isStar {
			return true, isSchemaSelector(star.X, local)
		}
	}
	return false, false
}

// isSchemaSelector reports whether expr is `local.Schema`.
func isSchemaSelector(expr ast.Expr, local string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Schema" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == local
}

// markers reads schemapbgen directives from a doc comment group.
func markers(doc *ast.CommentGroup) (skip bool, name string) {
	if doc == nil {
		return false, ""
	}
	for _, c := range doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		switch {
		case line == "schemapbgen:skip":
			skip = true
		case strings.HasPrefix(line, "schemapbgen:name "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "schemapbgen:name "))
		}
	}
	return skip, name
}
