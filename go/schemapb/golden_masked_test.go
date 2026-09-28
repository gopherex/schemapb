package schemapb_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

//nolint:paralleltest,tparallel // ordered cross-language fixture
func TestGoldenMasked(t *testing.T) {
	t.Parallel()

	obj := sp.StructV
	hidden := func() *sp.Value { return sp.StrV("***") }
	fields := func(f ...sp.FieldDef) *sp.Schema {
		return sp.NewSchema(sp.ID("conformance", "masked", sp.Ver(1, 0, 0))).Fields(f...).MustBuild()
	}
	child := []sp.FieldDef{sp.Str("token").Secret(), sp.Int32("port")}
	rawChild := obj(map[string]*sp.Value{"token": sp.StrV("sensitive"), "port": sp.Int32V(80)})
	safeChild := obj(map[string]*sp.Value{"token": hidden(), "port": sp.Int32V(80)})
	refSchema := sp.NewSchema(sp.ID("conformance", "masked", sp.Ver(1, 0, 0))).Def("child", child...).Fields(sp.Ref("db", "child"), sp.List("servers", sp.Ref("", "child")), sp.MapOf("tenants", sp.Ref("value", "child"))).MustBuild()

	identitySchema := fields(sp.RefID("db", sp.ID("test", "child", sp.Ver(1, 0, 0))))
	identitySchema.Defs = map[string]*sp.Schema{"test\x00child\x00v1.0.0": fields(child...)}
	recursiveSchema := sp.NewSchema(sp.ID("conformance", "recursive", sp.Ver(1, 0, 0))).Def("node", sp.Str("token").Secret(), sp.Ref("child", "node")).Fields(sp.Ref("tree", "node")).MustBuild()

	type maskCase struct {
		name   string
		schema *sp.Schema
		input  map[string]*sp.Value
		want   map[string]*sp.Value
	}

	cases := []maskCase{
		{"identity-ref", identitySchema, map[string]*sp.Value{"db": rawChild}, map[string]*sp.Value{"db": safeChild}},
		{"missing-ref", fields(sp.RefID("db", sp.ID("test", "missing", sp.Ver(1, 0, 0)))), map[string]*sp.Value{"db": rawChild}, map[string]*sp.Value{"db": hidden()}},
		{"recursive-ref", recursiveSchema, map[string]*sp.Value{"tree": obj(map[string]*sp.Value{"token": sp.StrV("outer"), "child": obj(map[string]*sp.Value{"token": sp.StrV("inner")})})}, map[string]*sp.Value{"tree": obj(map[string]*sp.Value{"token": hidden(), "child": obj(map[string]*sp.Value{"token": hidden()})})}},
		{"container-shape-mismatch", fields(sp.Object("db", child...).When("false")), map[string]*sp.Value{"db": sp.StrV("sensitive")}, map[string]*sp.Value{"db": hidden()}},
		{"oneof-in-collections", fields(sp.List("jobs", sp.OneOf("", "kind").Variant("a", sp.Str("kind"), sp.Str("token").Secret())), sp.MapOf("map", sp.OneOf("value", "kind").Variant("a", sp.Str("kind"), sp.Str("token").Secret()))), map[string]*sp.Value{"jobs": sp.ListV(obj(map[string]*sp.Value{"kind": sp.StrV("a"), "token": sp.StrV("sensitive")})), "map": obj(map[string]*sp.Value{"x": obj(map[string]*sp.Value{"kind": sp.StrV("a"), "token": sp.StrV("sensitive")})})}, map[string]*sp.Value{"jobs": sp.ListV(obj(map[string]*sp.Value{"kind": sp.StrV("a"), "token": hidden()})), "map": obj(map[string]*sp.Value{"x": obj(map[string]*sp.Value{"kind": sp.StrV("a"), "token": hidden()})})}},
		{
			"scalar-null-absent-inactive", fields(sp.Str("token").Secret(), sp.Int64("pin").Secret(), sp.Str("nil").Nullable().Secret(), sp.Str("absent").Secret(), sp.Str("inactive").When("false").Secret(), sp.Str("public")),
			map[string]*sp.Value{"token": sp.StrV("sensitive"), "pin": sp.Int64V(123), "nil": sp.NullV(), "inactive": sp.StrV("hidden too"), "public": sp.StrV("ok"), "unknown": sp.Int32V(7)},
			map[string]*sp.Value{"token": hidden(), "pin": hidden(), "nil": hidden(), "inactive": hidden(), "public": sp.StrV("ok"), "unknown": sp.Int32V(7)},
		},
		{
			"secret-containers", fields(sp.Object("db", child...).Secret(), sp.List("items", sp.Str("")).Secret(), sp.MapOf("map", sp.Str("value")).Secret(), sp.JSON("json").Secret()),
			map[string]*sp.Value{"db": rawChild, "items": sp.ListV(sp.StrV("sensitive")), "map": rawChild, "json": rawChild},
			map[string]*sp.Value{"db": hidden(), "items": hidden(), "map": hidden(), "json": hidden()},
		},
		{
			"object-list-tuple", fields(sp.Object("db", child...), sp.List("servers", sp.Object("", child...)), sp.List("tuple", sp.Str("").Secret(), sp.Int32("")), sp.List("tokens", sp.Str("").Secret())),
			map[string]*sp.Value{"db": rawChild, "servers": sp.ListV(rawChild, sp.NullV()), "tuple": sp.ListV(sp.StrV("sensitive"), sp.Int32V(80)), "tokens": sp.ListV(sp.StrV("sensitive"), sp.NullV())},
			map[string]*sp.Value{"db": safeChild, "servers": sp.ListV(safeChild, sp.NullV()), "tuple": sp.ListV(hidden(), sp.Int32V(80)), "tokens": sp.ListV(hidden(), hidden())},
		},
		{
			"maps", fields(sp.MapOf("tokens", sp.Str("value").Secret()), sp.Map("tenants", child...), sp.MapOf("nested", sp.List("value", sp.Object("", child...)))),
			map[string]*sp.Value{"tokens": obj(map[string]*sp.Value{"customer.a": sp.StrV("sensitive")}), "tenants": obj(map[string]*sp.Value{"customer.a": rawChild, "null": sp.NullV()}), "nested": obj(map[string]*sp.Value{"x": sp.ListV(rawChild)})},
			map[string]*sp.Value{"tokens": obj(map[string]*sp.Value{"customer.a": hidden()}), "tenants": obj(map[string]*sp.Value{"customer.a": safeChild, "null": sp.NullV()}), "nested": obj(map[string]*sp.Value{"x": sp.ListV(safeChild)})},
		},
		{
			"refs", refSchema,
			map[string]*sp.Value{"db": rawChild, "servers": sp.ListV(rawChild), "tenants": obj(map[string]*sp.Value{"x": rawChild})},
			map[string]*sp.Value{"db": safeChild, "servers": sp.ListV(safeChild), "tenants": obj(map[string]*sp.Value{"x": safeChild})},
		},
		{
			"oneof-secret-discriminator", fields(sp.OneOf("job", "kind").Variant("a", sp.Str("kind").Secret(), sp.Str("token").Secret()).Variant("b", sp.Str("kind"), sp.Str("token"))),
			map[string]*sp.Value{"job": obj(map[string]*sp.Value{"kind": sp.StrV("a"), "token": sp.StrV("sensitive")})},
			map[string]*sp.Value{"job": obj(map[string]*sp.Value{"kind": hidden(), "token": hidden()})},
		},
		{
			"oneof-selected-variant", fields(sp.OneOf("job", "kind").Variant("a", sp.Str("kind"), sp.Str("token").Secret()).Variant("b", sp.Str("kind"), sp.Str("token"))),
			map[string]*sp.Value{"job": obj(map[string]*sp.Value{"kind": sp.StrV("b"), "token": sp.StrV("public")})},
			map[string]*sp.Value{"job": obj(map[string]*sp.Value{"kind": sp.StrV("b"), "token": sp.StrV("public")})},
		},
		{
			"no-cel-execution", fields(sp.Object("db", child...).When("root.missing"), sp.Int64("counter").Normalize("this + 1"), sp.Computed("result", "root.missing").Secret()),
			map[string]*sp.Value{"db": rawChild, "counter": sp.Int64V(1), "result": sp.StrV("sensitive")},
			map[string]*sp.Value{"db": safeChild, "counter": sp.Int64V(1), "result": hidden()},
		},
		{
			"unknown-variant", fields(sp.OneOf("job", "kind").Variant("a", sp.Str("token").Secret()).When("false")),
			map[string]*sp.Value{"job": obj(map[string]*sp.Value{"kind": sp.StrV("__proto__"), "token": sp.StrV("sensitive")})},
			map[string]*sp.Value{"job": hidden()},
		},
		{"missing-schema", nil, map[string]*sp.Value{"db": rawChild}, map[string]*sp.Value{"db": hidden()}},
		{"null-container", fields(sp.Object("db", child...).Nullable()), map[string]*sp.Value{"db": sp.NullV()}, map[string]*sp.Value{"db": sp.NullV()}},
		{"empty", fields(sp.Str("absent").Secret()), nil, nil},
	}
	// Keep all public wire kinds and mutable payloads intact; no native/JSON round trip.
	public := map[string]*sp.Value{"bool": sp.BoolV(true), "null": sp.NullV(), "float": sp.FloatV(1.25), "double": sp.DoubleV(1.5), "int64": sp.Int64V(-9007199254740993), "uint32": sp.UInt32V(42), "string": sp.StrV("public"), "bytes": sp.BytesV([]byte{1, 2}), "duration": sp.DurationV(time.Second), "timestamp": sp.TimestampV(time.Unix(123, 456)), "u64": sp.UInt64V(^uint64(0)), "nested": obj(map[string]*sp.Value{"list": sp.ListV(sp.Int32V(7))})}
	cases = append(cases, maskCase{"public-wire-values", fields(), public, public})

	type fixture struct {
		Name   string          `json:"name"`
		Baked  json.RawMessage `json:"baked"`
		Masked json.RawMessage `json:"masked"`
	}

	var fixtures []fixture

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baked := &sp.Baked{Schema: tc.schema, Values: &sp.StructValue{Fields: tc.input}}
			before := proto.Clone(baked)

			got := baked.Masked()
			if !proto.Equal(got, &sp.StructValue{Fields: tc.want}) {
				t.Fatalf("masked = %v; want %v", got, tc.want)
			}

			if !proto.Equal(baked, before) {
				t.Fatal("masking mutated source")
			}

			fixtures = append(fixtures, fixture{tc.name, stableJSON(t, baked), stableJSON(t, got)})
			mutateMasked(got)

			if !proto.Equal(baked, before) {
				t.Fatal("masked output aliases source")
			}

			if !proto.Equal(baked.Masked(), &sp.StructValue{Fields: tc.want}) {
				t.Fatal("successive results are not independent")
			}
		})
	}

	raw, err := json.MarshalIndent(fixtures, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "masked.json", append(raw, '\n'))
}

func mutateMasked(values *sp.StructValue) {
	for _, value := range values.GetFields() {
		mutateMaskedValue(value)
	}
}

func mutateMaskedValue(value *sp.Value) {
	mutateMasked(value.GetStructValue())

	for _, item := range value.GetListValue().GetItems() {
		mutateMaskedValue(item)
	}

	if b := value.GetBytesValue(); len(b) != 0 {
		b[0] ^= 255
	}

	if d := value.GetDurationValue(); d != nil {
		d.Seconds++
	}

	if ts := value.GetTimestampValue(); ts != nil {
		ts.Seconds++
	}

	proto.Reset(value)
}

func TestMaskedNil(t *testing.T) {
	t.Parallel()

	var b *sp.Baked
	if b.Masked() == nil || len(b.Masked().GetFields()) != 0 {
		t.Fatal("nil snapshot must yield empty values")
	}
}

func TestMaskedBaked(t *testing.T) {
	t.Parallel()

	type Config struct {
		Token string `json:"token" schemapb:"secret=true;default=sensitive"`
		Port  int64  `json:"port"  schemapb:"default=5432"`
	}

	schema, err := sp.Reflect(reflect.TypeFor[Config](), sp.ID("test", "masked", sp.Ver(1, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}

	baked, result, err := schema.Bake(nil)
	if err != nil || !result.Ok() {
		t.Fatalf("bake: %v %v", result, err)
	}

	masked := baked.Masked()
	if masked.GetFields()["token"].GetStringValue() != "***" || masked.GetFields()["port"].GetInt64Value() != 5432 {
		t.Fatal(masked)
	}

	if baked.GetValues().GetFields()["token"].GetStringValue() != "sensitive" {
		t.Fatal("snapshot changed")
	}
}
