package schemapb

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file is the fluent (chain) builder API for assembling schemas, e.g.
//
//	s := schemapb.NewSchema("infra", "postgres", "v1").
//		Descr("PostgreSQL instance configuration").
//		Strict().
//		Fields(
//			schemapb.Int64("shared_buffers").Gte(16).Default(128).Unit("MB"),
//			schemapb.Str("wal_level").In("minimal", "replica", "logical").Default("replica"),
//			schemapb.Computed("cache", "root.shared_buffers * 3").Result(schemapb.ResultInt64),
//		).
//		Rules(schemapb.Rule("root.shared_buffers <= 65536", "too large").ID("buf")).
//		MustBuild()
//
// Kind-first: each kind has a constructor taking the field name; constraint
// methods are kind-specific (numeric kinds share one generic builder), common
// field methods (Required/Group/Unit/...) come from the shared generic base.

// Severity aliases for the verbose generated enum constants.
const (
	SeverityError   = Schema_Field_SEVERITY_ERROR
	SeverityWarning = Schema_Field_SEVERITY_WARNING
)

// ResultType aliases.
const (
	ResultDouble    = Schema_Field_RESULT_TYPE_DOUBLE
	ResultInt64     = Schema_Field_RESULT_TYPE_INT64
	ResultUint64    = Schema_Field_RESULT_TYPE_UINT64
	ResultBool      = Schema_Field_RESULT_TYPE_BOOL
	ResultString    = Schema_Field_RESULT_TYPE_STRING
	ResultDuration  = Schema_Field_RESULT_TYPE_DURATION
	ResultTimestamp = Schema_Field_RESULT_TYPE_TIMESTAMP
	ResultBytes     = Schema_Field_RESULT_TYPE_BYTES
	ResultJson      = Schema_Field_RESULT_TYPE_JSON
)

// Core string formats (see the spec's format registry; String.Format accepts
// any registry identifier, these are the always-supported core).
const (
	FormatEmail    = "email"
	FormatURL      = "url"
	FormatUUID     = "uuid"
	FormatIPv4     = "ipv4"
	FormatIPv6     = "ipv6"
	FormatIP       = "ip"
	FormatHostname = "hostname"
	FormatDate     = "date"
	FormatTime     = "time"
	FormatDatetime = "datetime"
)

// =============================================================================
// Schema builder
// =============================================================================

// SchemaB builds a Schema.
type SchemaB struct{ s *Schema }

// NewSchema starts a schema with the given identity (namespace and version may
// be empty; name is required by the validator).
func NewSchema(namespace, name, version string) *SchemaB {
	return &SchemaB{s: &Schema{Id: &SchemaIdentity{Namespace: namespace, Name: name, Version: version}}}
}

// Descr sets the schema description.
func (b *SchemaB) Descr(d string) *SchemaB { b.s.Description = &d; return b }

// Strict enables strict mode: unknown keys in the values map are rejected.
func (b *SchemaB) Strict() *SchemaB { b.s.Strict = true; return b }

// Coerce enables input coercion: string inputs are coerced to the field's kind.
func (b *SchemaB) Coerce() *SchemaB { b.s.Coerce = true; return b }

// MinProps sets the minimum number of properties required in the values map.
func (b *SchemaB) MinProps(n uint64) *SchemaB { b.s.MinProperties = &n; return b }

// MaxProps sets the maximum number of properties allowed in the values map.
func (b *SchemaB) MaxProps(n uint64) *SchemaB { b.s.MaxProperties = &n; return b }

// Template registers a named render template (Mustache) on the schema. Render
// it with (*Schema).Render(name, values) or (*Baked).Render(name).
func (b *SchemaB) Template(name, tmpl string) *SchemaB {
	if b.s.Templates == nil {
		b.s.Templates = map[string]string{}
	}
	b.s.Templates[name] = tmpl
	return b
}

// Fields appends fields (field builders or raw *Schema_Field).
func (b *SchemaB) Fields(defs ...FieldDef) *SchemaB {
	for _, d := range defs {
		b.s.Fields = append(b.s.Fields, d.Done())
	}
	return b
}

// Rules appends form-wide rules (rule builders or raw *Schema_Field_Rule).
func (b *SchemaB) Rules(rules ...RuleDef) *SchemaB {
	for _, r := range rules {
		b.s.Rules = append(b.s.Rules, r.Done())
	}
	return b
}

// Def registers a named sub-schema in the schema's defs map. Fields of kind
// Ref resolve against these named defs. The def schema has no identity — it is
// referenced by name only.
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

// --- conditional presence sugar ---------------------------------------------
//
// These emit a form-wide CEL rule (so they fire even when the field is absent)
// that toggles a top-level field's PRESENCE requirement on a condition over
// `root`. They differ from .When(cond): When gates EXISTENCE (an inactive
// field is hidden and its value ignored), while these keep the field active
// and only constrain whether it must be present. Presence is tested with
// `"field" in root` (works for any key, not only CEL identifiers). The error
// is reported with the rule id = field name.

// RequiredWhen makes `field` required only when `cond` (a CEL boolean over
// root) is true; otherwise it stays optional.
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

// ForbiddenWhen rejects `field` being present when `cond` is true: a hard
// error, unlike .When which silently ignores the value of an inactive field.
func (b *SchemaB) ForbiddenWhen(field, cond string) *SchemaB {
	return b.Rules(Rule(
		fmt.Sprintf(`!(%s) || !(%q in root)`, cond, field),
		fmt.Sprintf("%s must be absent when: %s", field, cond),
	).ID(field))
}

// Build assembles the schema and fully compiles it (descriptor checks, CEL
// expressions, patterns, templates, computed cycles): everything a schema can
// get wrong surfaces here as a *SchemaError. The compiled engine is cached,
// so the convenience methods on the returned schema pay nothing extra.
func (b *SchemaB) Build() (*Schema, error) {
	if err := hoistDefs(b.s); err != nil {
		return nil, err
	}
	if _, err := b.s.engine(); err != nil {
		return nil, err
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

// RuleDef is anything that yields a rule: a *RuleB or a raw *Schema_Field_Rule.
type RuleDef interface{ Done() *Schema_Field_Rule }

// Done lets a raw rule satisfy RuleDef.
func (r *Schema_Field_Rule) Done() *Schema_Field_Rule { return r }

// RuleB builds a validation Rule.
type RuleB struct{ r *Schema_Field_Rule }

// Rule builds a CEL validation rule. expr must evaluate to bool; true means
// valid. msg is shown when it is false.
func Rule(expr, msg string) *RuleB {
	return &RuleB{r: &Schema_Field_Rule{Expr: expr, Message: msg}}
}

// ID sets the stable rule id.
func (b *RuleB) ID(id string) *RuleB { b.r.Id = &id; return b }

// Warn marks the rule as a WARNING (does not block submit).
func (b *RuleB) Warn() *RuleB { s := SeverityWarning; b.r.Severity = &s; return b }

// Severity sets the rule severity explicitly.
func (b *RuleB) Severity(s Schema_Field_Severity) *RuleB { b.r.Severity = &s; return b }

// Done returns the built rule.
func (b *RuleB) Done() *Schema_Field_Rule { return b.r }

// =============================================================================
// Field base (shared by all kind builders)
// =============================================================================

// FieldDef is anything that yields a field: a kind builder or a raw *Schema_Field.
type FieldDef interface{ Done() *Schema_Field }

// Done lets a raw field satisfy FieldDef.
func (f *Schema_Field) Done() *Schema_Field { return f }

// fieldBase provides the common field methods. S is the concrete builder type
// so the methods chain back to it (self-type pattern, no per-kind duplication).
type fieldBase[S any] struct {
	f    *Schema_Field
	self S
}

func newField[S any](name string, self S) fieldBase[S] {
	return fieldBase[S]{f: &Schema_Field{Name: name}, self: self}
}

// Required marks the field as mandatory: its value must be present.
func (b fieldBase[S]) Required() S { b.f.Required = true; return b.self }

// Nullable allows an explicit null/empty value.
func (b fieldBase[S]) Nullable() S { b.f.Nullable = true; return b.self }

// Immutable pins the field to its default (system-fixed value).
func (b fieldBase[S]) Immutable() S { b.f.Immutable = true; return b.self }

// Group sets an informative section label. Purely informative.
func (b fieldBase[S]) Group(g string) S { b.f.Group = &g; return b.self }

// Unit sets an informative value unit ("MB", "ms"). Purely informative.
func (b fieldBase[S]) Unit(u string) S { b.f.Unit = &u; return b.self }

// Desc sets the human description.
func (b fieldBase[S]) Desc(d string) S { b.f.Description = &d; return b.self }

// Title sets an informative human title. Purely informative.
func (b fieldBase[S]) Title(t string) S { b.f.Title = &t; return b.self }

// Deprecated marks the field as deprecated. Purely informative.
func (b fieldBase[S]) Deprecated() S { b.f.Deprecated = true; return b.self }

// Secret marks the field as sensitive; its value is masked in errors.
func (b fieldBase[S]) Secret() S { b.f.Secret = true; return b.self }

// Examples attaches example values. Purely informative.
func (b fieldBase[S]) Examples(vals ...*Value) S {
	b.f.Examples = append(b.f.Examples, vals...)
	return b.self
}

// Rules attaches cross-field validation rules (`this` = this field's value).
func (b fieldBase[S]) Rules(rules ...RuleDef) S {
	for _, r := range rules {
		b.f.Rules = append(b.f.Rules, r.Done())
	}
	return b.self
}

// Normalize sets the normalize expression: `this` = current value, `root` =
// whole form; the expression result becomes the new value.
func (b fieldBase[S]) Normalize(e string) S { b.f.Normalize = &e; return b.self }

// When sets the conditional gate: a CEL boolean over `root`. When false the
// field (and its subtree) is inactive — skipped by validation, hidden by
// renderers. `this` is not bound.
func (b fieldBase[S]) When(e string) S { b.f.When = &e; return b.self }

// Done returns the built field.
func (b fieldBase[S]) Done() *Schema_Field { return b.f }

// =============================================================================
// Numeric kinds — one generic builder for all six
// =============================================================================

// numSpec wires the generic numeric builder to one concrete kind message
// (their shapes are identical, only the scalar type differs).
type numSpec[T any] struct {
	def, con, gt, gte, lt, lte, mul **T
	in, notIn                       *[]T
}

// NumB is the shared builder for Float/Double/Int32/Int64/UInt32/UInt64.
type NumB[T any] struct {
	fieldBase[*NumB[T]]
	k numSpec[T]
}

func newNum[T any](name string, k numSpec[T], kind isSchema_Field_Kind) *NumB[T] {
	b := &NumB[T]{k: k}
	b.fieldBase = newField(name, b)
	b.f.Kind = kind
	return b
}

// Default sets the value used when the field is unset.
func (b *NumB[T]) Default(v T) *NumB[T] { *b.k.def = &v; return b }

// Const requires the value to equal exactly v.
func (b *NumB[T]) Const(v T) *NumB[T] { *b.k.con = &v; return b }

// Gt sets the exclusive minimum.
func (b *NumB[T]) Gt(v T) *NumB[T] { *b.k.gt = &v; return b }

// Gte sets the inclusive minimum.
func (b *NumB[T]) Gte(v T) *NumB[T] { *b.k.gte = &v; return b }

// Lt sets the exclusive maximum.
func (b *NumB[T]) Lt(v T) *NumB[T] { *b.k.lt = &v; return b }

// Lte sets the inclusive maximum.
func (b *NumB[T]) Lte(v T) *NumB[T] { *b.k.lte = &v; return b }

// In requires the value to be one of vs.
func (b *NumB[T]) In(vs ...T) *NumB[T] { *b.k.in = vs; return b }

// NotIn forbids the value from being any of vs.
func (b *NumB[T]) NotIn(vs ...T) *NumB[T] { *b.k.notIn = vs; return b }

// MultipleOf requires the value to be divisible by v.
func (b *NumB[T]) MultipleOf(v T) *NumB[T] { *b.k.mul = &v; return b }

// Float builds a float field.
func Float(name string) *NumB[float32] {
	k := &Schema_Field_Float{}
	return newNum(name, numSpec[float32]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Float_{Float: k})
}

// Double builds a double field.
func Double(name string) *NumB[float64] {
	k := &Schema_Field_Double{}
	return newNum(name, numSpec[float64]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Double_{Double: k})
}

// Int32 builds an int32 field.
func Int32(name string) *NumB[int32] {
	k := &Schema_Field_Int32{}
	return newNum(name, numSpec[int32]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Int32_{Int32: k})
}

// Int64 builds an int64 field.
func Int64(name string) *NumB[int64] {
	k := &Schema_Field_Int64{}
	return newNum(name, numSpec[int64]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Int64_{Int64: k})
}

// UInt32 builds a uint32 field.
func UInt32(name string) *NumB[uint32] {
	k := &Schema_Field_UInt32{}
	return newNum(name, numSpec[uint32]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Uint32{Uint32: k})
}

// UInt64 builds a uint64 field.
func UInt64(name string) *NumB[uint64] {
	k := &Schema_Field_UInt64{}
	return newNum(name, numSpec[uint64]{
		def: &k.Default, con: &k.Const, gt: &k.Gt, gte: &k.Gte, lt: &k.Lt, lte: &k.Lte,
		mul: &k.MultipleOf, in: &k.In, notIn: &k.NotIn,
	}, &Schema_Field_Uint64{Uint64: k})
}

// =============================================================================
// Bool / String / Bytes / Enum / Json
// =============================================================================

// BoolB builds a bool field.
type BoolB struct {
	fieldBase[*BoolB]
	k *Schema_Field_Bool
}

// Bool builds a bool field.
func Bool(name string) *BoolB {
	b := &BoolB{k: &Schema_Field_Bool{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Bool_{Bool: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *BoolB) Default(v bool) *BoolB { b.k.Default = &v; return b }

// Const requires the value to equal exactly v.
func (b *BoolB) Const(v bool) *BoolB { b.k.Const = &v; return b }

// StrB builds a string field.
type StrB struct {
	fieldBase[*StrB]
	k *Schema_Field_String
}

// Str builds a string field.
func Str(name string) *StrB {
	b := &StrB{k: &Schema_Field_String{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_String_{String_: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *StrB) Default(v string) *StrB { b.k.Default = &v; return b }

// Const requires the value to equal exactly v.
func (b *StrB) Const(v string) *StrB { b.k.Const = &v; return b }

// Len requires the exact character length.
func (b *StrB) Len(v uint64) *StrB { b.k.Len = &v; return b }

// MinLen sets the minimum character length.
func (b *StrB) MinLen(v uint64) *StrB { b.k.MinLen = &v; return b }

// MaxLen sets the maximum character length.
func (b *StrB) MaxLen(v uint64) *StrB { b.k.MaxLen = &v; return b }

// Pattern sets the RE2 regular expression the value must match.
func (b *StrB) Pattern(v string) *StrB { b.k.Pattern = &v; return b }

// In requires the value to be one of vs.
func (b *StrB) In(vs ...string) *StrB { b.k.In = vs; return b }

// NotIn forbids the value from being any of vs.
func (b *StrB) NotIn(vs ...string) *StrB { b.k.NotIn = vs; return b }

// Format sets the semantic format: a registry identifier ("email", "uuid",
// "k8s.quantity", ...). Unknown formats fail validation loudly.
func (b *StrB) Format(f string) *StrB { b.k.Format = &f; return b }

// BytesB builds a bytes field.
type BytesB struct {
	fieldBase[*BytesB]
	k *Schema_Field_Bytes
}

// Bytes builds a bytes field.
func Bytes(name string) *BytesB {
	b := &BytesB{k: &Schema_Field_Bytes{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Bytes_{Bytes: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *BytesB) Default(v []byte) *BytesB { b.k.Default = v; return b }

// Const requires the value to equal exactly v.
func (b *BytesB) Const(v []byte) *BytesB { b.k.Const = v; return b }

// Len requires the exact byte length.
func (b *BytesB) Len(v uint64) *BytesB { b.k.Len = &v; return b }

// MinLen sets the minimum byte length.
func (b *BytesB) MinLen(v uint64) *BytesB { b.k.MinLen = &v; return b }

// MaxLen sets the maximum byte length.
func (b *BytesB) MaxLen(v uint64) *BytesB { b.k.MaxLen = &v; return b }

// Prefix requires the value to start with p.
func (b *BytesB) Prefix(p []byte) *BytesB { b.k.Prefix = p; return b }

// Suffix requires the value to end with s.
func (b *BytesB) Suffix(s []byte) *BytesB { b.k.Suffix = s; return b }

// In requires the value to be one of vs.
func (b *BytesB) In(vs ...[]byte) *BytesB { b.k.In = vs; return b }

// NotIn forbids the value from being any of vs.
func (b *BytesB) NotIn(vs ...[]byte) *BytesB { b.k.NotIn = vs; return b }

// EnumB builds an enum field.
type EnumB struct {
	fieldBase[*EnumB]
	k *Schema_Field_Enum
}

// Enum builds an enum field (integer value with human labels).
func Enum(name string) *EnumB {
	b := &EnumB{k: &Schema_Field_Enum{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Enum_{Enum: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *EnumB) Default(v int32) *EnumB { b.k.Default = &v; return b }

// Values sets the allowed enum values: integer -> human label.
func (b *EnumB) Values(v map[int32]string) *EnumB { b.k.Values = v; return b }

// DefinedOnly requires the value to be one of the keys in Values.
func (b *EnumB) DefinedOnly() *EnumB { b.k.DefinedOnly = true; return b }

// In requires the value to be one of vs.
func (b *EnumB) In(vs ...int32) *EnumB { b.k.In = vs; return b }

// NotIn forbids the value from being any of vs.
func (b *EnumB) NotIn(vs ...int32) *EnumB { b.k.NotIn = vs; return b }

// Options sets the dynamic options expression: a CEL expression over `root`
// returning the list of allowed integer values (replaces the static set).
func (b *EnumB) Options(e string) *EnumB { b.k.OptionsExpr = &e; return b }

// JsonB builds a free-form JSON field.
type JsonB struct {
	fieldBase[*JsonB]
	k *Schema_Field_Json
}

// Json builds a free-form field: any value passes without structural
// validation (rules still apply).
func Json(name string) *JsonB {
	b := &JsonB{k: &Schema_Field_Json{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Json_{Json: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *JsonB) Default(v *Value) *JsonB { b.k.Default = v; return b }

// =============================================================================
// Duration / Timestamp
// =============================================================================

// DurationB builds a duration field.
type DurationB struct {
	fieldBase[*DurationB]
	k *Schema_Field_Duration
}

// Duration builds a duration field.
func Duration(name string) *DurationB {
	b := &DurationB{k: &Schema_Field_Duration{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Duration_{Duration: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *DurationB) Default(d time.Duration) *DurationB { b.k.Default = durationpb.New(d); return b }

// Gt sets the exclusive minimum.
func (b *DurationB) Gt(d time.Duration) *DurationB { b.k.Gt = durationpb.New(d); return b }

// Gte sets the inclusive minimum.
func (b *DurationB) Gte(d time.Duration) *DurationB { b.k.Gte = durationpb.New(d); return b }

// Lt sets the exclusive maximum.
func (b *DurationB) Lt(d time.Duration) *DurationB { b.k.Lt = durationpb.New(d); return b }

// Lte sets the inclusive maximum.
func (b *DurationB) Lte(d time.Duration) *DurationB { b.k.Lte = durationpb.New(d); return b }

// TimestampB builds a timestamp field.
type TimestampB struct {
	fieldBase[*TimestampB]
	k *Schema_Field_Timestamp
}

// Timestamp builds a timestamp field.
func Timestamp(name string) *TimestampB {
	b := &TimestampB{k: &Schema_Field_Timestamp{}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Timestamp_{Timestamp: b.k}
	return b
}

// Default sets the value used when the field is unset.
func (b *TimestampB) Default(t time.Time) *TimestampB { b.k.Default = timestamppb.New(t); return b }

// Gt sets the exclusive minimum.
func (b *TimestampB) Gt(t time.Time) *TimestampB { b.k.Gt = timestamppb.New(t); return b }

// Gte sets the inclusive minimum.
func (b *TimestampB) Gte(t time.Time) *TimestampB { b.k.Gte = timestamppb.New(t); return b }

// Lt sets the exclusive maximum.
func (b *TimestampB) Lt(t time.Time) *TimestampB { b.k.Lt = timestamppb.New(t); return b }

// Lte sets the inclusive maximum.
func (b *TimestampB) Lte(t time.Time) *TimestampB { b.k.Lte = timestamppb.New(t); return b }

// =============================================================================
// List / Object / Map / Computed / OneOf / Ref
// =============================================================================

// ListB builds a list field.
type ListB struct {
	fieldBase[*ListB]
	k *Schema_Field_List
}

// List builds a list field; items describe the element field(s).
func List(name string, items ...FieldDef) *ListB {
	b := &ListB{k: &Schema_Field_List{}}
	b.fieldBase = newField(name, b)
	for _, d := range items {
		b.k.Items = append(b.k.Items, d.Done())
	}
	b.f.Kind = &Schema_Field_List_{List: b.k}
	return b
}

// MinItems sets the minimum number of items.
func (b *ListB) MinItems(v uint64) *ListB { b.k.MinItems = &v; return b }

// MaxItems sets the maximum number of items.
func (b *ListB) MaxItems(v uint64) *ListB { b.k.MaxItems = &v; return b }

// Unique requires items to be unique.
func (b *ListB) Unique() *ListB { b.k.Unique = true; return b }

// Count sets the dynamic length expression: a CEL expression over `root`
// returning the exact non-negative number of items. Inside an item's rules the
// item's zero-based position is bound as `index`.
func (b *ListB) Count(e string) *ListB { b.k.CountExpr = &e; return b }

// ObjectB builds a nested object field.
type ObjectB struct {
	fieldBase[*ObjectB]
	k *Schema_Field_Object
}

// Object builds a nested object field from its child fields (no identity
// needed for nested schemas).
func Object(name string, fields ...FieldDef) *ObjectB {
	sub := &Schema{}
	for _, d := range fields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b := &ObjectB{k: &Schema_Field_Object{Schema: sub}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Object_{Object: b.k}
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
	k *Schema_Field_Map
}

// Map builds a map field; valueFields describe the schema every map value must
// satisfy (an inline schema, like Object). A map with no valueFields keeps an
// unconstrained value schema: keys stay free, values are accepted as any
// object.
func Map(name string, valueFields ...FieldDef) *MapB {
	sub := &Schema{}
	for _, d := range valueFields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b := &MapB{k: &Schema_Field_Map{ValueSchema: sub}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Map_{Map: b.k}
	return b
}

// Strict enables strict mode on the map's value schema: an unknown key inside
// a map VALUE is rejected (the map's own keys are always free).
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

// ComputedB builds a derived field.
type ComputedB struct {
	fieldBase[*ComputedB]
	k *Schema_Field_Computed
}

// Computed builds a derived field; expr reads root (inputs + computed) and
// produces the value.
func Computed(name, expr string) *ComputedB {
	b := &ComputedB{k: &Schema_Field_Computed{Expr: expr}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Computed_{Computed: b.k}
	return b
}

// Result sets the result-type hint used to marshal the derived value.
func (b *ComputedB) Result(rt Schema_Field_ResultType) *ComputedB { b.k.Result = &rt; return b }

// OneOfB builds a discriminated-union field.
type OneOfB struct {
	fieldBase[*OneOfB]
	k *Schema_Field_OneOf
}

// OneOf builds a discriminated-union field. The value must be an object; the
// discriminator property selects the variant schema to validate against.
func OneOf(name, discriminator string) *OneOfB {
	b := &OneOfB{k: &Schema_Field_OneOf{Discriminator: discriminator, Variants: map[string]*Schema{}}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_OneOf_{OneOf: b.k}
	return b
}

// Variant adds a variant schema under the given key (an inline schema, like
// Object).
func (b *OneOfB) Variant(key string, fields ...FieldDef) *OneOfB {
	sub := &Schema{}
	for _, d := range fields {
		sub.Fields = append(sub.Fields, d.Done())
	}
	b.k.Variants[key] = sub
	return b
}

// RefB builds a Ref field.
type RefB struct {
	fieldBase[*RefB]
}

// Ref builds a field of kind Ref that resolves against a LOCAL def: defName is
// the key in the root schema's defs map the value must validate against.
func Ref(name, defName string) *RefB {
	b := &RefB{}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Ref_{Ref: &Schema_Field_Ref{Target: &Schema_Field_Ref_Name{Name: defName}}}
	return b
}

// RefID builds a Ref field that targets a separately-registered schema by
// identity. The referenced schema must be resolvable at validate time: either
// present in the root defs under its identity key, or pulled in by
// Link(resolver).
func RefID(name string, id *SchemaIdentity) *RefB {
	b := &RefB{}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Ref_{Ref: &Schema_Field_Ref{Target: &Schema_Field_Ref_Id{Id: id}}}
	return b
}

// RefIdentity is RefID with the identity spelled out inline.
func RefIdentity(name, namespace, schemaName, version string) *RefB {
	return RefID(name, &SchemaIdentity{Namespace: namespace, Name: schemaName, Version: version})
}
