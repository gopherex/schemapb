package schemapb

import (
	"fmt"
	"strings"
)

// SchemaError reports a malformed schema DESCRIPTOR (not invalid form values):
// missing identity, a field without a kind, duplicate names, uncompilable
// expressions, computed cycles. It carries the same proto ValidationError
// shape as runtime validation, with code ERROR_CODE_INVALID_SCHEMA.
type SchemaError struct {
	Result *ValidationResult
}

// Error implements error.
func (e *SchemaError) Error() string {
	var b strings.Builder
	b.WriteString("schemapb: invalid schema")
	for _, err := range e.Result.GetErrors() {
		b.WriteString("; ")
		if p := err.GetPath(); p != "" {
			b.WriteString(p + ": ")
		}
		b.WriteString(err.GetMessage())
	}
	return b.String()
}

// schemaErr builds one descriptor-level ValidationError.
func schemaErr(path, msg string) *ValidationError {
	return &ValidationError{
		Path:     path,
		Code:     ErrorCode_ERROR_CODE_INVALID_SCHEMA,
		Severity: SeverityError,
		Message:  msg,
	}
}

// CheckDescriptor verifies the schema descriptor itself is well-formed. It
// returns nil or a *SchemaError. Structural checks live here; expression
// compilation and computed-cycle checks are added by Compile (which calls
// this first).
func (s *Schema) CheckDescriptor() error {
	var errs []*ValidationError
	if s.GetId().GetName() == "" {
		errs = append(errs, schemaErr("id.name", "schema identity name is required"))
	}
	errs = append(errs, checkFields(s.GetFields(), "")...)
	for name, def := range s.GetDefs() {
		errs = append(errs, checkFields(def.GetFields(), "$defs."+name)...)
	}
	if len(errs) > 0 {
		return &SchemaError{Result: &ValidationResult{Errors: errs}}
	}
	return nil
}

// checkFields verifies structural field well-formedness recursively.
func checkFields(fields []*Schema_Field, prefix string) []*ValidationError {
	var errs []*ValidationError
	seen := map[string]bool{}
	for _, f := range fields {
		path := joinPath(prefix, f.GetName())
		if f.GetName() == "" {
			errs = append(errs, schemaErr(prefix, "field name is required"))
			continue
		}
		if seen[f.GetName()] {
			errs = append(errs, schemaErr(path, "duplicate field name"))
		}
		seen[f.GetName()] = true
		if f.GetKind() == nil {
			errs = append(errs, schemaErr(path, "field kind is required"))
			continue
		}
		for i, r := range f.GetRules() {
			if r.GetExpr() == "" {
				errs = append(errs, schemaErr(path, fmt.Sprintf("rule[%d]: empty expression", i)))
			}
		}
		switch {
		case f.GetComputed() != nil:
			if f.GetComputed().GetExpr() == "" {
				errs = append(errs, schemaErr(path, "computed field: empty expression"))
			}
		case f.GetOneOf() != nil:
			if f.GetOneOf().GetDiscriminator() == "" {
				errs = append(errs, schemaErr(path, "oneof field: discriminator is required"))
			}
		case f.GetRef() != nil:
			if f.GetRef().GetTarget() == nil {
				errs = append(errs, schemaErr(path, "ref field: target is required"))
			}
		}
		for _, child := range nestedSchemas(f) {
			errs = append(errs, checkFields(child.GetFields(), path)...)
		}
	}
	return errs
}

// joinPath joins two path segments with a dot, tolerating empty prefixes.
func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
