package schemapb

import (
	"fmt"
	"strings"
	"text/template"
)

// RenderField is one field as seen by a render template.
type RenderField struct {
	Name        string
	Title       string
	Description string
	Unit        string
	Group       string
	Kind        string // "int64", "string", "bool", "enum", ...
	Value       any    // resolved value from the values map (may be nil)
	Label       string // for enum fields: the label of Value, if known
	Computed    bool
	Secret      bool
	Immutable   bool
	Deprecated  bool
}

// RenderGroup is a set of fields sharing the same Group, in first-seen order.
type RenderGroup struct {
	Name   string
	Fields []RenderField
}

// RenderContext is the data a render template executes against.
type RenderContext struct {
	Fields []RenderField
	Groups []RenderGroup
	Values map[string]any
}

// renderFuncs is the fixed, portable function map available in templates. It
// runs in Go on both the server and (via WASM) the browser, so output is
// identical across platforms.
var renderFuncs = template.FuncMap{
	"onoff": func(v any) string {
		if b, ok := v.(bool); ok && b {
			return "on"
		}
		return "off"
	},
	"yesno": func(v any) string {
		if b, ok := v.(bool); ok && b {
			return "yes"
		}
		return "no"
	},
	"quote": func(v any) string { return fmt.Sprintf("%q", fmt.Sprint(v)) },
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"default": func(d, v any) any {
		if v == nil || v == "" {
			return d
		}
		return v
	},
}

// Render executes the named template (from the schema's templates map) against
// the given resolved values and returns the produced text.
func (s *Schema) Render(name string, values map[string]any) (string, error) {
	tmpl, ok := s.GetTemplates()[name]
	if !ok {
		return "", fmt.Errorf("schemapb: no template %q in schema", name)
	}
	t, err := template.New(name).Funcs(renderFuncs).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("schemapb: parse template %q: %w", name, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, s.renderContext(values)); err != nil {
		return "", fmt.Errorf("schemapb: execute template %q: %w", name, err)
	}
	return b.String(), nil
}

// Render renders a Baked snapshot with the named template of its embedded schema.
func (b *Baked) Render(name string) (string, error) {
	var values map[string]any
	if st := b.GetValues(); st != nil {
		values = st.AsMap()
	}
	return b.GetSchema().Render(name, values)
}

// renderContext builds the template data from the schema and resolved values.
func (s *Schema) renderContext(values map[string]any) RenderContext {
	if values == nil {
		values = map[string]any{}
	}
	ctx := RenderContext{Values: values}
	groupIdx := map[string]int{} // group name -> index in ctx.Groups
	for _, f := range s.GetFields() {
		rf := RenderField{
			Name:        f.GetName(),
			Title:       f.GetTitle(),
			Description: f.GetDescription(),
			Unit:        f.GetUnit(),
			Group:       f.GetGroup(),
			Kind:        kindName(f),
			Value:       values[f.GetName()],
			Secret:      f.GetSecret(),
			Immutable:   f.GetImmutable(),
			Deprecated:  f.GetDeprecated(),
		}
		if _, ok := f.GetKind().(*Schema_Filed_Computed_); ok {
			rf.Computed = true
		}
		rf.Label = enumLabel(f, rf.Value)
		ctx.Fields = append(ctx.Fields, rf)

		i, seen := groupIdx[rf.Group]
		if !seen {
			i = len(ctx.Groups)
			groupIdx[rf.Group] = i
			ctx.Groups = append(ctx.Groups, RenderGroup{Name: rf.Group})
		}
		ctx.Groups[i].Fields = append(ctx.Groups[i].Fields, rf)
	}
	return ctx
}

// enumLabel returns the label of an enum field's current value, if resolvable.
func enumLabel(f *Schema_Filed, v any) string {
	e, ok := f.GetKind().(*Schema_Filed_Enum_)
	if !ok || v == nil {
		return ""
	}
	var key int32
	switch n := v.(type) {
	case float64:
		key = int32(n)
	case int32:
		key = n
	case int64:
		key = int32(n)
	case int:
		key = int32(n)
	default:
		return ""
	}
	return e.Enum.GetValues()[key]
}

// kindName returns a short name for a field's kind.
func kindName(f *Schema_Filed) string {
	switch f.GetKind().(type) {
	case *Schema_Filed_Float_:
		return "float"
	case *Schema_Filed_Double_:
		return "double"
	case *Schema_Filed_Int32_:
		return "int32"
	case *Schema_Filed_Int64_:
		return "int64"
	case *Schema_Filed_Uint32:
		return "uint32"
	case *Schema_Filed_Uint64:
		return "uint64"
	case *Schema_Filed_Bool_:
		return "bool"
	case *Schema_Filed_String_:
		return "string"
	case *Schema_Filed_Enum_:
		return "enum"
	case *Schema_Filed_Duration_:
		return "duration"
	case *Schema_Filed_Timestamp_:
		return "timestamp"
	case *Schema_Filed_List_:
		return "list"
	case *Schema_Filed_Object_:
		return "object"
	case *Schema_Filed_OneOf_:
		return "oneof"
	case *Schema_Filed_Ref_:
		return "ref"
	default:
		return ""
	}
}
