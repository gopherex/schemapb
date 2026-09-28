package schemapb

import "google.golang.org/protobuf/proto"

// Masked returns an independent display copy of the snapshot values. Every
// present secret field becomes the string "***", including nulls and inactive
// fields. Containers are traversed using the embedded schema, without CEL or
// resolution. Unresolvable container schemas are concealed conservatively.
// The result is for display, not for validation or baking back into a config.
func (b *Baked) Masked() *StructValue {
	out, _ := proto.Clone(b.GetValues()).(*StructValue)
	if out == nil {
		return &StructValue{}
	}

	maskStruct(b.GetSchema(), out, b.GetSchema().GetDefs())

	return out
}

// maskStruct only mutates the private clone owned by Masked.
func maskStruct(schema *Schema, values *StructValue, defs map[string]*Schema) {
	if schema == nil {
		for key := range values.GetFields() {
			values.Fields[key] = StrV("***")
		}

		return
	}

	for _, field := range schema.GetFields() {
		if value, present := values.GetFields()[field.GetName()]; present {
			values.Fields[field.GetName()] = maskValue(field, value, defs)
		}
	}
}

func maskValue(field *Schema_Field, value *Value, defs map[string]*Schema) *Value {
	if field.GetSecret() {
		return StrV("***")
	}

	if value == nil || value.GetKind() == nil {
		return value
	}

	if _, isNull := value.GetKind().(*Value_NullValue); isNull {
		return value
	}

	switch {
	case field.GetObject() != nil:
		return maskObject(field.GetObject().GetSchema(), value, defs)
	case field.GetRef() != nil:
		return maskObject(defs[refDefKey(field.GetRef())], value, defs)
	case field.GetOneOf() != nil:
		oo := field.GetOneOf()
		discriminator := value.GetStructValue().GetFields()[oo.GetDiscriminator()]

		disc, ok := discriminator.GetKind().(*Value_StringValue)
		if !ok {
			return StrV("***")
		}

		return maskObject(oo.GetVariants()[disc.StringValue], value, defs)
	case field.GetList() != nil:
		if value.GetListValue() == nil {
			return StrV("***")
		}

		for i, item := range value.GetListValue().GetItems() {
			if def := listItemDef(field.GetList(), i); def != nil {
				value.GetListValue().Items[i] = maskValue(def, item, defs)
			}
		}
	case field.GetMap() != nil:
		return maskMap(field.GetMap(), value, defs)
	}

	return value
}

func maskObject(schema *Schema, value *Value, defs map[string]*Schema) *Value {
	if schema == nil || value.GetStructValue() == nil {
		return StrV("***")
	}

	maskStruct(schema, value.GetStructValue(), defs)

	return value
}

func maskMap(mp *Schema_Field_Map, value *Value, defs map[string]*Schema) *Value {
	if value.GetStructValue() == nil {
		return StrV("***")
	}

	for key, item := range value.GetStructValue().GetFields() {
		if def := mp.GetValueField(); def != nil {
			value.GetStructValue().Fields[key] = maskValue(def, item, defs)
		} else if schema := mp.GetValueSchema(); schema != nil {
			if _, isNull := item.GetKind().(*Value_NullValue); !isNull {
				value.GetStructValue().Fields[key] = maskObject(schema, item, defs)
			}
		}
	}

	return value
}
