package schemapb

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// This file holds schema composition: embedding an already-built *Schema into
// another (by value, via ObjectOf/VariantOf/DefSchema) and the $defs hoisting
// that makes inline composition self-contained. Identity-based composition
// (RefID + Link against a Registry) lives in registry.go.
//
// Two composition models:
//
//   - By value (inline): the embedded schema's fields are cloned into the
//     host. Its identity is not used by the engine (validation is
//     structural). Its own $defs are hoisted into the root at Build time so
//     internal Refs resolve.
//
//   - By identity (reference): a Ref carries the target's SchemaIdentity. The
//     identity is preserved on the node (renderers can resolve/link it).
//     Before validation the referenced schema must be present in the root
//     defs under its identity key — Link(resolver) pulls it in transitively.

// identityKey returns the root-defs key for a schema identity. The parts are
// joined with NUL bytes, which a local def name never contains, so identity
// keys and local def names never collide.
func identityKey(id *SchemaIdentity) string {
	return id.GetNamespace() + "\x00" + id.GetName() + "\x00" + id.GetVersion()
}

// identityString renders an identity for humans: "namespace/name@version".
func identityString(id *SchemaIdentity) string {
	out := id.GetName()
	if ns := id.GetNamespace(); ns != "" {
		out = ns + "/" + out
	}
	if v := id.GetVersion(); v != "" {
		out += "@" + v
	}
	return out
}

// refDefKey returns the root-defs lookup key for a Ref: the local def name, or
// the identity key for an id-ref.
func refDefKey(ref *Schema_Field_Ref) string {
	if id := ref.GetId(); id != nil {
		return identityKey(id)
	}
	return ref.GetName()
}

// --- inline composition builders --------------------------------------------

// ObjectOf embeds an already-built schema as a nested object field. The schema
// is cloned (the source is never aliased); its own $defs are hoisted into the
// root at Build time so any internal Refs resolve. The embedded schema's
// identity is informational only — the engine validates by structure. Use
// RefID when you need an identity-preserving reference instead of a value
// copy.
func ObjectOf(name string, s *Schema) *ObjectB {
	b := &ObjectB{k: &Schema_Field_Object{Schema: proto.Clone(s).(*Schema)}}
	b.fieldBase = newField(name, b)
	b.f.Kind = &Schema_Field_Object_{Object: b.k}
	return b
}

// VariantOf adds an already-built schema as a oneof variant (cloned).
func (b *OneOfB) VariantOf(key string, s *Schema) *OneOfB {
	b.k.Variants[key] = proto.Clone(s).(*Schema)
	return b
}

// DefSchema registers an already-built schema as a named def (cloned).
// Combine with Ref(field, name) to reference it. The schema's own $defs are
// hoisted into the root at Build time so its internal Refs resolve.
func (b *SchemaB) DefSchema(name string, s *Schema) *SchemaB {
	if b.s.Defs == nil {
		b.s.Defs = map[string]*Schema{}
	}
	b.s.Defs[name] = proto.Clone(s).(*Schema)
	return b
}

// --- defs hoisting (inline composition) --------------------------------------

// addDef inserts def under key into root.Defs, erroring if a different schema
// is already registered under that key (same content is idempotent).
func addDef(root *Schema, key string, def *Schema) error {
	if existing, ok := root.Defs[key]; ok {
		if !proto.Equal(existing, def) {
			return fmt.Errorf("schemapb: conflicting $defs key %q during schema composition", key)
		}
		return nil
	}
	root.Defs[key] = def
	return nil
}

// nestedSchemas returns the schemas directly embedded in a field: the object
// schema, oneof variants, the map value schema, and — recursively — list item
// schemas. Refs are not embedded.
func nestedSchemas(f *Schema_Field) []*Schema {
	var out []*Schema
	if o := f.GetObject(); o != nil && o.GetSchema() != nil {
		out = append(out, o.GetSchema())
	}
	if oo := f.GetOneOf(); oo != nil {
		for _, v := range oo.GetVariants() {
			out = append(out, v)
		}
	}
	if mp := f.GetMap(); mp != nil && mp.GetValueSchema() != nil {
		out = append(out, mp.GetValueSchema())
	}
	if l := f.GetList(); l != nil {
		for _, it := range l.GetItems() {
			out = append(out, nestedSchemas(it)...)
		}
	}
	return out
}

// hoistDefs lifts every embedded schema's $defs into the root defs map, so Ref
// resolution (which is root-only) sees defs declared inside embedded schemas.
// Root keeps its own defs. A key present with differing content is a conflict.
func hoistDefs(root *Schema) error {
	if root.Defs == nil {
		root.Defs = map[string]*Schema{}
	}
	// liftFrom moves the $defs of every schema embedded under fields into
	// root and recurses into those schemas.
	var liftFrom func(fields []*Schema_Field) error
	liftFrom = func(fields []*Schema_Field) error {
		for _, f := range fields {
			for _, child := range nestedSchemas(f) {
				for k, d := range child.GetDefs() {
					if err := addDef(root, k, d); err != nil {
						return err
					}
				}
				child.Defs = nil
				if err := liftFrom(child.GetFields()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := liftFrom(root.GetFields()); err != nil {
		return err
	}
	// Root defs may themselves carry nested defs / embedded schemas; process
	// to a fixpoint since addDef can grow root.Defs.
	seen := map[string]bool{}
	for {
		grew := false
		for name := range root.Defs {
			if seen[name] {
				continue
			}
			seen[name] = true
			grew = true
			def := root.Defs[name]
			for k, d := range def.GetDefs() {
				if err := addDef(root, k, d); err != nil {
					return err
				}
			}
			def.Defs = nil
			if err := liftFrom(def.GetFields()); err != nil {
				return err
			}
		}
		if !grew {
			break
		}
	}
	return nil
}
