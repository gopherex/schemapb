//! The validation engine, mirroring the Go reference validate.go: identical
//! codes, deterministic order, typed expected/actual, secret masking.

use crate::compute::{
    default_value, expr_err, field_is_active, is_tuple, list_item_def, ref_def_key, resolve,
};
use crate::descriptor::join_path;
use crate::duration::{duration_nanos, parse_go_duration, parse_rfc3339, timestamp_nanos};
use crate::engine::Engine;
use crate::gen::schemapb::schema::field::r#ref::Target;
use crate::gen::schemapb::schema::field::{
    Bytes as BytesKind, Choice, Duration as DurationKind, Kind as K, List as ListKind,
    Map as MapKind, OneOf, Ref, Rule, Severity, String as StringKind, Timestamp as TimestampKind,
};
use crate::gen::schemapb::{ErrorCode, Schema, ValidationError, ValidationResult, Value};
use crate::messages::{render_message, template};
use crate::render::{display_string, native_equals};
use crate::value::{
    as_double, as_int, as_uint, bytes_v, canonical_value, double_v, duration_v, from_native,
    int64_v, list_v, null_v, str_v, timestamp_v, to_native, uint64_v, Native, NativeStruct,
    SchemaField,
};

/// Validates form values against the compiled schema (values resolve in
/// place); every outcome lives in the `ValidationResult`.
pub fn validate(e: &Engine, values: &mut NativeStruct) -> ValidationResult {
    let mut errs = Vec::new();
    let snapshot = values.clone();
    check_immutable(
        e,
        &e.schema.fields.clone(),
        values,
        "",
        &snapshot,
        &mut errs,
    );
    errs.extend(resolve(e, values));
    let root = values.clone();
    validate_fields(e, &e.schema.clone(), values, &root, "", &mut errs);
    for r in &e.schema.rules {
        eval_rule(
            e,
            r,
            r.id.as_deref().unwrap_or(""),
            &Native::Null,
            &root,
            None,
            &mut errs,
        );
    }
    ValidationResult { errors: errs }
}

#[must_use]
pub const fn result_ok(r: &ValidationResult) -> bool {
    r.errors.is_empty()
}

#[must_use]
pub fn result_blocking(r: &ValidationResult) -> bool {
    r.errors
        .iter()
        .any(|e| e.severity != i32::from(Severity::Warning as u8))
}

fn verr(
    path: &str,
    code: ErrorCode,
    constraint: &str,
    expected: Option<Value>,
    actual: Option<Value>,
) -> ValidationError {
    let message = render_message(code, expected.as_ref(), actual.as_ref());
    ValidationError {
        path: path.to_owned(),
        code: code.into(),
        constraint: constraint.to_owned(),
        expected,
        actual,
        severity: Severity::Error.into(),
        message,
        ..Default::default()
    }
}

fn type_err(path: &str, want: &str, val: &Native) -> ValidationError {
    verr(
        path,
        ErrorCode::TypeMismatch,
        "",
        Some(str_v(want)),
        Some(from_native(val)),
    )
}

/// Masks secret fields: drops actual and re-renders the message.
fn mask(mut errs: Vec<ValidationError>, secret: bool) -> Vec<ValidationError> {
    if !secret {
        return errs;
    }
    for e in &mut errs {
        e.actual = None;
        let code = ErrorCode::try_from(e.code).unwrap_or(ErrorCode::Unspecified);
        if template(code).is_some() {
            e.message = render_message(code, e.expected.as_ref(), None);
        }
    }
    errs
}

fn check_immutable(
    e: &Engine,
    fields: &[SchemaField],
    scope: &NativeStruct,
    prefix: &str,
    root: &NativeStruct,
    errs: &mut Vec<ValidationError>,
) {
    for f in fields {
        let path = join_path(prefix, &f.name);
        if !field_is_active(e, f, root, &path, None) {
            continue;
        }
        if f.immutable {
            if let Some(cur) = scope.get(&f.name) {
                if let Some(dv) = default_value(f) {
                    if !native_equals(cur, &dv) {
                        let expected = canonical_value(f, &dv).unwrap_or_else(|_| from_native(&dv));
                        let err = verr(
                            &path,
                            ErrorCode::ImmutableModified,
                            "immutable",
                            Some(expected),
                            Some(from_native(cur)),
                        );
                        errs.extend(mask(vec![err], f.secret));
                    }
                }
            }
            continue;
        }
        match (f.kind.as_ref(), scope.get(&f.name)) {
            (Some(K::Object(o)), Some(Native::Struct(m))) => {
                if let Some(sub) = o.schema.as_ref() {
                    check_immutable(e, &sub.fields, m, &path, root, errs);
                }
            }
            (Some(K::List(l)), Some(Native::List(items))) => {
                for (i, el) in items.iter().enumerate() {
                    if let (Some(item), Native::Struct(m)) = (list_item_def(l, i), el) {
                        if let Some(K::Object(o)) = item.kind.as_ref() {
                            if let Some(sub) = o.schema.as_ref() {
                                check_immutable(
                                    e,
                                    &sub.fields,
                                    m,
                                    &format!("{path}[{i}]"),
                                    root,
                                    errs,
                                );
                            }
                        }
                    }
                }
            }
            (Some(K::Map(mp)), Some(Native::Struct(m))) => {
                if let Some(vs) = mp.value_schema.as_ref() {
                    for (k, el) in m {
                        if let Native::Struct(em) = el {
                            check_immutable(e, &vs.fields, em, &join_path(&path, k), root, errs);
                        }
                    }
                }
            }
            _ => {}
        }
    }
}

fn validate_fields(
    e: &Engine,
    schema: &Schema,
    scope: &NativeStruct,
    root: &NativeStruct,
    prefix: &str,
    errs: &mut Vec<ValidationError>,
) {
    let mut inactive = std::collections::HashSet::new();
    let mut declared = std::collections::HashSet::new();
    for f in &schema.fields {
        declared.insert(f.name.as_str());
        if !f.when.as_deref().unwrap_or("").is_empty()
            && !field_is_active(e, f, root, &f.name, None)
        {
            inactive.insert(f.name.as_str());
        }
    }

    if schema.strict {
        for (key, val) in scope {
            if !declared.contains(key.as_str()) {
                errs.push(verr(
                    &join_path(prefix, key),
                    ErrorCode::UnknownField,
                    "strict",
                    None,
                    Some(from_native(val)),
                ));
            }
        }
    }

    let present = scope
        .keys()
        .filter(|k| !inactive.contains(k.as_str()))
        .count() as u64;
    if let Some(min) = schema.min_properties {
        if present < min {
            errs.push(verr(
                prefix,
                ErrorCode::MinPropertiesViolated,
                "min_properties",
                Some(uint64_v(min)),
                Some(uint64_v(present)),
            ));
        }
    }
    if let Some(max) = schema.max_properties {
        if present > max {
            errs.push(verr(
                prefix,
                ErrorCode::MaxPropertiesViolated,
                "max_properties",
                Some(uint64_v(max)),
                Some(uint64_v(present)),
            ));
        }
    }

    for f in &schema.fields {
        if inactive.contains(f.name.as_str()) {
            continue;
        }
        validate_one(
            e,
            f,
            scope.get(&f.name),
            scope.contains_key(&f.name),
            &join_path(prefix, &f.name),
            root,
            None,
            errs,
        );
    }
}

#[allow(clippy::too_many_arguments)] // the traversal state is the signature
fn validate_one(
    e: &Engine,
    f: &SchemaField,
    val: Option<&Native>,
    exists: bool,
    path: &str,
    root: &NativeStruct,
    index: Option<i64>,
    errs: &mut Vec<ValidationError>,
) {
    if !exists {
        if f.required {
            errs.push(verr(
                path,
                ErrorCode::RequiredMissing,
                "required",
                None,
                None,
            ));
        }
        return;
    }
    let val = val.unwrap_or(&Native::Null);
    if val.is_null() {
        if f.required {
            errs.push(verr(
                path,
                ErrorCode::RequiredMissing,
                "required",
                None,
                None,
            ));
        } else if !f.nullable {
            errs.push(verr(
                path,
                ErrorCode::NotNullable,
                "nullable",
                None,
                Some(null_v()),
            ));
        }
        return;
    }

    errs.extend(mask(check_kind(e, f, val, path, root), f.secret));
    for r in &f.rules {
        eval_rule(e, r, path, val, root, index, errs);
    }
}

fn eval_rule(
    e: &Engine,
    r: &Rule,
    path: &str,
    this: &Native,
    root: &NativeStruct,
    index: Option<i64>,
    errs: &mut Vec<ValidationError>,
) {
    match e.eval(&r.expr, this, root, index) {
        Err(msg) => {
            let mut ve = expr_err(path, &r.expr, &format!("rule: {msg}"));
            ve.rule_id.clone_from(&r.id);
            errs.push(ve);
        }
        Ok(Native::Bool(true)) => {}
        Ok(_) => {
            let sev = r
                .severity
                .and_then(|s| Severity::try_from(s).ok())
                .filter(|s| *s != Severity::Unspecified)
                .unwrap_or(Severity::Error);
            errs.push(ValidationError {
                path: path.to_owned(),
                code: ErrorCode::RuleViolated.into(),
                expr: r.expr.clone(),
                rule_id: r.id.clone(),
                severity: sev.into(),
                message: r.message.clone(),
                ..Default::default()
            });
        }
    }
}

// =============================================================================
// Kind dispatch
// =============================================================================

fn check_kind(
    e: &Engine,
    f: &SchemaField,
    val: &Native,
    path: &str,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    let Some(kind) = f.kind.as_ref() else {
        return Vec::new();
    };
    match kind {
        K::Float(k) => check_double(
            path,
            val,
            k.default.map(f64::from),
            k.r#const.map(f64::from),
            k.gt.map(f64::from),
            k.gte.map(f64::from),
            k.lt.map(f64::from),
            k.lte.map(f64::from),
            k.multiple_of.map(f64::from),
            &k.r#in.iter().copied().map(f64::from).collect::<Vec<_>>(),
            &k.not_in.iter().copied().map(f64::from).collect::<Vec<_>>(),
        ),
        K::Double(k) => check_double(
            path,
            val,
            k.default,
            k.r#const,
            k.gt,
            k.gte,
            k.lt,
            k.lte,
            k.multiple_of,
            &k.r#in,
            &k.not_in,
        ),
        K::Int32(k) => check_int(
            path,
            val,
            k.r#const.map(i64::from),
            k.gt.map(i64::from),
            k.gte.map(i64::from),
            k.lt.map(i64::from),
            k.lte.map(i64::from),
            k.multiple_of.map(i64::from),
            &k.r#in.iter().copied().map(i64::from).collect::<Vec<_>>(),
            &k.not_in.iter().copied().map(i64::from).collect::<Vec<_>>(),
            i64::from(i32::MIN),
            i64::from(i32::MAX),
        ),
        K::Int64(k) => check_int(
            path,
            val,
            k.r#const,
            k.gt,
            k.gte,
            k.lt,
            k.lte,
            k.multiple_of,
            &k.r#in,
            &k.not_in,
            i64::MIN,
            i64::MAX,
        ),
        K::Uint32(k) => check_uint(
            path,
            val,
            k.r#const.map(u64::from),
            k.gt.map(u64::from),
            k.gte.map(u64::from),
            k.lt.map(u64::from),
            k.lte.map(u64::from),
            k.multiple_of.map(u64::from),
            &k.r#in.iter().copied().map(u64::from).collect::<Vec<_>>(),
            &k.not_in.iter().copied().map(u64::from).collect::<Vec<_>>(),
            u64::from(u32::MAX),
        ),
        K::Uint64(k) => check_uint(
            path,
            val,
            k.r#const,
            k.gt,
            k.gte,
            k.lt,
            k.lte,
            k.multiple_of,
            &k.r#in,
            &k.not_in,
            u64::MAX,
        ),
        K::Bool(k) => match val {
            Native::Bool(b) => k
                .r#const
                .filter(|c| b != c)
                .map(|c| {
                    vec![verr(
                        path,
                        ErrorCode::ConstMismatch,
                        "const",
                        Some(from_native(&Native::Bool(c))),
                        Some(from_native(val)),
                    )]
                })
                .unwrap_or_default(),
            _ => vec![type_err(path, "bool", val)],
        },
        K::String(k) => match val {
            Native::Str(s) => check_string(e, path, s, k),
            _ => vec![type_err(path, "string", val)],
        },
        K::Bytes(k) => match val {
            Native::Bytes(b) => check_bytes(path, b, k),
            _ => vec![type_err(path, "bytes", val)],
        },
        K::Choice(k) => check_choice(e, path, val, k, root),
        K::Duration(k) => check_duration(path, val, k),
        K::Timestamp(k) => check_timestamp(path, val, k),
        K::List(l) => match val {
            Native::List(items) => check_list(e, path, items, l, root),
            _ => vec![type_err(path, "list", val)],
        },
        K::Object(o) => match val {
            Native::Struct(m) => {
                let mut sub = Vec::new();
                if let Some(s) = o.schema.as_ref() {
                    validate_fields(e, s, m, root, path, &mut sub);
                    for r in &s.rules {
                        eval_rule(e, r, path, val, root, None, &mut sub);
                    }
                }
                sub
            }
            _ => vec![type_err(path, "object", val)],
        },
        K::Map(mp) => match val {
            Native::Struct(m) => check_map(e, path, m, mp, root),
            _ => vec![type_err(path, "map", val)],
        },
        K::OneOf(oo) => match val {
            Native::Struct(m) => check_one_of(e, path, m, val, oo, root),
            _ => vec![type_err(path, "object", val)],
        },
        K::Ref(r) => check_ref(e, path, val, r, root),
        K::Computed(_) | K::Json(_) => Vec::new(),
    }
}

// =============================================================================
// Numeric checks (honest 64-bit)
// =============================================================================

#[allow(clippy::too_many_arguments)]
fn check_double(
    path: &str,
    val: &Native,
    _default: Option<f64>,
    cst: Option<f64>,
    gt: Option<f64>,
    gte: Option<f64>,
    lt: Option<f64>,
    lte: Option<f64>,
    mul: Option<f64>,
    in_set: &[f64],
    not_in: &[f64],
) -> Vec<ValidationError> {
    let Some(n) = as_double(val) else {
        return vec![type_err(path, "number", val)];
    };
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, expected| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(expected),
            Some(double_v(n)),
        ));
    };
    if let Some(c) = cst {
        if (n - c).abs() != 0.0 {
            add(ErrorCode::ConstMismatch, "const", double_v(c));
        }
    }
    if let Some(b) = gt {
        if n <= b {
            add(ErrorCode::GtViolated, "gt", double_v(b));
        }
    }
    if let Some(b) = gte {
        if n < b {
            add(ErrorCode::GteViolated, "gte", double_v(b));
        }
    }
    if let Some(b) = lt {
        if n >= b {
            add(ErrorCode::LtViolated, "lt", double_v(b));
        }
    }
    if let Some(b) = lte {
        if n > b {
            add(ErrorCode::LteViolated, "lte", double_v(b));
        }
    }
    if !in_set.is_empty() && !in_set.contains(&n) {
        add(
            ErrorCode::NotInAllowedSet,
            "in",
            list_v(in_set.iter().map(|x| double_v(*x)).collect()),
        );
    }
    if !not_in.is_empty() && not_in.contains(&n) {
        add(
            ErrorCode::InForbiddenSet,
            "not_in",
            list_v(not_in.iter().map(|x| double_v(*x)).collect()),
        );
    }
    if let Some(m) = mul {
        if m != 0.0 && (n % m).abs() != 0.0 {
            add(ErrorCode::MultipleOfViolated, "multiple_of", double_v(m));
        }
    }
    out
}

#[allow(clippy::too_many_arguments)]
fn check_int(
    path: &str,
    val: &Native,
    cst: Option<i64>,
    gt: Option<i64>,
    gte: Option<i64>,
    lt: Option<i64>,
    lte: Option<i64>,
    mul: Option<i64>,
    in_set: &[i64],
    not_in: &[i64],
    min_v: i64,
    max_v: i64,
) -> Vec<ValidationError> {
    let Some(n) = as_int(val) else {
        return vec![type_err(path, "integer", val)];
    };
    if n < min_v || n > max_v {
        return vec![type_err(
            path,
            &format!("integer in [{min_v}, {max_v}]"),
            val,
        )];
    }
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, expected| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(expected),
            Some(int64_v(n)),
        ));
    };
    if let Some(c) = cst {
        if n != c {
            add(ErrorCode::ConstMismatch, "const", int64_v(c));
        }
    }
    if let Some(b) = gt {
        if n <= b {
            add(ErrorCode::GtViolated, "gt", int64_v(b));
        }
    }
    if let Some(b) = gte {
        if n < b {
            add(ErrorCode::GteViolated, "gte", int64_v(b));
        }
    }
    if let Some(b) = lt {
        if n >= b {
            add(ErrorCode::LtViolated, "lt", int64_v(b));
        }
    }
    if let Some(b) = lte {
        if n > b {
            add(ErrorCode::LteViolated, "lte", int64_v(b));
        }
    }
    if !in_set.is_empty() && !in_set.contains(&n) {
        add(
            ErrorCode::NotInAllowedSet,
            "in",
            list_v(in_set.iter().map(|x| int64_v(*x)).collect()),
        );
    }
    if !not_in.is_empty() && not_in.contains(&n) {
        add(
            ErrorCode::InForbiddenSet,
            "not_in",
            list_v(not_in.iter().map(|x| int64_v(*x)).collect()),
        );
    }
    if let Some(m) = mul {
        if m != 0 && n % m != 0 {
            add(ErrorCode::MultipleOfViolated, "multiple_of", int64_v(m));
        }
    }
    out
}

#[allow(clippy::too_many_arguments)]
fn check_uint(
    path: &str,
    val: &Native,
    cst: Option<u64>,
    gt: Option<u64>,
    gte: Option<u64>,
    lt: Option<u64>,
    lte: Option<u64>,
    mul: Option<u64>,
    in_set: &[u64],
    not_in: &[u64],
    max_v: u64,
) -> Vec<ValidationError> {
    let Some(n) = as_uint(val) else {
        return vec![type_err(path, "unsigned integer", val)];
    };
    if n > max_v {
        return vec![type_err(path, &format!("unsigned integer <= {max_v}"), val)];
    }
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, expected| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(expected),
            Some(uint64_v(n)),
        ));
    };
    if let Some(c) = cst {
        if n != c {
            add(ErrorCode::ConstMismatch, "const", uint64_v(c));
        }
    }
    if let Some(b) = gt {
        if n <= b {
            add(ErrorCode::GtViolated, "gt", uint64_v(b));
        }
    }
    if let Some(b) = gte {
        if n < b {
            add(ErrorCode::GteViolated, "gte", uint64_v(b));
        }
    }
    if let Some(b) = lt {
        if n >= b {
            add(ErrorCode::LtViolated, "lt", uint64_v(b));
        }
    }
    if let Some(b) = lte {
        if n > b {
            add(ErrorCode::LteViolated, "lte", uint64_v(b));
        }
    }
    if !in_set.is_empty() && !in_set.contains(&n) {
        add(
            ErrorCode::NotInAllowedSet,
            "in",
            list_v(in_set.iter().map(|x| uint64_v(*x)).collect()),
        );
    }
    if !not_in.is_empty() && not_in.contains(&n) {
        add(
            ErrorCode::InForbiddenSet,
            "not_in",
            list_v(not_in.iter().map(|x| uint64_v(*x)).collect()),
        );
    }
    if let Some(m) = mul {
        if m != 0 && n % m != 0 {
            add(ErrorCode::MultipleOfViolated, "multiple_of", uint64_v(m));
        }
    }
    out
}

// =============================================================================
// String / bytes / choice / time checks
// =============================================================================

fn check_string(e: &Engine, path: &str, s: &str, k: &StringKind) -> Vec<ValidationError> {
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, expected| {
        out.push(verr(path, code, constraint, Some(expected), Some(str_v(s))));
    };
    let n = s.chars().count() as u64;

    if let Some(c) = k.r#const.as_deref() {
        if s != c {
            add(ErrorCode::ConstMismatch, "const", str_v(c));
        }
    }
    if let Some(l) = k.len {
        if n != l {
            add(ErrorCode::LenMismatch, "len", uint64_v(l));
        }
    }
    if let Some(l) = k.min_len {
        if n < l {
            add(ErrorCode::MinLenViolated, "min_len", uint64_v(l));
        }
    }
    if let Some(l) = k.max_len {
        if n > l {
            add(ErrorCode::MaxLenViolated, "max_len", uint64_v(l));
        }
    }
    if let Some(p) = k.pattern.as_deref() {
        if let Some(re) = e.regexps.get(p) {
            if !re.is_match(s) {
                add(ErrorCode::PatternMismatch, "pattern", str_v(p));
            }
        }
    }
    if !k.r#in.is_empty() && !k.r#in.iter().any(|x| x == s) {
        add(
            ErrorCode::NotInAllowedSet,
            "in",
            list_v(k.r#in.iter().map(|x| str_v(x)).collect()),
        );
    }
    if !k.not_in.is_empty() && k.not_in.iter().any(|x| x == s) {
        add(
            ErrorCode::InForbiddenSet,
            "not_in",
            list_v(k.not_in.iter().map(|x| str_v(x)).collect()),
        );
    }
    let fmt = k.format.as_deref().unwrap_or("");
    if !fmt.is_empty() {
        match e.formats.get(fmt) {
            None => add(ErrorCode::UnsupportedFormat, "format", str_v(fmt)),
            Some(check) if !check(s) => add(ErrorCode::FormatMismatch, "format", str_v(fmt)),
            Some(_) => {}
        }
    }
    out
}

fn check_bytes(path: &str, b: &[u8], k: &BytesKind) -> Vec<ValidationError> {
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, expected| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(expected),
            Some(bytes_v(b.to_vec())),
        ));
    };
    let n = b.len() as u64;

    if let Some(c) = k.r#const.as_ref() {
        if b != AsRef::<[u8]>::as_ref(c) {
            add(ErrorCode::ConstMismatch, "const", bytes_v(c.clone()));
        }
    }
    if let Some(l) = k.len {
        if n != l {
            add(ErrorCode::LenMismatch, "len", uint64_v(l));
        }
    }
    if let Some(l) = k.min_len {
        if n < l {
            add(ErrorCode::MinLenViolated, "min_len", uint64_v(l));
        }
    }
    if let Some(l) = k.max_len {
        if n > l {
            add(ErrorCode::MaxLenViolated, "max_len", uint64_v(l));
        }
    }
    let prefix = k.prefix.as_deref().unwrap_or(&[]);
    if !prefix.is_empty() && !b.starts_with(prefix) {
        add(
            ErrorCode::PrefixMismatch,
            "prefix",
            bytes_v(prefix.to_vec()),
        );
    }
    let suffix = k.suffix.as_deref().unwrap_or(&[]);
    if !suffix.is_empty() && !b.ends_with(suffix) {
        add(
            ErrorCode::SuffixMismatch,
            "suffix",
            bytes_v(suffix.to_vec()),
        );
    }
    if !k.r#in.is_empty() && !k.r#in.iter().any(|x| AsRef::<[u8]>::as_ref(x) == b) {
        add(
            ErrorCode::NotInAllowedSet,
            "in",
            list_v(k.r#in.iter().map(|x| bytes_v(x.clone())).collect()),
        );
    }
    if !k.not_in.is_empty() && k.not_in.iter().any(|x| AsRef::<[u8]>::as_ref(x) == b) {
        add(
            ErrorCode::InForbiddenSet,
            "not_in",
            list_v(k.not_in.iter().map(|x| bytes_v(x.clone())).collect()),
        );
    }
    out
}

fn check_choice(
    e: &Engine,
    path: &str,
    val: &Native,
    k: &Choice,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    if k.open {
        return Vec::new();
    }
    let actual = from_native(val);
    let src = k.options_expr.as_deref().unwrap_or("");
    if !src.is_empty() {
        return match e.eval(src, &Native::Null, root, None) {
            Err(msg) => vec![expr_err(path, src, &format!("options_expr: {msg}"))],
            Ok(Native::List(allowed)) => {
                if allowed.iter().any(|a| native_equals(val, a)) {
                    Vec::new()
                } else {
                    vec![verr(
                        path,
                        ErrorCode::ChoiceNotAllowed,
                        "options_expr",
                        Some(list_v(allowed.iter().map(from_native).collect())),
                        Some(actual),
                    )]
                }
            }
            Ok(_) => vec![expr_err(path, src, "options_expr: want a list")],
        };
    }
    let mut expected = Vec::new();
    for o in &k.options {
        if native_equals(val, &to_native(o.value.as_ref())) {
            return Vec::new();
        }
        if let Some(v) = o.value.as_ref() {
            expected.push(v.clone());
        }
    }
    vec![verr(
        path,
        ErrorCode::ChoiceNotAllowed,
        "options",
        Some(list_v(expected)),
        Some(actual),
    )]
}

fn as_duration_native(val: &Native) -> Option<pbjson_types::Duration> {
    match val {
        Native::Duration(d) => Some(*d),
        Native::Str(s) => parse_go_duration(s),
        _ => None,
    }
}

fn check_duration(path: &str, val: &Native, k: &DurationKind) -> Vec<ValidationError> {
    let Some(d) = as_duration_native(val) else {
        return vec![type_err(path, "duration", val)];
    };
    let n = duration_nanos(&d);
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, bound: pbjson_types::Duration| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(duration_v(bound)),
            Some(duration_v(d)),
        ));
    };
    if let Some(b) = k.gt {
        if n <= duration_nanos(&b) {
            add(ErrorCode::GtViolated, "gt", b);
        }
    }
    if let Some(b) = k.gte {
        if n < duration_nanos(&b) {
            add(ErrorCode::GteViolated, "gte", b);
        }
    }
    if let Some(b) = k.lt {
        if n >= duration_nanos(&b) {
            add(ErrorCode::LtViolated, "lt", b);
        }
    }
    if let Some(b) = k.lte {
        if n > duration_nanos(&b) {
            add(ErrorCode::LteViolated, "lte", b);
        }
    }
    out
}

fn as_timestamp_native(val: &Native) -> Option<pbjson_types::Timestamp> {
    match val {
        Native::Timestamp(t) => Some(*t),
        Native::Str(s) => parse_rfc3339(s),
        _ => None,
    }
}

fn check_timestamp(path: &str, val: &Native, k: &TimestampKind) -> Vec<ValidationError> {
    let Some(ts) = as_timestamp_native(val) else {
        return vec![type_err(path, "timestamp (RFC3339)", val)];
    };
    let n = timestamp_nanos(&ts);
    let mut out = Vec::new();
    let mut add = |code, constraint: &str, bound: pbjson_types::Timestamp| {
        out.push(verr(
            path,
            code,
            constraint,
            Some(timestamp_v(bound)),
            Some(timestamp_v(ts)),
        ));
    };
    if let Some(b) = k.gt {
        if n <= timestamp_nanos(&b) {
            add(ErrorCode::GtViolated, "gt", b);
        }
    }
    if let Some(b) = k.gte {
        if n < timestamp_nanos(&b) {
            add(ErrorCode::GteViolated, "gte", b);
        }
    }
    if let Some(b) = k.lt {
        if n >= timestamp_nanos(&b) {
            add(ErrorCode::LtViolated, "lt", b);
        }
    }
    if let Some(b) = k.lte {
        if n > timestamp_nanos(&b) {
            add(ErrorCode::LteViolated, "lte", b);
        }
    }
    out
}

// =============================================================================
// Container checks
// =============================================================================

fn check_list(
    e: &Engine,
    path: &str,
    arr: &[Native],
    l: &ListKind,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    if is_tuple(l) {
        let mut out = Vec::new();
        let want = l.items.len();
        if arr.len() != want {
            out.push(verr(
                path,
                ErrorCode::ListCountMismatch,
                "tuple",
                Some(int64_v(i64::try_from(want).unwrap_or(i64::MAX))),
                Some(int64_v(i64::try_from(arr.len()).unwrap_or(i64::MAX))),
            ));
        }
        for i in 0..arr.len().min(want) {
            if let Some(item) = l.items.get(i) {
                validate_one(
                    e,
                    item,
                    arr.get(i),
                    true,
                    &format!("{path}[{i}]"),
                    root,
                    i64::try_from(i).ok(),
                    &mut out,
                );
            }
        }
        return out;
    }

    let mut out = Vec::new();
    let n = arr.len() as u64;
    if let Some(min) = l.min_items {
        if n < min {
            out.push(verr(
                path,
                ErrorCode::MinItemsViolated,
                "min_items",
                Some(uint64_v(min)),
                Some(uint64_v(n)),
            ));
        }
    }
    if let Some(max) = l.max_items {
        if n > max {
            out.push(verr(
                path,
                ErrorCode::MaxItemsViolated,
                "max_items",
                Some(uint64_v(max)),
                Some(uint64_v(n)),
            ));
        }
    }
    let ce = l.count_expr.as_deref().unwrap_or("");
    if !ce.is_empty() {
        match e.eval(ce, &Native::Null, root, None) {
            Err(msg) => out.push(expr_err(path, ce, &format!("count_expr: {msg}"))),
            Ok(v) => match as_int(&v) {
                Some(want) if want >= 0 => {
                    if u64::try_from(want).unwrap_or(0) != n {
                        out.push(verr(
                            path,
                            ErrorCode::ListCountMismatch,
                            "count_expr",
                            Some(int64_v(want)),
                            Some(uint64_v(n)),
                        ));
                    }
                }
                _ => out.push(expr_err(path, ce, "count_expr: want a non-negative int")),
            },
        }
    }
    if l.unique {
        let mut seen = std::collections::HashSet::new();
        for (i, el) in arr.iter().enumerate() {
            let key = unique_key(el);
            if !seen.insert(key) {
                out.push(verr(
                    &format!("{path}[{i}]"),
                    ErrorCode::NotUnique,
                    "unique",
                    None,
                    Some(from_native(el)),
                ));
            }
        }
    }
    if let Some(item) = l.items.first() {
        for (i, el) in arr.iter().enumerate() {
            validate_one(
                e,
                item,
                Some(el),
                true,
                &format!("{path}[{i}]"),
                root,
                i64::try_from(i).ok(),
                &mut out,
            );
        }
    }
    out
}

fn unique_key(v: &Native) -> String {
    match v {
        Native::Null => "null".to_owned(),
        Native::Bool(b) => b.to_string(),
        Native::Int(n) => n.to_string(),
        Native::Uint(n) => n.to_string(),
        Native::Double(f) => f.to_string(),
        Native::Str(s) => serde_json::to_string(s).unwrap_or_default(),
        Native::Bytes(_) | Native::Duration(_) | Native::Timestamp(_) => {
            serde_json::to_string(&display_string(v)).unwrap_or_default()
        }
        Native::List(items) => {
            let inner: Vec<String> = items.iter().map(unique_key).collect();
            format!("[{}]", inner.join(","))
        }
        Native::Struct(m) => {
            let inner: Vec<String> = m
                .iter()
                .map(|(k, x)| {
                    format!(
                        "{}:{}",
                        serde_json::to_string(k).unwrap_or_default(),
                        unique_key(x)
                    )
                })
                .collect();
            format!("{{{}}}", inner.join(","))
        }
    }
}

fn check_map(
    e: &Engine,
    path: &str,
    m: &NativeStruct,
    k: &MapKind,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    let mut out = Vec::new();
    let n = m.len() as u64;
    if let Some(min) = k.min_entries {
        if n < min {
            out.push(verr(
                path,
                ErrorCode::MinEntriesViolated,
                "min_entries",
                Some(uint64_v(min)),
                Some(uint64_v(n)),
            ));
        }
    }
    if let Some(max) = k.max_entries {
        if n > max {
            out.push(verr(
                path,
                ErrorCode::MaxEntriesViolated,
                "max_entries",
                Some(uint64_v(max)),
                Some(uint64_v(n)),
            ));
        }
    }
    let Some(vs) = k.value_schema.as_ref() else {
        return out;
    };
    for (key, el) in m {
        let vpath = join_path(path, key);
        let Native::Struct(em) = el else {
            out.push(type_err(&vpath, "object", el));
            continue;
        };
        let mut sub = Vec::new();
        validate_fields(e, vs, em, root, &vpath, &mut sub);
        for r in &vs.rules {
            eval_rule(e, r, &vpath, el, root, None, &mut sub);
        }
        out.extend(sub);
    }
    out
}

fn check_one_of(
    e: &Engine,
    path: &str,
    m: &NativeStruct,
    val: &Native,
    oo: &OneOf,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    let disc = match m.get(&oo.discriminator) {
        Some(Native::Str(d)) if !d.is_empty() => d.clone(),
        _ => {
            return vec![verr(
                path,
                ErrorCode::DiscriminatorMissing,
                "discriminator",
                Some(str_v(&oo.discriminator)),
                None,
            )];
        }
    };
    let Some(variant) = oo.variants.get(&disc) else {
        let mut keys: Vec<&String> = oo.variants.keys().collect();
        keys.sort();
        return vec![verr(
            path,
            ErrorCode::UnknownVariant,
            "variants",
            Some(list_v(keys.into_iter().map(|x| str_v(x)).collect())),
            Some(str_v(&disc)),
        )];
    };
    let mut sub = Vec::new();
    validate_fields(e, variant, m, root, path, &mut sub);
    for r in &variant.rules {
        eval_rule(e, r, path, val, root, None, &mut sub);
    }
    sub
}

fn check_ref(
    e: &Engine,
    path: &str,
    val: &Native,
    r: &Ref,
    root: &NativeStruct,
) -> Vec<ValidationError> {
    let key = ref_def_key(r);
    let Some(def) = e.schema.defs.get(&key) else {
        let label = match r.target.as_ref() {
            Some(Target::Id(id)) => {
                let ns = if id.namespace.is_empty() {
                    String::new()
                } else {
                    format!("{}/", id.namespace)
                };
                let ver = if id.version.is_empty() {
                    String::new()
                } else {
                    format!("@{}", id.version)
                };
                format!("{ns}{}{ver} (unlinked identity-ref — call link)", id.name)
            }
            _ => key,
        };
        return vec![verr(
            path,
            ErrorCode::UnknownRef,
            "ref",
            Some(str_v(&label)),
            None,
        )];
    };
    let Native::Struct(m) = val else {
        return vec![type_err(path, "object", val)];
    };
    let mut sub = Vec::new();
    validate_fields(e, def, m, root, path, &mut sub);
    for rule in &def.rules {
        eval_rule(e, rule, path, val, root, None, &mut sub);
    }
    sub
}
