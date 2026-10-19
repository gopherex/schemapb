package protomsg

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	anyName       = "google.protobuf.Any"
	structName    = "google.protobuf.Struct"
	valueName     = "google.protobuf.Value"
	listValueName = "google.protobuf.ListValue"
)

// ErrWriteBack reports a resolved value that does not fit its proto field
// (a computed value of the wrong type, an integer out of range, ...).
var ErrWriteBack = errors.New("protomsg: resolved value does not fit the field")

// writeBack stores the resolved value tree into msg, all or nothing: it
// writes into a clone and replaces msg only when every field converted.
// Keys absent from values leave their field unchanged. An Any of a listed
// type is unpacked, written and packed again, so what was validated is what
// is stored; an Any of any other type was seen only by its "@type" and is
// left as is.
func (mp mapping) writeBack(msg proto.Message, values map[string]any) error {
	clone := proto.Clone(msg)
	if err := mp.setMessage(clone.ProtoReflect(), values); err != nil {
		return err
	}

	proto.Reset(msg)
	proto.Merge(msg, clone)

	return nil
}

func (mp mapping) setMessage(m protoreflect.Message, values map[string]any) error {
	fields := m.Descriptor().Fields()

	for i := range fields.Len() {
		field := fields.Get(i)

		x, ok := values[string(field.Name())]
		if !ok {
			continue
		}

		if err := mp.setField(m, field, x); err != nil {
			return err
		}
	}

	return nil
}

// isObject reports message kinds written field by field: plain messages,
// and Any (unpacked, written, packed again).
func isObject(md protoreflect.MessageDescriptor) bool {
	return md != nil && (!isWellKnown(md) || md.FullName() == anyName)
}

// setObject writes obj into a plain message, or into the message an Any
// packs when its type is listed.
func (mp mapping) setObject(m protoreflect.Message, obj map[string]any) error {
	if m.Descriptor().FullName() != anyName {
		return mp.setMessage(m, obj)
	}

	desc := m.Descriptor()
	url := m.Get(desc.Fields().ByName("type_url")).String()

	t, ok := mp.anyTypes[url]
	if !ok {
		return nil // only "@type" was seen: nothing resolved
	}

	inner := t.New()
	raw := desc.Fields().ByName("value")

	if err := proto.Unmarshal(m.Get(raw).Bytes(), inner.Interface()); err != nil {
		return fmt.Errorf("%w: unpack Any %s: %w", ErrWriteBack, url, err)
	}

	if err := mp.setMessage(inner, obj); err != nil {
		return err
	}

	packed, err := proto.MarshalOptions{Deterministic: true}.Marshal(inner.Interface())
	if err != nil {
		return fmt.Errorf("%w: pack Any %s: %w", ErrWriteBack, url, err)
	}

	m.Set(raw, protoreflect.ValueOfBytes(packed))

	return nil
}

func (mp mapping) setField(m protoreflect.Message, field protoreflect.FieldDescriptor, x any) error {
	switch {
	case x == nil:
		m.Clear(field)

		return nil
	case field.IsMap():
		return mp.setMap(m, field, x)
	case field.IsList():
		return mp.setList(m, field, x)
	case isObject(field.Message()):
		obj, ok := x.(map[string]any)
		if !ok {
			return misfit(field, x)
		}

		return mp.setObject(m.Mutable(field).Message(), obj)
	default:
		v, err := toProto(field, m.NewField(field), x)
		if err != nil {
			return err
		}

		m.Set(field, v)

		return nil
	}
}

func (mp mapping) setList(m protoreflect.Message, field protoreflect.FieldDescriptor, x any) error {
	items, ok := x.([]any)
	if !ok {
		return misfit(field, x)
	}

	list := m.Mutable(field).List()
	objects := isObject(field.Message())

	for i, item := range items {
		if objects {
			obj, isObj := item.(map[string]any)
			if !isObj {
				return misfit(field, item)
			}

			if i >= list.Len() {
				list.Append(list.NewElement())
			}

			if err := mp.setObject(list.Get(i).Message(), obj); err != nil {
				return err
			}

			continue
		}

		v, err := toProto(field, list.NewElement(), item)
		if err != nil {
			return err
		}

		if i < list.Len() {
			list.Set(i, v)
		} else {
			list.Append(v)
		}
	}

	list.Truncate(len(items))

	return nil
}

func (mp mapping) setMap(m protoreflect.Message, field protoreflect.FieldDescriptor, x any) error {
	entries, ok := x.(map[string]any)
	if !ok {
		return misfit(field, x)
	}

	target := m.Mutable(field).Map()
	value := field.MapValue()
	objects := isObject(value.Message())
	keep := map[string]bool{} // MapKey.String() of the keys to keep

	for k, item := range entries {
		key, err := mapKey(field.MapKey(), k)
		if err != nil {
			return err
		}

		keep[key.String()] = true

		if objects {
			obj, isObj := item.(map[string]any)
			if !isObj {
				return misfit(field, item)
			}

			if err := mp.setObject(target.Mutable(key).Message(), obj); err != nil {
				return err
			}

			continue
		}

		v, err := toProto(value, target.NewValue(), item)
		if err != nil {
			return err
		}

		target.Set(key, v)
	}

	target.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		if !keep[k.String()] {
			target.Clear(k)
		}

		return true
	})

	return nil
}

// mapKey reads a string key of the value tree back as the proto key kind.
func mapKey(field protoreflect.FieldDescriptor, k string) (protoreflect.MapKey, error) {
	v, err := parseKey(field.Kind(), k)
	if err != nil {
		return protoreflect.MapKey{}, fmt.Errorf("%w: %s key %q: %w", ErrWriteBack, field.FullName(), k, err)
	}

	return v.MapKey(), nil
}

func parseKey(kind protoreflect.Kind, k string) (protoreflect.Value, error) {
	switch kind {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(k), nil
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(k)

		return protoreflect.ValueOfBool(b), err
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := strconv.ParseInt(k, 10, 32)

		return protoreflect.ValueOfInt32(int32(n)), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := strconv.ParseInt(k, 10, 64)

		return protoreflect.ValueOfInt64(n), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := strconv.ParseUint(k, 10, 32)

		return protoreflect.ValueOfUint32(uint32(n)), err
	default:
		n, err := strconv.ParseUint(k, 10, 64)

		return protoreflect.ValueOfUint64(n), err
	}
}

// toProto converts one native value to the proto value of field's kind;
// zero is a fresh value of the field (needed for message kinds).
//
//nolint:cyclop // flat exhaustive proto kind dispatch
func toProto(field protoreflect.FieldDescriptor, zero protoreflect.Value, x any) (protoreflect.Value, error) {
	switch field.Kind() {
	case protoreflect.BoolKind:
		if b, ok := x.(bool); ok {
			return protoreflect.ValueOfBool(b), nil
		}
	case protoreflect.EnumKind:
		return enumValue(field, x)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if n, ok := toInt(x, math.MinInt32, math.MaxInt32); ok {
			return protoreflect.ValueOfInt32(int32(n)), nil //nolint:gosec // range-checked
		}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if n, ok := toInt(x, math.MinInt64, math.MaxInt64); ok {
			return protoreflect.ValueOfInt64(n), nil
		}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if n, ok := toUint(x, math.MaxUint32); ok {
			return protoreflect.ValueOfUint32(uint32(n)), nil //nolint:gosec // range-checked
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if n, ok := toUint(x, math.MaxUint64); ok {
			return protoreflect.ValueOfUint64(n), nil
		}
	case protoreflect.FloatKind:
		if f, ok := toFloat(x); ok {
			return protoreflect.ValueOfFloat32(float32(f)), nil
		}
	case protoreflect.DoubleKind:
		if f, ok := toFloat(x); ok {
			return protoreflect.ValueOfFloat64(f), nil
		}
	case protoreflect.StringKind:
		if s, ok := x.(string); ok {
			return protoreflect.ValueOfString(s), nil
		}
	case protoreflect.BytesKind:
		if b, ok := x.([]byte); ok {
			return protoreflect.ValueOfBytes(b), nil
		}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if err := wellKnown(zero.Message(), x); err != nil {
			return protoreflect.Value{}, fmt.Errorf("%s: %w", field.FullName(), err)
		}

		return zero, nil
	}

	return protoreflect.Value{}, misfit(field, x)
}

func enumValue(field protoreflect.FieldDescriptor, x any) (protoreflect.Value, error) {
	switch v := x.(type) {
	case string:
		if ev := field.Enum().Values().ByName(protoreflect.Name(v)); ev != nil {
			return protoreflect.ValueOfEnum(ev.Number()), nil
		}
	default:
		if n, ok := toInt(x, math.MinInt32, math.MaxInt32); ok {
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)), nil //nolint:gosec // range-checked
		}
	}

	return protoreflect.Value{}, misfit(field, x)
}

// wellKnown fills m (a well-known type) from its natural value.
func wellKnown(m protoreflect.Message, x any) error {
	desc := m.Descriptor()
	set := func(name protoreflect.Name, v protoreflect.Value) { m.Set(desc.Fields().ByName(name), v) }

	switch desc.FullName() {
	case "google.protobuf.Timestamp":
		t, ok := x.(time.Time)
		if !ok {
			return misfitWKT(desc, x)
		}

		set("seconds", protoreflect.ValueOfInt64(t.Unix()))
		set("nanos", protoreflect.ValueOfInt32(int32(t.Nanosecond()))) //nolint:gosec // < 1e9

		return nil
	case "google.protobuf.Duration":
		d, ok := x.(time.Duration)
		if !ok {
			return misfitWKT(desc, x)
		}

		set("seconds", protoreflect.ValueOfInt64(int64(d/time.Second)))
		set("nanos", protoreflect.ValueOfInt32(int32(d%time.Second))) //nolint:gosec // |n| < 1e9

		return nil
	case "google.protobuf.Empty":
		return nil
	case "google.protobuf.FieldMask":
		items, ok := x.([]any)
		if !ok {
			return misfitWKT(desc, x)
		}

		paths := m.Mutable(desc.Fields().ByName("paths")).List()

		for _, item := range items {
			s, isStr := item.(string)
			if !isStr {
				return misfitWKT(desc, x)
			}

			paths.Append(protoreflect.ValueOfString(s))
		}

		return nil
	case structName, valueName, listValueName:
		return jsonInto(m, x)
	default: // wrappers
		field := desc.Fields().ByName("value")

		v, err := toProto(field, m.NewField(field), x)
		if err != nil {
			return err
		}

		m.Set(field, v)

		return nil
	}
}

// jsonInto fills a Struct, Value or ListValue (generated or dynamic) by
// round-tripping through structpb, which shares their wire format.
func jsonInto(m protoreflect.Message, x any) error {
	var (
		src proto.Message
		err error
	)

	switch m.Descriptor().FullName() {
	case structName:
		obj, ok := x.(map[string]any)
		if !ok {
			return misfitWKT(m.Descriptor(), x)
		}

		src, err = structpb.NewStruct(obj)
	case listValueName:
		items, ok := x.([]any)
		if !ok {
			return misfitWKT(m.Descriptor(), x)
		}

		src, err = structpb.NewList(items)
	default:
		src, err = structpb.NewValue(x)
	}

	if err != nil {
		return fmt.Errorf("%w: %w", ErrWriteBack, err)
	}

	raw, err := proto.Marshal(src)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWriteBack, err)
	}

	if err := proto.Unmarshal(raw, m.Interface()); err != nil {
		return fmt.Errorf("%w: %w", ErrWriteBack, err)
	}

	return nil
}

func toInt(x any, low, high int64) (int64, bool) {
	switch v := x.(type) {
	case int64:
		return v, v >= low && v <= high
	case int:
		return int64(v), int64(v) >= low && int64(v) <= high
	case uint64:
		return int64(v), v <= uint64(high) //nolint:gosec // bounded by high
	case float64:
		n := int64(v)

		return n, float64(n) == v && n >= low && n <= high
	default:
		return 0, false
	}
}

func toUint(x any, high uint64) (uint64, bool) {
	switch v := x.(type) {
	case uint64:
		return v, v <= high
	case int64:
		return uint64(v), v >= 0 && uint64(v) <= high //nolint:gosec // v >= 0
	case int:
		return uint64(v), v >= 0 && uint64(v) <= high //nolint:gosec // v >= 0
	case float64:
		n := uint64(v)

		return n, v >= 0 && float64(n) == v && n <= high
	default:
		return 0, false
	}
}

func toFloat(x any) (float64, bool) {
	switch v := x.(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func misfit(field protoreflect.FieldDescriptor, x any) error {
	return fmt.Errorf("%w: %s (%s) cannot hold %T", ErrWriteBack, field.FullName(), field.Kind(), x)
}

func misfitWKT(desc protoreflect.MessageDescriptor, x any) error {
	return fmt.Errorf("%w: %s cannot hold %T", ErrWriteBack, desc.FullName(), x)
}
