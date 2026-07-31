package schemapb_test

import (
	"strings"
	"testing"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

func TestTupleList(t *testing.T) {
	s := schemapb.NewSchema(schemapb.ID("t", "tuple", schemapb.Ver(1, 0, 0))).Fields(
		schemapb.List("pair",
			schemapb.Str("host").MinLen(1),
			schemapb.Int64("port").Gte(1).Lte(65535),
		),
	).MustBuild()

	if res := mustValidate(t, s, map[string]any{"pair": []any{"db", int64(5432)}}); !res.Ok() {
		t.Fatalf("valid tuple: %v", res.GetErrors())
	}
	// Wrong length + element violations.
	res := mustValidate(t, s, map[string]any{"pair": []any{""}})
	if !hasCode(res, "pair", schemapb.ErrorCode_ERROR_CODE_LIST_COUNT_MISMATCH) {
		t.Errorf("length: %v", codes(res))
	}
	if !hasCode(res, "pair[0]", schemapb.ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED) {
		t.Errorf("element 0: %v", codes(res))
	}
	res = mustValidate(t, s, map[string]any{"pair": []any{"db", "not-a-port"}})
	if !hasCode(res, "pair[1]", schemapb.ErrorCode_ERROR_CODE_TYPE_MISMATCH) {
		t.Errorf("element 1: %v", codes(res))
	}
	// Canonical form keeps per-position variants.
	baked, _, err := s.Bake(map[string]any{"pair": []any{"db", int64(80)}})
	if err != nil {
		t.Fatal(err)
	}
	items := baked.GetValues().GetFields()["pair"].GetListValue().GetItems()
	if items[0].GetStringValue() != "db" || items[1].GetInt64Value() != 80 {
		t.Errorf("canonical tuple: %v", items)
	}
	// Tuple + homogeneous-list constraints is a schema defect.
	if _, err := schemapb.NewSchema(schemapb.ID("t", "badtuple", schemapb.Ver(1, 0, 0))).Fields(
		schemapb.List("x", schemapb.Str("a"), schemapb.Str("b")).Unique(),
	).Build(); err == nil {
		t.Error("tuple with unique must fail Build")
	}
}

func TestCostLimit(t *testing.T) {
	s := schemapb.NewSchema(schemapb.ID("t", "cost", schemapb.Ver(1, 0, 0))).Fields(
		schemapb.List("xs", schemapb.Int64("")).Rules(
			schemapb.Rule("this.map(a, this.map(b, a + b)).size() > 0", "cartesian"),
		),
	).MustBuild()
	big := make([]any, 200)
	for i := range big {
		big[i] = int64(i)
	}

	unlimited, err := schemapb.Compile(s)
	if err != nil {
		t.Fatal(err)
	}
	if res := unlimited.Validate(map[string]any{"xs": append([]any{}, big...)}); !res.Ok() {
		t.Fatalf("unlimited engine: %v", res.GetErrors())
	}

	capped, err := schemapb.Compile(s, schemapb.WithCostLimit(100))
	if err != nil {
		t.Fatal(err)
	}
	res := capped.Validate(map[string]any{"xs": append([]any{}, big...)})
	var hit bool
	for _, e := range res.GetErrors() {
		if e.GetCode() == schemapb.ErrorCode_ERROR_CODE_EXPR_ERROR &&
			strings.Contains(e.GetMessage(), "cost") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("cost limit not enforced: %v", res.GetErrors())
	}
}

func TestBytesCoercion(t *testing.T) {
	s := schemapb.NewSchema(schemapb.ID("t", "bcoerce", schemapb.Ver(1, 0, 0))).Coerce().Fields(
		schemapb.Bytes("blob").MinLen(2),
	).MustBuild()
	vals := map[string]any{"blob": "3q0="} // base64 of 0xDE 0xAD
	if res := mustValidate(t, s, vals); !res.Ok() {
		t.Fatalf("coerced bytes: %v", res.GetErrors())
	}
	b, ok := vals["blob"].([]byte)
	if !ok || len(b) != 2 || b[0] != 0xDE {
		t.Errorf("blob = %#v", vals["blob"])
	}
}

func TestSecretMessageMasked(t *testing.T) {
	s := schemapb.NewSchema(schemapb.ID("t", "secretmsg", schemapb.Ver(1, 0, 0))).Fields(
		schemapb.Str("pass").Const("expected-secret").Secret(),
	).MustBuild()
	res := mustValidate(t, s, map[string]any{"pass": "LEAKME"})
	for _, e := range res.GetErrors() {
		if e.GetActual() != nil {
			t.Errorf("actual not masked: %v", e)
		}
		if strings.Contains(e.GetMessage(), "LEAKME") {
			t.Errorf("secret leaked via message: %q", e.GetMessage())
		}
	}
}
