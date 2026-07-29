package schemapb

import (
	"fmt"
	"strconv"
	"time"
)

// Resolve fills unset fields from their defaults, coerces string inputs (when
// the schema enables coercion), applies normalize expressions and evaluates
// Computed fields in dependency order. values is mutated in place and
// returned; the result reports expression failures (code EXPR_ERROR). A nil
// map starts an empty form.
//
// Value model: native Go (see value.go). Callers holding a wire StructValue
// convert with ToGo()/StructFromGo().
func (s *Schema) Resolve(values map[string]any) (map[string]any, *ValidationResult, error) {
	e, err := s.engine()
	if err != nil {
		return values, nil, err
	}
	out, res := e.Resolve(values)
	return out, res, nil
}

// Resolve is the compiled-engine form of (*Schema).Resolve.
func (e *Engine) Resolve(values map[string]any) (map[string]any, *ValidationResult) {
	if values == nil {
		values = map[string]any{}
	}
	res := &ValidationResult{}
	var tasks []computeTask
	e.seed(e.schema, values, "", &tasks, values, res)
	e.runNormalize(e.schema, values, values, res)
	e.runCompute(values, tasks, res)
	return values, res
}

// exprErr builds a runtime expression-failure ValidationError.
func exprErr(path, expr, msg string) *ValidationError {
	return &ValidationError{
		Path:     path,
		Code:     ErrorCode_ERROR_CODE_EXPR_ERROR,
		Expr:     expr,
		Severity: SeverityError,
		Message:  msg,
	}
}

// active evaluates a field's `when` gate over root. A field with no gate is
// always active. An evaluation error deactivates the field and is reported.
func (e *Engine) active(f *Schema_Field, root map[string]any, path string, res *ValidationResult) bool {
	when := f.GetWhen()
	if when == "" {
		return true
	}
	ok, err := e.evalBool(when, map[string]any{"this": nil, "root": root})
	if err != nil {
		res.Errors = append(res.Errors, exprErr(path, when, "when: "+err.Error()))
		return false
	}
	return ok
}

// computeTask is a Computed field pending evaluation: it reads from root
// (shared) and writes its result to scope[field.Name].
type computeTask struct {
	field *Schema_Field
	scope map[string]any
	path  string
}

// seed fills defaults for unset fields (immutable fields are forced to their
// default) and collects Computed fields as tasks, recursing into present
// containers. It never materialises an absent object, so optional sub-forms
// stay absent and don't spuriously trip their children's "required" checks.
func (e *Engine) seed(schema *Schema, scope map[string]any, prefix string, tasks *[]computeTask, root map[string]any, res *ValidationResult) {
	coerce := schema.GetCoerce()
	for _, f := range schema.GetFields() {
		name := f.GetName()
		path := joinPath(prefix, name)

		// Inactive fields are treated as absent: no default seeded, no
		// Computed scheduled, subtree not recursed. Existing value preserved.
		if !e.active(f, root, path, res) {
			continue
		}

		if coerce {
			if cur, ok := scope[name]; ok {
				if coerced, changed := coerceInput(f, cur); changed {
					scope[name] = coerced
				}
			}
		}

		if f.GetImmutable() {
			if dv, ok := defaultValue(f); ok {
				scope[name] = dv
			}
		} else if _, ok := scope[name]; !ok {
			if dv, ok := defaultValue(f); ok {
				scope[name] = dv
			}
		}

		switch {
		case f.GetComputed() != nil:
			*tasks = append(*tasks, computeTask{field: f, scope: scope, path: path})
		case f.GetObject() != nil && f.GetObject().GetSchema() != nil:
			if child, ok := scope[name].(map[string]any); ok {
				e.seed(f.GetObject().GetSchema(), child, path, tasks, root, res)
			}
		case f.GetList() != nil && len(f.GetList().GetItems()) >= 1:
			if o := f.GetList().GetItems()[0].GetObject(); o != nil && o.GetSchema() != nil {
				if arr, ok := scope[name].([]any); ok {
					for i, el := range arr {
						if m, ok := el.(map[string]any); ok {
							e.seed(o.GetSchema(), m, fmt.Sprintf("%s[%d]", path, i), tasks, root, res)
						}
					}
				}
			}
		case f.GetMap() != nil && f.GetMap().GetValueSchema() != nil:
			if mm, ok := scope[name].(map[string]any); ok {
				for k, el := range mm {
					if m, ok := el.(map[string]any); ok {
						e.seed(f.GetMap().GetValueSchema(), m, joinPath(path, k), tasks, root, res)
					}
				}
			}
		case f.GetOneOf() != nil:
			if variant, m := selectVariant(f.GetOneOf(), scope[name]); variant != nil {
				e.seed(variant, m, path, tasks, root, res)
			}
		case f.GetRef() != nil:
			if def := e.schema.GetDefs()[refDefKey(f.GetRef())]; def != nil {
				if child, ok := scope[name].(map[string]any); ok {
					e.seed(def, child, path, tasks, root, res)
				}
			}
		}
	}
}

// selectVariant picks the OneOf variant schema for a value by its
// discriminator property; nil when the value is not an object or the
// discriminator is missing/unknown.
func selectVariant(oo *Schema_Field_OneOf, val any) (*Schema, map[string]any) {
	m, ok := val.(map[string]any)
	if !ok {
		return nil, nil
	}
	disc, ok := m[oo.GetDiscriminator()].(string)
	if !ok || disc == "" {
		return nil, nil
	}
	variant, ok := oo.GetVariants()[disc]
	if !ok {
		return nil, nil
	}
	return variant, m
}

// runNormalize applies normalize expressions to present, active fields,
// recursing into containers. Runs after seed (defaults in place) and before
// runCompute (Computed reads normalised values).
func (e *Engine) runNormalize(schema *Schema, scope, root map[string]any, res *ValidationResult) {
	for _, f := range schema.GetFields() {
		name := f.GetName()
		cur, exists := scope[name]
		if !exists || cur == nil {
			continue
		}
		if f.GetWhen() != "" {
			if ok, err := e.evalBool(f.GetWhen(), map[string]any{"this": nil, "root": root}); err != nil || !ok {
				continue
			}
		}
		if norm := f.GetNormalize(); norm != "" {
			out, err := e.eval(norm, map[string]any{"this": cur, "root": root})
			if err != nil {
				res.Errors = append(res.Errors, exprErr(name, norm, "normalize: "+err.Error()))
			} else {
				scope[name] = out
				cur = out
			}
		}
		if o := f.GetObject(); o != nil && o.GetSchema() != nil {
			if child, ok := cur.(map[string]any); ok {
				e.runNormalize(o.GetSchema(), child, root, res)
			}
		}
		if l := f.GetList(); l != nil && len(l.GetItems()) >= 1 {
			if obj := l.GetItems()[0].GetObject(); obj != nil && obj.GetSchema() != nil {
				if arr, ok := cur.([]any); ok {
					for _, el := range arr {
						if m, ok := el.(map[string]any); ok {
							e.runNormalize(obj.GetSchema(), m, root, res)
						}
					}
				}
			}
		}
		if mp := f.GetMap(); mp != nil && mp.GetValueSchema() != nil {
			if mm, ok := cur.(map[string]any); ok {
				for _, el := range mm {
					if m, ok := el.(map[string]any); ok {
						e.runNormalize(mp.GetValueSchema(), m, root, res)
					}
				}
			}
		}
		if oo := f.GetOneOf(); oo != nil {
			if variant, m := selectVariant(oo, cur); variant != nil {
				e.runNormalize(variant, m, root, res)
			}
		}
		if ref := f.GetRef(); ref != nil {
			if def := e.schema.GetDefs()[refDefKey(ref)]; def != nil {
				if m, ok := cur.(map[string]any); ok {
					e.runNormalize(def, m, root, res)
				}
			}
		}
	}
}

// runCompute evaluates tasks in dependency order (the root paths each
// expression reads), writing each result into its scope. Cycles between
// nested scopes are reported and their fields left unevaluated (top-level
// cycles are already rejected at Compile).
func (e *Engine) runCompute(root map[string]any, tasks []computeTask, res *ValidationResult) {
	if len(tasks) == 0 {
		return
	}
	byPath := make(map[string]computeTask, len(tasks))
	for _, t := range tasks {
		byPath[t.path] = t
	}
	deps := map[string][]string{}
	for _, t := range tasks {
		for _, d := range e.exprDeps(t.field.GetComputed().GetExpr()) {
			if d != t.path {
				if _, ok := byPath[d]; ok {
					deps[t.path] = append(deps[t.path], d)
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
	var order []computeTask
	var visit func(string) bool
	visit = func(p string) bool {
		switch color[p] {
		case gray:
			return false
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
			res.Errors = append(res.Errors, schemaErr(t.path, "computed field cycle"))
		}
	}

	for _, t := range order {
		c := t.field.GetComputed()
		out, err := e.eval(c.GetExpr(), map[string]any{"this": nil, "root": root})
		if err != nil {
			res.Errors = append(res.Errors, exprErr(t.path, c.GetExpr(), "compute: "+err.Error()))
			continue
		}
		shaped, err := shapeResult(c.GetResult(), out)
		if err != nil {
			res.Errors = append(res.Errors, exprErr(t.path, c.GetExpr(), "compute: "+err.Error()))
			continue
		}
		t.scope[t.field.GetName()] = shaped
	}
}

// shapeResult converts a computed result to the native representation of its
// declared ResultType. UNSPECIFIED keeps the CEL-native value as is (numbers
// stay honestly typed — no float64 flattening).
func shapeResult(rt Schema_Field_ResultType, x any) (any, error) {
	if x == nil {
		return nil, nil
	}
	switch rt {
	case Schema_Field_RESULT_TYPE_UNSPECIFIED, Schema_Field_RESULT_TYPE_JSON:
		return x, nil
	case Schema_Field_RESULT_TYPE_DOUBLE:
		if n, ok := asFloat64(x); ok {
			return n, nil
		}
	case Schema_Field_RESULT_TYPE_INT64:
		if n, ok := asInt64(x); ok {
			return n, nil
		}
	case Schema_Field_RESULT_TYPE_UINT64:
		if n, ok := asUint64(x); ok {
			return n, nil
		}
	case Schema_Field_RESULT_TYPE_BOOL:
		if b, ok := x.(bool); ok {
			return b, nil
		}
	case Schema_Field_RESULT_TYPE_STRING:
		if s, ok := x.(string); ok {
			return s, nil
		}
	case Schema_Field_RESULT_TYPE_DURATION:
		if d, ok := x.(time.Duration); ok {
			return d, nil
		}
	case Schema_Field_RESULT_TYPE_TIMESTAMP:
		if t, ok := x.(time.Time); ok {
			return t, nil
		}
	case Schema_Field_RESULT_TYPE_BYTES:
		if b, ok := x.([]byte); ok {
			return b, nil
		}
	}
	return nil, fmt.Errorf("result %T does not match declared type %v", x, rt)
}

// coerceInput converts a string value to the field's expected native type.
// Returns the coerced value and whether conversion occurred; unparseable
// strings pass through so the type error reports normally.
func coerceInput(f *Schema_Field, val any) (any, bool) {
	s, ok := val.(string)
	if !ok {
		return val, false
	}
	switch {
	case f.GetInt32() != nil, f.GetInt64() != nil:
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
	case f.GetUint32() != nil, f.GetUint64() != nil:
		if n, err := strconv.ParseUint(s, 10, 64); err == nil {
			return n, true
		}
	case f.GetFloat() != nil, f.GetDouble() != nil:
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n, true
		}
	case f.GetBool() != nil:
		switch s {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	case f.GetEnum() != nil:
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			return n, true
		}
	case f.GetDuration() != nil:
		if d, err := time.ParseDuration(s); err == nil {
			return d, true
		}
	case f.GetTimestamp() != nil:
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return val, false
}

// defaultValue returns a field's default in the native value model and
// whether one is set.
func defaultValue(f *Schema_Field) (any, bool) {
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
			return int64(*d), true
		}
	case f.GetInt64() != nil:
		if d := f.GetInt64().Default; d != nil {
			return *d, true
		}
	case f.GetUint32() != nil:
		if d := f.GetUint32().Default; d != nil {
			return uint64(*d), true
		}
	case f.GetUint64() != nil:
		if d := f.GetUint64().Default; d != nil {
			return *d, true
		}
	case f.GetBool() != nil:
		if d := f.GetBool().Default; d != nil {
			return *d, true
		}
	case f.GetString_() != nil:
		if d := f.GetString_().Default; d != nil {
			return *d, true
		}
	case f.GetBytes() != nil:
		if d := f.GetBytes().GetDefault(); d != nil {
			return append([]byte(nil), d...), true
		}
	case f.GetEnum() != nil:
		if d := f.GetEnum().Default; d != nil {
			return int64(*d), true
		}
	case f.GetDuration() != nil:
		if d := f.GetDuration().GetDefault(); d != nil {
			return d.AsDuration(), true
		}
	case f.GetTimestamp() != nil:
		if d := f.GetTimestamp().GetDefault(); d != nil {
			return d.AsTime(), true
		}
	case f.GetJson() != nil:
		if d := f.GetJson().GetDefault(); d != nil {
			return d.ToGo(), true
		}
	}
	return nil, false
}

// =============================================================================
// Renderer helpers
// =============================================================================

// FieldActive reports whether the named top-level field is active for the
// given form (its `when` gate over root). A field with no `when` is always
// active.
func (s *Schema) FieldActive(name FieldName, root map[string]any) (bool, error) {
	e, err := s.engine()
	if err != nil {
		return false, err
	}
	return e.FieldActive(name, root)
}

// FieldActive is the compiled-engine form of (*Schema).FieldActive.
func (e *Engine) FieldActive(name FieldName, root map[string]any) (bool, error) {
	f := findField(e.schema.GetFields(), string(name))
	if f == nil {
		return false, fmt.Errorf("schemapb: unknown field %q", name)
	}
	if f.GetWhen() == "" {
		return true, nil
	}
	return e.evalBool(f.GetWhen(), map[string]any{"this": nil, "root": root})
}

// EnumOptions returns the allowed integer values for the named top-level enum
// field given the form: the options_expr result when set, the static enum
// values otherwise.
func (s *Schema) EnumOptions(name FieldName, root map[string]any) ([]int32, error) {
	e, err := s.engine()
	if err != nil {
		return nil, err
	}
	return e.EnumOptions(name, root)
}

// EnumOptions is the compiled-engine form of (*Schema).EnumOptions.
func (e *Engine) EnumOptions(name FieldName, root map[string]any) ([]int32, error) {
	f := findField(e.schema.GetFields(), string(name))
	if f == nil || f.GetEnum() == nil {
		return nil, fmt.Errorf("schemapb: field %q is not an enum", name)
	}
	en := f.GetEnum()
	if en.GetOptionsExpr() == "" {
		out := make([]int32, 0, len(en.GetValues()))
		for k := range en.GetValues() {
			out = append(out, k)
		}
		return out, nil
	}
	return e.evalEnumOptions(en.GetOptionsExpr(), root)
}

// evalEnumOptions evaluates an options_expr into the allowed enum values.
func (e *Engine) evalEnumOptions(src string, root map[string]any) ([]int32, error) {
	res, err := e.eval(src, map[string]any{"this": nil, "root": root})
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("options_expr yields %T, want a list of ints", res)
	}
	out := make([]int32, 0, len(arr))
	for _, el := range arr {
		n, ok := asInt64(el)
		if !ok {
			return nil, fmt.Errorf("options_expr element %v is not an int", el)
		}
		out = append(out, int32(n))
	}
	return out, nil
}

// ListCount returns the required length of the named top-level list field as
// derived from its count_expr over root.
func (s *Schema) ListCount(name FieldName, root map[string]any) (int64, error) {
	e, err := s.engine()
	if err != nil {
		return 0, err
	}
	return e.ListCount(name, root)
}

// ListCount is the compiled-engine form of (*Schema).ListCount.
func (e *Engine) ListCount(name FieldName, root map[string]any) (int64, error) {
	f := findField(e.schema.GetFields(), string(name))
	if f == nil || f.GetList() == nil {
		return 0, fmt.Errorf("schemapb: field %q is not a list", name)
	}
	ce := f.GetList().GetCountExpr()
	if ce == "" {
		return 0, fmt.Errorf("schemapb: field %q has no count_expr", name)
	}
	return e.evalCount(ce, root)
}

// evalCount evaluates a count_expr into a non-negative length.
func (e *Engine) evalCount(src string, root map[string]any) (int64, error) {
	res, err := e.eval(src, map[string]any{"this": nil, "root": root})
	if err != nil {
		return 0, err
	}
	n, ok := asInt64(res)
	if !ok || n < 0 {
		return 0, fmt.Errorf("count_expr yields %v, want a non-negative int", res)
	}
	return n, nil
}

// findField returns the field with the given name, or nil.
func findField(fields []*Schema_Field, name string) *Schema_Field {
	for _, f := range fields {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}
