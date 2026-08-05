//! Typed extraction from a wire Value and value path lookup.
//!
//! Extraction is the std-trait way: every target implements
//! `TryFrom<&Value>` (a `Result` with a typed kind-mismatch error), and
//! `Value::get::<T>()` is the `Option` sugar on top. One rule, shared by
//! every implementation and pinned by the conformance golden
//! value-as.json: a conversion succeeds iff the value is represented in
//! the target EXACTLY (lossless round-trip). Numeric values convert
//! across kinds under that rule; non-numeric targets are strict — no
//! string parsing, no truncation, no coercion.
//!
//! `StructValue::lookup` resolves a path in the `ValidationError` dialect
//! ("replicas[0].name") — the path from a validation error fetches the
//! offending value directly. The string form cannot address a map key
//! containing '.' or '['; the `field`/`index` steppers cover arbitrary
//! keys without parsing.

use std::collections::HashMap;

use crate::descriptor::join_path;
use crate::gen::schemapb::value::Kind;
use crate::gen::schemapb::{StructValue, Value};

// =============================================================================
// Kind names and numeric views
// =============================================================================

/// The wire kind of a value as its spec short name.
#[must_use]
pub const fn value_kind_name(v: &Value) -> &'static str {
    match v.kind.as_ref() {
        None | Some(Kind::NullValue(_)) => "null",
        Some(Kind::BoolValue(_)) => "bool",
        Some(Kind::Int32Value(_)) => "int32",
        Some(Kind::Int64Value(_)) => "int64",
        Some(Kind::Uint32Value(_)) => "uint32",
        Some(Kind::Uint64Value(_)) => "uint64",
        Some(Kind::FloatValue(_)) => "float",
        Some(Kind::DoubleValue(_)) => "double",
        Some(Kind::StringValue(_)) => "string",
        Some(Kind::BytesValue(_)) => "bytes",
        Some(Kind::DurationValue(_)) => "duration",
        Some(Kind::TimestampValue(_)) => "timestamp",
        Some(Kind::ListValue(_)) => "list",
        Some(Kind::StructValue(_)) => "struct",
    }
}

#[allow(
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::float_cmp
)]
fn float_to_i64(f: f64) -> Option<i64> {
    if f.fract() != 0.0 || f < i64::MIN as f64 || f >= i64::MAX as f64 {
        return None;
    }

    let n = f as i64;
    (n as f64 == f).then_some(n)
}

#[allow(
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    clippy::float_cmp
)]
fn float_to_u64(f: f64) -> Option<u64> {
    if f.fract() != 0.0 || f < 0.0 || f >= u64::MAX as f64 {
        return None;
    }

    let n = f as u64;
    (n as f64 == f).then_some(n)
}

/// The exact int64 view of a numeric value.
fn value_int(v: &Value) -> Option<i64> {
    match v.kind.as_ref()? {
        Kind::Int32Value(n) => Some(i64::from(*n)),
        Kind::Int64Value(n) => Some(*n),
        Kind::Uint32Value(n) => Some(i64::from(*n)),
        Kind::Uint64Value(n) => i64::try_from(*n).ok(),
        Kind::FloatValue(f) => float_to_i64(f64::from(*f)),
        Kind::DoubleValue(f) => float_to_i64(*f),
        _ => None,
    }
}

/// The exact uint64 view of a numeric value.
fn value_uint(v: &Value) -> Option<u64> {
    match v.kind.as_ref()? {
        Kind::Int32Value(n) => u64::try_from(*n).ok(),
        Kind::Int64Value(n) => u64::try_from(*n).ok(),
        Kind::Uint32Value(n) => Some(u64::from(*n)),
        Kind::Uint64Value(n) => Some(*n),
        Kind::FloatValue(f) => float_to_u64(f64::from(*f)),
        Kind::DoubleValue(f) => float_to_u64(*f),
        _ => None,
    }
}

/// The exact float64 view of a numeric value.
#[allow(
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss
)]
fn value_double(v: &Value) -> Option<f64> {
    match v.kind.as_ref()? {
        Kind::Int32Value(n) => Some(f64::from(*n)),
        Kind::Uint32Value(n) => Some(f64::from(*n)),
        Kind::Int64Value(n) => {
            let f = *n as f64;
            (f >= i64::MIN as f64 && f < i64::MAX as f64 && f as i64 == *n).then_some(f)
        }
        Kind::Uint64Value(n) => {
            let f = *n as f64;
            (f < u64::MAX as f64 && f as u64 == *n).then_some(f)
        }
        Kind::FloatValue(f) => Some(f64::from(*f)),
        Kind::DoubleValue(f) => Some(*f),
        _ => None,
    }
}

// =============================================================================
// TryFrom<&Value> for every target + Value::get sugar
// =============================================================================

/// A refused extraction: the value's wire kind is not represented in the
/// requested target exactly.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TryFromValueError {
    pub want: &'static str,
    pub got: &'static str,
}

impl std::fmt::Display for TryFromValueError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "schemapb: value is not representable as {} (kind {})",
            self.want, self.got
        )
    }
}

impl std::error::Error for TryFromValueError {}

const fn refused(want: &'static str, v: &Value) -> TryFromValueError {
    TryFromValueError {
        want,
        got: value_kind_name(v),
    }
}

macro_rules! try_from_value {
    ($ty:ty, $want:literal, $v:ident => $extract:expr) => {
        impl TryFrom<&Value> for $ty {
            type Error = TryFromValueError;

            fn try_from($v: &Value) -> Result<Self, Self::Error> {
                $extract.ok_or_else(|| refused($want, $v))
            }
        }
    };
}

try_from_value!(bool, "bool", v => match v.kind.as_ref() {
    Some(Kind::BoolValue(b)) => Some(*b),
    _ => None,
});
try_from_value!(i32, "int32", v => value_int(v).and_then(|n| Self::try_from(n).ok()));
try_from_value!(i64, "int64", v => value_int(v));
try_from_value!(u32, "uint32", v => value_uint(v).and_then(|n| Self::try_from(n).ok()));
try_from_value!(u64, "uint64", v => value_uint(v));
try_from_value!(f32, "float", v => {
    #[allow(clippy::cast_possible_truncation, clippy::float_cmp)]
    value_double(v).and_then(|f| {
        let g = f as Self;
        (f64::from(g) == f).then_some(g)
    })
});
try_from_value!(f64, "double", v => value_double(v));
try_from_value!(String, "string", v => match v.kind.as_ref() {
    Some(Kind::StringValue(s)) => Some(s.clone()),
    _ => None,
});
try_from_value!(Vec<u8>, "bytes", v => match v.kind.as_ref() {
    Some(Kind::BytesValue(b)) => Some(b.clone()),
    _ => None,
});
try_from_value!(pbjson_types::Duration, "duration", v => match v.kind.as_ref() {
    Some(Kind::DurationValue(d)) => Some(*d),
    _ => None,
});
try_from_value!(pbjson_types::Timestamp, "timestamp", v => match v.kind.as_ref() {
    Some(Kind::TimestampValue(t)) => Some(*t),
    _ => None,
});

/// Borrowed string extraction (no allocation).
impl<'a> TryFrom<&'a Value> for &'a str {
    type Error = TryFromValueError;

    fn try_from(v: &'a Value) -> Result<Self, Self::Error> {
        match v.kind.as_ref() {
            Some(Kind::StringValue(s)) => Ok(s.as_str()),
            _ => Err(refused("string", v)),
        }
    }
}

/// Borrowed bytes extraction (no allocation).
impl<'a> TryFrom<&'a Value> for &'a [u8] {
    type Error = TryFromValueError;

    fn try_from(v: &'a Value) -> Result<Self, Self::Error> {
        match v.kind.as_ref() {
            Some(Kind::BytesValue(b)) => Ok(b.as_slice()),
            _ => Err(refused("bytes", v)),
        }
    }
}

impl Value {
    /// `Option` sugar over the `TryFrom<&Value>` impls:
    /// `v.get::<i64>()`, `v.get::<&str>()`, …
    #[must_use]
    pub fn get<'a, T>(&'a self) -> Option<T>
    where
        T: TryFrom<&'a Self>,
    {
        T::try_from(self).ok()
    }

    /// Reads the value as a list (own kind only).
    #[must_use]
    #[allow(clippy::missing_const_for_fn)] // Vec deref is not const-stable
    pub fn as_list(&self) -> Option<&[Self]> {
        match self.kind.as_ref() {
            Some(Kind::ListValue(l)) => Some(&l.items),
            _ => None,
        }
    }

    /// Reads the value as a struct (own kind only).
    #[must_use]
    pub const fn as_struct(&self) -> Option<&HashMap<String, Self>> {
        match self.kind.as_ref() {
            Some(Kind::StructValue(s)) => Some(&s.fields),
            _ => None,
        }
    }

    /// Steps into a struct value member; `None` when the value is not a
    /// struct or has no such field. Handles keys the string path cannot
    /// spell.
    #[must_use]
    pub fn field(&self, name: &str) -> Option<&Self> {
        self.as_struct()?.get(name)
    }

    /// Steps into a list value element; `None` when the value is not a
    /// list or the index is out of range.
    #[must_use]
    pub fn index(&self, i: usize) -> Option<&Self> {
        self.as_list()?.get(i)
    }
}

// =============================================================================
// Value path lookup (the error-path dialect)
// =============================================================================

/// Why a value path failed to resolve. The wire strings are stable spec
/// values shared by all implementations (conformance).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ValueLookupReason {
    EmptyPath,
    BadPath,
    NotFound,
    IndexOutOfRange,
    NotAStruct,
    NotAList,
}

impl ValueLookupReason {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::EmptyPath => "empty_path",
            Self::BadPath => "bad_path",
            Self::NotFound => "not_found",
            Self::IndexOutOfRange => "index_out_of_range",
            Self::NotAStruct => "not_a_struct",
            Self::NotAList => "not_a_list",
        }
    }
}

/// Pinpoints the failing segment of a value path: `at` is the resolved
/// parent path (empty for root), `segment` the key or `[i]` index that
/// failed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ValueLookupError {
    pub at: String,
    pub segment: String,
    pub reason: ValueLookupReason,
}

impl std::fmt::Display for ValueLookupError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let where_ = if self.at.is_empty() {
            "root".to_owned()
        } else {
            format!("{:?}", self.at)
        };

        match self.reason {
            ValueLookupReason::EmptyPath => write!(f, "schemapb: value lookup: empty path"),
            ValueLookupReason::BadPath => {
                write!(
                    f,
                    "schemapb: value lookup: malformed path {:?}",
                    self.segment
                )
            }
            ValueLookupReason::NotFound => {
                write!(
                    f,
                    "schemapb: value lookup: no field {:?} in {where_}",
                    self.segment
                )
            }
            ValueLookupReason::IndexOutOfRange => write!(
                f,
                "schemapb: value lookup: index {} out of range in {where_}",
                self.segment
            ),
            ValueLookupReason::NotAStruct => write!(
                f,
                "schemapb: value lookup: {where_} is not a struct, cannot read field {:?}",
                self.segment
            ),
            ValueLookupReason::NotAList => write!(
                f,
                "schemapb: value lookup: {where_} is not a list, cannot index {}",
                self.segment
            ),
        }
    }
}

impl std::error::Error for ValueLookupError {}

fn verr(reason: ValueLookupReason, at: &str, segment: &str) -> ValueLookupError {
    ValueLookupError {
        at: at.to_owned(),
        segment: segment.to_owned(),
        reason,
    }
}

enum PathToken<'a> {
    Key(&'a str),
    Index(usize),
}

/// Tokenizes the error-path dialect: `key ('.' key | '[' int ']')*`.
fn parse_value_path(path: &str) -> Option<Vec<PathToken<'_>>> {
    let mut tokens = Vec::new();
    let mut rest = path;

    while !rest.is_empty() {
        if let Some(after) = rest.strip_prefix('[') {
            if tokens.is_empty() {
                return None; // paths start with a key, not an index
            }

            let end = after.find(']')?;
            let body = &after[..end];
            // digits only: no "+3", "-0", "[]"
            if body.is_empty() || !body.bytes().all(|b| b.is_ascii_digit()) {
                return None;
            }

            tokens.push(PathToken::Index(body.parse().ok()?));
            rest = &after[end + 1..];

            // After "]" only ".", "[" or the end may follow.
            if !rest.is_empty() && !rest.starts_with('.') && !rest.starts_with('[') {
                return None;
            }

            continue;
        }

        if let Some(after) = rest.strip_prefix('.') {
            if tokens.is_empty() {
                return None; // leading dot
            }

            rest = after;

            // A dot must be followed by a key: no trailing dot, "a..b",
            // "a.[0]".
            if rest.is_empty() || rest.starts_with('.') || rest.starts_with('[') {
                return None;
            }

            continue;
        }

        let end = rest.find(['.', '[']).unwrap_or(rest.len());
        tokens.push(PathToken::Key(&rest[..end]));
        rest = &rest[end..];
    }

    if tokens.is_empty() {
        return None;
    }

    Some(tokens)
}

impl StructValue {
    /// Resolves a path in the `ValidationError` dialect against the struct's
    /// values. Returns the addressed value, or a `ValueLookupError` naming
    /// the exact segment that failed.
    pub fn lookup(&self, path: &str) -> Result<&Value, ValueLookupError> {
        if path.is_empty() {
            return Err(verr(ValueLookupReason::EmptyPath, "", ""));
        }

        let Some(tokens) = parse_value_path(path) else {
            return Err(verr(ValueLookupReason::BadPath, "", path));
        };

        let mut cur: Option<&Value> = None;
        let mut parent = String::new();

        for tok in tokens {
            match tok {
                PathToken::Key(key) => {
                    let fields = match cur {
                        None => &self.fields,
                        Some(v) => v
                            .as_struct()
                            .ok_or_else(|| verr(ValueLookupReason::NotAStruct, &parent, key))?,
                    };

                    cur = Some(
                        fields
                            .get(key)
                            .ok_or_else(|| verr(ValueLookupReason::NotFound, &parent, key))?,
                    );
                    parent = join_path(&parent, key);
                }
                PathToken::Index(i) => {
                    let seg = format!("[{i}]");
                    let cur_v = cur.expect("index token is never first");
                    let items = cur_v
                        .as_list()
                        .ok_or_else(|| verr(ValueLookupReason::NotAList, &parent, &seg))?;

                    cur =
                        Some(items.get(i).ok_or_else(|| {
                            verr(ValueLookupReason::IndexOutOfRange, &parent, &seg)
                        })?);
                    parent.push_str(&seg);
                }
            }
        }

        Ok(cur.expect("tokens are never empty"))
    }

    /// A top-level member of the struct (`None` when absent).
    #[must_use]
    pub fn field(&self, name: &str) -> Option<&Value> {
        self.fields.get(name)
    }
}
