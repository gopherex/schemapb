// Hand-written include shim: the generated sources live next to this file
// (`make gen-rust` rewrites them, never this shim). schemapb.rs pulls in its
// serde companion itself.
#[allow(clippy::all, clippy::pedantic, clippy::nursery)]
pub mod schemapb {
    include!("schemapb/schemapb.rs");
}
