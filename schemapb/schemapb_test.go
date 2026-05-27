package schemapb_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	schemapb "github.com/stroppy-io/schemapb/schemapb"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func msgs(errs []*schemapb.FieldError) map[string]string {
	m := map[string]string{}
	for _, e := range errs {
		m[e.GetField()] = e.GetMessage()
	}
	return m
}

func mustValidator(t *testing.T, s *schemapb.Schema) *schemapb.Validator {
	t.Helper()
	v, err := schemapb.NewValidator(s)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

// validateJSON validates body and returns field->message.
func validateJSON(t *testing.T, v *schemapb.Validator, body string) map[string]string {
	t.Helper()
	errs, err := v.ValidateJSON(json.RawMessage(body))
	if err != nil {
		t.Fatalf("ValidateJSON(%s): %v", body, err)
	}
	return msgs(errs)
}

// id is a valid identity for building schemas in tests.
func id() schemapb.SchemaOption { return schemapb.Identity("test", "s", "v1") }

func has(m map[string]string, field string) bool { _, ok := m[field]; return ok }

// ---------------------------------------------------------------------------
// builders
// ---------------------------------------------------------------------------

func TestPtr(t *testing.T) {
	if *schemapb.Ptr(7) != 7 || *schemapb.Ptr("x") != "x" {
		t.Fatal("Ptr")
	}
}

func TestBuilders_AllKinds(t *testing.T) {
	s := schemapb.NewSchema(
		id(),
		schemapb.Description("kitchen sink"),
		schemapb.Fields(
			schemapb.Field("f", schemapb.Float(schemapb.FloatGte(0), schemapb.FloatLte(1))),
			schemapb.Field("d", schemapb.Double(schemapb.DoubleMultipleOf(0.5))),
			schemapb.Field("i32", schemapb.Int32(schemapb.Int32In(1, 2, 3))),
			schemapb.Field("i64", schemapb.Int64(schemapb.Int64Gt(0))),
			schemapb.Field("u32", schemapb.UInt32(schemapb.UInt32Lte(9))),
			schemapb.Field("u64", schemapb.UInt64(schemapb.UInt64Const(4))),
			schemapb.Field("b", schemapb.Bool(schemapb.BoolConst(true))),
			schemapb.Field("s", schemapb.String(schemapb.StringMinLen(1)), schemapb.FieldDescription("name")),
			schemapb.Field("e", schemapb.Enum(schemapb.EnumValues(map[int32]string{1: "a"}), schemapb.EnumDefinedOnly())),
			schemapb.Field("dur", schemapb.Duration(schemapb.DurationLte(time.Minute))),
			schemapb.Field("ts", schemapb.Timestamp(schemapb.TimestampGte(time.Unix(0, 0)))),
			schemapb.Field("list", schemapb.List(schemapb.ListMinItems(1), schemapb.ListItems(
				schemapb.Field("it", schemapb.String()),
			))),
			schemapb.Field("obj", schemapb.Object(schemapb.NewSchema(schemapb.Fields(
				schemapb.Field("inner", schemapb.Bool()),
			)))),
		),
	)
	if errs := schemapb.ValidateSchema(s); len(errs) != 0 {
		t.Fatalf("kitchen-sink schema invalid: %v", msgs(errs))
	}
	if s.GetId().GetName() != "s" || s.GetDescription() != "kitchen sink" {
		t.Errorf("identity/description not set: %v", s.GetId())
	}
	if len(s.GetFields()) != 13 {
		t.Errorf("fields = %d", len(s.GetFields()))
	}
	if s.GetFields()[0].GetFloat().GetLte() != 1 {
		t.Errorf("float lte not set")
	}
}

// ---------------------------------------------------------------------------
// self-schema validation
// ---------------------------------------------------------------------------

func TestValidateSchema_OK(t *testing.T) {
	if errs := schemapb.ValidateSchema(diskSchema()); len(errs) != 0 {
		t.Fatalf("expected valid, got %v", msgs(errs))
	}
}

func TestValidateSchema_Malformed(t *testing.T) {
	bad := &schemapb.Schema{
		Fields: []*schemapb.Schema_Filed{
			{Name: ""}, // no name, no kind
			{Name: "dup", Kind: &schemapb.Schema_Filed_String_{String_: &schemapb.Schema_Filed_String{Pattern: schemapb.Ptr("(")}}},
			{Name: "dup", Kind: &schemapb.Schema_Filed_Bool_{Bool: &schemapb.Schema_Filed_Bool{}}},
			{Name: "r", Kind: &schemapb.Schema_Filed_Bool_{Bool: &schemapb.Schema_Filed_Bool{}},
				Rules: []*schemapb.Schema_Filed_Rule{{Expr: "this >"}}}, // bad expr
			{Name: "lst", Kind: &schemapb.Schema_Filed_List_{List: &schemapb.Schema_Filed_List{}}}, // no items
		},
	}
	errs := schemapb.ValidateSchema(bad)
	if len(errs) == 0 {
		t.Fatal("expected schema errors")
	}
	if _, err := schemapb.NewValidator(bad); err == nil {
		t.Fatal("NewValidator accepted malformed schema")
	} else if _, ok := err.(*schemapb.SchemaError); !ok {
		t.Fatalf("expected *SchemaError, got %T", err)
	}
}

func TestValidateSchema_RequiresID(t *testing.T) {
	s := schemapb.NewSchema(schemapb.Fields(schemapb.Field("x", schemapb.Bool())))
	if !has(msgs(schemapb.ValidateSchema(s)), "id") {
		t.Fatal("expected id-required error")
	}
	if _, err := schemapb.NewValidator(s); err == nil {
		t.Fatal("NewValidator accepted schema without id")
	}
}

// ---------------------------------------------------------------------------
// per-kind value validation
// ---------------------------------------------------------------------------

func TestValidateNumeric(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("n", schemapb.Int32(schemapb.Int32Gte(0), schemapb.Int32Lte(10), schemapb.Int32MultipleOf(2))),
		schemapb.Field("f", schemapb.Float(schemapb.FloatGt(0))),
	)))
	if g := validateJSON(t, v, `{"n":4,"f":0.5}`); len(g) != 0 {
		t.Errorf("want valid, got %v", g)
	}
	if g := validateJSON(t, v, `{"n":5,"f":0}`); !has(g, "n") || !has(g, "f") {
		t.Errorf("want n(multiple) + f(gt), got %v", g)
	}
	if g := validateJSON(t, v, `{"n":-2,"f":1}`); !has(g, "n") { // below gte
		t.Errorf("want n gte error, got %v", g)
	}
	if g := validateJSON(t, v, `{"n":3.5,"f":1}`); !has(g, "n") { // not integer + not multiple
		t.Errorf("want n integer error, got %v", g)
	}
	if g := validateJSON(t, v, `{"n":"x","f":1}`); !has(g, "n") { // wrong type
		t.Errorf("want n type error, got %v", g)
	}
}

func TestValidateString(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("s", schemapb.String(schemapb.StringMinLen(2), schemapb.StringMaxLen(5), schemapb.StringPattern(`^[a-z]+$`))),
	)))
	if g := validateJSON(t, v, `{"s":"abc"}`); len(g) != 0 {
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{"s":"A"}`); !has(g, "s") { // pattern + minlen
		t.Errorf("want s error, got %v", g)
	}
	if g := validateJSON(t, v, `{"s":"abcdef"}`); !has(g, "s") { // maxlen
		t.Errorf("want s maxlen, got %v", g)
	}
}

func TestValidateBoolEnum(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("b", schemapb.Bool(schemapb.BoolConst(true))),
		schemapb.Field("e", schemapb.Enum(
			schemapb.EnumValues(map[int32]string{1: "a", 2: "b", 3: "c"}),
			schemapb.EnumDefinedOnly(), schemapb.EnumIn(1, 2),
		)),
	)))
	if g := validateJSON(t, v, `{"b":true,"e":1}`); len(g) != 0 {
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{"b":false,"e":3}`); !has(g, "b") || !has(g, "e") { // const + not in
		t.Errorf("want b+e, got %v", g)
	}
	if g := validateJSON(t, v, `{"b":true,"e":9}`); !has(g, "e") { // undefined
		t.Errorf("want e defined error, got %v", g)
	}
}

func TestValidateDurationTimestamp(t *testing.T) {
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("d", schemapb.Duration(schemapb.DurationGte(time.Second), schemapb.DurationLte(time.Minute))),
		schemapb.Field("t", schemapb.Timestamp(schemapb.TimestampGte(t0))),
	)))
	if g := validateJSON(t, v, `{"d":"30s","t":"2021-01-01T00:00:00Z"}`); len(g) != 0 {
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{"d":"90s","t":"2019-01-01T00:00:00Z"}`); !has(g, "d") || !has(g, "t") {
		t.Errorf("want d(lte)+t(gte), got %v", g)
	}
	if g := validateJSON(t, v, `{"d":"nope","t":"nope"}`); !has(g, "d") || !has(g, "t") {
		t.Errorf("want parse errors, got %v", g)
	}
}

func TestValidateList(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("tags", schemapb.List(
			schemapb.ListMinItems(1), schemapb.ListMaxItems(2), schemapb.ListUnique(),
			schemapb.ListItems(schemapb.Field("tag", schemapb.String(schemapb.StringMinLen(1)))),
		)),
	)))
	if g := validateJSON(t, v, `{"tags":["a","b"]}`); len(g) != 0 {
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{"tags":[]}`); !has(g, "tags") { // min
		t.Errorf("want min items, got %v", g)
	}
	if g := validateJSON(t, v, `{"tags":["a","b","c"]}`); !has(g, "tags") { // max
		t.Errorf("want max items, got %v", g)
	}
	if g := validateJSON(t, v, `{"tags":["a","a"]}`); !has(g, "tags[1]") { // unique
		t.Errorf("want unique, got %v", g)
	}
	if g := validateJSON(t, v, `{"tags":[""]}`); !has(g, "tags[0]") { // item rule
		t.Errorf("want item minlen, got %v", g)
	}
}

func TestValidateObject(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("addr", schemapb.Object(schemapb.NewSchema(schemapb.Fields(
			schemapb.Field("zip", schemapb.String(schemapb.StringLen(5)), schemapb.FieldRequired()),
		)))),
	)))
	if g := validateJSON(t, v, `{"addr":{"zip":"12345"}}`); len(g) != 0 {
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{"addr":{"zip":"12"}}`); !has(g, "addr.zip") {
		t.Errorf("want addr.zip len, got %v", g)
	}
	if g := validateJSON(t, v, `{"addr":{}}`); g["addr.zip"] != "required" {
		t.Errorf("want addr.zip required, got %v", g)
	}
}

func TestNullableRequired(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("req", schemapb.String(), schemapb.FieldRequired()),
		schemapb.Field("opt", schemapb.String(), schemapb.FieldNullable()),
		schemapb.Field("nn", schemapb.String()),
	)))
	if g := validateJSON(t, v, `{"req":"x","opt":null}`); len(g) != 0 { // nullable null ok
		t.Errorf("valid: %v", g)
	}
	if g := validateJSON(t, v, `{}`); g["req"] != "required" {
		t.Errorf("want req required, got %v", g)
	}
	if g := validateJSON(t, v, `{"req":"x","nn":null}`); g["nn"] != "must not be null" {
		t.Errorf("want nn not-null, got %v", g)
	}
}

// ---------------------------------------------------------------------------
// rules (field + form-wide) and severity
// ---------------------------------------------------------------------------

func TestRules(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(),
		schemapb.Fields(
			schemapb.Field("a", schemapb.Int32()),
			schemapb.Field("b", schemapb.Int32()),
			schemapb.Field("w", schemapb.Int32(), schemapb.FieldRules(
				schemapb.NewRule("this <= 100", "soft cap", schemapb.RuleID("cap"), schemapb.RuleSeverity(schemapb.SeverityWarning)),
			)),
		),
		schemapb.Rules(schemapb.NewRule("root.a < root.b", "a<b", schemapb.RuleID("ab"))),
	))
	// form-wide rule fails
	errs, _ := v.ValidateJSON(json.RawMessage(`{"a":5,"b":1,"w":1}`))
	if msgs(errs)["ab"] != "a<b" {
		t.Errorf("want form rule ab, got %v", msgs(errs))
	}
	// warning rule fires with WARNING severity, valid path otherwise
	errs, _ = v.ValidateJSON(json.RawMessage(`{"a":1,"b":2,"w":200}`))
	var sawWarn bool
	for _, e := range errs {
		if e.GetRuleId() == "cap" && e.GetSeverity() == schemapb.Schema_Filed_WARNING {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Errorf("want WARNING cap rule, got %v", msgs(errs))
	}
}

// ---------------------------------------------------------------------------
// entry points: Struct, JSON, one-shot, FieldError
// ---------------------------------------------------------------------------

func TestValidateStruct_AndOneShot(t *testing.T) {
	s := schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("age", schemapb.Int32(schemapb.Int32Gte(0)), schemapb.FieldRequired()),
	))
	st, _ := structpb.NewStruct(map[string]any{"age": 30})
	if errs, err := schemapb.ValidateStruct(s, st); err != nil || len(errs) != 0 {
		t.Fatalf("one-shot struct: errs=%v err=%v", msgs(errs), err)
	}
	if errs, err := schemapb.ValidateJSON(s, json.RawMessage(`{}`)); err != nil || errs[0].GetField() != "age" {
		t.Fatalf("one-shot json: errs=%v err=%v", msgs(errs), err)
	}
}

func TestValidateJSON_ParseError(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema(id(), schemapb.Fields(schemapb.Field("x", schemapb.Bool())))) //nolint
	if _, err := v.ValidateJSON(json.RawMessage(`not json`)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFieldError(t *testing.T) {
	e := schemapb.NewFieldError("age", "too low",
		schemapb.FieldErrorRuleID("adult"),
		schemapb.FieldErrorSeverity(schemapb.SeverityWarning),
	)
	if e.GetField() != "age" || e.GetRuleId() != "adult" || e.GetSeverity() != schemapb.Schema_Filed_WARNING {
		t.Errorf("field error: %v", e)
	}
}

// ---------------------------------------------------------------------------
// computed (derived) values + conditional disk scenario
// ---------------------------------------------------------------------------

// diskSchema models disk IOPS/bandwidth derived from size + type, plus the
// conditional size bounds. Reused by several tests.
func diskSchema() *schemapb.Schema {
	return schemapb.NewSchema(
		schemapb.Identity("infra", "disk", "v1"),
		schemapb.Fields(
			schemapb.Field("disk_type",
				schemapb.Enum(schemapb.EnumValues(map[int32]string{1: "ssd", 2: "hdd"}), schemapb.EnumDefinedOnly()),
				schemapb.FieldRequired(),
			),
			schemapb.Field("disk_size", schemapb.Int32(schemapb.Int32Gte(1)), schemapb.FieldRequired(),
				schemapb.FieldRules(
					schemapb.NewRule(
						`int(root.disk_type) != 1 || (int(this) >= 20 && int(this) <= 8192)`,
						"type 1: 20–8192 GB", schemapb.RuleID("disk1_range")),
					schemapb.NewRule(
						`int(root.disk_type) != 2 || (int(this) >= 93 && int(this) <= 262074 && int(this) % 93 == 0)`,
						"type 2: 93–262074 GB, multiple of 93", schemapb.RuleID("disk2_range")),
				),
			),
			schemapb.Field("iops", schemapb.Computed(
				`root.disk_type == 1 ? min(max(root.disk_size * 50, 3000), 16000) : min(max(root.disk_size * 5, 100), 3000)`,
				schemapb.ComputedResult(schemapb.ResultInt64),
			)),
			schemapb.Field("bandwidth_mbps", schemapb.Computed(
				`root.disk_type == 1 ? min(max(root.disk_size / 4, 125), 1000) : min(max(root.disk_size / 10, 40), 500)`,
				schemapb.ComputedResult(schemapb.ResultInt64),
			)),
			schemapb.Field("score", schemapb.Computed(
				`root.iops + root.bandwidth_mbps * 10`, schemapb.ComputedResult(schemapb.ResultInt64),
			)),
		),
	)
}

func TestCompute(t *testing.T) {
	v := mustValidator(t, diskSchema())

	cases := []struct {
		typ, size              int
		iops, bandwidth, score float64
	}{
		{1, 100, 5000, 125, 5000 + 125*10},
		{1, 1000, 16000, 250, 16000 + 250*10},
		{2, 1000, 3000, 100, 3000 + 100*10},
		{2, 10, 100, 40, 100 + 40*10},
	}
	for _, c := range cases {
		body := []byte(`{"disk_type":` + strconv.Itoa(c.typ) + `,"disk_size":` + strconv.Itoa(c.size) + `}`)
		out, errs, err := v.ComputeJSON(body)
		if err != nil || len(errs) != 0 {
			t.Fatalf("compute %v: errs=%v err=%v", c, msgs(errs), err)
		}
		if out["iops"] != c.iops || out["bandwidth_mbps"] != c.bandwidth || out["score"] != c.score {
			t.Errorf("type %d size %d => %v (want iops=%v bw=%v score=%v)", c.typ, c.size, out, c.iops, c.bandwidth, c.score)
		}
	}

	// ComputeStruct path
	st, _ := structpb.NewStruct(map[string]any{"disk_type": 1, "disk_size": 100})
	out, errs := v.ComputeStruct(st)
	if len(errs) != 0 || out["iops"] != float64(5000) {
		t.Errorf("ComputeStruct: %v %v", out, msgs(errs))
	}
}

func TestComputeCycleRejected(t *testing.T) {
	s := schemapb.NewSchema(schemapb.Identity("x", "cyc", "v1"), schemapb.Fields(
		schemapb.Field("a", schemapb.Computed(`root.b + 1`)),
		schemapb.Field("b", schemapb.Computed(`root.a + 1`)),
	))
	if _, err := schemapb.NewValidator(s); err == nil {
		t.Fatal("expected cycle rejection")
	}
}

func TestDiskConditional(t *testing.T) {
	v := mustValidator(t, diskSchema())
	cases := []struct {
		typ, size int
		ok        bool
		rule      string
	}{
		{1, 20, true, ""},
		{1, 8192, true, ""},
		{1, 19, false, "disk1_range"},
		{1, 8193, false, "disk1_range"},
		{2, 93, true, ""},
		{2, 186, true, ""},
		{2, 262074, true, ""},
		{2, 100, false, "disk2_range"},
		{2, 92, false, "disk2_range"},
	}
	for _, c := range cases {
		body := json.RawMessage(`{"disk_type":` + strconv.Itoa(c.typ) + `,"disk_size":` + strconv.Itoa(c.size) + `}`)
		errs, _ := v.ValidateJSON(body)
		if c.ok {
			if len(errs) != 0 {
				t.Errorf("type %d size %d: want valid, got %v", c.typ, c.size, msgs(errs))
			}
			continue
		}
		var hit bool
		for _, e := range errs {
			if e.GetRuleId() == c.rule {
				hit = true
			}
		}
		if !hit {
			t.Errorf("type %d size %d: want rule %s, got %v", c.typ, c.size, c.rule, msgs(errs))
		}
	}
}

// ---------------------------------------------------------------------------
// SchemaService server
// ---------------------------------------------------------------------------

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	st, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func refID(id *schemapb.SchemaIdentity) *schemapb.SchemaRef {
	return &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Id{Id: id}}
}
func refInline(s *schemapb.Schema) *schemapb.SchemaRef {
	return &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: s}}
}

func TestServer_RegisterGetList(t *testing.T) {
	srv := schemapb.NewServer(schemapb.DefaultConfig())
	ctx := context.Background()

	reg, err := srv.RegisterSchema(ctx, diskSchema())
	if err != nil || !reg.GetValid() {
		t.Fatalf("register: %v err=%v", reg, err)
	}
	got, err := srv.GetSchema(ctx, reg.GetId())
	if err != nil || got.GetId().GetName() != "disk" {
		t.Fatalf("get: %v err=%v", got, err)
	}

	other := schemapb.NewSchema(schemapb.Identity("infra", "net", "v1"), schemapb.Fields(schemapb.Field("x", schemapb.Bool())))
	if _, err := srv.RegisterSchema(ctx, other); err != nil {
		t.Fatal(err)
	}

	all, _ := srv.ListSchemas(ctx, &schemapb.Filter{})
	if len(all.GetSchemas()) != 2 {
		t.Errorf("list all = %d", len(all.GetSchemas()))
	}
	res, _ := srv.ListSchemas(ctx, &schemapb.Filter{NameContains: schemapb.Ptr("dis")})
	if len(res.GetSchemas()) != 1 || res.GetSchemas()[0].GetId().GetName() != "disk" {
		t.Errorf("filtered list: %v", res.GetSchemas())
	}
}

func TestServer_RegisterInvalid(t *testing.T) {
	srv := schemapb.NewServer(schemapb.DefaultConfig())
	bad := schemapb.NewSchema(schemapb.Fields(schemapb.Field("x", schemapb.Bool()))) // no identity
	resp, err := srv.RegisterSchema(context.Background(), bad)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetValid() || len(resp.GetErrors()) == 0 {
		t.Errorf("want invalid+errors, got %v", resp)
	}
}

func TestServer_ValidateCompute(t *testing.T) {
	srv := schemapb.NewServer(schemapb.DefaultConfig())
	ctx := context.Background()
	reg, _ := srv.RegisterSchema(ctx, diskSchema())

	ok, _ := srv.Validate(ctx, &schemapb.ValidateRequest{
		Schema: refID(reg.GetId()),
		Values: mustStruct(t, map[string]any{"disk_type": 1, "disk_size": 100}),
	})
	if !ok.GetValid() {
		t.Errorf("want valid, got %v", msgs(ok.GetErrors()))
	}

	bad, _ := srv.Validate(ctx, &schemapb.ValidateRequest{
		Schema: refInline(diskSchema()),
		Values: mustStruct(t, map[string]any{"disk_type": 3, "disk_size": 0}),
	})
	if bad.GetValid() {
		t.Error("want invalid (inline)")
	}

	comp, err := srv.Compute(ctx, &schemapb.ComputeRequest{
		Schema: refID(reg.GetId()),
		Values: mustStruct(t, map[string]any{"disk_type": 1, "disk_size": 100}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if comp.GetValues().GetFields()["iops"].GetNumberValue() != 5000 {
		t.Errorf("iops = %v", comp.GetValues().AsMap()["iops"])
	}
}

func TestServer_Policies(t *testing.T) {
	ctx := context.Background()
	locked := schemapb.NewServer(schemapb.Config{}) // zero config: nothing allowed

	if _, err := locked.RegisterSchema(ctx, diskSchema()); status.Code(err) != codes.PermissionDenied {
		t.Errorf("register disabled: want PermissionDenied, got %v", err)
	}
	if _, err := locked.Validate(ctx, &schemapb.ValidateRequest{Schema: refInline(diskSchema()), Values: mustStruct(t, map[string]any{})}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("inline disabled: want PermissionDenied, got %v", err)
	}
	if _, err := locked.GetSchema(ctx, &schemapb.SchemaIdentity{Namespace: "no", Name: "thing", Version: "v1"}); status.Code(err) != codes.NotFound {
		t.Errorf("get missing: want NotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolve: defaults + full resolved state + nested computed
// ---------------------------------------------------------------------------

func TestComputeDefaultsAndFullState(t *testing.T) {
	// disk_size defaults to 20, disk_type to 1; iops derived.
	s := schemapb.NewSchema(id(),
		schemapb.Fields(
			schemapb.Field("disk_type", schemapb.Int32(schemapb.Int32Default(1))),
			schemapb.Field("disk_size", schemapb.Int32(schemapb.Int32Default(20))),
			schemapb.Field("iops", schemapb.Computed(
				`root.disk_type == 1 ? root.disk_size * 50 : root.disk_size * 5`,
				schemapb.ComputedResult(schemapb.ResultInt64))),
		),
	)
	v := mustValidator(t, s)

	// no input at all -> defaults seed inputs, derived computed; full state out
	out, errs := v.Compute(map[string]any{})
	if len(errs) != 0 {
		t.Fatalf("compute({}): %v", msgs(errs))
	}
	if out["disk_type"] != float64(1) || out["disk_size"] != float64(20) {
		t.Errorf("defaults not seeded: %v", out)
	}
	if out["iops"] != float64(1000) { // 20*50
		t.Errorf("iops = %v, want 1000", out["iops"])
	}

	// partial input overrides default; output is the whole form
	out2, _ := v.Compute(map[string]any{"disk_size": float64(100)})
	if out2["disk_type"] != float64(1) || out2["disk_size"] != float64(100) || out2["iops"] != float64(5000) {
		t.Errorf("full state = %v", out2)
	}
}

func TestComputeNested(t *testing.T) {
	// computed inside a nested object, reading top-level via root path
	s := schemapb.NewSchema(id(),
		schemapb.Fields(
			schemapb.Field("base", schemapb.Int32()),
			schemapb.Field("box", schemapb.Object(schemapb.NewSchema(schemapb.Fields(
				schemapb.Field("factor", schemapb.Int32(schemapb.Int32Default(3))),
				schemapb.Field("total", schemapb.Computed(
					`root.base * root.box.factor`, schemapb.ComputedResult(schemapb.ResultInt64))),
			)))),
		),
	)
	v := mustValidator(t, s)
	out, errs := v.Compute(map[string]any{"base": float64(10), "box": map[string]any{}})
	if len(errs) != 0 {
		t.Fatalf("nested compute: %v", msgs(errs))
	}
	box, _ := out["box"].(map[string]any)
	if box["factor"] != float64(3) { // default seeded in nested object
		t.Errorf("nested default: %v", box)
	}
	if box["total"] != float64(30) { // 10 * 3, computed in nested scope
		t.Errorf("nested computed total = %v, want 30", box["total"])
	}
}

func TestImmutable(t *testing.T) {
	// system value: immutable + default. group/unit are informative only.
	s := schemapb.NewSchema(id(), schemapb.Fields(
		schemapb.Field("block_size", schemapb.Int32(schemapb.Int32Default(8192)),
			schemapb.FieldImmutable(), schemapb.FieldGroup("system"), schemapb.FieldUnit("B")),
		schemapb.Field("work_mem", schemapb.Int32(schemapb.Int32Default(4)), schemapb.FieldUnit("MB")),
	))
	v := mustValidator(t, s)

	// Compute forces the immutable field to its default even if an input is sent.
	out, _ := v.Compute(map[string]any{"block_size": float64(16384)})
	if out["block_size"] != float64(8192) {
		t.Errorf("immutable not forced: %v", out["block_size"])
	}
	if out["work_mem"] != float64(4) { // default seeded
		t.Errorf("work_mem default: %v", out["work_mem"])
	}

	// Validate rejects an attempt to change the immutable field.
	if g := validateJSON(t, v, `{"block_size":16384}`); g["block_size"] != "immutable: cannot be changed" {
		t.Errorf("want immutable error, got %v", g)
	}
	// Absent (or equal) immutable field is fine.
	if g := validateJSON(t, v, `{}`); len(g) != 0 {
		t.Errorf("want valid, got %v", g)
	}

	// group/unit are carried, engine ignores them.
	if s.GetFields()[0].GetGroup() != "system" || s.GetFields()[0].GetUnit() != "B" {
		t.Errorf("group/unit not set")
	}
}
