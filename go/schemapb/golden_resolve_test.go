package schemapb_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

type resolveGoldenCase struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Input  json.RawMessage `json:"input"`
	Result json.RawMessage `json:"result"`
	Report json.RawMessage `json:"report"`
	Baked  json.RawMessage `json:"baked,omitempty"`
}

// Explicit assertions guard the oracle: updating goldens cannot bless missing
// coercion, wrong presence, repeated normalize, truncated paths or lost reports.
//
//nolint:paralleltest,tparallel // one ordered cross-language fixture
func TestGoldenRecursiveResolve(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		fields    []sp.FieldDef
		input     map[string]any
		want      map[string]*sp.Value
		events    []string
		errorPath string
		errorCode sp.ErrorCode
	}

	port := func() sp.FieldDef { return sp.Int64("port").Default(5432) }
	cases := []testCase{
		{name: "when-disabled-by-normalize", fields: []sp.FieldDef{sp.Bool("enabled").Default(true).Normalize("false"), sp.Int64("value").When("root.enabled").Normalize("this + 1")}, input: map[string]any{"value": int64(3)}, want: map[string]*sp.Value{"enabled": sp.BoolV(false), "value": sp.Int64V(3)}, events: []string{"DEFAULT_APPLIED enabled", "NORMALIZED enabled", "INACTIVE value"}},
		{name: "when-enabled-by-normalize", fields: []sp.FieldDef{sp.Bool("enabled").Default(false).Normalize("true"), sp.Ref("db", "db").DefaultEmpty().When("root.enabled")}, want: map[string]*sp.Value{"enabled": sp.BoolV(true), "db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432)})}, events: []string{"DEFAULT_APPLIED enabled", "INACTIVE db", "NORMALIZED enabled", "DEFAULT_APPLIED db", "DEFAULT_APPLIED db.port"}},
		{name: "normalize-changes-oneof-gate", fields: []sp.FieldDef{sp.OneOf("job", "kind").Variant("a", sp.Str("kind"), sp.Int64("value").Default(1).When("false")).Variant("b", sp.Str("kind"), sp.Int64("value").Default(2).When("true")).Normalize(`{"kind": "b"}`)}, input: map[string]any{"job": map[string]any{"kind": "a"}}, want: map[string]*sp.Value{"job": sp.StructV(map[string]*sp.Value{"kind": sp.StrV("b"), "value": sp.Int64V(2)})}, events: []string{"INACTIVE job.value", "NORMALIZED job", "DEFAULT_APPLIED job.value"}},

		{name: "immutable-list-element", fields: []sp.FieldDef{sp.List("ports", sp.Int64("").Default(7).Immutable())}, input: map[string]any{"ports": []any{int64(8)}}, events: []string{"DEFAULT_APPLIED ports[0]"}, errorPath: "ports[0]", errorCode: sp.ErrorCode_ERROR_CODE_IMMUTABLE_MODIFIED},
		{name: "immutable-map-ref", fields: []sp.FieldDef{sp.MapOf("tenants", sp.Ref("value", "fixed"))}, input: map[string]any{"tenants": map[string]any{"customer.a": map[string]any{"port": int64(8)}}}, events: []string{`DEFAULT_APPLIED tenants["customer.a"].port`}, errorPath: `tenants["customer.a"].port`, errorCode: sp.ErrorCode_ERROR_CODE_IMMUTABLE_MODIFIED},
		{name: "unicode-map-order", fields: []sp.FieldDef{sp.MapOf("values", sp.Int64("value"))}, input: map[string]any{"values": map[string]any{"\ue000": "1", "😀": "2"}}, want: map[string]*sp.Value{"values": sp.StructV(map[string]*sp.Value{"\ue000": sp.Int64V(1), "😀": sp.Int64V(2)})}, events: []string{"COERCED values.\ue000", "COERCED values.😀"}},

		{name: "required-nullable-object", fields: []sp.FieldDef{sp.Object("db", port()).DefaultEmpty().Required().Nullable()}, input: map[string]any{"db": nil}, want: map[string]*sp.Value{"db": sp.NullV()}},
		{name: "required-nonnullable-object", fields: []sp.FieldDef{sp.Ref("db", "db").DefaultEmpty().Required()}, input: map[string]any{"db": nil}, errorPath: "db", errorCode: sp.ErrorCode_ERROR_CODE_NOT_NULLABLE},

		{name: "inactive-input-preserved", fields: []sp.FieldDef{sp.Object("db", sp.Int64("port").When("false"), sp.Int32("cpu").Default(2))}, input: map[string]any{"db": map[string]any{"port": "not a number"}}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.StrV("not a number"), "cpu": sp.Int32V(2)})}, events: []string{"INACTIVE db.port", "DEFAULT_APPLIED db.cpu"}},
		{name: "inactive-list-item", fields: []sp.FieldDef{sp.List("ports", sp.Int64("").When("false"))}, input: map[string]any{"ports": []any{"ignored", nil}}, want: map[string]*sp.Value{"ports": sp.ListV(sp.StrV("ignored"), sp.NullV())}, events: []string{"INACTIVE ports[0]", "INACTIVE ports[1]"}},
		{name: "computed-map-key-dependency", fields: []sp.FieldDef{sp.Computed("answer", `root.values["a.b"] + 1`).Result(sp.ResultInt64), sp.MapOf("values", sp.Computed("value", "40 + 1").Result(sp.ResultInt64))}, input: map[string]any{"values": map[string]any{"a.b": int64(0)}}, want: map[string]*sp.Value{"values": sp.StructV(map[string]*sp.Value{"a.b": sp.Int64V(41)}), "answer": sp.Int64V(42)}, events: []string{`COMPUTED values["a.b"]`, "COMPUTED answer"}},
		{name: "escaped-map-keys", fields: []sp.FieldDef{sp.MapOf("values", sp.Int64("value"))}, input: map[string]any{"values": map[string]any{"": "1", `a"b`: "2", `a[b]`: "3", `a\b`: "4"}}, want: map[string]*sp.Value{"values": sp.StructV(map[string]*sp.Value{"": sp.Int64V(1), `a"b`: sp.Int64V(2), `a[b]`: sp.Int64V(3), `a\b`: sp.Int64V(4)})}, events: []string{`COERCED values[""]`, `COERCED values["a\"b"]`, `COERCED values["a[b]"]`, `COERCED values["a\\b"]`}},

		{name: "scalar-list", fields: []sp.FieldDef{sp.List("ports", sp.Int64(""))}, input: map[string]any{"ports": []any{"8080", "9007199254740993"}}, want: map[string]*sp.Value{"ports": sp.ListV(sp.Int64V(8080), sp.Int64V(9007199254740993))}, events: []string{"COERCED ports[0]", "COERCED ports[1]"}},
		{name: "duration-list", fields: []sp.FieldDef{sp.List("timeouts", sp.Duration(""))}, input: map[string]any{"timeouts": []any{"1s", "1m30s"}}, want: map[string]*sp.Value{"timeouts": sp.ListV(sp.DurationV(time.Second), sp.DurationV(90*time.Second))}, events: []string{"COERCED timeouts[0]", "COERCED timeouts[1]"}},
		{name: "scalar-map", fields: []sp.FieldDef{sp.MapOf("limits", sp.Int32("value"))}, input: map[string]any{"limits": map[string]any{"cpu": "4", "memory": "8"}}, want: map[string]*sp.Value{"limits": sp.StructV(map[string]*sp.Value{"cpu": sp.Int32V(4), "memory": sp.Int32V(8)})}, events: []string{"COERCED limits.cpu", "COERCED limits.memory"}},
		{name: "tuple", fields: []sp.FieldDef{sp.List("tuple", sp.Int64(""), sp.Duration(""), sp.Str("").Normalize("this.lowerAscii()"))}, input: map[string]any{"tuple": []any{"7", "2s", "HELLO"}}, want: map[string]*sp.Value{"tuple": sp.ListV(sp.Int64V(7), sp.DurationV(2*time.Second), sp.StrV("hello"))}, events: []string{"COERCED tuple[0]", "COERCED tuple[1]", "NORMALIZED tuple[2]"}},
		{name: "nested-containers", fields: []sp.FieldDef{sp.List("matrix", sp.MapOf("", sp.List("value", sp.Int64("").Normalize("this + 1"))))}, input: map[string]any{"matrix": []any{map[string]any{"a.b": []any{"4"}}}}, want: map[string]*sp.Value{"matrix": sp.ListV(sp.StructV(map[string]*sp.Value{"a.b": sp.ListV(sp.Int64V(5))}))}, events: []string{`COERCED matrix[0]["a.b"][0]`, `NORMALIZED matrix[0]["a.b"][0]`}},
		{name: "map-ref", fields: []sp.FieldDef{sp.MapOf("tenants", sp.Ref("value", "db"))}, input: map[string]any{"tenants": map[string]any{"customer.a": map[string]any{}}}, want: map[string]*sp.Value{"tenants": sp.StructV(map[string]*sp.Value{"customer.a": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432)})})}, events: []string{`DEFAULT_APPLIED tenants["customer.a"].port`}},
		{name: "map-oneof", fields: []sp.FieldDef{sp.MapOf("jobs", sp.OneOf("value", "kind").Variant("db", sp.Str("kind"), port()))}, input: map[string]any{"jobs": map[string]any{"first": map[string]any{"kind": "db"}}}, want: map[string]*sp.Value{"jobs": sp.StructV(map[string]*sp.Value{"first": sp.StructV(map[string]*sp.Value{"kind": sp.StrV("db"), "port": sp.Int64V(5432)})})}, events: []string{"DEFAULT_APPLIED jobs.first.port"}},
		{name: "empty-object-default", fields: []sp.FieldDef{sp.Object("db", port()).DefaultEmpty().Required()}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432)})}, events: []string{"DEFAULT_APPLIED db", "DEFAULT_APPLIED db.port"}},
		{name: "empty-ref-default", fields: []sp.FieldDef{sp.Ref("db", "db").DefaultEmpty()}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432)})}, events: []string{"DEFAULT_APPLIED db", "DEFAULT_APPLIED db.port"}},
		{name: "optional-absent", fields: []sp.FieldDef{sp.Object("db", port())}, want: map[string]*sp.Value{}},
		{name: "required-absent", fields: []sp.FieldDef{sp.Object("db", port()).Required()}, errorPath: "db", errorCode: sp.ErrorCode_ERROR_CODE_REQUIRED_MISSING},
		{name: "nullable-null", fields: []sp.FieldDef{sp.Object("db", port()).DefaultEmpty().Nullable()}, input: map[string]any{"db": nil}, want: map[string]*sp.Value{"db": sp.NullV()}},
		{name: "nonnullable-null", fields: []sp.FieldDef{sp.Ref("db", "db").DefaultEmpty()}, input: map[string]any{"db": nil}, errorPath: "db", errorCode: sp.ErrorCode_ERROR_CODE_NOT_NULLABLE},
		{name: "inactive-default", fields: []sp.FieldDef{sp.Ref("db", "db").DefaultEmpty().When("false")}, want: map[string]*sp.Value{}, events: []string{"INACTIVE db"}},
		{name: "existing-empty-object", fields: []sp.FieldDef{sp.Object("db", port()).DefaultEmpty()}, input: map[string]any{"db": map[string]any{}}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432)})}, events: []string{"DEFAULT_APPLIED db.port"}},
		{name: "existing-object", fields: []sp.FieldDef{sp.Ref("db", "db").DefaultEmpty()}, input: map[string]any{"db": map[string]any{"port": int64(1)}}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(1)})}},
		{name: "null-item-preserved", fields: []sp.FieldDef{sp.List("ports", sp.Int64("").Default(1).Nullable())}, input: map[string]any{"ports": []any{nil}}, want: map[string]*sp.Value{"ports": sp.ListV(sp.NullV())}},
		{name: "empty-list-preserved", fields: []sp.FieldDef{sp.List("ports", sp.Int64("").Default(1))}, input: map[string]any{"ports": []any{}}, want: map[string]*sp.Value{"ports": sp.ListV()}},
		{name: "absent-list-preserved", fields: []sp.FieldDef{sp.List("ports", sp.Int64("").Default(1))}, want: map[string]*sp.Value{}},
		{name: "normalize-object-error", fields: []sp.FieldDef{sp.Object("db", sp.Int64("value").Normalize("this.missing"))}, input: map[string]any{"db": map[string]any{"value": int64(1)}}, errorPath: "db.value", errorCode: sp.ErrorCode_ERROR_CODE_EXPR_ERROR},
		{name: "normalize-list-error", fields: []sp.FieldDef{sp.List("servers", sp.Object("", sp.Int64("timeout").Normalize("this.missing")))}, input: map[string]any{"servers": []any{map[string]any{}, map[string]any{}, map[string]any{"timeout": int64(1)}}}, errorPath: "servers[2].timeout", errorCode: sp.ErrorCode_ERROR_CODE_EXPR_ERROR},
		{name: "normalize-map-error", fields: []sp.FieldDef{sp.MapOf("tenants", sp.Ref("value", "bad"))}, input: map[string]any{"tenants": map[string]any{"customer.a": map[string]any{"timeout": int64(1)}}}, errorPath: `tenants["customer.a"].timeout`, errorCode: sp.ErrorCode_ERROR_CODE_EXPR_ERROR},
		{name: "when-error", fields: []sp.FieldDef{sp.MapOf("tenants", sp.Int64("value").When("root.missing"))}, input: map[string]any{"tenants": map[string]any{"customer.a": int64(1)}}, events: []string{`INACTIVE tenants["customer.a"]`}, errorPath: `tenants["customer.a"]`, errorCode: sp.ErrorCode_ERROR_CODE_EXPR_ERROR},
		{name: "computed-error", fields: []sp.FieldDef{sp.List("servers", sp.Object("", sp.Computed("url", "root.missing").Result(sp.ResultString)))}, input: map[string]any{"servers": []any{map[string]any{}}}, errorPath: "servers[0].url", errorCode: sp.ErrorCode_ERROR_CODE_EXPR_ERROR},
		{name: "report-does-not-repeat-normalize", fields: []sp.FieldDef{sp.Int64("value").Default(0).Normalize("this + 1").Secret(), sp.Computed("derived", "root.value + 1").Result(sp.ResultInt64)}, want: map[string]*sp.Value{"value": sp.Int64V(1), "derived": sp.Int64V(2)}, events: []string{"DEFAULT_APPLIED value", "NORMALIZED value", "COMPUTED derived"}},
		{name: "unchanged-normalize", fields: []sp.FieldDef{sp.Int64("value").Normalize("this")}, input: map[string]any{"value": int64(3)}, want: map[string]*sp.Value{"value": sp.Int64V(3)}},
		{name: "computed-collection-dependency", fields: []sp.FieldDef{sp.Computed("answer", "root.numbers[0] + 1").Result(sp.ResultInt64), sp.List("numbers", sp.Computed("", "40 + 1").Result(sp.ResultInt64))}, input: map[string]any{"numbers": []any{int64(0)}}, want: map[string]*sp.Value{"numbers": sp.ListV(sp.Int64V(41)), "answer": sp.Int64V(42)}, events: []string{"COMPUTED numbers[0]", "COMPUTED answer"}},
		{name: "normalize-replaces-container", fields: []sp.FieldDef{sp.Object("db", port(), sp.Computed("label", "'ready'").Result(sp.ResultString)).Normalize("{}")}, input: map[string]any{"db": map[string]any{"port": int64(9)}}, want: map[string]*sp.Value{"db": sp.StructV(map[string]*sp.Value{"port": sp.Int64V(5432), "label": sp.StrV("ready")})}, events: []string{"NORMALIZED db", "DEFAULT_APPLIED db.port", "COMPUTED db.label"}},
	}

	var golden []resolveGoldenCase

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sp.NewSchema(sp.ID("conformance", "recursive_resolve", sp.Ver(1, 0, 0))).Coerce().Def("db", port()).Def("fixed", sp.Int64("port").Default(7).Immutable()).Def("bad", sp.Int64("timeout").Normalize("this.missing")).Fields(tc.fields...).MustBuild()
			input := sp.MustStructFromGo(tc.input)

			b, r, report, err := s.BakeDetailed(input.ToGo())
			if err != nil {
				t.Fatal(err)
			}

			var events []string
			for _, event := range report.Events {
				events = append(events, strings.TrimPrefix(event.Operation.String(), "RESOLVE_OPERATION_")+" "+event.Path)
			}

			if !reflect.DeepEqual(events, tc.events) {
				t.Fatalf("events %v want %v", events, tc.events)
			}

			if tc.errorPath != "" {
				if b != nil || len(r.Errors) != 1 || !hasCode(r, tc.errorPath, tc.errorCode) {
					t.Fatalf("expected %s %s: %v", tc.errorPath, tc.errorCode, r)
				}
			} else if !r.Ok() || b == nil || !proto.Equal(b.Values, &sp.StructValue{Fields: tc.want}) {
				t.Fatalf("bake: %v %v; want %v", b, r, tc.want)
			}

			plain, pr, err := s.Bake(input.ToGo())
			if err != nil || !proto.Equal(plain, b) || !proto.Equal(pr, r) {
				t.Fatalf("detailed changed semantics: %v", err)
			}
			// Runtime-specific CEL wording is informational, never conformance data.
			for _, failure := range r.Errors {
				failure.Message = ""
			}

			entry := resolveGoldenCase{Name: tc.name, Schema: stableJSON(t, s), Input: stableJSON(t, input), Result: stableJSON(t, r), Report: stableJSON(t, report)}
			if b != nil {
				entry.Baked = stableJSON(t, b.Values)
			}

			golden = append(golden, entry)
		})
	}

	raw, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "recursive-resolve.json", append(raw, '\n'))
}
