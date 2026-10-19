package schemapb

import (
	"maps"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
)

// The spec's shared message-template set: every implementation renders the
// SAME human text from these templates, so messages agree across languages
// even though they are not part of the conformance contract (codes and typed
// expected/actual are). The source of truth is the `error_spec` option on
// each ErrorCode value in errors.proto; the set is exported to
// conformance/golden/messages.json for the ports that cannot read options.
//
// Placeholders: {expected} and {actual} substitute the display form (see
// displayString) of the error's typed values; a missing value renders "…".
// RULE_VIOLATED carries the rule author's message and EXPR_ERROR /
// INVALID_SCHEMA carry runtime/compiler text — none of the three is
// templated.
//
// Loaded lazily: the generated descriptor registers in an init func, which
// runs after package-level variables are initialized.
//
//nolint:gochecknoglobals // derived once from the errors.proto descriptor
var errorSpecTables = sync.OnceValues(loadErrorSpecs)

func loadErrorSpecs() (map[ErrorCode]*ErrorSpec, map[ErrorCode]string) {
	specs := map[ErrorCode]*ErrorSpec{}
	templates := map[ErrorCode]string{}
	values := ErrorCode(0).Descriptor().Values()

	for i := range values.Len() {
		value := values.Get(i)

		spec, ok := proto.GetExtension(value.Options(), E_ErrorSpec).(*ErrorSpec)
		if !ok || spec == nil {
			continue
		}

		code := ErrorCode(value.Number())
		specs[code] = spec

		if spec.GetTemplate() != "" {
			templates[code] = spec.GetTemplate()
		}
	}

	return specs, templates
}

// SpecOf returns the ErrorSpec declared for code in errors.proto, or nil for
// ERROR_CODE_UNSPECIFIED and unknown codes. The result must not be mutated.
func SpecOf(code ErrorCode) *ErrorSpec {
	specs, _ := errorSpecTables()

	return specs[code]
}

func messageTemplate(code ErrorCode) (string, bool) {
	_, templates := errorSpecTables()
	t, ok := templates[code]

	return t, ok
}

// MessageTemplates returns a copy of the spec's message-template set (for
// tooling and the conformance export).
func MessageTemplates() map[ErrorCode]string {
	_, templates := errorSpecTables()

	return maps.Clone(templates)
}

// renderMessage substitutes the typed values into a code's template. A nil
// value renders as "…" (this is also how masked secrets re-render).
func renderMessage(code ErrorCode, expected, actual *Value) string {
	t, ok := messageTemplate(code)
	if !ok {
		return ""
	}

	return fillTemplate(t, expected, actual, DisplayValue)
}

func fillTemplate(t string, expected, actual *Value, display func(*Value) string) string {
	sub := func(s, ph string, v *Value) string {
		if !strings.Contains(s, ph) {
			return s
		}

		disp := "…"
		if v != nil {
			disp = display(v)
		}

		return strings.ReplaceAll(s, ph, disp)
	}
	t = sub(t, "{expected}", expected)
	t = sub(t, "{actual}", actual)

	return t
}

// DisplayValue is the spec's display form of a value, the one the default
// messages use for {expected} and {actual}.
func DisplayValue(v *Value) string { return displayString(v.ToGo()) }

// Catalog is a message catalog for one locale, the unit of translation.
// Keys are MessageKey values: an ErrorCode name ("ERROR_CODE_GTE_VIOLATED")
// for templated codes, the rule id for RULE_VIOLATED. Templates use the
// {expected} and {actual} placeholders of ErrorSpec.template. The English
// catalog is MessageTemplates keyed by code name, exported as
// conformance/golden/messages.json.
type Catalog map[string]string

// MessageKey is the catalog key that translates err: the rule id for a
// violated rule that declares one, the ErrorCode name otherwise.
func MessageKey(err *ValidationError) string {
	if err.GetCode() == ErrorCode_ERROR_CODE_RULE_VIOLATED && err.GetRuleId() != "" {
		return err.GetRuleId()
	}

	return err.GetCode().String()
}

// Message renders err from the catalog, substituting the typed values with
// display (nil => DisplayValue; pass a locale-aware one for numbers and
// times). A key absent from the catalog falls back to err.Message, so a
// partial translation degrades to the default text, never to nothing.
func (c Catalog) Message(err *ValidationError, display func(*Value) string) string {
	t, ok := c[MessageKey(err)]
	if !ok {
		return err.GetMessage()
	}

	if display == nil {
		display = DisplayValue
	}

	return fillTemplate(t, err.GetExpected(), err.GetActual(), display)
}
