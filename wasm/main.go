//go:build js && wasm

// Command schemapb-wasm exposes the schemapb validator + computer to JavaScript.
// Build:
//
//	GOOS=js GOARCH=wasm go build -o ts/schemapb.wasm ./wasm  (see `make wasm`)
//
// It registers two global functions, each taking two JSON strings (the schema
// as protojson, the form values as JSON) and returning a JSON string:
//
//	schemapbValidate(schemaJSON, valuesJSON) -> {"ok":bool,"errors":[...]}
//	schemapbCompute(schemaJSON, valuesJSON)  -> {"computed":{...},"errors":[...]}
//
// On a malformed schema or bad JSON, the result is {"error":"..."}.
package main

import (
	"encoding/json"
	"syscall/js"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
)

func main() {
	js.Global().Set("schemapbValidate", js.FuncOf(validate))
	js.Global().Set("schemapbCompute", js.FuncOf(compute))
	select {} // keep the Go runtime alive for the registered callbacks
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
	v, schemaErr := newValidator(args)
	if schemaErr != "" {
		return fail(schemaErr)
	}
	fes, err := v.ValidateJSON(json.RawMessage(args[1].String()))
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"ok": len(fes) == 0, "errors": errs(fes)})
}

func compute(_ js.Value, args []js.Value) any {
	v, schemaErr := newValidator(args)
	if schemaErr != "" {
		return fail(schemaErr)
	}
	computed, fes, err := v.ComputeJSON(json.RawMessage(args[1].String()))
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"computed": computed, "errors": errs(fes)})
}

func newValidator(args []js.Value) (*schemapb.Validator, string) {
	if len(args) < 2 {
		return nil, "expected (schemaJSON, valuesJSON)"
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal([]byte(args[0].String()), &s); err != nil {
		return nil, "parse schema: " + err.Error()
	}
	v, err := schemapb.NewValidator(&s)
	if err != nil {
		return nil, err.Error()
	}
	return v, ""
}

func result(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func fail(msg string) string {
	return result(map[string]any{"error": msg})
}
