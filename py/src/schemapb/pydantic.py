"""Reflect: a pydantic model -> a Schema.

The model's JSON shape is what travels, so pydantic's own vocabulary
becomes the schema: field types map to kinds, ``X | None`` to nullable,
a missing default to required, ``Field(ge=/le=/gt=/lt=/min_length=/
max_length=/pattern=/description=)`` to constraints, ``Literal[...]`` to
Choice, ``datetime``/``timedelta`` to Timestamp/Duration, nested models to
Objects (inheritance flattens), ``dict[str, V]`` to Map (``value_schema``
for models, ``value_field`` otherwise), model cycles to JSON.

Python's type system lacks sized numerics and a few schema notions, so
this module exports markers to close the gap: ``Int32``/``UInt32``/
``UInt64``/``Float32`` annotated ints/floats, ``exact_len(n)`` and
``fmt("email")`` metadata.

pydantic is an OPTIONAL dependency: ``pip install schemapb[pydantic]``.
Everything unrepresentable fails LOUDLY.
"""

from __future__ import annotations

import datetime as dt
import types
import typing
from dataclasses import dataclass
from typing import TYPE_CHECKING, Annotated, Any, Literal, get_args, get_origin

try:
    import pydantic as _pydantic
except ImportError as _exc:  # pragma: no cover - exercised only without the extra
    _MSG = "schemapb.pydantic requires the optional dependency: pip install schemapb[pydantic]"
    raise ImportError(_MSG) from _exc

from schemapb import builder as b
from schemapb.descriptor import _FIELD_NAME_RE

if TYPE_CHECKING:
    from collections.abc import Callable, Mapping

    from schemapb._gen.schemapb import Schema, SchemaField, SchemaIdentity

__all__ = [
    "Float32",
    "Int32",
    "UInt32",
    "UInt64",
    "exact_len",
    "fmt",
    "reflect",
]


@dataclass(frozen=True)
class _KindMarker:
    kind: str


@dataclass(frozen=True)
class _ExactLen:
    n: int


@dataclass(frozen=True)
class _Fmt:
    name: str


Int32 = Annotated[int, _KindMarker("int32")]
UInt32 = Annotated[int, _KindMarker("uint32")]
UInt64 = Annotated[int, _KindMarker("uint64")]
Float32 = Annotated[float, _KindMarker("float")]


def exact_len(n: int) -> _ExactLen:
    """Exact length constraint (string/bytes ``len``)."""
    return _ExactLen(n)


def fmt(name: str) -> _Fmt:
    """A core string format ("email", "uuid", ...)."""
    return _Fmt(name)


@dataclass
class _Constraints:
    ge: int | float | None = None
    gt: int | float | None = None
    le: int | float | None = None
    lt: int | float | None = None
    min_length: int | None = None
    max_length: int | None = None
    exact: int | None = None
    pattern: str | None = None
    fmt: str | None = None
    kind: str | None = None


def _collect(metadata: list[Any]) -> _Constraints:
    c = _Constraints()
    for m in metadata:
        if isinstance(m, _KindMarker):
            c.kind = m.kind
        elif isinstance(m, _ExactLen):
            c.exact = m.n
        elif isinstance(m, _Fmt):
            c.fmt = m.name
        else:
            # annotated_types.Ge/Gt/Le/Lt/MinLen/MaxLen and pydantic
            # StringConstraints all expose these attributes; duck-typing
            # keeps this stable across pydantic versions.
            for src, dst in (
                ("ge", "ge"),
                ("gt", "gt"),
                ("le", "le"),
                ("lt", "lt"),
                ("min_length", "min_length"),
                ("max_length", "max_length"),
                ("pattern", "pattern"),
            ):
                v = getattr(m, src, None)
                if v is not None:
                    setattr(c, dst, v)
    return c


def reflect(
    model: type[_pydantic.BaseModel],
    id_: SchemaIdentity,
    *,
    overrides: Mapping[type, Callable[[str], SchemaField]] | None = None,
) -> Schema:
    """Build a Schema from a pydantic model (coercion enabled on the root).

    ``overrides`` re-describes exact annotation types and binds BEFORE
    every default branch, stdlib types included.
    """
    r = _Reflector(dict(overrides or {}))
    fields = r.fields_of(model, set())
    schema, _ = b.new_schema(id_).coerce().fields(*fields).build()
    return schema


class _ReflectError(ValueError):
    def __init__(self, where: str, msg: str) -> None:
        super().__init__(f"schemapb: reflect: field {where}: {msg}")


class _Reflector:
    def __init__(self, overrides: dict[type, Callable[[str], SchemaField]]) -> None:
        self.overrides = overrides

    def fields_of(self, model: type[_pydantic.BaseModel], visited: set[type]) -> list[b.FieldB]:
        visited = visited | {model}
        out: list[b.FieldB] = []
        for pyname, info in model.model_fields.items():
            if info.exclude:
                continue
            name = info.alias or pyname
            if not _FIELD_NAME_RE.match(name):
                raise _ReflectError(pyname, f"name {name!r} is not a valid schemapb field name")
            fb = self.field_of(name, info.annotation, list(info.metadata), visited)
            if info.description:
                fb = fb.desc(info.description)
            if info.is_required():
                fb = fb.required()
            out.append(fb)
        return out

    def field_of(
        self,
        name: str,
        ann: Any,  # noqa: ANN401 - annotations are typing objects
        metadata: list[Any],
        visited: set[type],
    ) -> b.FieldB:
        nullable = False
        ann, opt = _unwrap_optional(ann)
        if opt:
            nullable = True
        while get_origin(ann) is Annotated:
            base, *extra = get_args(ann)
            metadata = [*metadata, *extra]
            ann = base

        c = _collect(metadata)
        fb = self._base_field(name, ann, c, visited)
        if nullable:
            fb = fb.nullable()
        return fb

    def _base_field(  # noqa: C901, PLR0911, PLR0912 - flat exhaustive dispatch
        self,
        name: str,
        ann: Any,  # noqa: ANN401 - annotations are typing objects
        c: _Constraints,
        visited: set[type],
    ) -> b.FieldB:
        if isinstance(ann, type) and ann in self.overrides:
            return _Prebuilt(self.overrides[ann](name))
        if ann is dt.datetime:
            return b.timestamp(name)
        if ann is dt.timedelta:
            return b.duration(name)
        if ann is Any or ann is object:
            return b.json_(name)
        if get_origin(ann) is Literal:
            return _choice_of(name, get_args(ann))
        if isinstance(ann, type) and issubclass(ann, _pydantic.BaseModel):
            if ann in visited:
                return b.json_(name)  # a cycle: the shape is not finite
            return b.object_(name, *self.fields_of(ann, visited))
        origin = get_origin(ann)
        if origin in (list, tuple, set, frozenset):
            return self._seq_field(name, ann, origin, c, visited)
        if origin is dict:
            return self._map_field(name, ann, visited)
        if ann is str:
            return _string_field(name, c)
        if ann is bytes:
            return _bytes_field(name, c)
        if ann is bool:
            return b.bool_(name)
        if ann is int:
            return _int_field(name, c)
        if ann is float:
            return _float_field(name, c)
        raise _ReflectError(name, f"unsupported annotation {ann!r}")

    def _seq_field(
        self,
        name: str,
        ann: Any,  # noqa: ANN401 - annotations are typing objects
        origin: type,
        c: _Constraints,
        visited: set[type],
    ) -> b.FieldB:
        args = get_args(ann)
        if origin in (set, frozenset):
            raise _ReflectError(name, "sets have no JSON shape; use a list")
        if origin is tuple and len(args) > 1 and args[-1] is not Ellipsis:
            if len(set(args)) > 1:
                # A genuinely heterogeneous tuple: a fixed-shape tuple list.
                items = [self.field_of("item", a, [], visited).required() for a in args]
                return b.list_(name, *items)
            n = len(args)
            item = self.field_of("item", args[0], [], visited).required()
            return b.list_(name, item).min_items(n).max_items(n)
        elem = args[0] if args else Any
        item = self.field_of("item", elem, [], visited).required()
        lb = b.list_(name, item)
        if c.min_length is not None:
            lb = lb.min_items(c.min_length)
        if c.max_length is not None:
            lb = lb.max_items(c.max_length)
        return lb

    def _map_field(self, name: str, ann: Any, visited: set[type]) -> b.FieldB:  # noqa: ANN401
        key, val = get_args(ann) or (str, Any)
        if key is not str:
            raise _ReflectError(name, f"map key {key!r} is not str (JSON object keys are strings)")
        base_val, _ = _unwrap_optional(val)
        if (
            isinstance(base_val, type)
            and issubclass(base_val, _pydantic.BaseModel)
            and base_val not in visited
            and base_val not in self.overrides
        ):
            return b.map_(name, *self.fields_of(base_val, visited))
        return b.map_of(name, self.field_of("value", val, [], visited).required())


class _Prebuilt(b.FieldB):
    """Wraps an override-produced field as a builder."""

    def __init__(self, f: SchemaField) -> None:
        super().__init__(f.name)
        self.f = f


def _unwrap_optional(ann: Any) -> tuple[Any, bool]:  # noqa: ANN401 - typing objects
    origin = get_origin(ann)
    if origin in (typing.Union, types.UnionType):
        args = [a for a in get_args(ann) if a is not type(None)]
        if len(args) == 1 and len(get_args(ann)) == 2:
            return args[0], True
        msg = f"schemapb: reflect: unsupported union {ann!r} (only X | None)"
        raise ValueError(msg)
    return ann, False


def _choice_of(name: str, options: tuple[Any, ...]) -> b.FieldB:
    if all(isinstance(o, str) for o in options):
        return b.choice(name).str_opts(*options)
    if all(isinstance(o, int) and not isinstance(o, bool) for o in options):
        return b.choice(name).int_opts(*options)
    raise _ReflectError(name, "Literal options must be all-str or all-int")


def _string_field(name: str, c: _Constraints) -> b.FieldB:
    if c.kind is not None:
        raise _ReflectError(name, f"kind marker {c.kind!r} does not apply to str")
    sb = b.str_(name)
    if c.min_length is not None:
        sb = sb.min_len(c.min_length)
    if c.max_length is not None:
        sb = sb.max_len(c.max_length)
    if c.exact is not None:
        sb = sb.len(c.exact)
    if c.pattern is not None:
        sb = sb.pattern(c.pattern)
    if c.fmt is not None:
        sb = sb.format(c.fmt)
    return sb


def _bytes_field(name: str, c: _Constraints) -> b.FieldB:
    bb = b.bytes_(name)
    if c.min_length is not None:
        bb = bb.min_len(c.min_length)
    if c.max_length is not None:
        bb = bb.max_len(c.max_length)
    if c.exact is not None:
        bb = bb.len(c.exact)
    return bb


def _int_field(name: str, c: _Constraints) -> b.FieldB:
    builders = {
        None: b.int64,
        "int64": b.int64,
        "int32": b.int32,
        "uint32": b.uint32,
        "uint64": b.uint64,
    }
    mk = builders.get(c.kind)
    if mk is None:
        raise _ReflectError(name, f"kind marker {c.kind!r} does not apply to int")
    nb = mk(name)
    return _apply_bounds(nb, c)


def _float_field(name: str, c: _Constraints) -> b.FieldB:
    builders = {None: b.double, "double": b.double, "float": b.float_}
    mk = builders.get(c.kind)
    if mk is None:
        raise _ReflectError(name, f"kind marker {c.kind!r} does not apply to float")
    nb = mk(name)
    return _apply_bounds(nb, c)


def _apply_bounds(nb: b.NumB, c: _Constraints) -> b.NumB:
    if c.ge is not None:
        nb = nb.gte(c.ge)
    if c.gt is not None:
        nb = nb.gt(c.gt)
    if c.le is not None:
        nb = nb.lte(c.le)
    if c.lt is not None:
        nb = nb.lt(c.lt)
    return nb
