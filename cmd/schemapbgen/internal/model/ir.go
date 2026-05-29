package model

import "github.com/stroppy-io/schemapb/schemapb"

// File is everything generated for one input schema.
type File struct {
	Package  string
	Root     string                   // root Go type name, e.g. InfraDiskV1
	Identity *schemapb.SchemaIdentity // for the Identity constant
	Wire     []byte                   // proto.Marshal of the original schema
	Types    []*Type                  // root + all nested structs, in declaration order
	Enums    []*EnumDef
}

// Type is one generated struct (root, nested object, oneof variant, or def).
type Type struct {
	Name   string // full Go name, e.g. InfraDiskV1_Wal
	Doc    string // schema description, if any
	Fields []*Field
	// OneOf, if non-nil, means this Type is the parent holding a oneof field;
	// variants are separate Types whose IfaceName == OneOf.IfaceName.
}

// Field is one struct field.
type Field struct {
	Name      string // Go field name (PascalCase)
	JSONName  string // original schema field name
	GoType    string // rendered Go type, e.g. "int64", "*InfraDiskV1_Wal", "[]string"
	Pointer   bool   // emitted as *T
	OmitEmpty bool
	Doc       string    // includes // when:/ // rule:/ // computed: lines
	Computed  bool      // omitted on ToValues
	OneOf     *OneOfDef // non-nil if this field is a discriminated union
	// ListElemOneOf is non-nil when this field is a list whose element is a
	// discriminated union (GoType == "[]<IfaceName>"). It drives both the
	// element interface/variant declarations and the per-element JSON codec.
	ListElemOneOf *OneOfDef
}

// OneOfDef describes a oneof field.
type OneOfDef struct {
	IfaceName     string            // e.g. InfraDiskV1_Storage
	Discriminator string            // value key selecting the variant
	Variants      map[string]string // discriminator value -> variant Go type name
}

// EnumDef is a generated enum type.
type EnumDef struct {
	Name   string           // e.g. InfraDiskV1_WalLevel
	Values map[int32]string // value -> label (from schema)
}

// Kind classifies a field's schema kind for type mapping.
type Kind int

const (
	KindScalar Kind = iota
	KindEnum
	KindDuration
	KindTimestamp
	KindList
	KindObject
	KindOneOf
	KindRef
	KindComputed
)
