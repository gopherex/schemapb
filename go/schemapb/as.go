// Typed extraction from a wire Value: As[T] reads one value as a native Go
// type, reporting presence instead of returning silent zero values (which
// is all the generated oneof getters can do).
//
// One rule, shared by every implementation and pinned by the conformance
// golden value-as.json: a conversion succeeds iff the value is represented
// in the target EXACTLY (lossless round-trip). Numeric values convert
// across kinds under that rule — uint32(5) reads as int64, double(3.0)
// reads as int64, double(3.5) does not, int64(2^53+1) does not read as
// double. Non-numeric targets are strict: no string parsing, no
// truncation, no coercion — "256" never reads as an integer here (input
// coercion is schema resolve's job, not value reading).

package schemapb

import (
	"math"
	"time"
)

// ValueScalar enumerates the native Go types As can extract.
type ValueScalar interface {
	bool | int32 | int64 | uint32 | uint64 | float32 | float64 |
		string | []byte | time.Duration | time.Time
}

// As reads the value as T, reporting whether the value is represented in T
// exactly.
//
//nolint:cyclop,funlen // flat exhaustive dispatch per target type
func As[T ValueScalar](v *Value) (T, bool) {
	var zero T

	var got any

	ok := false

	switch any(zero).(type) {
	case bool:
		if b, isBool := v.GetKind().(*Value_BoolValue); isBool {
			got, ok = b.BoolValue, true
		}
	case int32:
		if n, isInt := valueInt(v); isInt && n >= math.MinInt32 && n <= math.MaxInt32 {
			got, ok = int32(n), true //nolint:gosec // range-checked above
		}
	case int64:
		if n, isInt := valueInt(v); isInt {
			got, ok = n, true
		}
	case uint32:
		if n, isUint := valueUint(v); isUint && n <= math.MaxUint32 {
			got, ok = uint32(n), true
		}
	case uint64:
		if n, isUint := valueUint(v); isUint {
			got, ok = n, true
		}
	case float32:
		if f, isNum := valueDouble(v); isNum && float64(float32(f)) == f {
			got, ok = float32(f), true
		}
	case float64:
		if f, isNum := valueDouble(v); isNum {
			got, ok = f, true
		}
	case string:
		if s, isStr := v.GetKind().(*Value_StringValue); isStr {
			got, ok = s.StringValue, true
		}
	case []byte:
		if b, isBytes := v.GetKind().(*Value_BytesValue); isBytes {
			got, ok = b.BytesValue, true
		}
	case time.Duration:
		if d, isDur := v.GetKind().(*Value_DurationValue); isDur {
			got, ok = d.DurationValue.AsDuration(), true
		}
	case time.Time:
		if t, isTS := v.GetKind().(*Value_TimestampValue); isTS {
			got, ok = t.TimestampValue.AsTime(), true
		}
	}

	if !ok {
		return zero, false
	}

	res, _ := got.(T)

	return res, true
}

// AsList reads the value as a list (own kind only).
func (v *Value) AsList() ([]*Value, bool) {
	if l, ok := v.GetKind().(*Value_ListValue); ok {
		return l.ListValue.GetItems(), true
	}

	return nil, false
}

// AsStruct reads the value as a struct (own kind only).
func (v *Value) AsStruct() (map[string]*Value, bool) {
	if s, ok := v.GetKind().(*Value_StructValue); ok {
		return s.StructValue.GetFields(), true
	}

	return nil, false
}

// KindNull is the value-kind name of the null singleton (values only —
// fields have no null kind).
const KindNull FieldKind = "null"

// ValueKindName reports the wire kind of a value as its spec short name.
//
//nolint:cyclop // flat exhaustive dispatch per wire kind
func ValueKindName(v *Value) FieldKind {
	switch v.GetKind().(type) {
	case *Value_NullValue:
		return KindNull
	case *Value_BoolValue:
		return KindBool
	case *Value_Int32Value:
		return KindInt32
	case *Value_Int64Value:
		return KindInt64
	case *Value_Uint32Value:
		return KindUInt32
	case *Value_Uint64Value:
		return KindUInt64
	case *Value_FloatValue:
		return KindFloat
	case *Value_DoubleValue:
		return KindDouble
	case *Value_StringValue:
		return KindString
	case *Value_BytesValue:
		return KindBytes
	case *Value_DurationValue:
		return KindDuration
	case *Value_TimestampValue:
		return KindTimestamp
	case *Value_ListValue:
		return KindList
	case *Value_StructValue:
		return KindStruct
	default:
		return KindUnspecified
	}
}

// KindStruct is the value-kind name of a struct value (fields use
// KindObject; the wire container is a struct).
const KindStruct FieldKind = "struct"

// valueInt is the exact int64 view of a numeric value.
func valueInt(v *Value) (int64, bool) {
	switch k := v.GetKind().(type) {
	case *Value_Int32Value:
		return int64(k.Int32Value), true
	case *Value_Int64Value:
		return k.Int64Value, true
	case *Value_Uint32Value:
		return int64(k.Uint32Value), true
	case *Value_Uint64Value:
		if k.Uint64Value <= math.MaxInt64 {
			return int64(k.Uint64Value), true
		}
	case *Value_FloatValue:
		return floatToInt(float64(k.FloatValue))
	case *Value_DoubleValue:
		return floatToInt(k.DoubleValue)
	}

	return 0, false
}

func floatToInt(f float64) (int64, bool) {
	if f != math.Trunc(f) || f < math.MinInt64 || f >= math.MaxInt64 {
		return 0, false
	}

	n := int64(f)
	if float64(n) != f {
		return 0, false
	}

	return n, true
}

// valueUint is the exact uint64 view of a numeric value.
func valueUint(v *Value) (uint64, bool) {
	switch k := v.GetKind().(type) {
	case *Value_Int32Value:
		if k.Int32Value >= 0 {
			return uint64(k.Int32Value), true
		}
	case *Value_Int64Value:
		if k.Int64Value >= 0 {
			return uint64(k.Int64Value), true
		}
	case *Value_Uint32Value:
		return uint64(k.Uint32Value), true
	case *Value_Uint64Value:
		return k.Uint64Value, true
	case *Value_FloatValue:
		return floatToUint(float64(k.FloatValue))
	case *Value_DoubleValue:
		return floatToUint(k.DoubleValue)
	}

	return 0, false
}

func floatToUint(f float64) (uint64, bool) {
	if f != math.Trunc(f) || f < 0 || f >= math.MaxUint64 {
		return 0, false
	}

	n := uint64(f)
	if float64(n) != f {
		return 0, false
	}

	return n, true
}

// valueDouble is the exact float64 view of a numeric value.
func valueDouble(v *Value) (float64, bool) {
	switch k := v.GetKind().(type) {
	case *Value_Int32Value:
		return float64(k.Int32Value), true
	case *Value_Uint32Value:
		return float64(k.Uint32Value), true
	case *Value_Int64Value:
		// Range-guard before converting back: float->int conversion is
		// undefined outside the int64 range.
		f := float64(k.Int64Value)
		if f >= math.MinInt64 && f < math.MaxInt64 && int64(f) == k.Int64Value {
			return f, true
		}
	case *Value_Uint64Value:
		f := float64(k.Uint64Value)
		if f < math.MaxUint64 && uint64(f) == k.Uint64Value {
			return f, true
		}
	case *Value_FloatValue:
		return float64(k.FloatValue), true
	case *Value_DoubleValue:
		return k.DoubleValue, true
	}

	return 0, false
}
