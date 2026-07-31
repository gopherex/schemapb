//! Display formatting (the spec's display forms), native equality and the
//! contract render context entries.

use base64::Engine as _;
use serde_json::json;

use crate::duration::{format_go_duration, format_rfc3339};
use crate::value::{as_double, as_int, to_native, Native, NativeStruct, SchemaField};

/// The spec's display form of a native value.
#[must_use]
pub fn display_string(v: &Native) -> String {
    match v {
        Native::Null => String::new(),
        Native::Str(s) => s.clone(),
        Native::Bool(b) => if *b { "true" } else { "false" }.to_owned(),
        Native::Int(n) => n.to_string(),
        Native::Uint(n) => n.to_string(),
        Native::Double(_) | Native::List(_) | Native::Struct(_) => {
            serde_json::to_string(&display_json(v)).unwrap_or_default()
        }
        Native::Bytes(b) => base64::engine::general_purpose::STANDARD.encode(b),
        Native::Duration(d) => format_go_duration(d),
        Native::Timestamp(t) => format_rfc3339(t),
    }
}

/// Container JSON with the spec's leaf forms (sorted keys via `BTreeMap`).
fn display_json(v: &Native) -> serde_json::Value {
    #[allow(clippy::cast_precision_loss, clippy::cast_possible_truncation)]
    match v {
        Native::Null => serde_json::Value::Null,
        Native::Bool(b) => json!(b),
        Native::Int(n) => json!(n),
        Native::Uint(n) => json!(n),
        Native::Double(f) => {
            // Go's encoding/json renders integral doubles without ".0".
            if f.fract() == 0.0 && f.is_finite() && f.abs() < 1e15 {
                json!(*f as i64)
            } else {
                json!(f)
            }
        }
        Native::Str(s) => json!(s),
        Native::Bytes(_) | Native::Duration(_) | Native::Timestamp(_) => {
            json!(display_string(v))
        }
        Native::List(items) => serde_json::Value::Array(items.iter().map(display_json).collect()),
        Native::Struct(m) => serde_json::Value::Object(
            m.iter()
                .map(|(k, x)| (k.clone(), display_json(x)))
                .collect(),
        ),
    }
}

/// ASCII-only casing: locale-dependent Unicode casing would break
/// cross-implementation determinism.
#[must_use]
pub fn ascii_upper(s: &str) -> String {
    s.chars()
        .map(|c| {
            if c.is_ascii_lowercase() {
                c.to_ascii_uppercase()
            } else {
                c
            }
        })
        .collect()
}

#[must_use]
pub fn ascii_lower(s: &str) -> String {
    s.chars()
        .map(|c| {
            if c.is_ascii_uppercase() {
                c.to_ascii_lowercase()
            } else {
                c
            }
        })
        .collect()
}

/// Go `strconv.Quote`-compatible quoting for the render context.
#[must_use]
pub fn go_quote(s: &str) -> String {
    serde_json::to_string(s).unwrap_or_default()
}

/// The short kind name used in render contexts.
#[must_use]
pub const fn kind_name(f: &SchemaField) -> &'static str {
    use crate::gen::schemapb::schema::field::Kind as K;
    match f.kind.as_ref() {
        Some(K::Float(_)) => "float",
        Some(K::Double(_)) => "double",
        Some(K::Int32(_)) => "int32",
        Some(K::Int64(_)) => "int64",
        Some(K::Uint32(_)) => "uint32",
        Some(K::Uint64(_)) => "uint64",
        Some(K::Bool(_)) => "bool",
        Some(K::String(_)) => "string",
        Some(K::Bytes(_)) => "bytes",
        Some(K::Choice(_)) => "choice",
        Some(K::Duration(_)) => "duration",
        Some(K::Timestamp(_)) => "timestamp",
        Some(K::List(_)) => "list",
        Some(K::Object(_)) => "object",
        Some(K::Map(_)) => "map",
        Some(K::OneOf(_)) => "oneof",
        Some(K::Ref(_)) => "ref",
        Some(K::Computed(_)) => "computed",
        Some(K::Json(_)) => "json",
        None => "",
    }
}

/// Structural native equality with cross-numeric comparison (spec).
#[must_use]
pub fn native_equals(a: &Native, b: &Native) -> bool {
    if let (Some(x), Some(y)) = (numeric_int(a), numeric_int(b)) {
        return x == y;
    }
    if matches!(a, Native::Int(_) | Native::Uint(_) | Native::Double(_))
        || matches!(b, Native::Int(_) | Native::Uint(_) | Native::Double(_))
    {
        return match (as_double(a), as_double(b)) {
            (Some(x), Some(y)) => (x - y).abs() == 0.0,
            _ => false,
        };
    }
    match (a, b) {
        (Native::List(x), Native::List(y)) => {
            x.len() == y.len() && x.iter().zip(y).all(|(l, r)| native_equals(l, r))
        }
        (Native::Struct(x), Native::Struct(y)) => {
            x.len() == y.len()
                && x.iter()
                    .all(|(k, l)| y.get(k).is_some_and(|r| native_equals(l, r)))
        }
        _ => a == b,
    }
}

fn numeric_int(v: &Native) -> Option<i128> {
    match v {
        Native::Int(n) => Some(i128::from(*n)),
        Native::Uint(n) => Some(i128::from(*n)),
        Native::Double(_) => as_int(v).map(i128::from),
        _ => None,
    }
}

/// One field entry of the contract render context (serialized for Mustache).
#[derive(Debug, Clone, serde::Serialize)]
pub struct RenderField {
    pub name: String,
    pub title: String,
    pub description: String,
    pub unit: String,
    pub group: String,
    pub kind: &'static str,
    pub label: String,
    pub set: bool,
    pub computed: bool,
    pub secret: bool,
    pub immutable: bool,
    pub deprecated: bool,
    pub value: String,
    pub onoff: &'static str,
    pub yesno: &'static str,
    pub quoted: String,
    pub upper: String,
    pub lower: String,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct RenderGroup {
    pub name: String,
    pub fields: Vec<RenderField>,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct RenderContext {
    pub fields: Vec<RenderField>,
    pub groups: Vec<RenderGroup>,
    pub values: std::collections::BTreeMap<String, String>,
}

/// Builds one render-context field entry.
#[must_use]
pub fn render_field(f: &SchemaField, values: &NativeStruct) -> RenderField {
    let val = values.get(&f.name);
    let set = val.is_some_and(|v| !v.is_null());
    let display = if set {
        val.map(display_string).unwrap_or_default()
    } else {
        String::new()
    };
    let mut label = String::new();
    if set {
        if let Some(crate::gen::schemapb::schema::field::Kind::Choice(ch)) = f.kind.as_ref() {
            for o in &ch.options {
                if val.is_some_and(|v| native_equals(v, &to_native(o.value.as_ref()))) {
                    label.clone_from(&o.label);
                    break;
                }
            }
        }
    }
    let b = matches!(val, Some(Native::Bool(true)));
    RenderField {
        name: f.name.clone(),
        title: f.title.clone().unwrap_or_default(),
        description: f.description.clone().unwrap_or_default(),
        unit: f.unit.clone().unwrap_or_default(),
        group: f.group.clone().unwrap_or_default(),
        kind: kind_name(f),
        label,
        set,
        computed: matches!(
            f.kind.as_ref(),
            Some(crate::gen::schemapb::schema::field::Kind::Computed(_))
        ),
        secret: f.secret,
        immutable: f.immutable,
        deprecated: f.deprecated,
        quoted: go_quote(&display),
        upper: ascii_upper(&display),
        lower: ascii_lower(&display),
        value: display,
        onoff: if b { "on" } else { "off" },
        yesno: if b { "yes" } else { "no" },
    }
}
