"""The resolve pipeline, mirroring the Go reference compute.go."""

from __future__ import annotations

import base64
import binascii
import datetime as dt
from typing import TYPE_CHECKING, cast

from schemapb._gen.schemapb import (
    ErrorCode,
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


def resolve(e: Engine, values: NativeStruct) -> list[ValidationError]:
    """Defaults, coercion, normalize, computed — in place."""
    errs: list[ValidationError] = []
    tasks: list[tuple[SchemaField, NativeStruct, str]] = []
    _seed(e, e.schema, values, "", tasks, values, errs)
    _run_normalize(e, e.schema, values, values, errs)
    _run_compute(e, values, tasks, errs)
    return errs


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


def _seed(
    e: Engine,
    schema: Schema,
    scope: NativeStruct,
    prefix: str,
    tasks: list[tuple[SchemaField, NativeStruct, str]],
    root: NativeStruct,
    errs: list[ValidationError],
) -> None:
    coerce = schema.coerce
    for f in schema.fields:
        name = f.name
        path = join_path(prefix, name)
        if not field_is_active(e, f, root, path, errs):
            continue
        if coerce and name in scope:
            coerced = coerce_input(f, scope[name])
            if coerced is not None:
                scope[name] = coerced
        if f.immutable or name not in scope:
            dv = default_value(f)
            if dv is not None:
                scope[name] = dv

        cur = scope.get(name)
        if f.computed is not None:
            tasks.append((f, scope, path))
        elif f.object is not None and f.object.schema is not None and isinstance(cur, dict):
            _seed(e, f.object.schema, cur, path, tasks, root, errs)
        elif f.list is not None and len(f.list.items) >= 1 and isinstance(cur, list):
            for i, el in enumerate(cur):
                it = list_item_def(f.list, i)
                if (
                    it is not None
                    and it.object is not None
                    and it.object.schema is not None
                    and isinstance(el, dict)
                ):
                    _seed(e, it.object.schema, el, f"{path}[{i}]", tasks, root, errs)
        elif f.map is not None and f.map.value_schema is not None and isinstance(cur, dict):
            for k, el in cur.items():
                if isinstance(el, dict):
                    _seed(e, f.map.value_schema, el, join_path(path, k), tasks, root, errs)
        elif f.one_of is not None:
            sel = select_variant(f.one_of, cur)
            if sel is not None:
                _seed(e, sel[0], sel[1], path, tasks, root, errs)
        elif f.ref is not None:
            def_ = e.schema.defs.get(ref_def_key(f.ref))
            if def_ is not None and isinstance(cur, dict):
                _seed(e, def_, cur, path, tasks, root, errs)


def _run_normalize(
    e: Engine,
    schema: Schema,
    scope: NativeStruct,
    root: NativeStruct,
    errs: list[ValidationError],
) -> None:
    for f in schema.fields:
        name = f.name
        cur = scope.get(name)
        if cur is None:
            continue
        if not field_is_active(e, f, root, name, None):
            continue
        norm = f.normalize or ""
        if norm != "":
            value, err = e.eval(norm, {"this": cur, "root": root})
            if err is not None:
                errs.append(expr_err(name, norm, f"normalize: {err}"))
            else:
                scope[name] = value
                cur = value
        if f.object is not None and f.object.schema is not None and isinstance(cur, dict):
            _run_normalize(e, f.object.schema, cur, root, errs)
        elif f.list is not None and len(f.list.items) >= 1 and isinstance(cur, list):
            for i, el in enumerate(cur):
                it = list_item_def(f.list, i)
                if (
                    it is not None
                    and it.object is not None
                    and it.object.schema is not None
                    and isinstance(el, dict)
                ):
                    _run_normalize(e, it.object.schema, el, root, errs)
        elif f.map is not None and f.map.value_schema is not None and isinstance(cur, dict):
            for el in cur.values():
                if isinstance(el, dict):
                    _run_normalize(e, f.map.value_schema, el, root, errs)
        elif f.one_of is not None:
            sel = select_variant(f.one_of, cur)
            if sel is not None:
                _run_normalize(e, sel[0], sel[1], root, errs)
        elif f.ref is not None:
            def_ = e.schema.defs.get(ref_def_key(f.ref))
            if def_ is not None and isinstance(cur, dict):
                _run_normalize(e, def_, cur, root, errs)


def _run_compute(
    e: Engine,
    root: NativeStruct,
    tasks: list[tuple[SchemaField, NativeStruct, str]],
    errs: list[ValidationError],
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
        scope[f.name] = shaped


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
