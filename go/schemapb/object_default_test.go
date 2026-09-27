package schemapb_test

import (
	"encoding/json"
	"testing"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

func TestReflectEmptyObjectDefault(t *testing.T) {
	t.Parallel()

	type database struct {
		Port int64  `json:"port" schemapb:"default=5432"`
		Blob []byte `json:"blob" schemapb:"default=AQI="`
	}

	type config struct {
		DB database `json:"db" schemapb:"default={}"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	a, ar, err := s.Resolve(map[string]any{})
	if err != nil || !ar.Ok() {
		t.Fatalf("%v %v", ar, err)
	}

	b, br, err := s.Resolve(map[string]any{})
	if err != nil || !br.Ok() {
		t.Fatalf("%v %v", br, err)
	}

	first := a["db"].(map[string]any)
	first["port"] = int64(1)
	first["blob"].([]byte)[0] = 9

	second := b["db"].(map[string]any)
	if second["port"] != int64(5432) || second["blob"].([]byte)[0] != 1 {
		t.Fatal("defaults share mutable data")
	}

	baked, res, err := s.Bake(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("nil input: %v %v", res, err)
	}

	var got config
	if err := baked.Decode(&got); err != nil || got.DB.Port != 5432 {
		t.Fatalf("decode: %v %+v", err, got)
	}
}

func TestGoldenObjectDefaultErrors(t *testing.T) {
	t.Parallel()

	type entry struct {
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
	}

	cases := make([]entry, 0, 2)

	for _, kind := range []string{"object", "ref"} {
		s := sp.NewSchema(sp.ID("conformance", "invalid_default", sp.Ver(1, 0, 0))).Def("db", sp.Int64("port")).Fields(sp.Object("db", sp.Int64("port"))).MustBuild()

		field := s.Fields[0]
		if kind == "ref" {
			field = sp.Ref("db", "db").Done()
			s.Fields[0] = field
		}

		nonempty := &sp.StructValue{Fields: map[string]*sp.Value{"port": sp.Int64V(5432)}}
		if kind == "object" {
			field.GetObject().Default = nonempty
		} else {
			field.GetRef().Default = nonempty
		}

		if err := s.CheckDescriptor(); err == nil {
			t.Fatal("nonempty default accepted")
		}

		if _, err := sp.Compile(s); err == nil {
			t.Fatal("invalid default compiled")
		}

		cases = append(cases, struct {
			Name   string          `json:"name"`
			Schema json.RawMessage `json:"schema"`
		}{kind, stableJSON(t, s)})
	}

	raw, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "object-default-errors.json", append(raw, '\n'))
}

func TestObjectDefaultNestedBytesOwnership(t *testing.T) {
	t.Parallel()

	payload := sp.StructV(map[string]*sp.Value{"blob": sp.BytesV([]byte{1, 2})})
	schema := sp.NewSchema(testID(t)).Fields(sp.Object("db", sp.JSON("payload").Default(payload)).DefaultEmpty()).MustBuild()

	first, res, err := schema.Resolve(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("%v %v", res, err)
	}

	first["db"].(map[string]any)["payload"].(map[string]any)["blob"].([]byte)[0] = 9

	second, res, err := schema.Resolve(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("%v %v", res, err)
	}

	if second["db"].(map[string]any)["payload"].(map[string]any)["blob"].([]byte)[0] != 1 {
		t.Fatal("JSON default bytes alias another resolution")
	}
}
