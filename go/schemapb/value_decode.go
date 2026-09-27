package schemapb

import (
	"encoding"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// DecodeError describes an incompatible destination or value. Path uses the
// same field[index].child notation as validation. Source values are omitted.
type DecodeError struct {
	Path   string
	Target reflect.Type
	Kind   FieldKind
	Reason string
}

func (e *DecodeError) Error() string {
	path := e.Path
	if path == "" {
		path = "<root>"
	}

	return fmt.Sprintf("schemapb: decode %s (%s -> %v): %s", path, e.Kind, e.Target, e.Reason)
}

// Decode reads a typed wire object into a non-nil pointer to a Go struct,
// string-keyed map, or empty interface. JSON field names and untagged embedded
// structs follow Reflect's mapping. Unknown/ambiguous fields, incompatible
// types, numeric overflow and lossy conversions fail with a DecodeError.
//
// Decode replaces the target only on success; absent fields become zero.
// It does not apply defaults, validation, or string coercion. Pointers, native
// time values, bytes, named scalars and nested containers are supported.
// JSON/Text unmarshaler hooks run on fresh values; errors omit their payloads.
func (s *StructValue) Decode(target any) error {
	dst := reflect.ValueOf(target)
	if !dst.IsValid() || dst.Kind() != reflect.Pointer || dst.IsNil() {
		return &DecodeError{Target: reflect.TypeOf(target), Kind: "struct", Reason: "target must be a non-nil pointer"}
	}

	if s == nil {
		return &DecodeError{Target: dst.Elem().Type(), Kind: "struct", Reason: "nil StructValue"}
	}

	next := reflect.New(dst.Elem().Type()).Elem()
	if err := decodeValue(StructV(s.GetFields()), next, "", 0); err != nil {
		return err
	}

	dst.Elem().Set(next)

	return nil
}

// Decode reads the snapshot's values. It does not revalidate or resolve them.
func (b *Baked) Decode(target any) error {
	return b.GetValues().Decode(target)
}

func decodeFailure(v *Value, dst reflect.Value, path, reason string) error {
	return &DecodeError{Path: path, Target: dst.Type(), Kind: ValueKindName(v), Reason: reason}
}

//nolint:cyclop,gocognit,gocyclo,funlen // explicit native-kind dispatch, preserving exact values
func decodeValue(v *Value, dst reflect.Value, path string, depth int) error {
	const maxDecodeDepth = 256
	if depth > maxDecodeDepth {
		return decodeFailure(v, dst, path, "nesting limit exceeded")
	}

	if !dst.CanSet() {
		return decodeFailure(v, dst, path, "destination is not settable")
	}

	if v == nil || v.GetKind() == nil || ValueKindName(v) == KindNull {
		switch dst.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			dst.SetZero()
			return nil
		default:
			return decodeFailure(v, dst, path, "null requires a nullable destination")
		}
	}

	if dst.Kind() == reflect.Pointer {
		dst.Set(reflect.New(dst.Type().Elem()))
		return decodeValue(v, dst.Elem(), path, depth+1)
	}

	if dst.Type() == durationType {
		d := v.GetDurationValue()
		if d == nil || d.CheckValid() != nil || !proto.Equal(durationpb.New(d.AsDuration()), d) {
			return decodeFailure(v, dst, path, "expected a duration representable by time.Duration")
		}

		dst.SetInt(int64(d.AsDuration()))

		return nil
	}

	if dst.Type() == timeType {
		ts := v.GetTimestampValue()
		if ts == nil || ts.CheckValid() != nil {
			return decodeFailure(v, dst, path, "expected a valid timestamp")
		}

		dst.Set(reflect.ValueOf(ts.AsTime()))

		return nil
	}

	if handled, err := decodeCustom(v, dst, path, depth); handled {
		return err
	}

	switch dst.Kind() {
	case reflect.Interface:
		if dst.NumMethod() != 0 {
			return decodeFailure(v, dst, path, "destination interface must be empty")
		}

		return decodeInterface(v, dst, path, depth)
	case reflect.Struct:
		if _, ok := v.GetKind().(*Value_StructValue); !ok {
			return decodeFailure(v, dst, path, "expected an object")
		}

		return decodeStruct(v.GetStructValue(), dst, path, depth)
	case reflect.Map:
		if _, ok := v.GetKind().(*Value_StructValue); !ok || dst.Type().Key().Kind() != reflect.String {
			return decodeFailure(v, dst, path, "expected an object and string map keys")
		}

		fields := v.GetStructValue().GetFields()
		dst.Set(reflect.MakeMapWithSize(dst.Type(), len(fields)))

		for _, key := range slices.Sorted(maps.Keys(fields)) {
			el := reflect.New(dst.Type().Elem()).Elem()
			if err := decodeValue(fields[key], el, joinPath(path, key), depth+1); err != nil {
				return err
			}

			k := reflect.New(dst.Type().Key()).Elem()
			k.SetString(key)
			dst.SetMapIndex(k, el)
		}

		return nil
	case reflect.Slice, reflect.Array:
		return decodeSequence(v, dst, path, depth)
	case reflect.String:
		if x, ok := v.GetKind().(*Value_StringValue); ok {
			dst.SetString(x.StringValue)
			return nil
		}
	case reflect.Bool:
		if x, ok := v.GetKind().(*Value_BoolValue); ok {
			dst.SetBool(x.BoolValue)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := valueInt(v); ok && !dst.OverflowInt(n) {
			dst.SetInt(n)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := valueUint(v); ok && !dst.OverflowUint(n) {
			dst.SetUint(n)
			return nil
		}
	case reflect.Float32:
		if n, ok := As[float32](v); ok {
			dst.SetFloat(float64(n))
			return nil
		}
	case reflect.Float64:
		if n, ok := As[float64](v); ok {
			dst.SetFloat(n)
			return nil
		}
	default:
	}

	return decodeFailure(v, dst, path, "incompatible type or lossy conversion")
}

func decodeCustom(v *Value, dst reflect.Value, path string, depth int) (bool, error) {
	if !dst.CanAddr() || !dst.Addr().CanInterface() {
		return false, nil
	}

	if decoder, ok := dst.Addr().Interface().(json.Unmarshaler); ok {
		var native any
		if err := decodeValue(v, reflect.ValueOf(&native).Elem(), path, depth+1); err != nil {
			return true, err
		}

		data, err := json.Marshal(native)
		if err != nil {
			return true, decodeFailure(v, dst, path, "value has no JSON representation")
		}

		if err := decoder.UnmarshalJSON(data); err != nil {
			return true, decodeFailure(v, dst, path, "custom JSON decoder failed")
		}

		return true, nil
	}

	if decoder, ok := dst.Addr().Interface().(encoding.TextUnmarshaler); ok {
		str, isString := v.GetKind().(*Value_StringValue)
		if !isString {
			return true, decodeFailure(v, dst, path, "text decoder requires a string")
		}

		if err := decoder.UnmarshalText([]byte(str.StringValue)); err != nil {
			return true, decodeFailure(v, dst, path, "custom text decoder failed")
		}

		return true, nil
	}

	return false, nil
}

func decodeSequence(v *Value, dst reflect.Value, path string, depth int) error {
	if dst.Type().Elem().Kind() == reflect.Uint8 {
		b, ok := v.GetKind().(*Value_BytesValue)
		if !ok {
			return decodeFailure(v, dst, path, "expected bytes")
		}

		if dst.Kind() == reflect.Array && dst.Len() != len(b.BytesValue) {
			return decodeFailure(v, dst, path, "byte array length mismatch")
		}

		if dst.Kind() == reflect.Slice {
			dst.Set(reflect.MakeSlice(dst.Type(), len(b.BytesValue), len(b.BytesValue)))
		}

		for i, n := range b.BytesValue {
			dst.Index(i).SetUint(uint64(n))
		}

		return nil
	}

	list, ok := v.GetKind().(*Value_ListValue)
	if !ok {
		return decodeFailure(v, dst, path, "expected a list or bytes")
	}

	items := list.ListValue.GetItems()
	if dst.Kind() == reflect.Array && dst.Len() != len(items) {
		return decodeFailure(v, dst, path, "array length mismatch")
	}

	if dst.Kind() == reflect.Slice {
		dst.Set(reflect.MakeSlice(dst.Type(), len(items), len(items)))
	}

	for i, el := range items {
		if err := decodeValue(el, dst.Index(i), fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
			return err
		}
	}

	return nil
}

func decodeInterface(v *Value, dst reflect.Value, path string, depth int) error {
	var typ reflect.Type

	switch v.GetKind().(type) {
	case *Value_StructValue:
		typ = reflect.TypeFor[map[string]any]()
	case *Value_ListValue:
		typ = reflect.TypeFor[[]any]()
	case *Value_BytesValue:
		typ = reflect.TypeFor[[]byte]()
	case *Value_DurationValue:
		typ = durationType
	case *Value_TimestampValue:
		typ = timeType
	default:
		native := v.ToGo()
		if native == nil {
			return decodeFailure(v, dst, path, "unsupported wire kind")
		}

		dst.Set(reflect.ValueOf(native))

		return nil
	}

	concrete := reflect.New(typ).Elem()
	if err := decodeValue(v, concrete, path, depth+1); err != nil {
		return err
	}

	dst.Set(concrete)

	return nil
}

func decodeStruct(s *StructValue, dst reflect.Value, path string, depth int) error {
	fields := map[string][]int{}
	if err := decodeFieldIndexes(dst.Type(), nil, map[reflect.Type]bool{}, fields); err != nil {
		return decodeFailure(StructV(s.GetFields()), dst, path, err.Error())
	}

	for _, name := range slices.Sorted(maps.Keys(s.GetFields())) {
		childPath := joinPath(path, name)

		indexes, ok := fields[name]
		if !ok {
			return decodeFailure(s.GetFields()[name], dst, childPath, "unknown destination field")
		}

		child := dst
		for _, idx := range indexes {
			if child.Kind() == reflect.Pointer {
				if child.IsNil() {
					if !child.CanSet() {
						return decodeFailure(s.GetFields()[name], child, childPath, "cannot allocate unexported embedded pointer")
					}

					child.Set(reflect.New(child.Type().Elem()))
				}

				child = child.Elem()
			}

			child = child.Field(idx)
		}

		if err := decodeValue(s.GetFields()[name], child, childPath, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// Same flattening/name rules as Reflect. Ambiguous names fail rather than
// choosing a field differently from the schema producer.
func decodeFieldIndexes(t reflect.Type, prefix []int, visited map[reflect.Type]bool, out map[string][]int) error {
	if visited[t] {
		return nil
	}

	visited[t] = true
	defer delete(visited, t)

	for i := range t.NumField() {
		structField := t.Field(i)
		if !structField.IsExported() && !structField.Anonymous {
			continue
		}

		name, _, skip := jsonName(&structField)
		if skip {
			continue
		}

		indexes := append(slices.Clone(prefix), i)

		if structField.Anonymous && structField.Tag.Get("json") == "" {
			sub := structField.Type
			for sub.Kind() == reflect.Pointer {
				sub = sub.Elem()
			}

			if sub.Kind() == reflect.Struct {
				if err := decodeFieldIndexes(sub, indexes, visited, out); err != nil {
					return err
				}

				continue
			}
		}

		if !structField.IsExported() {
			continue
		}

		if _, exists := out[name]; exists {
			return fmt.Errorf("ambiguous destination field %q", name)
		}

		out[name] = indexes
	}

	return nil
}
