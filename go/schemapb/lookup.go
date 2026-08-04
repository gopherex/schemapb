// Schema path lookup: resolving a dot path ("a.b.c") to the field it
// addresses. Paths address FIELDS, not values, so they carry no list
// indices or map keys: lookup descends through Object fields and resolves
// Refs against root $defs, and every other kind is terminal — a path may
// END on a list/map/oneof field, but cannot continue through one (there is
// no index/key/discriminator in a schema path to pick a branch with).
//
// Failures are typed and point at the exact segment that broke
// ("no field b in a", never "a.b.c not found"), so a schema author sees
// where the path diverged from the schema instead of diffing it by eye.

package schemapb

import (
	"fmt"
	"strings"
)

// LookupReason classifies why a schema path failed to resolve. The values
// are stable spec strings shared by all implementations (conformance).
type LookupReason string

const (
	// LookupEmptyPath - the path has no segments.
	LookupEmptyPath LookupReason = "empty_path"
	// LookupNotFound - the segment names no field at its level.
	LookupNotFound LookupReason = "not_found"
	// LookupNotTraversable - the path continues through a field kind that
	// cannot be descended into (list, map, scalar, computed, ...).
	LookupNotTraversable LookupReason = "not_traversable"
	// LookupAmbiguousOneOf - the path continues through a oneof field;
	// without a discriminator value the variant is unknowable.
	LookupAmbiguousOneOf LookupReason = "ambiguous_oneof"
	// LookupUnknownRef - the path continues through a Ref whose target def
	// does not exist in the root schema.
	LookupUnknownRef LookupReason = "unknown_ref"
)

// LookupError pinpoints the failing segment of a schema path: At is the
// resolved parent path ("" for root), Segment the name that failed, Kind
// the kind of the offending field (set for the traversal reasons).
type LookupError struct {
	At      string
	Segment FieldName
	Reason  LookupReason
	Kind    FieldKind
}

func (e *LookupError) Error() string {
	where := "root"
	if e.At != "" {
		where = fmt.Sprintf("%q", e.At)
	}

	switch e.Reason {
	case LookupEmptyPath:
		return "schemapb: lookup: empty path"
	case LookupNotFound:
		return fmt.Sprintf("schemapb: lookup: no field %q in %s", e.Segment, where)
	case LookupAmbiguousOneOf:
		return fmt.Sprintf(
			"schemapb: lookup: cannot descend into oneof %q in %s: the variant depends on a discriminator value",
			e.Segment, where)
	case LookupUnknownRef:
		return fmt.Sprintf("schemapb: lookup: ref %q in %s points to a def that does not exist", e.Segment, where)
	default: // LookupNotTraversable
		return fmt.Sprintf("schemapb: lookup: cannot descend into %q in %s (kind %s)", e.Segment, where, e.Kind)
	}
}

// Lookup resolves a field path within the schema, one segment per field
// name. It returns the addressed field, or a *LookupError naming the exact
// segment that failed.
func (s *Schema) Lookup(segments ...FieldName) (*Schema_Field, error) {
	if len(segments) == 0 {
		return nil, &LookupError{Reason: LookupEmptyPath}
	}

	cur, parent := s, ""

	for i, seg := range segments {
		var f *Schema_Field

		for _, c := range cur.GetFields() {
			if c.GetName() == string(seg) {
				f = c

				break
			}
		}

		if f == nil {
			return nil, &LookupError{At: parent, Segment: seg, Reason: LookupNotFound}
		}

		if i == len(segments)-1 {
			return f, nil
		}

		switch k := f.GetKind().(type) {
		case *Schema_Field_Object_:
			cur = k.Object.GetSchema()
		case *Schema_Field_Ref_:
			def := s.GetDefs()[refDefKey(k.Ref)]
			if def == nil {
				return nil, &LookupError{At: parent, Segment: seg, Reason: LookupUnknownRef, Kind: KindRef}
			}

			cur = def
		case *Schema_Field_OneOf_:
			return nil, &LookupError{At: parent, Segment: seg, Reason: LookupAmbiguousOneOf, Kind: KindOneOf}
		default:
			return nil, &LookupError{At: parent, Segment: seg, Reason: LookupNotTraversable, Kind: KindName(f)}
		}

		parent = joinPath(parent, string(seg))
	}

	panic("unreachable")
}

// LookupPath is Lookup over a dot-separated path ("a.b.c"). Field names are
// identifiers (enforced by descriptor validation), so the dot is never part
// of a name.
func (s *Schema) LookupPath(path string) (*Schema_Field, error) {
	if path == "" {
		return nil, &LookupError{Reason: LookupEmptyPath}
	}

	parts := strings.Split(path, ".")
	segs := make([]FieldName, len(parts))

	for i, p := range parts {
		segs[i] = FieldName(p)
	}

	return s.Lookup(segs...)
}

// FieldKind is the spec's short kind name of a field — the same strings the
// render context exposes ("string", "list", "oneof", ...).
type FieldKind string

const (
	KindUnspecified FieldKind = ""
	KindFloat       FieldKind = "float"
	KindDouble      FieldKind = "double"
	KindInt32       FieldKind = "int32"
	KindInt64       FieldKind = "int64"
	KindUInt32      FieldKind = "uint32"
	KindUInt64      FieldKind = "uint64"
	KindBool        FieldKind = "bool"
	KindString      FieldKind = "string"
	KindBytes       FieldKind = "bytes"
	KindChoice      FieldKind = "choice"
	KindDuration    FieldKind = "duration"
	KindTimestamp   FieldKind = "timestamp"
	KindList        FieldKind = "list"
	KindObject      FieldKind = "object"
	KindMap         FieldKind = "map"
	KindOneOf       FieldKind = "oneof"
	KindRef         FieldKind = "ref"
	KindComputed    FieldKind = "computed"
	KindJSON        FieldKind = "json"
)

// KindName reports the field's kind as its spec short name.
func KindName(f *Schema_Field) FieldKind {
	return FieldKind(kindName(f))
}

// ListItemDef returns the item definition governing element i of a list
// field: homogeneous lists have one definition for every element, tuple
// lists one per position (nil out of range).
func ListItemDef(l *Schema_Field_List, i int) *Schema_Field {
	return listItemDef(l, i)
}
