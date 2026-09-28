package schemapb_test

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

func TestGoldenAnnotations(t *testing.T) {
	t.Parallel()

	metadata := map[string]*sp.Value{
		"backplate.live": sp.BoolV(true), "unknown.false": sp.BoolV(false), "unknown.null": sp.NullV(),
		"unknown.large": sp.UInt64V(^uint64(0)), "unknown.typed": {Kind: &sp.Value_ListValue{ListValue: coverageAllValues()}},
		"secret": sp.BoolV(true), "normalize": sp.StrV("root.missing"),
	}

	s := sp.NewSchema(sp.ID("conformance", "annotations", sp.Ver(1, 0, 0))).
		Def("db", sp.Str("token").Default("private").Secret()).
		Fields(sp.Int32("port").Default(10).Normalize("this + 1"), sp.Ref("db", "db").DefaultEmpty(),
			sp.List("workers", sp.Object("", sp.Int32("cpu").Default(2))),
			sp.MapOf("labels", sp.Str("value")),
			sp.OneOf("auth", "kind").Variant("password", sp.Str("kind"), sp.Str("token").Secret())).MustBuild()
	for _, f := range s.Fields {
		f.Annotations = metadata
	}

	s.Defs["db"].Fields[0].Annotations = metadata
	s.Fields[2].GetList().Items[0].Annotations = metadata
	s.Fields[3].GetMap().ValueField.Annotations = metadata
	s.Fields[4].GetOneOf().Variants["password"].Fields[1].Annotations = metadata
	before := proto.Clone(s)

	wire, err := proto.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	restored := &sp.Schema{}
	if err := proto.Unmarshal(wire, restored); err != nil || !proto.Equal(s, restored) {
		t.Fatal("annotations lost in binary round trip")
	}

	input := sp.MustStructFromGo(map[string]any{"workers": []any{map[string]any{}}, "labels": map[string]any{"a": "b"}, "auth": map[string]any{"kind": "password", "token": "private"}})

	baked, res, report, err := s.BakeDetailed(input.ToGo())
	if err != nil || !res.Ok() {
		t.Fatalf("bake: %v %v", res, err)
	}

	if !proto.Equal(s, before) || !proto.Equal(baked.Schema, s) {
		t.Fatal("bake altered schema annotations")
	}

	if baked.Values.Fields["port"].GetInt32Value() != 11 || baked.Masked().Fields["port"].GetInt32Value() != 11 {
		t.Fatal("annotations interpreted as engine directives")
	}

	plain := sp.NewSchema(sp.ID("conformance", "annotations", sp.Ver(1, 0, 0))).
		Def("db", sp.Str("token").Default("private").Secret()).
		Fields(sp.Int32("port").Default(10).Normalize("this + 1"), sp.Ref("db", "db").DefaultEmpty(), sp.List("workers", sp.Object("", sp.Int32("cpu").Default(2))), sp.MapOf("labels", sp.Str("value")), sp.OneOf("auth", "kind").Variant("password", sp.Str("kind"), sp.Str("token").Secret())).MustBuild()

	other, otherRes, otherReport, err := plain.BakeDetailed(input.ToGo())
	if err != nil || !proto.Equal(baked.Values, other.Values) || !proto.Equal(res, otherRes) || !proto.Equal(report, otherReport) || !proto.Equal(baked.Masked(), other.Masked()) {
		t.Fatal("metadata changed execution")
	}

	doc := struct {
		Schema json.RawMessage `json:"schema"`
		Input  json.RawMessage `json:"input"`
		Baked  json.RawMessage `json:"baked"`
		Masked json.RawMessage `json:"masked"`
		Report json.RawMessage `json:"report"`
	}{stableJSON(t, s), stableJSON(t, input), stableJSON(t, baked), stableJSON(t, baked.Masked()), stableJSON(t, report)}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "annotations.json", append(raw, '\n'))
}

func TestAnnotationsTag(t *testing.T) {
	t.Parallel()

	type config struct {
		Count int32 `json:"count" schemapb:"default=1;annotations={\"backplate.live\":{\"boolValue\":true},\"custom;key\":{\"stringValue\":\"a;b\"}}"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f := s.Fields[0]
	if !f.Annotations["backplate.live"].GetBoolValue() || f.Annotations["custom;key"].GetStringValue() != "a;b" {
		t.Fatal("generic annotations tag failed")
	}
}
