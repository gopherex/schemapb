//! schemapb — native Rust implementation of the schemapb contract: a
//! runtime, proto-defined form/config schema descriptor with validation,
//! CEL-computed values and Mustache rendering. Behaviour is pinned by the
//! cross-language conformance suite; the Go implementation is the reference.

pub mod bake;
pub mod compute;
pub mod descriptor;
pub mod duration;
pub mod engine;
pub mod formats;
pub mod gen;
pub mod messages;
pub mod registry;
pub mod render;
pub mod typed;
pub mod validate;
pub mod value;
