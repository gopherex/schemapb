package schemapb

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"google.golang.org/protobuf/types/known/structpb"
)

// Compute resolves form against the schema: it fills unset fields from their
// defaults, evaluates the Computed (derived) fields in dependency order, and
// returns the complete resolved form (inputs + defaults + derived). The
// returned errors report expressions that failed (missing referenced value) or
// a computed-field cycle. form is mutated in place and also returned.
func (v *Validator) Compute(form map[string]any) (map[string]any, []*FieldError) {
	errs := v.resolve(form)
	return form, errs
}

// ComputeStruct resolves a google.protobuf.Struct input (see Compute).
func (v *Validator) ComputeStruct(s *structpb.Struct) (map[string]any, []*FieldError) {
	m := map[string]any{}
	if s != nil {
		m = s.AsMap()
	}
	return v.Compute(m)
}

// ComputeJSON resolves a raw JSON object (see Compute). It errors only if the
// JSON cannot be parsed into an object.
func (v *Validator) ComputeJSON(raw json.RawMessage) (map[string]any, []*FieldError, error) {
	m := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, nil, fmt.Errorf("parse json: %w", err)
		}
	}
	out, errs := v.Compute(m)
	return out, errs, nil
}

// resolve seeds defaults and evaluates Computed fields across the whole schema
// tree, mutating form into the fully resolved state.
func (v *Validator) resolve(form map[string]any) []*FieldError {
	var tasks []computeTask
	v.seed(v.schema.GetFields(), form, "", &tasks)
	return v.runCompute(form, tasks)
}

// computeTask is a Computed field pending evaluation: it reads from root
// (shared) and writes its result to scope[field.Name].
type computeTask struct {
	field *Schema_Filed
	scope map[string]any
	path  string
}

// seed fills defaults for unset fields and collects Computed fields as tasks,
// recursing into present objects and object-typed list elements. It never
// materialises an absent object, so optional sub-forms stay absent (and don't
// spuriously trip their children's "required" checks).
func (v *Validator) seed(fields []*Schema_Filed, scope map[string]any, prefix string, tasks *[]computeTask) {
	for _, f := range fields {
		name := f.GetName()
		path := join(prefix, name)

		if _, ok := scope[name]; !ok {
			if dv, ok := defaultValue(f); ok {
				scope[name] = dv
			}
		}

		switch {
		case f.GetComputed() != nil:
			*tasks = append(*tasks, computeTask{field: f, scope: scope, path: path})
		case f.GetObject() != nil && f.GetObject().GetSchema() != nil:
			if child, ok := scope[name].(map[string]any); ok {
				v.seed(f.GetObject().GetSchema().GetFields(), child, path, tasks)
			}
		case f.GetList() != nil && len(f.GetList().GetItems()) >= 1:
			if o := f.GetList().GetItems()[0].GetObject(); o != nil && o.GetSchema() != nil {
				if arr, ok := scope[name].([]any); ok {
					for i, el := range arr {
						if m, ok := el.(map[string]any); ok {
							v.seed(o.GetSchema().GetFields(), m, fmt.Sprintf("%s[%d]", path, i), tasks)
						}
					}
				}
			}
		}
	}
}

// runCompute evaluates tasks in dependency order (the root paths each
// expression reads), writing each result into its scope. Cycles are reported
// and their fields left unevaluated.
func (v *Validator) runCompute(root map[string]any, tasks []computeTask) []*FieldError {
	if len(tasks) == 0 {
		return nil
	}
	byPath := make(map[string]computeTask, len(tasks))
	for _, t := range tasks {
		byPath[t.path] = t
	}
	deps := map[string][]string{}
	for _, t := range tasks {
		for _, d := range exprDeps(t.field.GetComputed().GetExpr()) {
			if d != t.path {
				if _, ok := byPath[d]; ok {
					deps[t.path] = append(deps[t.path], d)
				}
			}
		}
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var order []computeTask
	var out []*FieldError
	var visit func(string) bool
	visit = func(p string) bool {
		switch color[p] {
		case gray:
			return false // cycle
		case black:
			return true
		}
		color[p] = gray
		for _, d := range deps[p] {
			if !visit(d) {
				return false
			}
		}
		color[p] = black
		order = append(order, byPath[p])
		return true
	}
	for _, t := range tasks {
		if color[t.path] == black {
			continue
		}
		if !visit(t.path) {
			out = append(out, schemaErr(t.path, "computed field cycle"))
		}
	}

	for _, t := range order {
		c := t.field.GetComputed()
		prg := v.programs[c.GetExpr()]
		if prg == nil {
			continue
		}
		res, err := expr.Run(prg, map[string]any{"this": nil, "root": root})
		if err != nil {
			out = append(out, schemaErr(t.path, "compute error: "+err.Error()))
			continue
		}
		t.scope[t.field.GetName()] = coerceResult(res, c.GetResult())
	}
	return out
}

// defaultValue returns a field's default in the JSON value model and whether
// one is set. Numbers become float64; duration/timestamp become their string
// forms so they round-trip through validation.
func defaultValue(f *Schema_Filed) (any, bool) {
	switch {
	case f.GetFloat() != nil:
		if d := f.GetFloat().Default; d != nil {
			return float64(*d), true
		}
	case f.GetDouble() != nil:
		if d := f.GetDouble().Default; d != nil {
			return *d, true
		}
	case f.GetInt32() != nil:
		if d := f.GetInt32().Default; d != nil {
			return float64(*d), true
		}
	case f.GetInt64() != nil:
		if d := f.GetInt64().Default; d != nil {
			return float64(*d), true
		}
	case f.GetUint32() != nil:
		if d := f.GetUint32().Default; d != nil {
			return float64(*d), true
		}
	case f.GetUint64() != nil:
		if d := f.GetUint64().Default; d != nil {
			return float64(*d), true
		}
	case f.GetBool() != nil:
		if d := f.GetBool().Default; d != nil {
			return *d, true
		}
	case f.GetString_() != nil:
		if d := f.GetString_().Default; d != nil {
			return *d, true
		}
	case f.GetEnum() != nil:
		if d := f.GetEnum().Default; d != nil {
			return float64(*d), true
		}
	case f.GetDuration() != nil:
		if d := f.GetDuration().Default; d != nil {
			return d.AsDuration().String(), true
		}
	case f.GetTimestamp() != nil:
		if d := f.GetTimestamp().Default; d != nil {
			return d.AsTime().Format(time.RFC3339), true
		}
	}
	return nil, false
}

// coerceResult normalises expr-lang's numeric output to float64 (matching the
// JSON/Struct value model). With an integer result-type hint, the value is
// rounded.
func coerceResult(val any, rt Schema_Filed_ResultType) any {
	var f float64
	switch n := val.(type) {
	case int:
		f = float64(n)
	case int32:
		f = float64(n)
	case int64:
		f = float64(n)
	case float32:
		f = float64(n)
	case float64:
		f = n
	default:
		return val // non-numeric passes through
	}
	switch rt {
	case Schema_Filed_RESULT_TYPE_INT64, Schema_Filed_RESULT_TYPE_UINT64:
		return math.Round(f)
	default:
		return f
	}
}

// buildComputeOrder topologically orders the top-level Computed fields by their
// dependencies; it returns an error on a cycle. Used by ValidateSchema as a
// static well-formedness check (runtime resolution handles nested scopes).
func buildComputeOrder(fields []*Schema_Filed) ([]*Schema_Filed, error) {
	computed := map[string]*Schema_Filed{}
	var names []string
	for _, f := range fields {
		if f.GetComputed() != nil {
			computed[f.GetName()] = f
			names = append(names, f.GetName())
		}
	}
	if len(computed) == 0 {
		return nil, nil
	}
	deps := map[string][]string{}
	for name, f := range computed {
		for _, d := range exprDeps(f.GetComputed().GetExpr()) {
			if d != name {
				if _, ok := computed[d]; ok {
					deps[name] = append(deps[name], d)
				}
			}
		}
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var order []*Schema_Filed
	var visit func(string) error
	visit = func(n string) error {
		switch color[n] {
		case gray:
			return fmt.Errorf("computed field cycle involving %q", n)
		case black:
			return nil
		}
		color[n] = gray
		for _, d := range deps[n] {
			if err := visit(d); err != nil {
				return err
			}
		}
		color[n] = black
		order = append(order, computed[n])
		return nil
	}
	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// exprDeps returns the dotted root paths an expression reads (root.a -> "a",
// root.addr.zip -> "addr.zip"). Used to order Computed fields and detect cycles.
func exprDeps(code string) []string {
	tree, err := parser.Parse(code)
	if err != nil {
		return nil
	}
	var deps []string
	node := tree.Node
	ast.Walk(&node, depVisitor{&deps})
	return deps
}

type depVisitor struct{ deps *[]string }

func (d depVisitor) Visit(node *ast.Node) {
	if m, ok := (*node).(*ast.MemberNode); ok {
		if p, ok := memberPath(m); ok && p != "" {
			*d.deps = append(*d.deps, p)
		}
	}
}

// memberPath resolves a member chain rooted at the `root` identifier to a
// dotted path ("" for `root` itself, false if not rooted at `root`).
func memberPath(n ast.Node) (string, bool) {
	switch x := n.(type) {
	case *ast.IdentifierNode:
		if x.Value == "root" {
			return "", true
		}
		return "", false
	case *ast.MemberNode:
		base, ok := memberPath(x.Node)
		if !ok {
			return "", false
		}
		prop, ok := x.Property.(*ast.StringNode)
		if !ok {
			return "", false
		}
		if base == "" {
			return prop.Value, true
		}
		return base + "." + prop.Value, true
	default:
		return "", false
	}
}
