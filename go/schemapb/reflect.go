// Reflect: a Go type -> a Schema. The type's JSON shape is what travels,
// so json tags decide names/presence exactly like encoding/json; the
// go-playground/validator tag vocabulary (plus desc/pattern) becomes
// constraints. Everything unrepresentable fails LOUDLY: unsupported kinds,
// non-identifier json names, non-string map keys, malformed tag values.
//
// Shape rules:
//   - struct -> Object (recursively); embedded struct without a json tag
//     flattens, like encoding/json
//   - slice -> List, [N]T -> List with exactly N items, []byte/[N]byte -> Bytes
//   - map[string]V -> Map: struct values via value_schema, everything else
//     via value_field
//   - honest numbers: int8/16/32 -> Int32, int/int64 -> Int64, uint8/16/32 ->
//     UInt32, uint/uint64 -> UInt64, float32 -> Float, float64 -> Double
//   - stdlib natively: time.Time -> Timestamp, time.Duration -> Duration,
//     json.RawMessage -> JSON
//   - interface -> JSON; a type cycle -> JSON (its shape is not finite);
//     any other json.Marshaler -> JSON (its wire shape is unknowable)
//   - *T -> optional AND nullable (encoding/json writes null for a nil
//     pointer); `validate:"required"` forces required back on
//
// WithType overrides bind BEFORE every default branch, stdlib included, so
// a domain type (or even time.Duration) can be re-described.

package schemapb

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ReflectOption configures Reflect.
type ReflectOption func(*reflector)

// WithType overrides how one exact Go type reflects: f receives the field
// name and returns the complete field definition. Checked before every
// default branch (stdlib types included). Tag-level description and
// `validate:"required"` still apply on top of the returned field.
func WithType(t reflect.Type, f func(name FieldName) *Schema_Field) ReflectOption {
	return func(r *reflector) {
		r.overrides[t] = f
	}
}

// Reflect builds a Schema from a Go type. Coercion is enabled on the root:
// human input arrives as strings ("1h", "256") and the schema converts
// them itself on resolve.
func Reflect(t reflect.Type, id *SchemaIdentity, opts ...ReflectOption) (*Schema, error) {
	r := &reflector{overrides: map[reflect.Type]func(FieldName) *Schema_Field{}}
	for _, opt := range opts {
		opt(r)
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	root := NewSchema(id).Coerce()

	if _, isOverride := r.overrides[t]; !isOverride && t.Kind() == reflect.Struct && t != timeType {
		fields, err := r.fieldsOf(t, map[reflect.Type]bool{})
		if err != nil {
			return nil, fmt.Errorf("schemapb: reflect %s: %w", t, err)
		}

		return root.Fields(fieldDefs(fields)...).Build()
	}

	// A non-struct payload becomes one "value" field.
	f, err := r.fieldOf("value", t, true, &fieldConstraints{}, map[reflect.Type]bool{})
	if err != nil {
		return nil, fmt.Errorf("schemapb: reflect %s: %w", t, err)
	}

	return root.Fields(f).Build()
}

// ReflectType is Reflect for a compile-time-known type.
func ReflectType[T any](id *SchemaIdentity, opts ...ReflectOption) (*Schema, error) {
	return Reflect(reflect.TypeFor[T](), id, opts...)
}

//nolint:gochecknoglobals // immutable stdlib type handles, computed once
var (
	timeType     = reflect.TypeFor[time.Time]()
	durationType = reflect.TypeFor[time.Duration]()
	rawJSONType  = reflect.TypeFor[json.RawMessage]()
	marshalerT   = reflect.TypeFor[json.Marshaler]()
)

type reflector struct {
	overrides map[reflect.Type]func(FieldName) *Schema_Field
}

func fieldDefs(fields []*Schema_Field) []FieldDef {
	out := make([]FieldDef, 0, len(fields))
	for _, f := range fields {
		out = append(out, f)
	}

	return out
}

//nolint:cyclop // one branch per encoding/json name rule
func (r *reflector) fieldsOf(t reflect.Type, visited map[reflect.Type]bool) ([]*Schema_Field, error) {
	if visited[t] {
		return nil, nil
	}

	visited[t] = true
	defer delete(visited, t)

	out := make([]*Schema_Field, 0, t.NumField())

	for i := range t.NumField() {
		structField := t.Field(i)
		// An embedded field of an unexported struct type still flattens its
		// exported fields — encoding/json semantics.
		if !structField.IsExported() && !structField.Anonymous {
			continue
		}

		name, omit, skip := jsonName(&structField)
		if skip {
			continue
		}

		if structField.Anonymous && structField.Tag.Get("json") == "" {
			// Embedded struct: fields flatten, like encoding/json.
			embedded := structField.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}

			if embedded.Kind() == reflect.Struct {
				nested, err := r.fieldsOf(embedded, visited)
				if err != nil {
					return nil, err
				}

				out = append(out, nested...)

				continue
			}
		}

		if !structField.IsExported() {
			continue // an embedded unexported non-struct contributes nothing
		}

		if !fieldNameRE.MatchString(name) || isCELReserved(name) {
			return nil, fmt.Errorf(
				"field %s: json name %q is not a valid schemapb field name (identifier, not a CEL reserved word)",
				structField.Name, name)
		}

		c := parseConstraints(structField.Tag)

		required := structField.Type.Kind() != reflect.Pointer && !omit

		field, err := r.fieldOf(name, structField.Type, required, c, visited)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", structField.Name, err)
		}

		out = append(out, field)
	}

	return out, nil
}

// fieldOf builds one field descriptor for a Go type, enriched with the tag
// constraints (c). A field on a synthetic position — a list item, a map
// value — carries no tags, so an empty c leaves the base shape untouched.
//
//nolint:cyclop // one branch per special-cased type
func (r *reflector) fieldOf(
	name string, t reflect.Type, required bool, c *fieldConstraints, visited map[reflect.Type]bool,
) (*Schema_Field, error) {
	fname := FieldName(name)
	nullable := false

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		required = false
		// encoding/json writes null for a nil pointer: the schema must
		// tolerate it.
		nullable = true
	}

	// `validate:"required"` forces a field required even when it is a pointer.
	if c.required {
		required = true
	}

	var (
		field *Schema_Field
		err   error
	)

	switch {
	case r.overrides[t] != nil:
		field = r.overrides[t](fname)
	case t == timeType:
		field = Timestamp(fname).Done()
	case t == durationType:
		field = Duration(fname).Done()
	case t == rawJSONType:
		field = JSON(fname).Done()
	case t.Kind() != reflect.Interface && (t.Implements(marshalerT) || reflect.PointerTo(t).Implements(marshalerT)):
		// A custom marshaler's wire shape is unknowable by reflection.
		field = JSON(fname).Done()
	case visited[t] && t.Kind() == reflect.Struct:
		// A cycle: the shape cannot be described finitely.
		field = JSON(fname).Done()
	default:
		field, err = r.fieldOfKind(fname, t, c, visited)
		if err != nil {
			return nil, err
		}
	}

	if c.desc != "" {
		d := c.desc
		field.Description = &d
	}

	field.Required = required
	if nullable {
		field.Nullable = true
	}

	return field, nil
}

// fieldOfKind maps a plain Go kind (no stdlib/override/cycle special cases).
//
//nolint:gocognit,cyclop,gocyclo,funlen // flat exhaustive kind dispatch
func (r *reflector) fieldOfKind(
	fname FieldName, t reflect.Type, c *fieldConstraints, visited map[reflect.Type]bool,
) (*Schema_Field, error) {
	switch t.Kind() {
	case reflect.Struct:
		fields, err := r.fieldsOf(t, visited)
		if err != nil {
			return nil, err
		}

		return Object(fname, fieldDefs(fields)...).Done(), nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return r.bytesField(fname, t, c)
		}

		item, err := r.fieldOf("item", t.Elem(), true, &fieldConstraints{}, visited)
		if err != nil {
			return nil, err
		}

		listB := List(fname, item)

		if t.Kind() == reflect.Array {
			// Fixed length is part of the type.
			n := uint64(t.Len()) //nolint:gosec // array length is non-negative
			return listB.MinItems(n).MaxItems(n).Done(), nil
		}

		if v, ok, err := c.uintOf(c.min, "min"); err != nil {
			return nil, err
		} else if ok {
			listB = listB.MinItems(v)
		}

		if v, ok, err := c.uintOf(c.max, "max"); err != nil {
			return nil, err
		} else if ok {
			listB = listB.MaxItems(v)
		}

		return listB.Done(), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key %s is not a string (JSON object keys are strings)", t.Key())
		}

		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		if elem.Kind() == reflect.Struct && r.overrides[elem] == nil &&
			elem != timeType && elem != durationType && !visited[elem] &&
			!elem.Implements(marshalerT) && !reflect.PointerTo(elem).Implements(marshalerT) {
			fields, err := r.fieldsOf(elem, visited)
			if err != nil {
				return nil, err
			}

			return Map(fname, fieldDefs(fields)...).Done(), nil
		}

		value, err := r.fieldOf("value", t.Elem(), true, &fieldConstraints{}, visited)
		if err != nil {
			return nil, err
		}

		return MapOf(fname, value).Done(), nil
	case reflect.String:
		return r.stringField(fname, c)
	case reflect.Bool:
		return Bool(fname).Done(), nil
	case reflect.Int8, reflect.Int16, reflect.Int32:
		if len(c.oneof) > 0 {
			return intChoice(fname, c)
		}

		return numField(c, Int32(fname), math.MinInt32, math.MaxInt32)
	case reflect.Int, reflect.Int64:
		if len(c.oneof) > 0 {
			return intChoice(fname, c)
		}

		return numField(c, Int64(fname), math.MinInt64, math.MaxInt64)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return unumField(c, UInt32(fname), math.MaxUint32)
	case reflect.Uint, reflect.Uint64:
		return unumField(c, UInt64(fname), math.MaxUint64)
	case reflect.Float32:
		return floatField(c, Float(fname))
	case reflect.Float64:
		return floatField(c, Double(fname))
	case reflect.Interface:
		return JSON(fname).Done(), nil
	default:
		return nil, fmt.Errorf("unsupported kind %s", t.Kind())
	}
}

func (r *reflector) bytesField(fname FieldName, t reflect.Type, c *fieldConstraints) (*Schema_Field, error) {
	b := Bytes(fname)

	if t.Kind() == reflect.Array {
		n := uint64(t.Len()) //nolint:gosec // array length is non-negative
		return b.Len(n).Done(), nil
	}

	if v, ok, err := c.uintOf(c.min, "min"); err != nil {
		return nil, err
	} else if ok {
		b = b.MinLen(v)
	}

	if v, ok, err := c.uintOf(c.max, "max"); err != nil {
		return nil, err
	} else if ok {
		b = b.MaxLen(v)
	}

	if v, ok, err := c.uintOf(c.length, "len"); err != nil {
		return nil, err
	} else if ok {
		b = b.Len(v)
	}

	return b.Done(), nil
}

func (r *reflector) stringField(fname FieldName, c *fieldConstraints) (*Schema_Field, error) {
	if len(c.oneof) > 0 {
		return Choice(fname).StrOpts(c.oneof...).Done(), nil
	}

	strB := Str(fname)

	if v, ok, err := c.uintOf(c.min, "min"); err != nil {
		return nil, err
	} else if ok {
		strB = strB.MinLen(v)
	}

	if v, ok, err := c.uintOf(c.max, "max"); err != nil {
		return nil, err
	} else if ok {
		strB = strB.MaxLen(v)
	}

	if v, ok, err := c.uintOf(c.length, "len"); err != nil {
		return nil, err
	} else if ok {
		strB = strB.Len(v)
	}

	if c.pattern != "" {
		strB = strB.Pattern(c.pattern)
	}

	if c.format != "" {
		strB = strB.Format(Format(c.format))
	}

	return strB.Done(), nil
}

// numField applies the signed numeric bound tags, range-checked for the
// honest kind width.
func numField[T int32 | int64](c *fieldConstraints, b *NumB[T], minV, maxV int64) (*Schema_Field, error) {
	for _, bound := range []struct {
		raw string
		tag string
		set func(T) *NumB[T]
	}{
		{c.min, "min", b.Gte},
		{c.gte, "gte", b.Gte},
		{c.gt, "gt", b.Gt},
		{c.max, "max", b.Lte},
		{c.lte, "lte", b.Lte},
		{c.lt, "lt", b.Lt},
	} {
		if bound.raw == "" {
			continue
		}

		v, err := strconv.ParseInt(bound.raw, 10, 64)
		if err != nil || v < minV || v > maxV {
			return nil, fmt.Errorf("validate tag %s=%q: not a valid bound for this integer width", bound.tag, bound.raw)
		}

		b = bound.set(T(v))
	}

	return b.Done(), nil
}

func unumField[T uint32 | uint64](c *fieldConstraints, b *NumB[T], maxV uint64) (*Schema_Field, error) {
	for _, bound := range []struct {
		raw string
		tag string
		set func(T) *NumB[T]
	}{
		{c.min, "min", b.Gte},
		{c.gte, "gte", b.Gte},
		{c.gt, "gt", b.Gt},
		{c.max, "max", b.Lte},
		{c.lte, "lte", b.Lte},
		{c.lt, "lt", b.Lt},
	} {
		if bound.raw == "" {
			continue
		}

		v, err := strconv.ParseUint(bound.raw, 10, 64)
		if err != nil || v > maxV {
			return nil, fmt.Errorf("validate tag %s=%q: not a valid bound for this unsigned width", bound.tag, bound.raw)
		}

		b = bound.set(T(v))
	}

	return b.Done(), nil
}

func floatField[T float32 | float64](c *fieldConstraints, b *NumB[T]) (*Schema_Field, error) {
	for _, bound := range []struct {
		raw string
		tag string
		set func(T) *NumB[T]
	}{
		{c.min, "min", b.Gte},
		{c.gte, "gte", b.Gte},
		{c.gt, "gt", b.Gt},
		{c.max, "max", b.Lte},
		{c.lte, "lte", b.Lte},
		{c.lt, "lt", b.Lt},
	} {
		if bound.raw == "" {
			continue
		}

		v, err := strconv.ParseFloat(bound.raw, 64)
		if err != nil {
			return nil, fmt.Errorf("validate tag %s=%q: not a number", bound.tag, bound.raw)
		}

		b = bound.set(T(v))
	}

	return b.Done(), nil
}

// fieldConstraints is what a field's tags say beyond its Go type: the
// `desc:"..."`/`pattern:"..."` tags and the subset of the
// go-playground/validator `validate:"..."` vocabulary the schema can carry.
type fieldConstraints struct {
	desc     string
	required bool
	oneof    []string
	pattern  string
	format   string
	// min/max/len (validator's type-dependent meaning) and the explicit
	// numeric bounds — kept as strings, parsed against the field's own type.
	min, max, length string
	gte, gt, lte, lt string
}

// uintOf parses one unsigned tag value; ("", false, nil) when absent.
func (c *fieldConstraints) uintOf(raw, tag string) (uint64, bool, error) {
	if raw == "" {
		return 0, false, nil
	}

	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("validate tag %s=%q: not an unsigned integer", tag, raw)
	}

	return v, true, nil
}

func intChoice(fname FieldName, c *fieldConstraints) (*Schema_Field, error) {
	ints, err := c.oneofInts()
	if err != nil {
		return nil, err
	}

	return Choice(fname).IntOpts(ints...).Done(), nil
}

func (c *fieldConstraints) oneofInts() ([]int64, error) {
	out := make([]int64, 0, len(c.oneof))

	for _, s := range c.oneof {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("validate tag oneof: %q is not an integer", s)
		}

		out = append(out, v)
	}

	return out, nil
}

// reflectFormats maps validator format keywords onto the spec's core
// string-format registry.
//
//nolint:gochecknoglobals // immutable keyword set
var reflectFormats = map[string]bool{
	"email": true, "url": true, "uuid": true,
	"ip": true, "ipv4": true, "ipv6": true, "hostname": true,
}

// parseConstraints reads the field's `validate`, `desc` and `pattern`
// tags. Unknown validate keys are IGNORED (the full validator vocabulary
// is wider than what a schema can carry); malformed values of known keys
// error at the use site.
func parseConstraints(tag reflect.StructTag) *fieldConstraints {
	c := &fieldConstraints{desc: tag.Get("desc"), pattern: tag.Get("pattern")}

	for _, part := range strings.Split(tag.Get("validate"), ",") {
		key, val, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch key {
		case "required":
			c.required = true
		case "oneof":
			c.oneof = strings.Fields(val)
		case "min":
			c.min = val
		case "max":
			c.max = val
		case "len":
			c.length = val
		case "gte":
			c.gte = val
		case "gt":
			c.gt = val
		case "lte":
			c.lte = val
		case "lt":
			c.lt = val
		default:
			if reflectFormats[key] {
				c.format = key
			}
		}
	}

	return c
}

// jsonName resolves the traveling name of a struct field.
func jsonName(structField *reflect.StructField) (string, bool, bool) {
	tag := structField.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}

	parts := strings.Split(tag, ",")

	name := parts[0]
	if name == "" {
		name = structField.Name
	}

	omitempty := false

	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}

	return name, omitempty, false
}
