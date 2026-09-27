//! One field pipeline for properties, list positions and map entries.
use crate::compute::{
    coerce_input, default_value, expr_err, field_is_active, list_item_def, object_schema,
    run_compute, ComputeTask,
};
use crate::descriptor::join_path;
use crate::engine::Engine;
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::{
    ResolveEvent, ResolveOperation, ResolveReport, Schema, ValidationError,
};
use crate::render::native_equals;
use crate::value::{Native, NativeStruct, SchemaField};

#[derive(Clone)]
pub enum Step {
    Key(String),
    Index(usize),
}

pub fn resolve(
    e: &Engine,
    root: &mut NativeStruct,
    report: Option<&mut ResolveReport>,
) -> Vec<ValidationError> {
    let mut state = Resolver {
        engine: e,
        root,
        report,
        errs: Vec::new(),
        tasks: Vec::new(),
        gates: std::collections::HashMap::new(),
        gate_errors: std::collections::HashSet::new(),
        inactive: std::collections::HashSet::new(),
    };
    state.object(&e.schema, &[], "", false, false);
    state.object(&e.schema, &[], "", false, true);
    run_compute(e, state.root, &state.tasks, &mut state.errs, state.report);
    for err in &mut state.errs {
        err.path_segments = crate::path::segments(&err.path);
    }
    state.errs
}

fn value_at<'a>(root: &'a NativeStruct, keys: &[Step]) -> Option<&'a Native> {
    let (Step::Key(first), rest) = keys.split_first()? else {
        return None;
    };
    let mut cur = root.get(first)?;
    for step in rest {
        cur = match (step, cur) {
            (Step::Key(key), Native::Struct(m)) => m.get(key)?,
            (Step::Index(i), Native::List(a)) => a.get(*i)?,
            _ => return None,
        };
    }
    Some(cur)
}

pub fn set_at(root: &mut NativeStruct, keys: &[Step], value: Native) {
    let Some((Step::Key(first), rest)) = keys.split_first() else {
        return;
    };
    if rest.is_empty() {
        root.insert(first.clone(), value);
        return;
    }
    let Some(mut cur) = root.get_mut(first) else {
        return;
    };
    let Some((last, parents)) = rest.split_last() else {
        return;
    };
    for step in parents {
        let next = match (step, cur) {
            (Step::Key(key), Native::Struct(m)) => m.get_mut(key),
            (Step::Index(i), Native::List(a)) => a.get_mut(*i),
            _ => None,
        };
        let Some(next) = next else {
            return;
        };
        cur = next;
    }
    match (last, cur) {
        (Step::Key(key), Native::Struct(m)) => {
            m.insert(key.clone(), value);
        }
        (Step::Index(i), Native::List(a)) => {
            if let Some(slot) = a.get_mut(*i) {
                *slot = value;
            }
        }
        _ => {}
    }
}

struct Resolver<'a> {
    engine: &'a Engine,
    root: &'a mut NativeStruct,
    report: Option<&'a mut ResolveReport>,
    errs: Vec<ValidationError>,
    tasks: Vec<ComputeTask>,
    gates: std::collections::HashMap<String, bool>,
    gate_errors: std::collections::HashSet<(String, Option<String>)>,
    inactive: std::collections::HashSet<String>,
}
impl Resolver<'_> {
    fn object(
        &mut self,
        schema: &Schema,
        keys: &[Step],
        path: &str,
        inherited: bool,
        normalize: bool,
    ) {
        let coerce = inherited || schema.coerce;
        for f in &schema.fields {
            let mut child = keys.to_vec();
            child.push(Step::Key(f.name.clone()));
            self.field(f, &child, &join_path(path, &f.name), coerce, normalize);
        }
    }
    fn field(&mut self, f: &SchemaField, keys: &[Step], path: &str, coerce: bool, normalize: bool) {
        let seeded = self.gates.get(path).copied().unwrap_or(false);
        if !self.active(f, path) {
            return;
        }
        if normalize && !seeded {
            self.field(f, keys, path, coerce, false);
        }
        let mut cur = value_at(self.root, keys).cloned();
        if normalize {
            let norm = f.normalize.as_deref().unwrap_or("");
            if let Some(value) = cur.as_ref().filter(|v| !v.is_null() && !norm.is_empty()) {
                match self.engine.eval(norm, value, self.root, None) {
                    Err(msg) => self
                        .errs
                        .push(expr_err(path, norm, &format!("normalize: {msg}"))),
                    Ok(out) => {
                        let changed = !native_equals(value, &out);
                        set_at(self.root, keys, out.clone());
                        cur = Some(out.clone());
                        if changed {
                            record(
                                self.report.as_deref_mut(),
                                path,
                                ResolveOperation::Normalized,
                            );
                            self.children(f, &out, keys, path, coerce, false);
                        }
                    }
                }
            }
            if let Some(K::Computed(c)) = f.kind.as_ref() {
                self.tasks.push(ComputeTask {
                    keys: keys.to_vec(),
                    path: path.into(),
                    computed: c.clone(),
                });
                return;
            }
        } else {
            if coerce {
                if let Some(out) = cur.as_ref().and_then(|v| coerce_input(f, v)) {
                    set_at(self.root, keys, out.clone());
                    cur = Some(out);
                    record(self.report.as_deref_mut(), path, ResolveOperation::Coerced);
                }
            }
            if cur.is_none() || f.immutable {
                if let Some(out) = default_value(f) {
                    let changed = cur.as_ref().is_none_or(|v| !native_equals(v, &out));
                    set_at(self.root, keys, out.clone());
                    cur = Some(out);
                    if changed {
                        record(
                            self.report.as_deref_mut(),
                            path,
                            ResolveOperation::DefaultApplied,
                        );
                    }
                }
            }
        }

        if let Some(value) = cur.filter(|v| !v.is_null()) {
            self.children(f, &value, keys, path, coerce, normalize);
        }
    }
    fn active(&mut self, f: &SchemaField, path: &str) -> bool {
        let mut errors = Vec::new();
        let active = field_is_active(self.engine, f, self.root, path, Some(&mut errors));
        if !errors.is_empty() && self.gate_errors.insert((path.into(), f.when.clone())) {
            self.errs.extend(errors);
        }
        self.gates.insert(path.into(), active);
        if !active && self.inactive.insert(path.into()) {
            record(self.report.as_deref_mut(), path, ResolveOperation::Inactive);
        }
        active
    }
    fn children(
        &mut self,
        f: &SchemaField,
        cur: &Native,
        keys: &[Step],
        path: &str,
        coerce: bool,
        normalize: bool,
    ) {
        if let Some(sub) = object_schema(f, cur, &self.engine.schema.defs) {
            self.object(sub, keys, path, coerce, normalize);
            return;
        }
        match (f.kind.as_ref(), cur) {
            (Some(K::List(list)), Native::List(items)) => {
                for i in 0..items.len() {
                    if let Some(item) = list_item_def(list, i) {
                        let mut child = keys.to_vec();
                        child.push(Step::Index(i));
                        self.field(item, &child, &format!("{path}[{i}]"), coerce, normalize);
                    }
                }
            }
            (Some(K::Map(mp)), Native::Struct(values)) => {
                for (key, value) in values {
                    let mut child = keys.to_vec();
                    child.push(Step::Key(key.clone()));
                    let child_path = join_path(path, key);
                    if let Some(item) = mp.value_field.as_ref() {
                        self.field(item, &child, &child_path, coerce, normalize);
                    } else if let (Some(sub), Native::Struct(_)) = (mp.value_schema.as_ref(), value)
                    {
                        self.object(sub, &child, &child_path, coerce, normalize);
                    }
                }
            }
            _ => {}
        }
    }
}

pub fn record(report: Option<&mut ResolveReport>, path: &str, operation: ResolveOperation) {
    if let Some(report) = report {
        report.events.push(ResolveEvent {
            path: path.into(),
            path_segments: crate::path::segments(path),
            operation: operation.into(),
        });
    }
}
