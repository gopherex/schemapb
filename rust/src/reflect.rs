//! Reflect: a Rust struct -> a Schema, the trait side.
//!
//! `#[derive(Reflect)]` (feature `derive`) generates an impl of
//! [`ReflectField`] for a struct; this module provides the impls for every
//! leaf and container type and the [`reflect_schema`] entry point. Nested
//! types recurse through the TRAIT — the Rust-idiomatic `WithType`
//! override is simply implementing [`ReflectField`] for your own type
//! (serde-style).
//!
//! Shape rules mirror the other implementations: `Option<T>` -> optional
//! AND nullable, `Vec<u8>` -> Bytes (the schemapb wire assumption; use an
//! override impl for serde's number-array encoding), `Vec<T>` -> List,
//! `[T; N]` -> List with exactly N items (`[u8; N]` -> Bytes of length N),
//! string-keyed maps -> Map (`value_schema` for derived structs,
//! `value_field` otherwise), `serde_json::Value` -> JSON,
//! `pbjson_types::{Duration, Timestamp}` -> Duration/Timestamp, and a
//! type CYCLE degrades to JSON (detected at runtime via a visit stack).

use std::any::TypeId;
use std::cell::RefCell;
use std::collections::{BTreeMap, HashMap};

use crate::descriptor::SchemaError;
use crate::gen::schemapb::schema::field::{
    Bytes as BytesKind, Kind as K, List as ListKind, Map as MapKind, Object as ObjectKind,
};
use crate::gen::schemapb::{schema::Field, Schema, SchemaIdentity};

/// One reflectable position: what field definition this type contributes.
pub trait ReflectField {
    /// Whether an unwrapped value of this type is present by default
    /// (`Option<T>` flips this off).
    const REQUIRED: bool = true;

    /// The field definition for this type under `name`.
    fn field(name: &str) -> Field;

    /// The fields of this type when it is a struct (drives map
    /// `value_schema` vs `value_field`); `None` for non-objects.
    #[must_use]
    fn object_fields() -> Option<Vec<Field>> {
        None
    }
}

/// Builds a Schema from a reflectable struct (coercion enabled on the
/// root).
///
/// Compile-time unrepresentables are compile errors; the result still
/// runs descriptor validation like every other authored schema.
pub fn reflect_schema<T: ReflectField>(id: SchemaIdentity) -> Result<Schema, SchemaError> {
    let fields = T::object_fields().unwrap_or_else(|| vec![T::field("value")]);
    let schema = Schema {
        id: Some(id),
        coerce: true,
        fields,
        ..Default::default()
    };

    let errs = crate::descriptor::check_descriptor(&schema);
    if errs.is_empty() {
        Ok(schema)
    } else {
        Err(SchemaError {
            result: crate::gen::schemapb::ValidationResult { errors: errs },
        })
    }
}

thread_local! {
    static VISITING: RefCell<Vec<TypeId>> = const { RefCell::new(Vec::new()) };
}

/// Runs `f` with `t` on the visit stack; a re-entry yields `None` (a
/// cycle: the shape is not finite, the caller degrades to JSON).
pub fn visiting<R>(t: TypeId, f: impl FnOnce() -> R) -> Option<R> {
    let entered = VISITING.with(|v| {
        let mut stack = v.borrow_mut();
        if stack.contains(&t) {
            return false;
        }
        stack.push(t);
        true
    });

    if !entered {
        return None;
    }

    let out = f();
    VISITING.with(|v| {
        v.borrow_mut().pop();
    });

    Some(out)
}

/// A JSON field (the cycle/unknown-shape fallback).
#[must_use]
pub fn json_field(name: &str) -> Field {
    Field {
        name: name.to_owned(),
        kind: Some(K::Json(crate::gen::schemapb::schema::field::Json::default())),
        ..Default::default()
    }
}

fn leaf(name: &str, kind: K) -> Field {
    Field {
        name: name.to_owned(),
        kind: Some(kind),
        ..Default::default()
    }
}

macro_rules! leaf_impl {
    ($ty:ty, $kind:ident) => {
        impl ReflectField for $ty {
            fn field(name: &str) -> Field {
                leaf(name, K::$kind(Default::default()))
            }
        }
    };
}

leaf_impl!(bool, Bool);
leaf_impl!(String, String);
leaf_impl!(i8, Int32);
leaf_impl!(i16, Int32);
leaf_impl!(i32, Int32);
leaf_impl!(i64, Int64);
leaf_impl!(u16, Uint32);
leaf_impl!(u32, Uint32);
leaf_impl!(u64, Uint64);
leaf_impl!(f32, Float);
leaf_impl!(f64, Double);
leaf_impl!(pbjson_types::Duration, Duration);
leaf_impl!(pbjson_types::Timestamp, Timestamp);

impl ReflectField for serde_json::Value {
    fn field(name: &str) -> Field {
        json_field(name)
    }
}

impl<T: ReflectField> ReflectField for Option<T> {
    const REQUIRED: bool = false;

    fn field(name: &str) -> Field {
        let mut f = T::field(name);
        // serde writes null for None: the schema must tolerate it.
        f.nullable = true;
        f
    }

    fn object_fields() -> Option<Vec<Field>> {
        T::object_fields()
    }
}

impl<T: ReflectField> ReflectField for Box<T> {
    const REQUIRED: bool = T::REQUIRED;

    fn field(name: &str) -> Field {
        T::field(name)
    }

    fn object_fields() -> Option<Vec<Field>> {
        T::object_fields()
    }
}

/// `Vec<u8>` is Bytes on the schemapb wire; implement [`ReflectField`] for
/// a newtype if your serde encoding is a number array instead.
impl ReflectField for Vec<u8> {
    fn field(name: &str) -> Field {
        leaf(name, K::Bytes(<BytesKind as Default>::default()))
    }
}

impl<const N: usize> ReflectField for [u8; N] {
    fn field(name: &str) -> Field {
        leaf(
            name,
            K::Bytes(BytesKind {
                len: Some(N as u64),
                ..Default::default()
            }),
        )
    }
}

fn item_of<T: ReflectField>() -> Field {
    let mut item = T::field("item");
    item.required = true;
    item
}

macro_rules! list_impl {
    ($ty:ty) => {
        impl<T: ReflectField + 'static> ReflectField for $ty {
            fn field(name: &str) -> Field {
                let item =
                    visiting(TypeId::of::<T>(), item_of::<T>).unwrap_or_else(|| json_field("item"));
                leaf(
                    name,
                    K::List(ListKind {
                        items: vec![item],
                        ..Default::default()
                    }),
                )
            }
        }
    };
}

list_impl!(Vec<T>);

impl<T: ReflectField + 'static, const N: usize> ReflectField for [T; N] {
    fn field(name: &str) -> Field {
        let item = visiting(TypeId::of::<T>(), item_of::<T>).unwrap_or_else(|| json_field("item"));
        leaf(
            name,
            K::List(ListKind {
                items: vec![item],
                min_items: Some(N as u64),
                max_items: Some(N as u64),
                ..Default::default()
            }),
        )
    }
}

fn map_field<T: ReflectField + 'static>(name: &str) -> Field {
    let kind = visiting(TypeId::of::<T>(), || {
        T::object_fields().map_or_else(
            || {
                let mut value = T::field("value");
                value.required = true;
                MapKind {
                    value_field: Some(Box::new(value)),
                    ..Default::default()
                }
            },
            |fields| MapKind {
                value_schema: Some(Schema {
                    fields,
                    ..Default::default()
                }),
                ..Default::default()
            },
        )
    })
    .unwrap_or_else(|| MapKind {
        value_field: Some(Box::new({
            let mut f = json_field("value");
            f.required = true;
            f
        })),
        ..Default::default()
    });

    leaf(name, K::Map(Box::new(kind)))
}

impl<T: ReflectField + 'static> ReflectField for HashMap<String, T> {
    fn field(name: &str) -> Field {
        map_field::<T>(name)
    }
}

impl<T: ReflectField + 'static> ReflectField for BTreeMap<String, T> {
    fn field(name: &str) -> Field {
        map_field::<T>(name)
    }
}

/// A derived struct's own Object field: shared by the derive expansion.
#[must_use]
pub fn object_field(name: &str, fields: Vec<Field>) -> Field {
    leaf(
        name,
        K::Object(ObjectKind {
            schema: Some(Schema {
                fields,
                ..Default::default()
            }),
        }),
    )
}

// =============================================================================
// Derive-expansion helpers: constraint application. The derive macro passes
// attribute values as the literal tokens' text and these parse them against
// the field's actual kind — exactly the Go tag-string discipline. A
// constraint on a kind that cannot carry it PANICS with the field name: a
// derive misuse is a programmer error surfaced on first reflection.
// =============================================================================

#[doc(hidden)]
pub mod apply {
    use super::{Field, K};

    fn oops(field: &str, what: &str) -> ! {
        panic!("schemapb derive: field {field:?}: {what}");
    }

    fn parse<T: std::str::FromStr>(field: &str, key: &str, raw: &str) -> T {
        raw.parse().unwrap_or_else(|_| {
            oops(
                field,
                &format!("{key}={raw}: not a valid value for this kind"),
            )
        })
    }

    pub fn desc(f: &mut Field, d: &str) {
        f.description = Some(d.to_owned());
    }

    pub const fn required(f: &mut Field) {
        f.required = true;
    }

    pub const fn secret(f: &mut Field) {
        f.secret = true;
    }

    /// validator semantics: `min`/`max` mean length for strings/bytes,
    /// item count for lists, entry count for maps, >= / <= for numbers.
    #[allow(clippy::missing_panics_doc)]
    pub fn constraint(f: &mut Field, key: &str, raw: &str) {
        let name = f.name.clone();
        match f.kind.as_mut() {
            Some(K::String(k)) => match key {
                "min" => k.min_len = Some(parse(&name, key, raw)),
                "max" => k.max_len = Some(parse(&name, key, raw)),
                "len" => k.len = Some(parse(&name, key, raw)),
                "pattern" => k.pattern = Some(raw.to_owned()),
                "format" => k.format = Some(raw.to_owned()),
                _ => oops(&name, &format!("{key} does not apply to string")),
            },
            Some(K::Bytes(k)) => match key {
                "min" => k.min_len = Some(parse(&name, key, raw)),
                "max" => k.max_len = Some(parse(&name, key, raw)),
                "len" => k.len = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to bytes")),
            },
            Some(K::List(k)) => match key {
                "min" => k.min_items = Some(parse(&name, key, raw)),
                "max" => k.max_items = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to list")),
            },
            Some(K::Map(k)) => match key {
                "min" => k.min_entries = Some(parse(&name, key, raw)),
                "max" => k.max_entries = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to map")),
            },
            Some(K::Int32(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to int32")),
            },
            Some(K::Int64(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to int64")),
            },
            Some(K::Uint32(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to uint32")),
            },
            Some(K::Uint64(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to uint64")),
            },
            Some(K::Float(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to float")),
            },
            Some(K::Double(k)) => match key {
                "min" | "gte" => k.gte = Some(parse(&name, key, raw)),
                "gt" => k.gt = Some(parse(&name, key, raw)),
                "max" | "lte" => k.lte = Some(parse(&name, key, raw)),
                "lt" => k.lt = Some(parse(&name, key, raw)),
                _ => oops(&name, &format!("{key} does not apply to double")),
            },
            _ => oops(&name, &format!("{key} does not apply to this kind")),
        }
    }

    /// Replaces the field with a string Choice (`oneof` of strings).
    #[must_use]
    pub fn choice_str(name: &str, options: &[&str]) -> Field {
        use crate::gen::schemapb::schema::field::{choice::Option as ChoiceOption, Choice};
        Field {
            name: name.to_owned(),
            kind: Some(K::Choice(Choice {
                options: options
                    .iter()
                    .map(|o| ChoiceOption {
                        value: Some(crate::value::str_v(o)),
                        ..Default::default()
                    })
                    .collect(),
                ..Default::default()
            })),
            ..Default::default()
        }
    }

    /// Replaces the field with an integer Choice (`oneof` of ints).
    #[must_use]
    pub fn choice_int(name: &str, options: &[i64]) -> Field {
        use crate::gen::schemapb::schema::field::{choice::Option as ChoiceOption, Choice};
        Field {
            name: name.to_owned(),
            kind: Some(K::Choice(Choice {
                options: options
                    .iter()
                    .map(|o| ChoiceOption {
                        value: Some(crate::value::int64_v(*o)),
                        ..Default::default()
                    })
                    .collect(),
                ..Default::default()
            })),
            ..Default::default()
        }
    }
}
