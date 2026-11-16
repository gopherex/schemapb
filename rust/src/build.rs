//! Schemas built in code, without proto: the Rust form of the builders the
//! Go, TypeScript and Python ports have (same vocabulary, Rust names).
//!
//! ```
//! use schemapb::build::*;
//!
//! let tracker = schema("tracker", "project-42")
//!     .field(int64("estimate").gte(0).unit("h"))
//!     .field(choice("status").str_opts(["open", "done"]).default("open"))
//!     .field(list("labels", string("label").max_len(32)).unique())
//!     .rule(rule("root.estimate < 1000", "estimate too large").id("estimate"))
//!     .build()
//!     .unwrap();
//! ```
//!
//! A schema built here coerces by default: string inputs become the
//! declared kind, so [`crate::Values`] read back from plain JSON (durations,
//! timestamps and bytes are strings there) resolve to their kinds.

use std::collections::HashMap;
use std::time::{Duration as StdDuration, SystemTime};

use crate::descriptor::SchemaError;
use crate::gen::schemapb::schema::field::{
    choice::Option as ChoiceOption, r#ref::Target, Bool as BoolKind, Bytes as BytesKind, Choice,
    Computed, Double, Duration as DurationKind, Float, Int32, Int64, Json, Kind, List, Map, Object,
    OneOf, Ref, Rule as PbRule, Severity, String as StringKind, Timestamp as TimestampKind, UInt32,
    UInt64,
};
use crate::gen::schemapb::schema::Field;
use crate::gen::schemapb::{Schema, SchemaIdentity, StructValue};
use crate::typed::Version;
use crate::validator::Validator;
use crate::value::{from_native, Native};
use crate::values::IntoNative;

/// The declared result kind of a [`computed`] field.
pub use crate::gen::schemapb::schema::field::ResultType;

// =============================================================================
// Rules
// =============================================================================

/// A CEL rule: `expr` evaluates to bool, true means valid.
#[derive(Debug, Clone)]
pub struct RuleB(PbRule);

/// A CEL rule over `this` (a field rule) or `root` (a schema rule).
#[must_use]
pub fn rule(expr: &str, message: &str) -> RuleB {
    RuleB(PbRule {
        expr: expr.to_owned(),
        message: message.to_owned(),
        ..Default::default()
    })
}

impl RuleB {
    /// A stable id: the translation key and the violation's `rule_id`.
    #[must_use]
    pub fn id(mut self, id: &str) -> Self {
        self.0.id = Some(id.to_owned());
        self
    }

    /// Reported without blocking.
    #[must_use]
    pub const fn warn(mut self) -> Self {
        self.0.severity = Some(Severity::Warning as i32);
        self
    }

    /// Blocks, stated explicitly (an unset severity blocks too).
    #[must_use]
    pub const fn error(mut self) -> Self {
        self.0.severity = Some(Severity::Error as i32);
        self
    }
}

impl From<RuleB> for PbRule {
    fn from(r: RuleB) -> Self {
        r.0
    }
}

// =============================================================================
// Field attributes every kind shares
// =============================================================================

/// The attributes every field builder has. Kind-specific constraints are
/// methods of each builder.
pub trait FieldDef: Into<Field> + Sized {
    #[doc(hidden)]
    fn field_mut(&mut self) -> &mut Field;

    /// The key must be present.
    #[must_use]
    fn required(mut self) -> Self {
        self.field_mut().required = true;
        self
    }

    /// An explicit null is accepted.
    #[must_use]
    fn nullable(mut self) -> Self {
        self.field_mut().nullable = true;
        self
    }

    /// The value cannot differ from the default.
    #[must_use]
    fn immutable(mut self) -> Self {
        self.field_mut().immutable = true;
        self
    }

    /// Masked in displays and absent from violations.
    #[must_use]
    fn secret(mut self) -> Self {
        self.field_mut().secret = true;
        self
    }

    #[must_use]
    fn deprecated(mut self) -> Self {
        self.field_mut().deprecated = true;
        self
    }

    #[must_use]
    fn title(mut self, t: &str) -> Self {
        self.field_mut().title = Some(t.to_owned());
        self
    }

    #[must_use]
    fn desc(mut self, d: &str) -> Self {
        self.field_mut().description = Some(d.to_owned());
        self
    }

    #[must_use]
    fn group(mut self, g: &str) -> Self {
        self.field_mut().group = Some(g.to_owned());
        self
    }

    #[must_use]
    fn unit(mut self, u: &str) -> Self {
        self.field_mut().unit = Some(u.to_owned());
        self
    }

    #[must_use]
    fn example(mut self, v: impl IntoNative) -> Self {
        self.field_mut()
            .examples
            .push(from_native(&v.into_native()));
        self
    }

    /// Application metadata the engine carries but never interprets; use
    /// namespaced keys (`tracker.column`).
    #[must_use]
    fn annotation(mut self, key: &str, v: impl IntoNative) -> Self {
        self.field_mut()
            .annotations
            .insert(key.to_owned(), from_native(&v.into_native()));
        self
    }

    /// A CEL rule over `this` (the value) and `root`.
    #[must_use]
    fn rule(mut self, r: RuleB) -> Self {
        self.field_mut().rules.push(r.0);
        self
    }

    /// A CEL expression over `this` replacing the value before checks.
    #[must_use]
    fn normalize(mut self, expr: &str) -> Self {
        self.field_mut().normalize = Some(expr.to_owned());
        self
    }

    /// A CEL gate over `root`: when false the field is inactive (skipped).
    #[must_use]
    fn when(mut self, expr: &str) -> Self {
        self.field_mut().when = Some(expr.to_owned());
        self
    }
}

fn named(name: &str) -> Field {
    Field {
        name: name.to_owned(),
        ..Default::default()
    }
}

/// A builder that keeps the kind message apart from the field and joins
/// them on conversion, so kind methods need no match on `Field::kind`.
macro_rules! kind_builder {
    ($(#[$doc:meta])* $b:ident, $kind:ty, $variant:ident) => {
        $(#[$doc])*
        #[derive(Debug, Clone)]
        pub struct $b {
            f: Field,
            k: $kind,
        }

        impl FieldDef for $b {
            fn field_mut(&mut self) -> &mut Field {
                &mut self.f
            }
        }

        impl From<$b> for Field {
            fn from(b: $b) -> Self {
                let mut f = b.f;
                f.kind = Some(Kind::$variant(b.k.into()));
                f
            }
        }
    };
}

// =============================================================================
// Numbers
// =============================================================================

macro_rules! number {
    ($(#[$doc:meta])* $fn:ident, $b:ident, $kind:ty, $variant:ident, $ty:ty) => {
        kind_builder!($(#[$doc])* $b, $kind, $variant);

        $(#[$doc])*
        #[must_use]
        pub fn $fn(name: &str) -> $b {
            $b { f: named(name), k: <$kind as Default>::default() }
        }

        impl $b {
            #[must_use]
            pub const fn default(mut self, v: $ty) -> Self {
                self.k.default = Some(v);
                self
            }

            /// The value must equal `v`.
            #[must_use]
            pub const fn equals(mut self, v: $ty) -> Self {
                self.k.r#const = Some(v);
                self
            }

            #[must_use]
            pub const fn gt(mut self, v: $ty) -> Self {
                self.k.gt = Some(v);
                self
            }

            #[must_use]
            pub const fn gte(mut self, v: $ty) -> Self {
                self.k.gte = Some(v);
                self
            }

            #[must_use]
            pub const fn lt(mut self, v: $ty) -> Self {
                self.k.lt = Some(v);
                self
            }

            #[must_use]
            pub const fn lte(mut self, v: $ty) -> Self {
                self.k.lte = Some(v);
                self
            }

            /// The value must be one of `vs` (the spec's `in`).
            #[must_use]
            pub fn allowed(mut self, vs: impl IntoIterator<Item = $ty>) -> Self {
                self.k.r#in = vs.into_iter().collect();
                self
            }

            /// The value must be none of `vs` (the spec's `not_in`).
            #[must_use]
            pub fn forbidden(mut self, vs: impl IntoIterator<Item = $ty>) -> Self {
                self.k.not_in = vs.into_iter().collect();
                self
            }

            #[must_use]
            pub const fn multiple_of(mut self, v: $ty) -> Self {
                self.k.multiple_of = Some(v);
                self
            }
        }
    };
}

number!(/// A 32-bit signed integer field.
    int32, Int32B, Int32, Int32, i32);
number!(/// A 64-bit signed integer field.
    int64, Int64B, Int64, Int64, i64);
number!(/// A 32-bit unsigned integer field.
    uint32, Uint32B, UInt32, Uint32, u32);
number!(/// A 64-bit unsigned integer field.
    uint64, Uint64B, UInt64, Uint64, u64);
number!(/// A 32-bit float field.
    float, FloatB, Float, Float, f32);
number!(/// A 64-bit float field.
    double, DoubleB, Double, Double, f64);

impl FloatB {
    /// Rejects NaN and the infinities.
    #[must_use]
    pub const fn finite(mut self) -> Self {
        self.k.finite = true;
        self
    }
}

impl DoubleB {
    /// Rejects NaN and the infinities.
    #[must_use]
    pub const fn finite(mut self) -> Self {
        self.k.finite = true;
        self
    }
}

// =============================================================================
// Bool / String / Bytes
// =============================================================================

kind_builder!(/// A bool field.
    BoolB, BoolKind, Bool);

/// A bool field.
#[must_use]
pub fn bool(name: &str) -> BoolB {
    BoolB {
        f: named(name),
        k: <BoolKind as Default>::default(),
    }
}

impl BoolB {
    #[must_use]
    pub const fn default(mut self, v: bool) -> Self {
        self.k.default = Some(v);
        self
    }

    #[must_use]
    pub const fn equals(mut self, v: bool) -> Self {
        self.k.r#const = Some(v);
        self
    }
}

kind_builder!(/// A string field.
    StringB, StringKind, String);

/// A string field; lengths count Unicode code points.
#[must_use]
pub fn string(name: &str) -> StringB {
    StringB {
        f: named(name),
        k: <StringKind as Default>::default(),
    }
}

impl StringB {
    #[must_use]
    pub fn default(mut self, v: &str) -> Self {
        self.k.default = Some(v.to_owned());
        self
    }

    #[must_use]
    pub fn equals(mut self, v: &str) -> Self {
        self.k.r#const = Some(v.to_owned());
        self
    }

    #[must_use]
    pub const fn len(mut self, n: u64) -> Self {
        self.k.len = Some(n);
        self
    }

    #[must_use]
    pub const fn min_len(mut self, n: u64) -> Self {
        self.k.min_len = Some(n);
        self
    }

    #[must_use]
    pub const fn max_len(mut self, n: u64) -> Self {
        self.k.max_len = Some(n);
        self
    }

    /// An RE2-compatible pattern the whole value must match somewhere.
    #[must_use]
    pub fn pattern(mut self, re: &str) -> Self {
        self.k.pattern = Some(re.to_owned());
        self
    }

    /// A named format: `email`, `uuid`, `url`, `hostname`, `ipv4`, `ipv6`,
    /// `ip`, `date`, `time`, `date-time`, or one the validator registers.
    #[must_use]
    pub fn format(mut self, name: &str) -> Self {
        self.k.format = Some(name.to_owned());
        self
    }

    #[must_use]
    pub fn allowed<'a>(mut self, vs: impl IntoIterator<Item = &'a str>) -> Self {
        self.k.r#in = vs.into_iter().map(str::to_owned).collect();
        self
    }

    #[must_use]
    pub fn forbidden<'a>(mut self, vs: impl IntoIterator<Item = &'a str>) -> Self {
        self.k.not_in = vs.into_iter().map(str::to_owned).collect();
        self
    }
}

kind_builder!(/// A bytes field.
    BytesB, BytesKind, Bytes);

/// A bytes field.
#[must_use]
pub fn bytes(name: &str) -> BytesB {
    BytesB {
        f: named(name),
        k: <BytesKind as Default>::default(),
    }
}

impl BytesB {
    #[must_use]
    pub fn default(mut self, v: &[u8]) -> Self {
        self.k.default = Some(v.to_vec());
        self
    }

    #[must_use]
    pub fn equals(mut self, v: &[u8]) -> Self {
        self.k.r#const = Some(v.to_vec());
        self
    }

    #[must_use]
    pub const fn len(mut self, n: u64) -> Self {
        self.k.len = Some(n);
        self
    }

    #[must_use]
    pub const fn min_len(mut self, n: u64) -> Self {
        self.k.min_len = Some(n);
        self
    }

    #[must_use]
    pub const fn max_len(mut self, n: u64) -> Self {
        self.k.max_len = Some(n);
        self
    }

    #[must_use]
    pub fn prefix(mut self, v: &[u8]) -> Self {
        self.k.prefix = Some(v.to_vec());
        self
    }

    #[must_use]
    pub fn suffix(mut self, v: &[u8]) -> Self {
        self.k.suffix = Some(v.to_vec());
        self
    }

    #[must_use]
    pub fn allowed<'a>(mut self, vs: impl IntoIterator<Item = &'a [u8]>) -> Self {
        self.k.r#in = vs.into_iter().map(<[u8]>::to_vec).collect();
        self
    }

    #[must_use]
    pub fn forbidden<'a>(mut self, vs: impl IntoIterator<Item = &'a [u8]>) -> Self {
        self.k.not_in = vs.into_iter().map(<[u8]>::to_vec).collect();
        self
    }
}

// =============================================================================
// Choice / Duration / Timestamp / Json
// =============================================================================

kind_builder!(/// A choice field: one of a set of typed options.
    ChoiceB, Choice, Choice);

/// A choice field; add options with [`ChoiceB::opt`] or the `*_opts` helpers.
#[must_use]
pub fn choice(name: &str) -> ChoiceB {
    ChoiceB {
        f: named(name),
        k: <Choice as Default>::default(),
    }
}

impl ChoiceB {
    /// One option: a typed value and a label for people (empty: the value).
    #[must_use]
    pub fn opt(mut self, value: impl IntoNative, label: &str) -> Self {
        self.k.options.push(ChoiceOption {
            value: Some(from_native(&value.into_native())),
            label: label.to_owned(),
            ..Default::default()
        });
        self
    }

    /// One option with a description; a deprecated option stays valid but
    /// is flagged for people.
    #[must_use]
    pub fn opt_described(
        mut self,
        value: impl IntoNative,
        label: &str,
        description: &str,
        deprecated: bool,
    ) -> Self {
        self.k.options.push(ChoiceOption {
            value: Some(from_native(&value.into_native())),
            label: label.to_owned(),
            description: description.to_owned(),
            deprecated,
        });
        self
    }

    /// String options; each value is its own label.
    #[must_use]
    pub fn str_opts<'a>(self, vs: impl IntoIterator<Item = &'a str>) -> Self {
        vs.into_iter().fold(self, |b, v| b.opt(v, ""))
    }

    /// Integer options; each value is its own label.
    #[must_use]
    pub fn int_opts(self, vs: impl IntoIterator<Item = i64>) -> Self {
        vs.into_iter().fold(self, |b, v| b.opt(v, ""))
    }

    #[must_use]
    pub fn default(mut self, v: impl IntoNative) -> Self {
        self.k.default = Some(from_native(&v.into_native()));
        self
    }

    /// Advisory options: a value outside them is valid.
    #[must_use]
    pub const fn open(mut self) -> Self {
        self.k.open = true;
        self
    }

    /// A CEL expression over `root` producing the options (replaces them).
    #[must_use]
    pub fn options_expr(mut self, expr: &str) -> Self {
        self.k.options_expr = Some(expr.to_owned());
        self
    }
}

/// A duration bound or default: `std::time::Duration` or the proto
/// `Duration` (which can be negative).
pub trait IntoDuration {
    fn into_duration(self) -> pbjson_types::Duration;
}

impl IntoDuration for pbjson_types::Duration {
    fn into_duration(self) -> pbjson_types::Duration {
        self
    }
}

impl IntoDuration for StdDuration {
    fn into_duration(self) -> pbjson_types::Duration {
        match self.into_native() {
            Native::Duration(d) => d,
            _ => pbjson_types::Duration::default(),
        }
    }
}

/// A timestamp bound or default: `SystemTime` or the proto `Timestamp`.
pub trait IntoTimestamp {
    fn into_timestamp(self) -> pbjson_types::Timestamp;
}

impl IntoTimestamp for pbjson_types::Timestamp {
    fn into_timestamp(self) -> pbjson_types::Timestamp {
        self
    }
}

impl IntoTimestamp for SystemTime {
    fn into_timestamp(self) -> pbjson_types::Timestamp {
        match self.into_native() {
            Native::Timestamp(t) => t,
            _ => pbjson_types::Timestamp::default(),
        }
    }
}

macro_rules! bounded {
    ($(#[$doc:meta])* $fn:ident, $b:ident, $kind:ty, $variant:ident, $arg:ident, $into:ident) => {
        kind_builder!($(#[$doc])* $b, $kind, $variant);

        $(#[$doc])*
        #[must_use]
        pub fn $fn(name: &str) -> $b {
            $b { f: named(name), k: <$kind as Default>::default() }
        }

        impl $b {
            #[must_use]
            pub fn default(mut self, v: impl $arg) -> Self {
                self.k.default = Some(v.$into());
                self
            }

            #[must_use]
            pub fn gt(mut self, v: impl $arg) -> Self {
                self.k.gt = Some(v.$into());
                self
            }

            #[must_use]
            pub fn gte(mut self, v: impl $arg) -> Self {
                self.k.gte = Some(v.$into());
                self
            }

            #[must_use]
            pub fn lt(mut self, v: impl $arg) -> Self {
                self.k.lt = Some(v.$into());
                self
            }

            #[must_use]
            pub fn lte(mut self, v: impl $arg) -> Self {
                self.k.lte = Some(v.$into());
                self
            }
        }
    };
}

bounded!(/// A duration field.
    duration, DurationB, DurationKind, Duration, IntoDuration, into_duration);
bounded!(/// A timestamp field.
    timestamp, TimestampB, TimestampKind, Timestamp, IntoTimestamp, into_timestamp);

kind_builder!(/// A free-form JSON field: any value (rules still apply).
    JsonB, Json, Json);

/// A free-form JSON field: any value passes (rules still apply).
#[must_use]
pub fn json(name: &str) -> JsonB {
    JsonB {
        f: named(name),
        k: <Json as Default>::default(),
    }
}

impl JsonB {
    #[must_use]
    pub fn default(mut self, v: impl IntoNative) -> Self {
        self.k.default = Some(from_native(&v.into_native()));
        self
    }
}

// =============================================================================
// Containers
// =============================================================================

kind_builder!(/// A list field.
    ListB, List, List);

/// A list whose items all satisfy `item` (its name is ignored; error
/// paths index the list).
#[must_use]
pub fn list(name: &str, item: impl Into<Field>) -> ListB {
    ListB {
        f: named(name),
        k: List {
            items: vec![item.into()],
            ..Default::default()
        },
    }
}

/// A positional tuple: item `i` satisfies `items[i]`.
#[must_use]
pub fn tuple(name: &str, items: impl IntoIterator<Item = Field>) -> ListB {
    ListB {
        f: named(name),
        k: List {
            items: items.into_iter().collect(),
            ..Default::default()
        },
    }
}

impl ListB {
    #[must_use]
    pub const fn min_items(mut self, n: u64) -> Self {
        self.k.min_items = Some(n);
        self
    }

    #[must_use]
    pub const fn max_items(mut self, n: u64) -> Self {
        self.k.max_items = Some(n);
        self
    }

    /// No two items are equal.
    #[must_use]
    pub const fn unique(mut self) -> Self {
        self.k.unique = true;
        self
    }

    /// A CEL expression over `root` giving the exact length.
    #[must_use]
    pub fn count(mut self, expr: &str) -> Self {
        self.k.count_expr = Some(expr.to_owned());
        self
    }
}

/// An object field: a nested set of fields.
#[derive(Debug, Clone)]
pub struct ObjectB {
    f: Field,
    sub: Schema,
    default: Option<StructValue>,
}

impl FieldDef for ObjectB {
    fn field_mut(&mut self) -> &mut Field {
        &mut self.f
    }
}

impl From<ObjectB> for Field {
    fn from(b: ObjectB) -> Self {
        let mut f = b.f;
        f.kind = Some(Kind::Object(Object {
            schema: Some(b.sub),
            default: b.default,
        }));
        f
    }
}

/// An object field; add its fields with [`ObjectB::field`].
#[must_use]
pub fn object(name: &str) -> ObjectB {
    ObjectB {
        f: named(name),
        sub: Schema::default(),
        default: None,
    }
}

impl ObjectB {
    #[must_use]
    pub fn field(mut self, def: impl Into<Field>) -> Self {
        self.sub.fields.push(def.into());
        self
    }

    /// An absent object is created empty, so its fields' defaults apply.
    #[must_use]
    pub fn default_empty(mut self) -> Self {
        self.default = Some(StructValue::default());
        self
    }

    /// Unknown keys are errors.
    #[must_use]
    pub const fn strict(mut self) -> Self {
        self.sub.strict = true;
        self
    }

    #[must_use]
    pub const fn min_props(mut self, n: u64) -> Self {
        self.sub.min_properties = Some(n);
        self
    }

    #[must_use]
    pub const fn max_props(mut self, n: u64) -> Self {
        self.sub.max_properties = Some(n);
        self
    }

    /// A CEL rule over the object as `root`.
    #[must_use]
    pub fn object_rule(mut self, r: RuleB) -> Self {
        self.sub.rules.push(r.0);
        self
    }
}

kind_builder!(/// A map field: string keys to values of one shape.
    MapB, Map, Map);

/// A map whose values are objects; add their fields with [`MapB::field`].
#[must_use]
pub fn map(name: &str) -> MapB {
    MapB {
        f: named(name),
        k: Map {
            value_schema: Some(Schema::default()),
            ..Default::default()
        },
    }
}

/// A map whose values all satisfy `value` (not objects; its name is
/// ignored, error paths name the key).
#[must_use]
pub fn map_of(name: &str, value: impl Into<Field>) -> MapB {
    MapB {
        f: named(name),
        k: Map {
            value_field: Some(Box::new(value.into())),
            ..Default::default()
        },
    }
}

impl MapB {
    /// A field of every value object (maps built with [`map`]).
    #[must_use]
    pub fn field(mut self, def: impl Into<Field>) -> Self {
        self.k
            .value_schema
            .get_or_insert_with(Schema::default)
            .fields
            .push(def.into());
        self
    }

    /// The definition every key must satisfy (a string, integer or bool
    /// kind; keys are compared in their text form).
    #[must_use]
    pub fn key(mut self, def: impl Into<Field>) -> Self {
        self.k.key_field = Some(Box::new(def.into()));
        self
    }

    /// Unknown keys in a value object are errors (maps built with [`map`]).
    #[must_use]
    pub fn strict(mut self) -> Self {
        self.k
            .value_schema
            .get_or_insert_with(Schema::default)
            .strict = true;
        self
    }

    /// A CEL rule over each value object as `root` (maps built with [`map`]).
    #[must_use]
    pub fn value_rule(mut self, r: RuleB) -> Self {
        self.k
            .value_schema
            .get_or_insert_with(Schema::default)
            .rules
            .push(r.0);
        self
    }

    #[must_use]
    pub const fn min_entries(mut self, n: u64) -> Self {
        self.k.min_entries = Some(n);
        self
    }

    #[must_use]
    pub const fn max_entries(mut self, n: u64) -> Self {
        self.k.max_entries = Some(n);
        self
    }
}

kind_builder!(/// A discriminated union: the discriminator key selects a variant.
    OneOfB, OneOf, OneOf);

/// A discriminated union; `discriminator` is the key naming the variant.
#[must_use]
pub fn oneof(name: &str, discriminator: &str) -> OneOfB {
    OneOfB {
        f: named(name),
        k: OneOf {
            discriminator: discriminator.to_owned(),
            variants: HashMap::new(),
        },
    }
}

impl OneOfB {
    /// The fields of the variant selected by `key`.
    #[must_use]
    pub fn variant(mut self, key: &str, fields: impl IntoIterator<Item = Field>) -> Self {
        self.k.variants.insert(
            key.to_owned(),
            Schema {
                fields: fields.into_iter().collect(),
                ..Default::default()
            },
        );
        self
    }

    /// A variant given as a whole schema (another schema's identity and
    /// rules included).
    #[must_use]
    pub fn variant_schema(mut self, key: &str, s: Schema) -> Self {
        self.k.variants.insert(key.to_owned(), s);
        self
    }
}

kind_builder!(/// A computed field: a CEL expression over `root`.
    ComputedB, Computed, Computed);

/// A field computed from the others; never an input.
#[must_use]
pub fn computed(name: &str, expr: &str) -> ComputedB {
    ComputedB {
        f: named(name),
        k: Computed {
            expr: expr.to_owned(),
            result: None,
        },
    }
}

impl ComputedB {
    /// The declared result kind (checked at compile time of the schema).
    #[must_use]
    pub const fn result(mut self, rt: ResultType) -> Self {
        self.k.result = Some(rt as i32);
        self
    }
}

kind_builder!(/// A reference to a shared definition.
    RefB, Ref, Ref);

/// A field shaped by the schema's definition `def` ([`SchemaB::def`]).
#[must_use]
pub fn ref_def(name: &str, def: &str) -> RefB {
    RefB {
        f: named(name),
        k: Ref {
            default: None,
            target: Some(Target::Name(def.to_owned())),
        },
    }
}

/// A field shaped by a registered schema.
#[must_use]
pub fn ref_id(name: &str, id: SchemaIdentity) -> RefB {
    RefB {
        f: named(name),
        k: Ref {
            default: None,
            target: Some(Target::Id(id)),
        },
    }
}

impl RefB {
    /// An absent value is created empty, so the definition's defaults apply.
    #[must_use]
    pub fn default_empty(mut self) -> Self {
        self.k.default = Some(StructValue::default());
        self
    }
}

// =============================================================================
// Schema
// =============================================================================

/// A schema under construction.
#[derive(Debug, Clone)]
pub struct SchemaB(Schema);

/// Starts a schema named `name` in `namespace`; coercion is on.
#[must_use]
pub fn schema(namespace: &str, name: &str) -> SchemaB {
    SchemaB(Schema {
        id: Some(crate::typed::id(namespace, name, &Version::zero())),
        coerce: true,
        ..Default::default()
    })
}

impl SchemaB {
    #[must_use]
    pub fn version(mut self, v: &Version) -> Self {
        if let Some(id) = self.0.id.as_mut() {
            id.version = v.to_string();
        }
        self
    }

    #[must_use]
    pub fn desc(mut self, d: &str) -> Self {
        self.0.description = Some(d.to_owned());
        self
    }

    /// Unknown keys are errors.
    #[must_use]
    pub const fn strict(mut self) -> Self {
        self.0.strict = true;
        self
    }

    /// Whether string inputs convert to the declared kind (default on).
    #[must_use]
    pub const fn coerce(mut self, on: bool) -> Self {
        self.0.coerce = on;
        self
    }

    #[must_use]
    pub const fn min_props(mut self, n: u64) -> Self {
        self.0.min_properties = Some(n);
        self
    }

    #[must_use]
    pub const fn max_props(mut self, n: u64) -> Self {
        self.0.max_properties = Some(n);
        self
    }

    #[must_use]
    pub fn field(mut self, def: impl Into<Field>) -> Self {
        self.0.fields.push(def.into());
        self
    }

    #[must_use]
    pub fn fields(mut self, defs: impl IntoIterator<Item = Field>) -> Self {
        self.0.fields.extend(defs);
        self
    }

    /// A CEL rule over `root`.
    #[must_use]
    pub fn rule(mut self, r: RuleB) -> Self {
        self.0.rules.push(r.0);
        self
    }

    /// A named definition fields reach with [`ref_def`].
    #[must_use]
    pub fn def(mut self, name: &str, fields: impl IntoIterator<Item = Field>) -> Self {
        self.0.defs.insert(
            name.to_owned(),
            Schema {
                fields: fields.into_iter().collect(),
                ..Default::default()
            },
        );
        self
    }

    /// A named definition given as a whole schema.
    #[must_use]
    pub fn def_schema(mut self, name: &str, s: Schema) -> Self {
        self.0.defs.insert(name.to_owned(), s);
        self
    }

    /// A Mustache template rendered against resolved values.
    #[must_use]
    pub fn template(mut self, name: &str, text: &str) -> Self {
        self.0.templates.insert(name.to_owned(), text.to_owned());
        self
    }

    /// `field` must be present when `cond` (CEL over `root`) holds.
    #[must_use]
    pub fn required_when(self, field: &str, cond: &str) -> Self {
        let key = quoted(field);
        self.rule(
            rule(
                &format!("!({cond}) || ({key} in root)"),
                &format!("{field} is required when: {cond}"),
            )
            .id(field),
        )
    }

    /// `field` must be present unless `cond` holds.
    #[must_use]
    pub fn required_unless(self, field: &str, cond: &str) -> Self {
        let key = quoted(field);
        self.rule(
            rule(
                &format!("({cond}) || ({key} in root)"),
                &format!("{field} is required unless: {cond}"),
            )
            .id(field),
        )
    }

    /// `field` must be absent when `cond` holds.
    #[must_use]
    pub fn forbidden_when(self, field: &str, cond: &str) -> Self {
        let key = quoted(field);
        self.rule(
            rule(
                &format!("!({cond}) || !({key} in root)"),
                &format!("{field} must be absent when: {cond}"),
            )
            .id(field),
        )
    }

    /// The schema as data (to store, send or compile later).
    #[must_use]
    pub fn done(self) -> Schema {
        self.0
    }

    /// Compiles the schema: every defect surfaces here.
    pub fn build(self) -> Result<Validator, SchemaError> {
        Validator::new(self.0)
    }
}

fn quoted(s: &str) -> String {
    serde_json::to_string(s).unwrap_or_default()
}
