package schemapb_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

func has(m map[string]string, field string) bool { _, ok := m[field]; return ok }

func mustValidator(t *testing.T, s *schemapb.Schema) *schemapb.Schema {
	t.Helper()
	if errs := s.IsValid(); len(errs) != 0 {
		t.Fatalf("invalid schema: %v", msgs(errs))
	}
	return s
}

// build makes a validator for a test schema with the given fields.
func build(t *testing.T, fields ...schemapb.FieldDef) *schemapb.Schema {
	t.Helper()
	return mustValidator(t, schemapb.NewSchema("test", "s", "v1").Fields(fields...).MustBuild())
}

// validateJSON validates body and returns field->message.
func validateJSON(t *testing.T, v *schemapb.Schema, body string) map[string]string {
	t.Helper()
	errs, err := v.ValidateJSON(json.RawMessage(body))
	if err != nil {
		t.Fatalf("ValidateJSON(%s): %v", body, err)
	}
	return msgs(errs)
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	st, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// ---------------------------------------------------------------------------
// builders
// ---------------------------------------------------------------------------

func TestPtr(t *testing.T) {
	if *schemapb.Ptr(7) != 7 || *schemapb.Ptr("x") != "x" {
		t.Fatal("Ptr")
	}
}

func TestBuilders_AllKinds(t *testing.T) {
	s := schemapb.NewSchema("test", "s", "v1").
		Descr("kitchen sink").
		Fields(
			schemapb.Float("f").Gte(0).Lte(1),
			schemapb.Double("d").MultipleOf(0.5),
			schemapb.Int32("i32").In(1, 2, 3),
			schemapb.Int64("i64").Gt(0),
			schemapb.UInt32("u32").Lte(9),
			schemapb.UInt64("u64").Const(4),
			schemapb.Bool("b").Const(true),
			schemapb.Str("s").MinLen(1).Desc("name"),
			schemapb.Enum("e").Values(map[int32]string{1: "a"}).DefinedOnly(),
			schemapb.Duration("dur").Lte(time.Minute),
			schemapb.Timestamp("ts").Gte(time.Unix(0, 0)),
			schemapb.List("list", schemapb.Str("it")).MinItems(1),
			schemapb.Object("obj", schemapb.Bool("inner")),
		).
		MustBuild()

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
	if errs := diskSchema().IsValid(); len(errs) != 0 {
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
	errs := bad.IsValid()
	if len(errs) == 0 {
		t.Fatal("expected schema errors")
	}
}

func TestValidateSchema_RequiresID(t *testing.T) {
	s := &schemapb.Schema{Fields: []*schemapb.Schema_Filed{schemapb.Bool("x").Done()}} // no id
	if !has(msgs(s.IsValid()), "id") {
		t.Fatal("expected id-required error")
	}
}

// ---------------------------------------------------------------------------
// per-kind value validation
// ---------------------------------------------------------------------------

func TestValidateNumeric(t *testing.T) {
	v := build(t,
		schemapb.Int32("n").Gte(0).Lte(10).MultipleOf(2),
		schemapb.Float("f").Gt(0),
	)
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
	v := build(t, schemapb.Str("s").MinLen(2).MaxLen(5).Pattern(`^[a-z]+$`))
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
	v := build(t,
		schemapb.Bool("b").Const(true),
		schemapb.Enum("e").Values(map[int32]string{1: "a", 2: "b", 3: "c"}).DefinedOnly().In(1, 2),
	)
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
	v := build(t,
		schemapb.Duration("d").Gte(time.Second).Lte(time.Minute),
		schemapb.Timestamp("t").Gte(t0),
	)
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
	v := build(t, schemapb.List("tags", schemapb.Str("tag").MinLen(1)).
		MinItems(1).MaxItems(2).Unique())
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
	v := build(t, schemapb.Object("addr", schemapb.Str("zip").Len(5).Required()))
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
	v := build(t,
		schemapb.Str("req").Required(),
		schemapb.Str("opt").Nullable(),
		schemapb.Str("nn"),
	)
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
	v := mustValidator(t, schemapb.NewSchema("test", "s", "v1").
		Fields(
			schemapb.Int32("a"),
			schemapb.Int32("b"),
			schemapb.Int32("w").Rules(schemapb.Rule("this <= 100", "soft cap").ID("cap").Warn()),
		).
		Rules(schemapb.Rule("root.a < root.b", "a<b").ID("ab")).
		MustBuild())

	// form-wide rule fails
	errs, _ := v.ValidateJSON(json.RawMessage(`{"a":5,"b":1,"w":1}`))
	if msgs(errs)["ab"] != "a<b" {
		t.Errorf("want form rule ab, got %v", msgs(errs))
	}
	// warning rule fires with WARNING severity, valid path otherwise
	errs, _ = v.ValidateJSON(json.RawMessage(`{"a":1,"b":2,"w":200}`))
	var sawWarn bool
	for _, e := range errs {
		if e.GetRuleId() == "cap" && e.GetSeverity() == schemapb.SeverityWarning {
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
	s := schemapb.NewSchema("test", "s", "v1").
		Fields(schemapb.Int32("age").Gte(0).Required()).
		MustBuild()
	st := mustStruct(t, map[string]any{"age": 30})
	if errs := s.ValidateStruct(st); len(errs) != 0 {
		t.Fatalf("struct: errs=%v", msgs(errs))
	}
	errs, err := s.ValidateJSON(json.RawMessage(`{}`))
	if err != nil || errs[0].GetField() != "age" {
		t.Fatalf("json: errs=%v err=%v", msgs(errs), err)
	}
}

func TestValidateJSON_ParseError(t *testing.T) {
	v := build(t, schemapb.Bool("x"))
	if _, err := v.ValidateJSON(json.RawMessage(`not json`)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFieldError(t *testing.T) {
	e := schemapb.NewFieldError("age", "too low")
	if e.GetField() != "age" || e.GetMessage() != "too low" || e.GetSeverity() != schemapb.SeverityError {
		t.Errorf("field error: %v", e)
	}
}

// ---------------------------------------------------------------------------
// computed (derived) values + conditional disk scenario
// ---------------------------------------------------------------------------

// diskSchema models disk IOPS/bandwidth derived from size + type, plus the
// conditional size bounds. Reused by several tests.
func diskSchema() *schemapb.Schema {
	return schemapb.NewSchema("infra", "disk", "v1").
		Fields(
			schemapb.Enum("disk_type").Values(map[int32]string{1: "ssd", 2: "hdd"}).DefinedOnly().Required(),
			schemapb.Int32("disk_size").Gte(1).Required().Rules(
				schemapb.Rule(`int(root.disk_type) != 1 || (int(this) >= 20 && int(this) <= 8192)`,
					"type 1: 20–8192 GB").ID("disk1_range"),
				schemapb.Rule(`int(root.disk_type) != 2 || (int(this) >= 93 && int(this) <= 262074 && int(this) % 93 == 0)`,
					"type 2: 93–262074 GB, multiple of 93").ID("disk2_range"),
			),
			schemapb.Computed("iops",
				`root.disk_type == 1 ? min(max(root.disk_size * 50, 3000), 16000) : min(max(root.disk_size * 5, 100), 3000)`).
				Result(schemapb.ResultInt64),
			schemapb.Computed("bandwidth_mbps",
				`root.disk_type == 1 ? min(max(root.disk_size / 4, 125), 1000) : min(max(root.disk_size / 10, 40), 500)`).
				Result(schemapb.ResultInt64),
			schemapb.Computed("score", `root.iops + root.bandwidth_mbps * 10`).Result(schemapb.ResultInt64),
		).
		MustBuild()
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
	out, errs := v.ComputeStruct(mustStruct(t, map[string]any{"disk_type": 1, "disk_size": 100}))
	if len(errs) != 0 || out["iops"] != float64(5000) {
		t.Errorf("ComputeStruct: %v %v", out, msgs(errs))
	}
}

func TestComputeCycleRejected(t *testing.T) {
	_, err := schemapb.NewSchema("x", "cyc", "v1").
		Fields(
			schemapb.Computed("a", "root.b + 1"),
			schemapb.Computed("b", "root.a + 1"),
		).
		Build()
	if err == nil {
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

func TestComputeDefaultsAndFullState(t *testing.T) {
	v := build(t,
		schemapb.Int32("disk_type").Default(1),
		schemapb.Int32("disk_size").Default(20),
		schemapb.Computed("iops", `root.disk_type == 1 ? root.disk_size * 50 : root.disk_size * 5`).Result(schemapb.ResultInt64),
	)

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
	v := build(t,
		schemapb.Int32("base"),
		schemapb.Object("box",
			schemapb.Int32("factor").Default(3),
			schemapb.Computed("total", `root.base * root.box.factor`).Result(schemapb.ResultInt64),
		),
	)
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

// ---------------------------------------------------------------------------
// SchemaService server
// ---------------------------------------------------------------------------

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

	other := schemapb.NewSchema("infra", "net", "v1").Fields(schemapb.Bool("x")).MustBuild()
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
	bad := &schemapb.Schema{Fields: []*schemapb.Schema_Filed{schemapb.Bool("x").Done()}} // no identity
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

	ok, _ := srv.Validate(ctx, &schemapb.Filled{
		Schema: refID(reg.GetId()),
		Values: mustStruct(t, map[string]any{"disk_type": 1, "disk_size": 100}),
	})
	if !ok.GetValid() {
		t.Errorf("want valid, got %v", msgs(ok.GetErrors()))
	}

	bad, _ := srv.Validate(ctx, &schemapb.Filled{
		Schema: refInline(diskSchema()),
		Values: mustStruct(t, map[string]any{"disk_type": 3, "disk_size": 0}),
	})
	if bad.GetValid() {
		t.Error("want invalid (inline)")
	}

	comp, err := srv.Compute(ctx, &schemapb.Filled{
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
	if _, err := locked.Validate(ctx, &schemapb.Filled{Schema: refInline(diskSchema()), Values: mustStruct(t, map[string]any{})}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("inline disabled: want PermissionDenied, got %v", err)
	}
	if _, err := locked.GetSchema(ctx, &schemapb.SchemaIdentity{Namespace: "no", Name: "thing", Version: "v1"}); status.Code(err) != codes.NotFound {
		t.Errorf("get missing: want NotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// immutable + end-to-end postgresql.conf slice
// ---------------------------------------------------------------------------

func TestImmutable(t *testing.T) {
	s := schemapb.NewSchema("test", "s", "v1").Fields(
		schemapb.Int32("block_size").Default(8192).Immutable().Group("system").Unit("B"),
		schemapb.Int32("work_mem").Default(4).Unit("MB"),
	).MustBuild()
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
	if g := validateJSON(t, v, `{}`); len(g) != 0 {
		t.Errorf("want valid, got %v", g)
	}

	if s.GetFields()[0].GetGroup() != "system" || s.GetFields()[0].GetUnit() != "B" {
		t.Errorf("group/unit not set")
	}
}

// pgSchema models a few postgresql.conf settings via the chain API.
func pgSchema() *schemapb.Schema {
	return schemapb.NewSchema("pg", "postgresql", "16").
		Descr("postgresql.conf (slice)").
		Fields(
			schemapb.Int32("block_size").Default(8192).Immutable().
				Group("Preset Options").Unit("B").Desc("compile-time block size (fixed)"),
			schemapb.Int32("max_connections").Gte(1).Lte(262143).Default(100).
				Group("Connections").Desc("max concurrent connections"),
			schemapb.Int64("shared_buffers").Gte(16).Default(128).
				Group("Resource Usage").Unit("MB").Desc("shared memory for buffers"),
			schemapb.Int64("work_mem").Gte(1).Default(4).Group("Resource Usage").Unit("MB"),
			schemapb.Computed("effective_cache_size", "root.shared_buffers * 3").Result(schemapb.ResultInt64).
				Group("Resource Usage").Unit("MB").Desc("planner cache estimate"),
			schemapb.Str("wal_level").In("minimal", "replica", "logical").Default("replica").Group("WAL"),
		).
		Rules(schemapb.Rule("root.work_mem * root.max_connections <= 4096",
			"work_mem * max_connections exceeds the 4096 MB budget").ID("mem_budget")).
		MustBuild()
}

func TestPostgresConf(t *testing.T) {
	v := mustValidator(t, pgSchema())

	// Resolve from nothing: defaults seeded + derived computed.
	resolved, errs := v.Compute(map[string]any{})
	if len(errs) != 0 {
		t.Fatalf("compute: %v", msgs(errs))
	}
	want := map[string]float64{"block_size": 8192, "max_connections": 100, "shared_buffers": 128, "work_mem": 4, "effective_cache_size": 384}
	for k, w := range want {
		if resolved[k] != w {
			t.Errorf("%s = %v, want %v", k, resolved[k], w)
		}
	}
	if resolved["wal_level"] != "replica" {
		t.Errorf("wal_level = %v", resolved["wal_level"])
	}

	// Validation failures: immutable change, bad enum, blown memory budget.
	if g := validateJSON(t, v, `{"block_size":4096}`); g["block_size"] != "immutable: cannot be changed" {
		t.Errorf("block_size: %v", g)
	}
	if g := validateJSON(t, v, `{"wal_level":"bogus"}`); !has(g, "wal_level") {
		t.Errorf("wal_level enum: %v", g)
	}
	if g := validateJSON(t, v, `{"work_mem":50,"max_connections":100}`); g["mem_budget"] != "work_mem * max_connections exceeds the 4096 MB budget" {
		t.Errorf("budget rule: %v", g)
	}

	// Render the resolved values back into a postgresql.conf slice.
	conf := renderConf(pgSchema(), resolved)
	t.Logf("rendered:\n%s", conf)
	for _, line := range []string{
		"block_size = 8192",
		"max_connections = 100",
		"shared_buffers = 128MB",
		"effective_cache_size = 384MB", // derived
		"wal_level = replica",
	} {
		if !strings.Contains(conf, line) {
			t.Errorf("rendered conf missing %q", line)
		}
	}
}

// renderConf serialises resolved values into postgresql.conf text using the
// schema for comments, grouping and units.
func renderConf(s *schemapb.Schema, vals map[string]any) string {
	var b strings.Builder
	group := ""
	for _, f := range s.GetFields() {
		v, ok := vals[f.GetName()]
		if !ok || v == nil {
			continue
		}
		if g := f.GetGroup(); g != group {
			b.WriteString("\n# === " + g + " ===\n")
			group = g
		}
		if d := f.GetDescription(); d != "" {
			b.WriteString("# " + d + "\n")
		}
		b.WriteString(f.GetName() + " = " + confValue(f, v) + "\n")
	}
	return b.String()
}

func confValue(f *schemapb.Schema_Filed, v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "on"
		}
		return "off"
	case float64:
		s := strconv.FormatInt(int64(x), 10)
		if u := f.GetUnit(); u != "" && u != "B" { // 128 -> 128MB
			return s + u
		}
		return s
	case string:
		return x
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Hash, Bake, Merge
// ---------------------------------------------------------------------------

func TestHash(t *testing.T) {
	a := schemapb.NewSchema("x", "a", "v1").Fields(schemapb.Bool("b")).MustBuild()
	a2 := schemapb.NewSchema("x", "a", "v1").Fields(schemapb.Bool("b")).MustBuild()
	c := schemapb.NewSchema("x", "a", "v1").Fields(schemapb.Bool("c")).MustBuild()
	if schemapb.Hash(a) != schemapb.Hash(a2) {
		t.Error("equal schemas hash differently")
	}
	if schemapb.Hash(a) == schemapb.Hash(c) {
		t.Error("different schemas hash equally")
	}
}

func TestBakeMerge(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int32("disk_type").Default(1),
		schemapb.Int32("disk_size").Default(20).Gte(1),
		schemapb.Int32("block_size").Default(8192).Immutable(),
		schemapb.Computed("iops", "root.disk_size * 50").Result(schemapb.ResultInt64),
	).MustBuild()

	// Bake from nothing: defaults + computed sealed.
	baked, errs := s.Bake(map[string]any{})
	if len(errs) != 0 || baked == nil {
		t.Fatalf("bake: %v", msgs(errs))
	}
	bv := baked.GetValues().AsMap()
	if bv["disk_size"] != float64(20) || bv["iops"] != float64(1000) || bv["block_size"] != float64(8192) {
		t.Errorf("baked values: %v", bv)
	}
	if !baked.Matches(s) {
		t.Error("Matches returned false for the baking schema")
	}

	// Merge override: re-bake, iops recomputed, immutable kept.
	merged, errs := baked.Merge(mustStruct(t, map[string]any{"disk_size": 100}), false)
	if len(errs) != 0 || merged == nil {
		t.Fatalf("merge: %v", msgs(errs))
	}
	mv := merged.GetValues().AsMap()
	if mv["disk_size"] != float64(100) || mv["iops"] != float64(5000) || mv["block_size"] != float64(8192) {
		t.Errorf("merged values: %v", mv)
	}

	// Merge that violates a constraint: no baked, errors returned.
	bad := schemapb.NewSchema("t", "x", "v1").Fields(schemapb.Int32("n").Gte(0)).MustBuild()
	b2, _ := bad.Bake(map[string]any{"n": float64(5)})
	if got, e2 := b2.Merge(mustStruct(t, map[string]any{"n": -1}), false); got != nil || len(e2) == 0 {
		t.Errorf("want merge rejected, got %v / %v", got, msgs(e2))
	}
}

// ---------------------------------------------------------------------------
// Feature tests: string formats, strict, min/max properties, coercion,
// metadata, and error codes
// ---------------------------------------------------------------------------

func TestStringFormat_Email(t *testing.T) {
	v := build(t, schemapb.Str("email").Format(schemapb.FormatEmail))

	if g := validateJSON(t, v, `{"email":"user@example.com"}`); len(g) != 0 {
		t.Errorf("valid email rejected: %v", g)
	}
	if g := validateJSON(t, v, `{"email":"not-an-email"}`); !has(g, "email") {
		t.Errorf("invalid email accepted: %v", g)
	}
}

func TestStringFormat_URL(t *testing.T) {
	v := build(t, schemapb.Str("u").Format(schemapb.FormatURL))

	if g := validateJSON(t, v, `{"u":"https://example.com/path"}`); len(g) != 0 {
		t.Errorf("valid url rejected: %v", g)
	}
	if g := validateJSON(t, v, `{"u":"not a url"}`); !has(g, "u") {
		t.Errorf("invalid url accepted: %v", g)
	}
}

func TestStringFormat_UUID(t *testing.T) {
	v := build(t, schemapb.Str("uid").Format(schemapb.FormatUUID))

	if g := validateJSON(t, v, `{"uid":"550e8400-e29b-41d4-a716-446655440000"}`); len(g) != 0 {
		t.Errorf("valid uuid rejected: %v", g)
	}
	if g := validateJSON(t, v, `{"uid":"not-a-uuid"}`); !has(g, "uid") {
		t.Errorf("invalid uuid accepted: %v", g)
	}
}

func TestStringFormat_IP(t *testing.T) {
	v4 := build(t, schemapb.Str("ip").Format(schemapb.FormatIPv4))
	if g := validateJSON(t, v4, `{"ip":"192.168.1.1"}`); len(g) != 0 {
		t.Errorf("valid ipv4 rejected: %v", g)
	}
	if g := validateJSON(t, v4, `{"ip":"::1"}`); !has(g, "ip") {
		t.Errorf("ipv6 accepted as ipv4: %v", g)
	}

	v6 := build(t, schemapb.Str("ip").Format(schemapb.FormatIPv6))
	if g := validateJSON(t, v6, `{"ip":"2001:db8::1"}`); len(g) != 0 {
		t.Errorf("valid ipv6 rejected: %v", g)
	}
	if g := validateJSON(t, v6, `{"ip":"1.2.3.4"}`); !has(g, "ip") {
		t.Errorf("ipv4 accepted as ipv6: %v", g)
	}
}

func TestStringFormat_DateTimeDatetime(t *testing.T) {
	vdate := build(t, schemapb.Str("d").Format(schemapb.FormatDate))
	if g := validateJSON(t, vdate, `{"d":"2024-03-15"}`); len(g) != 0 {
		t.Errorf("valid date rejected: %v", g)
	}
	if g := validateJSON(t, vdate, `{"d":"not-a-date"}`); !has(g, "d") {
		t.Errorf("invalid date accepted: %v", g)
	}

	vtime := build(t, schemapb.Str("t").Format(schemapb.FormatTime))
	if g := validateJSON(t, vtime, `{"t":"14:30:00"}`); len(g) != 0 {
		t.Errorf("valid time rejected: %v", g)
	}
	if g := validateJSON(t, vtime, `{"t":"not-a-time"}`); !has(g, "t") {
		t.Errorf("invalid time accepted: %v", g)
	}

	vdt := build(t, schemapb.Str("dt").Format(schemapb.FormatDatetime))
	if g := validateJSON(t, vdt, `{"dt":"2024-03-15T14:30:00Z"}`); len(g) != 0 {
		t.Errorf("valid datetime rejected: %v", g)
	}
	if g := validateJSON(t, vdt, `{"dt":"not-a-datetime"}`); !has(g, "dt") {
		t.Errorf("invalid datetime accepted: %v", g)
	}
}

func TestStrictMode_UnknownField(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema("test", "s", "v1").
		Strict().
		Fields(schemapb.Str("name")).
		MustBuild())

	if g := validateJSON(t, v, `{"name":"alice"}`); len(g) != 0 {
		t.Errorf("valid strict: %v", g)
	}
	if g := validateJSON(t, v, `{"name":"alice","extra":"field"}`); !has(g, "extra") {
		t.Errorf("unknown field not rejected in strict mode: %v", g)
	}
}

func TestMinMaxProperties(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema("test", "s", "v1").
		MinProps(2).
		MaxProps(3).
		Fields(
			schemapb.Str("a"),
			schemapb.Str("b"),
			schemapb.Str("c"),
		).
		MustBuild())

	// exactly 2 — ok
	if g := validateJSON(t, v, `{"a":"x","b":"y"}`); len(g) != 0 {
		t.Errorf("2 props should be valid: %v", g)
	}
	// 1 — too few
	if g := validateJSON(t, v, `{"a":"x"}`); !has(g, "") {
		t.Errorf("1 prop should fail min_properties: %v", g)
	}
	// 4 (unknown in non-strict) — too many; we use 3 declared fields to test max
	if g := validateJSON(t, v, `{"a":"x","b":"y","c":"z"}`); len(g) != 0 {
		t.Errorf("3 props should be valid: %v", g)
	}
}

func TestCoercion(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema("test", "s", "v1").
		Coerce().
		Fields(
			schemapb.Int32("n").Gte(0).Lte(100),
			schemapb.Bool("flag"),
			schemapb.Enum("kind").Values(map[int32]string{1: "a", 2: "b"}).DefinedOnly(),
		).
		MustBuild())

	// string "5" should be coerced to numeric 5 and pass Gte(0).
	if g := validateJSON(t, v, `{"n":"5","flag":"true","kind":"1"}`); len(g) != 0 {
		t.Errorf("coercion: want valid, got %v", g)
	}
	// Unparseable string stays as string -> type error.
	if g := validateJSON(t, v, `{"n":"abc"}`); !has(g, "n") {
		t.Errorf("coercion: unparseable string should still fail: %v", g)
	}
}

func TestMetadataFields(t *testing.T) {
	exampleVal, _ := structpb.NewValue("alice@example.com")
	s := schemapb.NewSchema("test", "meta", "v1").
		Fields(
			schemapb.Str("email").
				Format(schemapb.FormatEmail).
				Title("Email address").
				Deprecated().
				Secret().
				Examples(exampleVal),
		).
		MustBuild()

	f := s.GetFields()[0]
	if f.GetTitle() != "Email address" {
		t.Errorf("title = %q", f.GetTitle())
	}
	if !f.GetDeprecated() {
		t.Error("deprecated not set")
	}
	if !f.GetSecret() {
		t.Error("secret not set")
	}
	if len(f.GetExamples()) != 1 || f.GetExamples()[0].GetStringValue() != "alice@example.com" {
		t.Errorf("examples = %v", f.GetExamples())
	}
}

func TestFieldError_Codes(t *testing.T) {
	v := build(t,
		schemapb.Str("name").Required(),
		schemapb.Int32("age").Gte(0),
		schemapb.Str("email").Format(schemapb.FormatEmail),
	)

	// required -> code "required"
	errs, _ := v.ValidateJSON(json.RawMessage(`{}`))
	var reqErr *schemapb.FieldError
	for _, e := range errs {
		if e.GetField() == "name" {
			reqErr = e
		}
	}
	if reqErr == nil || reqErr.GetCode() != "required" {
		t.Errorf("want code='required', got %v", reqErr)
	}

	// type error -> code "type"
	errs2, _ := v.ValidateJSON(json.RawMessage(`{"name":"x","age":"notanumber"}`))
	var typeErrFound bool
	for _, e := range errs2 {
		if e.GetField() == "age" && e.GetCode() == "type" {
			typeErrFound = true
		}
	}
	if !typeErrFound {
		t.Errorf("want code='type' for wrong type, got %v", msgs(errs2))
	}

	// format -> code "format"
	errs3, _ := v.ValidateJSON(json.RawMessage(`{"name":"x","email":"notanemail"}`))
	var fmtErr *schemapb.FieldError
	for _, e := range errs3 {
		if e.GetField() == "email" {
			fmtErr = e
		}
	}
	if fmtErr == nil || fmtErr.GetCode() != "format" {
		t.Errorf("want code='format', got %v", fmtErr)
	}
}

func TestObjectStrictNested(t *testing.T) {
	v := build(t,
		schemapb.Object("addr",
			schemapb.Str("zip").Required(),
		).Strict(),
	)

	if g := validateJSON(t, v, `{"addr":{"zip":"12345"}}`); len(g) != 0 {
		t.Errorf("valid nested strict: %v", g)
	}
	// unknown key in nested strict object
	if g := validateJSON(t, v, `{"addr":{"zip":"12345","extra":"x"}}`); !has(g, "addr.extra") {
		t.Errorf("unknown nested field not rejected: %v", g)
	}
}

// ---------------------------------------------------------------------------
// Feature 1: Normalize
// ---------------------------------------------------------------------------

func TestNormalize_LowercasesBeforePattern(t *testing.T) {
	// Normalize runs BEFORE structured validation (pattern check), so the
	// lowercased value must pass a lowercase-only pattern.
	v := build(t,
		schemapb.Str("name").Normalize("lower(this)").Pattern(`^[a-z]+$`),
	)
	// "Alice" -> lower -> "alice" -> matches pattern -> valid
	if g := validateJSON(t, v, `{"name":"Alice"}`); len(g) != 0 {
		t.Errorf("want valid (normalize runs first), got %v", g)
	}
	// Already lowercase -> still valid
	if g := validateJSON(t, v, `{"name":"bob"}`); len(g) != 0 {
		t.Errorf("want valid, got %v", g)
	}
}

func TestNormalize_Compute(t *testing.T) {
	// Normalize also updates the value seen by Computed fields.
	v := build(t,
		schemapb.Str("tag").Normalize("lower(this)"),
		schemapb.Computed("tag_upper", `upper(root.tag)`).Result(schemapb.ResultString),
	)
	out, errs := v.Compute(map[string]any{"tag": "Hello"})
	if len(errs) != 0 {
		t.Fatalf("compute: %v", msgs(errs))
	}
	if out["tag"] != "hello" {
		t.Errorf("normalize: tag = %v, want hello", out["tag"])
	}
	if out["tag_upper"] != "HELLO" {
		t.Errorf("computed after normalize: tag_upper = %v, want HELLO", out["tag_upper"])
	}
}

func TestNormalize_AbsentSkipped(t *testing.T) {
	v := build(t,
		schemapb.Str("name").Normalize("lower(this)"),
	)
	// absent field must not be forced present
	if g := validateJSON(t, v, `{}`); len(g) != 0 {
		t.Errorf("want valid (no value = no normalize), got %v", g)
	}
}

func TestNormalize_BadExprRejectedByIsValid(t *testing.T) {
	// bad normalize expr: deliberately malformed
	badField := schemapb.Str("x").Normalize("lower(this >")
	_, err := schemapb.NewSchema("t", "s", "v1").
		Fields(badField).
		Build()
	if err == nil {
		t.Fatal("want schema error for bad normalize expr")
	}
}

// ---------------------------------------------------------------------------
// Feature 2: OneOf (discriminated union)
// ---------------------------------------------------------------------------

func oneOfSchema(t *testing.T) *schemapb.Schema {
	t.Helper()
	return schemapb.NewSchema("test", "oneof", "v1").Fields(
		schemapb.OneOf("target", "kind").
			Variant("disk",
				schemapb.Int32("size").Required(),
			).
			Variant("net",
				schemapb.Str("cidr").Required(),
			).
			Required(),
	).MustBuild()
}

func TestOneOf_ValidDisk(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	if g := validateJSON(t, v, `{"target":{"kind":"disk","size":100}}`); len(g) != 0 {
		t.Errorf("valid disk: %v", g)
	}
}

func TestOneOf_ValidNet(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	if g := validateJSON(t, v, `{"target":{"kind":"net","cidr":"10.0.0.0/8"}}`); len(g) != 0 {
		t.Errorf("valid net: %v", g)
	}
}

func TestOneOf_MissingDiscriminator(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	g := validateJSON(t, v, `{"target":{"size":10}}`)
	if !has(g, "target") {
		t.Errorf("want oneof_discriminator error, got %v", g)
	}
}

func TestOneOf_UnknownVariant(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	g := validateJSON(t, v, `{"target":{"kind":"usb","size":10}}`)
	if !has(g, "target") {
		t.Errorf("want oneof_variant error, got %v", g)
	}
}

func TestOneOf_MissingRequiredInVariant(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	// disk variant chosen but 'size' is missing
	g := validateJSON(t, v, `{"target":{"kind":"disk"}}`)
	if !has(g, "target.size") {
		t.Errorf("want target.size required error, got %v", g)
	}
}

func TestOneOf_WrongVariantFieldError(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	// net variant chosen but 'cidr' missing
	g := validateJSON(t, v, `{"target":{"kind":"net"}}`)
	if !has(g, "target.cidr") {
		t.Errorf("want target.cidr required error, got %v", g)
	}
}

func TestOneOf_NotObject(t *testing.T) {
	v := mustValidator(t, oneOfSchema(t))
	g := validateJSON(t, v, `{"target":"disk"}`)
	if !has(g, "target") {
		t.Errorf("want type error for non-object, got %v", g)
	}
}

func TestOneOf_DefaultAndComputedInsideVariant(t *testing.T) {
	v := mustValidator(t, schemapb.NewSchema("test", "oo2", "v1").Fields(
		schemapb.OneOf("target", "kind").
			Variant("disk",
				schemapb.Int32("size").Default(20),
				schemapb.Computed("iops", "root.target.size * 50").Result(schemapb.ResultInt64),
			),
	).MustBuild())

	out, errs := v.Compute(map[string]any{
		"target": map[string]any{"kind": "disk"},
	})
	if len(errs) != 0 {
		t.Fatalf("compute: %v", msgs(errs))
	}
	inner, _ := out["target"].(map[string]any)
	if inner["size"] != float64(20) {
		t.Errorf("default inside variant: size = %v, want 20", inner["size"])
	}
	if inner["iops"] != float64(1000) {
		t.Errorf("computed inside variant: iops = %v, want 1000", inner["iops"])
	}
}

func TestOneOf_IsValidRejectsMissingDiscriminator(t *testing.T) {
	_, err := schemapb.NewSchema("t", "s", "v1").Fields(
		schemapb.OneOf("x", ""). // empty discriminator
						Variant("a", schemapb.Bool("flag")),
	).Build()
	if err == nil {
		t.Fatal("want schema error for empty discriminator")
	}
}

func TestOneOf_IsValidRejectsNoVariants(t *testing.T) {
	_, err := schemapb.NewSchema("t", "s", "v1").Fields(
		schemapb.OneOf("x", "kind"), // no variants
	).Build()
	if err == nil {
		t.Fatal("want schema error for no variants")
	}
}

func TestFilledBakeIntoBaked(t *testing.T) {
	s := schemapb.NewSchema("infra", "sizing", "v1").Fields(
		schemapb.Int32("size").Gte(1).Default(20),
		schemapb.Computed("iops", "root.size * 50").Result(schemapb.ResultInt64),
	).MustBuild()
	inline := func(vals map[string]any) *schemapb.Filled {
		return &schemapb.Filled{
			Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: s}},
			Values: mustStruct(t, vals),
		}
	}

	// Bake: validates + resolves (defaults + computed).
	baked, errs, err := inline(map[string]any{}).Bake()
	if err != nil || len(errs) != 0 || baked == nil {
		t.Fatalf("bake: errs=%v err=%v", msgs(errs), err)
	}
	if bv := baked.GetValues().AsMap(); bv["size"] != float64(20) || bv["iops"] != float64(1000) {
		t.Errorf("baked values: %v", bv)
	}

	// id-ref Filled cannot self-bake (needs the registry).
	fid := &schemapb.Filled{Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Id{Id: &schemapb.SchemaIdentity{Name: "x"}}}}
	if _, _, err := fid.Bake(); err == nil {
		t.Error("want error baking an id-ref Filled")
	}

	// IntoBaked: raw copy, NO resolve (iops not computed, default not applied).
	raw, err := inline(map[string]any{"size": float64(7)}).IntoBaked()
	if err != nil {
		t.Fatal(err)
	}
	rv := raw.GetValues().AsMap()
	if rv["size"] != float64(7) {
		t.Errorf("raw size = %v", rv["size"])
	}
	if _, ok := rv["iops"]; ok {
		t.Error("IntoBaked must not evaluate computed fields")
	}
}
