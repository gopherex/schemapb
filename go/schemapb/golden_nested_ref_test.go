package schemapb_test

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

// Inputs and outputs are typed protoJSON, shared verbatim by all four runners.
// Keep explicit expected values here: regenerating must not bless a regression.
type nestedRefGoldenCase struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result json.RawMessage `json:"result"`
	Baked  json.RawMessage `json:"baked,omitempty"`
}

func nestedRefSchema() *sp.Schema {
	segmentFields := func(rules bool) []sp.FieldDef {
		run := sp.Object("run", sp.Str("executor").Default("local"))
		if rules {
			run.Rules(sp.Rule("this.executor == 'local'", "executor must be local"))
		}

		return []sp.FieldDef{
			run,
			sp.Str("log_level").Default(" INFO ").Normalize("this.trim().lowerAscii()"),
			sp.Int32("replicas").Default(1),
			sp.UInt32("workers").Default(2),
			sp.Float("ratio").Default(1.25),
			sp.Duration("timeout").Default(1500 * time.Millisecond),
			sp.Computed("derived", "root.factor * 3").Result(sp.ResultInt64),
			sp.Ref("limits", "limits"),
			sp.List("children", sp.Ref("", "limits")),
			sp.Object("optional", sp.Str("required_child").Required()),
			sp.Str("inactive").Default("hidden").When("false"),
		}
	}
	s := sp.NewSchema(sp.ID("conformance", "nested_ref", sp.Ver(1, 0, 0))).Coerce().
		Def("segment", segmentFields(true)...).
		Def("quiet", segmentFields(false)...).
		Def("limits", sp.Int32("cpu").Default(2), sp.Computed("derived", "'leaf'").Result(sp.ResultString)).
		Fields(
			sp.Int64("factor").Default(2),
			sp.List("segments", sp.Ref("", "segment").Nullable()),
			sp.List("identity_segments", sp.RefID("", sp.ID("workload", "segment", sp.Ver(1, 0, 0)))),
			sp.List("pair", sp.Ref("", "segment"), sp.Ref("", "limits")),
			sp.Ref("direct", "segment"),
			sp.Object("wrapper", sp.Ref("segment", "segment")),
			sp.Map("mapped", sp.Ref("segment", "segment")),
			sp.OneOf("variant", "kind").Variant("run", sp.Str("kind"), sp.Ref("segment", "segment")),
			sp.List("variants", sp.OneOf("", "kind").Variant("run", sp.Str("kind"), sp.Ref("segment", "segment"))),
			sp.List("quiet_segments", sp.Ref("", "quiet")),
		).MustBuild()
	// Identity refs use the same root-def key as Link; no registry/network needed.
	s.Defs["workload\x00segment\x00v1.0.0"] = proto.Clone(s.Defs["segment"]).(*sp.Schema)

	return s
}

func nestedRefWantSegment(run, limits bool) *sp.Value {
	fields := map[string]*sp.Value{
		"log_level": sp.StrV("info"), "replicas": sp.Int32V(1),
		"workers": sp.UInt32V(2), "ratio": sp.FloatV(1.25),
		"timeout": sp.DurationV(1500 * time.Millisecond), "derived": sp.Int64V(6),
	}
	if run {
		fields["run"] = sp.StructV(map[string]*sp.Value{"executor": sp.StrV("local")})
	}

	if limits {
		fields["limits"] = nestedRefWantLimits()
	}

	return sp.StructV(fields)
}

func nestedRefWantLimits() *sp.Value {
	return sp.StructV(map[string]*sp.Value{"cpu": sp.Int32V(2), "derived": sp.StrV("leaf")})
}

//nolint:paralleltest,tparallel // subtests assemble one ordered golden fixture
func TestGoldenNestedRef(t *testing.T) {
	t.Parallel()

	schema := nestedRefSchema()

	e, err := sp.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "nested-ref-schema.json", stableJSON(t, schema))

	empty := func() map[string]any { return map[string]any{} }
	input := func() map[string]any { return map[string]any{"run": empty(), "limits": empty()} }
	want := func() *sp.Value { return nestedRefWantSegment(true, true) }
	object := func(v *sp.Value) *sp.Value { return sp.StructV(map[string]*sp.Value{"segment": v}) }
	variant := func() *sp.Value { v := object(want()); v.GetStructValue().Fields["kind"] = sp.StrV("run"); return v }
	adjusted := want()
	adjusted.GetStructValue().Fields["replicas"] = sp.Int32V(3)
	adjusted.GetStructValue().Fields["timeout"] = sp.DurationV(2 * time.Second)
	adjusted.GetStructValue().Fields["log_level"] = sp.StrV("debug")
	adjustedInput := input()
	adjustedInput["replicas"] = "3"
	adjustedInput["timeout"] = "2s"
	adjustedInput["log_level"] = " DEBUG "
	withChildren := want()
	withChildren.GetStructValue().Fields["children"] = sp.ListV(nestedRefWantLimits(), nestedRefWantLimits())
	childrenInput := input()
	childrenInput["children"] = []any{empty(), empty()}

	cases := []struct {
		name, field string
		input       any
		want        *sp.Value
		errorPath   string
		errorCode   sp.ErrorCode
	}{
		{name: "named-list", field: "segments", input: []any{input(), adjustedInput}, want: sp.ListV(want(), adjusted)},
		{name: "identity-list", field: "identity_segments", input: []any{input()}, want: sp.ListV(want())},
		{name: "tuple", field: "pair", input: []any{input(), empty()}, want: sp.ListV(want(), nestedRefWantLimits())},
		{name: "direct-ref", field: "direct", input: input(), want: want()},
		{name: "nested-object", field: "wrapper", input: map[string]any{"segment": input()}, want: object(want())},
		{name: "map-schema", field: "mapped", input: map[string]any{"a": map[string]any{"segment": input()}}, want: sp.StructV(map[string]*sp.Value{"a": object(want())})},
		{name: "oneof", field: "variant", input: map[string]any{"kind": "run", "segment": input()}, want: variant()},
		{name: "list-oneof", field: "variants", input: []any{map[string]any{"kind": "run", "segment": input()}}, want: sp.ListV(variant())},
		{name: "without-rules", field: "quiet_segments", input: []any{input()}, want: sp.ListV(want())},
		{name: "nested-list", field: "segments", input: []any{childrenInput}, want: sp.ListV(withChildren)},
		{name: "absent-objects", field: "segments", input: []any{empty()}, want: sp.ListV(nestedRefWantSegment(false, false))},
		{name: "empty-list", field: "segments", input: []any{}, want: sp.ListV()},
		{name: "null-item", field: "segments", input: []any{nil}, want: sp.ListV(sp.NullV())},
		{name: "absent-list"},
		{name: "rule-failure", field: "segments", input: []any{map[string]any{"run": map[string]any{"executor": "remote"}}}, errorPath: "segments[0].run", errorCode: sp.ErrorCode_ERROR_CODE_RULE_VIOLATED},
		{name: "wrong-item-type", field: "segments", input: []any{int64(7)}, errorPath: "segments[0]", errorCode: sp.ErrorCode_ERROR_CODE_TYPE_MISMATCH},
	}

	golden := make([]nestedRefGoldenCase, 0, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{}
			if tc.field != "" {
				values[tc.field] = tc.input
			}

			wireInput, err := sp.StructFromGo(values)
			if err != nil {
				t.Fatal(err)
			}

			entry := nestedRefGoldenCase{Name: tc.name, Input: stableJSON(t, wireInput)}

			baked, res, err := e.Bake(wireInput.ToGo())
			if err != nil {
				t.Fatal(err)
			}

			if tc.errorPath != "" {
				if baked != nil || len(res.Errors) != 1 || !hasCode(res, tc.errorPath, tc.errorCode) {
					t.Fatalf("expected blocking %s at %s, got baked=%v, %v", tc.errorCode, tc.errorPath, baked != nil, res)
				}

				entry.Result = stableJSON(t, res)
				golden = append(golden, entry)

				return
			}

			if baked == nil || !res.Ok() {
				t.Fatalf("bake: %v", res)
			}

			expected := &sp.StructValue{Fields: map[string]*sp.Value{"factor": sp.Int64V(2)}}
			if tc.field != "" {
				expected.Fields[tc.field] = tc.want
			}

			if !proto.Equal(baked.GetValues(), expected) {
				t.Fatalf("values:\n%s\nwant:\n%s", stableJSON(t, baked.GetValues()), stableJSON(t, expected))
			}
			// Resolve independently, then Bake; also re-bake a wire snapshot.
			resolved, resolution := e.Resolve(wireInput.ToGo())
			if !resolution.Ok() {
				t.Fatalf("resolve: %v", resolution)
			}

			for _, againInput := range []map[string]any{resolved, baked.GetValues().ToGo()} {
				again, r, err := e.Bake(againInput)
				if err != nil || !r.Ok() || !proto.Equal(again.GetValues(), expected) {
					t.Fatalf("rebake: %v %v %v", again, r, err)
				}
			}

			entry.Baked = stableJSON(t, baked.GetValues())
			entry.Result = stableJSON(t, res)
			golden = append(golden, entry)
		})
	}

	raw, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "nested-ref-cases.json", append(raw, '\n'))
}
