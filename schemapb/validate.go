package schemapb

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"google.golang.org/protobuf/types/known/structpb"
)

// Validator validates form values against a Schema and evaluates its derived
// (Computed) fields. It compiles every expr-lang expression (Rule + Computed)
// once at construction time and reuses the programs. Structured per-kind
// constraints (gt/gte/in/pattern/multiple_of/...) are checked in Go; custom Rule
// and Computed expressions are evaluated by expr-lang.
type Validator struct {
	schema   *Schema
	programs map[string]*vm.Program
	// computeOrder holds top-level Computed fields in dependency order.
	computeOrder []*Schema_Filed
}

// SchemaError reports that a Schema descriptor is itself malformed. It carries
// the individual problems as FieldError values.
type SchemaError struct {
	Errors []*FieldError
}

func (e *SchemaError) Error() string {
	parts := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		parts[i] = fe.GetField() + ": " + fe.GetMessage()
	}
	return "invalid schema: " + strings.Join(parts, "; ")
}

// NewValidator validates the descriptor (see ValidateSchema) and compiles its
// expressions. It returns a *SchemaError if the schema is malformed.
func NewValidator(s *Schema) (*Validator, error) {
	if errs := ValidateSchema(s); len(errs) > 0 {
		return nil, &SchemaError{Errors: errs}
	}
	v := &Validator{schema: s, programs: map[string]*vm.Program{}}
	if err := v.compileSchema(s); err != nil {
		return nil, &SchemaError{Errors: []*FieldError{schemaErr("", "expr: "+err.Error())}}
	}
	order, err := buildComputeOrder(s.GetFields())
	if err != nil {
		return nil, &SchemaError{Errors: []*FieldError{schemaErr("", err.Error())}}
	}
	v.computeOrder = order
	return v, nil
}

// ValidateStruct validates a google.protobuf.Struct against the schema and
// returns one FieldError per failure (empty if valid).
func (v *Validator) ValidateStruct(s *structpb.Struct) []*FieldError {
	var m map[string]any
	if s != nil {
		m = s.AsMap()
	} else {
		m = map[string]any{}
	}
	return v.validate(m)
}

// ValidateJSON validates a raw JSON object against the schema. It returns an
// error only if the JSON cannot be parsed into an object.
func (v *Validator) ValidateJSON(raw json.RawMessage) ([]*FieldError, error) {
	m := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
	}
	return v.validate(m), nil
}

// ValidateStruct is a one-shot helper: it builds a Validator and validates s.
func ValidateStruct(schema *Schema, s *structpb.Struct) ([]*FieldError, error) {
	v, err := NewValidator(schema)
	if err != nil {
		return nil, err
	}
	return v.ValidateStruct(s), nil
}

// ValidateJSON is a one-shot helper: it builds a Validator and validates raw.
func ValidateJSON(schema *Schema, raw json.RawMessage) ([]*FieldError, error) {
	v, err := NewValidator(schema)
	if err != nil {
		return nil, err
	}
	return v.ValidateJSON(raw)
}

// =============================================================================
// Expression compilation
// =============================================================================

func (v *Validator) compileSchema(s *Schema) error {
	for _, f := range s.GetFields() {
		if err := v.compileField(f); err != nil {
			return err
		}
	}
	for _, r := range s.GetRules() {
		if err := v.addProgram(r.GetExpr()); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) compileField(f *Schema_Filed) error {
	for _, r := range f.GetRules() {
		if err := v.addProgram(r.GetExpr()); err != nil {
			return err
		}
	}
	if c := f.GetComputed(); c != nil {
		if err := v.addProgram(c.GetExpr()); err != nil {
			return err
		}
	}
	if l := f.GetList(); l != nil {
		for _, it := range l.GetItems() {
			if err := v.compileField(it); err != nil {
				return err
			}
		}
	}
	if o := f.GetObject(); o != nil && o.GetSchema() != nil {
		if err := v.compileSchema(o.GetSchema()); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) addProgram(code string) error {
	if code == "" || v.programs[code] != nil {
		return nil
	}
	prg, err := expr.Compile(code)
	if err != nil {
		return err
	}
	v.programs[code] = prg
	return nil
}

// =============================================================================
// Value validation
// =============================================================================

func (v *Validator) validate(form map[string]any) []*FieldError {
	// Derived values first, injected into form so rules and other computed
	// fields can read them via root.
	out := v.compute(form)
	out = append(out, v.validateFields(v.schema.GetFields(), form, form, "")...)
	for _, r := range v.schema.GetRules() {
		out = append(out, v.evalRule(r, ruleScope(r), nil, form)...)
	}
	return out
}

func (v *Validator) validateFields(fields []*Schema_Filed, scope, root map[string]any, prefix string) []*FieldError {
	var out []*FieldError
	for _, f := range fields {
		val, exists := scope[f.GetName()]
		out = append(out, v.validateOne(f, val, exists, join(prefix, f.GetName()), root)...)
	}
	return out
}

func (v *Validator) validateOne(f *Schema_Filed, val any, exists bool, path string, root map[string]any) []*FieldError {
	if !exists {
		if f.GetRequired() {
			return []*FieldError{schemaErr(path, "required")}
		}
		return nil
	}
	if val == nil {
		switch {
		case f.GetRequired():
			return []*FieldError{schemaErr(path, "required")}
		case !f.GetNullable():
			return []*FieldError{schemaErr(path, "must not be null")}
		default:
			return nil
		}
	}

	out := v.checkKind(f, val, path, root)
	for _, r := range f.GetRules() {
		out = append(out, v.evalRule(r, path, val, root)...)
	}
	return out
}

func (v *Validator) checkKind(f *Schema_Filed, val any, path string, root map[string]any) []*FieldError {
	switch {
	case f.GetFloat() != nil:
		return numericCheck(path, val, numFromFloat(f.GetFloat()))
	case f.GetDouble() != nil:
		return numericCheck(path, val, numFromDouble(f.GetDouble()))
	case f.GetInt32() != nil:
		return numericCheck(path, val, numFromInt32(f.GetInt32()))
	case f.GetInt64() != nil:
		return numericCheck(path, val, numFromInt64(f.GetInt64()))
	case f.GetUint32() != nil:
		return numericCheck(path, val, numFromUint32(f.GetUint32()))
	case f.GetUint64() != nil:
		return numericCheck(path, val, numFromUint64(f.GetUint64()))
	case f.GetBool() != nil:
		b, ok := val.(bool)
		if !ok {
			return typeErr(path, "bool")
		}
		return checkBool(path, b, f.GetBool())
	case f.GetString_() != nil:
		s, ok := val.(string)
		if !ok {
			return typeErr(path, "string")
		}
		return checkString(path, s, f.GetString_())
	case f.GetEnum() != nil:
		return checkEnum(path, val, f.GetEnum())
	case f.GetDuration() != nil:
		s, ok := val.(string)
		if !ok {
			return typeErr(path, "duration string")
		}
		return checkDuration(path, s, f.GetDuration())
	case f.GetTimestamp() != nil:
		s, ok := val.(string)
		if !ok {
			return typeErr(path, "timestamp string")
		}
		return checkTimestamp(path, s, f.GetTimestamp())
	case f.GetList() != nil:
		arr, ok := val.([]any)
		if !ok {
			return typeErr(path, "array")
		}
		return v.checkList(path, arr, f.GetList(), root)
	case f.GetObject() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return typeErr(path, "object")
		}
		return v.checkObject(path, m, f.GetObject(), root)
	case f.GetComputed() != nil:
		// Derived value: no structured constraints. Its Rules (if any) run in
		// validateOne as sanity checks.
		return nil
	}
	return nil
}

func (v *Validator) checkObject(path string, m map[string]any, o *Schema_Filed_Object, root map[string]any) []*FieldError {
	s := o.GetSchema()
	if s == nil {
		return nil
	}
	out := v.validateFields(s.GetFields(), m, root, path)
	for _, r := range s.GetRules() {
		out = append(out, v.evalRule(r, path, m, root)...)
	}
	return out
}

func (v *Validator) checkList(path string, arr []any, l *Schema_Filed_List, root map[string]any) []*FieldError {
	var out []*FieldError
	n := uint64(len(arr))
	if l.MinItems != nil && n < *l.MinItems {
		out = append(out, schemaErr(path, fmt.Sprintf("must have at least %d items", *l.MinItems)))
	}
	if l.MaxItems != nil && n > *l.MaxItems {
		out = append(out, schemaErr(path, fmt.Sprintf("must have at most %d items", *l.MaxItems)))
	}
	if l.GetUnique() {
		seen := map[string]bool{}
		for i, el := range arr {
			key, _ := json.Marshal(el)
			if seen[string(key)] {
				out = append(out, schemaErr(fmt.Sprintf("%s[%d]", path, i), "must be unique"))
			}
			seen[string(key)] = true
		}
	}
	if items := l.GetItems(); len(items) >= 1 {
		def := items[0]
		for i, el := range arr {
			out = append(out, v.validateOne(def, el, true, fmt.Sprintf("%s[%d]", path, i), root)...)
		}
	}
	return out
}

func (v *Validator) evalRule(r *Schema_Filed_Rule, path string, this any, root map[string]any) []*FieldError {
	prg, ok := v.programs[r.GetExpr()]
	if !ok {
		return nil
	}
	out, err := expr.Run(prg, map[string]any{"this": this, "root": root})
	if err != nil {
		return []*FieldError{ferr(path, "rule error: "+err.Error(), Schema_Filed_ERROR, r.Id)}
	}
	if b, ok := out.(bool); ok && b {
		return nil
	}
	return []*FieldError{ferr(path, r.GetMessage(), r.GetSeverity(), r.Id)}
}

// =============================================================================
// Per-kind structured checks
// =============================================================================

// numRules is the common numeric constraint set, lowered to float64.
type numRules struct {
	cst, gt, gte, lt, lte, mul *float64
	in, notIn                  []float64
	isInt                      bool
}

func numericCheck(path string, val any, r numRules) []*FieldError {
	n, ok := val.(float64)
	if !ok {
		return typeErr(path, "number")
	}
	var out []*FieldError
	add := func(m string) { out = append(out, schemaErr(path, m)) }

	if r.isInt && n != math.Trunc(n) {
		add("must be an integer")
	}
	if r.cst != nil && n != *r.cst {
		add(fmt.Sprintf("must equal %v", *r.cst))
	}
	if r.gt != nil && !(n > *r.gt) {
		add(fmt.Sprintf("must be > %v", *r.gt))
	}
	if r.gte != nil && !(n >= *r.gte) {
		add(fmt.Sprintf("must be >= %v", *r.gte))
	}
	if r.lt != nil && !(n < *r.lt) {
		add(fmt.Sprintf("must be < %v", *r.lt))
	}
	if r.lte != nil && !(n <= *r.lte) {
		add(fmt.Sprintf("must be <= %v", *r.lte))
	}
	if len(r.in) > 0 && !contains(r.in, n) {
		add(fmt.Sprintf("must be one of %v", r.in))
	}
	if len(r.notIn) > 0 && contains(r.notIn, n) {
		add(fmt.Sprintf("must not be one of %v", r.notIn))
	}
	if r.mul != nil && *r.mul != 0 {
		if r.isInt {
			if int64(n)%int64(*r.mul) != 0 {
				add(fmt.Sprintf("must be a multiple of %v", *r.mul))
			}
		} else if math.Mod(n, *r.mul) != 0 {
			add(fmt.Sprintf("must be a multiple of %v", *r.mul))
		}
	}
	return out
}

func checkBool(path string, b bool, k *Schema_Filed_Bool) []*FieldError {
	if k.Const != nil && b != *k.Const {
		return []*FieldError{schemaErr(path, fmt.Sprintf("must be %v", *k.Const))}
	}
	return nil
}

func checkString(path, s string, k *Schema_Filed_String) []*FieldError {
	var out []*FieldError
	add := func(m string) { out = append(out, schemaErr(path, m)) }
	n := uint64(utf8.RuneCountInString(s))

	if k.Const != nil && s != *k.Const {
		add(fmt.Sprintf("must equal %q", *k.Const))
	}
	if k.Len != nil && n != *k.Len {
		add(fmt.Sprintf("must be exactly %d characters", *k.Len))
	}
	if k.MinLen != nil && n < *k.MinLen {
		add(fmt.Sprintf("must be at least %d characters", *k.MinLen))
	}
	if k.MaxLen != nil && n > *k.MaxLen {
		add(fmt.Sprintf("must be at most %d characters", *k.MaxLen))
	}
	if k.Pattern != nil {
		if re, err := regexp.Compile(*k.Pattern); err == nil && !re.MatchString(s) {
			add("must match pattern " + *k.Pattern)
		}
	}
	if len(k.In) > 0 && !contains(k.In, s) {
		add(fmt.Sprintf("must be one of %v", k.In))
	}
	if len(k.NotIn) > 0 && contains(k.NotIn, s) {
		add(fmt.Sprintf("must not be one of %v", k.NotIn))
	}
	return out
}

func checkEnum(path string, val any, k *Schema_Filed_Enum) []*FieldError {
	n, ok := val.(float64)
	if !ok {
		return typeErr(path, "enum (number)")
	}
	iv := int32(n)
	var out []*FieldError
	add := func(m string) { out = append(out, schemaErr(path, m)) }

	if n != math.Trunc(n) {
		add("must be an integer enum value")
	}
	if k.GetDefinedOnly() {
		if _, ok := k.GetValues()[iv]; !ok {
			add("must be a defined enum value")
		}
	}
	if len(k.In) > 0 && !contains(k.In, iv) {
		add(fmt.Sprintf("must be one of %v", k.In))
	}
	if len(k.NotIn) > 0 && contains(k.NotIn, iv) {
		add(fmt.Sprintf("must not be one of %v", k.NotIn))
	}
	return out
}

func checkDuration(path, s string, k *Schema_Filed_Duration) []*FieldError {
	d, err := time.ParseDuration(s)
	if err != nil {
		return []*FieldError{schemaErr(path, "invalid duration: "+err.Error())}
	}
	var out []*FieldError
	add := func(m string) { out = append(out, schemaErr(path, m)) }
	if k.Gt != nil && !(d > k.Gt.AsDuration()) {
		add(fmt.Sprintf("must be > %s", k.Gt.AsDuration()))
	}
	if k.Gte != nil && !(d >= k.Gte.AsDuration()) {
		add(fmt.Sprintf("must be >= %s", k.Gte.AsDuration()))
	}
	if k.Lt != nil && !(d < k.Lt.AsDuration()) {
		add(fmt.Sprintf("must be < %s", k.Lt.AsDuration()))
	}
	if k.Lte != nil && !(d <= k.Lte.AsDuration()) {
		add(fmt.Sprintf("must be <= %s", k.Lte.AsDuration()))
	}
	return out
}

func checkTimestamp(path, s string, k *Schema_Filed_Timestamp) []*FieldError {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return []*FieldError{schemaErr(path, "invalid timestamp (want RFC3339): "+err.Error())}
	}
	var out []*FieldError
	add := func(m string) { out = append(out, schemaErr(path, m)) }
	if k.Gt != nil && !t.After(k.Gt.AsTime()) {
		add(fmt.Sprintf("must be after %s", k.Gt.AsTime().Format(time.RFC3339)))
	}
	if k.Gte != nil && t.Before(k.Gte.AsTime()) {
		add(fmt.Sprintf("must be at or after %s", k.Gte.AsTime().Format(time.RFC3339)))
	}
	if k.Lt != nil && !t.Before(k.Lt.AsTime()) {
		add(fmt.Sprintf("must be before %s", k.Lt.AsTime().Format(time.RFC3339)))
	}
	if k.Lte != nil && t.After(k.Lte.AsTime()) {
		add(fmt.Sprintf("must be at or before %s", k.Lte.AsTime().Format(time.RFC3339)))
	}
	return out
}

// =============================================================================
// Self-schema validation
// =============================================================================

// ValidateSchema checks that the descriptor itself is well-formed: every field
// is named (and unique within its level), exactly one kind is set, rule
// expressions are non-empty and compile, patterns are valid, etc. It returns
// one FieldError per problem (empty if the schema is valid).
func ValidateSchema(s *Schema) []*FieldError {
	if s == nil {
		return []*FieldError{schemaErr("", "schema is nil")}
	}
	var out []*FieldError
	if s.GetId() == nil || s.GetId().GetName() == "" {
		out = append(out, schemaErr("id", "schema identity is required: id.name must be set"))
	}
	out = append(out, validateSchemaFields(s.GetFields(), "")...)
	for i, r := range s.GetRules() {
		out = append(out, validateRuleDef(r, fmt.Sprintf("rules[%d]", i))...)
	}
	if _, err := buildComputeOrder(s.GetFields()); err != nil {
		out = append(out, schemaErr("", err.Error()))
	}
	return out
}

func validateSchemaFields(fields []*Schema_Filed, prefix string) []*FieldError {
	var out []*FieldError
	seen := map[string]bool{}
	for i, f := range fields {
		name := f.GetName()
		path := join(prefix, name)
		if name == "" {
			path = join(prefix, fmt.Sprintf("fields[%d]", i))
			out = append(out, schemaErr(path, "field name is required"))
		} else if seen[name] {
			out = append(out, schemaErr(path, "duplicate field name"))
		}
		seen[name] = true

		if f.GetKind() == nil {
			out = append(out, schemaErr(path, "exactly one field kind must be set"))
		}
		if e := f.GetEnum(); e != nil && e.GetDefinedOnly() && len(e.GetValues()) == 0 {
			out = append(out, schemaErr(path, "enum with defined_only requires values"))
		}
		if st := f.GetString_(); st != nil && st.Pattern != nil {
			if _, err := regexp.Compile(*st.Pattern); err != nil {
				out = append(out, schemaErr(path, "invalid pattern: "+err.Error()))
			}
		}
		if c := f.GetComputed(); c != nil {
			if c.GetExpr() == "" {
				out = append(out, schemaErr(path, "computed field requires an expr"))
			} else if _, err := expr.Compile(c.GetExpr()); err != nil {
				out = append(out, schemaErr(path, "computed expr does not compile: "+err.Error()))
			}
		}
		if l := f.GetList(); l != nil {
			if len(l.GetItems()) == 0 {
				out = append(out, schemaErr(path, "list requires at least one item definition"))
			} else {
				out = append(out, validateSchemaFields(l.GetItems(), path+"[]")...)
			}
		}
		if o := f.GetObject(); o != nil {
			if o.GetSchema() == nil {
				out = append(out, schemaErr(path, "object requires a schema"))
			} else {
				out = append(out, validateSchemaFields(o.GetSchema().GetFields(), path)...)
				for j, r := range o.GetSchema().GetRules() {
					out = append(out, validateRuleDef(r, fmt.Sprintf("%s.rules[%d]", path, j))...)
				}
			}
		}
		for j, r := range f.GetRules() {
			out = append(out, validateRuleDef(r, fmt.Sprintf("%s.rules[%d]", path, j))...)
		}
	}
	return out
}

func validateRuleDef(r *Schema_Filed_Rule, path string) []*FieldError {
	if r.GetExpr() == "" {
		return []*FieldError{schemaErr(path, "rule expr is required")}
	}
	if _, err := expr.Compile(r.GetExpr()); err != nil {
		return []*FieldError{schemaErr(path, "rule does not compile: "+err.Error())}
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func schemaErr(field, msg string) *FieldError {
	return &FieldError{Field: field, Message: msg, Severity: Schema_Filed_ERROR}
}

func ferr(field, msg string, sev Schema_Filed_Severity, ruleID *string) *FieldError {
	if sev == Schema_Filed_SEVERITY_UNSPECIFIED {
		sev = Schema_Filed_ERROR
	}
	return &FieldError{Field: field, Message: msg, Severity: sev, RuleId: ruleID}
}

func typeErr(path, want string) []*FieldError {
	return []*FieldError{schemaErr(path, "expected "+want)}
}

func ruleScope(r *Schema_Filed_Rule) string {
	if id := r.GetId(); id != "" {
		return id
	}
	return ""
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func contains[T comparable](s []T, v T) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func toF64[T ~float32 | ~float64 | ~int32 | ~int64 | ~uint32 | ~uint64](p *T) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

func toF64s[T ~float32 | ~float64 | ~int32 | ~int64 | ~uint32 | ~uint64](s []T) []float64 {
	if len(s) == 0 {
		return nil
	}
	out := make([]float64, len(s))
	for i, x := range s {
		out[i] = float64(x)
	}
	return out
}

func numFromFloat(k *Schema_Filed_Float) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn)}
}

func numFromDouble(k *Schema_Filed_Double) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn)}
}

func numFromInt32(k *Schema_Filed_Int32) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn), isInt: true}
}

func numFromInt64(k *Schema_Filed_Int64) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn), isInt: true}
}

func numFromUint32(k *Schema_Filed_UInt32) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn), isInt: true}
}

func numFromUint64(k *Schema_Filed_UInt64) numRules {
	return numRules{cst: toF64(k.Const), gt: toF64(k.Gt), gte: toF64(k.Gte), lt: toF64(k.Lt), lte: toF64(k.Lte), mul: toF64(k.MultipleOf), in: toF64s(k.In), notIn: toF64s(k.NotIn), isInt: true}
}
