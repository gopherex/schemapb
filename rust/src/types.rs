//! String types that carry their format: a field of such a type is
//! validated against the format with no attribute (`email: Email`).
//!
//! The types hold any string, like a `String` field with a `format`: the
//! check happens where every other check does, in validation, so an
//! invalid value is reported with its path instead of failing to decode.

use std::fmt;
use std::ops::Deref;

use serde::{Deserialize, Serialize};

use crate::gen::schemapb::schema::Field;
use crate::reflect::{formatted, ReflectField};

macro_rules! format_type {
    ($(#[$doc:meta])* $name:ident, $format:literal) => {
        $(#[$doc])*
        #[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash, Default, Serialize, Deserialize)]
        #[serde(transparent)]
        pub struct $name(pub String);

        impl $name {
            /// The format this type carries.
            pub const FORMAT: &'static str = $format;

            #[must_use]
            pub fn as_str(&self) -> &str {
                &self.0
            }

            #[must_use]
            pub fn into_inner(self) -> String {
                self.0
            }
        }

        impl ReflectField for $name {
            fn field(name: &str) -> Field {
                formatted(name, $format)
            }
        }

        impl Deref for $name {
            type Target = str;

            fn deref(&self) -> &str {
                &self.0
            }
        }

        impl AsRef<str> for $name {
            fn as_ref(&self) -> &str {
                &self.0
            }
        }

        impl fmt::Display for $name {
            fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
                f.write_str(&self.0)
            }
        }

        impl From<String> for $name {
            fn from(s: String) -> Self {
                Self(s)
            }
        }

        impl From<&str> for $name {
            fn from(s: &str) -> Self {
                Self(s.to_owned())
            }
        }

        impl From<$name> for String {
            fn from(v: $name) -> Self {
                v.0
            }
        }
    };
}

format_type!(
    /// An email address (format `email`).
    Email,
    "email"
);

format_type!(
    /// A DNS host name (format `hostname`).
    Hostname,
    "hostname"
);
