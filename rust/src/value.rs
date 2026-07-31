//! The single conversion point between the value worlds.
//!
//! wire   — generated `Value` / `StructValue` (prost + pbjson-types)
//! native — the engine's value model, a Rust enum:
//!
//! ```text
//! Null            Bool(bool)         Int(i64)      Uint(u64)
//! Double(f64)     Str(String)        Bytes(Vec<u8>)
//! Duration(pbjson_types::Duration)   Timestamp(pbjson_types::Timestamp)
//! List(Vec<Native>)                  Struct(BTreeMap<String, Native>)
//! ```
//!
//! `BTreeMap` keeps struct iteration sorted — determinism is spec, not
//! accident (principle 9). The declared field kind narrows a native value to
//! the exact wire variant at the boundary (`canonical_value`).

use std::collections::BTreeMap;

use crate::gen::schemapb::{schema, value::Kind, ListValue, Schema, StructValue, Value};

/// The generated field type (prost nests it under `schema`).
pub type SchemaField = schema::Field;

pub use crate::gen::schemapb as pb;

pub type NativeStruct = BTreeMap<String, Native>;

/// The native value model.
#[derive(Debug, Clone, PartialEq)]
pub enum Native {
    Null,
    Bool(bool),
    Int(i64),
    Uint(u64),
    Double(f64),
    Str(String),
    Bytes(Vec<u8>),
    Duration(pbjson_types::Duration),
    Timestamp(pbjson_types::Timestamp),
    List(Vec<Self>),
    Struct(NativeStruct),
}

impl Native {
    #[must_use]
    pub const fn is_null(&self) -> bool {
        matches!(self, Self::Null)
    }

    #[must_use]
    pub const fn as_struct(&self) -> Option<&NativeStruct> {
        match self {
            Self::Struct(m) => Some(m),
            _ => None,
        }
    }

    #[must_use]
    pub const fn as_struct_mut(&mut self) -> Option<&mut NativeStruct> {
        match self {
            Self::Struct(m) => Some(m),
            _ => None,
        }
    }
}

// =============================================================================
// Constructors (wire values)
// =============================================================================

#[must_use]
pub const fn wire(kind: Kind) -> Value {
    Value { kind: Some(kind) }
}

#[must_use]
pub const fn null_v() -> Value {
    wire(Kind::NullValue(0))
}

#[must_use]
pub const fn bool_v(v: bool) -> Value {
    wire(Kind::BoolValue(v))
}

#[must_use]
pub const fn int32_v(v: i32) -> Value {
    wire(Kind::Int32Value(v))
}

#[must_use]
pub const fn int64_v(v: i64) -> Value {
    wire(Kind::Int64Value(v))
}

#[must_use]
pub const fn uint32_v(v: u32) -> Value {
    wire(Kind::Uint32Value(v))
}

#[must_use]
pub const fn uint64_v(v: u64) -> Value {
    wire(Kind::Uint64Value(v))
}

#[must_use]
pub const fn float_v(v: f32) -> Value {
    wire(Kind::FloatValue(v))
}

#[must_use]
pub const fn double_v(v: f64) -> Value {
    wire(Kind::DoubleValue(v))
}

#[must_use]
pub fn str_v(v: &str) -> Value {
    wire(Kind::StringValue(v.to_owned()))
}

#[must_use]
pub const fn bytes_v(v: Vec<u8>) -> Value {
    wire(Kind::BytesValue(v))
}

#[must_use]
pub const fn duration_v(v: pbjson_types::Duration) -> Value {
    wire(Kind::DurationValue(v))
}

#[must_use]
pub const fn timestamp_v(v: pbjson_types::Timestamp) -> Value {
    wire(Kind::TimestampValue(v))
}

#[must_use]
pub const fn list_v(items: Vec<Value>) -> Value {
    wire(Kind::ListValue(ListValue { items }))
}

#[must_use]
pub const fn struct_v(fields: std::collections::HashMap<String, Value>) -> Value {
    wire(Kind::StructValue(StructValue { fields }))
}

// =============================================================================
// Wire -> native
// =============================================================================

#[must_use]
pub fn to_native(v: Option<&Value>) -> Native {
    let Some(kind) = v.and_then(|x| x.kind.as_ref()) else {
        return Native::Null;
    };
    match kind {
        Kind::NullValue(_) => Native::Null,
        Kind::BoolValue(b) => Native::Bool(*b),
        Kind::Int32Value(n) => Native::Int(i64::from(*n)),
        Kind::Int64Value(n) => Native::Int(*n),
        Kind::Uint32Value(n) => Native::Uint(u64::from(*n)),
        Kind::Uint64Value(n) => Native::Uint(*n),
        Kind::FloatValue(n) => Native::Double(f64::from(*n)),
        Kind::DoubleValue(n) => Native::Double(*n),
        Kind::StringValue(s) => Native::Str(s.clone()),
        Kind::BytesValue(b) => Native::Bytes(b.clone()),
        Kind::DurationValue(d) => Native::Duration(*d),
        Kind::TimestampValue(t) => Native::Timestamp(*t),
        Kind::ListValue(l) => Native::List(l.items.iter().map(|it| to_native(Some(it))).collect()),
        Kind::StructValue(s) => Native::Struct(struct_to_native(Some(s))),
    }
}

#[must_use]
pub fn struct_to_native(s: Option<&StructValue>) -> NativeStruct {
    s.map(|sv| {
        sv.fields
            .iter()
            .map(|(name, v)| (name.clone(), to_native(Some(v))))
            .collect()
    })
    .unwrap_or_default()
}

// =============================================================================
// Native -> wire (best fit, no schema)
// =============================================================================

#[must_use]
pub fn from_native(x: &Native) -> Value {
    match x {
        Native::Null => null_v(),
        Native::Bool(b) => bool_v(*b),
        Native::Int(n) => int64_v(*n),
        Native::Uint(n) => uint64_v(*n),
        Native::Double(n) => double_v(*n),
        Native::Str(s) => str_v(s),
        Native::Bytes(b) => bytes_v(b.clone()),
        Native::Duration(d) => duration_v(*d),
        Native::Timestamp(t) => timestamp_v(*t),
        Native::List(items) => list_v(items.iter().map(from_native).collect()),
        Native::Struct(m) => struct_v(
            m.iter()
                .map(|(name, v)| (name.clone(), from_native(v)))
                .collect(),
        ),
    }
}

#[must_use]
pub fn struct_from_native(m: &NativeStruct) -> StructValue {
    StructValue {
        fields: m
            .iter()
            .map(|(name, v)| (name.clone(), from_native(v)))
            .collect(),
    }
}

// =============================================================================
// Numeric coercion helpers
// =============================================================================

#[must_use]
pub fn as_int(x: &Native) -> Option<i64> {
    match x {
        Native::Int(n) => Some(*n),
        Native::Uint(n) => i64::try_from(*n).ok(),
        Native::Double(f) if f.is_finite() && f.fract() == 0.0 && f.abs() < 9.3e18 =>
        {
            #[allow(clippy::cast_possible_truncation)]
            Some(*f as i64)
        }
        _ => None,
    }
}

#[must_use]
pub fn as_uint(x: &Native) -> Option<u64> {
    match x {
        Native::Uint(n) => Some(*n),
        Native::Int(n) => u64::try_from(*n).ok(),
        Native::Double(_) => as_int(x).and_then(|n| u64::try_from(n).ok()),
        _ => None,
    }
}

#[must_use]
pub const fn as_double(x: &Native) -> Option<f64> {
    #[allow(clippy::cast_precision_loss)]
    match x {
        Native::Double(f) => Some(*f),
        Native::Int(n) => Some(*n as f64),
        Native::Uint(n) => Some(*n as f64),
        _ => None,
    }
}

// =============================================================================
// Canonical form (field kind -> exact wire variant)
// =============================================================================

/// A value that cannot represent the declared kind.
#[derive(Debug)]
pub struct CanonicalError(pub String);

/// Converts a native value to the exact wire variant the declared kind
/// mandates.
pub fn canonical_value(f: &SchemaField, x: &Native) -> Result<Value, CanonicalError> {
    use schema::field::Kind as K;
    if x.is_null() {
        return Ok(null_v());
    }
    let fail = |msg: &str| Err(CanonicalError(format!("field {}: {msg}", f.name)));
    let Some(kind) = f.kind.as_ref() else {
        return Ok(from_native(x));
    };
    match kind {
        #[allow(clippy::cast_possible_truncation)]
        K::Float(_) => as_double(x).map_or_else(|| fail("not numeric"), |n| Ok(float_v(n as f32))),
        K::Double(_) => as_double(x).map_or_else(|| fail("not numeric"), |n| Ok(double_v(n))),
        K::Int32(_) => as_int(x)
            .and_then(|n| i32::try_from(n).ok())
            .map_or_else(|| fail("does not fit int32"), |n| Ok(int32_v(n))),
        K::Int64(_) => as_int(x).map_or_else(|| fail("not an integer"), |n| Ok(int64_v(n))),
        K::Uint32(_) => as_uint(x)
            .and_then(|n| u32::try_from(n).ok())
            .map_or_else(|| fail("does not fit uint32"), |n| Ok(uint32_v(n))),
        K::Uint64(_) => {
            as_uint(x).map_or_else(|| fail("not an unsigned integer"), |n| Ok(uint64_v(n)))
        }
        K::Bool(_) => match x {
            Native::Bool(b) => Ok(bool_v(*b)),
            _ => fail("not bool"),
        },
        K::String(_) => match x {
            Native::Str(s) => Ok(str_v(s)),
            _ => fail("not string"),
        },
        K::Bytes(_) => match x {
            Native::Bytes(b) => Ok(bytes_v(b.clone())),
            _ => fail("not bytes"),
        },
        K::Duration(_) => match x {
            Native::Duration(d) => Ok(duration_v(*d)),
            _ => fail("not a duration"),
        },
        K::Timestamp(_) => match x {
            Native::Timestamp(t) => Ok(timestamp_v(*t)),
            _ => fail("not a timestamp"),
        },
        K::List(l) => match x {
            Native::List(items) => {
                let mut out = Vec::with_capacity(items.len());
                for (i, el) in items.iter().enumerate() {
                    let item = if l.items.len() == 1 {
                        l.items.first()
                    } else {
                        l.items.get(i)
                    };
                    out.push(match item {
                        Some(def) => canonical_value(def, el)?,
                        None => from_native(el),
                    });
                }
                Ok(list_v(out))
            }
            _ => fail("not a list"),
        },
        K::Object(o) => match (x, o.schema.as_ref()) {
            (Native::Struct(m), Some(sub)) => canonical_struct(sub, m),
            (Native::Struct(_), None) => Ok(from_native(x)),
            _ => fail("not an object"),
        },
        K::Map(mp) => match x {
            Native::Struct(m) => {
                let mut fields = std::collections::HashMap::with_capacity(m.len());
                for (key, el) in m {
                    let v = match (mp.value_schema.as_ref(), el) {
                        (Some(vs), Native::Struct(em)) => canonical_struct(vs, em)?,
                        _ => from_native(el),
                    };
                    fields.insert(key.clone(), v);
                }
                Ok(struct_v(fields))
            }
            _ => fail("not a map"),
        },
        // Choice / Json canonicalize best-fit; Computed by result type at the
        // engine; OneOf / Ref structurally through their target schemas.
        K::Choice(_) | K::Json(_) | K::Computed(_) | K::OneOf(_) | K::Ref(_) => Ok(from_native(x)),
    }
}

/// Canonicalizes a native map against a schema's declared fields.
pub fn canonical_struct(s: &Schema, m: &NativeStruct) -> Result<Value, CanonicalError> {
    let mut fields = std::collections::HashMap::with_capacity(m.len());
    for (key, el) in m {
        let fld = s.fields.iter().find(|f| &f.name == key);
        let v = match fld {
            Some(f) => canonical_value(f, el)?,
            None => from_native(el),
        };
        fields.insert(key.clone(), v);
    }
    Ok(struct_v(fields))
}
