package schemapb

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
)

// SetFieldAttribute sets one common or active-kind attribute by its protobuf
// name (snake_case or JSON lowerCamelCase). It is also useful in WithFieldTags
// adapters. Strings/bytes/enums may be bare or JSON quoted; lists, maps and
// messages use protoJSON. Duration attributes additionally accept Go duration
// strings. Setting an attribute replaces that attribute, preserving its siblings.
// Name and kind are structural and cannot be changed through this API.
// An error leaves field unchanged and never includes the supplied value.
func SetFieldAttribute(field *Schema_Field, name, value string) error {
	if field == nil {
		return errors.New("schemapb: attribute: nil field")
	}

	owner := field.ProtoReflect()

	descriptor := attributeDescriptor(owner.Descriptor().Fields(), name)
	if descriptor != nil {
		oneof := descriptor.ContainingOneof()
		if descriptor.Name() == "name" || oneof != nil && !oneof.IsSynthetic() {
			return fmt.Errorf("schemapb: attribute %q is structural", name)
		}
	}

	if descriptor == nil {
		kind := owner.WhichOneof(owner.Descriptor().Oneofs().ByName("kind"))
		if kind != nil {
			owner = owner.Get(kind).Message()
			descriptor = attributeDescriptor(owner.Descriptor().Fields(), name)
		}
	}

	if descriptor == nil {
		return fmt.Errorf("schemapb: unknown or inapplicable attribute %q", name)
	}

	if !owner.IsValid() {
		return fmt.Errorf("schemapb: attribute %q has no kind descriptor instance", name)
	}

	raw, err := attributeJSON(descriptor, strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("schemapb: invalid value for attribute %q (%s)", name, descriptor.Kind())
	}

	key, err := json.Marshal(descriptor.JSONName())
	if err != nil {
		return fmt.Errorf("schemapb: attribute name: %w", err)
	}

	document := append(append(append([]byte{'{'}, key...), ':'), raw...)
	document = append(document, '}')

	patch := owner.New()
	if err := protojson.Unmarshal(document, patch.Interface()); err != nil {
		return fmt.Errorf("schemapb: invalid value for attribute %q (%s)", name, descriptor.Kind())
	}

	owner.Set(descriptor, patch.Get(descriptor))

	return nil
}

//nolint:ireturn // protobuf exposes descriptors as interfaces
func attributeDescriptor(fields protoreflect.FieldDescriptors, name string) protoreflect.FieldDescriptor {
	if descriptor := fields.ByName(protoreflect.Name(name)); descriptor != nil {
		return descriptor
	}

	return fields.ByJSONName(name)
}

//nolint:wrapcheck // caller adds attribute context and omits raw tag values
func attributeJSON(descriptor protoreflect.FieldDescriptor, raw string) ([]byte, error) {
	if descriptor.IsList() || descriptor.IsMap() {
		if raw == string(KindNull) {
			return nil, errors.New("null is not an attribute value")
		}

		return []byte(raw), nil
	}

	switch descriptor.Kind() {
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.EnumKind:
		if strings.HasPrefix(raw, "\"") {
			return []byte(raw), nil
		}

		return json.Marshal(raw)
	case protoreflect.MessageKind:
		if descriptor.Message().FullName() == "google.protobuf.Duration" {
			if strings.HasPrefix(raw, "\"") {
				if err := json.Unmarshal([]byte(raw), &raw); err != nil {
					return nil, err
				}
			}

			if d, err := time.ParseDuration(raw); err == nil {
				return protojson.Marshal(durationpb.New(d))
			}
			// The protobuf duration range exceeds time.Duration's range.
			return json.Marshal(raw)
		}

		if descriptor.Message().FullName() == "google.protobuf.Timestamp" && !strings.HasPrefix(raw, "\"") {
			return json.Marshal(raw)
		}
	default:
	}

	if raw == string(KindNull) {
		return nil, errors.New("null is not an attribute value")
	}

	return []byte(raw), nil
}

func applyFieldTags(raw string, field *Schema_Field) error {
	parts, err := splitFieldTag(raw)
	if err != nil {
		return err
	}

	seen := map[string]bool{}

	for _, part := range parts {
		name, value, ok := strings.Cut(part, "=")

		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return errors.New("schemapb: expected attribute=value")
		}
		// Normalize aliases before checking duplicates.
		owner := field.ProtoReflect()

		descriptor := attributeDescriptor(owner.Descriptor().Fields(), name)
		if descriptor == nil {
			if k := owner.WhichOneof(owner.Descriptor().Oneofs().ByName("kind")); k != nil {
				descriptor = attributeDescriptor(owner.Get(k).Message().Descriptor().Fields(), name)
			}
		}

		if descriptor != nil {
			name = string(descriptor.Name())
		}

		if seen[name] {
			return fmt.Errorf("schemapb: duplicate attribute %q", name)
		}

		seen[name] = true
		if err := SetFieldAttribute(field, name, value); err != nil {
			return err
		}
	}

	return nil
}

// A separator inside a JSON string, object or array is part of the value.
//
//nolint:cyclop // small lexer tracks JSON quoting and balanced containers
func splitFieldTag(raw string) ([]string, error) {
	var (
		parts []string
		stack []rune
	)

	quoted, escaped, start := false, false, 0
	for i, c := range raw {
		if quoted {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				quoted = false
			}

			continue
		}

		switch c {
		case '"':
			quoted = true
		case '{', '[':
			stack = append(stack, c)
		case '}', ']':
			if len(stack) == 0 || c == '}' && stack[len(stack)-1] != '{' || c == ']' && stack[len(stack)-1] != '[' {
				return nil, errors.New("schemapb: unbalanced attribute value")
			}

			stack = stack[:len(stack)-1]
		case ';':
			if len(stack) == 0 {
				if part := strings.TrimSpace(raw[start:i]); part != "" {
					parts = append(parts, part)
				}

				start = i + 1
			}
		}
	}

	if quoted || len(stack) != 0 {
		return nil, errors.New("schemapb: unterminated attribute value")
	}

	if part := strings.TrimSpace(raw[start:]); part != "" {
		parts = append(parts, part)
	}

	return parts, nil
}
