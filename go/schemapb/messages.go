package schemapb

import (
	"maps"
	"strings"
)

// The spec's shared message-template set: every implementation renders the
// SAME human text from these templates, so messages agree across languages
// even though they are not part of the conformance contract (codes and typed
// expected/actual are). The Go reference is the source of truth; the set is
// exported to conformance/golden/messages.json for the other ports.
//
// Placeholders: {expected} and {actual} substitute the display form (see
// displayString) of the error's typed values; a missing value renders "…".
// RULE_VIOLATED carries the rule author's message and EXPR_ERROR /
// INVALID_SCHEMA carry runtime/compiler text — none of the three is
// templated.
//
//nolint:gochecknoglobals // the spec-owned message-template table
var messageTemplates = map[ErrorCode]string{
	ErrorCode_ERROR_CODE_TYPE_MISMATCH:      "expected {expected}",
	ErrorCode_ERROR_CODE_REQUIRED_MISSING:   "required",
	ErrorCode_ERROR_CODE_NOT_NULLABLE:       "must not be null",
	ErrorCode_ERROR_CODE_UNKNOWN_FIELD:      "unknown field",
	ErrorCode_ERROR_CODE_IMMUTABLE_MODIFIED: "immutable: cannot be changed",

	ErrorCode_ERROR_CODE_CONST_MISMATCH:       "must equal {expected}",
	ErrorCode_ERROR_CODE_GT_VIOLATED:          "must be > {expected}",
	ErrorCode_ERROR_CODE_GTE_VIOLATED:         "must be >= {expected}",
	ErrorCode_ERROR_CODE_LT_VIOLATED:          "must be < {expected}",
	ErrorCode_ERROR_CODE_LTE_VIOLATED:         "must be <= {expected}",
	ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET:   "must be one of {expected}",
	ErrorCode_ERROR_CODE_IN_FORBIDDEN_SET:     "must not be one of {expected}",
	ErrorCode_ERROR_CODE_MULTIPLE_OF_VIOLATED: "must be a multiple of {expected}",

	ErrorCode_ERROR_CODE_LEN_MISMATCH:       "length must be exactly {expected}",
	ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED:   "length must be at least {expected}",
	ErrorCode_ERROR_CODE_MAX_LEN_VIOLATED:   "length must be at most {expected}",
	ErrorCode_ERROR_CODE_PATTERN_MISMATCH:   "must match pattern {expected}",
	ErrorCode_ERROR_CODE_FORMAT_MISMATCH:    "must be a valid {expected}",
	ErrorCode_ERROR_CODE_PREFIX_MISMATCH:    "must start with {expected}",
	ErrorCode_ERROR_CODE_SUFFIX_MISMATCH:    "must end with {expected}",
	ErrorCode_ERROR_CODE_UNSUPPORTED_FORMAT: "format {expected} is not supported by this implementation",

	ErrorCode_ERROR_CODE_CHOICE_NOT_ALLOWED: "must be one of {expected}",

	ErrorCode_ERROR_CODE_MIN_ITEMS_VIOLATED:  "must have at least {expected} items",
	ErrorCode_ERROR_CODE_MAX_ITEMS_VIOLATED:  "must have at most {expected} items",
	ErrorCode_ERROR_CODE_NOT_UNIQUE:          "must be unique",
	ErrorCode_ERROR_CODE_LIST_COUNT_MISMATCH: "must have exactly {expected} items",

	ErrorCode_ERROR_CODE_MIN_ENTRIES_VIOLATED: "must have at least {expected} entries",
	ErrorCode_ERROR_CODE_MAX_ENTRIES_VIOLATED: "must have at most {expected} entries",

	ErrorCode_ERROR_CODE_MIN_PROPERTIES_VIOLATED: "must have at least {expected} properties",
	ErrorCode_ERROR_CODE_MAX_PROPERTIES_VIOLATED: "must have at most {expected} properties",

	ErrorCode_ERROR_CODE_DISCRIMINATOR_MISSING: "discriminator {expected} must be a non-empty string",
	ErrorCode_ERROR_CODE_UNKNOWN_VARIANT:       "unknown variant {actual}",
	ErrorCode_ERROR_CODE_UNKNOWN_REF:           "unknown $ref {expected}",
}

// MessageTemplates returns a copy of the spec's message-template set (for
// tooling and the conformance export).
func MessageTemplates() map[ErrorCode]string {
	return maps.Clone(messageTemplates)
}

// renderMessage substitutes the typed values into a code's template. A nil
// value renders as "…" (this is also how masked secrets re-render).
func renderMessage(code ErrorCode, expected, actual *Value) string {
	t, ok := messageTemplates[code]
	if !ok {
		return ""
	}

	sub := func(s, ph string, v *Value) string {
		if !strings.Contains(s, ph) {
			return s
		}

		disp := "…"
		if v != nil {
			disp = displayString(v.ToGo())
		}

		return strings.ReplaceAll(s, ph, disp)
	}
	t = sub(t, "{expected}", expected)
	t = sub(t, "{actual}", actual)

	return t
}
