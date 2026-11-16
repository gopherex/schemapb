//! Attributes are typed by their field: one that does not apply, or a
//! literal that does not fit, fails to compile at the attribute.
#![cfg(feature = "derive")]

#[test]
fn misapplied_attributes_do_not_compile() {
    trybuild::TestCases::new().compile_fail("tests/ui/*.rs");
}
