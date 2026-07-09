package schemapb

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"google.golang.org/protobuf/types/known/structpb"
)

// validator holds a schema's compiled expr-lang programs (Rule + Computed). It
// is built once per distinct schema and cached by the schema's content hash, so
// the public API is methods on *Schema — no validator handle to pass around.
type validator struct {
	schema   *Schema
	programs map[string]*vm.Program
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

// Hash returns the SHA-256 of a message's content (via its generated HashPB).
// Equal messages hash equal — use it to compare messages or key by content.
func Hash(m interface {
	HashPB(hash.Hash, map[string]struct{})
}) [32]byte {
	h := sha256.New()
	m.HashPB(h, nil)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// compiledCache maps a schema content hash to its compiled programs.
var compiledCache sync.Map // [32]byte -> *validator

// compiled returns the schema's compiled programs, cached by content hash. It
// errors only if an expression fails to compile.
func (s *Schema) compiled() (*validator, error) {
	key := Hash(s)
	if v, ok := compiledCache.Load(key); ok {
		return v.(*validator), nil
	}
	v := &validator{schema: s, programs: map[string]*vm.Program{}}
	if err := v.compileSchema(s); err != nil {
		return nil, err
	}
	compiledCache.Store(key, v)
	return v, nil
}

// Validate checks form values against the schema: it seeds defaults, resolves
// Computed fields, then runs structured and expr rules. Empty result = valid.
func (s *Schema) Validate(values map[string]any) []*FieldError {
	v, err := s.compiled()
	if err != nil {
		return []*FieldError{schemaErr("", "expr: "+err.Error())}
	}
	return v.validate(values)
}

// ValidateStruct validates a google.protobuf.Struct.
func (s *Schema) ValidateStruct(st *structpb.Struct) []*FieldError {
	m := map[string]any{}
	if st != nil {
		m = st.AsMap()
	}
	return s.Validate(m)
}

// ValidateJSON validates a raw JSON object (error only on parse failure).
func (s *Schema) ValidateJSON(raw json.RawMessage) ([]*FieldError, error) {
	m := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
	}
	return s.Validate(m), nil
}

// =============================================================================
// Expression compilation
// =============================================================================

func (v *validator) compileSchema(s *Schema) error {
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
	// Compile expressions inside each named def (only meaningful at root, but
	// we compile whatever schema we're given to be safe).
	for _, def := range s.GetDefs() {
		if err := v.compileSchema(def); err != nil {
			return err
		}
	}
	return nil
}

func (v *validator) compileField(f *Schema_Filed) error {
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
	if norm := f.GetNormalize(); norm != "" {
		if err := v.addProgram(norm); err != nil {
			return err
		}
	}
	if w := f.GetWhen(); w != "" {
		if err := v.addProgram(w); err != nil {
			return err
		}
	}
	if e := f.GetEnum(); e != nil && e.GetOptionsExpr() != "" {
		if err := v.addProgram(e.GetOptionsExpr()); err != nil {
			return err
		}
	}
	if l := f.GetList(); l != nil {
		if ce := l.GetCountExpr(); ce != "" {
			if err := v.addProgram(ce); err != nil {
				return err
			}
		}
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
	if mp := f.GetMap(); mp != nil && mp.GetValueSchema() != nil {
		if err := v.compileSchema(mp.GetValueSchema()); err != nil {
			return err
		}
	}
	if oo := f.GetOneOf(); oo != nil {
		for _, variant := range oo.GetVariants() {
			if err := v.compileSchema(variant); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *validator) addProgram(code string) error {
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

func (v *validator) validate(form map[string]any) []*FieldError {
	// Reject attempts to change immutable fields, checked on the raw input
	// before resolve forces them back to their defaults.
	out := v.checkImmutable(v.schema.GetFields(), form, "", form)
	// Resolve: fill defaults and evaluate Computed fields, so structured and
	// expr rules validate the fully resolved form.
	out = append(out, v.resolve(form)...)
	out = append(out, v.validateFields(v.schema, form, form, "")...)
	for _, r := range v.schema.GetRules() {
		out = append(out, v.evalRule(r, ruleScope(r), nil, form, nil)...)
	}
	return out
}

// active reports whether a field is active for the given root. An empty `when`
// is always active. A `when` that errors or yields a non-bool returns an error;
// callers treat an errored field as inactive (and surface the error in the
// validation phase). `this` is intentionally not bound: a field's own value
// must not gate its existence.
func (v *validator) active(f *Schema_Filed, root map[string]any) (bool, error) {
	w := f.GetWhen()
	if w == "" {
		return true, nil
	}
	prg, ok := v.programs[w]
	if !ok {
		return false, fmt.Errorf("when expression not compiled")
	}
	res, err := expr.Run(prg, map[string]any{"root": root})
	if err != nil {
		return false, err
	}
	b, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf("when must evaluate to bool, got %T", res)
	}
	return b, nil
}

// checkImmutable reports a submitted value that differs from an immutable
// field's default (a system-fixed value cannot be changed). It walks present
// objects and object-typed list elements. Only enforced when a default exists.
func (v *validator) checkImmutable(fields []*Schema_Filed, scope map[string]any, prefix string, root map[string]any) []*FieldError {
	var out []*FieldError
	for _, f := range fields {
		name := f.GetName()
		path := join(prefix, name)
		// Inactive fields (when=false) are treated as absent: skip them and
		// their subtree entirely.
		if act, err := v.active(f, root); err != nil || !act {
			continue
		}
		if f.GetImmutable() {
			if cur, ok := scope[name]; ok {
				if dv, has := defaultValue(f); has && cur != dv {
					out = append(out, codeErr(path, "immutable: cannot be changed", "immutable", nil))
				}
			}
			continue
		}
		if o := f.GetObject(); o != nil && o.GetSchema() != nil {
			if child, ok := scope[name].(map[string]any); ok {
				out = append(out, v.checkImmutable(o.GetSchema().GetFields(), child, path, root)...)
			}
		}
		if l := f.GetList(); l != nil && len(l.GetItems()) >= 1 {
			if o := l.GetItems()[0].GetObject(); o != nil && o.GetSchema() != nil {
				if arr, ok := scope[name].([]any); ok {
					for i, el := range arr {
						if m, ok := el.(map[string]any); ok {
							out = append(out, v.checkImmutable(o.GetSchema().GetFields(), m, fmt.Sprintf("%s[%d]", path, i), root)...)
						}
					}
				}
			}
		}
		if mp := f.GetMap(); mp != nil && mp.GetValueSchema() != nil {
			if mm, ok := scope[name].(map[string]any); ok {
				for k, el := range mm {
					if m, ok := el.(map[string]any); ok {
						out = append(out, v.checkImmutable(mp.GetValueSchema().GetFields(), m, join(path, k), root)...)
					}
				}
			}
		}
	}
	return out
}

func (v *validator) validateFields(schema *Schema, scope, root map[string]any, prefix string) []*FieldError {
	fields := schema.GetFields()
	var out []*FieldError

	// Evaluate `when` once per field: an errored field surfaces a "when" error
	// and is treated as inactive; an inactive field is skipped entirely (its
	// value key is ignored, not counted, and not validated).
	type fstate struct {
		active bool
		err    error
	}
	states := make(map[string]fstate, len(fields))
	inactive := make(map[string]bool, len(fields))
	for _, f := range fields {
		a, err := v.active(f, root)
		states[f.GetName()] = fstate{active: a, err: err}
		if err != nil || !a {
			inactive[f.GetName()] = true
		}
	}

	// Strict mode: reject unknown keys. Declared fields (active or not) are
	// always known, so an inactive field's value key never trips strict.
	if schema.GetStrict() {
		known := make(map[string]bool, len(fields))
		for _, f := range fields {
			known[f.GetName()] = true
		}
		for key := range scope {
			if !known[key] {
				p := join(prefix, key)
				out = append(out, codeErr(p, "unknown field: "+key, "unknown_field", map[string]string{"field": key}))
			}
		}
	}

	// min_properties / max_properties: inactive fields' present keys do not
	// count toward the present-property total.
	var n uint64
	for key := range scope {
		if inactive[key] {
			continue
		}
		n++
	}
	if schema.MinProperties != nil && n < *schema.MinProperties {
		out = append(out, codeErr(prefix, fmt.Sprintf("must have at least %d properties", *schema.MinProperties), "min_properties", map[string]string{"min": fmt.Sprintf("%d", *schema.MinProperties)}))
	}
	if schema.MaxProperties != nil && n > *schema.MaxProperties {
		out = append(out, codeErr(prefix, fmt.Sprintf("must have at most %d properties", *schema.MaxProperties), "max_properties", map[string]string{"max": fmt.Sprintf("%d", *schema.MaxProperties)}))
	}

	for _, f := range fields {
		st := states[f.GetName()]
		path := join(prefix, f.GetName())
		if st.err != nil {
			out = append(out, codeErr(path, "when error: "+st.err.Error(), "when", nil))
			continue
		}
		if !st.active {
			continue
		}
		val, exists := scope[f.GetName()]
		out = append(out, v.validateOne(f, val, exists, path, root, nil)...)
	}
	return out
}

func (v *validator) validateOne(f *Schema_Filed, val any, exists bool, path string, root, extra map[string]any) []*FieldError {
	if !exists {
		if f.GetRequired() {
			return []*FieldError{codeErr(path, "required", "required", nil)}
		}
		return nil
	}
	if val == nil {
		switch {
		case f.GetRequired():
			return []*FieldError{codeErr(path, "required", "required", nil)}
		case !f.GetNullable():
			return []*FieldError{codeErr(path, "must not be null", "not_null", nil)}
		default:
			return nil
		}
	}

	out := v.checkKind(f, val, path, root)
	for _, r := range f.GetRules() {
		out = append(out, v.evalRule(r, path, val, root, extra)...)
	}
	return out
}

func (v *validator) checkKind(f *Schema_Filed, val any, path string, root map[string]any) []*FieldError {
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
		return v.checkEnum(path, val, f.GetEnum(), root)
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
	case f.GetMap() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return typeErr(path, "object (map)")
		}
		return v.checkMap(path, m, f.GetMap(), root)
	case f.GetComputed() != nil:
		// Derived value: no structured constraints. Its Rules (if any) run in
		// validateOne as sanity checks.
		return nil
	case f.GetOneOf() != nil:
		m, ok := val.(map[string]any)
		if !ok {
			return typeErr(path, "object")
		}
		return v.checkOneOf(path, m, f.GetOneOf(), root)
	case f.GetRef() != nil:
		ref := f.GetRef()
		name := refDefKey(ref)
		def := v.schema.GetDefs()[name]
		if def == nil {
			label := name
			if id := ref.GetId(); id != nil {
				// Readable identity (the key embeds NUL separators); hint Link.
				label = identityString(id) + " (unlinked identity-ref — call Schema.Link)"
			}
			return []*FieldError{codeErr(path, "unknown $ref: "+label, "ref", map[string]string{"ref": name})}
		}
		m, ok := val.(map[string]any)
		if !ok {
			return typeErr(path, "object")
		}
		var out []*FieldError
		out = append(out, v.validateFields(def, m, root, path)...)
		for _, r := range def.GetRules() {
			out = append(out, v.evalRule(r, path, m, root, nil)...)
		}
		return out
	}
	return nil
}

func (v *validator) checkObject(path string, m map[string]any, o *Schema_Filed_Object, root map[string]any) []*FieldError {
	s := o.GetSchema()
	if s == nil {
		return nil
	}
	out := v.validateFields(s, m, root, path)
	for _, r := range s.GetRules() {
		out = append(out, v.evalRule(r, path, m, root, nil)...)
	}
	return out
}

// checkMap validates a Map field: entry-count bounds, then each value against
// the shared value_schema (nil value_schema => values accepted unvalidated).
// Keys are never rejected — only what's inside each value can be. Keys are
// visited in sorted order so error output is deterministic; the field path
// for a rejected key names the map key, e.g. "subnets.my-subnet-a.evil_key".
func (v *validator) checkMap(path string, m map[string]any, mk *Schema_Filed_Map, root map[string]any) []*FieldError {
	var out []*FieldError
	n := uint64(len(m))
	if mk.MinEntries != nil && n < *mk.MinEntries {
		out = append(out, codeErr(path, fmt.Sprintf("must have at least %d entries", *mk.MinEntries), "min_entries", map[string]string{"min": fmt.Sprintf("%d", *mk.MinEntries)}))
	}
	if mk.MaxEntries != nil && n > *mk.MaxEntries {
		out = append(out, codeErr(path, fmt.Sprintf("must have at most %d entries", *mk.MaxEntries), "max_entries", map[string]string{"max": fmt.Sprintf("%d", *mk.MaxEntries)}))
	}
	vs := mk.GetValueSchema()
	if vs == nil {
		return out
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		vpath := join(path, k)
		vm, ok := m[k].(map[string]any)
		if !ok {
			out = append(out, typeErr(vpath, "object")...)
			continue
		}
		out = append(out, v.validateFields(vs, vm, root, vpath)...)
		for _, r := range vs.GetRules() {
			out = append(out, v.evalRule(r, vpath, vm, root, nil)...)
		}
	}
	return out
}

func (v *validator) checkOneOf(path string, m map[string]any, oo *Schema_Filed_OneOf, root map[string]any) []*FieldError {
	disc := oo.GetDiscriminator()
	discVal, ok := m[disc]
	if !ok {
		return []*FieldError{codeErr(path, "discriminator field "+disc+" is missing", "oneof_discriminator", map[string]string{"discriminator": disc})}
	}
	discStr, ok := discVal.(string)
	if !ok || discStr == "" {
		return []*FieldError{codeErr(path, "discriminator field "+disc+" must be a non-empty string", "oneof_discriminator", map[string]string{"discriminator": disc})}
	}
	variant, ok := oo.GetVariants()[discStr]
	if !ok {
		return []*FieldError{codeErr(path, "unknown variant: "+discStr, "oneof_variant", map[string]string{"variant": discStr})}
	}
	// Validate object fields against the chosen variant schema.
	var out []*FieldError
	out = append(out, v.validateFields(variant, m, root, path)...)
	for _, r := range variant.GetRules() {
		out = append(out, v.evalRule(r, path, m, root, nil)...)
	}
	return out
}

func (v *validator) checkList(path string, arr []any, l *Schema_Filed_List, root map[string]any) []*FieldError {
	var out []*FieldError
	n := uint64(len(arr))
	if l.MinItems != nil && n < *l.MinItems {
		out = append(out, codeErr(path, fmt.Sprintf("must have at least %d items", *l.MinItems), "min_items", map[string]string{"min": fmt.Sprintf("%d", *l.MinItems)}))
	}
	if l.MaxItems != nil && n > *l.MaxItems {
		out = append(out, codeErr(path, fmt.Sprintf("must have at most %d items", *l.MaxItems), "max_items", map[string]string{"max": fmt.Sprintf("%d", *l.MaxItems)}))
	}
	// count_expr: the list length must equal the dynamic count over root.
	if ce := l.GetCountExpr(); ce != "" {
		want, err := v.evalCount(ce, root)
		if err != nil {
			out = append(out, codeErr(path, "count_expr error: "+err.Error(), "list_count_mismatch", nil))
		} else if uint64(want) != n {
			out = append(out, codeErr(path, fmt.Sprintf("must have exactly %d items", want), "list_count_mismatch", map[string]string{"count": fmt.Sprintf("%d", want), "actual": fmt.Sprintf("%d", n)}))
		}
	}
	if l.GetUnique() {
		seen := map[string]bool{}
		for i, el := range arr {
			key, _ := json.Marshal(el)
			if seen[string(key)] {
				out = append(out, codeErr(fmt.Sprintf("%s[%d]", path, i), "must be unique", "unique", nil))
			}
			seen[string(key)] = true
		}
	}
	if items := l.GetItems(); len(items) >= 1 {
		def := items[0]
		for i, el := range arr {
			errs := v.validateOne(def, el, true, fmt.Sprintf("%s[%d]", path, i), root, map[string]any{"index": float64(i)})
			for _, e := range errs {
				if e.Code == "" {
					e.Code = "item"
				}
			}
			out = append(out, errs...)
		}
	}
	return out
}

func (v *validator) evalRule(r *Schema_Filed_Rule, path string, this any, root, extra map[string]any) []*FieldError {
	prg, ok := v.programs[r.GetExpr()]
	if !ok {
		return nil
	}
	env := map[string]any{"this": this, "root": root}
	for k, val := range extra {
		env[k] = val
	}
	out, err := expr.Run(prg, env)
	if err != nil {
		e := ferr(path, "rule error: "+err.Error(), Schema_Filed_ERROR, r.Id)
		e.Code = "rule"
		return []*FieldError{e}
	}
	if b, ok := out.(bool); ok && b {
		return nil
	}
	e := ferr(path, r.GetMessage(), r.GetSeverity(), r.Id)
	e.Code = "rule"
	return []*FieldError{e}
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
	addc := func(m, code string, params map[string]string) {
		out = append(out, codeErr(path, m, code, params))
	}

	if r.isInt && n != math.Trunc(n) {
		addc("must be an integer", "integer", nil)
	}
	if r.cst != nil && n != *r.cst {
		addc(fmt.Sprintf("must equal %v", *r.cst), "const", map[string]string{"const": fmt.Sprintf("%v", *r.cst)})
	}
	if r.gt != nil && !(n > *r.gt) {
		addc(fmt.Sprintf("must be > %v", *r.gt), "gt", map[string]string{"gt": fmt.Sprintf("%v", *r.gt)})
	}
	if r.gte != nil && !(n >= *r.gte) {
		addc(fmt.Sprintf("must be >= %v", *r.gte), "gte", map[string]string{"gte": fmt.Sprintf("%v", *r.gte)})
	}
	if r.lt != nil && !(n < *r.lt) {
		addc(fmt.Sprintf("must be < %v", *r.lt), "lt", map[string]string{"lt": fmt.Sprintf("%v", *r.lt)})
	}
	if r.lte != nil && !(n <= *r.lte) {
		addc(fmt.Sprintf("must be <= %v", *r.lte), "lte", map[string]string{"lte": fmt.Sprintf("%v", *r.lte)})
	}
	if len(r.in) > 0 && !contains(r.in, n) {
		addc(fmt.Sprintf("must be one of %v", r.in), "in", nil)
	}
	if len(r.notIn) > 0 && contains(r.notIn, n) {
		addc(fmt.Sprintf("must not be one of %v", r.notIn), "not_in", nil)
	}
	if r.mul != nil && *r.mul != 0 {
		if r.isInt {
			if int64(n)%int64(*r.mul) != 0 {
				addc(fmt.Sprintf("must be a multiple of %v", *r.mul), "multiple_of", map[string]string{"multiple_of": fmt.Sprintf("%v", *r.mul)})
			}
		} else if math.Mod(n, *r.mul) != 0 {
			addc(fmt.Sprintf("must be a multiple of %v", *r.mul), "multiple_of", map[string]string{"multiple_of": fmt.Sprintf("%v", *r.mul)})
		}
	}
	return out
}

func checkBool(path string, b bool, k *Schema_Filed_Bool) []*FieldError {
	if k.Const != nil && b != *k.Const {
		return []*FieldError{codeErr(path, fmt.Sprintf("must be %v", *k.Const), "const", map[string]string{"const": fmt.Sprintf("%v", *k.Const)})}
	}
	return nil
}

// uuidRE matches a canonical UUID v4 string (case-insensitive).
var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$`)

// hostnameRE matches an RFC 1123 hostname label sequence.
var hostnameRE = regexp.MustCompile(`(?i)^([a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)

func checkString(path, s string, k *Schema_Filed_String) []*FieldError {
	var out []*FieldError
	addc := func(m, code string, params map[string]string) {
		out = append(out, codeErr(path, m, code, params))
	}
	n := uint64(utf8.RuneCountInString(s))

	if k.Const != nil && s != *k.Const {
		addc(fmt.Sprintf("must equal %q", *k.Const), "const", map[string]string{"const": *k.Const})
	}
	if k.Len != nil && n != *k.Len {
		addc(fmt.Sprintf("must be exactly %d characters", *k.Len), "len", map[string]string{"len": fmt.Sprintf("%d", *k.Len)})
	}
	if k.MinLen != nil && n < *k.MinLen {
		addc(fmt.Sprintf("must be at least %d characters", *k.MinLen), "min_len", map[string]string{"min": fmt.Sprintf("%d", *k.MinLen)})
	}
	if k.MaxLen != nil && n > *k.MaxLen {
		addc(fmt.Sprintf("must be at most %d characters", *k.MaxLen), "max_len", map[string]string{"max": fmt.Sprintf("%d", *k.MaxLen)})
	}
	if k.Pattern != nil {
		if re, err := regexp.Compile(*k.Pattern); err == nil && !re.MatchString(s) {
			addc("must match pattern "+*k.Pattern, "pattern", map[string]string{"pattern": *k.Pattern})
		}
	}
	if len(k.In) > 0 && !contains(k.In, s) {
		addc(fmt.Sprintf("must be one of %v", k.In), "in", nil)
	}
	if len(k.NotIn) > 0 && contains(k.NotIn, s) {
		addc(fmt.Sprintf("must not be one of %v", k.NotIn), "not_in", nil)
	}
	if k.Format != nil {
		if ferrs := checkStringFormat(path, s, *k.Format); len(ferrs) > 0 {
			out = append(out, ferrs...)
		}
	}
	return out
}

func checkStringFormat(path, s string, fmt_ Schema_Filed_String_StringFormat) []*FieldError {
	bad := func(fmtName string) []*FieldError {
		return []*FieldError{codeErr(path, "must be a valid "+fmtName, "format", map[string]string{"format": fmtName})}
	}
	switch fmt_ {
	case Schema_Filed_String_STRING_FORMAT_EMAIL:
		if _, err := mail.ParseAddress(s); err != nil {
			return bad("email")
		}
	case Schema_Filed_String_STRING_FORMAT_URL:
		if _, err := url.ParseRequestURI(s); err != nil {
			return bad("url")
		}
	case Schema_Filed_String_STRING_FORMAT_UUID:
		if !uuidRE.MatchString(s) {
			return bad("uuid")
		}
	case Schema_Filed_String_STRING_FORMAT_IPV4:
		addr, err := netip.ParseAddr(s)
		if err != nil || !addr.Is4() {
			return bad("ipv4")
		}
	case Schema_Filed_String_STRING_FORMAT_IPV6:
		addr, err := netip.ParseAddr(s)
		if err != nil || !addr.Is6() || addr.Is4In6() {
			return bad("ipv6")
		}
	case Schema_Filed_String_STRING_FORMAT_IP:
		if _, err := netip.ParseAddr(s); err != nil {
			return bad("ip")
		}
	case Schema_Filed_String_STRING_FORMAT_HOSTNAME:
		if !hostnameRE.MatchString(s) {
			return bad("hostname")
		}
	case Schema_Filed_String_STRING_FORMAT_DATE:
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return bad("date")
		}
	case Schema_Filed_String_STRING_FORMAT_TIME:
		if _, err := time.Parse("15:04:05", s); err != nil {
			return bad("time")
		}
	case Schema_Filed_String_STRING_FORMAT_DATETIME:
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return bad("datetime")
		}
	}
	return nil
}

// checkEnum validates an enum value. When the field carries options_expr, the
// allowed set is computed dynamically over root and REPLACES the static checks
// (defined_only/in/not_in); otherwise the static set applies.
func (v *validator) checkEnum(path string, val any, k *Schema_Filed_Enum, root map[string]any) []*FieldError {
	if k.GetOptionsExpr() == "" {
		return checkEnumStatic(path, val, k)
	}
	n, ok := val.(float64)
	if !ok {
		return typeErr(path, "enum (number)")
	}
	var out []*FieldError
	if n != math.Trunc(n) {
		out = append(out, codeErr(path, "must be an integer enum value", "integer", nil))
	}
	allowed, err := v.evalEnumOptions(k.GetOptionsExpr(), root)
	if err != nil {
		return append(out, codeErr(path, "options_expr error: "+err.Error(), "enum_not_allowed", nil))
	}
	if !contains(allowed, int32(n)) {
		out = append(out, codeErr(path, fmt.Sprintf("must be one of %v", allowed), "enum_not_allowed", nil))
	}
	return out
}

// evalEnumOptions runs an enum options_expr over root and returns the allowed
// integer values. The result must be a list of numbers.
func (v *validator) evalEnumOptions(code string, root map[string]any) ([]int32, error) {
	prg, ok := v.programs[code]
	if !ok {
		return nil, fmt.Errorf("options_expr not compiled")
	}
	res, err := expr.Run(prg, map[string]any{"root": root})
	if err != nil {
		return nil, err
	}
	list, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("options_expr must return a list, got %T", res)
	}
	out := make([]int32, 0, len(list))
	for _, el := range list {
		f, ok := toFloat(el)
		if !ok {
			return nil, fmt.Errorf("options_expr element must be a number, got %T", el)
		}
		out = append(out, int32(f))
	}
	return out, nil
}

// evalCount runs a list count_expr over root and returns the required length.
// The result must be a non-negative integer.
func (v *validator) evalCount(code string, root map[string]any) (int64, error) {
	prg, ok := v.programs[code]
	if !ok {
		return 0, fmt.Errorf("count_expr not compiled")
	}
	res, err := expr.Run(prg, map[string]any{"root": root})
	if err != nil {
		return 0, err
	}
	f, ok := toFloat(res)
	if !ok {
		return 0, fmt.Errorf("count_expr must return a number, got %T", res)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("count_expr must return an integer, got %v", f)
	}
	if f < 0 {
		return 0, fmt.Errorf("count_expr must be non-negative, got %v", f)
	}
	return int64(f), nil
}

// toFloat converts expr-lang numeric output to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func checkEnumStatic(path string, val any, k *Schema_Filed_Enum) []*FieldError {
	n, ok := val.(float64)
	if !ok {
		return typeErr(path, "enum (number)")
	}
	iv := int32(n)
	var out []*FieldError
	addc := func(m, code string, params map[string]string) {
		out = append(out, codeErr(path, m, code, params))
	}

	if n != math.Trunc(n) {
		addc("must be an integer enum value", "integer", nil)
	}
	if k.GetDefinedOnly() {
		if _, ok := k.GetValues()[iv]; !ok {
			addc("must be a defined enum value", "enum_defined", nil)
		}
	}
	if len(k.In) > 0 && !contains(k.In, iv) {
		addc(fmt.Sprintf("must be one of %v", k.In), "enum_in", nil)
	}
	if len(k.NotIn) > 0 && contains(k.NotIn, iv) {
		addc(fmt.Sprintf("must not be one of %v", k.NotIn), "enum_not_in", nil)
	}
	return out
}

func checkDuration(path, s string, k *Schema_Filed_Duration) []*FieldError {
	d, err := time.ParseDuration(s)
	if err != nil {
		return []*FieldError{codeErr(path, "invalid duration: "+err.Error(), "duration", nil)}
	}
	var out []*FieldError
	addc := func(m, code string, params map[string]string) {
		out = append(out, codeErr(path, m, code, params))
	}
	if k.Gt != nil && !(d > k.Gt.AsDuration()) {
		addc(fmt.Sprintf("must be > %s", k.Gt.AsDuration()), "gt", map[string]string{"gt": k.Gt.AsDuration().String()})
	}
	if k.Gte != nil && !(d >= k.Gte.AsDuration()) {
		addc(fmt.Sprintf("must be >= %s", k.Gte.AsDuration()), "gte", map[string]string{"gte": k.Gte.AsDuration().String()})
	}
	if k.Lt != nil && !(d < k.Lt.AsDuration()) {
		addc(fmt.Sprintf("must be < %s", k.Lt.AsDuration()), "lt", map[string]string{"lt": k.Lt.AsDuration().String()})
	}
	if k.Lte != nil && !(d <= k.Lte.AsDuration()) {
		addc(fmt.Sprintf("must be <= %s", k.Lte.AsDuration()), "lte", map[string]string{"lte": k.Lte.AsDuration().String()})
	}
	return out
}

func checkTimestamp(path, s string, k *Schema_Filed_Timestamp) []*FieldError {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return []*FieldError{codeErr(path, "invalid timestamp (want RFC3339): "+err.Error(), "timestamp", nil)}
	}
	var out []*FieldError
	addc := func(m, code string, params map[string]string) {
		out = append(out, codeErr(path, m, code, params))
	}
	if k.Gt != nil && !t.After(k.Gt.AsTime()) {
		addc(fmt.Sprintf("must be after %s", k.Gt.AsTime().Format(time.RFC3339)), "gt", map[string]string{"gt": k.Gt.AsTime().Format(time.RFC3339)})
	}
	if k.Gte != nil && t.Before(k.Gte.AsTime()) {
		addc(fmt.Sprintf("must be at or after %s", k.Gte.AsTime().Format(time.RFC3339)), "gte", map[string]string{"gte": k.Gte.AsTime().Format(time.RFC3339)})
	}
	if k.Lt != nil && !t.Before(k.Lt.AsTime()) {
		addc(fmt.Sprintf("must be before %s", k.Lt.AsTime().Format(time.RFC3339)), "lt", map[string]string{"lt": k.Lt.AsTime().Format(time.RFC3339)})
	}
	if k.Lte != nil && t.After(k.Lte.AsTime()) {
		addc(fmt.Sprintf("must be at or before %s", k.Lte.AsTime().Format(time.RFC3339)), "lte", map[string]string{"lte": k.Lte.AsTime().Format(time.RFC3339)})
	}
	return out
}

// =============================================================================
// Self-schema validation
// =============================================================================

// IsValid checks that the descriptor itself is well-formed: every field is
// named (and unique within its level), exactly one kind is set, rule
// expressions are non-empty and compile, patterns are valid, no computed-field
// cycles, etc. It returns one FieldError per problem (empty = valid).
func (s *Schema) IsValid() []*FieldError {
	if s == nil {
		return []*FieldError{schemaErr("", "schema is nil")}
	}
	var out []*FieldError
	if s.GetId() == nil || s.GetId().GetName() == "" {
		out = append(out, schemaErr("id", "schema identity is required: id.name must be set"))
	}
	// Validate each named def's fields.
	for defName, def := range s.GetDefs() {
		out = append(out, validateSchemaFields(def.GetFields(), "$defs."+defName)...)
		for j, r := range def.GetRules() {
			out = append(out, validateRuleDef(r, fmt.Sprintf("$defs.%s.rules[%d]", defName, j))...)
		}
		if _, err := buildComputeOrder(def.GetFields()); err != nil {
			out = append(out, schemaErr("$defs."+defName, err.Error()))
		}
	}
	out = append(out, validateSchemaFields(s.GetFields(), "")...)
	for i, r := range s.GetRules() {
		out = append(out, validateRuleDef(r, fmt.Sprintf("rules[%d]", i))...)
	}
	if _, err := buildComputeOrder(s.GetFields()); err != nil {
		out = append(out, schemaErr("", err.Error()))
	}
	// Verify that every Ref name in the schema (including inside defs) refers
	// to a known def.
	out = append(out, collectRefErrors(s.GetFields(), s.GetDefs(), "")...)
	for defName, def := range s.GetDefs() {
		out = append(out, collectRefErrors(def.GetFields(), s.GetDefs(), "$defs."+defName)...)
	}
	return out
}

// collectRefErrors walks a field list recursively and reports any Ref that
// names a def not present in rootDefs.
func collectRefErrors(fields []*Schema_Filed, rootDefs map[string]*Schema, prefix string) []*FieldError {
	var out []*FieldError
	for _, f := range fields {
		name := f.GetName()
		path := join(prefix, name)
		switch {
		case f.GetRef() != nil:
			ref := f.GetRef()
			// Identity-refs target an external registered schema; they are
			// resolved by Link at validate time, not against local defs, so a
			// missing one is not a schema (build-time) error.
			if ref.GetId() != nil {
				break
			}
			refName := ref.GetName()
			if _, ok := rootDefs[refName]; !ok {
				out = append(out, schemaErr(path, fmt.Sprintf("ref %q is not defined in schema defs", refName)))
			}
		case f.GetList() != nil:
			out = append(out, collectRefErrors(f.GetList().GetItems(), rootDefs, path+"[]")...)
		case f.GetObject() != nil && f.GetObject().GetSchema() != nil:
			out = append(out, collectRefErrors(f.GetObject().GetSchema().GetFields(), rootDefs, path)...)
		case f.GetMap() != nil && f.GetMap().GetValueSchema() != nil:
			out = append(out, collectRefErrors(f.GetMap().GetValueSchema().GetFields(), rootDefs, path)...)
		case f.GetOneOf() != nil:
			for vkey, variant := range f.GetOneOf().GetVariants() {
				vpath := fmt.Sprintf("%s[variant=%s]", path, vkey)
				out = append(out, collectRefErrors(variant.GetFields(), rootDefs, vpath)...)
			}
		}
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
		// Ref kind: a target must be set — either a local def name or an
		// identity (actual resolvability checked in IsValid / at Link time).
		if r := f.GetRef(); r != nil && r.GetName() == "" && r.GetId() == nil {
			out = append(out, schemaErr(path, "ref field requires a name or id target"))
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
		if norm := f.GetNormalize(); norm != "" {
			if _, err := expr.Compile(norm); err != nil {
				out = append(out, schemaErr(path, "normalize expr does not compile: "+err.Error()))
			}
		}
		if w := f.GetWhen(); w != "" {
			if _, err := expr.Compile(w); err != nil {
				out = append(out, schemaErr(path, "when expr does not compile: "+err.Error()))
			}
		}
		if e := f.GetEnum(); e != nil && e.GetOptionsExpr() != "" {
			if _, err := expr.Compile(e.GetOptionsExpr()); err != nil {
				out = append(out, schemaErr(path, "options_expr does not compile: "+err.Error()))
			}
		}
		if l := f.GetList(); l != nil && l.GetCountExpr() != "" {
			if _, err := expr.Compile(l.GetCountExpr()); err != nil {
				out = append(out, schemaErr(path, "count_expr does not compile: "+err.Error()))
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
		if mp := f.GetMap(); mp != nil {
			if mp.MinEntries != nil && mp.MaxEntries != nil && *mp.MinEntries > *mp.MaxEntries {
				out = append(out, schemaErr(path, "map min_entries must be <= max_entries"))
			}
			if vs := mp.GetValueSchema(); vs != nil {
				out = append(out, validateSchemaFields(vs.GetFields(), path)...)
				for j, r := range vs.GetRules() {
					out = append(out, validateRuleDef(r, fmt.Sprintf("%s.rules[%d]", path, j))...)
				}
				if _, err := buildComputeOrder(vs.GetFields()); err != nil {
					out = append(out, schemaErr(path, err.Error()))
				}
			}
		}
		if oo := f.GetOneOf(); oo != nil {
			if oo.GetDiscriminator() == "" {
				out = append(out, schemaErr(path, "oneof requires a discriminator"))
			}
			if len(oo.GetVariants()) == 0 {
				out = append(out, schemaErr(path, "oneof requires at least one variant"))
			}
			for vkey, variant := range oo.GetVariants() {
				vpath := fmt.Sprintf("%s[variant=%s]", path, vkey)
				out = append(out, validateSchemaFields(variant.GetFields(), vpath)...)
				for j, r := range variant.GetRules() {
					out = append(out, validateRuleDef(r, fmt.Sprintf("%s.rules[%d]", vpath, j))...)
				}
				if _, err := buildComputeOrder(variant.GetFields()); err != nil {
					out = append(out, schemaErr(vpath, err.Error()))
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

// codeErr builds a structured FieldError with a stable machine code and optional i18n params.
func codeErr(field, msg, code string, params map[string]string) *FieldError {
	return &FieldError{Field: field, Message: msg, Severity: Schema_Filed_ERROR, Code: code, Params: params}
}

func ferr(field, msg string, sev Schema_Filed_Severity, ruleID *string) *FieldError {
	if sev == Schema_Filed_SEVERITY_UNSPECIFIED {
		sev = Schema_Filed_ERROR
	}
	return &FieldError{Field: field, Message: msg, Severity: sev, RuleId: ruleID}
}

func typeErr(path, want string) []*FieldError {
	return []*FieldError{codeErr(path, "expected "+want, "type", map[string]string{"want": want})}
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
