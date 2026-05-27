//go:build js && wasm

// Command schemapb-wasm exposes the schemapb validator + computer to JavaScript.
// Build:
//
//	GOOS=js GOARCH=wasm go build -o ts/schemapb.wasm ./wasm  (see `make wasm`)
//
// It registers these global functions (JSON strings in, JSON string out):
//
//	schemapbValidate(schemaJSON, valuesJSON) -> {"ok":bool,"errors":[...]}
//	schemapbCompute(schemaJSON, valuesJSON)  -> {"values":{...},"errors":[...]}
//	schemapbBake(schemaJSON, valuesJSON)     -> {"baked":{schema,values},"errors":[...]}
//	schemapbMerge(bakedJSON, overridesJSON, replaceLists) -> {"baked":{...},"errors":[...]}
//	   ("values" = the fully resolved form; "baked" = the sealed Baked, omitted
//	    when errors block sealing)
//
// On a malformed schema or bad JSON, the result is {"error":"..."}.
package main

import (
	"encoding/json"
	"syscall/js"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/stroppy-io/schemapb/schemapb"
)

func main() {
	js.Global().Set("schemapbValidate", js.FuncOf(validate))
	js.Global().Set("schemapbCompute", js.FuncOf(compute))
	js.Global().Set("schemapbBake", js.FuncOf(bake))
	js.Global().Set("schemapbMerge", js.FuncOf(merge))
	select {} // keep the Go runtime alive for the registered callbacks
}

// bakeResult renders a Baked + errors. baked is omitted when nil.
func bakeResult(baked *schemapb.Baked, fes []*schemapb.FieldError) string {
	out := map[string]any{"errors": errs(fes)}
	if baked != nil {
		b, err := protojson.Marshal(baked)
		if err != nil {
			return fail("marshal baked: " + err.Error())
		}
		out["baked"] = json.RawMessage(b)
	}
	return result(out)
}

func bake(_ js.Value, args []js.Value) any {
	s, schemaErr := parseSchema(args)
	if schemaErr != "" {
		return fail(schemaErr)
	}
	values := map[string]any{}
	if err := json.Unmarshal([]byte(args[1].String()), &values); err != nil && args[1].String() != "" {
		return fail("parse values: " + err.Error())
	}
	baked, fes := s.Bake(values)
	return bakeResult(baked, fes)
}

func merge(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return fail("expected (bakedJSON, overridesJSON, replaceLists?)")
	}
	var b schemapb.Baked
	if err := protojson.Unmarshal([]byte(args[0].String()), &b); err != nil {
		return fail("parse baked: " + err.Error())
	}
	overrides := &structpb.Struct{}
	if s := args[1].String(); s != "" && s != "null" {
		if err := protojson.Unmarshal([]byte(s), overrides); err != nil {
			return fail("parse overrides: " + err.Error())
		}
	}
	replace := len(args) > 2 && args[2].Bool()
	baked, fes := b.Merge(overrides, replace)
	return bakeResult(baked, fes)
}

type fieldError struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	RuleID   string `json:"ruleId,omitempty"`
	Severity string `json:"severity"`
}

func errs(in []*schemapb.FieldError) []fieldError {
	out := make([]fieldError, 0, len(in))
	for _, e := range in {
		out = append(out, fieldError{
			Field:    e.GetField(),
			Message:  e.GetMessage(),
			RuleID:   e.GetRuleId(),
			Severity: e.GetSeverity().String(),
		})
	}
	return out
}

func validate(_ js.Value, args []js.Value) any {
	s, schemaErr := parseSchema(args)
	if schemaErr != "" {
		return fail(schemaErr)
	}
	fes, err := s.ValidateJSON(json.RawMessage(args[1].String()))
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"ok": len(fes) == 0, "errors": errs(fes)})
}

func compute(_ js.Value, args []js.Value) any {
	s, schemaErr := parseSchema(args)
	if schemaErr != "" {
		return fail(schemaErr)
	}
	values, fes, err := s.ComputeJSON(json.RawMessage(args[1].String()))
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"values": values, "errors": errs(fes)})
}

func parseSchema(args []js.Value) (*schemapb.Schema, string) {
	if len(args) < 2 {
		return nil, "expected (schemaJSON, valuesJSON)"
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal([]byte(args[0].String()), &s); err != nil {
		return nil, "parse schema: " + err.Error()
	}
	if errs := s.IsValid(); len(errs) > 0 {
		return nil, (&schemapb.SchemaError{Errors: errs}).Error()
	}
	return &s, ""
}

func result(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func fail(msg string) string {
	return result(map[string]any{"error": msg})
}
