//! The spec's shared message-template set (must match
//! conformance/golden/messages.json, written by the Go reference).

use crate::gen::schemapb::{ErrorCode, Value};
use crate::render::display_string;
use crate::value::to_native;

/// The template for one error code; `None` for the free-text codes
/// (`RuleViolated`, `ExprError`, `InvalidSchema`).
#[must_use]
pub const fn template(code: ErrorCode) -> Option<&'static str> {
    use ErrorCode as C;
    match code {
        C::TypeMismatch => Some("expected {expected}"),
        C::RequiredMissing => Some("required"),
        C::NotNullable => Some("must not be null"),
        C::UnknownField => Some("unknown field"),
        C::ImmutableModified => Some("immutable: cannot be changed"),
        C::ConstMismatch => Some("must equal {expected}"),
        C::GtViolated => Some("must be > {expected}"),
        C::GteViolated => Some("must be >= {expected}"),
        C::LtViolated => Some("must be < {expected}"),
        C::LteViolated => Some("must be <= {expected}"),
        C::NotInAllowedSet => Some("must be one of {expected}"),
        C::InForbiddenSet => Some("must not be one of {expected}"),
        C::MultipleOfViolated => Some("must be a multiple of {expected}"),
        C::LenMismatch => Some("length must be exactly {expected}"),
        C::MinLenViolated => Some("length must be at least {expected}"),
        C::MaxLenViolated => Some("length must be at most {expected}"),
        C::PatternMismatch => Some("must match pattern {expected}"),
        C::FormatMismatch => Some("must be a valid {expected}"),
        C::PrefixMismatch => Some("must start with {expected}"),
        C::SuffixMismatch => Some("must end with {expected}"),
        C::UnsupportedFormat => Some("format {expected} is not supported by this implementation"),
        C::ChoiceNotAllowed => Some("must be one of {expected}"),
        C::MinItemsViolated => Some("must have at least {expected} items"),
        C::MaxItemsViolated => Some("must have at most {expected} items"),
        C::NotUnique => Some("must be unique"),
        C::ListCountMismatch => Some("must have exactly {expected} items"),
        C::MinEntriesViolated => Some("must have at least {expected} entries"),
        C::MaxEntriesViolated => Some("must have at most {expected} entries"),
        C::MinPropertiesViolated => Some("must have at least {expected} properties"),
        C::MaxPropertiesViolated => Some("must have at most {expected} properties"),
        C::DiscriminatorMissing => Some("discriminator {expected} must be a non-empty string"),
        C::UnknownVariant => Some("unknown variant {actual}"),
        C::UnknownRef => Some("unknown $ref {expected}"),
        C::Unspecified | C::RuleViolated | C::ExprError | C::InvalidSchema => None,
    }
}

/// Renders a code's template with the typed values ("…" for absent ones).
#[must_use]
pub fn render_message(code: ErrorCode, expected: Option<&Value>, actual: Option<&Value>) -> String {
    let Some(tpl) = template(code) else {
        return String::new();
    };
    let sub = |s: &str, placeholder: &str, v: Option<&Value>| -> String {
        if !s.contains(placeholder) {
            return s.to_owned();
        }
        let disp = v.map_or_else(|| "…".to_owned(), |x| display_string(&to_native(Some(x))));
        s.replace(placeholder, &disp)
    };
    sub(&sub(tpl, "{expected}", expected), "{actual}", actual)
}
