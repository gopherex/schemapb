package schemapb_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

type enumConfig struct {
	Mode   descriptorpb.FieldDescriptorProto_Type            `json:"mode"              schemapb:"default=2"`
	Modes  []descriptorpb.FieldDescriptorProto_Type          `json:"modes,omitempty"`
	Fixed  [2]descriptorpb.FieldDescriptorProto_Type         `json:"fixed,omitempty"`
	ByName map[string]descriptorpb.FieldDescriptorProto_Type `json:"by_name,omitempty"`
	Ptr    *descriptorpb.FieldDescriptorProto_Type           `json:"ptr,omitempty"`
}

func TestReflectProtoEnum(t *testing.T) {
	t.Parallel()

	s, err := sp.ReflectType[enumConfig](sp.ID("test", "enum", sp.Ver(1, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}

	fields := s.GetFields()
	for _, field := range []*sp.Schema_Field{fields[0], fields[1].GetList().GetItems()[0], fields[2].GetList().GetItems()[0], fields[3].GetMap().GetValueField(), fields[4]} {
		options := field.GetChoice().GetOptions()
		if len(options) != descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Descriptor().Values().Len() || options[1].GetLabel() != "TYPE_FLOAT" || !proto.Equal(options[1].Value, sp.Int32V(2)) {
			t.Fatalf("enum metadata missing at %s: %v", field.GetName(), field)
		}
	}

	if !fields[4].GetNullable() || fields[4].GetRequired() || fields[2].GetList().GetMinItems() != 2 {
		t.Fatal("enum changed pointer/array semantics")
	}

	baked, result, report, err := s.BakeDetailed(map[string]any{
		"modes": []any{"1", "2"}, "fixed": []any{"2", int64(1)},
		"by_name": map[string]any{"first": "2"}, "ptr": nil,
	})
	if err != nil || !result.Ok() {
		t.Fatalf("bake: %v %v", result, err)
	}

	var got enumConfig
	if err := baked.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Mode != descriptorpb.FieldDescriptorProto_TYPE_FLOAT || got.Modes[0] != descriptorpb.FieldDescriptorProto_TYPE_DOUBLE || got.Modes[1] != got.Mode || got.ByName["first"] != got.Mode || got.Fixed[0] != got.Mode || got.Ptr != nil {
		t.Fatalf("decode: %+v", got)
	}

	if len(report.Events) != 5 || !proto.Equal(baked.Values.Fields["mode"], sp.Int32V(2)) {
		t.Fatalf("default/coercion report or wire kind: %v %v", report, baked.Values)
	}

	for _, input := range []map[string]any{{"mode": "999"}, {"modes": []any{"999"}}, {"by_name": map[string]any{"a": "999"}}} {
		if b, r, err := s.Bake(input); err != nil || b != nil || r.Ok() {
			t.Fatalf("unknown enum accepted: %v %v", r, err)
		}
	}
}

func TestEnumOverridesAndSubset(t *testing.T) {
	t.Parallel()

	type config struct {
		Mode descriptorpb.FieldDescriptorProto_Type `json:"mode" validate:"oneof=2 1"`
	}

	s, err := sp.ReflectType[config](sp.ID("test", "subset", sp.Ver(1, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}

	options := s.Fields[0].GetChoice().GetOptions()
	if len(options) != 2 || options[0].Label != "TYPE_DOUBLE" || options[1].Label != "TYPE_FLOAT" {
		t.Fatalf("subset lost labels/declaration order: %v", options)
	}

	type invalid struct {
		Mode descriptorpb.FieldDescriptorProto_Type `validate:"oneof=999"`
	}

	if _, err := sp.ReflectType[invalid](sp.ID("test", "invalid", sp.Ver(1, 0, 0))); err == nil {
		t.Fatal("unknown enum restriction accepted")
	}

	s, err = sp.ReflectType[config](sp.ID("test", "override", sp.Ver(1, 0, 0)), sp.WithType(reflect.TypeFor[descriptorpb.FieldDescriptorProto_Type](), func(name sp.FieldName) *sp.Schema_Field {
		return sp.Choice(name).Opt(sp.Int32V(2), "Custom label").Done()
	}))
	if err != nil || s.Fields[0].GetChoice().Options[0].Label != "Custom label" {
		t.Fatalf("explicit override lost: %v %v", s, err)
	}
}

func TestEnumDescriptorAliases(t *testing.T) {
	t.Parallel()

	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("enum_test.proto"), Syntax: proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Mode"), Options: &descriptorpb.EnumOptions{AllowAlias: proto.Bool(true)}, Value: []*descriptorpb.EnumValueDescriptorProto{
			{Name: proto.String("UNKNOWN"), Number: proto.Int32(0)},
			{Name: proto.String("FAST"), Number: proto.Int32(2)},
			{Name: proto.String("QUICK"), Number: proto.Int32(2)},
			{Name: proto.String("DISABLED"), Number: proto.Int32(-1)},
		}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	options := sp.Enum("mode", file.Enums().Get(0)).Done().GetChoice().GetOptions()
	if len(options) != 3 || options[1].Label != "FAST" || options[2].Value.GetInt32Value() != -1 {
		t.Fatalf("wrong descriptor options: %v", options)
	}
}

func TestIntegerChoiceJSONNumbers(t *testing.T) {
	t.Parallel()

	s := sp.NewSchema(sp.ID("test", "json_enum", sp.Ver(1, 0, 0))).Fields(sp.Choice("mode").StrOpts("2").Opt(sp.Int32V(2), "two")).MustBuild()
	for _, number := range []json.Number{"2", "2.0", "2e0"} {
		baked, result, report, err := s.BakeDetailed(map[string]any{"mode": number})
		if err != nil || !result.Ok() || !proto.Equal(baked.Values.Fields["mode"], sp.Int32V(2)) || len(report.Events) != 0 {
			t.Fatalf("numeric JSON %s requires coercion or lost integer type: %v %v", number, result, err)
		}
	}

	for _, number := range []json.Number{"2.1", "3", "1e1000000"} {
		if b, result, err := s.Bake(map[string]any{"mode": number}); err != nil || b != nil || result.Ok() {
			t.Fatalf("invalid numeric choice accepted: %s %v %v", number, result, err)
		}
	}
}
