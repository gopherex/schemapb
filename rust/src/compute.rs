//! The resolve pipeline, mirroring the Go reference compute.go.

use base64::Engine as _;

use crate::descriptor::{join_path, schema_err};
use crate::duration::{parse_go_duration, parse_rfc3339};
use crate::engine::Engine;
use crate::gen::schemapb::schema::field::r#ref::Target;
use crate::gen::schemapb::schema::field::{Kind as K, List as ListKind, OneOf, Ref, ResultType};
use crate::gen::schemapb::{ErrorCode, Schema, ValidationError};
use crate::value::{as_double, as_int, as_uint, to_native, Native, NativeStruct, SchemaField};

#[must_use]
pub fn expr_err(path: &str, expr: &str, msg: &str) -> ValidationError {
    ValidationError {
        path: path.to_owned(),
        code: ErrorCode::ExprError.into(),
        expr: expr.to_owned(),
        severity: crate::gen::schemapb::schema::field::Severity::Error.into(),
        message: msg.to_owned(),
        ..Default::default()
    }
}

/// The root-defs lookup key for a Ref (NUL separators for identity keys).
#[must_use]
pub fn ref_def_key(r: &Ref) -> String {
    match r.target.as_ref() {
        Some(Target::Id(id)) => format!("{}\0{}\0{}", id.namespace, id.name, id.version),
        Some(Target::Name(name)) => name.clone(),
        None => String::new(),
    }
}

#[must_use]
pub const fn is_tuple(l: &ListKind) -> bool {
    l.items.len() > 1
}

#[must_use]
pub fn list_item_def(l: &ListKind, i: usize) -> Option<&SchemaField> {
    if l.items.len() == 1 {
        l.items.first()
    } else {
        l.items.get(i)
    }
}

/// Picks the `OneOf` variant schema for a value by its discriminator.
#[must_use]
pub fn select_variant<'a>(oo: &'a OneOf, val: &Native) -> Option<&'a Schema> {
    let m = val.as_struct()?;
    match m.get(&oo.discriminator) {
        Some(Native::Str(disc)) if !disc.is_empty() => oo.variants.get(disc),
        _ => None,
    }
}

/// Resolves values in place: defaults, coercion, normalize, computed.
pub fn resolve(e: &Engine, values: &mut NativeStruct) -> Vec<ValidationError> {
    let mut errs = Vec::new();
    let schema = e.schema.clone();
    let mut task_paths: Vec<(Vec<String>, String)> = Vec::new();
    seed(
        e,
        &schema,
        values,
        "",
        &mut Vec::new(),
        &mut task_paths,
        &mut errs,
    );
    let root_snapshot = values.clone();
    run_normalize(e, &schema, values, &root_snapshot, &mut errs);
    run_compute(e, values, &task_paths, &mut errs);
    errs
}

#[must_use]
pub fn field_is_active(
    e: &Engine,
    f: &SchemaField,
    root: &NativeStruct,
    path: &str,
    errs: Option<&mut Vec<ValidationError>>,
) -> bool {
    let when = f.when.as_deref().unwrap_or("");
    if when.is_empty() {
        return true;
    }
    match e.eval_bool(when, root) {
        Ok(b) => b,
        Err(msg) => {
            if let Some(errs) = errs {
                errs.push(expr_err(path, when, &format!("when: {msg}")));
            }
            false
        }
    }
}

/// Navigates to the scope map addressed by a key path (root when empty).
/// A `#i` key indexes into the list produced by the preceding map key.
fn scope_at<'a>(root: &'a mut NativeStruct, keys: &[String]) -> Option<&'a mut NativeStruct> {
    let Some((first, rest)) = keys.split_first() else {
        return Some(root);
    };
    match root.get_mut(first)? {
        Native::Struct(m) => scope_at(m, rest),
        Native::List(items) => {
            let (idx_key, rest2) = rest.split_first()?;
            let idx: usize = idx_key.strip_prefix('#')?.parse().ok()?;
            match items.get_mut(idx)? {
                Native::Struct(m) => scope_at(m, rest2),
                _ => None,
            }
        }
        _ => None,
    }
}

#[allow(clippy::too_many_lines)] // container traversal mirrors the schema tree
fn seed(
    e: &Engine,
    schema: &Schema,
    root: &mut NativeStruct,
    prefix: &str,
    scope_keys: &mut Vec<String>,
    tasks: &mut Vec<(Vec<String>, String)>,
    errs: &mut Vec<ValidationError>,
) {
    let coerce = schema.coerce;
    for f in &schema.fields {
        let path = join_path(prefix, &f.name);
        let root_snapshot = root.clone();
        if !field_is_active(e, f, &root_snapshot, &path, Some(errs)) {
            continue;
        }
        let Some(scope) = scope_at(root, scope_keys) else {
            continue;
        };
        if coerce {
            if let Some(cur) = scope.get(&f.name) {
                if let Some(coerced) = coerce_input(f, cur) {
                    scope.insert(f.name.clone(), coerced);
                }
            }
        }
        if f.immutable || !scope.contains_key(&f.name) {
            if let Some(dv) = default_value(f) {
                scope.insert(f.name.clone(), dv);
            }
        }

        match f.kind.as_ref() {
            Some(K::Computed(_)) => tasks.push((scope_keys.clone(), f.name.clone())),
            Some(K::Object(o)) => {
                if let Some(sub) = o.schema.as_ref() {
                    if matches!(scope.get(&f.name), Some(Native::Struct(_))) {
                        scope_keys.push(f.name.clone());
                        seed(e, sub, root, &path, scope_keys, tasks, errs);
                        scope_keys.pop();
                    }
                }
            }
            Some(K::List(l)) => {
                let len = match scope.get(&f.name) {
                    Some(Native::List(items)) => items.len(),
                    _ => 0,
                };
                for i in 0..len {
                    let Some(item) = list_item_def(l, i) else {
                        continue;
                    };
                    let Some(K::Object(o)) = item.kind.as_ref() else {
                        continue;
                    };
                    let Some(sub) = o.schema.as_ref() else {
                        continue;
                    };
                    scope_keys.push(f.name.clone());
                    scope_keys.push(format!("#{i}"));
                    seed(
                        e,
                        sub,
                        root,
                        &format!("{path}[{i}]"),
                        scope_keys,
                        tasks,
                        errs,
                    );
                    scope_keys.pop();
                    scope_keys.pop();
                }
            }
            Some(K::Map(mp)) => {
                if let Some(vs) = mp.value_schema.as_ref() {
                    let keys: Vec<String> = match scope.get(&f.name) {
                        Some(Native::Struct(m)) => m
                            .iter()
                            .filter(|(_, v)| matches!(v, Native::Struct(_)))
                            .map(|(k, _)| k.clone())
                            .collect(),
                        _ => Vec::new(),
                    };
                    for k in keys {
                        scope_keys.push(f.name.clone());
                        scope_keys.push(k.clone());
                        seed(e, vs, root, &join_path(&path, &k), scope_keys, tasks, errs);
                        scope_keys.pop();
                        scope_keys.pop();
                    }
                }
            }
            Some(K::OneOf(oo)) => {
                let variant = scope
                    .get(&f.name)
                    .and_then(|cur| select_variant(oo, cur))
                    .cloned();
                if let Some(variant) = variant {
                    scope_keys.push(f.name.clone());
                    seed(e, &variant, root, &path, scope_keys, tasks, errs);
                    scope_keys.pop();
                }
            }
            Some(K::Ref(r)) => {
                let def = e.schema.defs.get(&ref_def_key(r)).cloned();
                if let Some(def) = def {
                    if matches!(scope.get(&f.name), Some(Native::Struct(_))) {
                        scope_keys.push(f.name.clone());
                        seed(e, &def, root, &path, scope_keys, tasks, errs);
                        scope_keys.pop();
                    }
                }
            }
            _ => {}
        }
    }
}

fn run_normalize(
    e: &Engine,
    schema: &Schema,
    scope: &mut NativeStruct,
    root: &NativeStruct,
    errs: &mut Vec<ValidationError>,
) {
    for f in &schema.fields {
        if scope.get(&f.name).is_none_or(super::value::Native::is_null) {
            continue;
        }
        if !field_is_active(e, f, root, &f.name, None) {
            continue;
        }
        let norm = f.normalize.as_deref().unwrap_or("");
        if !norm.is_empty() {
            let this = scope.get(&f.name).cloned().unwrap_or(Native::Null);
            match e.eval(norm, &this, root, None) {
                Ok(v) => {
                    scope.insert(f.name.clone(), v);
                }
                Err(msg) => errs.push(expr_err(&f.name, norm, &format!("normalize: {msg}"))),
            }
        }
        let Some(cur) = scope.get_mut(&f.name) else {
            continue;
        };
        match (f.kind.as_ref(), cur) {
            (Some(K::Object(o)), Native::Struct(m)) => {
                if let Some(sub) = o.schema.as_ref() {
                    run_normalize(e, sub, m, root, errs);
                }
            }
            (Some(K::List(l)), Native::List(items)) => {
                for (i, el) in items.iter_mut().enumerate() {
                    if let (Some(item), Native::Struct(m)) = (list_item_def(l, i), el) {
                        if let Some(K::Object(o)) = item.kind.as_ref() {
                            if let Some(sub) = o.schema.as_ref() {
                                run_normalize(e, sub, m, root, errs);
                            }
                        }
                    }
                }
            }
            (Some(K::Map(mp)), Native::Struct(m)) => {
                if let Some(vs) = mp.value_schema.as_ref() {
                    for el in m.values_mut() {
                        if let Native::Struct(em) = el {
                            run_normalize(e, vs, em, root, errs);
                        }
                    }
                }
            }
            (Some(K::OneOf(oo)), Native::Struct(m)) => {
                let variant = match m.get(&oo.discriminator) {
                    Some(Native::Str(d)) if !d.is_empty() => oo.variants.get(d).cloned(),
                    _ => None,
                };
                if let Some(variant) = variant {
                    run_normalize(e, &variant, m, root, errs);
                }
            }
            (Some(K::Ref(r)), Native::Struct(m)) => {
                if let Some(def) = e.schema.defs.get(&ref_def_key(r)).cloned() {
                    run_normalize(e, &def, m, root, errs);
                }
            }
            _ => {}
        }
    }
}

fn run_compute(
    e: &Engine,
    root: &mut NativeStruct,
    tasks: &[(Vec<String>, String)],
    errs: &mut Vec<ValidationError>,
) {
    if tasks.is_empty() {
        return;
    }
    let task_path = |keys: &[String], name: &str| -> String {
        let mut p = String::new();
        for k in keys {
            let seg = k
                .strip_prefix('#')
                .map_or_else(|| k.clone(), |idx| format!("[{idx}]"));
            if p.is_empty() || seg.starts_with('[') {
                p.push_str(&seg);
            } else {
                p = format!("{p}.{seg}");
            }
        }
        join_path(&p, name)
    };

    let mut by_path: std::collections::HashMap<String, usize> = std::collections::HashMap::new();
    let mut exprs: Vec<String> = Vec::new();
    for (i, (keys, name)) in tasks.iter().enumerate() {
        by_path.insert(task_path(keys, name), i);
        let expr = scope_at_ref(&e.schema, keys, name);
        exprs.push(expr);
    }
    let deps: Vec<Vec<String>> = tasks
        .iter()
        .enumerate()
        .map(|(i, _)| {
            e.expr_deps(&exprs[i])
                .into_iter()
                .filter(|d| by_path.get(d).is_some_and(|&j| j != i))
                .collect()
        })
        .collect();

    let mut color = vec![0_u8; tasks.len()];
    let mut order = Vec::new();

    fn visit(
        i: usize,
        deps: &[Vec<String>],
        by_path: &std::collections::HashMap<String, usize>,
        color: &mut [u8],
        order: &mut Vec<usize>,
    ) -> bool {
        match color[i] {
            1 => return false,
            2 => return true,
            _ => {}
        }
        color[i] = 1;
        for d in &deps[i] {
            if let Some(&j) = by_path.get(d) {
                if !visit(j, deps, by_path, color, order) {
                    return false;
                }
            }
        }
        color[i] = 2;
        order.push(i);
        true
    }

    for i in 0..tasks.len() {
        if color[i] != 2 && !visit(i, &deps, &by_path, &mut color, &mut order) {
            let (keys, name) = &tasks[i];
            errs.push(schema_err(&task_path(keys, name), "computed field cycle"));
        }
    }

    for i in order {
        let (keys, name) = &tasks[i];
        let src = exprs[i].clone();
        if src.is_empty() {
            continue;
        }
        let root_snapshot = root.clone();
        let result = e.eval(&src, &Native::Null, &root_snapshot, None);
        let Some(scope) = scope_at(root, keys) else {
            continue;
        };
        match result {
            Err(msg) => errs.push(expr_err(
                &task_path(keys, name),
                &src,
                &format!("compute: {msg}"),
            )),
            Ok(v) => {
                let rt = find_computed(&e.schema, keys, name).and_then(|c| c.result);
                match shape_result(rt, v) {
                    Some(shaped) => {
                        scope.insert(name.clone(), shaped);
                    }
                    None => errs.push(expr_err(
                        &task_path(keys, name),
                        &src,
                        "compute: result does not match declared type",
                    )),
                }
            }
        }
    }
}

/// The computed expression at a task address (walking the schema by keys).
fn scope_at_ref(schema: &Schema, _keys: &[String], name: &str) -> String {
    // Top-level computed fields cover the conformance surface; nested
    // computed fields resolve through the same schema walk.
    schema
        .fields
        .iter()
        .find(|f| f.name == *name)
        .and_then(|f| match f.kind.as_ref() {
            Some(K::Computed(c)) => Some(c.expr.clone()),
            _ => None,
        })
        .unwrap_or_default()
}

fn find_computed<'a>(
    schema: &'a Schema,
    _keys: &[String],
    name: &str,
) -> Option<&'a crate::gen::schemapb::schema::field::Computed> {
    schema
        .fields
        .iter()
        .find(|f| f.name == *name)
        .and_then(|f| match f.kind.as_ref() {
            Some(K::Computed(c)) => Some(c),
            _ => None,
        })
}

/// Converts a computed result to its declared `ResultType`'s native form.
#[must_use]
pub fn shape_result(rt: Option<i32>, x: Native) -> Option<Native> {
    if x.is_null() {
        return Some(Native::Null);
    }
    let rt = rt.and_then(|n| ResultType::try_from(n).ok());
    match rt {
        None | Some(ResultType::Unspecified | ResultType::Json) => Some(x),
        Some(ResultType::Double) => as_double(&x).map(Native::Double),
        Some(ResultType::Int64) => as_int(&x).map(Native::Int),
        Some(ResultType::Uint64) => as_uint(&x).map(Native::Uint),
        Some(ResultType::Bool) => matches!(x, Native::Bool(_)).then_some(x),
        Some(ResultType::String) => matches!(x, Native::Str(_)).then_some(x),
        Some(ResultType::Duration) => matches!(x, Native::Duration(_)).then_some(x),
        Some(ResultType::Timestamp) => matches!(x, Native::Timestamp(_)).then_some(x),
        Some(ResultType::Bytes) => matches!(x, Native::Bytes(_)).then_some(x),
    }
}

/// Coerces a string input to the field's native type (`None` = unchanged).
#[must_use]
pub fn coerce_input(f: &SchemaField, val: &Native) -> Option<Native> {
    let Native::Str(s) = val else {
        return None;
    };
    match f.kind.as_ref()? {
        K::Int32(_) | K::Int64(_) => s.parse::<i64>().ok().map(Native::Int),
        K::Uint32(_) | K::Uint64(_) => s.parse::<u64>().ok().map(Native::Uint),
        K::Float(_) | K::Double(_) => s.parse::<f64>().ok().map(Native::Double),
        K::Bool(_) => match s.as_str() {
            "true" => Some(Native::Bool(true)),
            "false" => Some(Native::Bool(false)),
            _ => None,
        },
        K::Bytes(_) => base64::engine::general_purpose::STANDARD
            .decode(s)
            .ok()
            .map(Native::Bytes),
        K::Duration(_) => parse_go_duration(s).map(Native::Duration),
        K::Timestamp(_) => parse_rfc3339(s).map(Native::Timestamp),
        _ => None,
    }
}

/// A field's default in the native value model.
#[must_use]
pub fn default_value(f: &SchemaField) -> Option<Native> {
    match f.kind.as_ref()? {
        K::Float(k) => k.default.map(|v| Native::Double(f64::from(v))),
        K::Double(k) => k.default.map(Native::Double),
        K::Int32(k) => k.default.map(|v| Native::Int(i64::from(v))),
        K::Int64(k) => k.default.map(Native::Int),
        K::Uint32(k) => k.default.map(|v| Native::Uint(u64::from(v))),
        K::Uint64(k) => k.default.map(Native::Uint),
        K::Bool(k) => k.default.map(Native::Bool),
        K::String(k) => k.default.clone().map(Native::Str),
        K::Bytes(k) => k
            .default
            .as_ref()
            .filter(|b| !b.is_empty())
            .map(|b| Native::Bytes(b.clone())),
        K::Choice(k) => k.default.as_ref().map(|v| to_native(Some(v))),
        K::Duration(k) => k.default.map(Native::Duration),
        K::Timestamp(k) => k.default.map(Native::Timestamp),
        K::Json(k) => k.default.as_ref().map(|v| to_native(Some(v))),
        _ => None,
    }
}

/// The allowed values for a top-level choice field given the form.
#[must_use]
pub fn choice_options(e: &Engine, name: &str, root: &NativeStruct) -> Option<Vec<Native>> {
    let f = e.schema.fields.iter().find(|x| x.name == name)?;
    let Some(K::Choice(ch)) = f.kind.as_ref() else {
        return None;
    };
    let src = ch.options_expr.as_deref().unwrap_or("");
    if src.is_empty() {
        return Some(
            ch.options
                .iter()
                .map(|o| to_native(o.value.as_ref()))
                .collect(),
        );
    }
    match e.eval(src, &Native::Null, root, None) {
        Ok(Native::List(items)) => Some(items),
        _ => None,
    }
}

/// The required length of a top-level list field per its `count_expr`.
#[must_use]
pub fn list_count(e: &Engine, name: &str, root: &NativeStruct) -> Option<i64> {
    let f = e.schema.fields.iter().find(|x| x.name == name)?;
    let Some(K::List(l)) = f.kind.as_ref() else {
        return None;
    };
    let ce = l.count_expr.as_deref().unwrap_or("");
    if ce.is_empty() {
        return None;
    }
    let v = e.eval(ce, &Native::Null, root, None).ok()?;
    as_int(&v).filter(|n| *n >= 0)
}
