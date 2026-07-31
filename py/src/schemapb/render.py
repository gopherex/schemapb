"""Display formatting (the spec's display forms) and the contract render
context; rendering itself lives in bake.py (chevron).
"""

from __future__ import annotations

import base64
import datetime as dt
import json
from typing import TYPE_CHECKING, Any, TypedDict

from schemapb.duration import format_go_duration, format_rfc3339
from schemapb.value import Native, NativeStruct, as_float, as_int, to_native

if TYPE_CHECKING:
    from schemapb._gen.schemapb import SchemaField


def display_string(v: Native) -> str:
    if v is None:
        return ""
    if isinstance(v, str):
        return v
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, float):
        return _json_number(v)
    if isinstance(v, bytes):
        return base64.b64encode(v).decode("ascii")
    if isinstance(v, dt.timedelta):
        return format_go_duration(v)
    if isinstance(v, dt.datetime):
        return format_rfc3339(v)
    return json.dumps(_display_json(v), separators=(",", ":"), ensure_ascii=False)


def _json_number(v: float) -> str:
    # Match JSON/Go shortest form: integral floats render without ".0"?
    # encoding/json renders 2.0 as "2"; Python json gives "2.0" — align.
    if v == int(v) and abs(v) < 1e15:
        return str(int(v))
    return json.dumps(v)


def _display_json(v: Native) -> Any:  # noqa: ANN401 - JSON-shaped output
    if v is None or isinstance(v, (bool, str)):
        return v
    if isinstance(v, int):
        return v if -(2**53) < v < 2**53 else str(v)
    if isinstance(v, float):
        return int(v) if v == int(v) and abs(v) < 1e15 else v
    if isinstance(v, (bytes, dt.timedelta, dt.datetime)):
        return display_string(v)
    if isinstance(v, list):
        return [_display_json(el) for el in v]
    return {k: _display_json(v[k]) for k in sorted(v)}


def ascii_upper(s: str) -> str:
    return "".join(chr(ord(c) - 32) if "a" <= c <= "z" else c for c in s)


def ascii_lower(s: str) -> str:
    return "".join(chr(ord(c) + 32) if "A" <= c <= "Z" else c for c in s)


def go_quote(s: str) -> str:
    return json.dumps(s, ensure_ascii=False)


_KIND_NAMES = (
    "float",
    "double",
    "int32",
    "int64",
    "uint32",
    "uint64",
    "bool",
    "string",
    "bytes",
    "choice",
    "duration",
    "timestamp",
    "list",
    "object",
    "map",
    "one_of",
    "ref",
    "computed",
    "json",
)


def kind_name(f: SchemaField) -> str:
    for attr in _KIND_NAMES:
        if getattr(f, attr) is not None:
            return "oneof" if attr == "one_of" else attr
    return ""


class RenderField(TypedDict):
    name: str
    title: str
    description: str
    unit: str
    group: str
    kind: str
    label: str
    set: bool
    computed: bool
    secret: bool
    immutable: bool
    deprecated: bool
    value: str
    onoff: str
    yesno: str
    quoted: str
    upper: str
    lower: str


def native_equals(a: Native, b: Native) -> bool:
    """Structural equality with cross-numeric comparison (spec)."""
    if isinstance(a, bool) or isinstance(b, bool):
        return isinstance(a, bool) and isinstance(b, bool) and a == b
    an, bn = _numeric(a), _numeric(b)
    if an is not None or bn is not None:
        return an is not None and bn is not None and an == bn
    if isinstance(a, list) and isinstance(b, list):
        return len(a) == len(b) and all(native_equals(x, y) for x, y in zip(a, b, strict=True))
    if isinstance(a, dict) and isinstance(b, dict):
        return a.keys() == b.keys() and all(native_equals(a[k], b[k]) for k in a)
    return type(a) is type(b) and a == b


def _numeric(v: Native) -> int | float | None:
    if isinstance(v, bool) or not isinstance(v, (int, float)):
        return None
    i = as_int(v)
    return i if i is not None else as_float(v)


def render_field(f: SchemaField, values: NativeStruct) -> RenderField:
    val = values.get(f.name)
    is_set = f.name in values and val is not None
    display = display_string(val) if is_set else ""
    label = ""
    if f.choice is not None and is_set:
        for o in f.choice.options:
            if native_equals(val, to_native(o.value)):
                label = o.label
                break
    b = val if isinstance(val, bool) else False
    return RenderField(
        name=f.name,
        title=f.title or "",
        description=f.description or "",
        unit=f.unit or "",
        group=f.group or "",
        kind=kind_name(f),
        label=label,
        set=is_set,
        computed=f.computed is not None,
        secret=f.secret,
        immutable=f.immutable,
        deprecated=f.deprecated,
        value=display,
        onoff="on" if b else "off",
        yesno="yes" if b else "no",
        quoted=go_quote(display),
        upper=ascii_upper(display),
        lower=ascii_lower(display),
    )
