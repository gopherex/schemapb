package emit

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeSugar(b *bytes.Buffer, f *model.File) {
	for _, t := range f.Types {
		writeGetters(b, t)
		writeBuilder(b, t)
		writeClone(b, t)
	}
	writeDefault(b, f)
}

// writeGetters emits nil-safe getters protobuf-style. For pointer fields the
// getter dereferences and returns the zero value when nil.
func writeGetters(b *bytes.Buffer, t *model.Type) {
	for _, fld := range t.Fields {
		if fld.OneOf != nil {
			continue // oneof read via type switch
		}
		base := strings.TrimPrefix(fld.GoType, "*")
		fmt.Fprintf(b, "func (c *%s) Get%s() %s {\n", t.Name, fld.Name, base)
		if fld.Pointer || strings.HasPrefix(fld.GoType, "[]") {
			fmt.Fprintf(b, "\tif c == nil || c.%s == nil {\n\t\tvar zero %s\n\t\treturn zero\n\t}\n", fld.Name, base)
			if fld.Pointer {
				fmt.Fprintf(b, "\treturn *c.%s\n}\n\n", fld.Name)
			} else {
				fmt.Fprintf(b, "\treturn c.%s\n}\n\n", fld.Name) // slice: return as-is
			}
		} else {
			fmt.Fprintf(b, "\tif c == nil {\n\t\tvar zero %s\n\t\treturn zero\n\t}\n\treturn c.%s\n}\n\n", base, fld.Name)
		}
	}
}

func writeBuilder(b *bytes.Buffer, t *model.Type) {
	fmt.Fprintf(b, "// New%s returns an empty %s for chained construction.\n", t.Name, t.Name)
	fmt.Fprintf(b, "func New%s() *%s { return &%s{} }\n\n", t.Name, t.Name, t.Name)
	for _, fld := range t.Fields {
		if fld.OneOf != nil {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = v; return c }\n\n",
				t.Name, fld.Name, fld.GoType, t.Name, fld.Name)
			continue
		}
		base := strings.TrimPrefix(fld.GoType, "*")
		if fld.Pointer {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = &v; return c }\n\n",
				t.Name, fld.Name, base, t.Name, fld.Name)
		} else {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = v; return c }\n\n",
				t.Name, fld.Name, fld.GoType, t.Name, fld.Name)
		}
	}
}

// writeClone emits a JSON-bridge deep clone (correct for all generated shapes).
func writeClone(b *bytes.Buffer, t *model.Type) {
	fmt.Fprintf(b, "// Clone deep-copies the value via its JSON representation.\n")
	fmt.Fprintf(b, "func (c *%s) Clone() *%s {\n", t.Name, t.Name)
	fmt.Fprintf(b, "\tif c == nil {\n\t\treturn nil\n\t}\n")
	fmt.Fprintf(b, "\tj, _ := json.Marshal(c)\n\tvar out %s\n\t_ = json.Unmarshal(j, &out)\n\treturn &out\n}\n\n", t.Name)
}

// writeDefault emits Default<Root>() seeding schema defaults via the engine.
// It builds an empty struct, marshals to values, lets the engine apply defaults
// through Compute, then reads back.
func writeDefault(b *bytes.Buffer, f *model.File) {
	root := f.Root
	fmt.Fprintf(b, "// Default%s returns a new value with schema defaults applied.\n", root)
	fmt.Fprintf(b, "func Default%s() *%s {\n", root, root)
	fmt.Fprintf(b, "\tc := &%s{}\n\tc.Default()\n\treturn c\n}\n\n", root)
	fmt.Fprintf(b, "// Default fills schema default values into empty fields of c.\n")
	fmt.Fprintf(b, "func (c *%s) Default() {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(b, "\tm := st.AsMap()\n\t_schema%s().ApplyDefaults(m)\n", root)
	fmt.Fprintf(b, "\tns, err := structpb.NewStruct(m)\n\tif err != nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(b, "\tif got, err := FromValues%s(ns); err == nil {\n\t\t*c = *got\n\t}\n}\n\n", root)
}
