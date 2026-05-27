package schemapb

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"google.golang.org/protobuf/types/known/structpb"
)

// Compute evaluates the schema's top-level Computed fields against form and
// returns the derived values (by field name). The derived values are also
// written into form so downstream consumers can read them. The returned errors
// report expressions that failed to evaluate (e.g. a referenced value missing).
func (v *Validator) Compute(form map[string]any) (map[string]any, []*FieldError) {
	errs := v.compute(form)
	out := map[string]any{}
	for _, f := range v.computeOrder {
		if val, ok := form[f.GetName()]; ok {
			out[f.GetName()] = val
		}
	}
	return out, errs
}

// ComputeStruct evaluates Computed fields for a google.protobuf.Struct input.
func (v *Validator) ComputeStruct(s *structpb.Struct) (map[string]any, []*FieldError) {
	m := map[string]any{}
	if s != nil {
		m = s.AsMap()
	}
	return v.Compute(m)
}

// ComputeJSON evaluates Computed fields for a raw JSON object. It returns an
// error only if the JSON cannot be parsed into an object.
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

// compute evaluates the top-level Computed fields in dependency order and writes
// each result back into form. A field referencing an absent value yields a
// FieldError on that field's path (missing-input is an error).
func (v *Validator) compute(form map[string]any) []*FieldError {
	var out []*FieldError
	for _, f := range v.computeOrder {
		c := f.GetComputed()
		prg := v.programs[c.GetExpr()]
		if prg == nil {
			continue
		}
		res, err := expr.Run(prg, map[string]any{"this": nil, "root": form})
		if err != nil {
			out = append(out, schemaErr(f.GetName(), "compute error: "+err.Error()))
			continue
		}
		form[f.GetName()] = coerceResult(res, c.GetResult())
	}
	return out
}

// coerceResult normalises expr-lang's numeric output to float64 (matching the
// JSON/Struct value model the rest of the validator uses). When the result type
// hints an integer, the value is rounded.
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
		return val // non-numeric (string/bool/...) passes through
	}
	switch rt {
	case Schema_Filed_RESULT_TYPE_INT64, Schema_Filed_RESULT_TYPE_UINT64:
		return math.Round(f)
	default:
		return f
	}
}

// buildComputeOrder returns the top-level Computed fields sorted so that a field
// comes after every Computed field it references via root. It returns an error
// if the dependencies contain a cycle.
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
	for _, n := range names { // field order → deterministic output
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// exprDeps returns the field names an expression reads via root (root.field or
// root["field"]). Used to order Computed fields and detect cycles.
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
	m, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	id, ok := m.Node.(*ast.IdentifierNode)
	if !ok || id.Value != "root" {
		return
	}
	if p, ok := m.Property.(*ast.StringNode); ok {
		*d.deps = append(*d.deps, p.Value)
	}
}
