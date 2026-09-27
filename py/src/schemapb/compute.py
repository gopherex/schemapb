"""The resolve pipeline, mirroring the Go reference compute.go."""

from __future__ import annotations

import base64
import binascii
import datetime as dt
from collections.abc import Callable
from functools import partial
from typing import TYPE_CHECKING, cast

from schemapb._gen.schemapb import (
    ErrorCode,
    ResolveEvent,
    ResolveOperation,
    ResolveReport,
    Schema,
    SchemaField,
    SchemaFieldList,
    SchemaFieldOneOf,
    SchemaFieldRef,
    SchemaFieldResultType,
    SchemaFieldSeverity,
    ValidationError,
)
from schemapb.descriptor import join_path, schema_err
from schemapb.duration import parse_go_duration, parse_rfc3339
from schemapb.path import path_segments
from schemapb.render import native_equals
from schemapb.value import (
    Native,
    NativeStruct,
    as_float,
    as_int,
    as_uint,
    to_native,
)

if TYPE_CHECKING:
    from schemapb.engine import Engine


def expr_err(path: str, expr: str, msg: str) -> ValidationError:
    return ValidationError(
        path=path,
        code=ErrorCode.EXPR_ERROR,
        expr=expr,
        severity=SchemaFieldSeverity.ERROR,
        message=msg,
    )


def ref_def_key(ref: SchemaFieldRef) -> str:
    if ref.id is not None:
        return f"{ref.id.namespace}\x00{ref.id.name}\x00{ref.id.version}"
    return ref.name or ""


def is_tuple(l: SchemaFieldList) -> bool:  # noqa: E741
    return len(l.items) > 1


def list_item_def(l: SchemaFieldList, i: int) -> SchemaField | None:  # noqa: E741
    if len(l.items) == 1:
        return l.items[0]
    return l.items[i] if i < len(l.items) else None


def select_variant(
    oo: SchemaFieldOneOf,
    val: Native,
) -> tuple[Schema, NativeStruct] | None:
    if not isinstance(val, dict):
        return None
    disc = val.get(oo.discriminator)
    if not isinstance(disc, str) or disc == "":
        return None
    variant = oo.variants.get(disc)
    return None if variant is None else (variant, val)


def object_schema(
    f: SchemaField,
    val: Native,
    defs: dict[str, Schema],
) -> tuple[Schema, NativeStruct] | None:
    """Resolve present Object/Ref/OneOf scopes, including list/tuple items."""
    if not isinstance(val, dict):
        return None
    if f.one_of is not None:
        return select_variant(f.one_of, val)
    sub = None
    if f.object is not None:
        sub = f.object.schema
    elif f.ref is not None:
        sub = defs.get(ref_def_key(f.ref))
    return None if sub is None else (sub, val)


def resolve(
    e: Engine, values: NativeStruct, report: ResolveReport | None = None
) -> list[ValidationError]:
    state = _Resolver(e, values, report)
    state.object(e.schema, values, "", inherited=False, normalize=False)
    state.object(e.schema, values, "", inherited=False, normalize=True)
    _run_compute(e, values, state.tasks, state.errs, report)
    for err in state.errs:
        err.path_segments = path_segments(err.path)
    return state.errs


def field_is_active(
    e: Engine,
    f: SchemaField,
    root: NativeStruct,
    path: str,
    errs: list[ValidationError] | None,
) -> bool:
    when = f.when or ""
    if when == "":
        return True
    ok, err = e.eval_bool(when, {"root": root})
    if err is not None:
        if errs is not None:
            errs.append(expr_err(path, when, f"when: {err}"))
        return False
    return ok


class _Resolver:
    def __init__(self, engine: Engine, root: NativeStruct, report: ResolveReport | None) -> None:
        self.engine = engine
        self.root = root
        self.report = report
        self.errs: list[ValidationError] = []
        self.tasks: list[tuple[SchemaField, Callable[[Native], None], str]] = []
        self.gates: dict[str, bool] = {}
        self.gate_errors: set[tuple[str, str | None]] = set()
        self.inactive: set[str] = set()

    def object(
        self, schema: Schema, scope: NativeStruct, path: str, inherited: bool, normalize: bool
    ) -> None:
        coerce = inherited or schema.coerce
        for f in schema.fields:
            self.field(
                f,
                scope.get(f.name),
                f.name in scope,
                partial(scope.__setitem__, f.name),
                join_path(path, f.name),
                coerce,
                normalize,
            )

    def field(
        self,
        f: SchemaField,
        cur: Native,
        present: bool,
        setter: Callable[[Native], None],
        path: str,
        coerce: bool,
        normalize: bool,
    ) -> None:
        seeded = self.gates.get(path, False)
        if not self.active(f, path):
            return
        if normalize and not seeded:

            def seed_setter(value: Native) -> None:
                nonlocal cur, present
                cur, present = value, True
                setter(value)

            self.field(f, cur, present, seed_setter, path, coerce, normalize=False)
        if not normalize:
            if present and coerce:
                out = coerce_input(f, cur)
                if out is not None:
                    cur = out
                    setter(cur)
                    _record(self.report, path, ResolveOperation.COERCED)
            if not present or f.immutable:
                out = default_value(f)
                if out is not None:
                    changed = not present or not native_equals(cur, out)
                    cur, present = out, True
                    setter(cur)
                    if changed:
                        _record(self.report, path, ResolveOperation.DEFAULT_APPLIED)
        else:
            if present and cur is not None and f.normalize:
                out, err = self.engine.eval(f.normalize, {"this": cur, "root": self.root})
                if err is not None:
                    self.errs.append(expr_err(path, f.normalize, f"normalize: {err}"))
                else:
                    changed = not native_equals(cur, out)
                    cur = out
                    setter(cur)
                    if changed:
                        _record(self.report, path, ResolveOperation.NORMALIZED)
                        self.children(f, cur, path, coerce, normalize=False)
            if f.computed is not None:
                self.tasks.append((f, setter, path))
                return
        if present and cur is not None:
            self.children(f, cur, path, coerce, normalize)

    def active(self, f: SchemaField, path: str) -> bool:
        errors: list[ValidationError] = []
        active = field_is_active(self.engine, f, self.root, path, errors)
        key = (path, f.when)
        if errors and key not in self.gate_errors:
            self.errs.extend(errors)
            self.gate_errors.add(key)
        self.gates[path] = active
        if not active and path not in self.inactive:
            _record(self.report, path, ResolveOperation.INACTIVE)
            self.inactive.add(path)
        return active

    def children(
        self, f: SchemaField, cur: Native, path: str, coerce: bool, normalize: bool
    ) -> None:
        sub = object_schema(f, cur, self.engine.schema.defs)
        if sub is not None:
            self.object(sub[0], sub[1], path, coerce, normalize)
            return
        if f.list is not None and isinstance(cur, list):
            for i, value in enumerate(cur):
                item = list_item_def(f.list, i)
                if item is not None:
                    self.field(
                        item,
                        value,
                        True,  # noqa: FBT003 - every existing collection slot is present
                        partial(cur.__setitem__, i),
                        f"{path}[{i}]",
                        coerce,
                        normalize,
                    )
        if f.map is not None and isinstance(cur, dict):
            for key in sorted(cur):
                value, child_path = cur[key], join_path(path, key)
                if f.map.value_field is not None:
                    self.field(
                        f.map.value_field,
                        value,
                        True,  # noqa: FBT003 - every existing collection slot is present
                        partial(cur.__setitem__, key),
                        child_path,
                        coerce,
                        normalize,
                    )
                elif f.map.value_schema is not None and isinstance(value, dict):
                    self.object(f.map.value_schema, value, child_path, coerce, normalize)


def _record(report: ResolveReport | None, path: str, operation: ResolveOperation) -> None:
    if report is not None:
        report.events.append(
            ResolveEvent(path=path, path_segments=path_segments(path), operation=operation)
        )


def _run_compute(
    e: Engine,
    root: NativeStruct,
    tasks: list[tuple[SchemaField, Callable[[Native], None], str]],
    errs: list[ValidationError],
    report: ResolveReport | None = None,
) -> None:
    if not tasks:
        return
    by_path = {path: (f, scope) for f, scope, path in tasks}
    deps: dict[str, list[str]] = {}
    for f, _scope, path in tasks:
        assert f.computed is not None  # noqa: S101 - collected as computed
        deps[path] = [d for d in e.expr_deps(f.computed.expr) if d != path and d in by_path]

    color: dict[str, int] = {}
    order: list[str] = []

    def visit(p: str) -> bool:
        c = color.get(p, 0)
        if c == 1:
            return False
        if c == 2:
            return True
        color[p] = 1
        for d in deps.get(p, []):
            if not visit(d):
                return False
        color[p] = 2
        order.append(p)
        return True

    for _f, _scope, path in tasks:
        if color.get(path) != 2 and not visit(path):
            errs.append(schema_err(path, "computed field cycle"))

    for path in order:
        f, scope = by_path[path]
        assert f.computed is not None  # noqa: S101
        value, err = e.eval(f.computed.expr, {"root": root})
        if err is not None:
            errs.append(expr_err(path, f.computed.expr, f"compute: {err}"))
            continue
        shaped = shape_result(f.computed.result, value)
        if shaped is _MISMATCH:
            errs.append(
                expr_err(path, f.computed.expr, "compute: result does not match declared type")
            )
            continue
        scope(shaped)
        _record(report, path, ResolveOperation.COMPUTED)


_MISMATCH = object()


def shape_result(rt: SchemaFieldResultType | None, x: Native) -> Native:
    if x is None:
        return None
    match rt:
        case None | SchemaFieldResultType.UNSPECIFIED | SchemaFieldResultType.JSON:
            return x
        case SchemaFieldResultType.DOUBLE:
            n = as_float(x)
            return n if n is not None else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.INT64:
            n = as_int(x)
            return n if n is not None else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.UINT64:
            n = as_uint(x)
            return n if n is not None else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.BOOL:
            return x if isinstance(x, bool) else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.STRING:
            return x if isinstance(x, str) else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.DURATION:
            return x if isinstance(x, dt.timedelta) else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.TIMESTAMP:
            return x if isinstance(x, dt.datetime) else _MISMATCH  # type: ignore[return-value]
        case SchemaFieldResultType.BYTES:
            return x if isinstance(x, bytes) else _MISMATCH  # type: ignore[return-value]
        case _:
            return _MISMATCH  # type: ignore[return-value]


def coerce_input(f: SchemaField, val: Native) -> Native | None:
    if not isinstance(val, str):
        return None
    if f.int32 is not None or f.int64 is not None:
        try:
            return int(val, 10)
        except ValueError:
            return None
    if f.uint32 is not None or f.uint64 is not None:
        if val.startswith("-"):
            return None
        try:
            return int(val, 10)
        except ValueError:
            return None
    if f.float is not None or f.double is not None:
        try:
            return float(val)
        except ValueError:
            return None
    if f.bool is not None:
        return True if val == "true" else False if val == "false" else None
    if f.bytes is not None:
        try:
            return base64.b64decode(val, validate=True)
        except binascii.Error:
            return None
    if f.duration is not None:
        return parse_go_duration(val)
    if f.timestamp is not None:
        return parse_rfc3339(val)
    return None


def default_value(f: SchemaField) -> Native | None:
    if f.object is not None and f.object.default is not None:
        return {}
    if f.ref is not None and f.ref.default is not None:
        return {}
    if f.float is not None and f.float.default is not None:
        return cast("Native", f.float.default)
    if f.double is not None and f.double.default is not None:
        return cast("Native", f.double.default)
    if f.int32 is not None and f.int32.default is not None:
        return cast("Native", f.int32.default)
    if f.int64 is not None and f.int64.default is not None:
        return cast("Native", f.int64.default)
    if f.uint32 is not None and f.uint32.default is not None:
        return cast("Native", f.uint32.default)
    if f.uint64 is not None and f.uint64.default is not None:
        return cast("Native", f.uint64.default)
    if f.bool is not None and f.bool.default is not None:
        return cast("Native", f.bool.default)
    if f.string is not None and f.string.default is not None:
        return cast("Native", f.string.default)
    if f.bytes is not None and f.bytes.default:
        return cast("Native", f.bytes.default)
    if f.choice is not None and f.choice.default is not None:
        return to_native(f.choice.default)
    if f.duration is not None and f.duration.default is not None:
        return cast("Native", f.duration.default)
    if f.timestamp is not None and f.timestamp.default is not None:
        return cast("Native", f.timestamp.default)
    if f.json is not None and f.json.default is not None:
        return to_native(f.json.default)
    return None


def choice_options(e: Engine, name: str, root: NativeStruct) -> list[Native] | None:
    f = next((x for x in e.schema.fields if x.name == name), None)
    if f is None or f.choice is None:
        return None
    src = f.choice.options_expr or ""
    if src == "":
        return [to_native(o.value) for o in f.choice.options]
    value, err = e.eval(src, {"root": root})
    return value if err is None and isinstance(value, list) else None


def list_count(e: Engine, name: str, root: NativeStruct) -> int | None:
    f = next((x for x in e.schema.fields if x.name == name), None)
    if f is None or f.list is None:
        return None
    ce = f.list.count_expr or ""
    if ce == "":
        return None
    value, err = e.eval(ce, {"root": root})
    if err is not None:
        return None
    n = as_int(value)
    return n if n is not None and n >= 0 else None
