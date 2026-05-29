package emit

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeStructs(b *bytes.Buffer, f *model.File) {
	for _, t := range f.Types {
		if t.Doc != "" {
			fmt.Fprintf(b, "// %s: %s\n", t.Name, t.Doc)
		}
		fmt.Fprintf(b, "type %s struct {\n", t.Name)
		for _, fld := range t.Fields {
			writeDoc(b, fld.Doc)
			tag := fld.JSONName
			if fld.OmitEmpty {
				tag += ",omitempty"
			}
			fmt.Fprintf(b, "\t%s %s `json:%q`\n", fld.Name, fld.GoType, tag)
		}
		fmt.Fprintf(b, "}\n\n")
		writeOneOfIfaces(b, t)
	}
}

func writeDoc(b *bytes.Buffer, doc string) {
	if doc == "" {
		return
	}
	for _, line := range splitLines(doc) {
		fmt.Fprintf(b, "\t// %s\n", line)
	}
}

func writeOneOfIfaces(b *bytes.Buffer, t *model.Type) {
	for _, fld := range t.Fields {
		if fld.OneOf != nil {
			writeOneOfIface(b, fld.OneOf)
		}
		if fld.ListElemOneOf != nil {
			writeOneOfIface(b, fld.ListElemOneOf)
		}
	}
}

func writeOneOfIface(b *bytes.Buffer, od *model.OneOfDef) {
	fmt.Fprintf(b, "// %s is a discriminated union (discriminator %q).\n", od.IfaceName, od.Discriminator)
	fmt.Fprintf(b, "type %s interface{ is%s() }\n\n", od.IfaceName, od.IfaceName)
	keys := make([]string, 0, len(od.Variants))
	for k := range od.Variants {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "func (*%s) is%s() {}\n", od.Variants[k], od.IfaceName)
	}
	fmt.Fprintf(b, "\n")
}

func writeEnums(b *bytes.Buffer, f *model.File) {
	for _, e := range f.Enums {
		fmt.Fprintf(b, "type %s int32\n\n", e.Name)
		vals := make([]int, 0, len(e.Values))
		for k := range e.Values {
			vals = append(vals, int(k))
		}
		sort.Ints(vals)
		fmt.Fprintf(b, "const (\n")
		for _, v := range vals {
			label := e.Values[int32(v)]
			fmt.Fprintf(b, "\t%s%s %s = %d\n", e.Name, pascalLabel(label), e.Name, v)
		}
		fmt.Fprintf(b, ")\n\n")
		// String()
		fmt.Fprintf(b, "func (e %s) String() string {\n\tswitch e {\n", e.Name)
		for _, v := range vals {
			fmt.Fprintf(b, "\tcase %d:\n\t\treturn %q\n", v, e.Values[int32(v)])
		}
		fmt.Fprintf(b, "\tdefault:\n\t\treturn \"\"\n\t}\n}\n\n")
	}
}

// writeStrEnums emits a `type <Name> = string` alias plus a const per allowed
// value for each String field that carried an `in` set. The alias keeps the
// field assignable from a plain string (backward compatible) while giving the
// allowed values typed names.
func writeStrEnums(b *bytes.Buffer, f *model.File) {
	for _, e := range f.StrEnums {
		fmt.Fprintf(b, "// %s enumerates the allowed values of this string field.\n", e.Name)
		fmt.Fprintf(b, "type %s = string\n\n", e.Name)
		fmt.Fprintf(b, "const (\n")
		for _, v := range e.Values {
			fmt.Fprintf(b, "\t%s%s %s = %q\n", e.Name, pascalLabel(v), e.Name, v)
		}
		fmt.Fprintf(b, ")\n\n")
	}
}

func splitLines(s string) []string {
	res := []string{}
	start := 0
	for i, r := range s {
		if r == '\n' {
			res = append(res, s[start:i])
			start = i + 1
		}
	}
	res = append(res, s[start:])
	return res
}

func pascalLabel(s string) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return "Unspecified"
	}
	out := ""
	up := true
	for _, r := range rs {
		if r == '_' || r == '-' || r == '.' || r == ' ' {
			up = true
			continue
		}
		if up {
			out += string(toUpper(r))
			up = false
		} else {
			out += string(r)
		}
	}
	return out
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
