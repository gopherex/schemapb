package schemapb

import (
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file is the single conversion point between the three value worlds:
//
//   wire    — *Value / *StructValue (the typed protobuf contract)
//   native  — plain Go values the engine and CEL operate on
//   schema  — the canonical wire variant dictated by a field's declared kind
//
// Native model (what the engine sees):
//
//	Null      -> nil                 Bool      -> bool
//	Int32/64  -> int64               UInt32/64 -> uint64
//	Float/Double -> float64          String    -> string
//	Bytes     -> []byte              Enum      -> int64
//	Duration  -> time.Duration       Timestamp -> time.Time
//	List      -> []any               Object/Map -> map[string]any
//	Json      -> any of the above
//
// All signed integers widen to int64 and floats to float64 in the native
// model (CEL's int/uint/double are 64-bit); the declared field kind narrows
// them back to the canonical wire variant at the boundary (CanonicalValue).

// =============================================================================
// Constructors (wire values)
// =============================================================================

// NullV returns a wire null.
func NullV() *Value { return &Value{Kind: &Value_NullValue{}} }

// BoolV returns a wire bool.
func BoolV(v bool) *Value { return &Value{Kind: &Value_BoolValue{BoolValue: v}} }

// Int32V returns a wire int32.
func Int32V(v int32) *Value { return &Value{Kind: &Value_Int32Value{Int32Value: v}} }

// Int64V returns a wire int64.
func Int64V(v int64) *Value { return &Value{Kind: &Value_Int64Value{Int64Value: v}} }

// UInt32V returns a wire uint32.
func UInt32V(v uint32) *Value { return &Value{Kind: &Value_Uint32Value{Uint32Value: v}} }

// UInt64V returns a wire uint64.
func UInt64V(v uint64) *Value { return &Value{Kind: &Value_Uint64Value{Uint64Value: v}} }

// FloatV returns a wire float.
func FloatV(v float32) *Value { return &Value{Kind: &Value_FloatValue{FloatValue: v}} }

// DoubleV returns a wire double.
func DoubleV(v float64) *Value { return &Value{Kind: &Value_DoubleValue{DoubleValue: v}} }

// StrV returns a wire string.
func StrV(v string) *Value { return &Value{Kind: &Value_StringValue{StringValue: v}} }

// BytesV returns a wire bytes value.
func BytesV(v []byte) *Value { return &Value{Kind: &Value_BytesValue{BytesValue: v}} }

// DurationV returns a wire duration.
func DurationV(d time.Duration) *Value {
	return &Value{Kind: &Value_DurationValue{DurationValue: durationpb.New(d)}}
}

// TimestampV returns a wire timestamp.
func TimestampV(t time.Time) *Value {
	return &Value{Kind: &Value_TimestampValue{TimestampValue: timestamppb.New(t)}}
}

// ListV returns a wire list of the given items.
func ListV(items ...*Value) *Value {
	return &Value{Kind: &Value_ListValue{ListValue: &ListValue{Items: items}}}
}

// StructV returns a wire struct with the given fields.
func StructV(fields map[string]*Value) *Value {
	return &Value{Kind: &Value_StructValue{StructValue: &StructValue{Fields: fields}}}
}

// =============================================================================
// Wire -> native
// =============================================================================

// ToGo converts a wire value to its native representation.
//
//nolint:cyclop // flat exhaustive wire-variant dispatch
func (v *Value) ToGo() any {
	switch k := v.GetKind().(type) {
	case nil, *Value_NullValue:
		return nil
	case *Value_BoolValue:
		return k.BoolValue
	case *Value_Int32Value:
		return int64(k.Int32Value)
	case *Value_Int64Value:
		return k.Int64Value
	case *Value_Uint32Value:
		return uint64(k.Uint32Value)
	case *Value_Uint64Value:
		return k.Uint64Value
	case *Value_FloatValue:
		return float64(k.FloatValue)
	case *Value_DoubleValue:
		return k.DoubleValue
	case *Value_StringValue:
		return k.StringValue
	case *Value_BytesValue:
		return k.BytesValue
	case *Value_DurationValue:
		return k.DurationValue.AsDuration()
	case *Value_TimestampValue:
		return k.TimestampValue.AsTime()
	case *Value_ListValue:
		items := k.ListValue.GetItems()
		out := make([]any, len(items))

		for i, it := range items {
			out[i] = it.ToGo()
		}

		return out
	case *Value_StructValue:
		return k.StructValue.ToGo()
	default:
		return nil
	}
}

// ToGo converts a wire struct to a native map. A nil struct yields an empty map.
func (s *StructValue) ToGo() map[string]any {
	fields := s.GetFields()
	out := make(map[string]any, len(fields))

	for name, v := range fields {
		out[name] = v.ToGo()
	}

	return out
}

// =============================================================================
// Native -> wire (best fit, no schema)
// =============================================================================

// FromGo converts a native Go value to a wire value using the best-fitting
// variant (int64 for signed integers, uint64 for unsigned, double for floats).
// Use CanonicalValue when the field kind is known — it picks the exact
// contract variant. Unsupported types return an error.
//
//nolint:cyclop,funlen // flat exhaustive native-type dispatch
func FromGo(x any) (*Value, error) {
	switch t := x.(type) {
	case nil:
		return NullV(), nil
	case *Value:
		return t, nil
	case bool:
		return BoolV(t), nil
	case int:
		return Int64V(int64(t)), nil
	case int8:
		return Int64V(int64(t)), nil
	case int16:
		return Int64V(int64(t)), nil
	case int32:
		return Int64V(int64(t)), nil
	case int64:
		return Int64V(t), nil
	case uint:
		return UInt64V(uint64(t)), nil
	case uint8:
		return UInt64V(uint64(t)), nil
	case uint16:
		return UInt64V(uint64(t)), nil
	case uint32:
		return UInt64V(uint64(t)), nil
	case uint64:
		return UInt64V(t), nil
	case float32:
		return DoubleV(float64(t)), nil
	case float64:
		return DoubleV(t), nil
	case string:
		return StrV(t), nil
	case []byte:
		return BytesV(t), nil
	case time.Duration:
		return DurationV(t), nil
	case time.Time:
		return TimestampV(t), nil
	case []any:
		items := make([]*Value, len(t))

		for i, el := range t {
			v, err := FromGo(el)
			if err != nil {
				return nil, fmt.Errorf("list[%d]: %w", i, err)
			}

			items[i] = v
		}

		return ListV(items...), nil
	case map[string]any:
		fields := make(map[string]*Value, len(t))

		for name, el := range t {
			v, err := FromGo(el)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}

			fields[name] = v
		}

		return StructV(fields), nil
	default:
		return nil, fmt.Errorf("schemapb: cannot convert %T to Value", x)
	}
}

// MustFromGo is FromGo that panics on an unsupported type. For literals and
// tests; prefer FromGo for runtime data.
func MustFromGo(x any) *Value {
	v, err := FromGo(x)
	if err != nil {
		panic(err)
	}

	return v
}

// StructFromGo converts a native map to a wire struct.
func StructFromGo(m map[string]any) (*StructValue, error) {
	fields := make(map[string]*Value, len(m))

	for name, el := range m {
		v, err := FromGo(el)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		fields[name] = v
	}

	return &StructValue{Fields: fields}, nil
}

// MustStructFromGo is StructFromGo that panics on an unsupported type. For
// literals and tests; prefer StructFromGo for runtime data.
func MustStructFromGo(m map[string]any) *StructValue {
	st, err := StructFromGo(m)
	if err != nil {
		panic(err)
	}

	return st
}

// =============================================================================
// Native numeric coercion helpers (shared by validate/compute)
// =============================================================================

// asInt64 extracts a signed integer from any native numeric representation.
// Floats convert only when integral; uints only when in range.
func asInt64(x any) (int64, bool) {
	switch n := x.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case uint64:
		if n <= math.MaxInt64 {
			return int64(n), true
		}
	case float64:
		if n == math.Trunc(n) && n >= math.MinInt64 && n <= math.MaxInt64 {
			return int64(n), true
		}
	case float32:
		return asInt64(float64(n))
	}

	return 0, false
}

// asUint64 extracts an unsigned integer from any native numeric representation.
func asUint64(x any) (uint64, bool) {
	switch n := x.(type) {
	case uint64:
		return n, true
	case uint32:
		return uint64(n), true
	case int64:
		if n >= 0 {
			return uint64(n), true
		}
	case int:
		if n >= 0 {
			return uint64(n), true
		}
	case float64:
		if n == math.Trunc(n) && n >= 0 && n <= math.MaxUint64 {
			return uint64(n), true
		}
	case float32:
		return asUint64(float64(n))
	}

	return 0, false
}

// asFloat64 extracts a float from any native numeric representation.
func asFloat64(x any) (float64, bool) {
	switch n := x.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	}

	return 0, false
}

// =============================================================================
// Canonical form (field kind -> exact wire variant)
// =============================================================================

// CanonicalValue converts a native value to the exact wire variant the field's
// declared kind mandates (the contract's canonical form): an Int64 field's
// value is always int64_value, a Float field's always float_value, and so on.
// A value that cannot represent the kind (wrong type, out of range) returns an
// error; the validator reports such values as TYPE_MISMATCH before this point.
//
//nolint:gocognit,cyclop,gocyclo,funlen // flat exhaustive kind dispatch
func CanonicalValue(f *Schema_Field, x any) (*Value, error) {
	if x == nil {
		return NullV(), nil
	}

	switch f.GetKind().(type) {
	case *Schema_Field_Float_:
		n, ok := asFloat64(x)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not numeric", f.GetName(), x)
		}

		return FloatV(float32(n)), nil
	case *Schema_Field_Double_:
		n, ok := asFloat64(x)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not numeric", f.GetName(), x)
		}

		return DoubleV(n), nil
	case *Schema_Field_Int32_:
		n, ok := asInt64(x)
		if !ok || n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("field %s: %v does not fit int32", f.GetName(), x)
		}

		return Int32V(int32(n)), nil
	case *Schema_Field_Int64_:
		n, ok := asInt64(x)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not an integer", f.GetName(), x)
		}

		return Int64V(n), nil
	case *Schema_Field_Uint32:
		n, ok := asUint64(x)
		if !ok || n > math.MaxUint32 {
			return nil, fmt.Errorf("field %s: %v does not fit uint32", f.GetName(), x)
		}

		return UInt32V(uint32(n)), nil
	case *Schema_Field_Uint64:
		n, ok := asUint64(x)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not an unsigned integer", f.GetName(), x)
		}

		return UInt64V(n), nil
	case *Schema_Field_Bool_:
		b, ok := x.(bool)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not bool", f.GetName(), x)
		}

		return BoolV(b), nil
	case *Schema_Field_String_:
		s, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not string", f.GetName(), x)
		}

		return StrV(s), nil
	case *Schema_Field_Bytes_:
		b, ok := x.([]byte)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not bytes", f.GetName(), x)
		}

		return BytesV(b), nil
	case *Schema_Field_Choice_:
		// Choice values are typed by their options; canonical form is the
		// best-fit variant of the native value.
		return FromGo(x)
	case *Schema_Field_Duration_:
		d, ok := x.(time.Duration)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not a duration", f.GetName(), x)
		}

		return DurationV(d), nil
	case *Schema_Field_Timestamp_:
		t, ok := x.(time.Time)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not a timestamp", f.GetName(), x)
		}

		return TimestampV(t), nil
	case *Schema_Field_List_:
		arr, ok := x.([]any)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not a list", f.GetName(), x)
		}

		items := make([]*Value, len(arr))

		for i, el := range arr {
			var v *Value

			var err error
			if item := listItemDef(f.GetList(), i); item != nil {
				v, err = CanonicalValue(item, el)
			} else {
				v, err = FromGo(el)
			}

			if err != nil {
				return nil, fmt.Errorf("field %s[%d]: %w", f.GetName(), i, err)
			}

			items[i] = v
		}

		return ListV(items...), nil
	case *Schema_Field_Object_:
		m, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not an object", f.GetName(), x)
		}

		return canonicalStruct(f.GetObject().GetSchema(), m, f.GetName())
	case *Schema_Field_Map_:
		m, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field %s: %T is not a map", f.GetName(), x)
		}

		vs := f.GetMap().GetValueSchema()
		fields := make(map[string]*Value, len(m))

		for key, el := range m {
			if vs != nil {
				em, isObj := el.(map[string]any)
				if !isObj {
					return nil, fmt.Errorf("field %s.%s: %T is not an object", f.GetName(), key, el)
				}

				v, err := canonicalStruct(vs, em, f.GetName()+"."+key)
				if err != nil {
					return nil, err
				}

				fields[key] = v

				continue
			}

			v, err := FromGo(el)
			if err != nil {
				return nil, fmt.Errorf("field %s.%s: %w", f.GetName(), key, err)
			}

			fields[key] = v
		}

		return StructV(fields), nil
	case *Schema_Field_Json_:
		return FromGo(x)
	case *Schema_Field_Computed_:
		// Computed values canonicalize by the declared result type.
		return canonicalResult(f.GetComputed().GetResult(), x)
	default:
		// OneOf / Ref values canonicalize structurally (variant/def schemas are
		// resolved by the engine, which canonicalizes through them instead).
		return FromGo(x)
	}
}

// canonicalStruct canonicalizes a native map against a schema's declared
// fields; keys without a declared field fall back to best-fit conversion.
func canonicalStruct(s *Schema, m map[string]any, path string) (*Value, error) {
	fields := make(map[string]*Value, len(m))

	for key, el := range m {
		var fld *Schema_Field

		for _, f := range s.GetFields() {
			if f.GetName() == key {
				fld = f

				break
			}
		}

		var v *Value

		var err error
		if fld != nil {
			v, err = CanonicalValue(fld, el)
		} else {
			v, err = FromGo(el)
		}

		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", path, key, err)
		}

		fields[key] = v
	}

	return StructV(fields), nil
}

// canonicalResult canonicalizes a computed result by its declared ResultType.
//
//nolint:cyclop // flat exhaustive ResultType dispatch
func canonicalResult(rt Schema_Field_ResultType, x any) (*Value, error) {
	switch rt {
	case Schema_Field_RESULT_TYPE_DOUBLE:
		if n, ok := asFloat64(x); ok {
			return DoubleV(n), nil
		}
	case Schema_Field_RESULT_TYPE_INT64:
		if n, ok := asInt64(x); ok {
			return Int64V(n), nil
		}
	case Schema_Field_RESULT_TYPE_UINT64:
		if n, ok := asUint64(x); ok {
			return UInt64V(n), nil
		}
	case Schema_Field_RESULT_TYPE_BOOL:
		if b, ok := x.(bool); ok {
			return BoolV(b), nil
		}
	case Schema_Field_RESULT_TYPE_STRING:
		if s, ok := x.(string); ok {
			return StrV(s), nil
		}
	case Schema_Field_RESULT_TYPE_DURATION:
		if d, ok := x.(time.Duration); ok {
			return DurationV(d), nil
		}
	case Schema_Field_RESULT_TYPE_TIMESTAMP:
		if t, ok := x.(time.Time); ok {
			return TimestampV(t), nil
		}
	case Schema_Field_RESULT_TYPE_BYTES:
		if b, ok := x.([]byte); ok {
			return BytesV(b), nil
		}
	default:
		return FromGo(x)
	}

	return nil, fmt.Errorf("computed result %T does not match declared type %v", x, rt)
}
