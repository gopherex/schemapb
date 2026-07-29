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
	errs = append(errs, checkRefTargets(s.GetFields(), s.GetDefs(), "")...)
	for name, def := range s.GetDefs() {
		errs = append(errs, checkRefTargets(def.GetFields(), s.GetDefs(), "$defs."+name)...)
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
			if len(f.GetOneOf().GetVariants()) == 0 {
				errs = append(errs, schemaErr(path, "oneof field: at least one variant is required"))
			}
		case f.GetRef() != nil:
			if f.GetRef().GetTarget() == nil {
				errs = append(errs, schemaErr(path, "ref field: target is required"))
			}
		case f.GetList() != nil:
			if len(f.GetList().GetItems()) == 0 {
				errs = append(errs, schemaErr(path, "list field: at least one item definition is required"))
			}
		case f.GetChoice() != nil:
			ch := f.GetChoice()
			if !ch.GetOpen() && len(ch.GetOptions()) == 0 && ch.GetOptionsExpr() == "" {
				errs = append(errs, schemaErr(path, "choice field: a closed choice requires options or options_expr"))
			}
			for i, o := range ch.GetOptions() {
				if o.GetValue() == nil {
					errs = append(errs, schemaErr(path, fmt.Sprintf("choice option[%d]: value is required", i)))
				}
			}
		case f.GetMap() != nil:
			mp := f.GetMap()
			if mp.MinEntries != nil && mp.MaxEntries != nil && *mp.MinEntries > *mp.MaxEntries {
				errs = append(errs, schemaErr(path, "map field: min_entries must be <= max_entries"))
			}
		}
		for _, child := range nestedSchemas(f) {
			errs = append(errs, checkFields(child.GetFields(), path)...)
		}
	}
	return errs
}

// checkRefTargets walks fields recursively and reports any name-Ref that
// targets a def absent from rootDefs. Identity-refs are exempt: they resolve
// at Link time, not build time.
func checkRefTargets(fields []*Schema_Field, rootDefs map[string]*Schema, prefix string) []*ValidationError {
	var errs []*ValidationError
	for _, f := range fields {
		path := joinPath(prefix, f.GetName())
		if ref := f.GetRef(); ref != nil && ref.GetId() == nil && ref.GetName() != "" {
			if _, ok := rootDefs[ref.GetName()]; !ok {
				errs = append(errs, schemaErr(path, fmt.Sprintf("ref %q is not defined in schema defs", ref.GetName())))
			}
		}
		if l := f.GetList(); l != nil {
			// Item fields carry their own nested schemas; recursing the items
			// covers both (nestedSchemas would double-visit them).
			errs = append(errs, checkRefTargets(l.GetItems(), rootDefs, path+"[]")...)
			continue
		}
		for _, child := range nestedSchemas(f) {
			errs = append(errs, checkRefTargets(child.GetFields(), rootDefs, path)...)
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
