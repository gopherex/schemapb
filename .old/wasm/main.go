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
//	schemapbLink(schemaJSON, registrySchemasJSON)         -> {"schema":{...}}
//	schemapbHash(bakedJSON)                               -> {"hash":"<64-hex-char sha256>"}
//	   ("values" = the fully resolved form; "baked" = the sealed Baked, omitted
//	    when errors block sealing; "schema" = the linked, self-contained schema;
//	    "hash" = the same schemapb.Hash/HashPB digest the Go server computes —
//	    hex-encoded SHA-256 — so a browser-baked and server-baked snapshot of
//	    the same schema+values are byte-comparable.)
//
// On a malformed schema or bad JSON, the result is {"error":"..."}.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	js.Global().Set("schemapbLink", js.FuncOf(link))
	js.Global().Set("schemapbFieldActive", js.FuncOf(fieldActive))
	js.Global().Set("schemapbEnumOptions", js.FuncOf(enumOptions))
	js.Global().Set("schemapbListCount", js.FuncOf(listCount))
	js.Global().Set("schemapbRender", js.FuncOf(render))
	js.Global().Set("schemapbHash", js.FuncOf(hashBaked))
	select {} // keep the Go runtime alive for the registered callbacks
}

// render: schemapbRender(schemaJSON, valuesJSON, templateName) ->
// {"text":string} or {"error":...}. Mirrors Schema.Render so the browser
// renders the schema's named template identically to the Go server.
func render(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("expected (schemaJSON, valuesJSON, templateName)")
	}
	s, e := parseSchemaOnly(args[0])
	if e != "" {
		return fail(e)
	}
	values, err := parseRoot(args[1])
	if err != nil {
		return fail("parse values: " + err.Error())
	}
	text, err := s.Render(args[2].String(), values)
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"text": text})
}

// parseSchemaOnly parses + validates a schema from a single arg.
func parseSchemaOnly(arg js.Value) (*schemapb.Schema, string) {
	var s schemapb.Schema
	if err := protojson.Unmarshal([]byte(arg.String()), &s); err != nil {
		return nil, "parse schema: " + err.Error()
	}
	if errs := s.IsValid(); len(errs) > 0 {
		return nil, (&schemapb.SchemaError{Errors: errs}).Error()
	}
	return &s, ""
}

// parseRoot parses a form object (the `root` map) from an arg.
func parseRoot(arg js.Value) (map[string]any, error) {
	root := map[string]any{}
	if str := arg.String(); str != "" && str != "null" {
		if err := json.Unmarshal([]byte(str), &root); err != nil {
			return nil, err
		}
	}
	return root, nil
}

// fieldActive: schemapbFieldActive(schemaJSON, fieldName, rootJSON) ->
// {"active":bool} or {"error":...}. Mirrors Schema.FieldActive (renderer helper).
func fieldActive(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("expected (schemaJSON, fieldName, rootJSON)")
	}
	s, e := parseSchemaOnly(args[0])
	if e != "" {
		return fail(e)
	}
	root, err := parseRoot(args[2])
	if err != nil {
		return fail("parse root: " + err.Error())
	}
	active, err := s.FieldActive(args[1].String(), root)
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"active": active})
}

// enumOptions: schemapbEnumOptions(schemaJSON, fieldName, rootJSON) ->
// {"options":[int...]}. Mirrors Schema.EnumOptions.
func enumOptions(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("expected (schemaJSON, fieldName, rootJSON)")
	}
	s, e := parseSchemaOnly(args[0])
	if e != "" {
		return fail(e)
	}
	root, err := parseRoot(args[2])
	if err != nil {
		return fail("parse root: " + err.Error())
	}
	opts, err := s.EnumOptions(args[1].String(), root)
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"options": opts})
}

// listCount: schemapbListCount(schemaJSON, fieldName, rootJSON) ->
// {"count":int}. Mirrors Schema.ListCount.
func listCount(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("expected (schemaJSON, fieldName, rootJSON)")
	}
	s, e := parseSchemaOnly(args[0])
	if e != "" {
		return fail(e)
	}
	root, err := parseRoot(args[2])
	if err != nil {
		return fail("parse root: " + err.Error())
	}
	n, err := s.ListCount(args[1].String(), root)
	if err != nil {
		return fail(err.Error())
	}
	return result(map[string]any{"count": n})
}

// link resolves all identity-Refs in schemaJSON against a registry built from
// registrySchemasJSON (a JSON array of schema protojson objects), returning the
// linked, self-contained schema: {"schema": <protojson>} or {"error": "..."}.
func link(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return fail("expected (schemaJSON, registrySchemasJSON)")
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal([]byte(args[0].String()), &s); err != nil {
		return fail("parse schema: " + err.Error())
	}
	var raw []json.RawMessage
	if str := args[1].String(); str != "" && str != "null" {
		if err := json.Unmarshal([]byte(str), &raw); err != nil {
			return fail("parse registry: " + err.Error())
		}
	}
	ctx := context.Background()
	reg := schemapb.NewInMemoryRegistry()
	for i, r := range raw {
		var rs schemapb.Schema
		if err := protojson.Unmarshal(r, &rs); err != nil {
			return fail(fmt.Sprintf("parse registry[%d]: %v", i, err))
		}
		if err := reg.Put(ctx, &rs); err != nil {
			return fail(fmt.Sprintf("register[%d]: %v", i, err))
		}
	}
	linked, err := s.Link(ctx, reg)
	if err != nil {
		return fail(err.Error())
	}
	b, err := protojson.Marshal(linked)
	if err != nil {
		return fail("marshal linked: " + err.Error())
	}
	return result(map[string]any{"schema": json.RawMessage(b)})
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

// hashBaked: schemapbHash(bakedJSON) -> {"hash":"<hex>"} or {"error":"..."}.
// Hashes a Baked via the same schemapb.Hash (HashPB-based) code path the Go
// server uses, so a browser-baked and server-baked snapshot of the same
// schema+values are byte-comparable. No hashing logic is reimplemented here.
func hashBaked(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("expected (bakedJSON)")
	}
	var b schemapb.Baked
	if err := protojson.Unmarshal([]byte(args[0].String()), &b); err != nil {
		return fail("parse baked: " + err.Error())
	}
	sum := schemapb.Hash(&b)
	return result(map[string]any{"hash": hex.EncodeToString(sum[:])})
}

type fieldError struct {
	Field    string            `json:"field"`
	Message  string            `json:"message"`
	RuleID   string            `json:"ruleId,omitempty"`
	Severity string            `json:"severity"`
	Code     string            `json:"code,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

func errs(in []*schemapb.FieldError) []fieldError {
	out := make([]fieldError, 0, len(in))
	for _, e := range in {
		out = append(out, fieldError{
			Field:    e.GetField(),
			Message:  e.GetMessage(),
			RuleID:   e.GetRuleId(),
			Severity: e.GetSeverity().String(),
			Code:     e.GetCode(),
			Params:   e.GetParams(),
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
