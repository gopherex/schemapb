"""The single conversion point between the value worlds.

wire   — generated Value / StructValue (betterproto2 dataclasses)
native — natural Python values the engine and CEL operate on:

    Null -> None            Bool -> bool
    Int32/64, UInt32/64 -> int (honest, arbitrary precision)
    Float/Double -> float   String -> str      Bytes -> bytes
    Duration -> datetime.timedelta   Timestamp -> datetime.datetime (UTC)
    List -> list            Object/Map -> dict

The declared field kind — not the runtime type — picks the wire variant at
the boundary (canonical_value).
"""

from __future__ import annotations

import datetime as dt
import math
from typing import TypeAlias, Union, cast

import betterproto2

from schemapb._gen.schemapb import (
    ListValue,
    NullValue,
    Schema,
    SchemaField,
    StructValue,
    Value,
)

# Recursive alias: the container members stay as forward references (they
# must — the alias refers to itself), hence the quoted members.
Native: TypeAlias = Union[  # noqa: RUF036 - None leads: mirrors the wire variant order
    None,
    bool,
    int,
    float,
    str,
    bytes,
    dt.timedelta,
    dt.datetime,
    "list[Native]",
    "dict[str, Native]",
]
NativeStruct: TypeAlias = "dict[str, Native]"

_INT32_MIN = -(2**31)
_INT32_MAX = 2**31 - 1
_UINT32_MAX = 2**32 - 1


# =============================================================================
# Constructors (wire values)
# =============================================================================


def null_v() -> Value:
    return Value(null_value=NullValue.NULL_VALUE)


def bool_v(v: bool) -> Value:
    return Value(bool_value=v)


def int32_v(v: int) -> Value:
    return Value(int32_value=v)


def int64_v(v: int) -> Value:
    return Value(int64_value=v)


def uint32_v(v: int) -> Value:
    return Value(uint32_value=v)


def uint64_v(v: int) -> Value:
    return Value(uint64_value=v)


def float_v(v: float) -> Value:
    return Value(float_value=v)


def double_v(v: float) -> Value:
    return Value(double_value=v)


def str_v(v: str) -> Value:
    return Value(string_value=v)


def bytes_v(v: bytes) -> Value:
    return Value(bytes_value=v)


def duration_v(v: dt.timedelta) -> Value:
    return Value(duration_value=v)


def timestamp_v(v: dt.datetime) -> Value:
    return Value(timestamp_value=v)


def list_v(*items: Value) -> Value:
    return Value(list_value=ListValue(items=list(items)))


def struct_v(fields: dict[str, Value]) -> Value:
    return Value(struct_value=StructValue(fields=fields))


# =============================================================================
# Wire -> native
# =============================================================================


def to_native(v: Value | None) -> Native:
    if v is None:
        return None
    name, val = betterproto2.which_one_of(v, "kind")
    match name:
        case "" | "null_value":
            return None
        case "list_value":
            lv = v.list_value
            return [to_native(it) for it in (lv.items if lv is not None else [])]
        case "struct_value":
            return struct_to_native(v.struct_value)
        case "float_value":
            # float32 wire value: keep the float32-rounded double.
            return float(cast("float", val))
        case _:
            return cast("Native", val)


def struct_to_native(s: StructValue | None) -> NativeStruct:
    if s is None:
        return {}
    return {name: to_native(v) for name, v in s.fields.items()}


# =============================================================================
# Native -> wire (best fit, no schema)
# =============================================================================


def from_native(x: Native) -> Value:
    """Best-fitting variant: int -> int64, float -> double."""
    match x:
        case None:
            return null_v()
        case bool():
            return bool_v(x)
        case int():
            return int64_v(x)
        case float():
            return double_v(x)
        case str():
            return str_v(x)
        case bytes():
            return bytes_v(x)
        case dt.timedelta():
            return duration_v(x)
        case dt.datetime():
            return timestamp_v(x)
        case list():
            return list_v(*(from_native(el) for el in x))
        case dict():
            return struct_v({name: from_native(el) for name, el in x.items()})
    msg = f"schemapb: cannot convert {type(x).__name__} to Value"
    raise TypeError(msg)


def struct_from_native(m: NativeStruct) -> StructValue:
    return StructValue(fields={name: from_native(v) for name, v in m.items()})


# =============================================================================
# Numeric coercion helpers
# =============================================================================


def as_int(x: Native) -> int | None:
    """Signed integer view; floats only when integral, bools never."""
    if isinstance(x, bool):
        return None
    if isinstance(x, int):
        return x
    if isinstance(x, float) and math.isfinite(x) and x == math.trunc(x):
        return int(x)
    return None


def as_uint(x: Native) -> int | None:
    n = as_int(x)
    return n if n is not None and n >= 0 else None


def as_float(x: Native) -> float | None:
    if isinstance(x, bool):
        return None
    if isinstance(x, (int, float)):
        return float(x)
    return None


def is_native_struct(x: Native) -> bool:
    return isinstance(x, dict)


# =============================================================================
# Canonical form (field kind -> exact wire variant)
# =============================================================================


class CanonicalError(ValueError):
    """A value that cannot represent the declared kind."""


def _fail(msg: str) -> Value:
    raise CanonicalError(msg)


def canonical_value(f: SchemaField, x: Native) -> Value:
    if x is None:
        return null_v()
    if f.float is not None:
        n = as_float(x)
        return _fail(f"field {f.name}: not numeric") if n is None else float_v(n)
    if f.double is not None:
        n = as_float(x)
        return _fail(f"field {f.name}: not numeric") if n is None else double_v(n)
    if f.int32 is not None:
        n = as_int(x)
        if n is None or not _INT32_MIN <= n <= _INT32_MAX:
            return _fail(f"field {f.name}: does not fit int32")
        return int32_v(n)
    if f.int64 is not None:
        n = as_int(x)
        return _fail(f"field {f.name}: not an integer") if n is None else int64_v(n)
    if f.uint32 is not None:
        n = as_uint(x)
        if n is None or n > _UINT32_MAX:
            return _fail(f"field {f.name}: does not fit uint32")
        return uint32_v(n)
    if f.uint64 is not None:
        n = as_uint(x)
        return _fail(f"field {f.name}: not an unsigned integer") if n is None else uint64_v(n)
    if f.bool is not None:
        return bool_v(x) if isinstance(x, bool) else _fail(f"field {f.name}: not bool")
    if f.string is not None:
        return str_v(x) if isinstance(x, str) else _fail(f"field {f.name}: not string")
    if f.bytes is not None:
        return bytes_v(x) if isinstance(x, bytes) else _fail(f"field {f.name}: not bytes")
    if f.choice is not None or f.json is not None:
        return from_native(x)
    if f.duration is not None:
        return (
            duration_v(x)
            if isinstance(x, dt.timedelta)
            else _fail(f"field {f.name}: not a duration")
        )
    if f.timestamp is not None:
        return (
            timestamp_v(x)
            if isinstance(x, dt.datetime)
            else _fail(f"field {f.name}: not a timestamp")
        )
    if f.list is not None:
        if not isinstance(x, list):
            return _fail(f"field {f.name}: not a list")
        items = f.list.items
        out: list[Value] = []
        for i, el in enumerate(x):
            item = items[0] if len(items) == 1 else (items[i] if i < len(items) else None)
            out.append(from_native(el) if item is None else canonical_value(item, el))
        return list_v(*out)
    if f.object is not None:
        if not isinstance(x, dict):
            return _fail(f"field {f.name}: not an object")
        sub = f.object.schema
        return from_native(x) if sub is None else canonical_struct(sub, x)
    if f.map is not None:
        if not isinstance(x, dict):
            return _fail(f"field {f.name}: not a map")
        vs = f.map.value_schema
        fields: dict[str, Value] = {}
        for key, el in x.items():
            if vs is not None and isinstance(el, dict):
                fields[key] = canonical_struct(vs, el)
            else:
                fields[key] = from_native(el)
        return struct_v(fields)
    # Computed / OneOf / Ref canonicalize structurally; the engine resolves
    # through their target schemas instead.
    return from_native(x)


def canonical_struct(s: Schema, m: NativeStruct) -> Value:
    fields: dict[str, Value] = {}
    by_name = {f.name: f for f in s.fields}
    for key, el in m.items():
        fld = by_name.get(key)
        fields[key] = from_native(el) if fld is None else canonical_value(fld, el)
    return struct_v(fields)
