package schemapb

import (
	"fmt"
	"reflect"
	"strconv"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Enum builds a closed integer choice from a protobuf enum descriptor.
// Labels are protobuf value names, in declaration order. Numeric aliases use
// the first declared name. Modify the resulting option descriptors to supply
// application-specific labels. A nil descriptor yields an empty
// choice, which schema validation rejects unless it is made open or populated.
func Enum(name FieldName, descriptor protoreflect.EnumDescriptor) *ChoiceB {
	b := Choice(name)
	if descriptor == nil {
		return b
	}

	seen := map[protoreflect.EnumNumber]bool{}

	values := descriptor.Values()
	for i := range values.Len() {
		value := values.Get(i)
		if !seen[value.Number()] {
			b.Opt(Int32V(int32(value.Number())), string(value.Name()))
			seen[value.Number()] = true
		}
	}

	return b
}

func reflectedEnum(name FieldName, typ reflect.Type, constraints *fieldConstraints) (*Schema_Field, error) {
	value, _ := reflect.Zero(typ).Interface().(protoreflect.Enum)

	field := Enum(name, value.Descriptor()).Done()
	if len(constraints.oneof) == 0 {
		return field, nil
	}

	allowed := map[int32]bool{}

	for _, raw := range constraints.oneof {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value.Descriptor().Values().ByNumber(protoreflect.EnumNumber(n)) == nil {
			return nil, fmt.Errorf("enum %s: oneof must contain declared integer values", typ)
		}

		allowed[int32(n)] = true
	}

	options := field.GetChoice().Options
	field.GetChoice().Options = nil

	for _, option := range options {
		if allowed[option.Value.GetInt32Value()] {
			field.GetChoice().Options = append(field.GetChoice().Options, option)
		}
	}

	return field, nil
}
