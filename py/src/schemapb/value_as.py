"""Typed extraction from a wire Value and value path lookup.

``value_as(v, int)`` reads one value as a native Python type, dispatching
on the type object (Python's runtime type token). One rule, shared by
every implementation and pinned by the conformance golden value-as.json: a
conversion succeeds iff the value is represented in the target EXACTLY
(lossless round-trip). Numeric values convert across kinds under that
rule; non-numeric targets are strict - no string parsing, no truncation,
no coercion. Python note: ``int`` is unbounded, so the int token covers
the full int64 AND uint64 wire ranges (the exactness rule with no range
constraint on the target).

``value_lookup`` resolves a path in the ValidationError dialect
("replicas[0].name") against a StructValue - the path from a validation
error fetches the offending value directly. The string form cannot address
a map key containing '.' or '['; the ``value_field``/``value_index``
steppers cover arbitrary keys without parsing.
"""

from __future__ import annotations

import datetime as dt
import json
from enum import StrEnum
from typing import TYPE_CHECKING, overload

import betterproto2

from schemapb.descriptor import join_path
from schemapb.value import struct_v

if TYPE_CHECKING:
    from schemapb._gen.schemapb import StructValue, Value

__all__ = [
    "ValueLookupError",
    "ValueLookupReason",
    "value_as",
    "value_field",
    "value_index",
    "value_kind_name",
    "value_lookup",
]

_KIND_NAMES = {
    "null_value": "null",
    "bool_value": "bool",
    "int32_value": "int32",
    "int64_value": "int64",
    "uint32_value": "uint32",
    "uint64_value": "uint64",
    "float_value": "float",
    "double_value": "double",
    "string_value": "string",
    "bytes_value": "bytes",
    "duration_value": "duration",
    "timestamp_value": "timestamp",
    "list_value": "list",
    "struct_value": "struct",
}

_INT_KINDS = {"int32_value", "int64_value", "uint32_value", "uint64_value"}
_FLOAT_KINDS = {"float_value", "double_value"}


def value_kind_name(v: Value) -> str:
    """The wire kind of a value as its spec short name."""
    name, _ = betterproto2.which_one_of(v, "kind")
    return _KIND_NAMES.get(name, "null" if name == "" else "")


def _value_int(v: Value) -> int | None:
    """The exact integer view of a numeric value (unbounded target)."""
    name, val = betterproto2.which_one_of(v, "kind")
    if name in _INT_KINDS and isinstance(val, int):
        return val
    if name in _FLOAT_KINDS and isinstance(val, float):
        return int(val) if val.is_integer() else None
    return None


def _value_double(v: Value) -> float | None:
    """The exact float64 view of a numeric value."""
    name, val = betterproto2.which_one_of(v, "kind")
    if name in _FLOAT_KINDS and isinstance(val, float):
        return val
    if name in _INT_KINDS and isinstance(val, int):
        f = float(val)
        return f if int(f) == val else None
    return None


@overload
def value_as(v: Value, target: type[bool]) -> bool | None: ...
@overload
def value_as(v: Value, target: type[int]) -> int | None: ...
@overload
def value_as(v: Value, target: type[float]) -> float | None: ...
@overload
def value_as(v: Value, target: type[str]) -> str | None: ...
@overload
def value_as(v: Value, target: type[bytes]) -> bytes | None: ...
@overload
def value_as(v: Value, target: type[dt.timedelta]) -> dt.timedelta | None: ...
@overload
def value_as(v: Value, target: type[dt.datetime]) -> dt.datetime | None: ...
@overload
def value_as(v: Value, target: type[list]) -> list[Value] | None: ...  # type: ignore[type-arg]
@overload
def value_as(v: Value, target: type[dict]) -> dict[str, Value] | None: ...  # type: ignore[type-arg]
def value_as(v: Value, target: type) -> object | None:  # noqa: PLR0911 - flat dispatch per token
    """Read the value as the target type, ``None`` unless exact.

    ``bool`` must come before ``int`` checks: Python bools are ints.
    """
    name, val = betterproto2.which_one_of(v, "kind")
    if target is bool:
        return val if name == "bool_value" else None
    if target is int:
        return _value_int(v)
    if target is float:
        return _value_double(v)
    if target is str:
        return val if name == "string_value" else None
    if target is bytes:
        return val if name == "bytes_value" else None
    if target is dt.timedelta:
        return val if name == "duration_value" else None
    if target is dt.datetime:
        return val if name == "timestamp_value" else None
    if target is list:
        lv = v.list_value
        return list(lv.items) if name == "list_value" and lv is not None else None
    if target is dict:
        sv = v.struct_value
        return dict(sv.fields) if name == "struct_value" and sv is not None else None
    return None


class ValueLookupReason(StrEnum):
    """Stable spec strings shared by all implementations (conformance)."""

    EMPTY_PATH = "empty_path"
    BAD_PATH = "bad_path"
    NOT_FOUND = "not_found"
    INDEX_OUT_OF_RANGE = "index_out_of_range"
    NOT_A_STRUCT = "not_a_struct"
    NOT_A_LIST = "not_a_list"


class ValueLookupError(Exception):
    """Pinpoints the failing segment of a value path.

    ``at`` is the resolved parent path ("" for root), ``segment`` the key
    or "[i]" index that failed.
    """

    def __init__(self, reason: ValueLookupReason, at: str = "", segment: str = "") -> None:
        self.at = at
        self.segment = segment
        self.reason = reason
        super().__init__(self._message())

    def _message(self) -> str:
        where = json.dumps(self.at) if self.at else "root"
        seg = json.dumps(self.segment)
        if self.reason is ValueLookupReason.EMPTY_PATH:
            return "schemapb: value lookup: empty path"
        if self.reason is ValueLookupReason.BAD_PATH:
            return f"schemapb: value lookup: malformed path {seg}"
        if self.reason is ValueLookupReason.NOT_FOUND:
            return f"schemapb: value lookup: no field {seg} in {where}"
        if self.reason is ValueLookupReason.INDEX_OUT_OF_RANGE:
            return f"schemapb: value lookup: index {self.segment} out of range in {where}"
        if self.reason is ValueLookupReason.NOT_A_STRUCT:
            return f"schemapb: value lookup: {where} is not a struct, cannot read field {seg}"
        return f"schemapb: value lookup: {where} is not a list, cannot index {self.segment}"


def value_field(v: Value, name: str) -> Value | None:
    """Step into a struct value member (handles keys paths cannot spell)."""
    sv = v.struct_value
    return sv.fields.get(name) if sv is not None else None


def value_index(v: Value, i: int) -> Value | None:
    """Step into a list value element (``None`` when absent)."""
    lv = v.list_value
    if lv is None or i < 0 or i >= len(lv.items):
        return None
    return lv.items[i]


def _parse_value_path(path: str) -> list[str | int] | None:  # noqa: C901 - one branch per token form
    """Tokenize the error-path dialect: key ('.' key | '[' int ']')*."""
    tokens: list[str | int] = []
    rest = path
    while rest:
        if rest[0] == "[":
            if not tokens:
                return None  # paths start with a key, not an index
            end = rest.find("]")
            body = rest[1:end] if end >= 0 else ""
            # digits only: no "+3", "-0", "[]"
            if end < 2 or not body.isdigit():
                return None
            tokens.append(int(body))
            rest = rest[end + 1 :]
            if rest and rest[0] not in ".[":
                return None  # after "]" only ".", "[" or the end
            continue
        if rest[0] == ".":
            if not tokens:
                return None  # leading dot
            rest = rest[1:]
            # A dot must be followed by a key: no trailing dot, "a..b", "a.[0]".
            if not rest or rest[0] in ".[":
                return None
            continue
        cut = len(rest)
        for i, ch in enumerate(rest):
            if ch in ".[":
                cut = i
                break
        tokens.append(rest[:cut])
        rest = rest[cut:]
    return tokens or None


def value_lookup(s: StructValue, path: str) -> Value:
    """Resolve an error-path-dialect path against the struct's values.

    Returns the addressed value or raises ``ValueLookupError`` naming the
    exact segment that failed.
    """
    if path == "":
        raise ValueLookupError(ValueLookupReason.EMPTY_PATH)

    tokens = _parse_value_path(path)
    if tokens is None:
        raise ValueLookupError(ValueLookupReason.BAD_PATH, "", path)

    cur = struct_v(dict(s.fields))
    parent = ""
    for tok in tokens:
        if isinstance(tok, str):
            sv = cur.struct_value
            if sv is None:
                raise ValueLookupError(ValueLookupReason.NOT_A_STRUCT, parent, tok)
            nxt = sv.fields.get(tok)
            if nxt is None:
                raise ValueLookupError(ValueLookupReason.NOT_FOUND, parent, tok)
            cur = nxt
            parent = join_path(parent, tok)
            continue

        seg = f"[{tok}]"
        lv = cur.list_value
        if lv is None:
            raise ValueLookupError(ValueLookupReason.NOT_A_LIST, parent, seg)
        if tok >= len(lv.items):
            raise ValueLookupError(ValueLookupReason.INDEX_OUT_OF_RANGE, parent, seg)
        cur = lv.items[tok]
        parent += seg

    return cur
