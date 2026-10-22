package schemapb_test

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/gopherex/schemapb/go/schemapb"
)

// reservedSchemaJSON names fields with CEL reserved words. `this` is a map,
// so expressions reach them by index (`this["in"]`) and test presence with
// `"in" in this` — no identifier is needed.
const reservedSchemaJSON = `{
  "id": {"namespace": "conformance", "name": "reserved", "version": "v1.0.0"},
  "fields": [
    {"name": "namespace", "required": true, "string": {"minLen": "1"}},
    {"name": "in", "string": {}},
    {"name": "const", "string": {}, "when": "root[\"namespace\"] == \"strict\""},
    {"name": "function", "computed": {"expr": "size(root[\"namespace\"])", "result": "RESULT_TYPE_INT64"}},
    {"name": "while", "int64": {}, "rules": [{"expr": "this >= 0", "message": "must not be negative"}]}
  ],
  "rules": [
    {"expr": "!(\"in\" in this) || this[\"in\"] != this[\"namespace\"]", "message": "in must differ from namespace", "id": "in_ne_namespace"}
  ]
}`

// TestGoldenReservedNames pins reserved-names.json: field names that are CEL
// reserved words are valid, and expressions address them by index.
func TestGoldenReservedNames(t *testing.T) {
	t.Parallel()

	schema := &schemapb.Schema{}
	if err := protojson.Unmarshal([]byte(reservedSchemaJSON), schema); err != nil {
		t.Fatal(err)
	}

	eng, err := schemapb.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		input map[string]any
		ok    bool
	}{
		{"valid", map[string]any{"namespace": "a", "in": "b", "while": int64(1)}, true},
		{"absent in", map[string]any{"namespace": "abc"}, true},
		{"rule on index", map[string]any{"namespace": "a", "in": "a"}, false},
		{"field rule", map[string]any{"namespace": "a", "while": int64(-1)}, false},
		{"empty namespace", map[string]any{"namespace": ""}, false},
		{"when active", map[string]any{"namespace": "strict", "const": "c"}, true},
		{"when inactive", map[string]any{"namespace": "loose", "const": "c"}, true},
	}

	type entry struct {
		Name   string          `json:"name"`
		Input  json.RawMessage `json:"input"`
		Result json.RawMessage `json:"result"`
		Baked  json.RawMessage `json:"baked,omitempty"`
	}

	out := struct {
		Schema json.RawMessage `json:"schema"`
		Cases  []entry         `json:"cases"`
	}{Schema: stableJSON(t, schema)}

	for _, tc := range cases {
		input, err := schemapb.StructFromGo(tc.input)
		if err != nil {
			t.Fatal(err)
		}

		baked, res, err := eng.Bake(input.ToGo())
		if err != nil {
			t.Fatal(err)
		}

		if res.Ok() != tc.ok {
			t.Fatalf("%s: ok = %v, want %v: %v", tc.name, res.Ok(), tc.ok, res)
		}

		e := entry{Name: tc.name, Input: stableJSON(t, input), Result: stableJSON(t, res)}
		if baked != nil {
			e.Baked = stableJSON(t, baked.GetValues())
		}

		out.Cases = append(out.Cases, e)
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "reserved-names.json", append(raw, '\n'))
}
