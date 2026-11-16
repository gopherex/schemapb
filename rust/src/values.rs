//! `Values`: a set of field values an application keeps in its own types
//! and storage — the dynamic side of schemapb (a tracker's custom fields, a
//! form's answers), validated against a [`crate::Validator`].
//!
//! The serde form is plain JSON, the one a `jsonb` column stores and queries:
//! numbers are JSON numbers (`i64`/`u64` exact), bytes standard base64,
//! durations Go form (`"1h30m0s"`), timestamps RFC 3339, NaN and the
//! infinities `"NaN"`, `"Infinity"`, `"-Infinity"`. Reading JSON back yields
//! strings for those kinds; a schema with coercion (the builder's default)
//! restores each declared kind when it validates or resolves the values.

use std::collections::BTreeMap;
use std::fmt;
use std::time::{Duration as StdDuration, SystemTime, UNIX_EPOCH};

use base64::Engine as _;
use pbjson_types::{Duration, Timestamp};
use serde::de::{self, MapAccess, SeqAccess, Visitor};
use serde::ser::{SerializeMap, SerializeSeq};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::duration::{format_go_duration, format_rfc3339};
use crate::value::{as_double, as_int, as_uint, Native, NativeStruct};

/// Field values keyed by field name.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Values(NativeStruct);

/// A value that does not have the requested Rust type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ValueError {
    /// The key (or nested position) that was read.
    pub key: String,
    /// The Rust type that was requested.
    pub expected: &'static str,
    /// The kind found instead.
    pub found: &'static str,
}

impl fmt::Display for ValueError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "{}: expected {}, found {}",
            self.key, self.expected, self.found
        )
    }
}

impl std::error::Error for ValueError {}

impl Values {
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// The value under `key` as `T`: `Ok(None)` when absent or null.
    pub fn get<T: FromNative>(&self, key: &str) -> Result<Option<T>, ValueError> {
        match self.0.get(key) {
            None | Some(Native::Null) => Ok(None),
            Some(v) => T::from_native(v).map(Some).map_err(|e| e.at(key)),
        }
    }

    /// The raw value under `key`.
    #[must_use]
    pub fn raw(&self, key: &str) -> Option<&Native> {
        self.0.get(key)
    }

    /// Sets `key`; returns the previous value.
    pub fn set(&mut self, key: impl Into<String>, value: impl IntoNative) -> Option<Native> {
        self.0.insert(key.into(), value.into_native())
    }

    /// `set` for building: `Values::new().with("a", 1).with("b", "x")`.
    #[must_use]
    pub fn with(mut self, key: impl Into<String>, value: impl IntoNative) -> Self {
        self.set(key, value);
        self
    }

    pub fn remove(&mut self, key: &str) -> Option<Native> {
        self.0.remove(key)
    }

    #[must_use]
    pub fn contains(&self, key: &str) -> bool {
        self.0.contains_key(key)
    }

    #[must_use]
    pub fn len(&self) -> usize {
        self.0.len()
    }

    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// Entries in key order.
    pub fn iter(&self) -> impl Iterator<Item = (&str, &Native)> {
        self.0.iter().map(|(k, v)| (k.as_str(), v))
    }

    #[must_use]
    pub const fn as_native(&self) -> &NativeStruct {
        &self.0
    }

    pub const fn as_native_mut(&mut self) -> &mut NativeStruct {
        &mut self.0
    }

    #[must_use]
    pub fn into_native(self) -> NativeStruct {
        self.0
    }
}

impl From<NativeStruct> for Values {
    fn from(m: NativeStruct) -> Self {
        Self(m)
    }
}

impl From<Values> for NativeStruct {
    fn from(v: Values) -> Self {
        v.0
    }
}

impl<K: Into<String>, V: IntoNative> FromIterator<(K, V)> for Values {
    fn from_iter<I: IntoIterator<Item = (K, V)>>(iter: I) -> Self {
        Self(
            iter.into_iter()
                .map(|(k, v)| (k.into(), v.into_native()))
                .collect(),
        )
    }
}

// =============================================================================
// Rust <-> native conversions
// =============================================================================

/// A Rust type readable from a native value.
pub trait FromNative: Sized {
    fn from_native(v: &Native) -> Result<Self, ValueError>;
}

/// A Rust type storable as a native value.
pub trait IntoNative {
    fn into_native(self) -> Native;
}

const fn kind_name(v: &Native) -> &'static str {
    match v {
        Native::Null => "null",
        Native::Bool(_) => "bool",
        Native::Int(_) => "int",
        Native::Uint(_) => "uint",
        Native::Double(_) => "double",
        Native::Str(_) => "string",
        Native::Bytes(_) => "bytes",
        Native::Duration(_) => "duration",
        Native::Timestamp(_) => "timestamp",
        Native::List(_) => "list",
        Native::Struct(_) => "object",
    }
}

impl ValueError {
    const fn mismatch(expected: &'static str, v: &Native) -> Self {
        Self {
            key: String::new(),
            expected,
            found: kind_name(v),
        }
    }

    fn at(mut self, key: &str) -> Self {
        self.key = if self.key.is_empty() {
            key.to_owned()
        } else if self.key.starts_with('[') {
            format!("{key}{}", self.key)
        } else {
            format!("{key}.{}", self.key)
        };
        self
    }
}

impl FromNative for Native {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        Ok(v.clone())
    }
}

impl IntoNative for Native {
    fn into_native(self) -> Native {
        self
    }
}

impl FromNative for bool {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Bool(b) => Ok(*b),
            _ => Err(ValueError::mismatch("bool", v)),
        }
    }
}

impl IntoNative for bool {
    fn into_native(self) -> Native {
        Native::Bool(self)
    }
}

/// Integers read exactly: a value that does not fit the type is an error,
/// never a truncation.
macro_rules! int_native {
    ($($ty:ty => $name:literal via $read:ident as $variant:ident($wide:ty)),* $(,)?) => {$(
        impl FromNative for $ty {
            fn from_native(v: &Native) -> Result<Self, ValueError> {
                $read(v)
                    .and_then(|n| <$ty>::try_from(n).ok())
                    .ok_or_else(|| ValueError::mismatch($name, v))
            }
        }

        impl IntoNative for $ty {
            fn into_native(self) -> Native {
                Native::$variant(<$wide>::from(self))
            }
        }
    )*};
}

int_native!(
    i8 => "i8" via as_int as Int(i64),
    i16 => "i16" via as_int as Int(i64),
    i32 => "i32" via as_int as Int(i64),
    i64 => "i64" via as_int as Int(i64),
    u8 => "u8" via as_uint as Uint(u64),
    u16 => "u16" via as_uint as Uint(u64),
    u32 => "u32" via as_uint as Uint(u64),
    u64 => "u64" via as_uint as Uint(u64),
);

impl FromNative for f64 {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        as_double(v).ok_or_else(|| ValueError::mismatch("f64", v))
    }
}

impl IntoNative for f64 {
    fn into_native(self) -> Native {
        Native::Double(self)
    }
}

impl FromNative for f32 {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        #[allow(clippy::cast_possible_truncation)]
        as_double(v)
            .map(|n| n as Self)
            .ok_or_else(|| ValueError::mismatch("f32", v))
    }
}

impl IntoNative for f32 {
    fn into_native(self) -> Native {
        Native::Double(f64::from(self))
    }
}

impl FromNative for String {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Str(s) => Ok(s.clone()),
            _ => Err(ValueError::mismatch("string", v)),
        }
    }
}

impl IntoNative for String {
    fn into_native(self) -> Native {
        Native::Str(self)
    }
}

impl IntoNative for &str {
    fn into_native(self) -> Native {
        Native::Str(self.to_owned())
    }
}

/// `Vec<u8>` is bytes, as on the schemapb wire; a list of numbers is
/// `Vec<u64>` (or another integer type).
impl FromNative for Vec<u8> {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Bytes(b) => Ok(b.clone()),
            _ => Err(ValueError::mismatch("bytes", v)),
        }
    }
}

impl IntoNative for Vec<u8> {
    fn into_native(self) -> Native {
        Native::Bytes(self)
    }
}

impl IntoNative for &[u8] {
    fn into_native(self) -> Native {
        Native::Bytes(self.to_vec())
    }
}

impl FromNative for Duration {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Duration(d) => Ok(*d),
            _ => Err(ValueError::mismatch("duration", v)),
        }
    }
}

impl IntoNative for Duration {
    fn into_native(self) -> Native {
        Native::Duration(self)
    }
}

/// A negative duration does not fit `std::time::Duration`: an error.
impl FromNative for StdDuration {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        let d = Duration::from_native(v).map_err(|_| ValueError::mismatch("std duration", v))?;
        match (u64::try_from(d.seconds), u32::try_from(d.nanos)) {
            (Ok(secs), Ok(nanos)) => Ok(Self::new(secs, nanos)),
            _ => Err(ValueError::mismatch("non-negative duration", v)),
        }
    }
}

/// Saturates at the proto Duration range (about 10,000 years).
impl IntoNative for StdDuration {
    fn into_native(self) -> Native {
        Native::Duration(Duration {
            seconds: i64::try_from(self.as_secs()).unwrap_or(i64::MAX),
            nanos: i32::try_from(self.subsec_nanos()).unwrap_or(0),
        })
    }
}

impl FromNative for Timestamp {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Timestamp(t) => Ok(*t),
            _ => Err(ValueError::mismatch("timestamp", v)),
        }
    }
}

impl IntoNative for Timestamp {
    fn into_native(self) -> Native {
        Native::Timestamp(self)
    }
}

impl FromNative for SystemTime {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        let t = Timestamp::from_native(v).map_err(|_| ValueError::mismatch("system time", v))?;
        let nanos = u32::try_from(t.nanos).map_err(|_| ValueError::mismatch("system time", v))?;
        let since = StdDuration::new(t.seconds.unsigned_abs(), 0);
        let base = if t.seconds >= 0 {
            UNIX_EPOCH.checked_add(since)
        } else {
            UNIX_EPOCH.checked_sub(since)
        };
        base.and_then(|b| b.checked_add(StdDuration::from_nanos(u64::from(nanos))))
            .ok_or_else(|| ValueError::mismatch("system time", v))
    }
}

impl IntoNative for SystemTime {
    fn into_native(self) -> Native {
        let (seconds, nanos) = match self.duration_since(UNIX_EPOCH) {
            Ok(after) => (
                i64::try_from(after.as_secs()).unwrap_or(i64::MAX),
                after.subsec_nanos(),
            ),
            Err(before) => {
                let before = before.duration();
                let secs = i64::try_from(before.as_secs()).unwrap_or(i64::MAX);
                // Timestamp nanos are non-negative: borrow one second.
                if before.subsec_nanos() == 0 {
                    (-secs, 0)
                } else {
                    (-secs - 1, 1_000_000_000 - before.subsec_nanos())
                }
            }
        };
        Native::Timestamp(Timestamp {
            seconds,
            nanos: i32::try_from(nanos).unwrap_or(0),
        })
    }
}

impl<T: FromNative> FromNative for Option<T> {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Null => Ok(None),
            _ => T::from_native(v).map(Some),
        }
    }
}

impl<T: IntoNative> IntoNative for Option<T> {
    fn into_native(self) -> Native {
        self.map_or(Native::Null, IntoNative::into_native)
    }
}

/// A list; `Vec<u8>` is bytes instead (see its impl).
macro_rules! list_native {
    ($($ty:ty),*) => {$(
        impl FromNative for Vec<$ty> {
            fn from_native(v: &Native) -> Result<Self, ValueError> {
                list_from(v)
            }
        }

        impl IntoNative for Vec<$ty> {
            fn into_native(self) -> Native {
                Native::List(self.into_iter().map(IntoNative::into_native).collect())
            }
        }
    )*};
}

fn list_from<T: FromNative>(v: &Native) -> Result<Vec<T>, ValueError> {
    let Native::List(items) = v else {
        return Err(ValueError::mismatch("list", v));
    };
    items
        .iter()
        .enumerate()
        .map(|(i, el)| T::from_native(el).map_err(|e| e.at(&format!("[{i}]"))))
        .collect()
}

list_native!(
    bool,
    i8,
    i16,
    i32,
    i64,
    u16,
    u32,
    u64,
    f32,
    f64,
    String,
    Vec<u8>,
    Duration,
    Timestamp,
    StdDuration,
    SystemTime,
    Values,
    Native
);

impl<T: FromNative> FromNative for BTreeMap<String, T> {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        let Native::Struct(m) = v else {
            return Err(ValueError::mismatch("object", v));
        };
        m.iter()
            .map(|(k, el)| {
                T::from_native(el)
                    .map(|x| (k.clone(), x))
                    .map_err(|e| e.at(k))
            })
            .collect()
    }
}

impl<T: IntoNative> IntoNative for BTreeMap<String, T> {
    fn into_native(self) -> Native {
        Native::Struct(
            self.into_iter()
                .map(|(k, v)| (k, v.into_native()))
                .collect(),
        )
    }
}

impl FromNative for Values {
    fn from_native(v: &Native) -> Result<Self, ValueError> {
        match v {
            Native::Struct(m) => Ok(Self(m.clone())),
            _ => Err(ValueError::mismatch("object", v)),
        }
    }
}

impl IntoNative for Values {
    fn into_native(self) -> Native {
        Native::Struct(self.0)
    }
}

// =============================================================================
// serde: plain JSON
// =============================================================================

/// One native value in the plain JSON form (see the module docs).
struct Plain<'a>(&'a Native);

impl Serialize for Plain<'_> {
    fn serialize<S: Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        match self.0 {
            Native::Null => s.serialize_unit(),
            Native::Bool(b) => s.serialize_bool(*b),
            Native::Int(n) => s.serialize_i64(*n),
            Native::Uint(n) => s.serialize_u64(*n),
            Native::Double(n) if n.is_nan() => s.serialize_str("NaN"),
            Native::Double(n) if n.is_infinite() => {
                s.serialize_str(if *n > 0.0 { "Infinity" } else { "-Infinity" })
            }
            Native::Double(n) => s.serialize_f64(*n),
            Native::Str(v) => s.serialize_str(v),
            Native::Bytes(b) => {
                s.serialize_str(&base64::engine::general_purpose::STANDARD.encode(b))
            }
            Native::Duration(d) => s.serialize_str(&format_go_duration(d)),
            Native::Timestamp(t) => s.serialize_str(&format_rfc3339(t)),
            Native::List(items) => {
                let mut seq = s.serialize_seq(Some(items.len()))?;
                for el in items {
                    seq.serialize_element(&Plain(el))?;
                }
                seq.end()
            }
            Native::Struct(m) => plain_map(m, s),
        }
    }
}

fn plain_map<S: Serializer>(m: &NativeStruct, s: S) -> Result<S::Ok, S::Error> {
    let mut map = s.serialize_map(Some(m.len()))?;
    for (k, v) in m {
        map.serialize_entry(k, &Plain(v))?;
    }
    map.end()
}

impl Serialize for Values {
    fn serialize<S: Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        plain_map(&self.0, s)
    }
}

struct NativeVisitor;

impl<'de> Visitor<'de> for NativeVisitor {
    type Value = Native;

    fn expecting(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("a JSON value")
    }

    fn visit_unit<E: de::Error>(self) -> Result<Native, E> {
        Ok(Native::Null)
    }

    fn visit_none<E: de::Error>(self) -> Result<Native, E> {
        Ok(Native::Null)
    }

    fn visit_some<D: Deserializer<'de>>(self, d: D) -> Result<Native, D::Error> {
        d.deserialize_any(self)
    }

    fn visit_bool<E: de::Error>(self, v: bool) -> Result<Native, E> {
        Ok(Native::Bool(v))
    }

    fn visit_i64<E: de::Error>(self, v: i64) -> Result<Native, E> {
        Ok(Native::Int(v))
    }

    /// A non-negative JSON integer is an `Int` when it fits, so plain JSON
    /// round-trips signed fields; larger ones stay exact as `Uint`.
    fn visit_u64<E: de::Error>(self, v: u64) -> Result<Native, E> {
        Ok(i64::try_from(v).map_or(Native::Uint(v), Native::Int))
    }

    fn visit_f64<E: de::Error>(self, v: f64) -> Result<Native, E> {
        Ok(Native::Double(v))
    }

    fn visit_bytes<E: de::Error>(self, v: &[u8]) -> Result<Native, E> {
        Ok(Native::Bytes(v.to_vec()))
    }

    fn visit_byte_buf<E: de::Error>(self, v: Vec<u8>) -> Result<Native, E> {
        Ok(Native::Bytes(v))
    }

    fn visit_str<E: de::Error>(self, v: &str) -> Result<Native, E> {
        Ok(Native::Str(v.to_owned()))
    }

    fn visit_string<E: de::Error>(self, v: String) -> Result<Native, E> {
        Ok(Native::Str(v))
    }

    fn visit_seq<A: SeqAccess<'de>>(self, mut seq: A) -> Result<Native, A::Error> {
        let mut items = Vec::with_capacity(seq.size_hint().unwrap_or(0));
        while let Some(el) = seq.next_element::<PlainOwned>()? {
            items.push(el.0);
        }
        Ok(Native::List(items))
    }

    fn visit_map<A: MapAccess<'de>>(self, mut map: A) -> Result<Native, A::Error> {
        let mut out = NativeStruct::new();
        while let Some((k, v)) = map.next_entry::<String, PlainOwned>()? {
            out.insert(k, v.0);
        }
        Ok(Native::Struct(out))
    }
}

struct PlainOwned(Native);

impl<'de> Deserialize<'de> for PlainOwned {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        d.deserialize_any(NativeVisitor).map(PlainOwned)
    }
}

impl<'de> Deserialize<'de> for Values {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        match d.deserialize_any(NativeVisitor)? {
            Native::Struct(m) => Ok(Self(m)),
            other => Err(de::Error::invalid_type(
                de::Unexpected::Other(kind_name(&other)),
                &"a JSON object",
            )),
        }
    }
}
