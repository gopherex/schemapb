package schemapb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"time"
	"unicode/utf8"
)

// Validate checks form values against the schema: it rejects changed
// immutable values, resolves the form (defaults, normalize, Computed), then
// runs structured constraints, CEL rules and form-wide rules against the
// fully resolved form. values is mutated in place by the resolve step.
//
// The returned error is programmatic only (schema does not compile); every
// validation outcome — including warnings — lives in the ValidationResult.
func (s *Schema) Validate(values map[string]any) (*ValidationResult, error) {
	e, err := s.engine()
	if err != nil {
		return nil, err
	}

	return e.Validate(values), nil
}

// Validate is the compiled-engine form of (*Schema).Validate.
func (e *Engine) Validate(values map[string]any) *ValidationResult {
	if values == nil {
		values = map[string]any{}
	}

	res := &ValidationResult{}
	// Immutable changes are checked on the raw input, before resolve forces
	// the values back to their defaults.
	e.checkImmutable(e.schema.GetFields(), values, "", values, res)
	_, resolved := e.Resolve(values)
	res.Errors = append(res.Errors, resolved.GetErrors()...)
	e.validateFields(e.schema, values, values, "", res)

	for _, r := range e.schema.GetRules() {
		e.evalRule(r, ruleErrPath(r), nil, values, nil, res)
	}

	return res
}

// Ok reports whether the result contains no failures at all.
func (r *ValidationResult) Ok() bool { return len(r.GetErrors()) == 0 }

// Blocking reports whether the result contains at least one ERROR-severity
// failure (warnings alone do not block).
func (r *ValidationResult) Blocking() bool {
	for _, e := range r.GetErrors() {
		if e.GetSeverity() != Schema_Field_SEVERITY_WARNING {
			return true
		}
	}

	return false
}

// =============================================================================
// Error construction
// =============================================================================

// verr builds one ValidationError; the human message renders from the spec's
// shared template set (messages.go). actual is masked for secret fields by
// the callers that know the field.
func verr(path string, code ErrorCode, constraint string, expected, actual *Value) *ValidationError {
	return &ValidationError{
		Path:       path,
		Code:       code,
		Constraint: constraint,
		Expected:   expected,
		Actual:     actual,
		Severity:   SeverityError,
		Message:    renderMessage(code, expected, actual),
	}
}

// typeErr reports a value of the wrong type for the field kind.
func typeErr(path, want string, val any) *ValidationError {
	actual, _ := FromGo(val)

	return verr(path, ErrorCode_ERROR_CODE_TYPE_MISMATCH, "", StrV(want), actual)
}

// ruleErrPath is the path a form-wide rule reports on: its id when set.
func ruleErrPath(r *Schema_Field_Rule) string { return r.GetId() }

// mask drops the actual value from errors on secret fields and re-renders
// the message so the secret cannot leak through the text either.
func mask(errs []*ValidationError, secret bool) []*ValidationError {
	if !secret {
		return errs
	}

	for _, e := range errs {
		e.Actual = nil
		if _, templated := messageTemplates[e.GetCode()]; templated {
			e.Message = renderMessage(e.GetCode(), e.GetExpected(), nil)
		}
	}

	return errs
}

// =============================================================================
// Field traversal
// =============================================================================

// checkImmutable reports a submitted value that differs from an immutable
// field's default (a system-fixed value cannot be changed). Walks present
// containers. Only enforced when a default exists.
//
//nolint:gocognit,cyclop // container traversal mirrors the schema tree
func (e *Engine) checkImmutable(
	fields []*Schema_Field, scope map[string]any, prefix string,
	root map[string]any, res *ValidationResult,
) {
	for _, f := range fields {
		name := f.GetName()
		path := joinPath(prefix, name)

		if f.GetWhen() != "" {
			if ok, err := e.evalBool(f.GetWhen(), map[string]any{"this": nil, "root": root}); err != nil || !ok {
				continue
			}
		}

		if f.GetImmutable() {
			if cur, ok := scope[name]; ok {
				if dv, has := defaultValue(f); has && !nativeEqual(cur, dv) {
					expected, _ := CanonicalValue(f, dv)
					actual, _ := FromGo(cur)
					err := verr(path, ErrorCode_ERROR_CODE_IMMUTABLE_MODIFIED, "immutable", expected, actual)
					res.Errors = append(res.Errors, mask([]*ValidationError{err}, f.GetSecret())...)
				}
			}

			continue
		}

		if o := f.GetObject(); o != nil && o.GetSchema() != nil {
			if child, ok := scope[name].(map[string]any); ok {
				e.checkImmutable(o.GetSchema().GetFields(), child, path, root, res)
			}
		}

		if l := f.GetList(); l != nil && len(l.GetItems()) >= 1 { //nolint:nestif // per-index tuple/list descent
			if arr, ok := scope[name].([]any); ok {
				for i, el := range arr {
					it := listItemDef(l, i)
					if it == nil {
						continue
					}

					if o := it.GetObject(); o != nil && o.GetSchema() != nil {
						if m, isObj := el.(map[string]any); isObj {
							e.checkImmutable(o.GetSchema().GetFields(), m, fmt.Sprintf("%s[%d]", path, i), root, res)
						}
					}
				}
			}
		}

		if mp := f.GetMap(); mp != nil && mp.GetValueSchema() != nil {
			if mm, ok := scope[name].(map[string]any); ok {
				for _, k := range slices.Sorted(maps.Keys(mm)) {
					if m, isObj := mm[k].(map[string]any); isObj {
						e.checkImmutable(mp.GetValueSchema().GetFields(), m, joinPath(path, k), root, res)
					}
				}
			}
		}
	}
}

// validateFields validates one scope (the root form, a nested object, a map
// value, a oneof variant) against its schema.
//
//nolint:cyclop // one linear scope validation pass
func (e *Engine) validateFields(schema *Schema, scope, root map[string]any, prefix string, res *ValidationResult) {
	fields := schema.GetFields()

	// `when` gating: an inactive field is skipped entirely — its value key is
	// ignored, not counted, and not validated. Evaluation errors were already
	// reported by the resolve step; here an errored gate means inactive.
	inactive := map[string]bool{}

	declared := map[string]bool{}
	for _, f := range fields {
		declared[f.GetName()] = true

		if f.GetWhen() != "" {
			if ok, err := e.evalBool(f.GetWhen(), map[string]any{"this": nil, "root": root}); err != nil || !ok {
				inactive[f.GetName()] = true
			}
		}
	}

	// Strict mode: unknown keys are rejected. Declared fields (active or not)
	// are always known. Keys sorted for deterministic output.
	if schema.GetStrict() {
		for _, key := range slices.Sorted(maps.Keys(scope)) {
			if !declared[key] {
				actual, _ := FromGo(scope[key])
				res.Errors = append(res.Errors, verr(joinPath(prefix, key),
					ErrorCode_ERROR_CODE_UNKNOWN_FIELD, "strict", nil, actual))
			}
		}
	}

	// min/max properties: inactive fields' present keys do not count.
	var n uint64

	for key := range scope {
		if !inactive[key] {
			n++
		}
	}

	if mn := schema.MinProperties; mn != nil && n < *mn {
		res.Errors = append(res.Errors, verr(prefix, ErrorCode_ERROR_CODE_MIN_PROPERTIES_VIOLATED,
			"min_properties", UInt64V(*mn), UInt64V(n)))
	}

	if mx := schema.MaxProperties; mx != nil && n > *mx {
		res.Errors = append(res.Errors, verr(prefix, ErrorCode_ERROR_CODE_MAX_PROPERTIES_VIOLATED,
			"max_properties", UInt64V(*mx), UInt64V(n)))
	}

	for _, f := range fields {
		if inactive[f.GetName()] {
			continue
		}

		val, exists := scope[f.GetName()]
		e.validateOne(f, val, exists, joinPath(prefix, f.GetName()), root, nil, res)
	}
}

// validateOne validates a single field value: presence, nullability, kind
// constraints, then the field's CEL rules.
func (e *Engine) validateOne(
	f *Schema_Field, val any, exists bool, path string,
	root, extra map[string]any, res *ValidationResult,
) {
	if !exists {
		if f.GetRequired() {
			res.Errors = append(res.Errors, verr(path, ErrorCode_ERROR_CODE_REQUIRED_MISSING, "required", nil, nil))
		}

		return
	}

	if val == nil {
		switch {
		case f.GetRequired():
			res.Errors = append(res.Errors, verr(path, ErrorCode_ERROR_CODE_REQUIRED_MISSING, "required", nil, nil))
		case !f.GetNullable():
			res.Errors = append(res.Errors, verr(path, ErrorCode_ERROR_CODE_NOT_NULLABLE, "nullable", nil, NullV()))
		}

		return
	}

	res.Errors = append(res.Errors, mask(e.checkKind(f, val, path, root), f.GetSecret())...)
	for _, r := range f.GetRules() {
		e.evalRule(r, path, val, root, extra, res)
	}
}

// evalRule evaluates one CEL rule; false yields RULE_VIOLATED with the rule's
// message and severity, an evaluation error yields EXPR_ERROR.
func (e *Engine) evalRule(
	r *Schema_Field_Rule, path string, this any,
	root, extra map[string]any, res *ValidationResult,
) {
	vars := map[string]any{"this": this, "root": root}
	for k, v := range extra {
		vars[k] = v
	}

	out, err := e.eval(r.GetExpr(), vars)
	if err != nil {
		ve := exprErr(path, r.GetExpr(), "rule: "+err.Error())
		ve.RuleId = r.Id
		res.Errors = append(res.Errors, ve)

		return
	}

	if b, ok := out.(bool); ok && b {
		return
	}

	sev := r.GetSeverity()
	if sev == Schema_Field_SEVERITY_UNSPECIFIED {
		sev = SeverityError
	}

	res.Errors = append(res.Errors, &ValidationError{
		Path:     path,
		Code:     ErrorCode_ERROR_CODE_RULE_VIOLATED,
		Expr:     r.GetExpr(),
		RuleId:   r.Id,
		Severity: sev,
		Message:  r.GetMessage(),
	})
}

// =============================================================================
// Kind dispatch
// =============================================================================

//nolint:cyclop,funlen // flat exhaustive kind dispatch
func (e *Engine) checkKind(f *Schema_Field, val any, path string, root map[string]any) []*ValidationError {
	switch {
	case f.GetFloat() != nil:
		k := f.GetFloat()

		return checkNumber(path, val, numFloat[float32]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn})
	case f.GetDouble() != nil:
		k := f.GetDouble()

		return checkNumber(path, val, numFloat[float64]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn})
	case f.GetInt32() != nil:
		k := f.GetInt32()

		return checkInt(path, val,
			numInt[int32]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn},
			math.MinInt32, math.MaxInt32)
	case f.GetInt64() != nil:
		k := f.GetInt64()

		return checkInt(path, val,
			numInt[int64]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn},
			math.MinInt64, math.MaxInt64)
	case f.GetUint32() != nil:
		k := f.GetUint32()

		return checkUint(path, val,
			numInt[uint32]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn},
			math.MaxUint32)
	case f.GetUint64() != nil:
		k := f.GetUint64()

		return checkUint(path, val,
			numInt[uint64]{k.Const, k.Gt, k.Gte, k.Lt, k.Lte, k.MultipleOf, k.In, k.NotIn},
			math.MaxUint64)
	case f.GetBool() != nil:
		b, ok := val.(bool)
		if !ok {
			return []*ValidationError{typeErr(path, "bool", val)}
		}

		if c := f.GetBool().Const; c != nil && b != *c {
			return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_CONST_MISMATCH, "const",
				BoolV(*c), BoolV(b))}
		}

		return nil
	case f.GetString_() != nil:
		s, ok := val.(string)
		if !ok {
			return []*ValidationError{typeErr(path, "string", val)}
		}

		return e.checkString(path, s, f.GetString_())
	case f.GetBytes() != nil:
		b, ok := val.([]byte)
		if !ok {
			return []*ValidationError{typeErr(path, "bytes", val)}
		}

		return checkBytes(path, b, f.GetBytes())
	case f.GetChoice() != nil:
		return e.checkChoice(path, val, f.GetChoice(), root)
	case f.GetDuration() != nil:
		return checkDuration(path, val, f.GetDuration())
	case f.GetTimestamp() != nil:
		return checkTimestamp(path, val, f.GetTimestamp())
	case f.GetList() != nil:
		arr, ok := val.([]any)
		if !ok {
			return []*ValidationError{typeErr(path, "list", val)}
		}

		return e.checkList(path, arr, f.GetList(), root)
	case f.GetObject() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return []*ValidationError{typeErr(path, "object", val)}
		}

		sub := &ValidationResult{}
		if s := f.GetObject().GetSchema(); s != nil {
			e.validateFields(s, m, root, path, sub)

			for _, r := range s.GetRules() {
				e.evalRule(r, path, m, root, nil, sub)
			}
		}

		return sub.GetErrors()
	case f.GetMap() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return []*ValidationError{typeErr(path, "map", val)}
		}

		return e.checkMap(path, m, f.GetMap(), root)
	case f.GetOneOf() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return []*ValidationError{typeErr(path, "object", val)}
		}

		return e.checkOneOf(path, m, f.GetOneOf(), root)
	case f.GetRef() != nil:
		return e.checkRef(path, val, f.GetRef(), root)
	case f.GetComputed() != nil, f.GetJson() != nil:
		// Computed: derived, no structured constraints (its rules ran above).
		// Json: free-form by definition.
		return nil
	}

	return nil
}

// =============================================================================
// Numeric checks (typed, no float64 lowering)
// =============================================================================

// numInt carries the constraints of one integer kind.
type numInt[T int32 | int64 | uint32 | uint64] struct {
	cst, gt, gte, lt, lte, mul *T
	in, notIn                  []T
}

// numFloat carries the constraints of one float kind.
type numFloat[T float32 | float64] struct {
	cst, gt, gte, lt, lte, mul *T
	in, notIn                  []T
}

// numViolations runs the shared ordered constraint sequence for any ordered
// numeric type; mkV canonicalizes a bound into a wire Value for the error.
func numViolations[T int64 | uint64 | float64](path string, n T,
	cst, gt, gte, lt, lte *T, in, notIn []T, mkV func(T) *Value,
) []*ValidationError {
	var out []*ValidationError

	add := func(code ErrorCode, constraint string, expected *Value) {
		out = append(out, verr(path, code, constraint, expected, mkV(n)))
	}
	if cst != nil && n != *cst {
		add(ErrorCode_ERROR_CODE_CONST_MISMATCH, "const", mkV(*cst))
	}

	if gt != nil && n <= *gt {
		add(ErrorCode_ERROR_CODE_GT_VIOLATED, "gt", mkV(*gt))
	}

	if gte != nil && n < *gte {
		add(ErrorCode_ERROR_CODE_GTE_VIOLATED, "gte", mkV(*gte))
	}

	if lt != nil && n >= *lt {
		add(ErrorCode_ERROR_CODE_LT_VIOLATED, "lt", mkV(*lt))
	}

	if lte != nil && n > *lte {
		add(ErrorCode_ERROR_CODE_LTE_VIOLATED, "lte", mkV(*lte))
	}

	if len(in) > 0 && !slices.Contains(in, n) {
		add(ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET, "in", listOf(in, mkV))
	}

	if len(notIn) > 0 && slices.Contains(notIn, n) {
		add(ErrorCode_ERROR_CODE_IN_FORBIDDEN_SET, "not_in", listOf(notIn, mkV))
	}

	return out
}

// listOf builds a wire list from typed bounds.
func listOf[T any](vs []T, mkV func(T) *Value) *Value {
	items := make([]*Value, len(vs))
	for i, v := range vs {
		items[i] = mkV(v)
	}

	return ListV(items...)
}

// widen converts a typed constraint set to the 64-bit domain used for
// comparison.
func widen[S, T int32 | int64 | uint32 | uint64 | float32 | float64](p *S) *T {
	if p == nil {
		return nil
	}

	v := T(*p)

	return &v
}

func widenSlice[S, T int32 | int64 | uint32 | uint64 | float32 | float64](s []S) []T {
	if len(s) == 0 {
		return nil
	}

	out := make([]T, len(s))
	for i, v := range s {
		out[i] = T(v)
	}

	return out
}

func checkInt[T int32 | int64](path string, val any, k numInt[T], minV, maxV int64) []*ValidationError {
	n, ok := asInt64(val)
	if !ok {
		return []*ValidationError{typeErr(path, "integer", val)}
	}

	if n < minV || n > maxV {
		return []*ValidationError{typeErr(path, fmt.Sprintf("integer in [%d, %d]", minV, maxV), val)}
	}

	errs := numViolations(path, n,
		widen[T, int64](k.cst), widen[T, int64](k.gt), widen[T, int64](k.gte),
		widen[T, int64](k.lt), widen[T, int64](k.lte),
		widenSlice[T, int64](k.in), widenSlice[T, int64](k.notIn), Int64V)
	if k.mul != nil && *k.mul != 0 && n%int64(*k.mul) != 0 {
		errs = append(errs, verr(path, ErrorCode_ERROR_CODE_MULTIPLE_OF_VIOLATED, "multiple_of",
			Int64V(int64(*k.mul)), Int64V(n)))
	}

	return errs
}

func checkUint[T uint32 | uint64](path string, val any, k numInt[T], maxV uint64) []*ValidationError {
	n, ok := asUint64(val)
	if !ok {
		return []*ValidationError{typeErr(path, "unsigned integer", val)}
	}

	if n > maxV {
		return []*ValidationError{typeErr(path, fmt.Sprintf("unsigned integer <= %d", maxV), val)}
	}

	errs := numViolations(path, n,
		widen[T, uint64](k.cst), widen[T, uint64](k.gt), widen[T, uint64](k.gte),
		widen[T, uint64](k.lt), widen[T, uint64](k.lte),
		widenSlice[T, uint64](k.in), widenSlice[T, uint64](k.notIn), UInt64V)
	if k.mul != nil && *k.mul != 0 && n%uint64(*k.mul) != 0 {
		errs = append(errs, verr(path, ErrorCode_ERROR_CODE_MULTIPLE_OF_VIOLATED, "multiple_of",
			UInt64V(uint64(*k.mul)), UInt64V(n)))
	}

	return errs
}

func checkNumber[T float32 | float64](path string, val any, k numFloat[T]) []*ValidationError {
	n, ok := asFloat64(val)
	if !ok {
		return []*ValidationError{typeErr(path, "number", val)}
	}

	errs := numViolations(path, n,
		widen[T, float64](k.cst), widen[T, float64](k.gt), widen[T, float64](k.gte),
		widen[T, float64](k.lt), widen[T, float64](k.lte),
		widenSlice[T, float64](k.in), widenSlice[T, float64](k.notIn), DoubleV)
	if k.mul != nil && float64(*k.mul) != 0 && math.Mod(n, float64(*k.mul)) != 0 {
		errs = append(errs, verr(path, ErrorCode_ERROR_CODE_MULTIPLE_OF_VIOLATED, "multiple_of",
			DoubleV(float64(*k.mul)), DoubleV(n)))
	}

	return errs
}

// =============================================================================
// String / bytes / enum / duration / timestamp checks
// =============================================================================

//nolint:cyclop // flat constraint sequence in spec order
func (e *Engine) checkString(path, s string, k *Schema_Field_String) []*ValidationError {
	var out []*ValidationError

	add := func(code ErrorCode, constraint string, expected *Value) {
		out = append(out, verr(path, code, constraint, expected, StrV(s)))
	}
	n := uint64(utf8.RuneCountInString(s)) //nolint:gosec // rune count is never negative

	if k.Const != nil && s != k.GetConst() {
		add(ErrorCode_ERROR_CODE_CONST_MISMATCH, "const", StrV(k.GetConst()))
	}

	if k.Len != nil && n != k.GetLen() {
		add(ErrorCode_ERROR_CODE_LEN_MISMATCH, "len", UInt64V(k.GetLen()))
	}

	if k.MinLen != nil && n < k.GetMinLen() {
		add(ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED, "min_len", UInt64V(k.GetMinLen()))
	}

	if k.MaxLen != nil && n > k.GetMaxLen() {
		add(ErrorCode_ERROR_CODE_MAX_LEN_VIOLATED, "max_len", UInt64V(k.GetMaxLen()))
	}

	if k.Pattern != nil {
		if re := e.regexps[k.GetPattern()]; re != nil && !re.MatchString(s) {
			add(ErrorCode_ERROR_CODE_PATTERN_MISMATCH, "pattern", StrV(k.GetPattern()))
		}
	}

	if len(k.GetIn()) > 0 && !slices.Contains(k.GetIn(), s) {
		add(ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET, "in", listOf(k.GetIn(), StrV))
	}

	if len(k.GetNotIn()) > 0 && slices.Contains(k.GetNotIn(), s) {
		add(ErrorCode_ERROR_CODE_IN_FORBIDDEN_SET, "not_in", listOf(k.GetNotIn(), StrV))
	}

	if k.Format != nil && k.GetFormat() != "" {
		check, known := e.formats[Format(k.GetFormat())]

		switch {
		case !known:
			add(ErrorCode_ERROR_CODE_UNSUPPORTED_FORMAT, "format", StrV(k.GetFormat()))
		case !check(s):
			add(ErrorCode_ERROR_CODE_FORMAT_MISMATCH, "format", StrV(k.GetFormat()))
		}
	}

	return out
}

//nolint:cyclop // flat constraint sequence in spec order
func checkBytes(path string, b []byte, k *Schema_Field_Bytes) []*ValidationError {
	var out []*ValidationError

	add := func(code ErrorCode, constraint string, expected *Value) {
		out = append(out, verr(path, code, constraint, expected, BytesV(b)))
	}
	n := uint64(len(b))

	if k.Const != nil && !bytes.Equal(b, k.GetConst()) {
		add(ErrorCode_ERROR_CODE_CONST_MISMATCH, "const", BytesV(k.GetConst()))
	}

	if k.Len != nil && n != k.GetLen() {
		add(ErrorCode_ERROR_CODE_LEN_MISMATCH, "len", UInt64V(k.GetLen()))
	}

	if k.MinLen != nil && n < k.GetMinLen() {
		add(ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED, "min_len", UInt64V(k.GetMinLen()))
	}

	if k.MaxLen != nil && n > k.GetMaxLen() {
		add(ErrorCode_ERROR_CODE_MAX_LEN_VIOLATED, "max_len", UInt64V(k.GetMaxLen()))
	}

	if len(k.GetPrefix()) > 0 && !bytes.HasPrefix(b, k.GetPrefix()) {
		add(ErrorCode_ERROR_CODE_PREFIX_MISMATCH, "prefix", BytesV(k.GetPrefix()))
	}

	if len(k.GetSuffix()) > 0 && !bytes.HasSuffix(b, k.GetSuffix()) {
		add(ErrorCode_ERROR_CODE_SUFFIX_MISMATCH, "suffix", BytesV(k.GetSuffix()))
	}

	if len(k.GetIn()) > 0 && !slices.ContainsFunc(k.GetIn(), func(x []byte) bool { return bytes.Equal(x, b) }) {
		add(ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET, "in", listOf(k.GetIn(), BytesV))
	}

	if len(k.GetNotIn()) > 0 && slices.ContainsFunc(k.GetNotIn(), func(x []byte) bool { return bytes.Equal(x, b) }) {
		add(ErrorCode_ERROR_CODE_IN_FORBIDDEN_SET, "not_in", listOf(k.GetNotIn(), BytesV))
	}

	return out
}

// checkChoice validates a choice value: membership in the allowed set (the
// static options, or the options_expr result which replaces them). An open
// choice treats the set as advisory and never fails membership.
func (e *Engine) checkChoice(path string, val any, k *Schema_Field_Choice, root map[string]any) []*ValidationError {
	if k.GetOpen() {
		return nil
	}

	actual, _ := FromGo(val)

	if src := k.GetOptionsExpr(); src != "" {
		allowed, err := e.evalChoiceOptions(src, root)
		if err != nil {
			return []*ValidationError{exprErr(path, src, "options_expr: "+err.Error())}
		}

		for _, a := range allowed {
			if nativeEqual(val, a) {
				return nil
			}
		}

		expected := make([]*Value, 0, len(allowed))

		for _, a := range allowed {
			v, _ := FromGo(a)
			expected = append(expected, v)
		}

		return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_CHOICE_NOT_ALLOWED, "options_expr",
			ListV(expected...), actual)}
	}

	expected := make([]*Value, 0, len(k.GetOptions()))

	for _, o := range k.GetOptions() {
		if nativeEqual(val, o.GetValue().ToGo()) {
			return nil
		}

		expected = append(expected, o.GetValue())
	}

	return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_CHOICE_NOT_ALLOWED, "options",
		ListV(expected...), actual)}
}

// asDuration accepts the native representation or a parseable string.
func asDuration(val any) (time.Duration, bool) {
	switch t := val.(type) {
	case time.Duration:
		return t, true
	case string:
		d, err := time.ParseDuration(t)

		return d, err == nil
	}

	return 0, false
}

func checkDuration(path string, val any, k *Schema_Field_Duration) []*ValidationError {
	d, ok := asDuration(val)
	if !ok {
		return []*ValidationError{typeErr(path, "duration", val)}
	}

	var out []*ValidationError

	add := func(code ErrorCode, constraint string, bound time.Duration) {
		out = append(out, verr(path, code, constraint, DurationV(bound), DurationV(d)))
	}
	if k.GetGt() != nil && d <= k.GetGt().AsDuration() {
		add(ErrorCode_ERROR_CODE_GT_VIOLATED, "gt", k.GetGt().AsDuration())
	}

	if k.GetGte() != nil && d < k.GetGte().AsDuration() {
		add(ErrorCode_ERROR_CODE_GTE_VIOLATED, "gte", k.GetGte().AsDuration())
	}

	if k.GetLt() != nil && d >= k.GetLt().AsDuration() {
		add(ErrorCode_ERROR_CODE_LT_VIOLATED, "lt", k.GetLt().AsDuration())
	}

	if k.GetLte() != nil && d > k.GetLte().AsDuration() {
		add(ErrorCode_ERROR_CODE_LTE_VIOLATED, "lte", k.GetLte().AsDuration())
	}

	return out
}

// asTimestamp accepts the native representation or an RFC3339 string.
func asTimestamp(val any) (time.Time, bool) {
	switch t := val.(type) {
	case time.Time:
		return t, true
	case string:
		ts, err := time.Parse(time.RFC3339, t)

		return ts, err == nil
	}

	return time.Time{}, false
}

func checkTimestamp(path string, val any, k *Schema_Field_Timestamp) []*ValidationError {
	ts, ok := asTimestamp(val)
	if !ok {
		return []*ValidationError{typeErr(path, "timestamp (RFC3339)", val)}
	}

	var out []*ValidationError

	add := func(code ErrorCode, constraint string, bound time.Time) {
		out = append(out, verr(path, code, constraint, TimestampV(bound), TimestampV(ts)))
	}
	if k.GetGt() != nil && !ts.After(k.GetGt().AsTime()) {
		add(ErrorCode_ERROR_CODE_GT_VIOLATED, "gt", k.GetGt().AsTime())
	}

	if k.GetGte() != nil && ts.Before(k.GetGte().AsTime()) {
		add(ErrorCode_ERROR_CODE_GTE_VIOLATED, "gte", k.GetGte().AsTime())
	}

	if k.GetLt() != nil && !ts.Before(k.GetLt().AsTime()) {
		add(ErrorCode_ERROR_CODE_LT_VIOLATED, "lt", k.GetLt().AsTime())
	}

	if k.GetLte() != nil && ts.After(k.GetLte().AsTime()) {
		add(ErrorCode_ERROR_CODE_LTE_VIOLATED, "lte", k.GetLte().AsTime())
	}

	return out
}

// =============================================================================
// Container checks
// =============================================================================

//nolint:cyclop // flat constraint sequence in spec order
func (e *Engine) checkList(path string, arr []any, l *Schema_Field_List, root map[string]any) []*ValidationError {
	// Tuple semantics: element i validates against items[i], the length must
	// equal the tuple size exactly; the homogeneous-list constraints
	// (min/max/unique/count_expr) are rejected by the descriptor check.
	if isTuple(l) {
		var out []*ValidationError

		want := len(l.GetItems())
		if len(arr) != want {
			out = append(out, verr(path, ErrorCode_ERROR_CODE_LIST_COUNT_MISMATCH, "tuple",
				Int64V(int64(want)), Int64V(int64(len(arr)))))
		}

		sub := &ValidationResult{}
		for i := range min(len(arr), want) {
			e.validateOne(l.GetItems()[i], arr[i], true,
				fmt.Sprintf("%s[%d]", path, i), root, map[string]any{"index": int64(i)}, sub)
		}

		return append(out, sub.GetErrors()...)
	}

	var out []*ValidationError

	n := uint64(len(arr))
	if l.MinItems != nil && n < l.GetMinItems() {
		out = append(out, verr(path, ErrorCode_ERROR_CODE_MIN_ITEMS_VIOLATED, "min_items",
			UInt64V(l.GetMinItems()), UInt64V(n)))
	}

	if l.MaxItems != nil && n > l.GetMaxItems() {
		out = append(out, verr(path, ErrorCode_ERROR_CODE_MAX_ITEMS_VIOLATED, "max_items",
			UInt64V(l.GetMaxItems()), UInt64V(n)))
	}

	if ce := l.GetCountExpr(); ce != "" {
		want, err := e.evalCount(ce, root)
		if err != nil {
			out = append(out, exprErr(path, ce, "count_expr: "+err.Error()))
		} else if want < 0 || uint64(want) != n {
			out = append(out, verr(path, ErrorCode_ERROR_CODE_LIST_COUNT_MISMATCH, "count_expr",
				Int64V(want), UInt64V(n)))
		}
	}

	if l.GetUnique() {
		seen := map[string]bool{}

		for i, el := range arr {
			key := uniqueKey(el)
			if seen[key] {
				actual, _ := FromGo(el)
				out = append(out, verr(fmt.Sprintf("%s[%d]", path, i), ErrorCode_ERROR_CODE_NOT_UNIQUE,
					"unique", nil, actual))
			}

			seen[key] = true
		}
	}

	if items := l.GetItems(); len(items) >= 1 {
		def := items[0]
		sub := &ValidationResult{}

		for i, el := range arr {
			e.validateOne(def, el, true, fmt.Sprintf("%s[%d]", path, i), root, map[string]any{"index": int64(i)}, sub)
		}

		out = append(out, sub.GetErrors()...)
	}

	return out
}

// uniqueKey serializes a native value for uniqueness comparison.
func uniqueKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}

	return string(b)
}

// checkMap validates a Map field: entry-count bounds, then each value against
// the shared value_schema (nil => values accepted unvalidated). Keys are never
// rejected; they are visited in sorted order for deterministic output.
func (e *Engine) checkMap(path string, m map[string]any, mk *Schema_Field_Map, root map[string]any) []*ValidationError {
	var out []*ValidationError

	n := uint64(len(m))
	if mk.MinEntries != nil && n < mk.GetMinEntries() {
		out = append(out, verr(path, ErrorCode_ERROR_CODE_MIN_ENTRIES_VIOLATED, "min_entries",
			UInt64V(mk.GetMinEntries()), UInt64V(n)))
	}

	if mk.MaxEntries != nil && n > mk.GetMaxEntries() {
		out = append(out, verr(path, ErrorCode_ERROR_CODE_MAX_ENTRIES_VIOLATED, "max_entries",
			UInt64V(mk.GetMaxEntries()), UInt64V(n)))
	}

	vs := mk.GetValueSchema()
	if vs == nil {
		return out
	}

	for _, k := range slices.Sorted(maps.Keys(m)) {
		vpath := joinPath(path, k)
		vm, ok := m[k].(map[string]any)

		if !ok {
			out = append(out, typeErr(vpath, "object", m[k]))

			continue
		}

		sub := &ValidationResult{}
		e.validateFields(vs, vm, root, vpath, sub)

		for _, r := range vs.GetRules() {
			e.evalRule(r, vpath, vm, root, nil, sub)
		}

		out = append(out, sub.GetErrors()...)
	}

	return out
}

func (e *Engine) checkOneOf(
	path string, m map[string]any, oo *Schema_Field_OneOf, root map[string]any,
) []*ValidationError {
	disc := oo.GetDiscriminator()
	discVal, present := m[disc]
	discStr, isStr := discVal.(string)

	if !present || !isStr || discStr == "" {
		return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_DISCRIMINATOR_MISSING, "discriminator",
			StrV(disc), nil)}
	}

	variant, known := oo.GetVariants()[discStr]
	if !known {
		keys := slices.Sorted(maps.Keys(oo.GetVariants()))

		return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_UNKNOWN_VARIANT, "variants",
			listOf(keys, StrV), StrV(discStr))}
	}

	sub := &ValidationResult{}
	e.validateFields(variant, m, root, path, sub)

	for _, r := range variant.GetRules() {
		e.evalRule(r, path, m, root, nil, sub)
	}

	return sub.GetErrors()
}

func (e *Engine) checkRef(path string, val any, ref *Schema_Field_Ref, root map[string]any) []*ValidationError {
	key := refDefKey(ref)
	def := e.schema.GetDefs()[key]

	if def == nil {
		label := key
		if id := ref.GetId(); id != nil {
			label = identityString(id) + " (unlinked identity-ref — call Schema.Link)"
		}

		return []*ValidationError{verr(path, ErrorCode_ERROR_CODE_UNKNOWN_REF, "ref",
			StrV(label), nil)}
	}

	m, ok := val.(map[string]any)
	if !ok {
		return []*ValidationError{typeErr(path, "object", val)}
	}

	sub := &ValidationResult{}
	e.validateFields(def, m, root, path, sub)

	for _, r := range def.GetRules() {
		e.evalRule(r, path, m, root, nil, sub)
	}

	return sub.GetErrors()
}

// nativeEqual compares two native values structurally.
func nativeEqual(a, b any) bool {
	if an, ok := asFloat64(a); ok {
		if bn, bothNum := asFloat64(b); bothNum {
			return an == bn
		}

		return false
	}

	return reflect.DeepEqual(a, b)
}
