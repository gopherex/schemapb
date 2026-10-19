package schemapb_test

import (
	"testing"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

func TestCatalogMessage(t *testing.T) {
	t.Parallel()

	russian := schemapb.Catalog{
		"ERROR_CODE_GTE_VIOLATED": "должно быть не меньше {expected} (сейчас {actual})",
		"cross":                   "i64 должно покрывать i32",
	}

	gte := &schemapb.ValidationError{
		Code:     schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED,
		Expected: schemapb.Int64V(1),
		Actual:   schemapb.Int64V(0),
		Message:  "must be >= 1",
	}
	rule := &schemapb.ValidationError{
		Code: schemapb.ErrorCode_ERROR_CODE_RULE_VIOLATED, RuleId: ptr("cross"), Message: "i64 must cover i32",
	}
	untranslated := &schemapb.ValidationError{Code: schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING, Message: "required"}

	cases := []struct {
		err  *schemapb.ValidationError
		key  string
		want string
	}{
		{gte, "ERROR_CODE_GTE_VIOLATED", "должно быть не меньше 1 (сейчас 0)"},
		{rule, "cross", "i64 должно покрывать i32"},
		{untranslated, "ERROR_CODE_REQUIRED_MISSING", "required"},
	}

	for _, tc := range cases {
		if got := schemapb.MessageKey(tc.err); got != tc.key {
			t.Errorf("MessageKey = %q, want %q", got, tc.key)
		}

		if got := russian.Message(tc.err, nil); got != tc.want {
			t.Errorf("Message = %q, want %q", got, tc.want)
		}
	}

	display := func(v *schemapb.Value) string { return "<" + schemapb.DisplayValue(v) + ">" }
	if got := russian.Message(gte, display); got != "должно быть не меньше <1> (сейчас <0>)" {
		t.Errorf("custom display: %q", got)
	}
}
