//! Reflect: a Rust type -> a Schema field, the trait side.
//!
//! `#[derive(Reflect)]` / `#[derive(Validate)]` (feature `derive`) generate
//! an impl of [`ReflectField`] for a struct or an enum; this module provides
//! the impls for leaf and container types, the [`reflect_schema`] entry
//! point and the typed attribute functions the derive expansion calls
//! ([`attr`]). Nested types recurse through the TRAIT: implementing
//! [`ReflectField`] for your own type is the override mechanism
//! (serde-style).
//!
//! Shape rules mirror the other implementations: `Option<T>` -> optional
//! AND nullable, `Vec<u8>` -> Bytes (the schemapb wire form), `Vec<T>` and
//! the sets -> List (sets unique), `[T; N]` -> List with exactly N items
//! (`[u8; N]` -> Bytes of length N), tuples -> positional lists,
//! string-keyed maps -> Map (`value_schema` for derived structs,
//! `value_field` otherwise), `serde_json::Value` -> JSON, durations
//! (`std::time::Duration`, `pbjson_types::Duration`) -> Duration,
//! `SystemTime` and `pbjson_types::Timestamp` -> Timestamp, and a type CYCLE
//! degrades to JSON (detected at runtime via a visit stack). Integer types
//! narrower than their kind carry their range (`u8` -> uint32 `lte 255`),
//! `NonZero*` exclude zero, `std::time::Duration` is never negative.
//!
//! A struct field whose type is a derived struct (not `Option<T>`) is a
//! SECTION: optional with an implicit empty object default (Go's
//! `schemapb:"default={}"`), so an absent section resolves its inner
//! defaults. A `bool` field is false when absent (Go: a bool without
//! `omitempty`). `#[schemapb(required)]` keeps either required without a
//! default (Go's `validate:"required"`); a hand-written [`ReflectField`]
//! impl (an override) is never a section.

use std::any::TypeId;
use std::borrow::Cow;
use std::cell::RefCell;
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet, VecDeque};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use std::num::{
    NonZeroI16, NonZeroI32, NonZeroI64, NonZeroI8, NonZeroIsize, NonZeroU16, NonZeroU32,
    NonZeroU64, NonZeroU8, NonZeroUsize,
};
use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::time::{Duration as StdDuration, SystemTime};

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

    /// Whether a struct field of this type is a section (optional with an
    /// implicit empty object default). `#[derive(Reflect)]` sets it; an
    /// override impl leaves it off.
    const SECTION: bool = false;

    /// The field definition for this type under `name`.
    fn field(name: &str) -> Field;

    /// The fields of this type when it is a struct (drives map
    /// `value_schema` vs `value_field`); `None` for non-objects.
    #[must_use]
    fn object_fields() -> Option<Vec<Field>> {
        None
    }

    /// Schema-level rules and attributes of a derived struct (its
    /// `#[schemapb(rule(...), strict)]`); applied wherever its fields form
    /// a schema.
    fn decorate_schema(_schema: &mut Schema) {}
}

/// Builds a Schema from a reflectable struct (coercion enabled on the
/// root).
///
/// Compile-time unrepresentables are compile errors; the result still
/// runs descriptor validation like every other authored schema.
pub fn reflect_schema<T: ReflectField>(id: SchemaIdentity) -> Result<Schema, SchemaError> {
    let fields = T::object_fields().unwrap_or_else(|| vec![T::field("value")]);
    let mut schema = Schema {
        id: Some(id),
        coerce: true,
        fields,
        ..Default::default()
    };
    T::decorate_schema(&mut schema);

    let errs = crate::descriptor::check_descriptor(&schema);
    if errs.is_empty() {
        Ok(schema)
    } else {
        Err(SchemaError {
            result: crate::gen::schemapb::ValidationResult {
                errors: errs,
                schema: None,
            },
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
    ($($ty:ty => $kind:ident),* $(,)?) => {$(
        impl ReflectField for $ty {
            fn field(name: &str) -> Field {
                leaf(name, K::$kind(Default::default()))
            }
        }
    )*};
}

leaf_impl!(
    bool => Bool,
    String => String,
    str => String,
    i32 => Int32,
    i64 => Int64,
    isize => Int64,
    u32 => Uint32,
    u64 => Uint64,
    usize => Uint64,
    f32 => Float,
    f64 => Double,
    PathBuf => String,
    pbjson_types::Duration => Duration,
    pbjson_types::Timestamp => Timestamp,
    SystemTime => Timestamp,
);

/// Integers narrower than their kind carry their range, so a value that
/// validates also fits the Rust type it decodes into.
macro_rules! narrow_int {
    ($($ty:ty => $kind:ident($msg:ident, $wide:ty)),* $(,)?) => {$(
        impl ReflectField for $ty {
            fn field(name: &str) -> Field {
                leaf(name, K::$kind(crate::gen::schemapb::schema::field::$msg {
                    gte: (<$wide>::from(<$ty>::MIN) != <$wide>::MIN).then(|| <$wide>::from(<$ty>::MIN)),
                    lte: Some(<$wide>::from(<$ty>::MAX)),
                    ..Default::default()
                }))
            }
        }
    )*};
}

narrow_int!(
    i8 => Int32(Int32, i32),
    i16 => Int32(Int32, i32),
    u8 => Uint32(UInt32, u32),
    u16 => Uint32(UInt32, u32),
);

/// `NonZero*` excludes zero: `gt 0` unsigned, `not_in [0]` signed.
macro_rules! nonzero {
    (unsigned: $($nz:ty => $inner:ty),*; signed: $($snz:ty => $sinner:ty),*) => {
        $(impl ReflectField for $nz {
            fn field(name: &str) -> Field {
                let mut f = <$inner>::field(name);
                attr::set_bound(&mut f, attr::Which::Gt, attr::Num::U(0));
                f
            }
        })*
        $(impl ReflectField for $snz {
            fn field(name: &str) -> Field {
                let mut f = <$sinner>::field(name);
                attr::forbid_zero(&mut f);
                f
            }
        })*
    };
}

nonzero!(
    unsigned: NonZeroU8 => u8, NonZeroU16 => u16, NonZeroU32 => u32, NonZeroU64 => u64,
        NonZeroUsize => usize;
    signed: NonZeroI8 => i8, NonZeroI16 => i16, NonZeroI32 => i32, NonZeroI64 => i64,
        NonZeroIsize => isize
);

/// A `char` is a one-code-point string.
impl ReflectField for char {
    fn field(name: &str) -> Field {
        leaf(
            name,
            K::String(crate::gen::schemapb::schema::field::String {
                len: Some(1),
                ..Default::default()
            }),
        )
    }
}

/// `std::time::Duration` cannot be negative.
impl ReflectField for StdDuration {
    fn field(name: &str) -> Field {
        leaf(
            name,
            K::Duration(crate::gen::schemapb::schema::field::Duration {
                gte: Some(pbjson_types::Duration::default()),
                ..Default::default()
            }),
        )
    }
}

/// A string field carrying a named format.
#[must_use]
pub fn formatted(name: &str, format: &str) -> Field {
    leaf(
        name,
        K::String(crate::gen::schemapb::schema::field::String {
            format: Some(format.to_owned()),
            ..Default::default()
        }),
    )
}

impl ReflectField for IpAddr {
    fn field(name: &str) -> Field {
        formatted(name, "ip")
    }
}

impl ReflectField for Ipv4Addr {
    fn field(name: &str) -> Field {
        formatted(name, "ipv4")
    }
}

impl ReflectField for Ipv6Addr {
    fn field(name: &str) -> Field {
        formatted(name, "ipv6")
    }
}

impl ReflectField for serde_json::Value {
    fn field(name: &str) -> Field {
        json_field(name)
    }
}

impl ReflectField for crate::values::Values {
    fn field(name: &str) -> Field {
        json_field(name)
    }
}

/// Transparent wrappers: the wrapped type's shape.
macro_rules! transparent {
    ($($wrapper:ident),*) => {$(
        impl<T: ReflectField + ?Sized> ReflectField for $wrapper<T> {
            const REQUIRED: bool = T::REQUIRED;
            const SECTION: bool = T::SECTION;

            fn field(name: &str) -> Field {
                T::field(name)
            }

            fn object_fields() -> Option<Vec<Field>> {
                T::object_fields()
            }

            fn decorate_schema(schema: &mut Schema) {
                T::decorate_schema(schema);
            }
        }
    )*};
}

transparent!(Box, Rc, Arc);

impl<T: ReflectField + ?Sized> ReflectField for &T {
    const REQUIRED: bool = T::REQUIRED;
    const SECTION: bool = T::SECTION;

    fn field(name: &str) -> Field {
        T::field(name)
    }

    fn object_fields() -> Option<Vec<Field>> {
        T::object_fields()
    }

    fn decorate_schema(schema: &mut Schema) {
        T::decorate_schema(schema);
    }
}

impl<T: ReflectField + ToOwned + ?Sized> ReflectField for Cow<'_, T> {
    const REQUIRED: bool = T::REQUIRED;
    const SECTION: bool = T::SECTION;

    fn field(name: &str) -> Field {
        T::field(name)
    }

    fn object_fields() -> Option<Vec<Field>> {
        T::object_fields()
    }

    fn decorate_schema(schema: &mut Schema) {
        T::decorate_schema(schema);
    }
}

/// `Option<T>`: optional and nullable.
macro_rules! optional {
    () => {
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

            fn decorate_schema(schema: &mut Schema) {
                T::decorate_schema(schema);
            }
        }
    };
}

optional!();

fn item_of<T: ReflectField>() -> Field {
    let mut item = T::field("item");
    item.required = true;
    item
}

/// Bytes is the schemapb wire form of a byte sequence: a `Vec<u8>` or a
/// `[u8; N]` is Bytes, not a list of numbers.
fn is_byte<T: 'static>() -> bool {
    TypeId::of::<T>() == TypeId::of::<u8>()
}

fn list_field<T: ReflectField + 'static>(name: &str, unique: bool) -> Field {
    let item = visiting(TypeId::of::<T>(), item_of::<T>).unwrap_or_else(|| json_field("item"));
    leaf(
        name,
        K::List(ListKind {
            items: vec![item],
            unique,
            ..Default::default()
        }),
    )
}

/// `Vec<u8>` is Bytes; implement [`ReflectField`] for a newtype if your
/// serde encoding is a number array instead.
impl<T: ReflectField + 'static> ReflectField for Vec<T> {
    fn field(name: &str) -> Field {
        if is_byte::<T>() {
            return leaf(name, K::Bytes(<BytesKind as Default>::default()));
        }
        list_field::<T>(name, false)
    }
}

impl ReflectField for [u8] {
    fn field(name: &str) -> Field {
        leaf(name, K::Bytes(<BytesKind as Default>::default()))
    }
}

impl<T: ReflectField + 'static> ReflectField for VecDeque<T> {
    fn field(name: &str) -> Field {
        list_field::<T>(name, false)
    }
}

impl<T: ReflectField + 'static> ReflectField for HashSet<T> {
    fn field(name: &str) -> Field {
        list_field::<T>(name, true)
    }
}

impl<T: ReflectField + 'static> ReflectField for BTreeSet<T> {
    fn field(name: &str) -> Field {
        list_field::<T>(name, true)
    }
}

/// `[u8; N]` is Bytes of length N; other arrays are lists of exactly N.
impl<T: ReflectField + 'static, const N: usize> ReflectField for [T; N] {
    fn field(name: &str) -> Field {
        if is_byte::<T>() {
            return leaf(
                name,
                K::Bytes(BytesKind {
                    len: Some(N as u64),
                    ..Default::default()
                }),
            );
        }
        let mut f = list_field::<T>(name, false);
        if let Some(K::List(l)) = f.kind.as_mut() {
            l.min_items = Some(N as u64);
            l.max_items = Some(N as u64);
        }
        f
    }
}

/// Tuples are positional lists (serde writes them as sequences).
macro_rules! tuple_impl {
    ($($t:ident),+) => {
        impl<$($t: ReflectField + 'static),+> ReflectField for ($($t,)+) {
            fn field(name: &str) -> Field {
                leaf(
                    name,
                    K::List(ListKind {
                        items: vec![$(
                            visiting(TypeId::of::<$t>(), item_of::<$t>)
                                .unwrap_or_else(|| json_field("item"))
                        ),+],
                        ..Default::default()
                    }),
                )
            }
        }
    };
}

tuple_impl!(A, B);
tuple_impl!(A, B, C);
tuple_impl!(A, B, C, D);

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
            |fields| {
                let mut value = Schema {
                    fields,
                    ..Default::default()
                };
                T::decorate_schema(&mut value);
                MapKind {
                    value_schema: Some(value),
                    ..Default::default()
                }
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
            default: None,
            schema: Some(Schema {
                fields,
                ..Default::default()
            }),
        }),
    )
}

/// A derived struct's Object field with its schema-level attributes.
#[must_use]
pub fn object_field_of<T: ReflectField + ?Sized>(name: &str, fields: Vec<Field>) -> Field {
    let mut f = object_field(name, fields);
    if let Some(K::Object(ObjectKind {
        schema: Some(s), ..
    })) = f.kind.as_mut()
    {
        T::decorate_schema(s);
    }
    f
}

#[cfg(feature = "uuid")]
impl ReflectField for uuid::Uuid {
    fn field(name: &str) -> Field {
        formatted(name, "uuid")
    }
}

#[cfg(feature = "url")]
impl ReflectField for url::Url {
    fn field(name: &str) -> Field {
        formatted(name, "url")
    }
}

impl<Tz: chrono::TimeZone> ReflectField for chrono::DateTime<Tz> {
    fn field(name: &str) -> Field {
        leaf(
            name,
            K::Timestamp(<crate::gen::schemapb::schema::field::Timestamp as Default>::default()),
        )
    }
}

impl ReflectField for chrono::NaiveDate {
    fn field(name: &str) -> Field {
        formatted(name, "date")
    }
}

impl ReflectField for chrono::NaiveTime {
    fn field(name: &str) -> Field {
        formatted(name, "time")
    }
}

// =============================================================================
// Typed attributes: the functions the derive expansion calls. Each is bound
// by a trait the field's type must implement, so an attribute that does not
// apply (a bound on a string, a pattern on a number) is a COMPILE error, and
// the literal is typed by the field (300 for a u8 does not compile).
// =============================================================================

pub mod attr {
    use super::{BTreeMap, BTreeSet, Cow, HashMap, HashSet, VecDeque};
    use super::{Field, ReflectField, K};
    use super::{
        IpAddr, Ipv4Addr, Ipv6Addr, NonZeroI16, NonZeroI32, NonZeroI64, NonZeroI8, NonZeroIsize,
        NonZeroU16, NonZeroU32, NonZeroU64, NonZeroU8, NonZeroUsize, PathBuf, StdDuration,
        SystemTime,
    };
    use crate::gen::schemapb::schema::field::Rule;
    use crate::value::{from_native, Native};
    use crate::values::IntoNative;
    use std::rc::Rc;
    use std::sync::Arc;

    /// Which comparison a bound sets.
    #[derive(Debug, Clone, Copy)]
    pub enum Which {
        Gt,
        Gte,
        Lt,
        Lte,
    }

    /// A numeric bound in its widest exact form.
    #[derive(Debug, Clone, Copy)]
    pub enum Num {
        I(i64),
        U(u64),
        F(f64),
    }

    fn kind_mut(f: &mut Field) -> &mut K {
        f.kind
            .as_mut()
            .unwrap_or_else(|| unreachable!("reflected fields always have a kind"))
    }

    #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    pub(super) fn set_bound(f: &mut Field, which: Which, v: Num) {
        macro_rules! put {
            ($k:expr, $conv:expr) => {{
                let k = $k;
                let v = Some($conv);
                match which {
                    Which::Gt => k.gt = v,
                    Which::Gte => k.gte = v,
                    Which::Lt => k.lt = v,
                    Which::Lte => k.lte = v,
                }
            }};
        }
        let int = || match v {
            Num::I(n) => n,
            Num::U(n) => i64::try_from(n).unwrap_or(i64::MAX),
            Num::F(n) => n as i64,
        };
        let uint = || match v {
            Num::U(n) => n,
            Num::I(n) => u64::try_from(n).unwrap_or(0),
            Num::F(n) => n as u64,
        };
        #[allow(clippy::cast_precision_loss)]
        let float = || match v {
            Num::F(n) => n,
            Num::I(n) => n as f64,
            Num::U(n) => n as f64,
        };
        match kind_mut(f) {
            K::Int32(k) => put!(k, i32::try_from(int()).unwrap_or(i32::MAX)),
            K::Int64(k) => put!(k, int()),
            K::Uint32(k) => put!(k, u32::try_from(uint()).unwrap_or(u32::MAX)),
            K::Uint64(k) => put!(k, uint()),
            K::Float(k) => put!(k, float() as f32),
            K::Double(k) => put!(k, float()),
            _ => unreachable!("numeric bound on a non-numeric kind"),
        }
    }

    pub(super) fn forbid_zero(f: &mut Field) {
        match kind_mut(f) {
            K::Int32(k) => k.not_in = vec![0],
            K::Int64(k) => k.not_in = vec![0],
            _ => unreachable!("signed NonZero reflects as a signed integer"),
        }
    }

    // ---------------------------------------------------------------- traits

    /// Types with ordered values: numbers, durations, timestamps.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` has no order: gt, gte, lt and lte do not apply",
        label = "not a number, duration or timestamp"
    )]
    pub trait Ordered: ReflectField {
        /// The bound's Rust type (the field's own value type).
        type Bound;
        fn bound(f: &mut Field, which: Which, v: Self::Bound);
    }

    /// Types with a length: strings, bytes, lists, sets, maps.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` has no length: len does not apply",
        label = "not a string, bytes, list or map"
    )]
    pub trait Length: ReflectField {}

    /// Types whose `min`/`max` the validator crate gives a meaning to: a
    /// bound for an ordered type, a length otherwise.
    #[diagnostic::on_unimplemented(
        message = "schemapb: min and max do not apply to `{Self}`",
        label = "neither ordered nor sized"
    )]
    pub trait MinMax: ReflectField {
        type Arg;
        fn min(f: &mut Field, v: Self::Arg);
        fn max(f: &mut Field, v: Self::Arg);
    }

    /// String-shaped types: pattern and format apply.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` is not text: pattern, format and string options do not apply",
        label = "not a string type"
    )]
    pub trait Text: ReflectField {}

    /// Integer types: integer options apply.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` is not an integer: integer options do not apply",
        label = "not an integer type"
    )]
    pub trait Integer: ReflectField {}

    /// Float types: `finite` applies.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` is not a float: finite does not apply",
        label = "not f32 or f64"
    )]
    pub trait Float: ReflectField {}

    /// Types with a schema default of a Rust value.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` cannot have a schema default",
        label = "no default for this type"
    )]
    pub trait HasDefault: ReflectField {
        type Arg;
        fn default(f: &mut Field, v: Self::Arg);
    }

    /// List-shaped types: `items(...)` and `unique` apply.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` is not a list: items(...) and unique do not apply",
        label = "not a list or set"
    )]
    pub trait Items: ReflectField {
        type Item: ReflectField;
    }

    /// Map types: `keys(...)` and `values(...)` apply.
    #[diagnostic::on_unimplemented(
        message = "schemapb: `{Self}` is not a map: keys(...) and values(...) do not apply",
        label = "not a string-keyed map"
    )]
    pub trait Entries: ReflectField {
        type Value: ReflectField;
    }

    // ------------------------------------------------------- entry points

    pub fn gt<T: Ordered + ?Sized>(f: &mut Field, v: T::Bound) {
        T::bound(f, Which::Gt, v);
    }

    pub fn gte<T: Ordered + ?Sized>(f: &mut Field, v: T::Bound) {
        T::bound(f, Which::Gte, v);
    }

    pub fn lt<T: Ordered + ?Sized>(f: &mut Field, v: T::Bound) {
        T::bound(f, Which::Lt, v);
    }

    pub fn lte<T: Ordered + ?Sized>(f: &mut Field, v: T::Bound) {
        T::bound(f, Which::Lte, v);
    }

    pub fn min<T: MinMax + ?Sized>(f: &mut Field, v: T::Arg) {
        T::min(f, v);
    }

    pub fn max<T: MinMax + ?Sized>(f: &mut Field, v: T::Arg) {
        T::max(f, v);
    }

    /// Exact length (code points, bytes, items or entries).
    pub fn len<T: Length + ?Sized>(f: &mut Field, n: u64) {
        set_len(f, Some(n), Some(n), true);
    }

    pub fn pattern<T: Text + ?Sized>(f: &mut Field, re: &str) {
        if let K::String(k) = kind_mut(f) {
            k.pattern = Some(re.to_owned());
        }
    }

    pub fn format<T: Text + ?Sized>(f: &mut Field, name: &str) {
        if let K::String(k) = kind_mut(f) {
            k.format = Some(name.to_owned());
        }
    }

    pub fn finite<T: Float + ?Sized>(f: &mut Field) {
        match kind_mut(f) {
            K::Float(k) => k.finite = true,
            K::Double(k) => k.finite = true,
            _ => {}
        }
    }

    pub fn default<T: HasDefault + ?Sized>(f: &mut Field, v: T::Arg) {
        T::default(f, v);
    }

    pub fn unique<T: Items + ?Sized>(f: &mut Field) {
        if let K::List(k) = kind_mut(f) {
            k.unique = true;
        }
    }

    /// Constraints on every item (validator's `dive`).
    pub fn items<T: Items + ?Sized>(f: &mut Field, apply: impl FnOnce(&mut Field)) {
        let name = f.name.clone();
        match kind_mut(f) {
            K::List(k) => {
                if let Some(item) = k.items.first_mut() {
                    apply(item);
                }
            }
            _ => panic!("schemapb: field {name:?}: items(...) on bytes; use min/max/len"),
        }
    }

    /// Constraints on every key (validator's `keys`).
    pub fn keys<T: Entries + ?Sized>(f: &mut Field, apply: impl FnOnce(&mut Field)) {
        if let K::Map(k) = kind_mut(f) {
            let mut key = String::field("key");
            apply(&mut key);
            k.key_field = Some(Box::new(key));
        }
    }

    /// Constraints on every value of a map whose values are not structs
    /// (constrain a struct value's own fields instead).
    pub fn values<T: Entries + ?Sized>(f: &mut Field, apply: impl FnOnce(&mut Field)) {
        if let K::Map(k) = kind_mut(f) {
            if let Some(value) = k.value_field.as_mut() {
                apply(value);
            } else {
                panic!(
                    "schemapb: field {:?}: values(...) on a map of structs; constrain the struct's fields",
                    f.name
                );
            }
        }
    }

    /// Replaces the field with a string Choice.
    pub fn choice_str<T: Text + ?Sized>(f: &mut Field, options: &[&str]) {
        choice(f, options.iter().map(|o| Native::Str((*o).to_owned())));
    }

    /// Replaces the field with an integer Choice.
    pub fn choice_int<T: Integer + ?Sized>(f: &mut Field, options: &[i64]) {
        choice(f, options.iter().map(|o| Native::Int(*o)));
    }

    fn choice(f: &mut Field, options: impl Iterator<Item = Native>) {
        use crate::gen::schemapb::schema::field::{choice::Option as ChoiceOption, Choice};
        f.kind = Some(K::Choice(Choice {
            options: options
                .map(|v| ChoiceOption {
                    value: Some(from_native(&v)),
                    ..Default::default()
                })
                .collect(),
            ..Default::default()
        }));
    }

    pub fn rule(f: &mut Field, expr: &str, message: &str, id: Option<&str>, warn: bool) {
        f.rules.push(rule_of(expr, message, id, warn));
    }

    #[must_use]
    pub fn rule_of(expr: &str, message: &str, id: Option<&str>, warn: bool) -> Rule {
        use crate::gen::schemapb::schema::field::Severity;
        Rule {
            expr: expr.to_owned(),
            message: message.to_owned(),
            id: id.map(str::to_owned),
            severity: warn.then_some(Severity::Warning as i32),
        }
    }

    pub fn example(f: &mut Field, v: impl IntoNative) {
        f.examples.push(from_native(&v.into_native()));
    }

    pub fn annotation(f: &mut Field, key: &str, v: impl IntoNative) {
        f.annotations
            .insert(key.to_owned(), from_native(&v.into_native()));
    }

    /// Presence of a struct field without `#[schemapb(required)]`.
    ///
    /// The type's own [`ReflectField::REQUIRED`], except that a section (a
    /// derived struct that reflected as an Object, not a cycle) is optional
    /// with an implicit empty object default, and a bool is optional with
    /// default false.
    pub fn presence<T: ReflectField + ?Sized>(f: &mut Field) {
        f.required = T::REQUIRED;
        // A bool without a default is false when absent (Go: a bool without
        // omitempty).
        if let Some(K::Bool(k)) = f.kind.as_mut() {
            if T::REQUIRED && !f.nullable && k.default.is_none() {
                k.default = Some(false);
                f.required = false;
            }
            return;
        }
        if !T::SECTION || !T::REQUIRED || f.nullable {
            return;
        }
        if let Some(K::Object(o)) = f.kind.as_mut() {
            o.default = Some(crate::gen::schemapb::StructValue::default());
            f.required = false;
        }
    }

    fn set_len(f: &mut Field, min: Option<u64>, max: Option<u64>, exact: bool) {
        match kind_mut(f) {
            K::String(k) if exact => k.len = min,
            K::Bytes(k) if exact => k.len = min,
            K::String(k) => {
                k.min_len = min.or(k.min_len);
                k.max_len = max.or(k.max_len);
            }
            K::Bytes(k) => {
                k.min_len = min.or(k.min_len);
                k.max_len = max.or(k.max_len);
            }
            K::List(k) => {
                k.min_items = min.or(k.min_items);
                k.max_items = max.or(k.max_items);
            }
            K::Map(k) => {
                k.min_entries = min.or(k.min_entries);
                k.max_entries = max.or(k.max_entries);
            }
            _ => unreachable!("length on a kind without one"),
        }
    }

    // ------------------------------------------------------------- impls

    macro_rules! ordered_num {
        ($($ty:ty => $num:ident as $wide:ty),* $(,)?) => {$(
            impl Ordered for $ty {
                type Bound = $ty;
                fn bound(f: &mut Field, which: Which, v: $ty) {
                    set_bound(f, which, Num::$num(<$wide>::from(v)));
                }
            }

            impl MinMax for $ty {
                type Arg = $ty;
                fn min(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Gte, v);
                }
                fn max(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Lte, v);
                }
            }

            impl HasDefault for $ty {
                type Arg = $ty;
                fn default(f: &mut Field, v: $ty) {
                    set_default(f, &v.into_native());
                }
            }
        )*};
    }

    ordered_num!(
        i8 => I as i64, i16 => I as i64, i32 => I as i64, i64 => I as i64,
        u8 => U as u64, u16 => U as u64, u32 => U as u64, u64 => U as u64,
        f32 => F as f64, f64 => F as f64,
    );

    /// `isize`/`usize` bounds widen without loss on every supported target.
    macro_rules! ordered_size {
        ($($ty:ty => $num:ident / $native:ident as $wide:ty),*) => {$(
            impl Ordered for $ty {
                type Bound = $ty;
                #[allow(clippy::cast_possible_wrap, clippy::cast_sign_loss)]
                fn bound(f: &mut Field, which: Which, v: $ty) {
                    set_bound(f, which, Num::$num(v as $wide));
                }
            }

            impl MinMax for $ty {
                type Arg = $ty;
                fn min(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Gte, v);
                }
                fn max(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Lte, v);
                }
            }

            impl HasDefault for $ty {
                type Arg = $ty;
                #[allow(clippy::cast_possible_wrap, clippy::cast_sign_loss)]
                fn default(f: &mut Field, v: $ty) {
                    set_default(f, &Native::$native(v as $wide));
                }
            }
        )*};
    }

    ordered_size!(isize => I / Int as i64, usize => U / Uint as u64);

    macro_rules! integer {
        ($($ty:ty),*) => {$( impl Integer for $ty {} )*};
    }

    integer!(i8, i16, i32, i64, isize, u8, u16, u32, u64, usize);
    integer!(
        NonZeroI8,
        NonZeroI16,
        NonZeroI32,
        NonZeroI64,
        NonZeroIsize,
        NonZeroU8,
        NonZeroU16,
        NonZeroU32,
        NonZeroU64,
        NonZeroUsize
    );

    impl Float for f32 {}
    impl Float for f64 {}

    /// `NonZero*` bounds are written with the primitive (`gte = 5`).
    macro_rules! ordered_nonzero {
        ($($nz:ty => $prim:ty),*) => {$(
            impl Ordered for $nz {
                type Bound = $prim;
                fn bound(f: &mut Field, which: Which, v: $prim) {
                    <$prim as Ordered>::bound(f, which, v);
                }
            }

            impl MinMax for $nz {
                type Arg = $prim;
                fn min(f: &mut Field, v: $prim) {
                    <$prim as MinMax>::min(f, v);
                }
                fn max(f: &mut Field, v: $prim) {
                    <$prim as MinMax>::max(f, v);
                }
            }

            impl HasDefault for $nz {
                type Arg = $nz;
                fn default(f: &mut Field, v: $nz) {
                    <$prim as HasDefault>::default(f, v.get());
                }
            }
        )*};
    }

    ordered_nonzero!(
        NonZeroI8 => i8, NonZeroI16 => i16, NonZeroI32 => i32, NonZeroI64 => i64,
        NonZeroIsize => isize, NonZeroU8 => u8, NonZeroU16 => u16, NonZeroU32 => u32,
        NonZeroU64 => u64, NonZeroUsize => usize
    );

    fn set_default(f: &mut Field, v: &Native) {
        use crate::value::canonical_value;
        let wire = canonical_value(f, v).unwrap_or_else(|_| from_native(v));
        let kind = wire.kind;
        match (kind_mut(f), kind) {
            (K::Int32(k), Some(crate::gen::schemapb::value::Kind::Int32Value(n))) => {
                k.default = Some(n);
            }
            (K::Int64(k), Some(crate::gen::schemapb::value::Kind::Int64Value(n))) => {
                k.default = Some(n);
            }
            (K::Uint32(k), Some(crate::gen::schemapb::value::Kind::Uint32Value(n))) => {
                k.default = Some(n);
            }
            (K::Uint64(k), Some(crate::gen::schemapb::value::Kind::Uint64Value(n))) => {
                k.default = Some(n);
            }
            (K::Float(k), Some(crate::gen::schemapb::value::Kind::FloatValue(n))) => {
                k.default = Some(n);
            }
            (K::Double(k), Some(crate::gen::schemapb::value::Kind::DoubleValue(n))) => {
                k.default = Some(n);
            }
            (K::Bool(k), Some(crate::gen::schemapb::value::Kind::BoolValue(b))) => {
                k.default = Some(b);
            }
            (K::String(k), Some(crate::gen::schemapb::value::Kind::StringValue(s))) => {
                k.default = Some(s);
            }
            (K::Bytes(k), Some(crate::gen::schemapb::value::Kind::BytesValue(b))) => {
                k.default = Some(b);
            }
            (K::Duration(k), Some(crate::gen::schemapb::value::Kind::DurationValue(d))) => {
                k.default = Some(d);
            }
            (K::Timestamp(k), Some(crate::gen::schemapb::value::Kind::TimestampValue(t))) => {
                k.default = Some(t);
            }
            (K::Choice(k), Some(value)) => {
                k.default = Some(crate::gen::schemapb::Value { kind: Some(value) });
            }
            (K::Json(k), Some(value)) => {
                k.default = Some(crate::gen::schemapb::Value { kind: Some(value) });
            }
            _ => unreachable!("a default of the field's own type"),
        }
    }

    /// The default of a derived enum: its variant's wire name.
    pub fn default_variant(f: &mut Field, name: &str) {
        set_default(f, &Native::Str(name.to_owned()));
    }

    impl HasDefault for bool {
        type Arg = Self;
        fn default(f: &mut Field, v: Self) {
            set_default(f, &Native::Bool(v));
        }
    }

    impl HasDefault for char {
        type Arg = Self;
        fn default(f: &mut Field, v: Self) {
            set_default(f, &Native::Str(v.to_string()));
        }
    }

    macro_rules! text {
        ($($ty:ty),*) => {$(
            impl Text for $ty {}
            impl Length for $ty {}

            impl MinMax for $ty {
                type Arg = u64;
                fn min(f: &mut Field, n: u64) {
                    set_len(f, Some(n), None, false);
                }
                fn max(f: &mut Field, n: u64) {
                    set_len(f, None, Some(n), false);
                }
            }

            impl HasDefault for $ty {
                type Arg = &'static str;
                fn default(f: &mut Field, v: &'static str) {
                    set_default(f, &Native::Str(v.to_owned()));
                }
            }
        )*};
    }

    text!(
        String,
        str,
        PathBuf,
        crate::types::Email,
        crate::types::Hostname
    );

    impl Text for char {}

    macro_rules! formatted_text {
        ($($ty:ty),*) => {$(
            impl Text for $ty {}
            impl HasDefault for $ty {
                type Arg = &'static str;
                fn default(f: &mut Field, v: &'static str) {
                    set_default(f, &Native::Str(v.to_owned()));
                }
            }
        )*};
    }

    formatted_text!(IpAddr, Ipv4Addr, Ipv6Addr);

    macro_rules! sized {
        ($($ty:ty),*) => {$(
            impl Length for $ty {}
            impl MinMax for $ty {
                type Arg = u64;
                fn min(f: &mut Field, n: u64) {
                    set_len(f, Some(n), None, false);
                }
                fn max(f: &mut Field, n: u64) {
                    set_len(f, None, Some(n), false);
                }
            }
        )*};
    }

    sized!([u8]);

    impl HasDefault for Vec<u8> {
        type Arg = &'static [u8];
        fn default(f: &mut Field, v: &'static [u8]) {
            set_default(f, &Native::Bytes(v.to_vec()));
        }
    }

    macro_rules! list_like {
        ($($ty:ident),*) => {$(
            impl<T: ReflectField + 'static> Items for $ty<T> where $ty<T>: ReflectField {
                type Item = T;
            }
            impl<T: ReflectField + 'static> Length for $ty<T> where $ty<T>: ReflectField {}
            impl<T: ReflectField + 'static> MinMax for $ty<T> where $ty<T>: ReflectField {
                type Arg = u64;
                fn min(f: &mut Field, n: u64) {
                    set_len(f, Some(n), None, false);
                }
                fn max(f: &mut Field, n: u64) {
                    set_len(f, None, Some(n), false);
                }
            }
        )*};
    }

    list_like!(Vec, VecDeque, HashSet, BTreeSet);

    macro_rules! map_like {
        ($($ty:ident),*) => {$(
            impl<V: ReflectField + 'static> Entries for $ty<String, V> {
                type Value = V;
            }
            impl<V: ReflectField + 'static> Length for $ty<String, V> {}
            impl<V: ReflectField + 'static> MinMax for $ty<String, V> {
                type Arg = u64;
                fn min(f: &mut Field, n: u64) {
                    set_len(f, Some(n), None, false);
                }
                fn max(f: &mut Field, n: u64) {
                    set_len(f, None, Some(n), false);
                }
            }
        )*};
    }

    map_like!(HashMap, BTreeMap);

    fn duration_of(v: impl IntoNative) -> pbjson_types::Duration {
        match v.into_native() {
            Native::Duration(d) => d,
            _ => unreachable!("a duration converts to a duration"),
        }
    }

    fn timestamp_of(v: impl IntoNative) -> pbjson_types::Timestamp {
        match v.into_native() {
            Native::Timestamp(t) => t,
            _ => unreachable!("a timestamp converts to a timestamp"),
        }
    }

    macro_rules! ordered_time {
        ($($ty:ty => $variant:ident via $of:ident),*) => {$(
            impl Ordered for $ty {
                type Bound = $ty;
                fn bound(f: &mut Field, which: Which, v: $ty) {
                    if let K::$variant(k) = kind_mut(f) {
                        let v = Some($of(v));
                        match which {
                            Which::Gt => k.gt = v,
                            Which::Gte => k.gte = v,
                            Which::Lt => k.lt = v,
                            Which::Lte => k.lte = v,
                        }
                    }
                }
            }

            impl MinMax for $ty {
                type Arg = $ty;
                fn min(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Gte, v);
                }
                fn max(f: &mut Field, v: $ty) {
                    Self::bound(f, Which::Lte, v);
                }
            }

            impl HasDefault for $ty {
                type Arg = $ty;
                fn default(f: &mut Field, v: $ty) {
                    set_default(f, &v.into_native());
                }
            }
        )*};
    }

    ordered_time!(
        StdDuration => Duration via duration_of,
        pbjson_types::Duration => Duration via duration_of,
        SystemTime => Timestamp via timestamp_of,
        pbjson_types::Timestamp => Timestamp via timestamp_of
    );

    /// Wrappers forward every attribute trait to the wrapped type.
    macro_rules! forward {
        ($($wrapper:ident),*) => {$(
            impl<T: Ordered> Ordered for $wrapper<T> where $wrapper<T>: ReflectField {
                type Bound = T::Bound;
                fn bound(f: &mut Field, which: Which, v: T::Bound) {
                    T::bound(f, which, v);
                }
            }
            impl<T: MinMax> MinMax for $wrapper<T> where $wrapper<T>: ReflectField {
                type Arg = T::Arg;
                fn min(f: &mut Field, v: T::Arg) {
                    T::min(f, v);
                }
                fn max(f: &mut Field, v: T::Arg) {
                    T::max(f, v);
                }
            }
            impl<T: Length> Length for $wrapper<T> where $wrapper<T>: ReflectField {}
            impl<T: Text> Text for $wrapper<T> where $wrapper<T>: ReflectField {}
            impl<T: Integer> Integer for $wrapper<T> where $wrapper<T>: ReflectField {}
            impl<T: Float> Float for $wrapper<T> where $wrapper<T>: ReflectField {}
            impl<T: HasDefault> HasDefault for $wrapper<T> where $wrapper<T>: ReflectField {
                type Arg = T::Arg;
                fn default(f: &mut Field, v: T::Arg) {
                    T::default(f, v);
                }
            }
            impl<T: Items> Items for $wrapper<T> where $wrapper<T>: ReflectField {
                type Item = T::Item;
            }
            impl<T: Entries> Entries for $wrapper<T> where $wrapper<T>: ReflectField {
                type Value = T::Value;
            }
        )*};
    }

    forward!(Option, Box, Rc, Arc);

    macro_rules! forward_cow {
        ($($tr:ident),*) => {$(
            impl<T: $tr + ToOwned + ?Sized> $tr for Cow<'_, T> {}
        )*};
    }

    forward_cow!(Length, Text, Integer, Float);

    impl<T: MinMax + ToOwned + ?Sized> MinMax for Cow<'_, T> {
        type Arg = T::Arg;
        fn min(f: &mut Field, v: T::Arg) {
            T::min(f, v);
        }
        fn max(f: &mut Field, v: T::Arg) {
            T::max(f, v);
        }
    }

    impl<T: HasDefault + ToOwned + ?Sized> HasDefault for Cow<'_, T> {
        type Arg = T::Arg;
        fn default(f: &mut Field, v: T::Arg) {
            T::default(f, v);
        }
    }

    #[cfg(feature = "uuid")]
    impl Text for uuid::Uuid {}

    #[cfg(feature = "url")]
    impl Text for url::Url {}

    impl Text for chrono::NaiveDate {}

    impl Text for chrono::NaiveTime {}
}
