//! serde between Rust values and the native value model, exact: integers
//! stay integers of their width, bytes stay bytes, durations and
//! timestamps keep their kinds.
//!
//! [`encode`] writes a value as a [`Native`] tree; `None` struct fields are
//! left out, so an absent value can take its schema default. [`fit`] then
//! reshapes what serde cannot express against the schema: a byte sequence
//! where the field is Bytes, `std::time::Duration` / `SystemTime` where it is
//! a Duration / Timestamp (strings are left to the schema's coercion).
//! [`decode`] reads a value back, following the hints the target type gives
//! serde (a `SystemTime` asks for its struct form, `pbjson_types` for the
//! protoJSON string).

use std::collections::HashMap;
use std::fmt;

use serde::de::value::{MapDeserializer, SeqDeserializer};
use serde::de::{self, DeserializeOwned, IntoDeserializer, Visitor};
use serde::ser::{self, Serialize};

use crate::compute::object_schema;
use crate::duration::format_rfc3339;
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::Schema;
use crate::value::{Native, NativeStruct, SchemaField};

/// A value serde cannot carry into or out of the native model.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NativeError(pub String);

impl fmt::Display for NativeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for NativeError {}

impl ser::Error for NativeError {
    fn custom<T: fmt::Display>(msg: T) -> Self {
        Self(msg.to_string())
    }
}

impl de::Error for NativeError {
    fn custom<T: fmt::Display>(msg: T) -> Self {
        Self(msg.to_string())
    }
}

/// Marks a struct field serialized as `None`: left out, not null.
const ABSENT: &str = "\u{0}schemapb:absent";

/// Writes `v` as a native tree.
pub fn encode<T: Serialize + ?Sized>(v: &T) -> Result<Native, NativeError> {
    v.serialize(Enc { field: false })
}

/// Reads a `T` from a native tree.
pub fn decode<T: DeserializeOwned>(v: Native) -> Result<T, NativeError> {
    T::deserialize(Dec(v))
}

// =============================================================================
// Serializer
// =============================================================================

#[derive(Clone, Copy)]
struct Enc {
    /// A struct field's value: `None` is absence.
    field: bool,
}

const VALUE: Enc = Enc { field: false };

fn int<N: TryInto<i64>>(n: N) -> Result<Native, NativeError> {
    n.try_into()
        .map(Native::Int)
        .map_err(|_| NativeError("integer out of the 64-bit range".to_owned()))
}

fn uint<N: TryInto<u64>>(n: N) -> Result<Native, NativeError> {
    n.try_into()
        .map(Native::Uint)
        .map_err(|_| NativeError("integer out of the 64-bit range".to_owned()))
}

impl ser::Serializer for Enc {
    type Ok = Native;
    type Error = NativeError;
    type SerializeSeq = SeqEnc;
    type SerializeTuple = SeqEnc;
    type SerializeTupleStruct = SeqEnc;
    type SerializeTupleVariant = Tagged<SeqEnc>;
    type SerializeMap = MapEnc;
    type SerializeStruct = StructEnc;
    type SerializeStructVariant = Tagged<StructEnc>;

    fn serialize_bool(self, v: bool) -> Result<Native, NativeError> {
        Ok(Native::Bool(v))
    }

    fn serialize_i8(self, v: i8) -> Result<Native, NativeError> {
        int(v)
    }

    fn serialize_i16(self, v: i16) -> Result<Native, NativeError> {
        int(v)
    }

    fn serialize_i32(self, v: i32) -> Result<Native, NativeError> {
        int(v)
    }

    fn serialize_i64(self, v: i64) -> Result<Native, NativeError> {
        int(v)
    }

    fn serialize_i128(self, v: i128) -> Result<Native, NativeError> {
        int(v)
    }

    fn serialize_u8(self, v: u8) -> Result<Native, NativeError> {
        uint(v)
    }

    fn serialize_u16(self, v: u16) -> Result<Native, NativeError> {
        uint(v)
    }

    fn serialize_u32(self, v: u32) -> Result<Native, NativeError> {
        uint(v)
    }

    fn serialize_u64(self, v: u64) -> Result<Native, NativeError> {
        uint(v)
    }

    fn serialize_u128(self, v: u128) -> Result<Native, NativeError> {
        uint(v)
    }

    fn serialize_f32(self, v: f32) -> Result<Native, NativeError> {
        Ok(Native::Double(f64::from(v)))
    }

    fn serialize_f64(self, v: f64) -> Result<Native, NativeError> {
        Ok(Native::Double(v))
    }

    fn serialize_char(self, v: char) -> Result<Native, NativeError> {
        Ok(Native::Str(v.to_string()))
    }

    fn serialize_str(self, v: &str) -> Result<Native, NativeError> {
        Ok(Native::Str(v.to_owned()))
    }

    fn serialize_bytes(self, v: &[u8]) -> Result<Native, NativeError> {
        Ok(Native::Bytes(v.to_vec()))
    }

    fn serialize_none(self) -> Result<Native, NativeError> {
        if self.field {
            Err(NativeError(ABSENT.to_owned()))
        } else {
            Ok(Native::Null)
        }
    }

    fn serialize_some<T: Serialize + ?Sized>(self, v: &T) -> Result<Native, NativeError> {
        v.serialize(VALUE)
    }

    fn serialize_unit(self) -> Result<Native, NativeError> {
        Ok(Native::Null)
    }

    fn serialize_unit_struct(self, _: &'static str) -> Result<Native, NativeError> {
        Ok(Native::Null)
    }

    fn serialize_unit_variant(
        self,
        _: &'static str,
        _: u32,
        variant: &'static str,
    ) -> Result<Native, NativeError> {
        Ok(Native::Str(variant.to_owned()))
    }

    fn serialize_newtype_struct<T: Serialize + ?Sized>(
        self,
        _: &'static str,
        v: &T,
    ) -> Result<Native, NativeError> {
        v.serialize(self)
    }

    fn serialize_newtype_variant<T: Serialize + ?Sized>(
        self,
        _: &'static str,
        _: u32,
        variant: &'static str,
        v: &T,
    ) -> Result<Native, NativeError> {
        Ok(Native::Struct(NativeStruct::from([(
            variant.to_owned(),
            v.serialize(VALUE)?,
        )])))
    }

    fn serialize_seq(self, len: Option<usize>) -> Result<SeqEnc, NativeError> {
        Ok(SeqEnc(Vec::with_capacity(len.unwrap_or(0))))
    }

    fn serialize_tuple(self, len: usize) -> Result<SeqEnc, NativeError> {
        Ok(SeqEnc(Vec::with_capacity(len)))
    }

    fn serialize_tuple_struct(self, _: &'static str, len: usize) -> Result<SeqEnc, NativeError> {
        Ok(SeqEnc(Vec::with_capacity(len)))
    }

    fn serialize_tuple_variant(
        self,
        _: &'static str,
        _: u32,
        variant: &'static str,
        len: usize,
    ) -> Result<Tagged<SeqEnc>, NativeError> {
        Ok(Tagged(variant, SeqEnc(Vec::with_capacity(len))))
    }

    fn serialize_map(self, _: Option<usize>) -> Result<MapEnc, NativeError> {
        Ok(MapEnc(NativeStruct::new(), None))
    }

    fn serialize_struct(self, _: &'static str, _: usize) -> Result<StructEnc, NativeError> {
        Ok(StructEnc(NativeStruct::new()))
    }

    fn serialize_struct_variant(
        self,
        _: &'static str,
        _: u32,
        variant: &'static str,
        _: usize,
    ) -> Result<Tagged<StructEnc>, NativeError> {
        Ok(Tagged(variant, StructEnc(NativeStruct::new())))
    }
}

pub struct SeqEnc(Vec<Native>);

impl ser::SerializeSeq for SeqEnc {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_element<T: Serialize + ?Sized>(&mut self, v: &T) -> Result<(), NativeError> {
        self.0.push(v.serialize(VALUE)?);
        Ok(())
    }

    fn end(self) -> Result<Native, NativeError> {
        Ok(Native::List(self.0))
    }
}

impl ser::SerializeTuple for SeqEnc {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_element<T: Serialize + ?Sized>(&mut self, v: &T) -> Result<(), NativeError> {
        ser::SerializeSeq::serialize_element(self, v)
    }

    fn end(self) -> Result<Native, NativeError> {
        ser::SerializeSeq::end(self)
    }
}

impl ser::SerializeTupleStruct for SeqEnc {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_field<T: Serialize + ?Sized>(&mut self, v: &T) -> Result<(), NativeError> {
        ser::SerializeSeq::serialize_element(self, v)
    }

    fn end(self) -> Result<Native, NativeError> {
        ser::SerializeSeq::end(self)
    }
}

/// A variant's content under its name (serde's external tagging).
pub struct Tagged<S>(&'static str, S);

impl ser::SerializeTupleVariant for Tagged<SeqEnc> {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_field<T: Serialize + ?Sized>(&mut self, v: &T) -> Result<(), NativeError> {
        ser::SerializeSeq::serialize_element(&mut self.1, v)
    }

    fn end(self) -> Result<Native, NativeError> {
        Ok(Native::Struct(NativeStruct::from([(
            self.0.to_owned(),
            Native::List(self.1 .0),
        )])))
    }
}

impl ser::SerializeStructVariant for Tagged<StructEnc> {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_field<T: Serialize + ?Sized>(
        &mut self,
        key: &'static str,
        v: &T,
    ) -> Result<(), NativeError> {
        ser::SerializeStruct::serialize_field(&mut self.1, key, v)
    }

    fn end(self) -> Result<Native, NativeError> {
        Ok(Native::Struct(NativeStruct::from([(
            self.0.to_owned(),
            Native::Struct(self.1 .0),
        )])))
    }
}

pub struct MapEnc(NativeStruct, Option<String>);

impl ser::SerializeMap for MapEnc {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_key<T: Serialize + ?Sized>(&mut self, key: &T) -> Result<(), NativeError> {
        self.1 = Some(map_key(key.serialize(VALUE)?)?);
        Ok(())
    }

    fn serialize_value<T: Serialize + ?Sized>(&mut self, v: &T) -> Result<(), NativeError> {
        let key = self
            .1
            .take()
            .ok_or_else(|| NativeError("map value without a key".to_owned()))?;
        self.0.insert(key, v.serialize(VALUE)?);
        Ok(())
    }

    fn end(self) -> Result<Native, NativeError> {
        Ok(Native::Struct(self.0))
    }
}

/// Map keys are text, as on the wire: strings as is, integers and bools
/// in their decimal / `true`/`false` form.
fn map_key(k: Native) -> Result<String, NativeError> {
    match k {
        Native::Str(s) => Ok(s),
        Native::Int(n) => Ok(n.to_string()),
        Native::Uint(n) => Ok(n.to_string()),
        Native::Bool(b) => Ok(b.to_string()),
        _ => Err(NativeError(
            "map keys must be strings, integers or bools".to_owned(),
        )),
    }
}

pub struct StructEnc(NativeStruct);

impl ser::SerializeStruct for StructEnc {
    type Ok = Native;
    type Error = NativeError;

    fn serialize_field<T: Serialize + ?Sized>(
        &mut self,
        key: &'static str,
        v: &T,
    ) -> Result<(), NativeError> {
        match v.serialize(Enc { field: true }) {
            Ok(value) => {
                self.0.insert(key.to_owned(), value);
                Ok(())
            }
            Err(e) if e.0 == ABSENT => Ok(()),
            Err(e) => Err(e),
        }
    }

    fn end(self) -> Result<Native, NativeError> {
        Ok(Native::Struct(self.0))
    }
}

// =============================================================================
// Schema fit
// =============================================================================

/// Reshapes `values` where serde's form differs from the schema's kind:
/// byte sequences into Bytes, `std::time` structs into Duration and
/// Timestamp. Everything else is left to validation.
pub fn fit(schema: &Schema, values: &mut NativeStruct) {
    fit_struct(schema, &schema.defs, values);
}

fn fit_struct(s: &Schema, defs: &HashMap<String, Schema>, m: &mut NativeStruct) {
    for f in &s.fields {
        if let Some(v) = m.get_mut(&f.name) {
            fit_value(f, defs, v);
        }
    }
}

fn fit_value(f: &SchemaField, defs: &HashMap<String, Schema>, v: &mut Native) {
    let Some(kind) = f.kind.as_ref() else {
        return;
    };
    match (kind, &mut *v) {
        (K::Bytes(_), Native::List(items)) => {
            if let Some(bytes) = items.iter().map(byte).collect::<Option<Vec<u8>>>() {
                *v = Native::Bytes(bytes);
            }
        }
        (K::Duration(_), Native::Struct(m)) => {
            if let Some(d) = std_duration(m) {
                *v = Native::Duration(d);
            }
        }
        (K::Timestamp(_), Native::Struct(m)) => {
            if let Some(t) = system_time(m) {
                *v = Native::Timestamp(t);
            }
        }
        (K::List(l), Native::List(items)) => {
            for (i, el) in items.iter_mut().enumerate() {
                let def = if l.items.len() == 1 {
                    l.items.first()
                } else {
                    l.items.get(i)
                };
                if let Some(def) = def {
                    fit_value(def, defs, el);
                }
            }
        }
        (K::Map(mp), Native::Struct(m)) => {
            for el in m.values_mut() {
                if let Some(vf) = mp.value_field.as_ref() {
                    fit_value(vf, defs, el);
                } else if let (Some(vs), Native::Struct(em)) = (mp.value_schema.as_ref(), el) {
                    fit_struct(vs, defs, em);
                }
            }
        }
        (K::Object(_) | K::OneOf(_) | K::Ref(_), Native::Struct(_)) => {
            if let Some(sub) = object_schema(f, v, defs).cloned() {
                if let Native::Struct(m) = v {
                    fit_struct(&sub, defs, m);
                }
            }
        }
        _ => {}
    }
}

fn byte(v: &Native) -> Option<u8> {
    match v {
        Native::Uint(n) => u8::try_from(*n).ok(),
        Native::Int(n) => u8::try_from(*n).ok(),
        _ => None,
    }
}

fn std_duration(m: &NativeStruct) -> Option<pbjson_types::Duration> {
    let (secs, nanos) = (m.get("secs")?, m.get("nanos")?);
    if m.len() != 2 {
        return None;
    }
    Some(pbjson_types::Duration {
        seconds: i64::try_from(crate::value::as_uint(secs)?).ok()?,
        nanos: i32::try_from(crate::value::as_uint(nanos)?).ok()?,
    })
}

fn system_time(m: &NativeStruct) -> Option<pbjson_types::Timestamp> {
    let (secs, nanos) = (m.get("secs_since_epoch")?, m.get("nanos_since_epoch")?);
    if m.len() != 2 {
        return None;
    }
    Some(pbjson_types::Timestamp {
        seconds: i64::try_from(crate::value::as_uint(secs)?).ok()?,
        nanos: i32::try_from(crate::value::as_uint(nanos)?).ok()?,
    })
}

// =============================================================================
// Deserializer
// =============================================================================

/// A serde deserializer over a native tree (see [`decode`]).
pub struct Dec(Native);

impl IntoDeserializer<'_, NativeError> for Native {
    type Deserializer = Dec;

    fn into_deserializer(self) -> Dec {
        Dec(self)
    }
}

/// protoJSON's duration form (`"5400s"`, `"1.5s"`): what `pbjson_types`
/// reads, and a form Go-style and humantime parsers accept too.
fn proto_duration(d: &pbjson_types::Duration) -> String {
    let negative = d.seconds < 0 || d.nanos < 0;
    let secs = d.seconds.unsigned_abs();
    let nanos = d.nanos.unsigned_abs();
    let sign = if negative { "-" } else { "" };
    if nanos == 0 {
        format!("{sign}{secs}s")
    } else {
        let frac = format!("{nanos:09}");
        format!("{sign}{secs}.{}s", frac.trim_end_matches('0'))
    }
}

impl Dec {
    fn unexpected(&self, want: &str) -> NativeError {
        NativeError(format!("expected {want}, found {}", kind(&self.0)))
    }
}

const fn kind(v: &Native) -> &'static str {
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

fn byte_seq(bytes: Vec<u8>) -> SeqDeserializer<std::vec::IntoIter<Native>, NativeError> {
    SeqDeserializer::new(
        bytes
            .into_iter()
            .map(|b| Native::Uint(u64::from(b)))
            .collect::<Vec<_>>()
            .into_iter(),
    )
}

impl<'de> de::Deserializer<'de> for Dec {
    type Error = NativeError;

    fn deserialize_any<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Null => v.visit_unit(),
            Native::Bool(b) => v.visit_bool(b),
            Native::Int(n) => v.visit_i64(n),
            Native::Uint(n) => v.visit_u64(n),
            Native::Double(n) => v.visit_f64(n),
            Native::Str(s) => v.visit_string(s),
            Native::Bytes(b) => v.visit_byte_buf(b),
            Native::Duration(d) => v.visit_string(proto_duration(&d)),
            Native::Timestamp(t) => v.visit_string(format_rfc3339(&t)),
            Native::List(items) => v.visit_seq(SeqDeserializer::new(items.into_iter())),
            Native::Struct(m) => v.visit_map(MapDeserializer::new(
                m.into_iter().map(|(k, val)| (KeyDec(k), val)),
            )),
        }
    }

    fn deserialize_option<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Null => v.visit_none(),
            other => v.visit_some(Self(other)),
        }
    }

    fn deserialize_unit<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Null => v.visit_unit(),
            _ => Err(self.unexpected("null")),
        }
    }

    fn deserialize_unit_struct<V: Visitor<'de>>(
        self,
        _: &'static str,
        v: V,
    ) -> Result<V::Value, NativeError> {
        self.deserialize_unit(v)
    }

    fn deserialize_newtype_struct<V: Visitor<'de>>(
        self,
        _: &'static str,
        v: V,
    ) -> Result<V::Value, NativeError> {
        v.visit_newtype_struct(self)
    }

    fn deserialize_bytes<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        self.deserialize_byte_buf(v)
    }

    fn deserialize_byte_buf<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Bytes(b) => v.visit_byte_buf(b),
            other => Self(other).deserialize_any(v),
        }
    }

    /// Bytes read as a sequence of numbers (a `Vec<u8>`, a `[u8; N]`).
    fn deserialize_seq<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Bytes(b) => v.visit_seq(byte_seq(b)),
            other => Self(other).deserialize_any(v),
        }
    }

    fn deserialize_tuple<V: Visitor<'de>>(self, _: usize, v: V) -> Result<V::Value, NativeError> {
        self.deserialize_seq(v)
    }

    fn deserialize_tuple_struct<V: Visitor<'de>>(
        self,
        _: &'static str,
        _: usize,
        v: V,
    ) -> Result<V::Value, NativeError> {
        self.deserialize_seq(v)
    }

    /// `std::time::Duration` and `SystemTime` ask for their struct form.
    fn deserialize_struct<V: Visitor<'de>>(
        self,
        name: &'static str,
        fields: &'static [&'static str],
        v: V,
    ) -> Result<V::Value, NativeError> {
        match (self.0, fields) {
            (Native::Duration(d), ["secs", "nanos"]) => {
                let secs = u64::try_from(d.seconds)
                    .map_err(|_| NativeError(format!("{name}: a negative duration")))?;
                let nanos = u64::try_from(d.nanos)
                    .map_err(|_| NativeError(format!("{name}: a negative duration")))?;
                v.visit_map(MapDeserializer::new(
                    [("secs", Native::Uint(secs)), ("nanos", Native::Uint(nanos))].into_iter(),
                ))
            }
            (Native::Timestamp(t), ["secs_since_epoch", "nanos_since_epoch"]) => {
                let secs = u64::try_from(t.seconds)
                    .map_err(|_| NativeError(format!("{name}: a time before 1970")))?;
                let nanos = u64::try_from(t.nanos)
                    .map_err(|_| NativeError(format!("{name}: a negative nanos")))?;
                v.visit_map(MapDeserializer::new(
                    [
                        ("secs_since_epoch", Native::Uint(secs)),
                        ("nanos_since_epoch", Native::Uint(nanos)),
                    ]
                    .into_iter(),
                ))
            }
            (other, _) => Self(other).deserialize_any(v),
        }
    }

    fn deserialize_enum<V: Visitor<'de>>(
        self,
        _: &'static str,
        _: &'static [&'static str],
        v: V,
    ) -> Result<V::Value, NativeError> {
        match self.0 {
            Native::Str(variant) => v.visit_enum(variant.into_deserializer()),
            Native::Struct(m) if m.len() == 1 => {
                let (variant, value) = m
                    .into_iter()
                    .next()
                    .unwrap_or_else(|| unreachable!("one entry"));
                v.visit_enum(VariantDec(variant, value))
            }
            other => Err(Self(other).unexpected("an enum variant")),
        }
    }

    fn deserialize_ignored_any<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        v.visit_unit()
    }

    serde::forward_to_deserialize_any! {
        bool i8 i16 i32 i64 i128 u8 u16 u32 u64 u128 f32 f64 char str string
        map identifier
    }
}

/// A map key: text on the wire, read back as the integer or bool the key
/// type asks for.
pub struct KeyDec(String);

impl IntoDeserializer<'_, NativeError> for KeyDec {
    type Deserializer = Self;

    fn into_deserializer(self) -> Self {
        self
    }
}

macro_rules! parsed_key {
    ($($method:ident => $visit:ident($ty:ty)),*) => {$(
        fn $method<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
            let n: $ty = self
                .0
                .parse()
                .map_err(|_| NativeError(format!("map key {:?} is not a {}", self.0, stringify!($ty))))?;
            v.$visit(n)
        }
    )*};
}

impl<'de> de::Deserializer<'de> for KeyDec {
    type Error = NativeError;

    fn deserialize_any<V: Visitor<'de>>(self, v: V) -> Result<V::Value, NativeError> {
        v.visit_string(self.0)
    }

    parsed_key!(
        deserialize_bool => visit_bool(bool),
        deserialize_i8 => visit_i8(i8),
        deserialize_i16 => visit_i16(i16),
        deserialize_i32 => visit_i32(i32),
        deserialize_i64 => visit_i64(i64),
        deserialize_u8 => visit_u8(u8),
        deserialize_u16 => visit_u16(u16),
        deserialize_u32 => visit_u32(u32),
        deserialize_u64 => visit_u64(u64)
    );

    fn deserialize_newtype_struct<V: Visitor<'de>>(
        self,
        _: &'static str,
        v: V,
    ) -> Result<V::Value, NativeError> {
        v.visit_newtype_struct(self)
    }

    fn deserialize_enum<V: Visitor<'de>>(
        self,
        _: &'static str,
        _: &'static [&'static str],
        v: V,
    ) -> Result<V::Value, NativeError> {
        v.visit_enum(self.0.into_deserializer())
    }

    serde::forward_to_deserialize_any! {
        i128 u128 f32 f64 char str string bytes byte_buf option unit unit_struct
        seq tuple tuple_struct map struct identifier ignored_any
    }
}

struct VariantDec(String, Native);

impl<'de> de::EnumAccess<'de> for VariantDec {
    type Error = NativeError;
    type Variant = Dec;

    fn variant_seed<S: de::DeserializeSeed<'de>>(
        self,
        seed: S,
    ) -> Result<(S::Value, Dec), NativeError> {
        let variant = seed.deserialize(self.0.into_deserializer())?;
        Ok((variant, Dec(self.1)))
    }
}

impl<'de> de::VariantAccess<'de> for Dec {
    type Error = NativeError;

    fn unit_variant(self) -> Result<(), NativeError> {
        Ok(())
    }

    fn newtype_variant_seed<S: de::DeserializeSeed<'de>>(
        self,
        seed: S,
    ) -> Result<S::Value, NativeError> {
        seed.deserialize(self)
    }

    fn tuple_variant<V: Visitor<'de>>(self, _: usize, v: V) -> Result<V::Value, NativeError> {
        de::Deserializer::deserialize_seq(self, v)
    }

    fn struct_variant<V: Visitor<'de>>(
        self,
        _: &'static [&'static str],
        v: V,
    ) -> Result<V::Value, NativeError> {
        de::Deserializer::deserialize_any(self, v)
    }
}
