package schemapb

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/cbroglie/mustache"
	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
)

// Engine is a compiled schema: every CEL expression (when / normalize /
// computed / options_expr / count_expr / rules) and every regex pattern is
// compiled exactly once, up front. Compile surfaces bad expressions as a
// *SchemaError instead of failing later at evaluation time.
//
// An Engine is immutable and safe for concurrent use.
type Engine struct {
	schema    *Schema
	progs     map[string]cel.Program
	asts      map[string]*cel.Ast
	regexps   map[string]*regexp.Regexp
	formats   FormatRegistry
	templates map[string]*mustache.Template
}

// celEnv is the single CEL environment of the spec: variables `this`, `root`,
// `index`; full CEL plus the strings extension; numeric comparisons work
// across int/uint/double.
//
//nolint:gochecknoglobals // one shared, immutable CEL environment per process
var celEnv = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("this", cel.DynType),
		cel.Variable("root", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("index", cel.IntType),
		ext.Strings(),
		cel.CrossTypeNumericComparisons(true),
	)
})

// Compile checks the descriptor, compiles every expression and pattern in the
// schema (including defs), and statically rejects top-level computed-field
// cycles. The returned Engine evaluates without further compilation.
//
//nolint:cyclop,funlen // one linear compile pipeline
func Compile(s *Schema, opts ...CompileOption) (*Engine, error) {
	if err := s.CheckDescriptor(); err != nil {
		return nil, err
	}

	env, err := celEnv()
	if err != nil {
		return nil, fmt.Errorf("schemapb: cel environment: %w", err)
	}

	cfg := compileConfig{formats: CoreFormats()}
	for _, opt := range opts {
		opt(&cfg)
	}

	e := &Engine{
		schema:    s,
		progs:     map[string]cel.Program{},
		asts:      map[string]*cel.Ast{},
		regexps:   map[string]*regexp.Regexp{},
		formats:   cfg.formats,
		templates: map[string]*mustache.Template{},
	}

	var progOpts []cel.ProgramOption
	if cfg.costLimit > 0 {
		progOpts = append(progOpts, cel.CostLimit(cfg.costLimit), cel.CostTracking(nil))
	}

	var errs []*ValidationError

	for src, path := range schemaExprs(s) {
		if _, done := e.progs[src]; done {
			continue
		}

		ast, iss := env.Compile(src)
		if iss.Err() != nil {
			errs = append(errs, schemaErr(path, fmt.Sprintf("cel: %v", iss.Err())))

			continue
		}

		prg, err := env.Program(ast, progOpts...)
		if err != nil {
			errs = append(errs, schemaErr(path, fmt.Sprintf("cel: %v", err)))

			continue
		}

		e.asts[src] = ast
		e.progs[src] = prg
	}

	for name, src := range s.GetTemplates() {
		tmpl, err := mustache.ParseString(src)
		if err != nil {
			errs = append(errs, schemaErr("templates."+name, fmt.Sprintf("mustache: %v", err)))

			continue
		}

		e.templates[name] = tmpl
	}

	for pattern, path := range schemaPatterns(s) {
		if _, done := e.regexps[pattern]; done {
			continue
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			errs = append(errs, schemaErr(path, fmt.Sprintf("pattern: %v", err)))

			continue
		}

		e.regexps[pattern] = re
	}

	if len(errs) == 0 {
		if err := e.checkComputedCycles(); err != nil {
			errs = append(errs, err...)
		}
	}

	if len(errs) > 0 {
		return nil, &SchemaError{Result: &ValidationResult{Errors: errs}}
	}

	return e, nil
}

// Schema returns the schema this engine was compiled from.
func (e *Engine) Schema() *Schema { return e.schema }

// engineCache backs the convenience methods on *Schema (s.Validate, s.Resolve,
// ...): one compiled engine per schema pointer.
//
//nolint:gochecknoglobals // process-wide compile cache is the point
var engineCache sync.Map // *Schema -> engineEntry

type engineEntry struct {
	engine *Engine
	err    error
}

// engine returns the cached compiled engine for s, compiling on first use.
// The cache is keyed by pointer: mutating a schema after first use is not
// supported (compile explicitly with Compile for that).
func (s *Schema) engine() (*Engine, error) {
	if v, ok := engineCache.Load(s); ok {
		if entry, isEntry := v.(engineEntry); isEntry {
			return entry.engine, entry.err
		}
	}

	eng, err := Compile(s)

	v, _ := engineCache.LoadOrStore(s, engineEntry{engine: eng, err: err})
	if entry, isEntry := v.(engineEntry); isEntry {
		return entry.engine, entry.err
	}

	return eng, err // unreachable: only engineEntry values are stored
}

// =============================================================================
// Expression / pattern collection
// =============================================================================

// schemaExprs yields every CEL expression source in the schema with a
// representative field path (for compile-error reporting). Duplicate sources
// yield once per occurrence; the compiler dedupes.
func schemaExprs(s *Schema) map[string]string {
	out := map[string]string{}
	add := func(src, path string) {
		if src != "" {
			if _, ok := out[src]; !ok {
				out[src] = path
			}
		}
	}

	var walkFields func(fields []*Schema_Field, prefix string)

	walkSchema := func(sub *Schema, prefix string) {
		for i, r := range sub.GetRules() {
			add(r.GetExpr(), fmt.Sprintf("%s#rule[%d]", prefix, i))
		}
	}
	walkFields = func(fields []*Schema_Field, prefix string) {
		for _, f := range fields {
			path := joinPath(prefix, f.GetName())
			add(f.GetWhen(), path+"#when")
			add(f.GetNormalize(), path+"#normalize")

			for i, r := range f.GetRules() {
				add(r.GetExpr(), fmt.Sprintf("%s#rule[%d]", path, i))
			}

			if c := f.GetComputed(); c != nil {
				add(c.GetExpr(), path+"#computed")
			}

			if ch := f.GetChoice(); ch != nil {
				add(ch.GetOptionsExpr(), path+"#options")
			}

			if l := f.GetList(); l != nil {
				add(l.GetCountExpr(), path+"#count")
				walkFields(l.GetItems(), path+"[]")
			}

			for _, child := range nestedSchemas(f) {
				walkSchema(child, path)
				walkFields(child.GetFields(), path)
			}
		}
	}

	walkSchema(s, "")
	walkFields(s.GetFields(), "")

	for name, def := range s.GetDefs() {
		walkSchema(def, "$defs."+name)
		walkFields(def.GetFields(), "$defs."+name)
	}

	return out
}

// schemaPatterns yields every RE2 pattern in the schema with a field path.
func schemaPatterns(s *Schema) map[string]string {
	out := map[string]string{}

	var walkFields func(fields []*Schema_Field, prefix string)
	walkFields = func(fields []*Schema_Field, prefix string) {
		for _, f := range fields {
			path := joinPath(prefix, f.GetName())

			if str := f.GetString_(); str != nil && str.GetPattern() != "" {
				if _, ok := out[str.GetPattern()]; !ok {
					out[str.GetPattern()] = path + "#pattern"
				}
			}

			if l := f.GetList(); l != nil {
				walkFields(l.GetItems(), path+"[]")
			}

			for _, child := range nestedSchemas(f) {
				walkFields(child.GetFields(), path)
			}
		}
	}
	walkFields(s.GetFields(), "")

	for name, def := range s.GetDefs() {
		walkFields(def.GetFields(), "$defs."+name)
	}

	return out
}

// =============================================================================
// Evaluation
// =============================================================================

// eval runs a compiled expression with the given bindings and returns the
// result in the native value model.
func (e *Engine) eval(src string, vars map[string]any) (any, error) {
	prg := e.progs[src]
	if prg == nil {
		return nil, fmt.Errorf("expression not compiled: %s", src)
	}

	out, _, err := prg.Eval(vars)
	if err != nil {
		return nil, fmt.Errorf("cel: %w", err)
	}

	return celToNative(out), nil
}

// evalBool runs a compiled expression and requires a boolean result.
func (e *Engine) evalBool(src string, vars map[string]any) (bool, error) {
	res, err := e.eval(src, vars)
	if err != nil {
		return false, err
	}

	b, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf("expression yields %T, want bool", res)
	}

	return b, nil
}

// celToNative converts a CEL value into the native value model.
func celToNative(v ref.Val) any {
	switch t := v.(type) {
	case types.Bool:
		return bool(t)
	case types.Int:
		return int64(t)
	case types.Uint:
		return uint64(t)
	case types.Double:
		return float64(t)
	case types.String:
		return string(t)
	case types.Bytes:
		return []byte(t)
	case types.Duration:
		return t.Duration
	case types.Timestamp:
		return t.Time
	case types.Null:
		return nil
	}

	if l, ok := v.(traits.Lister); ok {
		var out []any

		it := l.Iterator()
		for it.HasNext() == types.True {
			out = append(out, celToNative(it.Next()))
		}

		return out
	}

	if m, ok := v.(traits.Mapper); ok {
		out := map[string]any{}

		it := m.Iterator()
		for it.HasNext() == types.True {
			k := it.Next()
			ks, isStr := celToNative(k).(string)

			if !isStr {
				ks = fmt.Sprint(celToNative(k))
			}

			out[ks] = celToNative(m.Get(k))
		}

		return out
	}

	return v.Value()
}

// =============================================================================
// Dependency extraction (computed ordering, cycle detection)
// =============================================================================

// exprDeps returns the dotted root paths a compiled expression reads
// (root.a -> "a", root.addr.zip -> "addr.zip", root["a"] -> "a").
//
//nolint:cyclop,funlen // flat exhaustive CEL AST-kind dispatch
func (e *Engine) exprDeps(src string) []string {
	a := e.asts[src]
	if a == nil {
		return nil
	}

	var deps []string

	var walk func(x celast.Expr)
	walk = func(x celast.Expr) {
		if x == nil {
			return
		}

		if p, ok := selectPath(x); ok {
			if p != "" {
				deps = append(deps, p)
			}

			return
		}

		switch x.Kind() {
		case celast.SelectKind:
			walk(x.AsSelect().Operand())
		case celast.CallKind:
			c := x.AsCall()
			if c.IsMemberFunction() {
				walk(c.Target())
			}

			for _, a := range c.Args() {
				walk(a)
			}
		case celast.ListKind:
			for _, el := range x.AsList().Elements() {
				walk(el)
			}
		case celast.MapKind:
			for _, en := range x.AsMap().Entries() {
				me := en.AsMapEntry()
				walk(me.Key())
				walk(me.Value())
			}
		case celast.ComprehensionKind:
			cp := x.AsComprehension()
			walk(cp.IterRange())
			walk(cp.AccuInit())
			walk(cp.LoopCondition())
			walk(cp.LoopStep())
			walk(cp.Result())
		case celast.StructKind:
			for _, f := range x.AsStruct().Fields() {
				walk(f.AsStructField().Value())
			}
		default: // idents and literals carry no root selections
		}
	}
	walk(a.NativeRep().Expr())

	return deps
}

// selectPath resolves a select/index chain rooted at the `root` identifier to
// a dotted path ("" for `root` itself; false when not rooted at `root`).
func selectPath(x celast.Expr) (string, bool) {
	switch x.Kind() {
	case celast.IdentKind:
		if x.AsIdent() == "root" {
			return "", true
		}

		return "", false
	case celast.SelectKind:
		sel := x.AsSelect()

		base, ok := selectPath(sel.Operand())
		if !ok {
			return "", false
		}

		if base == "" {
			return sel.FieldName(), true
		}

		return base + "." + sel.FieldName(), true
	case celast.CallKind:
		c := x.AsCall()
		if c.FunctionName() != "_[_]" || len(c.Args()) != 2 {
			return "", false
		}

		base, ok := selectPath(c.Args()[0])
		if !ok {
			return "", false
		}

		lit := c.Args()[1]
		if lit.Kind() != celast.LiteralKind {
			return "", false
		}

		key, ok := lit.AsLiteral().Value().(string)
		if !ok {
			return "", false
		}

		if base == "" {
			return key, true
		}

		return base + "." + key, true
	default:
		return "", false
	}
}

// checkComputedCycles statically rejects dependency cycles between top-level
// Computed fields (runtime resolution handles nested scopes).
func (e *Engine) checkComputedCycles() []*ValidationError {
	computed := map[string]*Schema_Field{}

	var names []string

	for _, f := range e.schema.GetFields() {
		if f.GetComputed() != nil {
			computed[f.GetName()] = f
			names = append(names, f.GetName())
		}
	}

	if len(computed) == 0 {
		return nil
	}

	deps := map[string][]string{}

	for name, f := range computed {
		for _, d := range e.exprDeps(f.GetComputed().GetExpr()) {
			if d != name {
				if _, ok := computed[d]; ok {
					deps[name] = append(deps[name], d)
				}
			}
		}
	}

	const (
		white = iota
		gray
		black
	)

	color := map[string]int{}

	var errs []*ValidationError

	var visit func(string) bool
	visit = func(n string) bool {
		switch color[n] {
		case gray:
			return false
		case black:
			return true
		}

		color[n] = gray

		for _, d := range deps[n] {
			if !visit(d) {
				return false
			}
		}

		color[n] = black

		return true
	}

	for _, n := range names {
		if color[n] == black {
			continue
		}

		if !visit(n) {
			errs = append(errs, schemaErr(n, "computed field cycle"))
		}
	}

	return errs
}
