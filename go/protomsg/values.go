package protomsg

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ErrUnsupported reports a message field the value mapping cannot express.
var ErrUnsupported = errors.New("protomsg: unsupported field type")

// Values presents msg as the value tree the schemapb engine validates (see
// the package documentation for the mapping).
func Values(msg proto.Message) (map[string]any, error) {
	return messageValues(msg.ProtoReflect())
}

func messageValues(m protoreflect.Message) (map[string]any, error) {
	fields := m.Descriptor().Fields()
	out := make(map[string]any, fields.Len())

	for i := range fields.Len() {
		field := fields.Get(i)
		if field.HasPresence() && !m.Has(field) {
			continue
		}

		v, err := fieldValue(field, m.Get(field))
		if err != nil {
			return nil, err
		}

		out[string(field.Name())] = v
	}

	return out, nil
}

func fieldValue(field protoreflect.FieldDescriptor, v protoreflect.Value) (any, error) {
	switch {
	case field.IsMap():
		mp := v.Map()
		out := make(map[string]any, mp.Len())

		var err error

		mp.Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			var x any

			x, err = singular(field.MapValue(), mv)
			out[k.String()] = x

			return err == nil
		})

		return out, err
	case field.IsList():
		list := v.List()
		out := make([]any, list.Len())

		for i := range list.Len() {
			x, err := singular(field, list.Get(i))
			if err != nil {
				return nil, err
			}

			out[i] = x
		}

		return out, nil
	default:
		return singular(field, v)
	}
}

func singular(field protoreflect.FieldDescriptor, v protoreflect.Value) (any, error) {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return v.Bool(), nil
	case protoreflect.EnumKind:
		if ev := field.Enum().Values().ByNumber(v.Enum()); ev != nil {
			return string(ev.Name()), nil
		}

		return int64(v.Enum()), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int(), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint(), nil
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return v.Float(), nil
	case protoreflect.StringKind:
		return v.String(), nil
	case protoreflect.BytesKind:
		return v.Bytes(), nil
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageValue(v.Message())
	default:
		return nil, fmt.Errorf("%w: %s has kind %s", ErrUnsupported, field.FullName(), field.Kind())
	}
}

func messageValue(m protoreflect.Message) (any, error) {
	desc := m.Descriptor()

	switch desc.FullName() {
	case "google.protobuf.Timestamp":
		return time.Unix(intField(m, "seconds"), intField(m, "nanos")).UTC(), nil
	case "google.protobuf.Duration":
		return time.Duration(intField(m, "seconds"))*time.Second + time.Duration(intField(m, "nanos")), nil
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value",
		"google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.BoolValue", "google.protobuf.StringValue", "google.protobuf.BytesValue":
		value := desc.Fields().ByName("value")

		return singular(value, m.Get(value))
	case "google.protobuf.Struct":
		return structValue(m)
	case "google.protobuf.Value":
		return jsonValue(m)
	case "google.protobuf.ListValue":
		return listValue(m)
	case "google.protobuf.FieldMask":
		paths := m.Get(desc.Fields().ByName("paths")).List()
		out := make([]any, paths.Len())

		for i := range paths.Len() {
			out[i] = paths.Get(i).String()
		}

		return out, nil
	case "google.protobuf.Any":
		return nil, fmt.Errorf("%w: google.protobuf.Any", ErrUnsupported)
	default:
		return messageValues(m)
	}
}

func intField(m protoreflect.Message, name protoreflect.Name) int64 {
	return m.Get(m.Descriptor().Fields().ByName(name)).Int()
}

func structValue(m protoreflect.Message) (map[string]any, error) {
	fields := m.Get(m.Descriptor().Fields().ByName("fields")).Map()
	out := make(map[string]any, fields.Len())

	var err error

	fields.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		var x any

		x, err = jsonValue(v.Message())
		out[k.String()] = x

		return err == nil
	})

	return out, err
}

func listValue(m protoreflect.Message) ([]any, error) {
	values := m.Get(m.Descriptor().Fields().ByName("values")).List()
	out := make([]any, values.Len())

	for i := range values.Len() {
		x, err := jsonValue(values.Get(i).Message())
		if err != nil {
			return nil, err
		}

		out[i] = x
	}

	return out, nil
}

// jsonValue maps google.protobuf.Value; an unset kind is null, as in protoJSON.
//
//nolint:nilnil // nil is the JSON null value, not a missing result
func jsonValue(m protoreflect.Message) (any, error) {
	kind := m.WhichOneof(m.Descriptor().Oneofs().ByName("kind"))
	if kind == nil {
		return nil, nil
	}

	v := m.Get(kind)

	switch kind.Name() {
	case "number_value":
		return v.Float(), nil
	case "string_value":
		return v.String(), nil
	case "bool_value":
		return v.Bool(), nil
	case "struct_value":
		return structValue(v.Message())
	case "list_value":
		return listValue(v.Message())
	default: // null_value
		return nil, nil
	}
}
