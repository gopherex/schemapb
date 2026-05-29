package model

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/schemapb/schemapb"
)

// Build walks a schema into a File. It returns an error on a name collision
// (two generated types resolving to the same Go identifier).
func Build(s *schemapb.Schema, pkg string) (*File, error) {
	wire, err := proto.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	b := &builder{
		file: &File{Package: pkg, Root: RootName(s.GetId()), Identity: s.GetId(), Wire: wire},
		seen: map[string]bool{},
		defs: s.GetDefs(),
	}
	b.walkSchema(s, b.file.Root, s.GetDescription())
	if b.err != nil {
		return nil, b.err
	}
	return b.file, nil
}

type builder struct {
	file *File
	seen map[string]bool // type names already declared
	defs map[string]*schemapb.Schema
	err  error
}

// walkSchema emits a Type named `name` for the schema's fields and recurses.
func (b *builder) walkSchema(s *schemapb.Schema, name, doc string) {
	if b.err != nil {
		return
	}
	if b.seen[name] {
		b.err = fmt.Errorf("type name collision: %q generated twice", name)
		return
	}
	b.seen[name] = true
	t := &Type{Name: name, Doc: doc}
	b.file.Types = append(b.file.Types, t)
	for _, f := range s.GetFields() {
		fld := b.field(f, name)
		if b.err != nil {
			return
		}
		t.Fields = append(t.Fields, fld)
	}
}

// field builds one Field and recurses into nested kinds.
func (b *builder) field(f *schemapb.Schema_Filed, parent string) *Field {
	fld := &Field{Name: pascal(f.GetName()), JSONName: f.GetName(), Doc: fieldDoc(f)}
	ptr := !f.GetRequired() || f.GetNullable()
	switch k := f.GetKind().(type) {

	case *schemapb.Schema_Filed_Object_:
		child := Child(parent, f.GetName())
		b.walkSchema(k.Object.GetSchema(), child, f.GetDescription())
		fld.GoType, fld.Pointer, fld.OmitEmpty = "*"+child, true, true

	case *schemapb.Schema_Filed_Enum_:
		name := Child(parent, f.GetName())
		b.file.Enums = append(b.file.Enums, &EnumDef{Name: name, Values: k.Enum.GetValues()})
		fld.GoType = name
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_Duration_:
		fld.GoType = "time.Duration"
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_Timestamp_:
		fld.GoType = "time.Time"
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_List_:
		elem, elemOneOf := b.listElem(k.List, parent, f.GetName())
		fld.GoType = "[]" + elem
		fld.ListElemOneOf = elemOneOf
		fld.OmitEmpty = true // slices already nil-able; no extra pointer

	case *schemapb.Schema_Filed_Ref_:
		fld.GoType = "*" + b.refType(k.Ref, parent)
		fld.Pointer, fld.OmitEmpty = true, true

	case *schemapb.Schema_Filed_Computed_:
		fld.Computed, fld.OmitEmpty = true, true
		fld.GoType = computedGoType(k.Computed.GetResult())
		if !f.GetRequired() {
			fld.GoType, fld.Pointer = "*"+fld.GoType, true
		}

	case *schemapb.Schema_Filed_OneOf_:
		iface := Child(parent, f.GetName())
		od := &OneOfDef{IfaceName: iface, Discriminator: k.OneOf.GetDiscriminator(), Variants: map[string]string{}}
		// stable order: sort variant keys
		for _, key := range sortedKeys(k.OneOf.GetVariants()) {
			vname := Child(iface, key)
			b.walkSchema(k.OneOf.GetVariants()[key], vname, "")
			od.Variants[key] = vname
		}
		fld.OneOf, fld.GoType = od, iface

	default:
		fld.GoType = goScalar(f)
		b.applyPtr(fld, ptr)
	}
	return fld
}

// applyPtr wraps a value type in a pointer when the field is optional/nullable.
func (b *builder) applyPtr(fld *Field, ptr bool) {
	if ptr {
		fld.GoType, fld.Pointer, fld.OmitEmpty = "*"+fld.GoType, true, true
	}
}

// listElem maps a list's element type from items[0] ONLY. The engine
// (validate.go checkList) validates every array element against items[0] and
// ignores items[1:], so the generated element type mirrors that single
// definition. An Object item becomes a generated <Parent>_<Field>Item struct;
// an Enum item a generated enum type; a Ref item the def type; scalars/
// duration/timestamp their direct Go type.
func (b *builder) listElem(l *schemapb.Schema_Filed_List, parent, field string) (string, *OneOfDef) {
	items := l.GetItems()
	if len(items) == 0 {
		return "any", nil
	}
	item := items[0]
	elemName := parent + "_" + pascal(field) + "Item"
	switch k := item.GetKind().(type) {
	case *schemapb.Schema_Filed_Object_:
		b.walkSchema(k.Object.GetSchema(), elemName, item.GetDescription())
		return elemName, nil
	case *schemapb.Schema_Filed_Enum_:
		b.file.Enums = append(b.file.Enums, &EnumDef{Name: elemName, Values: k.Enum.GetValues()})
		return elemName, nil
	case *schemapb.Schema_Filed_Ref_:
		return b.refType(k.Ref, parent), nil
	case *schemapb.Schema_Filed_Duration_:
		return "time.Duration", nil
	case *schemapb.Schema_Filed_Timestamp_:
		return "time.Time", nil
	case *schemapb.Schema_Filed_OneOf_:
		// The element is a discriminated union; elemName is the interface type.
		od := &OneOfDef{IfaceName: elemName, Discriminator: k.OneOf.GetDiscriminator(), Variants: map[string]string{}}
		for _, key := range sortedKeys(k.OneOf.GetVariants()) {
			vname := Child(elemName, key)
			b.walkSchema(k.OneOf.GetVariants()[key], vname, "")
			od.Variants[key] = vname
		}
		return elemName, od
	default:
		return goScalar(item), nil
	}
}

// refType resolves a Ref to its generated def type name (local defs only here;
// id-refs are resolved to the def key by the engine's Link — Phase 1 supports
// name-target refs, which is what the builder emits).
func (b *builder) refType(r *schemapb.Schema_Filed_Ref, parent string) string {
	name := r.GetName()
	if name == "" {
		b.err = fmt.Errorf("id-target Ref not supported in Phase 1; use a named def")
		return "any"
	}
	defName := b.file.Root + "_" + pascal(name)
	if !b.seen[defName] {
		b.walkSchema(b.defs[name], defName, "")
	}
	return defName
}

func computedGoType(rt schemapb.Schema_Filed_ResultType) string {
	switch rt {
	case schemapb.ResultDouble:
		return "float64"
	case schemapb.ResultInt64:
		return "int64"
	case schemapb.ResultUint64:
		return "uint64"
	case schemapb.ResultBool:
		return "bool"
	case schemapb.ResultString:
		return "string"
	case schemapb.ResultDuration:
		return "time.Duration"
	default:
		return "any"
	}
}

func sortedKeys(m map[string]*schemapb.Schema) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// goScalar maps the simple scalar kinds. Non-scalar kinds return "any".
func goScalar(f *schemapb.Schema_Filed) string {
	switch f.GetKind().(type) {
	case *schemapb.Schema_Filed_Float_:
		return "float32"
	case *schemapb.Schema_Filed_Double_:
		return "float64"
	case *schemapb.Schema_Filed_Int32_:
		return "int32"
	case *schemapb.Schema_Filed_Int64_:
		return "int64"
	case *schemapb.Schema_Filed_Uint32:
		return "uint32"
	case *schemapb.Schema_Filed_Uint64:
		return "uint64"
	case *schemapb.Schema_Filed_Bool_:
		return "bool"
	case *schemapb.Schema_Filed_String_:
		return "string"
	default:
		return "any"
	}
}

// fieldDoc assembles the doc comment: description + dynamic-logic markers.
func fieldDoc(f *schemapb.Schema_Filed) string {
	var lines []string
	if d := f.GetDescription(); d != "" {
		lines = append(lines, d)
	}
	if w := f.GetWhen(); w != "" {
		lines = append(lines, "when: "+w)
	}
	if n := f.GetNormalize(); n != "" {
		lines = append(lines, "normalize: "+n)
	}
	for _, r := range f.GetRules() {
		lines = append(lines, "rule: "+r.GetExpr())
	}
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
