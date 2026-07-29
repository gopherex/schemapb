package schemapb

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// Bake validates and resolves values, then seals them with the schema into a
// Baked snapshot (canonical wire values). On a blocking (ERROR) failure baked
// is nil; warnings do not block and are returned alongside the Baked. The
// returned error is programmatic only (schema does not compile, value cannot
// be canonicalised).
func (s *Schema) Bake(values map[string]any) (*Baked, *ValidationResult, error) {
	e, err := s.engine()
	if err != nil {
		return nil, nil, err
	}
	return e.Bake(values)
}

// Bake is the compiled-engine form of (*Schema).Bake.
func (e *Engine) Bake(values map[string]any) (*Baked, *ValidationResult, error) {
	res := e.Validate(values) // resolves values in place + checks
	if res.Blocking() {
		return nil, res, nil
	}
	st, err := e.canonicalStruct(values)
	if err != nil {
		return nil, res, fmt.Errorf("schemapb: bake: %w", err)
	}
	return &Baked{Schema: e.schema, Values: st}, res, nil
}

// canonicalStruct projects a resolved native form into the contract's
// canonical wire variants, keyed by the schema's declared fields (unknown
// keys fall back to best-fit conversion).
func (e *Engine) canonicalStruct(values map[string]any) (*StructValue, error) {
	fields := make(map[string]*Value, len(values))
	for name, val := range values {
		f := findField(e.schema.GetFields(), name)
		var v *Value
		var err error
		switch {
		case f == nil:
			v, err = FromGo(val)
		case f.GetRef() != nil:
			if def := e.schema.GetDefs()[refDefKey(f.GetRef())]; def != nil {
				if m, ok := val.(map[string]any); ok {
					v, err = canonicalStruct(def, m, name)
					break
				}
			}
			v, err = FromGo(val)
		case f.GetOneOf() != nil:
			if variant, m := selectVariant(f.GetOneOf(), val); variant != nil {
				v, err = canonicalStruct(variant, m, name)
				break
			}
			v, err = FromGo(val)
		default:
			v, err = CanonicalValue(f, val)
		}
		if err != nil {
			return nil, err
		}
		fields[name] = v
	}
	return &StructValue{Fields: fields}, nil
}

// Merge layers overrides onto a baked form and re-seals against the same
// schema. Nested objects merge recursively; lists append unless replaceLists;
// immutable fields keep their baked values (a changed immutable is rejected).
// It compiles the embedded schema with default options; a schema needing
// custom compile options (format extensions) must merge through its own
// engine — see (*Engine).Merge.
func (b *Baked) Merge(overrides *StructValue, replaceLists bool) (*Baked, *ValidationResult, error) {
	base := b.GetValues().ToGo()
	ov := overrides.ToGo()
	return b.GetSchema().Bake(mergeMaps(base, ov, replaceLists))
}

// Merge is (*Baked).Merge evaluated on this engine (keeping its compile
// options, e.g. WithFormats extensions).
func (e *Engine) Merge(b *Baked, overrides *StructValue, replaceLists bool) (*Baked, *ValidationResult, error) {
	base := b.GetValues().ToGo()
	ov := overrides.ToGo()
	return e.Bake(mergeMaps(base, ov, replaceLists))
}

// Matches reports whether the baked schema is identical in content to s.
func (b *Baked) Matches(s *Schema) bool {
	return proto.Equal(b.GetSchema(), s)
}

// Bake validates and seals an inline Filled into a Baked. It works only for a
// Filled carrying an inline schema; a Filled that references a schema by id
// must be baked where a registry can resolve the id.
func (f *Filled) Bake() (*Baked, *ValidationResult, error) {
	s := f.GetSchema().GetSchema()
	if s == nil {
		return nil, nil, errors.New("schemapb: Filled.Bake requires an inline schema (id refs resolve via a registry)")
	}
	return s.Bake(f.GetValues().ToGo())
}

// mergeMaps deep-merges src over dst (objects recurse; lists append unless
// replaceLists; scalars overwrite). It returns a new map.
func mergeMaps(dst, src map[string]any, replaceLists bool) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, sv := range src {
		if dv, ok := out[k]; ok {
			if dm, ok1 := dv.(map[string]any); ok1 {
				if sm, ok2 := sv.(map[string]any); ok2 {
					out[k] = mergeMaps(dm, sm, replaceLists)
					continue
				}
			}
			if !replaceLists {
				if dl, ok1 := dv.([]any); ok1 {
					if sl, ok2 := sv.([]any); ok2 {
						out[k] = append(append([]any{}, dl...), sl...)
						continue
					}
				}
			}
		}
		out[k] = sv
	}
	return out
}
