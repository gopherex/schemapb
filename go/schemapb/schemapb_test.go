package schemapb_test

import (
	"context"
	"strings"
	"testing"
	"time"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// codes extracts the set of error codes from a result, path -> codes.
func codes(res *schemapb.ValidationResult) map[string][]schemapb.ErrorCode {
	out := map[string][]schemapb.ErrorCode{}
	for _, e := range res.GetErrors() {
		out[e.GetPath()] = append(out[e.GetPath()], e.GetCode())
	}
	return out
}

func hasCode(res *schemapb.ValidationResult, path string, code schemapb.ErrorCode) bool {
	for _, e := range res.GetErrors() {
		if e.GetPath() == path && e.GetCode() == code {
			return true
		}
	}
	return false
}

func mustValidate(t *testing.T, s *schemapb.Schema, values map[string]any) *schemapb.ValidationResult {
	t.Helper()
	res, err := s.Validate(values)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return res
}

// =============================================================================
// Numeric kinds
// =============================================================================

func TestNumericConstraints(t *testing.T) {
	s := schemapb.NewSchema("t", "num", "v1").Fields(
		schemapb.Int64("i").Gt(0).Lte(100).MultipleOf(5),
		schemapb.Double("d").Gte(0.5).Lt(2.5),
		schemapb.UInt32("u").In(1, 2, 3),
		schemapb.Int32("n").NotIn(13),
		schemapb.Float("f").Const(1.5),
	).MustBuild()

	ok := mustValidate(t, s, map[string]any{
		"i": int64(25), "d": 1.0, "u": uint64(2), "n": int64(7), "f": 1.5,
	})
	if !ok.Ok() {
		t.Fatalf("want valid, got %v", ok.GetErrors())
	}

	bad := mustValidate(t, s, map[string]any{
		"i": int64(101), "d": 2.5, "u": uint64(9), "n": int64(13), "f": 2.0,
	})
	for path, code := range map[string]schemapb.ErrorCode{
		"i": schemapb.ErrorCode_ERROR_CODE_LTE_VIOLATED,
		"d": schemapb.ErrorCode_ERROR_CODE_LT_VIOLATED,
		"u": schemapb.ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET,
		"n": schemapb.ErrorCode_ERROR_CODE_IN_FORBIDDEN_SET,
		"f": schemapb.ErrorCode_ERROR_CODE_CONST_MISMATCH,
	} {
		if !hasCode(bad, path, code) {
			t.Errorf("%s: want %v, got %v", path, code, codes(bad)[path])
		}
	}
	if !hasCode(bad, "i", schemapb.ErrorCode_ERROR_CODE_MULTIPLE_OF_VIOLATED) {
		t.Errorf("i: want MULTIPLE_OF violation too")
	}
}

func TestNumericTypeMismatch(t *testing.T) {
	s := schemapb.NewSchema("t", "num2", "v1").Fields(
		schemapb.Int64("i"),
		schemapb.UInt64("u"),
		schemapb.Int32("small"),
	).MustBuild()

	res := mustValidate(t, s, map[string]any{
		"i":     1.5,            // non-integral float
		"u":     int64(-1),      // negative for unsigned
		"small": int64(1 << 40), // does not fit int32
	})
	for _, path := range []string{"i", "u", "small"} {
		if !hasCode(res, path, schemapb.ErrorCode_ERROR_CODE_TYPE_MISMATCH) {
			t.Errorf("%s: want TYPE_MISMATCH, got %v", path, codes(res)[path])
		}
	}
}

// Big int64 values survive exactly (the v0 float64 model lost them).
func TestBigInt64Precision(t *testing.T) {
	big := int64(1<<62) + 12345
	s := schemapb.NewSchema("t", "big", "v1").Fields(
		schemapb.Int64("x").Gte(big - 1).Lte(big + 1),
	).MustBuild()
	if res := mustValidate(t, s, map[string]any{"x": big}); !res.Ok() {
		t.Fatalf("want valid: %v", res.GetErrors())
	}
	baked, _, err := s.Bake(map[string]any{"x": big})
	if err != nil {
		t.Fatal(err)
	}
	if got := baked.GetValues().GetFields()["x"].GetInt64Value(); got != big {
		t.Fatalf("precision lost: %d != %d", got, big)
	}
	// protoJSON carries int64 as a string.
	j, err := protojson.Marshal(baked.GetValues())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"4611686018427400249"`) {
		t.Errorf("int64 not a JSON string: %s", j)
	}
}

// =============================================================================
// String / bytes / enum
// =============================================================================

func TestStringConstraints(t *testing.T) {
	s := schemapb.NewSchema("t", "str", "v1").Fields(
		schemapb.Str("host").MinLen(2).MaxLen(10).Pattern(`^[a-z-]+$`),
		schemapb.Str("mode").In("fast", "slow"),
		schemapb.Str("mail").Format(schemapb.FormatEmail),
		schemapb.Str("ip").Format(schemapb.FormatIPv4),
	).MustBuild()

	if res := mustValidate(t, s, map[string]any{
		"host": "db-main", "mode": "fast", "mail": "a@b.co", "ip": "10.0.0.1",
	}); !res.Ok() {
		t.Fatalf("want valid: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{
		"host": "DB!", "mode": "warp", "mail": "nope", "ip": "::1",
	})
	if !hasCode(res, "host", schemapb.ErrorCode_ERROR_CODE_PATTERN_MISMATCH) {
		t.Errorf("host: %v", codes(res)["host"])
	}
	if !hasCode(res, "mode", schemapb.ErrorCode_ERROR_CODE_NOT_IN_ALLOWED_SET) {
		t.Errorf("mode: %v", codes(res)["mode"])
	}
	if !hasCode(res, "mail", schemapb.ErrorCode_ERROR_CODE_FORMAT_MISMATCH) {
		t.Errorf("mail: %v", codes(res)["mail"])
	}
	if !hasCode(res, "ip", schemapb.ErrorCode_ERROR_CODE_FORMAT_MISMATCH) {
		t.Errorf("ip: %v", codes(res)["ip"])
	}
}

func TestBytesConstraints(t *testing.T) {
	s := schemapb.NewSchema("t", "byt", "v1").Fields(
		schemapb.Bytes("b").MinLen(2).MaxLen(4).Prefix([]byte{0xDE}),
	).MustBuild()
	if res := mustValidate(t, s, map[string]any{"b": []byte{0xDE, 0xAD}}); !res.Ok() {
		t.Fatalf("want valid: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"b": []byte{0x01}})
	if !hasCode(res, "b", schemapb.ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED) ||
		!hasCode(res, "b", schemapb.ErrorCode_ERROR_CODE_PREFIX_MISMATCH) {
		t.Errorf("b: %v", codes(res)["b"])
	}
}

func TestEnum(t *testing.T) {
	s := schemapb.NewSchema("t", "enum", "v1").Fields(
		schemapb.Enum("lvl").Values(map[int32]string{0: "min", 1: "replica", 2: "logical"}).DefinedOnly(),
		schemapb.Str("kind").Default("a"),
		schemapb.Enum("dyn").Options(`root.kind == "a" ? [1, 2] : [3]`),
	).MustBuild()

	if res := mustValidate(t, s, map[string]any{"lvl": int64(2), "dyn": int64(1)}); !res.Ok() {
		t.Fatalf("want valid: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"lvl": int64(9), "dyn": int64(3)})
	if !hasCode(res, "lvl", schemapb.ErrorCode_ERROR_CODE_ENUM_NOT_DEFINED) {
		t.Errorf("lvl: %v", codes(res)["lvl"])
	}
	if !hasCode(res, "dyn", schemapb.ErrorCode_ERROR_CODE_ENUM_NOT_ALLOWED) {
		t.Errorf("dyn: %v", codes(res)["dyn"])
	}

	opts, err := s.EnumOptions("dyn", map[string]any{"kind": "b"})
	if err != nil || len(opts) != 1 || opts[0] != 3 {
		t.Errorf("EnumOptions: %v %v", opts, err)
	}
}

// =============================================================================
// Duration / timestamp
// =============================================================================

func TestDurationTimestamp(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := schemapb.NewSchema("t", "dt", "v1").Fields(
		schemapb.Duration("d").Gte(time.Second).Lte(time.Minute),
		schemapb.Timestamp("ts").Gte(now),
	).MustBuild()

	// Native and string forms both validate.
	if res := mustValidate(t, s, map[string]any{"d": 30 * time.Second, "ts": now.Add(time.Hour)}); !res.Ok() {
		t.Fatalf("native: %v", res.GetErrors())
	}
	if res := mustValidate(t, s, map[string]any{"d": "30s", "ts": "2026-06-01T00:00:00Z"}); !res.Ok() {
		t.Fatalf("string: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"d": "2h", "ts": "2020-01-01T00:00:00Z"})
	if !hasCode(res, "d", schemapb.ErrorCode_ERROR_CODE_LTE_VIOLATED) {
		t.Errorf("d: %v", codes(res)["d"])
	}
	if !hasCode(res, "ts", schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED) {
		t.Errorf("ts: %v", codes(res)["ts"])
	}
}

// =============================================================================
// Presence / nullability / strict / props
// =============================================================================

func TestPresence(t *testing.T) {
	s := schemapb.NewSchema("t", "pres", "v1").Strict().MinProps(1).MaxProps(3).Fields(
		schemapb.Str("req").Required(),
		schemapb.Str("opt"),
		schemapb.Str("nul").Nullable(),
	).MustBuild()

	res := mustValidate(t, s, map[string]any{"opt": nil, "junk": "x"})
	if !hasCode(res, "req", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("req: %v", codes(res)["req"])
	}
	if !hasCode(res, "opt", schemapb.ErrorCode_ERROR_CODE_NOT_NULLABLE) {
		t.Errorf("opt: %v", codes(res)["opt"])
	}
	if !hasCode(res, "junk", schemapb.ErrorCode_ERROR_CODE_UNKNOWN_FIELD) {
		t.Errorf("junk: %v", codes(res)["junk"])
	}
	if res := mustValidate(t, s, map[string]any{"req": "x", "nul": nil}); !res.Ok() {
		t.Errorf("nullable null must pass: %v", res.GetErrors())
	}
}

// =============================================================================
// Coercion
// =============================================================================

func TestCoerce(t *testing.T) {
	s := schemapb.NewSchema("t", "coerce", "v1").Coerce().Fields(
		schemapb.Int64("i").Gte(10),
		schemapb.Bool("b"),
		schemapb.Double("d"),
	).MustBuild()
	vals := map[string]any{"i": "42", "b": "true", "d": "1.5"}
	if res := mustValidate(t, s, vals); !res.Ok() {
		t.Fatalf("coerce: %v", res.GetErrors())
	}
	if vals["i"] != int64(42) || vals["b"] != true || vals["d"] != 1.5 {
		t.Errorf("coerced values: %#v", vals)
	}
}

// =============================================================================
// Computed / normalize / when
// =============================================================================

func TestComputedDependencyOrder(t *testing.T) {
	s := schemapb.NewSchema("t", "comp", "v1").Fields(
		schemapb.Int64("base").Default(10),
		schemapb.Computed("double", "root.base * 2").Result(schemapb.ResultInt64),
		schemapb.Computed("quad", "root.double * 2").Result(schemapb.ResultInt64),
	).MustBuild()
	out, res, err := s.Resolve(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("%v %v", err, res.GetErrors())
	}
	if out["quad"] != int64(40) {
		t.Errorf("quad = %v (%T)", out["quad"], out["quad"])
	}
}

func TestComputedCycleRejectedAtCompile(t *testing.T) {
	_, err := schemapb.NewSchema("t", "cycle", "v1").Fields(
		schemapb.Computed("a", "root.b + 1"),
		schemapb.Computed("b", "root.a + 1"),
	).Build()
	if err == nil {
		t.Fatal("want cycle error")
	}
	var se *schemapb.SchemaError
	if ok := errorsAs(err, &se); !ok {
		t.Fatalf("want *SchemaError, got %T: %v", err, err)
	}
}

func errorsAs[T error](err error, target *T) bool {
	for err != nil {
		if t, ok := err.(T); ok {
			*target = t
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestWhenGate(t *testing.T) {
	s := schemapb.NewSchema("t", "when", "v1").Strict().Fields(
		schemapb.Bool("advanced").Default(false),
		schemapb.Int64("tuning").When("root.advanced == true").Required().Gte(1),
	).MustBuild()

	// Inactive: required is skipped, a stale value is ignored, strict not hit.
	if res := mustValidate(t, s, map[string]any{"advanced": false, "tuning": int64(0)}); !res.Ok() {
		t.Fatalf("inactive: %v", res.GetErrors())
	}
	// Active: required fires.
	res := mustValidate(t, s, map[string]any{"advanced": true})
	if !hasCode(res, "tuning", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("active: %v", codes(res))
	}
	active, err := s.FieldActive("tuning", map[string]any{"advanced": true})
	if err != nil || !active {
		t.Errorf("FieldActive: %v %v", active, err)
	}
}

func TestNormalize(t *testing.T) {
	s := schemapb.NewSchema("t", "norm", "v1").Fields(
		schemapb.Str("host").Normalize("this.lowerAscii()").In("db-1"),
	).MustBuild()
	vals := map[string]any{"host": "DB-1"}
	if res := mustValidate(t, s, vals); !res.Ok() {
		t.Fatalf("normalize: %v", res.GetErrors())
	}
	if vals["host"] != "db-1" {
		t.Errorf("host = %v", vals["host"])
	}
}

// =============================================================================
// Immutable / secret / rules
// =============================================================================

func TestImmutable(t *testing.T) {
	s := schemapb.NewSchema("t", "imm", "v1").Fields(
		schemapb.Str("sys").Immutable().Default("fixed"),
	).MustBuild()
	res := mustValidate(t, s, map[string]any{"sys": "changed"})
	if !hasCode(res, "sys", schemapb.ErrorCode_ERROR_CODE_IMMUTABLE_MODIFIED) {
		t.Fatalf("immutable: %v", res.GetErrors())
	}
	// And the value is forced back to the default by resolve.
	vals := map[string]any{"sys": "changed"}
	_, _ = s.Validate(vals)
	if vals["sys"] != "fixed" {
		t.Errorf("not forced: %v", vals["sys"])
	}
}

func TestRulesAndSeverity(t *testing.T) {
	s := schemapb.NewSchema("t", "rules", "v1").Fields(
		schemapb.Int64("work_mem").Default(64),
		schemapb.Int64("conns").Default(10).Rules(
			schemapb.Rule("int(this) * int(root.work_mem) <= 1024", "memory budget").ID("budget"),
		),
	).Rules(
		schemapb.Rule("int(root.conns) < 100", "too many").Warn(),
	).MustBuild()

	res := mustValidate(t, s, map[string]any{"conns": int64(200)})
	if !hasCode(res, "conns", schemapb.ErrorCode_ERROR_CODE_RULE_VIOLATED) {
		t.Errorf("field rule: %v", codes(res))
	}
	var sawWarn bool
	for _, e := range res.GetErrors() {
		if e.GetSeverity() == schemapb.SeverityWarning {
			sawWarn = true
		}
		if e.GetRuleId() == "budget" && e.GetExpr() == "" {
			t.Error("rule error must carry expr")
		}
	}
	if !sawWarn {
		t.Error("warning severity lost")
	}
}

// =============================================================================
// Containers: list / object / map / oneof / ref
// =============================================================================

func TestList(t *testing.T) {
	s := schemapb.NewSchema("t", "list", "v1").Fields(
		schemapb.Int64("replicas").Default(2),
		schemapb.List("names", schemapb.Str("").MinLen(1)).MinItems(1).Unique().Count("int(root.replicas)"),
	).MustBuild()

	if res := mustValidate(t, s, map[string]any{"names": []any{"a", "b"}}); !res.Ok() {
		t.Fatalf("valid list: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"names": []any{"a", "a", ""}})
	if !hasCode(res, "names", schemapb.ErrorCode_ERROR_CODE_LIST_COUNT_MISMATCH) {
		t.Errorf("count: %v", codes(res))
	}
	if !hasCode(res, "names[1]", schemapb.ErrorCode_ERROR_CODE_NOT_UNIQUE) {
		t.Errorf("unique: %v", codes(res))
	}
	if !hasCode(res, "names[2]", schemapb.ErrorCode_ERROR_CODE_MIN_LEN_VIOLATED) {
		t.Errorf("item: %v", codes(res))
	}
	if n, err := s.ListCount("names", map[string]any{"replicas": int64(5)}); err != nil || n != 5 {
		t.Errorf("ListCount: %v %v", n, err)
	}
}

func TestListItemIndexBinding(t *testing.T) {
	s := schemapb.NewSchema("t", "idx", "v1").Fields(
		schemapb.List("ports",
			schemapb.Int64("").Rules(schemapb.Rule("int(this) == 8000 + index", "port must follow index")),
		),
	).MustBuild()
	if res := mustValidate(t, s, map[string]any{"ports": []any{int64(8000), int64(8001)}}); !res.Ok() {
		t.Fatalf("index binding: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"ports": []any{int64(9999)}})
	if !hasCode(res, "ports[0]", schemapb.ErrorCode_ERROR_CODE_RULE_VIOLATED) {
		t.Errorf("index rule: %v", codes(res))
	}
}

func TestNestedObject(t *testing.T) {
	s := schemapb.NewSchema("t", "obj", "v1").Fields(
		schemapb.Object("db",
			schemapb.Str("host").Required(),
			schemapb.Int64("port").Default(5432).Gte(1).Lte(65535),
		).Required(),
	).MustBuild()

	res := mustValidate(t, s, map[string]any{"db": map[string]any{"port": int64(70000)}})
	if !hasCode(res, "db.host", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("nested required: %v", codes(res))
	}
	if !hasCode(res, "db.port", schemapb.ErrorCode_ERROR_CODE_LTE_VIOLATED) {
		t.Errorf("nested range: %v", codes(res))
	}
	// Defaults seed inside present objects.
	vals := map[string]any{"db": map[string]any{"host": "x"}}
	if res := mustValidate(t, s, vals); !res.Ok() {
		t.Fatalf("%v", res.GetErrors())
	}
	if vals["db"].(map[string]any)["port"] != int64(5432) {
		t.Errorf("nested default: %v", vals)
	}
}

func TestMapKind(t *testing.T) {
	s := schemapb.NewSchema("t", "map", "v1").Fields(
		schemapb.Map("subnets",
			schemapb.Str("cidr").Required().Format("ipv4"),
		).Strict().MinEntries(1),
	).MustBuild()

	res := mustValidate(t, s, map[string]any{"subnets": map[string]any{
		"b-net": map[string]any{"cidr": "10.0.0.1", "evil": 1},
		"a-net": map[string]any{},
	}})
	if !hasCode(res, "subnets.a-net.cidr", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("map value required: %v", codes(res))
	}
	if !hasCode(res, "subnets.b-net.evil", schemapb.ErrorCode_ERROR_CODE_UNKNOWN_FIELD) {
		t.Errorf("map value strict: %v", codes(res))
	}
	// Deterministic order: a-net errors before b-net.
	var order []string
	for _, e := range res.GetErrors() {
		order = append(order, e.GetPath())
	}
	ai, bi := -1, -1
	for i, p := range order {
		if strings.HasPrefix(p, "subnets.a-net") && ai == -1 {
			ai = i
		}
		if strings.HasPrefix(p, "subnets.b-net") && bi == -1 {
			bi = i
		}
	}
	if ai == -1 || bi == -1 || ai > bi {
		t.Errorf("map error order: %v", order)
	}
}

func TestOneOf(t *testing.T) {
	s := schemapb.NewSchema("t", "oneof", "v1").Fields(
		schemapb.OneOf("store", "type").
			Variant("s3", schemapb.Str("bucket").Required()).
			Variant("disk", schemapb.Str("path").Required()),
	).MustBuild()

	if res := mustValidate(t, s, map[string]any{"store": map[string]any{"type": "s3", "bucket": "b"}}); !res.Ok() {
		t.Fatalf("oneof valid: %v", res.GetErrors())
	}
	res := mustValidate(t, s, map[string]any{"store": map[string]any{"bucket": "b"}})
	if !hasCode(res, "store", schemapb.ErrorCode_ERROR_CODE_DISCRIMINATOR_MISSING) {
		t.Errorf("discriminator: %v", codes(res))
	}
	res = mustValidate(t, s, map[string]any{"store": map[string]any{"type": "gcs"}})
	if !hasCode(res, "store", schemapb.ErrorCode_ERROR_CODE_UNKNOWN_VARIANT) {
		t.Errorf("variant: %v", codes(res))
	}
}

func TestRefAndDefs(t *testing.T) {
	s := schemapb.NewSchema("t", "ref", "v1").
		Def("endpoint",
			schemapb.Str("host").Required(),
			schemapb.Int64("port").Default(80),
		).
		Fields(
			schemapb.Ref("primary", "endpoint").Required(),
		).MustBuild()

	res := mustValidate(t, s, map[string]any{"primary": map[string]any{}})
	if !hasCode(res, "primary.host", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("ref: %v", codes(res))
	}
	// Unknown def is a build-time error.
	if _, err := schemapb.NewSchema("t", "bad", "v1").Fields(schemapb.Ref("x", "nope")).Build(); err == nil {
		t.Error("unknown def must fail Build")
	}
}

func TestLink(t *testing.T) {
	ctx := context.Background()
	reg := schemapb.NewInMemoryRegistry()
	shared := schemapb.NewSchema("infra", "endpoint", "v1").Fields(
		schemapb.Str("host").Required(),
	).MustBuild()
	if err := reg.Put(ctx, shared); err != nil {
		t.Fatal(err)
	}

	s := schemapb.NewSchema("t", "linked", "v1").Fields(
		schemapb.RefIdentity("ep", "infra", "endpoint", "v1"),
	).MustBuild()

	// Unlinked: identity-ref reports UNKNOWN_REF.
	res := mustValidate(t, s, map[string]any{"ep": map[string]any{"host": "h"}})
	if !hasCode(res, "ep", schemapb.ErrorCode_ERROR_CODE_UNKNOWN_REF) {
		t.Errorf("unlinked: %v", codes(res))
	}

	linked, err := s.Link(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if res := mustValidate(t, linked, map[string]any{"ep": map[string]any{"host": "h"}}); !res.Ok() {
		t.Errorf("linked: %v", res.GetErrors())
	}
	res = mustValidate(t, linked, map[string]any{"ep": map[string]any{}})
	if !hasCode(res, "ep.host", schemapb.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Errorf("linked required: %v", codes(res))
	}
}

// =============================================================================
// Bake / merge
// =============================================================================

func TestBakeMerge(t *testing.T) {
	s := schemapb.NewSchema("t", "bake", "v1").Fields(
		schemapb.Str("name").Required(),
		schemapb.Object("res",
			schemapb.Int64("cpu").Default(1),
			schemapb.Int64("mem").Default(256),
		),
	).MustBuild()

	baked, res, err := s.Bake(map[string]any{"name": "a", "res": map[string]any{"cpu": int64(2)}})
	if err != nil || res.Blocking() {
		t.Fatalf("%v %v", err, res.GetErrors())
	}
	ov, _ := schemapb.StructFromGo(map[string]any{"res": map[string]any{"mem": int64(512)}})
	merged, res2, err := baked.Merge(ov, false)
	if err != nil || res2.Blocking() {
		t.Fatalf("%v %v", err, res2.GetErrors())
	}
	got := merged.GetValues().ToGo()
	rm := got["res"].(map[string]any)
	if rm["cpu"] != int64(2) || rm["mem"] != int64(512) {
		t.Errorf("merge: %#v", got)
	}
	if !merged.Matches(s) || merged.Matches(schemapb.NewSchema("x", "y", "z").MustBuild()) {
		t.Error("Matches broken")
	}
}

func TestFilledBake(t *testing.T) {
	s := schemapb.NewSchema("t", "filled", "v1").Fields(
		schemapb.Int64("x").Default(7),
	).MustBuild()
	vals, _ := schemapb.StructFromGo(map[string]any{})
	filled := &schemapb.Filled{
		Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: s}},
		Values: vals,
	}
	baked, res, err := filled.Bake()
	if err != nil || res.Blocking() {
		t.Fatalf("%v %v", err, res.GetErrors())
	}
	if baked.GetValues().GetFields()["x"].GetInt64Value() != 7 {
		t.Errorf("filled bake: %v", baked.GetValues())
	}
}

// =============================================================================
// Wire round-trips
// =============================================================================

func TestSchemaProtoRoundTrip(t *testing.T) {
	s := schemapb.NewSchema("infra", "pg", "v1").Strict().Fields(
		schemapb.Int64("conns").Gte(1).Default(10),
		schemapb.Enum("lvl").Values(map[int32]string{0: "a"}).DefinedOnly(),
		schemapb.List("xs", schemapb.Str("").MinLen(1)),
	).Template("conf", "{{#fields}}{{name}}\n{{/fields}}").MustBuild()

	wire, err := proto.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back schemapb.Schema
	if err := proto.Unmarshal(wire, &back); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(s, &back) {
		t.Fatal("schema wire round-trip lost data")
	}
	// The decoded schema validates identically.
	res, err := back.Validate(map[string]any{"conns": int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(res, "conns", schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED) {
		t.Errorf("decoded schema: %v", codes(res))
	}
}

// =============================================================================
// Composition builders
// =============================================================================

func TestInlineComposition(t *testing.T) {
	inner := schemapb.NewSchema("lib", "endpoint", "v1").
		Def("port", schemapb.Int64("value").Gte(1).Lte(65535)).
		Fields(
			schemapb.Str("host").Required(),
			schemapb.Ref("port", "port"),
		).MustBuild()

	outer := schemapb.NewSchema("t", "compose", "v1").
		Fields(schemapb.ObjectOf("ep", inner)).
		MustBuild()

	// Inner defs are hoisted to the outer root.
	res := mustValidate(t, outer, map[string]any{
		"ep": map[string]any{"host": "h", "port": map[string]any{"value": int64(0)}},
	})
	if !hasCode(res, "ep.port.value", schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED) {
		t.Errorf("hoisted def: %v", codes(res))
	}
}
