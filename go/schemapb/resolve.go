package schemapb

import (
	"fmt"
	"maps"
	"slices"
)

// resolveState is local to one execution. It never lives on the shared Engine.
type resolveState struct {
	engine     *Engine
	root       map[string]any
	result     *ValidationResult
	report     *ResolveReport
	tasks      []computeTask
	gates      map[string]bool
	gateErrors map[string]bool
	inactive   map[string]bool
}

func (e *Engine) resolveDetailed(values map[string]any, report *ResolveReport) (map[string]any, *ValidationResult) {
	if values == nil {
		values = map[string]any{}
	}

	r := &resolveState{
		engine: e, root: values, result: &ValidationResult{}, report: report,
		gates: map[string]bool{}, gateErrors: map[string]bool{}, inactive: map[string]bool{},
	}
	r.object(e.sch(), values, "", false, false)
	r.object(e.sch(), values, "", false, true)
	e.runCompute(values, r.tasks, r.result, report)
	completeErrorPaths(r.result)

	return values, r.result
}

func (r *resolveState) object(schema *Schema, scope map[string]any, path string, inherited, normalize bool) {
	coerce := inherited || schema.GetCoerce()
	for _, f := range schema.GetFields() {
		name := f.GetName()
		cur, present := scope[name]
		r.field(f, cur, present, func(v any) { scope[name] = v }, joinPath(path, name), coerce, normalize)
	}
}

// A single field pipeline serves object properties, list/tuple positions and map values.
func (r *resolveState) field(
	f *Schema_Field, cur any, present bool, set func(any), path string, coerce, normalize bool,
) {
	seeded := r.gates[path]
	if !r.active(f, path) {
		return
	}

	if normalize && !seeded {
		r.field(f, cur, present, func(v any) { cur, present = v, true; set(v) }, path, coerce, false)
	}

	if !normalize {
		cur, present = r.seedValue(f, cur, present, set, path, coerce)
	} else {
		if present && cur != nil {
			cur = r.normalizeValue(f, cur, set, path, coerce)
		}

		if f.GetComputed() != nil {
			r.tasks = append(r.tasks, computeTask{field: f, set: set, path: path})
			return
		}
	}

	if present && cur != nil {
		r.children(f, cur, path, coerce, normalize)
	}
}

// Evaluate gates against the current root at each phase; only diagnostics are deduplicated.
func (r *resolveState) active(f *Schema_Field, path string) bool {
	result := &ValidationResult{}
	active := r.engine.active(f, r.root, path, result)

	key := path + "\x00" + f.GetWhen()
	if len(result.Errors) != 0 && !r.gateErrors[key] {
		r.result.Errors = append(r.result.Errors, result.Errors...)
		r.gateErrors[key] = true
	}

	r.gates[path] = active
	if !active && !r.inactive[path] {
		recordResolve(r.report, path, ResolveOperation_RESOLVE_OPERATION_INACTIVE)
		r.inactive[path] = true
	}

	return active
}

func (r *resolveState) seedValue(
	f *Schema_Field, cur any, present bool, set func(any), path string, coerce bool,
) (any, bool) {
	if present {
		if out, ok := jsonNumberInput(f, cur); ok {
			cur = out
			set(cur)
		}

		if coerce {
			if out, ok := coerceInput(f, cur); ok {
				cur = out
				set(cur)
				recordResolve(r.report, path, ResolveOperation_RESOLVE_OPERATION_COERCED)
			}
		}
	}

	if !present || f.GetImmutable() {
		if out, ok := defaultValue(f); ok {
			changed := !present || !nativeEqual(cur, out)
			cur, present = out, true
			set(cur)

			if changed {
				recordResolve(r.report, path, ResolveOperation_RESOLVE_OPERATION_DEFAULT_APPLIED)
			}
		}
	}

	return cur, present
}

func (r *resolveState) normalizeValue(f *Schema_Field, cur any, set func(any), path string, coerce bool) any {
	norm := f.GetNormalize()
	if norm == "" {
		return cur
	}

	out, err := r.engine.eval(norm, map[string]any{"this": cur, "root": r.root})
	if err != nil {
		r.result.Errors = append(r.result.Errors, exprErr(path, norm, "normalize: "+err.Error()))
		return cur
	}

	changed := !nativeEqual(cur, out)
	set(out)

	if changed {
		recordResolve(r.report, path, ResolveOperation_RESOLVE_OPERATION_NORMALIZED)
		// A replacement container gets fresh defaults before its children normalize.
		r.children(f, out, path, coerce, false)
	}

	return out
}

func (r *resolveState) children(f *Schema_Field, cur any, path string, coerce, normalize bool) {
	if sub, scope := objectSchema(f, cur, r.engine.sch().GetDefs()); sub != nil {
		r.object(sub, scope, path, coerce, normalize)
		return
	}

	if list := f.GetList(); list != nil {
		if items, ok := cur.([]any); ok {
			for i, value := range items {
				if item := listItemDef(list, i); item != nil {
					r.field(item, value, true, func(v any) { items[i] = v }, fmt.Sprintf("%s[%d]", path, i), coerce, normalize)
				}
			}
		}
	}

	if mp := f.GetMap(); mp != nil {
		if values, ok := cur.(map[string]any); ok {
			for _, key := range slices.Sorted(maps.Keys(values)) {
				childPath := joinPath(path, key)
				if item := mp.GetValueField(); item != nil {
					r.field(item, values[key], true, func(v any) { values[key] = v }, childPath, coerce, normalize)
				} else if scope, isObject := values[key].(map[string]any); isObject && mp.GetValueSchema() != nil {
					r.object(mp.GetValueSchema(), scope, childPath, coerce, normalize)
				}
			}
		}
	}
}

func recordResolve(report *ResolveReport, path string, operation ResolveOperation) {
	if report != nil {
		report.Events = append(report.Events, &ResolveEvent{
			Path: path, PathSegments: pathSegments(path), Operation: operation,
		})
	}
}
