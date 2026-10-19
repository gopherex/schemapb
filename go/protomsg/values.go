package protomsg

import (
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// ErrUnsupported reports a message field the value mapping cannot express.
	ErrUnsupported = errors.New("protomsg: unsupported field type")
	// ErrOutOfRange reports a Duration outside the range the engine
	// represents (time.Duration, about ±292 years). It is rejected rather
	// than wrapped, so a bound can never be bypassed by overflow.
	ErrOutOfRange = errors.New("protomsg: duration out of range")
)

// Option configures the value mapping.
type Option func(*mapping)

// WithAnyTypes lists the message types a google.protobuf.Any may hold and
// that are unpacked into the value tree. An Any holding any other type
// presents only its "@type".
func WithAnyTypes(types ...protoreflect.MessageType) Option {
	return func(m *mapping) {
		for _, t := range types {
			m.anyTypes[anyURLPrefix+string(t.Descriptor().FullName())] = t
		}
	}
}

// anyURLPrefix is the type URL prefix protobuf runtimes write.
const anyURLPrefix = "type.googleapis.com/"

// AnyTypeKey is the property naming an Any's type URL in the value tree.
const AnyTypeKey = "@type"

// mapping is the immutable configuration of one conversion.
type mapping struct {
	anyTypes map[string]protoreflect.MessageType
}

func newMapping(opts []Option) mapping {
	m := mapping{anyTypes: map[string]protoreflect.MessageType{}}
	for _, o := range opts {
		o(&m)
	}

	return m
}

// Values presents msg as the value tree the schemapb engine validates (see
// the package documentation for the mapping).
func Values(msg proto.Message, opts ...Option) (map[string]any, error) {
	return newMapping(opts).messageValues(msg.ProtoReflect())
}

func (mp mapping) messageValues(m protoreflect.Message) (map[string]any, error) {
	fields := m.Descriptor().Fields()
	out := make(map[string]any, fields.Len())

	for i := range fields.Len() {
		field := fields.Get(i)
		if field.HasPresence() && !m.Has(field) {
			continue
		}

		v, err := mp.fieldValue(field, m.Get(field))
		if err != nil {
			return nil, err
		}

		out[string(field.Name())] = v
	}

	return out, nil
}

func (mp mapping) fieldValue(field protoreflect.FieldDescriptor, v protoreflect.Value) (any, error) {
	switch {
	case field.IsMap():
		entries := v.Map()
		out := make(map[string]any, entries.Len())

		var err error

		entries.Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			var x any

			x, err = mp.singular(field.MapValue(), mv)
			out[k.String()] = x

			return err == nil
		})

		return out, err
	case field.IsList():
		list := v.List()
		out := make([]any, list.Len())

		for i := range list.Len() {
			x, err := mp.singular(field, list.Get(i))
			if err != nil {
				return nil, err
			}

			out[i] = x
		}

		return out, nil
	default:
		return mp.singular(field, v)
	}
}

func (mp mapping) singular(field protoreflect.FieldDescriptor, v protoreflect.Value) (any, error) {
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
		return mp.messageValue(v.Message())
	default:
		return nil, fmt.Errorf("%w: %s has kind %s", ErrUnsupported, field.FullName(), field.Kind())
	}
}

func (mp mapping) messageValue(m protoreflect.Message) (any, error) {
	desc := m.Descriptor()

	switch desc.FullName() {
	case "google.protobuf.Timestamp":
		return time.Unix(intField(m, "seconds"), intField(m, "nanos")).UTC(), nil
	case "google.protobuf.Duration":
		return duration(intField(m, "seconds"), intField(m, "nanos"))
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value",
		"google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.BoolValue", "google.protobuf.StringValue", "google.protobuf.BytesValue":
		value := desc.Fields().ByName("value")

		return mp.singular(value, m.Get(value))
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
		return mp.anyValue(m)
	default:
		return mp.messageValues(m)
	}
}

// anyValue unpacks an Any of a listed type into its fields plus "@type";
// any other Any presents only "@type".
func (mp mapping) anyValue(m protoreflect.Message) (any, error) {
	desc := m.Descriptor()
	url := m.Get(desc.Fields().ByName("type_url")).String()

	t, ok := mp.anyTypes[url]
	if !ok {
		return map[string]any{AnyTypeKey: url}, nil
	}

	inner := t.New()
	if err := proto.Unmarshal(m.Get(desc.Fields().ByName("value")).Bytes(), inner.Interface()); err != nil {
		return nil, fmt.Errorf("protomsg: unpack Any %s: %w", url, err)
	}

	out, err := mp.messageValues(inner)
	if err != nil {
		return nil, err
	}

	out[AnyTypeKey] = url

	return out, nil
}

func duration(seconds, nanos int64) (time.Duration, error) {
	const maxSeconds = math.MaxInt64 / int64(time.Second)

	outOfRange := fmt.Errorf("%w: %ds %dns", ErrOutOfRange, seconds, nanos)

	// google.protobuf.Duration: |nanos| < 1e9 and the same sign as seconds.
	if nanos <= -int64(time.Second) || nanos >= int64(time.Second) ||
		(seconds > 0 && nanos < 0) || (seconds < 0 && nanos > 0) ||
		seconds > maxSeconds || seconds < -maxSeconds {
		return 0, outOfRange
	}

	whole := time.Duration(seconds) * time.Second
	sum := whole + time.Duration(nanos)

	// Same-sign addition overflowed iff the sign flipped.
	if (nanos > 0 && sum < whole) || (nanos < 0 && sum > whole) {
		return 0, outOfRange
	}

	return sum, nil
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
