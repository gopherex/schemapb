package schemapb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Rendering executes a schema-carried Mustache template against the resolved
// values. The render context is part of the cross-language contract (every
// implementation builds the identical structure — the conformance suite
// compares rendered output byte-for-byte):
//
//	fields  — ordered list of ACTIVE fields, each a map with keys:
//	          name title description unit group kind label     (strings)
//	          set computed secret immutable deprecated         (bools)
//	          value onoff yesno quoted upper lower             (display forms)
//	groups  — [{name, fields}] grouped by Field.group, first-seen order
//	values  — map of field name -> display string for every present value
//
// Formatting helpers of the old engine (onoff, quote, ...) are precomputed
// context fields — Mustache stays logic-less; anything smarter belongs in
// Computed fields.

// Render executes the named template (from the schema's templates map)
// against the given resolved values.
func (s *Schema) Render(name TemplateName, values map[string]any) (string, error) {
	e, err := s.engine()
	if err != nil {
		return "", err
	}

	return e.Render(name, values)
}

// Render is the compiled-engine form of (*Schema).Render.
func (e *Engine) Render(name TemplateName, values map[string]any) (string, error) {
	tmpl := e.templates[string(name)]
	if tmpl == nil {
		return "", fmt.Errorf("schemapb: no template %q in schema", name)
	}

	out, err := tmpl.Render(e.renderContext(values))
	if err != nil {
		return "", fmt.Errorf("schemapb: render %q: %w", name, err)
	}

	return out, nil
}

// Render renders a Baked snapshot with the named template of its embedded
// schema.
func (b *Baked) Render(name TemplateName) (string, error) {
	return b.GetSchema().Render(name, b.GetValues().ToGo())
}

// renderContext builds the contract context from the schema and resolved
// values. Inactive fields (when=false over the values) are excluded entirely.
//
//nolint:cyclop,funlen // one linear context build
func (e *Engine) renderContext(values map[string]any) map[string]any {
	if values == nil {
		values = map[string]any{}
	}

	fields := make([]map[string]any, 0, len(e.schema.GetFields()))

	var groups []map[string]any

	groupIdx := map[string]int{}

	for _, f := range e.schema.GetFields() {
		if f.GetWhen() != "" {
			if ok, err := e.evalBool(f.GetWhen(), map[string]any{"this": nil, "root": values}); err != nil || !ok {
				continue
			}
		}

		val, set := values[f.GetName()]
		display := ""

		if set && val != nil {
			display = displayString(val)
		}

		label := ""

		if ch := f.GetChoice(); ch != nil && set {
			for _, o := range ch.GetOptions() {
				if nativeEqual(val, o.GetValue().ToGo()) {
					label = o.GetLabel()

					break
				}
			}
		}

		b, _ := val.(bool)
		rf := map[string]any{
			"name":        f.GetName(),
			"title":       f.GetTitle(),
			"description": f.GetDescription(),
			"unit":        f.GetUnit(),
			"group":       f.GetGroup(),
			"kind":        kindName(f),
			"label":       label,
			"set":         set && val != nil,
			"computed":    f.GetComputed() != nil,
			"secret":      f.GetSecret(),
			"immutable":   f.GetImmutable(),
			"deprecated":  f.GetDeprecated(),
			"value":       display,
			"onoff":       onoff(b),
			"yesno":       yesno(b),
			"quoted":      strconv.Quote(display),
			"upper":       toUpper(display),
			"lower":       toLower(display),
		}
		fields = append(fields, rf)

		g := f.GetGroup()

		i, seen := groupIdx[g]
		if !seen {
			i = len(groups)
			groupIdx[g] = i

			groups = append(groups, map[string]any{"name": g, "fields": []map[string]any{}})
		}

		if gf, ok := groups[i]["fields"].([]map[string]any); ok {
			groups[i]["fields"] = append(gf, rf)
		}
	}

	display := make(map[string]any, len(values))

	for name, val := range values {
		if val == nil {
			display[name] = ""

			continue
		}

		display[name] = displayString(val)
	}

	return map[string]any{"fields": fields, "groups": groups, "values": display}
}

func onoff(b bool) string {
	if b {
		return "on"
	}

	return "off"
}

func yesno(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}

// toUpper / toLower are ASCII-only on purpose: locale-dependent Unicode
// casing (Turkish i) would break cross-implementation determinism.
func toUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if 'a' <= c && c <= 'z' {
			out[i] = c - 'a' + 'A'
		}
	}

	return string(out)
}

func toLower(s string) string {
	out := []byte(s)
	for i, c := range out {
		if 'A' <= c && c <= 'Z' {
			out[i] = c - 'A' + 'a'
		}
	}

	return string(out)
}

// displayString renders a native value in the spec's display form: integers
// in decimal, doubles in JSON number form, bool as true/false, duration in
// Go form ("5m0s"), timestamps as RFC3339, bytes as std base64, lists and
// objects as compact JSON (with the same leaf forms).
//
//nolint:cyclop // flat exhaustive native-type dispatch
func displayString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case float64:
		return jsonNumber(t)
	case float32:
		return jsonNumber(t)
	case time.Duration:
		return t.String()
	case time.Time:
		return t.Format(time.RFC3339)
	case []byte:
		return base64.StdEncoding.EncodeToString(t)
	case []any, map[string]any:
		b, err := json.Marshal(displayJSON(t))
		if err != nil {
			return fmt.Sprint(t)
		}

		return string(b)
	default:
		return fmt.Sprint(t)
	}
}

// jsonNumber renders a float in JSON number form (NaN/Inf fall back to
// fmt's form — they have no JSON encoding).
func jsonNumber(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}

	return string(b)
}

// displayJSON converts non-JSON leaves (duration, time, bytes) to their
// display strings so container JSON stays in the spec's leaf forms.
func displayJSON(v any) any {
	switch t := v.(type) {
	case time.Duration, time.Time, []byte:
		return displayString(t)
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = displayJSON(el)
		}

		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, el := range t {
			out[k] = displayJSON(el)
		}

		return out
	default:
		return v
	}
}

// kindName returns the short kind name used in render contexts.
//
//nolint:cyclop,funlen // flat exhaustive kind dispatch
func kindName(f *Schema_Field) string {
	switch f.GetKind().(type) {
	case *Schema_Field_Float_:
		return "float"
	case *Schema_Field_Double_:
		return "double"
	case *Schema_Field_Int32_:
		return "int32"
	case *Schema_Field_Int64_:
		return "int64"
	case *Schema_Field_Uint32:
		return "uint32"
	case *Schema_Field_Uint64:
		return "uint64"
	case *Schema_Field_Bool_:
		return "bool"
	case *Schema_Field_String_:
		return "string"
	case *Schema_Field_Bytes_:
		return "bytes"
	case *Schema_Field_Choice_:
		return "choice"
	case *Schema_Field_Duration_:
		return "duration"
	case *Schema_Field_Timestamp_:
		return "timestamp"
	case *Schema_Field_List_:
		return "list"
	case *Schema_Field_Object_:
		return "object"
	case *Schema_Field_Map_:
		return "map"
	case *Schema_Field_OneOf_:
		return "oneof"
	case *Schema_Field_Ref_:
		return "ref"
	case *Schema_Field_Computed_:
		return "computed"
	case *Schema_Field_Json_:
		return "json"
	default:
		return ""
	}
}
