package schemapb

import (
	"fmt"
	"reflect"
)

// SchemaWrapper describes a transparent Go wrapper. SchemaInner returns its
// immediate inner type (not the type's storage fields). Reflect calls this on
// a fresh zero receiver; it must not depend on instance state or return nil.
// Both value and pointer receiver methods are recognized.
type SchemaWrapper interface {
	SchemaInner() reflect.Type
}

// SchemaFieldConfigurer decorates a type's field after ordinary field tags
// and WithFieldTags. For nested wrappers, decorators run inside out. The
// descriptor is private to this Reflect invocation. WithType overrides skip
// that type's hooks. Implementations may set metadata or enforce invariants;
// they must not retain the descriptor or perform external side effects.
type SchemaFieldConfigurer interface {
	SchemaField(field *Schema_Field) error
}

// SchemaDecodeTarget lets Decode fill a wrapper using the ordinary typed
// decoder, without a JSON conversion. The receiver must also implement
// SchemaWrapper. Return a non-nil pointer to storage of exactly SchemaInner's
// type, owned by this fresh receiver (e.g. &w.value or an allocated private
// cell). Do not return storage belonging to an existing live instance or
// publish updates: Decode commits the new destination only after success.
// For an inner *T, return **T. Pointer receivers are required for mutations.
type SchemaDecodeTarget interface {
	SchemaDecodeTarget() any
}

func hasSchemaHooks(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	ptr := reflect.PointerTo(t)

	return ptr.Implements(reflect.TypeFor[SchemaWrapper]()) || ptr.Implements(reflect.TypeFor[SchemaFieldConfigurer]())
}

type reflectedType struct {
	typ         reflect.Type
	required    bool
	nullable    bool
	configurers []SchemaFieldConfigurer // outermost first
}

// unwrapType checks only the transparent wrapper chain, independently of the
// existing recursive-struct fallback. Ordinary recursive objects remain JSON.
func (r *reflector) unwrapType(t reflect.Type, required bool) (reflectedType, error) {
	info := reflectedType{required: required}
	seen := map[reflect.Type]bool{}

	for {
		for t.Kind() == reflect.Pointer {
			if seen[t] || len(seen) >= 256 {
				return info, fmt.Errorf("cyclic or excessively nested schema pointer %s", t)
			}

			seen[t] = true
			t = t.Elem()
			info.required, info.nullable = false, true
		}

		info.typ = t
		if r.overrides[t] != nil || !hasSchemaHooks(t) {
			return info, nil
		}

		if seen[t] || len(seen) >= 256 {
			return info, fmt.Errorf("cyclic or excessively nested schema wrapper %s", t)
		}

		seen[t] = true

		receiver := reflect.New(t).Interface()
		if configurer, ok := receiver.(SchemaFieldConfigurer); ok {
			info.configurers = append(info.configurers, configurer)
		}

		wrapper, ok := receiver.(SchemaWrapper)
		if !ok {
			return info, nil
		}

		t = wrapper.SchemaInner()
		if t == nil {
			return info, fmt.Errorf("schema wrapper %s returned nil inner type", info.typ)
		}
	}
}

func (info reflectedType) configure(field *Schema_Field) error {
	for i := len(info.configurers) - 1; i >= 0; i-- {
		if err := info.configurers[i].SchemaField(field); err != nil {
			return fmt.Errorf("schema field configuration: %w", err)
		}
	}

	return nil
}

func decodeWrapper(v *Value, dst reflect.Value, path string, depth int) (bool, error) {
	if dst.Kind() == reflect.Pointer || !dst.CanAddr() || !dst.Addr().CanInterface() {
		return false, nil
	}

	receiver := dst.Addr().Interface()

	hook, ok := receiver.(SchemaDecodeTarget)
	if !ok {
		return false, nil
	}

	wrapper, ok := receiver.(SchemaWrapper)
	if !ok {
		return true, decodeFailure(v, dst, path, "SchemaDecodeTarget requires SchemaWrapper")
	}

	inner := wrapper.SchemaInner()
	if inner == nil {
		return true, decodeFailure(v, dst, path, "schema wrapper returned nil inner type")
	}

	if inner == dst.Type() {
		return true, decodeFailure(v, dst, path, "schema wrapper refers to itself")
	}

	target := reflect.ValueOf(hook.SchemaDecodeTarget())
	if !target.IsValid() || target.Kind() != reflect.Pointer || target.IsNil() {
		return true, decodeFailure(v, dst, path, "schema decode target must be a non-nil pointer")
	}

	if target.Type().Elem() != inner {
		return true, decodeFailure(v, dst, path, "schema decode target does not match inner type")
	}

	return true, decodeValue(v, target.Elem(), path, depth+1)
}
