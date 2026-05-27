package schemapb

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Ptr returns a pointer to v. Handy for the many optional scalar fields,
// e.g. Ptr("hello") or Ptr[float32](1.5).
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
)

// =============================================================================
// Schema
// =============================================================================

// SchemaOption configures a Schema.
type SchemaOption func(*Schema)

// Schema builds a Schema (form descriptor) from the given options.
func NewSchema(opts ...SchemaOption) *Schema {
	s := &Schema{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Identity sets the schema identity. namespace and version may be empty; name
// is required by the validator.
func Identity(namespace, name, version string) SchemaOption {
	return func(s *Schema) {
		s.Id = &SchemaIdentity{Namespace: namespace, Name: name, Version: version}
	}
}

// Description sets the human description of the schema.
func Description(desc string) SchemaOption {
	return func(s *Schema) { s.Description = &desc }
}

// Fields sets the form fields.
func Fields(fields ...*Schema_Filed) SchemaOption {
	return func(s *Schema) { s.Fields = fields }
}

// Rules sets the form-wide invariant rules (`root` only).
func Rules(rules ...*Schema_Filed_Rule) SchemaOption {
	return func(s *Schema) { s.Rules = rules }
}

// =============================================================================
// Field
// =============================================================================

// FieldOption configures a Schema_Filed.
type FieldOption func(*Schema_Filed)

// Kind sets the oneof field kind on a Schema_Filed. Produced by the kind
// builders (Float, Int32, String, Object, ...).
type Kind func(*Schema_Filed)

// Field builds a field with the given name and kind:
//
//	Field("temp", Float(FloatGte(0), FloatLte(100)), FieldRequired())
func Field(name string, kind Kind, opts ...FieldOption) *Schema_Filed {
	f := &Schema_Filed{Name: name}
	kind(f)
	for _, o := range opts {
		o(f)
	}
	return f
}

// FieldDescription sets the human description.
func FieldDescription(desc string) FieldOption {
	return func(f *Schema_Filed) { f.Description = &desc }
}

// FieldNullable marks the value as allowed to be null/empty.
func FieldNullable() FieldOption {
	return func(f *Schema_Filed) { f.Nullable = true }
}

// FieldRequired marks the value as required.
func FieldRequired() FieldOption {
	return func(f *Schema_Filed) { f.Required = true }
}

// FieldRules sets the cross-field CEL validation rules (see NewRule).
func FieldRules(rules ...*Schema_Filed_Rule) FieldOption {
	return func(f *Schema_Filed) { f.Rules = rules }
}

// FieldImmutable marks the field as system-fixed: it is forced to its default
// and any different submitted value is rejected (renderers show it disabled).
func FieldImmutable() FieldOption {
	return func(f *Schema_Filed) { f.Immutable = true }
}

// FieldGroup sets the informative section label (the engine ignores it).
func FieldGroup(group string) FieldOption {
	return func(f *Schema_Filed) { f.Group = &group }
}

// FieldUnit sets the informative value unit, e.g. "MB" (the engine ignores it).
func FieldUnit(unit string) FieldOption {
	return func(f *Schema_Filed) { f.Unit = &unit }
}

// =============================================================================
// Kind builders — each returns a Kind setter accepted by Field.
// =============================================================================

// FloatOption configures a Float kind.
type FloatOption func(*Schema_Filed_Float)

// Float builds a float field kind.
func Float(opts ...FloatOption) Kind {
	k := &Schema_Filed_Float{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Float_{Float: k} }
}

func FloatDefault(v float32) FloatOption  { return func(k *Schema_Filed_Float) { k.Default = &v } }
func FloatConst(v float32) FloatOption    { return func(k *Schema_Filed_Float) { k.Const = &v } }
func FloatGt(v float32) FloatOption       { return func(k *Schema_Filed_Float) { k.Gt = &v } }
func FloatGte(v float32) FloatOption      { return func(k *Schema_Filed_Float) { k.Gte = &v } }
func FloatLt(v float32) FloatOption       { return func(k *Schema_Filed_Float) { k.Lt = &v } }
func FloatLte(v float32) FloatOption      { return func(k *Schema_Filed_Float) { k.Lte = &v } }
func FloatIn(v ...float32) FloatOption    { return func(k *Schema_Filed_Float) { k.In = v } }
func FloatNotIn(v ...float32) FloatOption { return func(k *Schema_Filed_Float) { k.NotIn = v } }
func FloatMultipleOf(v float32) FloatOption {
	return func(k *Schema_Filed_Float) { k.MultipleOf = &v }
}

// DoubleOption configures a Double kind.
type DoubleOption func(*Schema_Filed_Double)

// Double builds a double field kind.
func Double(opts ...DoubleOption) Kind {
	k := &Schema_Filed_Double{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Double_{Double: k} }
}

func DoubleDefault(v float64) DoubleOption  { return func(k *Schema_Filed_Double) { k.Default = &v } }
func DoubleConst(v float64) DoubleOption    { return func(k *Schema_Filed_Double) { k.Const = &v } }
func DoubleGt(v float64) DoubleOption       { return func(k *Schema_Filed_Double) { k.Gt = &v } }
func DoubleGte(v float64) DoubleOption      { return func(k *Schema_Filed_Double) { k.Gte = &v } }
func DoubleLt(v float64) DoubleOption       { return func(k *Schema_Filed_Double) { k.Lt = &v } }
func DoubleLte(v float64) DoubleOption      { return func(k *Schema_Filed_Double) { k.Lte = &v } }
func DoubleIn(v ...float64) DoubleOption    { return func(k *Schema_Filed_Double) { k.In = v } }
func DoubleNotIn(v ...float64) DoubleOption { return func(k *Schema_Filed_Double) { k.NotIn = v } }
func DoubleMultipleOf(v float64) DoubleOption {
	return func(k *Schema_Filed_Double) { k.MultipleOf = &v }
}

// Int32Option configures an Int32 kind.
type Int32Option func(*Schema_Filed_Int32)

// Int32 builds an int32 field kind.
func Int32(opts ...Int32Option) Kind {
	k := &Schema_Filed_Int32{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Int32_{Int32: k} }
}

func Int32Default(v int32) Int32Option  { return func(k *Schema_Filed_Int32) { k.Default = &v } }
func Int32Const(v int32) Int32Option    { return func(k *Schema_Filed_Int32) { k.Const = &v } }
func Int32Gt(v int32) Int32Option       { return func(k *Schema_Filed_Int32) { k.Gt = &v } }
func Int32Gte(v int32) Int32Option      { return func(k *Schema_Filed_Int32) { k.Gte = &v } }
func Int32Lt(v int32) Int32Option       { return func(k *Schema_Filed_Int32) { k.Lt = &v } }
func Int32Lte(v int32) Int32Option      { return func(k *Schema_Filed_Int32) { k.Lte = &v } }
func Int32In(v ...int32) Int32Option    { return func(k *Schema_Filed_Int32) { k.In = v } }
func Int32NotIn(v ...int32) Int32Option { return func(k *Schema_Filed_Int32) { k.NotIn = v } }
func Int32MultipleOf(v int32) Int32Option {
	return func(k *Schema_Filed_Int32) { k.MultipleOf = &v }
}

// Int64Option configures an Int64 kind.
type Int64Option func(*Schema_Filed_Int64)

// Int64 builds an int64 field kind.
func Int64(opts ...Int64Option) Kind {
	k := &Schema_Filed_Int64{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Int64_{Int64: k} }
}

func Int64Default(v int64) Int64Option  { return func(k *Schema_Filed_Int64) { k.Default = &v } }
func Int64Const(v int64) Int64Option    { return func(k *Schema_Filed_Int64) { k.Const = &v } }
func Int64Gt(v int64) Int64Option       { return func(k *Schema_Filed_Int64) { k.Gt = &v } }
func Int64Gte(v int64) Int64Option      { return func(k *Schema_Filed_Int64) { k.Gte = &v } }
func Int64Lt(v int64) Int64Option       { return func(k *Schema_Filed_Int64) { k.Lt = &v } }
func Int64Lte(v int64) Int64Option      { return func(k *Schema_Filed_Int64) { k.Lte = &v } }
func Int64In(v ...int64) Int64Option    { return func(k *Schema_Filed_Int64) { k.In = v } }
func Int64NotIn(v ...int64) Int64Option { return func(k *Schema_Filed_Int64) { k.NotIn = v } }
func Int64MultipleOf(v int64) Int64Option {
	return func(k *Schema_Filed_Int64) { k.MultipleOf = &v }
}

// UInt32Option configures a UInt32 kind.
type UInt32Option func(*Schema_Filed_UInt32)

// UInt32 builds a uint32 field kind.
func UInt32(opts ...UInt32Option) Kind {
	k := &Schema_Filed_UInt32{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Uint32{Uint32: k} }
}

func UInt32Default(v uint32) UInt32Option  { return func(k *Schema_Filed_UInt32) { k.Default = &v } }
func UInt32Const(v uint32) UInt32Option    { return func(k *Schema_Filed_UInt32) { k.Const = &v } }
func UInt32Gt(v uint32) UInt32Option       { return func(k *Schema_Filed_UInt32) { k.Gt = &v } }
func UInt32Gte(v uint32) UInt32Option      { return func(k *Schema_Filed_UInt32) { k.Gte = &v } }
func UInt32Lt(v uint32) UInt32Option       { return func(k *Schema_Filed_UInt32) { k.Lt = &v } }
func UInt32Lte(v uint32) UInt32Option      { return func(k *Schema_Filed_UInt32) { k.Lte = &v } }
func UInt32In(v ...uint32) UInt32Option    { return func(k *Schema_Filed_UInt32) { k.In = v } }
func UInt32NotIn(v ...uint32) UInt32Option { return func(k *Schema_Filed_UInt32) { k.NotIn = v } }
func UInt32MultipleOf(v uint32) UInt32Option {
	return func(k *Schema_Filed_UInt32) { k.MultipleOf = &v }
}

// UInt64Option configures a UInt64 kind.
type UInt64Option func(*Schema_Filed_UInt64)

// UInt64 builds a uint64 field kind.
func UInt64(opts ...UInt64Option) Kind {
	k := &Schema_Filed_UInt64{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Uint64{Uint64: k} }
}

func UInt64Default(v uint64) UInt64Option  { return func(k *Schema_Filed_UInt64) { k.Default = &v } }
func UInt64Const(v uint64) UInt64Option    { return func(k *Schema_Filed_UInt64) { k.Const = &v } }
func UInt64Gt(v uint64) UInt64Option       { return func(k *Schema_Filed_UInt64) { k.Gt = &v } }
func UInt64Gte(v uint64) UInt64Option      { return func(k *Schema_Filed_UInt64) { k.Gte = &v } }
func UInt64Lt(v uint64) UInt64Option       { return func(k *Schema_Filed_UInt64) { k.Lt = &v } }
func UInt64Lte(v uint64) UInt64Option      { return func(k *Schema_Filed_UInt64) { k.Lte = &v } }
func UInt64In(v ...uint64) UInt64Option    { return func(k *Schema_Filed_UInt64) { k.In = v } }
func UInt64NotIn(v ...uint64) UInt64Option { return func(k *Schema_Filed_UInt64) { k.NotIn = v } }
func UInt64MultipleOf(v uint64) UInt64Option {
	return func(k *Schema_Filed_UInt64) { k.MultipleOf = &v }
}

// BoolOption configures a Bool kind.
type BoolOption func(*Schema_Filed_Bool)

// Bool builds a bool field kind.
func Bool(opts ...BoolOption) Kind {
	k := &Schema_Filed_Bool{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Bool_{Bool: k} }
}

func BoolDefault(v bool) BoolOption { return func(k *Schema_Filed_Bool) { k.Default = &v } }
func BoolConst(v bool) BoolOption   { return func(k *Schema_Filed_Bool) { k.Const = &v } }

// StringOption configures a String kind.
type StringOption func(*Schema_Filed_String)

// String builds a string field kind.
func String(opts ...StringOption) Kind {
	k := &Schema_Filed_String{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_String_{String_: k} }
}

func StringDefault(v string) StringOption  { return func(k *Schema_Filed_String) { k.Default = &v } }
func StringConst(v string) StringOption    { return func(k *Schema_Filed_String) { k.Const = &v } }
func StringLen(v uint64) StringOption      { return func(k *Schema_Filed_String) { k.Len = &v } }
func StringMinLen(v uint64) StringOption   { return func(k *Schema_Filed_String) { k.MinLen = &v } }
func StringMaxLen(v uint64) StringOption   { return func(k *Schema_Filed_String) { k.MaxLen = &v } }
func StringPattern(v string) StringOption  { return func(k *Schema_Filed_String) { k.Pattern = &v } }
func StringIn(v ...string) StringOption    { return func(k *Schema_Filed_String) { k.In = v } }
func StringNotIn(v ...string) StringOption { return func(k *Schema_Filed_String) { k.NotIn = v } }

// EnumOption configures an Enum kind.
type EnumOption func(*Schema_Filed_Enum)

// Enum builds an enum field kind.
func Enum(opts ...EnumOption) Kind {
	k := &Schema_Filed_Enum{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Enum_{Enum: k} }
}

func EnumDefault(v int32) EnumOption           { return func(k *Schema_Filed_Enum) { k.Default = &v } }
func EnumValues(v map[int32]string) EnumOption { return func(k *Schema_Filed_Enum) { k.Values = v } }
func EnumDefinedOnly() EnumOption              { return func(k *Schema_Filed_Enum) { k.DefinedOnly = true } }
func EnumIn(v ...int32) EnumOption             { return func(k *Schema_Filed_Enum) { k.In = v } }
func EnumNotIn(v ...int32) EnumOption          { return func(k *Schema_Filed_Enum) { k.NotIn = v } }

// DurationOption configures a Duration kind.
type DurationOption func(*Schema_Filed_Duration)

// Duration builds a duration field kind.
func Duration(opts ...DurationOption) Kind {
	k := &Schema_Filed_Duration{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Duration_{Duration: k} }
}

func DurationDefault(d time.Duration) DurationOption {
	return func(k *Schema_Filed_Duration) { k.Default = durationpb.New(d) }
}
func DurationGt(d time.Duration) DurationOption {
	return func(k *Schema_Filed_Duration) { k.Gt = durationpb.New(d) }
}
func DurationGte(d time.Duration) DurationOption {
	return func(k *Schema_Filed_Duration) { k.Gte = durationpb.New(d) }
}
func DurationLt(d time.Duration) DurationOption {
	return func(k *Schema_Filed_Duration) { k.Lt = durationpb.New(d) }
}
func DurationLte(d time.Duration) DurationOption {
	return func(k *Schema_Filed_Duration) { k.Lte = durationpb.New(d) }
}

// TimestampOption configures a Timestamp kind.
type TimestampOption func(*Schema_Filed_Timestamp)

// Timestamp builds a timestamp field kind.
func Timestamp(opts ...TimestampOption) Kind {
	k := &Schema_Filed_Timestamp{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Timestamp_{Timestamp: k} }
}

func TimestampDefault(t time.Time) TimestampOption {
	return func(k *Schema_Filed_Timestamp) { k.Default = timestamppb.New(t) }
}
func TimestampGt(t time.Time) TimestampOption {
	return func(k *Schema_Filed_Timestamp) { k.Gt = timestamppb.New(t) }
}
func TimestampGte(t time.Time) TimestampOption {
	return func(k *Schema_Filed_Timestamp) { k.Gte = timestamppb.New(t) }
}
func TimestampLt(t time.Time) TimestampOption {
	return func(k *Schema_Filed_Timestamp) { k.Lt = timestamppb.New(t) }
}
func TimestampLte(t time.Time) TimestampOption {
	return func(k *Schema_Filed_Timestamp) { k.Lte = timestamppb.New(t) }
}

// ListOption configures a List kind.
type ListOption func(*Schema_Filed_List)

// List builds a list field kind. Items describe the element field(s).
func List(opts ...ListOption) Kind {
	k := &Schema_Filed_List{}
	for _, o := range opts {
		o(k)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_List_{List: k} }
}

func ListItems(items ...*Schema_Filed) ListOption {
	return func(k *Schema_Filed_List) { k.Items = items }
}
func ListMinItems(v uint64) ListOption { return func(k *Schema_Filed_List) { k.MinItems = &v } }
func ListMaxItems(v uint64) ListOption { return func(k *Schema_Filed_List) { k.MaxItems = &v } }
func ListUnique() ListOption           { return func(k *Schema_Filed_List) { k.Unique = true } }

// Object builds a nested object field kind from a sub-schema.
func Object(schema *Schema) Kind {
	return func(f *Schema_Filed) {
		f.Kind = &Schema_Filed_Object_{Object: &Schema_Filed_Object{Schema: schema}}
	}
}

// ComputedOption configures a Computed kind.
type ComputedOption func(*Schema_Filed_Computed)

// Computed builds a derived field kind. expr reads root (inputs + already
// computed values) and produces the field's value.
func Computed(expr string, opts ...ComputedOption) Kind {
	c := &Schema_Filed_Computed{Expr: expr}
	for _, o := range opts {
		o(c)
	}
	return func(f *Schema_Filed) { f.Kind = &Schema_Filed_Computed_{Computed: c} }
}

// ComputedResult sets the result type hint used to marshal the derived value.
func ComputedResult(rt Schema_Filed_ResultType) ComputedOption {
	return func(c *Schema_Filed_Computed) { c.Result = &rt }
}

// =============================================================================
// Rule
// =============================================================================

// RuleOption configures a validation Rule.
type RuleOption func(*Schema_Filed_Rule)

// NewRule builds a CEL validation rule. expr must evaluate to bool; true means
// valid. message is shown to the user when expr is false.
func NewRule(expr, message string, opts ...RuleOption) *Schema_Filed_Rule {
	r := &Schema_Filed_Rule{Expr: expr, Message: message}
	for _, o := range opts {
		o(r)
	}
	return r
}

// RuleID sets the stable rule id.
func RuleID(id string) RuleOption {
	return func(r *Schema_Filed_Rule) { r.Id = &id }
}

// RuleSeverity sets the rule severity (defaults to ERROR).
func RuleSeverity(s Schema_Filed_Severity) RuleOption {
	return func(r *Schema_Filed_Rule) { r.Severity = &s }
}

// =============================================================================
// FieldError
// =============================================================================

// FieldErrorOption configures a FieldError.
type FieldErrorOption func(*FieldError)

// NewFieldError builds a single validation failure for the given field.
func NewFieldError(field, message string, opts ...FieldErrorOption) *FieldError {
	e := &FieldError{Field: field, Message: message}
	for _, o := range opts {
		o(e)
	}
	return e
}

// FieldErrorRuleID sets the id of the rule that failed.
func FieldErrorRuleID(id string) FieldErrorOption {
	return func(e *FieldError) { e.RuleId = &id }
}

// FieldErrorSeverity sets the failure severity.
func FieldErrorSeverity(s Schema_Filed_Severity) FieldErrorOption {
	return func(e *FieldError) { e.Severity = s }
}
