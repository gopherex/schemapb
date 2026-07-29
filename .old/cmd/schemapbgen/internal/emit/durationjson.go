package emit

import (
	"bytes"
	"fmt"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

// writeCustomJSON emits custom MarshalJSON/UnmarshalJSON for every generated
// type that needs non-default JSON handling, in a single pair of methods per
// type so the two concerns never produce conflicting methods:
//
//   - time.Duration fields: the engine expects a string parseable by
//     time.ParseDuration (e.g. "5m0s"); Go's default marshals nanoseconds as an
//     integer. A shadow field of string type at depth 0 shadows the embedded
//     alias's duration field. (time.Time already round-trips as RFC3339, which
//     the engine accepts, so timestamps need no shadow.)
//   - oneof fields: the engine expects a single object carrying the
//     discriminator property alongside the active variant's fields. The bare
//     interface cannot marshal the discriminator nor unmarshal into a concrete
//     type, so the field is shadowed by a json.RawMessage produced/consumed by
//     the per-oneof codec helpers (marshal<Iface>/unmarshal<Iface>).
func writeCustomJSON(b *bytes.Buffer, f *model.File) {
	for _, t := range f.Types {
		var durs, durLists, oneofs, oneofLists []*model.Field
		for _, fld := range t.Fields {
			switch {
			case fld.OneOf != nil:
				oneofs = append(oneofs, fld)
			case fld.ListElemOneOf != nil:
				oneofLists = append(oneofLists, fld)
			case fld.GoType == "[]time.Duration":
				durLists = append(durLists, fld)
			case fld.GoType == "time.Duration" || fld.GoType == "*time.Duration":
				durs = append(durs, fld)
			}
		}
		if len(durs) == 0 && len(durLists) == 0 && len(oneofs) == 0 && len(oneofLists) == 0 {
			continue
		}
		writeTypeJSON(b, t, durs, durLists, oneofs, oneofLists)
	}
	writeOneOfCodecs(b, f)
}

func writeTypeJSON(b *bytes.Buffer, t *model.Type, durs, durLists, oneofs, oneofLists []*model.Field) {
	name := t.Name

	// ---- MarshalJSON ----
	fmt.Fprintf(b, "// MarshalJSON serializes duration/oneof fields in the form the engine expects.\n")
	fmt.Fprintf(b, "func (c %s) MarshalJSON() ([]byte, error) {\n", name)
	fmt.Fprintf(b, "\ttype Alias %s\n", name)
	for _, o := range oneofs {
		v := firstLower(o.Name) + "Raw"
		fmt.Fprintf(b, "\t%s, err := marshal%s(c.%s)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n", v, o.OneOf.IfaceName, o.Name)
	}
	for _, o := range oneofLists {
		v := firstLower(o.Name) + "Raw"
		fmt.Fprintf(b, "\tvar %s []json.RawMessage\n\tif c.%s != nil {\n\t\t%s = make([]json.RawMessage, len(c.%s))\n\t\tfor i, e := range c.%s {\n\t\t\tr, err := marshal%s(e)\n\t\t\tif err != nil {\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\t%s[i] = r\n\t\t}\n\t}\n",
			v, o.Name, v, o.Name, o.Name, o.ListElemOneOf.IfaceName, v)
	}
	fmt.Fprintf(b, "\treturn json.Marshal(&struct {\n\t\t*Alias\n")
	for _, d := range durs {
		if d.Pointer {
			fmt.Fprintf(b, "\t\t%s *string `json:%q`\n", d.Name, d.JSONName+",omitempty")
		} else {
			fmt.Fprintf(b, "\t\t%s string `json:%q`\n", d.Name, d.JSONName)
		}
	}
	for _, d := range durLists {
		fmt.Fprintf(b, "\t\t%s []string `json:%q`\n", d.Name, d.JSONName+",omitempty")
	}
	for _, o := range oneofs {
		fmt.Fprintf(b, "\t\t%s json.RawMessage `json:%q`\n", o.Name, o.JSONName+",omitempty")
	}
	for _, o := range oneofLists {
		fmt.Fprintf(b, "\t\t%s []json.RawMessage `json:%q`\n", o.Name, o.JSONName+",omitempty")
	}
	fmt.Fprintf(b, "\t}{\n\t\tAlias: (*Alias)(&c),\n")
	for _, d := range durs {
		if d.Pointer {
			fmt.Fprintf(b, "\t\t%s: func() *string { if c.%s == nil { return nil }; s := c.%s.String(); return &s }(),\n", d.Name, d.Name, d.Name)
		} else {
			fmt.Fprintf(b, "\t\t%s: time.Duration(c.%s).String(),\n", d.Name, d.Name)
		}
	}
	for _, d := range durLists {
		fmt.Fprintf(b, "\t\t%s: func() []string { if c.%s == nil { return nil }; out := make([]string, len(c.%s)); for i, d := range c.%s { out[i] = d.String() }; return out }(),\n",
			d.Name, d.Name, d.Name, d.Name)
	}
	for _, o := range oneofs {
		fmt.Fprintf(b, "\t\t%s: %s,\n", o.Name, firstLower(o.Name)+"Raw")
	}
	for _, o := range oneofLists {
		fmt.Fprintf(b, "\t\t%s: %s,\n", o.Name, firstLower(o.Name)+"Raw")
	}
	fmt.Fprintf(b, "\t})\n}\n\n")

	// ---- UnmarshalJSON ----
	fmt.Fprintf(b, "// UnmarshalJSON parses the engine-form duration/oneof fields back into Go values.\n")
	fmt.Fprintf(b, "func (c *%s) UnmarshalJSON(data []byte) error {\n", name)
	fmt.Fprintf(b, "\ttype Alias %s\n", name)
	fmt.Fprintf(b, "\taux := &struct {\n\t\t*Alias\n")
	for _, d := range durs {
		typ := "string"
		if d.Pointer {
			typ = "*string"
		}
		fmt.Fprintf(b, "\t\t%s %s `json:%q`\n", d.Name, typ, d.JSONName+",omitempty")
	}
	for _, d := range durLists {
		fmt.Fprintf(b, "\t\t%s []string `json:%q`\n", d.Name, d.JSONName+",omitempty")
	}
	for _, o := range oneofs {
		fmt.Fprintf(b, "\t\t%s json.RawMessage `json:%q`\n", o.Name, o.JSONName+",omitempty")
	}
	for _, o := range oneofLists {
		fmt.Fprintf(b, "\t\t%s []json.RawMessage `json:%q`\n", o.Name, o.JSONName+",omitempty")
	}
	fmt.Fprintf(b, "\t}{Alias: (*Alias)(c)}\n")
	fmt.Fprintf(b, "\tif err := json.Unmarshal(data, aux); err != nil {\n\t\treturn err\n\t}\n")
	for _, d := range durs {
		if d.Pointer {
			fmt.Fprintf(b, "\tif aux.%s != nil && *aux.%s != \"\" {\n\t\tv, err := time.ParseDuration(*aux.%s)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tc.%s = &v\n\t}\n", d.Name, d.Name, d.Name, d.Name)
		} else {
			fmt.Fprintf(b, "\tif aux.%s != \"\" {\n\t\tv, err := time.ParseDuration(aux.%s)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tc.%s = v\n\t}\n", d.Name, d.Name, d.Name)
		}
	}
	for _, d := range durLists {
		fmt.Fprintf(b, "\tif aux.%s != nil {\n\t\tc.%s = make([]time.Duration, len(aux.%s))\n\t\tfor i, s := range aux.%s {\n\t\t\tv, err := time.ParseDuration(s)\n\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t\tc.%s[i] = v\n\t\t}\n\t}\n",
			d.Name, d.Name, d.Name, d.Name, d.Name)
	}
	for _, o := range oneofs {
		fmt.Fprintf(b, "\tif len(aux.%s) > 0 {\n\t\tv, err := unmarshal%s(aux.%s)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tc.%s = v\n\t}\n", o.Name, o.OneOf.IfaceName, o.Name, o.Name)
	}
	for _, o := range oneofLists {
		fmt.Fprintf(b, "\tif aux.%s != nil {\n\t\tc.%s = make([]%s, len(aux.%s))\n\t\tfor i, r := range aux.%s {\n\t\t\tv, err := unmarshal%s(r)\n\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t\tc.%s[i] = v\n\t\t}\n\t}\n",
			o.Name, o.Name, o.ListElemOneOf.IfaceName, o.Name, o.Name, o.ListElemOneOf.IfaceName, o.Name)
	}
	fmt.Fprintf(b, "\treturn nil\n}\n\n")
}

// writeOneOfCodecs emits one marshal/unmarshal helper per distinct oneof
// interface in the file. marshal<Iface> flattens the active variant's fields
// and injects the discriminator; unmarshal<Iface> reads the discriminator and
// decodes into the matching concrete variant.
func writeOneOfCodecs(b *bytes.Buffer, f *model.File) {
	seen := map[string]bool{}
	var defs []*model.OneOfDef
	add := func(od *model.OneOfDef) {
		if od != nil && !seen[od.IfaceName] {
			seen[od.IfaceName] = true
			defs = append(defs, od)
		}
	}
	for _, t := range f.Types {
		for _, fld := range t.Fields {
			add(fld.OneOf)
			add(fld.ListElemOneOf)
		}
	}
	for _, od := range defs {
		keys := make([]string, 0, len(od.Variants))
		for k := range od.Variants {
			keys = append(keys, k)
		}
		sortStrings(keys)

		// marshal
		fmt.Fprintf(b, "func marshal%s(v %s) (json.RawMessage, error) {\n", od.IfaceName, od.IfaceName)
		fmt.Fprintf(b, "\tif v == nil {\n\t\treturn nil, nil\n\t}\n\tvar key string\n\tswitch v.(type) {\n")
		for _, k := range keys {
			fmt.Fprintf(b, "\tcase *%s:\n\t\tkey = %q\n", od.Variants[k], k)
		}
		fmt.Fprintf(b, "\tdefault:\n\t\treturn nil, fmt.Errorf(\"unknown %s variant: %%T\", v)\n\t}\n", od.IfaceName)
		fmt.Fprintf(b, "\traw, err := json.Marshal(v)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(b, "\tm := map[string]json.RawMessage{}\n\tif err := json.Unmarshal(raw, &m); err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(b, "\tkb, err := json.Marshal(key)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(b, "\tm[%q] = kb\n\treturn json.Marshal(m)\n}\n\n", od.Discriminator)

		// unmarshal
		fmt.Fprintf(b, "func unmarshal%s(data json.RawMessage) (%s, error) {\n", od.IfaceName, od.IfaceName)
		fmt.Fprintf(b, "\tvar disc struct {\n\t\tKind string `json:%q`\n\t}\n", od.Discriminator)
		fmt.Fprintf(b, "\tif err := json.Unmarshal(data, &disc); err != nil {\n\t\treturn nil, err\n\t}\n\tswitch disc.Kind {\n")
		for _, k := range keys {
			fmt.Fprintf(b, "\tcase %q:\n\t\tvar x %s\n\t\tif err := json.Unmarshal(data, &x); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\treturn &x, nil\n", k, od.Variants[k])
		}
		fmt.Fprintf(b, "\tdefault:\n\t\treturn nil, fmt.Errorf(\"unknown %s variant: %%q\", disc.Kind)\n\t}\n}\n\n", od.IfaceName)
	}
}

func firstLower(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] += 32
	}
	return string(r)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
