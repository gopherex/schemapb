package emit

import (
	"bytes"
	"fmt"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeSchemaWrap(b *bytes.Buffer, f *model.File) {
	root := f.Root

	// wire bytes
	fmt.Fprintf(b, "var _schema%sWire = []byte{", root)
	for i, by := range f.Wire {
		if i%16 == 0 {
			fmt.Fprintf(b, "\n\t")
		}
		fmt.Fprintf(b, "0x%02x, ", by)
	}
	fmt.Fprintf(b, "\n}\n\n")

	// lazy decode
	fmt.Fprintf(b, "var (\n\t_schema%sOnce sync.Once\n\t_schema%sVal  *schemapb.Schema\n)\n\n", root, root)
	fmt.Fprintf(b, "func _schema%s() *schemapb.Schema {\n", root)
	fmt.Fprintf(b, "\t_schema%sOnce.Do(func() {\n", root)
	fmt.Fprintf(b, "\t\tvar s schemapb.Schema\n")
	fmt.Fprintf(b, "\t\tif err := proto.Unmarshal(_schema%sWire, &s); err != nil {\n\t\t\tpanic(err)\n\t\t}\n", root)
	fmt.Fprintf(b, "\t\t_schema%sVal = &s\n\t})\n\treturn _schema%sVal\n}\n\n", root, root)

	// Schema()
	fmt.Fprintf(b, "// Schema returns the schema this type was generated from.\n")
	fmt.Fprintf(b, "func (c *%s) Schema() *schemapb.Schema { return _schema%s() }\n\n", root, root)

	// Validate()
	fmt.Fprintf(b, "// Validate marshals the value and runs the schemapb engine against it.\n")
	fmt.Fprintf(b, "func (c *%s) Validate() []*schemapb.FieldError {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n")
	fmt.Fprintf(b, "\tif err != nil {\n\t\treturn []*schemapb.FieldError{schemapb.NewFieldError(\"\", err.Error())}\n\t}\n")
	fmt.Fprintf(b, "\treturn _schema%s().ValidateStruct(st)\n}\n\n", root)

	// Render
	fmt.Fprintf(b, "// Render serialises the value with the schema's named template\n")
	fmt.Fprintf(b, "// (Go text/template), e.g. a postgresql.conf. Same output as the engine/WASM.\n")
	fmt.Fprintf(b, "func (c *%s) Render(template string) (string, error) {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn \"\", err\n\t}\n")
	fmt.Fprintf(b, "\treturn _schema%s().Render(template, st.AsMap())\n}\n\n", root)

	// Identity
	id := f.Identity
	fmt.Fprintf(b, "// %sIdentity is the schema identity this type was generated from.\n", root)
	fmt.Fprintf(b, "var %sIdentity = &schemapb.SchemaIdentity{Namespace: %q, Name: %q, Version: %q}\n\n",
		root, id.GetNamespace(), id.GetName(), id.GetVersion())
}
