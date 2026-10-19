//! The spec's shared message-template set (must match
//! conformance/golden/messages.json, written by the Go reference).

use std::collections::HashMap;

use crate::gen::schemapb::{ErrorCode, ValidationError, Value};
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

/// The spec's display form of a value, used by the default messages.
#[must_use]
pub fn display_value(v: &Value) -> String {
    display_string(&to_native(Some(v)))
}

fn fill(
    tpl: &str,
    expected: Option<&Value>,
    actual: Option<&Value>,
    display: &dyn Fn(&Value) -> String,
) -> String {
    let sub = |s: &str, placeholder: &str, v: Option<&Value>| -> String {
        if !s.contains(placeholder) {
            return s.to_owned();
        }
        let disp = v.map_or_else(|| "…".to_owned(), display);
        s.replace(placeholder, &disp)
    };
    sub(&sub(tpl, "{expected}", expected), "{actual}", actual)
}

/// Renders a code's template with the typed values ("…" for absent ones).
#[must_use]
pub fn render_message(code: ErrorCode, expected: Option<&Value>, actual: Option<&Value>) -> String {
    template(code).map_or_else(String::new, |tpl| {
        fill(tpl, expected, actual, &display_value)
    })
}

/// A message catalog for one locale: keys are [`message_key`] values,
/// templates use the `{expected}` and `{actual}` placeholders.
pub type Catalog = HashMap<String, String>;

/// The catalog key that translates `err`: the rule id for a violated rule
/// that declares one, the `ErrorCode` name otherwise.
#[must_use]
pub fn message_key(err: &ValidationError) -> String {
    match err.rule_id.as_deref() {
        Some(id) if err.code() == ErrorCode::RuleViolated && !id.is_empty() => id.to_owned(),
        _ => err.code().as_str_name().to_owned(),
    }
}

/// Renders `err` from `catalog`, substituting the typed values with
/// `display` (`None` => [`display_value`]); a missing key falls back to
/// `err.message`.
#[must_use]
pub fn format_message(
    err: &ValidationError,
    catalog: &Catalog,
    display: Option<&dyn Fn(&Value) -> String>,
) -> String {
    catalog.get(&message_key(err)).map_or_else(
        || err.message.clone(),
        |tpl| {
            fill(
                tpl,
                err.expected.as_ref(),
                err.actual.as_ref(),
                display.unwrap_or(&display_value),
            )
        },
    )
}
