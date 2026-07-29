package schemapb

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file is a fluent (chain) builder API for assembling schemas, e.g.
//
//	s := NewSchema("infra", "disk", "v1").
//		Descr("disk config").
//		Fields(
//			Int64("shared_buffers").Gte(16).Default(128).Unit("MB").Group("Resource Usage"),
//			Str("wal_level").In("minimal", "replica", "logical").Default("replica"),
//			Computed("effective_cache_size", "root.shared_buffers * 3").Result(ResultInt64),
//		).
//		Rules(Rule("root.work_mem * root.max_connections <= 4096", "budget").ID("mem")).
//		MustBuild()
//
// Kind-first: each kind has a constructor taking the field name; constraint
// methods are kind-specific, common field methods (Required/Group/Unit/...)
// come from the shared generic base.

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T { return &v }

// Short aliases for the verbose generated enum constants.
const (
	SeverityUnspecified = Schema_Filed_SEVERITY_UNSPECIFIED
	SeverityError       = Schema_Filed_ERROR
	SeverityWarning     = Schema_Filed_WARNING

	ResultDouble   = Schema_Filed_RESULT_TYPE_DOUBLE
	ResultInt64    = Schema_Filed_RESULT_TYPE_INT64
	ResultUint64   = Schema_Filed_RESULT_TYPE_UINT64
	ResultBool     = Schema_Filed_RESULT_TYPE_BOOL
	ResultString   = Schema_Filed_RESULT_TYPE_STRING
	ResultDuration = Schema_Filed_RESULT_TYPE_DURATION

	// StringFormat aliases.
	FormatEmail    = Schema_Filed_String_STRING_FORMAT_EMAIL
	FormatURL      = Schema_Filed_String_STRING_FORMAT_URL
	FormatUUID     = Schema_Filed_String_STRING_FORMAT_UUID
	FormatIPv4     = Schema_Filed_String_STRING_FORMAT_IPV4
	FormatIPv6     = Schema_Filed_String_STRING_FORMAT_IPV6
	FormatIP       = Schema_Filed_String_STRING_FORMAT_IP
	FormatHostname = Schema_Filed_String_STRING_FORMAT_HOSTNAME
	FormatDate     = Schema_Filed_String_STRING_FORMAT_DATE
	FormatTime     = Schema_Filed_String_STRING_FORMAT_TIME
	FormatDatetime = Schema_Filed_String_STRING_FORMAT_DATETIME
)

// =============================================================================
// Schema builder
// =============================================================================

// SchemaB builds a Schema.
type SchemaB struct{ s *Schema }

// NewSchema starts a schema with the given identity (namespace may be empty;
// name is required by the validator).
func NewSchema(namespace, name, version string) *SchemaB {
	return &SchemaB{s: &Schema{Id: &SchemaIdentity{Namespace: namespace, Name: name, Version: version}}}
}

// Descr sets the schema description.
func (b *SchemaB) Descr(d string) *SchemaB { b.s.Description = &d; return b }

// Strict enables strict mode: unknown keys in the values map are rejected.
func (b *SchemaB) Strict() *SchemaB { b.s.Strict = true; return b }

// MinProps sets the minimum number of properties required in the values map.
func (b *SchemaB) MinProps(n uint64) *SchemaB { b.s.MinProperties = &n; return b }

// MaxProps sets the maximum number of properties allowed in the values map.
func (b *SchemaB) MaxProps(n uint64) *SchemaB { b.s.MaxProperties = &n; return b }

// Coerce enables input coercion: string inputs are coerced to the field's kind.
func (b *SchemaB) Coerce() *SchemaB { b.s.Coerce = true; return b }

// Template registers a named render template (Go text/template) on the schema.
// Render it with (*Schema).Render(name, values) or (*Baked).Render(name); the
// same template renders identically in the browser via WASM.
func (b *SchemaB) Template(name, tmpl string) *SchemaB {
	if b.s.Templates == nil {
		b.s.Templates = map[string]string{}
	}
	b.s.Templates[name] = tmpl
	return b
}

// Fields appends fields (field builders or raw *Schema_Filed).
func (b *SchemaB) Fields(defs ...FieldDef) *SchemaB {
	for _, d := range defs {
		b.s.Fields = append(b.s.Fields, d.Done())
	}
	return b
}

// Rules appends form-wide rules (rule builders or raw *Schema_Filed_Rule).
func (b *SchemaB) Rules(rules ...RuleDef) *SchemaB {
	for _, r := range rules {
		b.s.Rules = append(b.s.Rules, r.Done())
	}
	return b
}

// --- conditional presence sugar --------------------------------------------
//
// These emit a form-wide rule (so they fire even when the field is absent) that
// toggles a top-level field's PRESENCE requirement on a condition over `root`.
// They differ from .When(cond): When gates EXISTENCE (an inactive field is
// hidden and its value ignored), while these keep the field active and only
// constrain whether it must be present. Presence is tested with `"field" in
// root`, so they target top-level fields by name. The error is reported on
// `field` (code "rule").

// RequiredWhen makes `field` required only when `cond` (an expr over root) is
// true; otherwise it stays optional.
func (b *SchemaB) RequiredWhen(field, cond string) *SchemaB {
	return b.Rules(Rule(
		fmt.Sprintf(`!(%s) || (%q in root)`, cond, field),
		fmt.Sprintf("%s is required when: %s", field, cond),
	).ID(field))
}

// RequiredUnless makes `field` required unless `cond` is true (the inverse of
// RequiredWhen).
func (b *SchemaB) RequiredUnless(field, cond string) *SchemaB {
	return b.Rules(Rule(
		fmt.Sprintf(`(%s) || (%q in root)`, cond, field),
		fmt.Sprintf("%s is required unless: %s", field, cond),
	).ID(field))
}

// ForbiddenWhen rejects `field` being present when `cond` is true: a hard error,
// unlike .When which silently ignores the value of an inactive field.
func (b *SchemaB) ForbiddenWhen(field, cond string) *SchemaB {
	return b.Rules(Rule(
		fmt.Sprintf(`!(%s) || !(%q in root)`, cond, field),
		fmt.Sprintf("%s must be absent when: %s", field, cond),
	).ID(field))
}

// Build assembles and validates the schema; it returns a *SchemaError if the
// descriptor is malformed.
func (b *SchemaB) Build() (*Schema, error) {
	// Lift any embedded schema's $defs to the root so internal Refs resolve
	// (inline composition via ObjectOf/VariantOf/DefSchema).
	if err := hoistDefs(b.s); err != nil {
		return nil, &SchemaError{Errors: []*FieldError{schemaErr("$defs", err.Error())}}
	}
	if errs := b.s.IsValid(); len(errs) > 0 {
		return nil, &SchemaError{Errors: errs}
	}
	return b.s, nil
}

// MustBuild is Build that panics on a malformed schema.
func (b *SchemaB) MustBuild() *Schema {
	s, err := b.Build()
	if err != nil {
		panic(err)
	}
	return s
}

// =============================================================================
// Rule builder
// =============================================================================

// RuleDef is anything that yields a rule: a *RuleB or a raw *Schema_Filed_Rule.
type RuleDef interface{ Done() *Schema_Filed_Rule }

// Done lets a raw rule satisfy RuleDef.
func (r *Schema_Filed_Rule) Done() *Schema_Filed_Rule { return r }

// RuleB builds a validation Rule.
type RuleB struct{ r *Schema_Filed_Rule }

// Rule builds a CEL/expr validation rule. expr must evaluate to bool; true
// means valid. msg is shown when it is false.
func Rule(expr, msg string) *RuleB {
	return &RuleB{r: &Schema_Filed_Rule{Expr: expr, Message: msg}}
}

// ID sets the stable rule id.
func (b *RuleB) ID(id string) *RuleB { b.r.Id = &id; return b }

// Warn marks the rule as a WARNING (does not block submit).
func (b *RuleB) Warn() *RuleB { s := SeverityWarning; b.r.Severity = &s; return b }

// Severity sets the rule severity explicitly.
func (b *RuleB) Severity(s Schema_Filed_Severity) *RuleB { b.r.Severity = &s; return b }

// Done returns the built rule.
func (b *RuleB) Done() *Schema_Filed_Rule { return b.r }

// =============================================================================
// Field builders
// =============================================================================

// FieldDef is anything that yields a field: a kind builder or a raw *Schema_Filed.
type FieldDef interface{ Done() *Schema_Filed }

// Done lets a raw field satisfy FieldDef.
func (f *Schema_Filed) Done() *Schema_Filed { return f }

// fieldBase provides the common field methods. S is the concrete builder type
// so the methods chain back to it (self-type pattern, no per-kind duplication).
type fieldBase[S any] struct {
	f    *Schema_Filed
	self S
}

func newField[S any](name string, self S) fieldBase[S] {
	return fieldBase[S]{f: &Schema_Filed{Name: name}, self: self}
}

func (b fieldBase[S]) Required() S      { b.f.Required = true; return b.self }
func (b fieldBase[S]) Nullable() S      { b.f.Nullable = true; return b.self }
func (b fieldBase[S]) Immutable() S     { b.f.Immutable = true; return b.self }
func (b fieldBase[S]) Group(g string) S { b.f.Group = &g; return b.self }
func (b fieldBase[S]) Unit(u string) S  { b.f.Unit = &u; return b.self }
func (b fieldBase[S]) Desc(d string) S  { b.f.Description = &d; return b.self }

// Title sets an informative human title for the field. Purely informative.
func (b fieldBase[S]) Title(t string) S { b.f.Title = &t; return b.self }

// Deprecated marks the field as deprecated. Purely informative.
func (b fieldBase[S]) Deprecated() S { b.f.Deprecated = true; return b.self }

// Secret marks the field as sensitive (e.g. a password). Purely informative.
func (b fieldBase[S]) Secret() S { b.f.Secret = true; return b.self }

// Examples sets example values for the field. Purely informative.
func (b fieldBase[S]) Examples(vals ...*structpb.Value) S {
	b.f.Examples = append(b.f.Examples, vals...)
	return b.self
}

// Rules attaches cross-field validation rules to this field.
func (b fieldBase[S]) Rules(rules ...RuleDef) S {
	for _, r := range rules {
		b.f.Rules = append(b.f.Rules, r.Done())
	}
	return b.self
}

// Normalize sets the normalize expression for the field. `this` = current
// value, `root` = whole form; the expression result becomes the new value.
func (b fieldBase[S]) Normalize(e string) S { b.f.Normalize = &e; return b.self }

// When sets the conditional gate: an expr boolean over `root`. When it is false
// the field (and, for container kinds, its whole subtree) is inactive — skipped
// by validation and hidden by renderers. `this` is not bound.
func (b fieldBase[S]) When(e string) S { b.f.When = &e; return b.self }

// Done returns the built field.
func (b fieldBase[S]) Done() *Schema_Filed { return b.f }

// --- numeric kinds ---------------------------------------------------------

// FloatB builds a float field.
type FloatB struct {
	fieldBase[*FloatB]
	k *Schema_Filed_Float
}

func Float(name string) *FloatB {
	b := &FloatB{k: &Schema_Filed_Float{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Float_{Float: b.k}
	return b
}

func (b *FloatB) Default(v float32) *FloatB    { b.k.Default = &v; return b }
func (b *FloatB) Const(v float32) *FloatB      { b.k.Const = &v; return b }
func (b *FloatB) Gt(v float32) *FloatB         { b.k.Gt = &v; return b }
func (b *FloatB) Gte(v float32) *FloatB        { b.k.Gte = &v; return b }
func (b *FloatB) Lt(v float32) *FloatB         { b.k.Lt = &v; return b }
func (b *FloatB) Lte(v float32) *FloatB        { b.k.Lte = &v; return b }
func (b *FloatB) In(v ...float32) *FloatB      { b.k.In = v; return b }
func (b *FloatB) NotIn(v ...float32) *FloatB   { b.k.NotIn = v; return b }
func (b *FloatB) MultipleOf(v float32) *FloatB { b.k.MultipleOf = &v; return b }

// DoubleB builds a double field.
type DoubleB struct {
	fieldBase[*DoubleB]
	k *Schema_Filed_Double
}

func Double(name string) *DoubleB {
	b := &DoubleB{k: &Schema_Filed_Double{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Double_{Double: b.k}
	return b
}

func (b *DoubleB) Default(v float64) *DoubleB    { b.k.Default = &v; return b }
func (b *DoubleB) Const(v float64) *DoubleB      { b.k.Const = &v; return b }
func (b *DoubleB) Gt(v float64) *DoubleB         { b.k.Gt = &v; return b }
func (b *DoubleB) Gte(v float64) *DoubleB        { b.k.Gte = &v; return b }
func (b *DoubleB) Lt(v float64) *DoubleB         { b.k.Lt = &v; return b }
func (b *DoubleB) Lte(v float64) *DoubleB        { b.k.Lte = &v; return b }
func (b *DoubleB) In(v ...float64) *DoubleB      { b.k.In = v; return b }
func (b *DoubleB) NotIn(v ...float64) *DoubleB   { b.k.NotIn = v; return b }
func (b *DoubleB) MultipleOf(v float64) *DoubleB { b.k.MultipleOf = &v; return b }

// Int32B builds an int32 field.
type Int32B struct {
	fieldBase[*Int32B]
	k *Schema_Filed_Int32
}

func Int32(name string) *Int32B {
	b := &Int32B{k: &Schema_Filed_Int32{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Int32_{Int32: b.k}
	return b
}

func (b *Int32B) Default(v int32) *Int32B    { b.k.Default = &v; return b }
func (b *Int32B) Const(v int32) *Int32B      { b.k.Const = &v; return b }
func (b *Int32B) Gt(v int32) *Int32B         { b.k.Gt = &v; return b }
func (b *Int32B) Gte(v int32) *Int32B        { b.k.Gte = &v; return b }
func (b *Int32B) Lt(v int32) *Int32B         { b.k.Lt = &v; return b }
func (b *Int32B) Lte(v int32) *Int32B        { b.k.Lte = &v; return b }
func (b *Int32B) In(v ...int32) *Int32B      { b.k.In = v; return b }
func (b *Int32B) NotIn(v ...int32) *Int32B   { b.k.NotIn = v; return b }
func (b *Int32B) MultipleOf(v int32) *Int32B { b.k.MultipleOf = &v; return b }

// Int64B builds an int64 field.
type Int64B struct {
	fieldBase[*Int64B]
	k *Schema_Filed_Int64
}

func Int64(name string) *Int64B {
	b := &Int64B{k: &Schema_Filed_Int64{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Int64_{Int64: b.k}
	return b
}

func (b *Int64B) Default(v int64) *Int64B    { b.k.Default = &v; return b }
func (b *Int64B) Const(v int64) *Int64B      { b.k.Const = &v; return b }
func (b *Int64B) Gt(v int64) *Int64B         { b.k.Gt = &v; return b }
func (b *Int64B) Gte(v int64) *Int64B        { b.k.Gte = &v; return b }
func (b *Int64B) Lt(v int64) *Int64B         { b.k.Lt = &v; return b }
func (b *Int64B) Lte(v int64) *Int64B        { b.k.Lte = &v; return b }
func (b *Int64B) In(v ...int64) *Int64B      { b.k.In = v; return b }
func (b *Int64B) NotIn(v ...int64) *Int64B   { b.k.NotIn = v; return b }
func (b *Int64B) MultipleOf(v int64) *Int64B { b.k.MultipleOf = &v; return b }

// UInt32B builds a uint32 field.
type UInt32B struct {
	fieldBase[*UInt32B]
	k *Schema_Filed_UInt32
}

func UInt32(name string) *UInt32B {
	b := &UInt32B{k: &Schema_Filed_UInt32{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Uint32{Uint32: b.k}
	return b
}

func (b *UInt32B) Default(v uint32) *UInt32B    { b.k.Default = &v; return b }
func (b *UInt32B) Const(v uint32) *UInt32B      { b.k.Const = &v; return b }
func (b *UInt32B) Gt(v uint32) *UInt32B         { b.k.Gt = &v; return b }
func (b *UInt32B) Gte(v uint32) *UInt32B        { b.k.Gte = &v; return b }
func (b *UInt32B) Lt(v uint32) *UInt32B         { b.k.Lt = &v; return b }
func (b *UInt32B) Lte(v uint32) *UInt32B        { b.k.Lte = &v; return b }
func (b *UInt32B) In(v ...uint32) *UInt32B      { b.k.In = v; return b }
func (b *UInt32B) NotIn(v ...uint32) *UInt32B   { b.k.NotIn = v; return b }
func (b *UInt32B) MultipleOf(v uint32) *UInt32B { b.k.MultipleOf = &v; return b }

// UInt64B builds a uint64 field.
type UInt64B struct {
	fieldBase[*UInt64B]
	k *Schema_Filed_UInt64
}

func UInt64(name string) *UInt64B {
	b := &UInt64B{k: &Schema_Filed_UInt64{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Uint64{Uint64: b.k}
	return b
}

func (b *UInt64B) Default(v uint64) *UInt64B    { b.k.Default = &v; return b }
func (b *UInt64B) Const(v uint64) *UInt64B      { b.k.Const = &v; return b }
func (b *UInt64B) Gt(v uint64) *UInt64B         { b.k.Gt = &v; return b }
func (b *UInt64B) Gte(v uint64) *UInt64B        { b.k.Gte = &v; return b }
func (b *UInt64B) Lt(v uint64) *UInt64B         { b.k.Lt = &v; return b }
func (b *UInt64B) Lte(v uint64) *UInt64B        { b.k.Lte = &v; return b }
func (b *UInt64B) In(v ...uint64) *UInt64B      { b.k.In = v; return b }
func (b *UInt64B) NotIn(v ...uint64) *UInt64B   { b.k.NotIn = v; return b }
func (b *UInt64B) MultipleOf(v uint64) *UInt64B { b.k.MultipleOf = &v; return b }

// --- bool / string / enum --------------------------------------------------

// BoolB builds a bool field.
type BoolB struct {
	fieldBase[*BoolB]
	k *Schema_Filed_Bool
}

func Bool(name string) *BoolB {
	b := &BoolB{k: &Schema_Filed_Bool{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Bool_{Bool: b.k}
	return b
}

func (b *BoolB) Default(v bool) *BoolB { b.k.Default = &v; return b }
func (b *BoolB) Const(v bool) *BoolB   { b.k.Const = &v; return b }

// StrB builds a string field.
type StrB struct {
	fieldBase[*StrB]
	k *Schema_Filed_String
}

func Str(name string) *StrB {
	b := &StrB{k: &Schema_Filed_String{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_String_{String_: b.k}
	return b
}

func (b *StrB) Default(v string) *StrB  { b.k.Default = &v; return b }
func (b *StrB) Const(v string) *StrB    { b.k.Const = &v; return b }
func (b *StrB) Len(v uint64) *StrB      { b.k.Len = &v; return b }
func (b *StrB) MinLen(v uint64) *StrB   { b.k.MinLen = &v; return b }
func (b *StrB) MaxLen(v uint64) *StrB   { b.k.MaxLen = &v; return b }
func (b *StrB) Pattern(v string) *StrB  { b.k.Pattern = &v; return b }
func (b *StrB) In(v ...string) *StrB    { b.k.In = v; return b }
func (b *StrB) NotIn(v ...string) *StrB { b.k.NotIn = v; return b }

// Format sets the semantic string format constraint.
func (b *StrB) Format(f Schema_Filed_String_StringFormat) *StrB { b.k.Format = &f; return b }

// EnumB builds an enum field.
type EnumB struct {
	fieldBase[*EnumB]
	k *Schema_Filed_Enum
}

func Enum(name string) *EnumB {
	b := &EnumB{k: &Schema_Filed_Enum{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Enum_{Enum: b.k}
	return b
}

func (b *EnumB) Default(v int32) *EnumB           { b.k.Default = &v; return b }
func (b *EnumB) Values(v map[int32]string) *EnumB { b.k.Values = v; return b }
func (b *EnumB) DefinedOnly() *EnumB              { b.k.DefinedOnly = true; return b }
func (b *EnumB) In(v ...int32) *EnumB             { b.k.In = v; return b }
func (b *EnumB) NotIn(v ...int32) *EnumB          { b.k.NotIn = v; return b }

// Options sets the dynamic options expression: an expr over `root` returning the
// list of allowed integer values. When set it replaces the static allowed set.
func (b *EnumB) Options(e string) *EnumB { b.k.OptionsExpr = &e; return b }

// --- duration / timestamp --------------------------------------------------

// DurationB builds a duration field.
type DurationB struct {
	fieldBase[*DurationB]
	k *Schema_Filed_Duration
}

func Duration(name string) *DurationB {
	b := &DurationB{k: &Schema_Filed_Duration{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Duration_{Duration: b.k}
	return b
}

func (b *DurationB) Default(d time.Duration) *DurationB { b.k.Default = durationpb.New(d); return b }
func (b *DurationB) Gt(d time.Duration) *DurationB      { b.k.Gt = durationpb.New(d); return b }
func (b *DurationB) Gte(d time.Duration) *DurationB     { b.k.Gte = durationpb.New(d); return b }
func (b *DurationB) Lt(d time.Duration) *DurationB      { b.k.Lt = durationpb.New(d); return b }
func (b *DurationB) Lte(d time.Duration) *DurationB     { b.k.Lte = durationpb.New(d); return b }

// TimestampB builds a timestamp field.
type TimestampB struct {
	fieldBase[*TimestampB]
	k *Schema_Filed_Timestamp
}

func Timestamp(name string) *TimestampB {
	b := &TimestampB{k: &Schema_Filed_Timestamp{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Timestamp_{Timestamp: b.k}
	return b
}

func (b *TimestampB) Default(t time.Time) *TimestampB { b.k.Default = timestamppb.New(t); return b }
func (b *TimestampB) Gt(t time.Time) *TimestampB      { b.k.Gt = timestamppb.New(t); return b }
func (b *TimestampB) Gte(t time.Time) *TimestampB     { b.k.Gte = timestamppb.New(t); return b }
func (b *TimestampB) Lt(t time.Time) *TimestampB      { b.k.Lt = timestamppb.New(t); return b }
func (b *TimestampB) Lte(t time.Time) *TimestampB     { b.k.Lte = timestamppb.New(t); return b }

// --- list / object / computed ----------------------------------------------

// ListB builds a list field.
type ListB struct {
	fieldBase[*ListB]
	k *Schema_Filed_List
}

// List builds a list field; items describe the element field(s).
func List(name string, items ...FieldDef) *ListB {
	b := &ListB{k: &Schema_Filed_List{}}
	b.fieldBase = newField(name, b)
	for _, d := range items {
		b.k.Items = append(b.k.Items, d.Done())
	}
	b.f.Kind = &Schema_Filed_List_{List: b.k}
	return b
}

func (b *ListB) MinItems(v uint64) *ListB { b.k.MinItems = &v; return b }
func (b *ListB) MaxItems(v uint64) *ListB { b.k.MaxItems = &v; return b }
func (b *ListB) Unique() *ListB           { b.k.Unique = true; return b }

// Count sets the dynamic length expression: an expr over `root` returning the
// exact non-negative number of items the list must have. Inside an item's rules
// the item's zero-based position is bound as `index`.
func (b *ListB) Count(e string) *ListB { b.k.CountExpr = &e; return b }

// ObjectB builds a nested object field.
type ObjectB struct {
	fieldBase[*ObjectB]
	k *Schema_Filed_Object
}

// Object builds a nested object field from its child fields (no identity needed
// for nested schemas).
func Object(name string, fields ...FieldDef) *ObjectB {
	sub := &Schema{}
	for _, d := range fields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b := &ObjectB{k: &Schema_Filed_Object{Schema: sub}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Object_{Object: b.k}
	return b
}

// Rule adds a form-wide rule to the nested object schema.
func (b *ObjectB) Rule(rules ...RuleDef) *ObjectB {
	for _, r := range rules {
		b.k.Schema.Rules = append(b.k.Schema.Rules, r.Done())
	}
	return b
}

// Strict enables strict mode on the nested object schema.
func (b *ObjectB) Strict() *ObjectB { b.k.Schema.Strict = true; return b }

// MinProps sets the minimum number of properties on the nested object schema.
func (b *ObjectB) MinProps(n uint64) *ObjectB { b.k.Schema.MinProperties = &n; return b }

// MaxProps sets the maximum number of properties on the nested object schema.
func (b *ObjectB) MaxProps(n uint64) *ObjectB { b.k.Schema.MaxProperties = &n; return b }

// MapB builds a map field: free-form string keys (never rejected), values
// validated against a shared schema.
type MapB struct {
	fieldBase[*MapB]
	k *Schema_Filed_Map
}

// Map builds a map field; valueFields describe the schema every map value
// must satisfy (like Object, an inline schema with no identity needed). A
// map with no valueFields still gets an (unconstrained, non-strict) value
// schema, so keys stay free and values are accepted as any object.
func Map(name string, valueFields ...FieldDef) *MapB {
	sub := &Schema{}
	for _, d := range valueFields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b := &MapB{k: &Schema_Filed_Map{ValueSchema: sub}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Map_{Map: b.k}
	return b
}

// Strict enables strict mode on the map's value schema: an unknown key
// inside a map VALUE is rejected (the map's own keys are always free).
func (b *MapB) Strict() *MapB { b.k.ValueSchema.Strict = true; return b }

// MinEntries sets the minimum number of map entries.
func (b *MapB) MinEntries(n uint64) *MapB { b.k.MinEntries = &n; return b }

// MaxEntries sets the maximum number of map entries.
func (b *MapB) MaxEntries(n uint64) *MapB { b.k.MaxEntries = &n; return b }

// Rule adds a form-wide rule to the map's value schema.
func (b *MapB) Rule(rules ...RuleDef) *MapB {
	for _, r := range rules {
		b.k.ValueSchema.Rules = append(b.k.ValueSchema.Rules, r.Done())
	}
	return b
}

// MinProps sets the minimum number of properties on the map's value schema.
func (b *MapB) MinProps(n uint64) *MapB { b.k.ValueSchema.MinProperties = &n; return b }

// MaxProps sets the maximum number of properties on the map's value schema.
func (b *MapB) MaxProps(n uint64) *MapB { b.k.ValueSchema.MaxProperties = &n; return b }

// ComputedB builds a derived field.
type ComputedB struct {
	fieldBase[*ComputedB]
	k *Schema_Filed_Computed
}

// Computed builds a derived field; expr reads root (inputs + computed) and
// produces the value.
func Computed(name, expr string) *ComputedB {
	b := &ComputedB{k: &Schema_Filed_Computed{Expr: expr}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Computed_{Computed: b.k}
	return b
}

// Result sets the result-type hint used to marshal the derived value.
func (b *ComputedB) Result(rt Schema_Filed_ResultType) *ComputedB { b.k.Result = &rt; return b }

// OneOfB builds a discriminated-union (oneof) field.
type OneOfB struct {
	fieldBase[*OneOfB]
	k *Schema_Filed_OneOf
}

// OneOf builds a discriminated-union field. The value must be an object; the
// discriminator property selects the variant schema to validate against.
func OneOf(name, discriminator string) *OneOfB {
	b := &OneOfB{k: &Schema_Filed_OneOf{Discriminator: discriminator, Variants: map[string]*Schema{}}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_OneOf_{OneOf: b.k}
	return b
}

// Variant adds a variant schema under the given key. fields become the variant
// schema's fields (no identity — inline schema, like Object).
func (b *OneOfB) Variant(key string, fields ...FieldDef) *OneOfB {
	sub := &Schema{}
	for _, d := range fields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b.k.Variants[key] = sub
	return b
}

// Def registers a named sub-schema in the schema's defs map. Fields of kind
// Ref resolve against these named defs. The def schema has no identity — it
// is referenced by name only.
func (b *SchemaB) Def(name string, fields ...FieldDef) *SchemaB {
	if b.s.Defs == nil {
		b.s.Defs = map[string]*Schema{}
	}
	sub := &Schema{}
	for _, d := range fields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b.s.Defs[name] = sub
	return b
}

// RefB builds a Ref field that resolves its value against a named def in the
// root schema's defs map.
type RefB struct {
	fieldBase[*RefB]
}

// Ref builds a field of kind Ref that resolves against a LOCAL def. name is the
// field name; defName is the key in the root schema's defs map that the value
// must validate against.
func Ref(name, defName string) *RefB {
	b := &RefB{}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Ref_{Ref: &Schema_Filed_Ref{Target: &Schema_Filed_Ref_Name{Name: defName}}}
	return b
}

// RefID builds a Ref field that targets a separately-registered schema by
// identity. The identity is preserved on the node (renderers can resolve/link
// the target). The referenced schema must be resolvable at validate time:
// either already present in the root defs under its identity key, or pulled in
// by Link(resolver). name is the field name.
func RefID(name string, id *SchemaIdentity) *RefB {
	b := &RefB{}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Filed_Ref_{Ref: &Schema_Filed_Ref{Target: &Schema_Filed_Ref_Id{Id: id}}}
	return b
}

// RefIdentity is RefID with the identity spelled out inline.
func RefIdentity(name, namespace, schemaName, version string) *RefB {
	return RefID(name, &SchemaIdentity{Namespace: namespace, Name: schemaName, Version: version})
}

// =============================================================================
// FieldError builder (server-side validation failure)
// =============================================================================

// NewFieldError builds a single validation failure for the given field.
func NewFieldError(field, message string) *FieldError {
	return &FieldError{Field: field, Message: message, Severity: SeverityError}
}
