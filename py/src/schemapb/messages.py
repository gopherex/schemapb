"""The spec's shared message-template set (must match
conformance/golden/messages.json, written by the Go reference).
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from schemapb._gen.schemapb import ErrorCode, Value

if TYPE_CHECKING:
    from collections.abc import Callable, Mapping

    from schemapb._gen.schemapb import ValidationError
from schemapb.render import display_string
from schemapb.value import to_native

MESSAGE_TEMPLATES: dict[ErrorCode, str] = {
    ErrorCode.TYPE_MISMATCH: "expected {expected}",
    ErrorCode.REQUIRED_MISSING: "required",
    ErrorCode.NOT_NULLABLE: "must not be null",
    ErrorCode.UNKNOWN_FIELD: "unknown field",
    ErrorCode.IMMUTABLE_MODIFIED: "immutable: cannot be changed",
    ErrorCode.CONST_MISMATCH: "must equal {expected}",
    ErrorCode.GT_VIOLATED: "must be > {expected}",
    ErrorCode.GTE_VIOLATED: "must be >= {expected}",
    ErrorCode.LT_VIOLATED: "must be < {expected}",
    ErrorCode.LTE_VIOLATED: "must be <= {expected}",
    ErrorCode.NOT_IN_ALLOWED_SET: "must be one of {expected}",
    ErrorCode.IN_FORBIDDEN_SET: "must not be one of {expected}",
    ErrorCode.MULTIPLE_OF_VIOLATED: "must be a multiple of {expected}",
    ErrorCode.LEN_MISMATCH: "length must be exactly {expected}",
    ErrorCode.MIN_LEN_VIOLATED: "length must be at least {expected}",
    ErrorCode.MAX_LEN_VIOLATED: "length must be at most {expected}",
    ErrorCode.PATTERN_MISMATCH: "must match pattern {expected}",
    ErrorCode.FORMAT_MISMATCH: "must be a valid {expected}",
    ErrorCode.PREFIX_MISMATCH: "must start with {expected}",
    ErrorCode.SUFFIX_MISMATCH: "must end with {expected}",
    ErrorCode.UNSUPPORTED_FORMAT: "format {expected} is not supported by this implementation",
    ErrorCode.CHOICE_NOT_ALLOWED: "must be one of {expected}",
    ErrorCode.MIN_ITEMS_VIOLATED: "must have at least {expected} items",
    ErrorCode.MAX_ITEMS_VIOLATED: "must have at most {expected} items",
    ErrorCode.NOT_UNIQUE: "must be unique",
    ErrorCode.LIST_COUNT_MISMATCH: "must have exactly {expected} items",
    ErrorCode.MIN_ENTRIES_VIOLATED: "must have at least {expected} entries",
    ErrorCode.MAX_ENTRIES_VIOLATED: "must have at most {expected} entries",
    ErrorCode.MIN_PROPERTIES_VIOLATED: "must have at least {expected} properties",
    ErrorCode.MAX_PROPERTIES_VIOLATED: "must have at most {expected} properties",
    ErrorCode.DISCRIMINATOR_MISSING: "discriminator {expected} must be a non-empty string",
    ErrorCode.UNKNOWN_VARIANT: "unknown variant {actual}",
    ErrorCode.UNKNOWN_REF: "unknown $ref {expected}",
}


def display_value(v: Value) -> str:
    """The spec's display form of a value, used by the default messages."""
    return display_string(to_native(v))


def _fill(
    tpl: str,
    expected: Value | None,
    actual: Value | None,
    display: Callable[[Value], str],
) -> str:
    def sub(s: str, placeholder: str, v: Value | None) -> str:
        if placeholder not in s:
            return s
        disp = "…" if v is None else display(v)
        return s.replace(placeholder, disp)

    return sub(sub(tpl, "{expected}", expected), "{actual}", actual)


def render_message(code: ErrorCode, expected: Value | None, actual: Value | None) -> str:
    tpl = MESSAGE_TEMPLATES.get(code)
    if tpl is None:
        return ""
    return _fill(tpl, expected, actual, display_value)


def message_key(err: ValidationError) -> str:
    """The catalog key that translates err: the rule id for a violated rule
    that declares one, the ErrorCode name otherwise."""
    rule_id: str | None = err.rule_id
    if err.code == ErrorCode.RULE_VIOLATED and rule_id:
        return rule_id
    return f"ERROR_CODE_{ErrorCode(err.code).name}"


def format_message(
    err: ValidationError,
    catalog: Mapping[str, str],
    display: Callable[[Value], str] | None = None,
) -> str:
    """Renders err from a per-locale catalog (keys: message_key), with the
    {expected}/{actual} placeholders; a missing key falls back to
    err.message."""
    tpl = catalog.get(message_key(err))
    if tpl is None:
        message: str = err.message
        return message
    return _fill(tpl, err.expected, err.actual, display or display_value)
