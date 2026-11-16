//! schemapb — native Rust implementation of the schemapb contract: a
//! runtime, proto-defined form/config schema descriptor with validation,
//! CEL-computed values and Mustache rendering. Behaviour is pinned by the
//! cross-language conformance suite; the Go implementation is the reference.

pub mod bake;
pub mod build;
pub mod compute;
pub mod descriptor;
pub mod duration;
pub mod engine;
pub mod formats;
pub mod gen;
pub mod lookup;
pub mod messages;
pub mod model;
pub mod native;
pub mod reflect;
#[cfg(feature = "derive")]
pub use schemapb_derive::{Reflect, Validate};
pub mod registry;
pub mod render;
pub mod typed;
pub mod types;
pub mod validate;
pub mod validator;
pub mod value;
pub mod value_as;
pub mod values;

pub use gen::schemapb::ErrorCode;
pub use model::Validate;
pub use types::{Email, Hostname};
pub use validator::{Invalid, LoadError, Report, Severity, Validator, Violation};
pub use values::{FromNative, IntoNative, ValueError, Values};

mod path;

mod resolve;

mod masked;

/// What the derive expansion names; not a public API.
#[doc(hidden)]
pub mod __private {
    pub use serde;
}
