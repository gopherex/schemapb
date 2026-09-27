//! The resolve pipeline, mirroring the Go reference compute.go.

use base64::Engine as _;

use crate::descriptor::schema_err;
use crate::duration::{parse_go_duration, parse_rfc3339};
use crate::engine::Engine;
use crate::gen::schemapb::schema::field::r#ref::Target;
use crate::gen::schemapb::schema::field::{Kind as K, List as ListKind, OneOf, Ref, ResultType};
use crate::gen::schemapb::{ErrorCode, Schema, ValidationError};
use crate::gen::schemapb::{ResolveOperation, ResolveReport};
use crate::resolve::{record, set_at, Step};
use crate::value::{as_double, as_int, as_uint, to_native, Native, NativeStruct, SchemaField};

#[must_use]
pub(crate) fn expr_err(path: &str, expr: &str, msg: &str) -> ValidationError {
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
pub(crate) fn ref_def_key(r: &Ref) -> String {
    match r.target.as_ref() {
        Some(Target::Id(id)) => format!("{}\0{}\0{}", id.namespace, id.name, id.version),
        Some(Target::Name(name)) => name.clone(),
        None => String::new(),
    }
}

#[must_use]
pub(crate) const fn is_tuple(l: &ListKind) -> bool {
    l.items.len() > 1
}

#[must_use]
pub(crate) fn list_item_def(l: &ListKind, i: usize) -> Option<&SchemaField> {
    if l.items.len() == 1 {
        l.items.first()
    } else {
        l.items.get(i)
    }
}

/// Picks the `OneOf` variant schema for a value by its discriminator.
#[must_use]
pub(crate) fn select_variant<'a>(oo: &'a OneOf, val: &Native) -> Option<&'a Schema> {
    let m = val.as_struct()?;
    match m.get(&oo.discriminator) {
        Some(Native::Str(disc)) if !disc.is_empty() => oo.variants.get(disc),
        _ => None,
    }
}

/// Resolve a present Object/Ref/OneOf, including a list or tuple item.
pub(crate) fn object_schema<'a>(
    f: &'a SchemaField,
    val: &Native,
    defs: &'a std::collections::HashMap<String, Schema>,
) -> Option<&'a Schema> {
    val.as_struct()?;
    match f.kind.as_ref()? {
        K::Object(o) => o.schema.as_ref(),
        K::Ref(r) => defs.get(&ref_def_key(r)),
        K::OneOf(oo) => select_variant(oo, val),
        _ => None,
    }
}

pub(crate) struct ComputeTask {
    pub keys: Vec<Step>,
    pub path: String,
    pub computed: crate::gen::schemapb::schema::field::Computed,
}

pub(crate) fn resolve(e: &Engine, values: &mut NativeStruct) -> Vec<ValidationError> {
    crate::resolve::resolve(e, values, None)
}

#[must_use]
pub(crate) fn field_is_active(
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

pub(crate) fn run_compute(
    e: &Engine,
    root: &mut NativeStruct,
    tasks: &[ComputeTask],
    errs: &mut Vec<ValidationError>,
    mut report: Option<&mut ResolveReport>,
) {
    if tasks.is_empty() {
        return;
    }
    let mut by_path: std::collections::HashMap<String, usize> = std::collections::HashMap::new();
    let mut exprs: Vec<String> = Vec::new();
    for (i, task) in tasks.iter().enumerate() {
        by_path.insert(task.path.clone(), i);
        exprs.push(task.computed.expr.clone());
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
            let ComputeTask { path, .. } = &tasks[i];
            errs.push(schema_err(path, "computed field cycle"));
        }
    }

    for i in order {
        let ComputeTask { keys, path, .. } = &tasks[i];
        let src = exprs[i].clone();
        if src.is_empty() {
            continue;
        }
        let root_snapshot = root.clone();
        let result = e.eval(&src, &Native::Null, &root_snapshot, None);

        match result {
            Err(msg) => errs.push(expr_err(path, &src, &format!("compute: {msg}"))),
            Ok(v) => {
                let rt = tasks[i].computed.result;
                match shape_result(rt, v) {
                    Some(shaped) => {
                        set_at(root, keys, shaped);
                        record(report.as_deref_mut(), path, ResolveOperation::Computed);
                    }
                    None => errs.push(expr_err(
                        path,
                        &src,
                        "compute: result does not match declared type",
                    )),
                }
            }
        }
    }
}

/// Converts a computed result to its declared `ResultType`'s native form.
#[must_use]
pub(crate) fn shape_result(rt: Option<i32>, x: Native) -> Option<Native> {
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
pub(crate) fn coerce_input(f: &SchemaField, val: &Native) -> Option<Native> {
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
pub(crate) fn default_value(f: &SchemaField) -> Option<Native> {
    match f.kind.as_ref()? {
        K::Object(k) if k.default.is_some() => Some(Native::Struct(NativeStruct::new())),
        K::Ref(k) if k.default.is_some() => Some(Native::Struct(NativeStruct::new())),
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
pub(crate) fn choice_options(e: &Engine, name: &str, root: &NativeStruct) -> Option<Vec<Native>> {
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
pub(crate) fn list_count(e: &Engine, name: &str, root: &NativeStruct) -> Option<i64> {
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
