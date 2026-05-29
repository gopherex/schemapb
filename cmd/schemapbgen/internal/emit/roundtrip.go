package emit

import (
	"bytes"
	"fmt"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeRoundtrip(b *bytes.Buffer, f *model.File) {
	root := f.Root

	fmt.Fprintf(b, "// ToValues marshals the value to a protobuf Struct (JSON-bridged).\n")
	fmt.Fprintf(b, "func (c *%s) ToValues() (*structpb.Struct, error) {\n", root)
	fmt.Fprintf(b, "\tj, err := json.Marshal(c)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\tst := &structpb.Struct{}\n\tif err := st.UnmarshalJSON(j); err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn st, nil\n}\n\n")

	fmt.Fprintf(b, "// FromValues%s decodes a protobuf Struct into a new value.\n", root)
	fmt.Fprintf(b, "func FromValues%s(st *structpb.Struct) (*%s, error) {\n", root, root)
	fmt.Fprintf(b, "\tj, err := st.MarshalJSON()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\tvar c %s\n\tif err := json.Unmarshal(j, &c); err != nil {\n\t\treturn nil, err\n\t}\n", root)
	fmt.Fprintf(b, "\treturn &c, nil\n}\n\n")

	fmt.Fprintf(b, "// ToFilled wraps the value as a schemapb.Filled against this schema.\n")
	fmt.Fprintf(b, "func (c *%s) ToFilled() (*schemapb.Filled, error) {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn &schemapb.Filled{Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: _schema%s()}}, Values: st}, nil\n}\n\n", root)

	fmt.Fprintf(b, "// ToBaked wraps the value as a schemapb.Baked (embedded schema + values).\n")
	fmt.Fprintf(b, "func (c *%s) ToBaked() (*schemapb.Baked, error) {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn &schemapb.Baked{Schema: _schema%s(), Values: st}, nil\n}\n\n", root)
}
