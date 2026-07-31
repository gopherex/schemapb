"""The validation engine, mirroring the Go reference validate.go: identical
codes, deterministic order, typed expected/actual, secret masking.
"""

from __future__ import annotations

import datetime as dt
import json
from typing import TYPE_CHECKING

from schemapb._gen.schemapb import (
    ErrorCode,
    Schema,
    SchemaField,
    SchemaFieldBytes,
    SchemaFieldChoice,
    SchemaFieldDuration,
    SchemaFieldList,
    SchemaFieldMap,
    SchemaFieldOneOf,
    SchemaFieldRef,
    SchemaFieldRule,
    SchemaFieldSeverity,
    SchemaFieldString,
    SchemaFieldTimestamp,
    ValidationError,
    ValidationResult,
    Value,
)
from schemapb.compute import (
    default_value,
    expr_err,
    field_is_active,
    is_tuple,
    list_item_def,
    ref_def_key,
    resolve,
)
from schemapb.descriptor import join_path
from schemapb.duration import parse_go_duration, parse_rfc3339
from schemapb.messages import MESSAGE_TEMPLATES, render_message
from schemapb.render import display_string, native_equals
from schemapb.typed import Format
from schemapb.value import (
    CanonicalError,
    Native,
    NativeStruct,
    as_float,
    as_int,
    as_uint,
    bytes_v,
    canonical_value,
    double_v,
    duration_v,
    from_native,
    int64_v,
    list_v,
    null_v,
    str_v,
    timestamp_v,
    to_native,
    uint64_v,
)

if TYPE_CHECKING:
    from schemapb.engine import Engine

_INT32 = (-(2**31), 2**31 - 1)
_INT64 = (-(2**63), 2**63 - 1)
_UINT32_MAX = 2**32 - 1
_UINT64_MAX = 2**64 - 1


def validate(e: Engine, values: NativeStruct) -> ValidationResult:
    errs: list[ValidationError] = []
    _check_immutable(e, e.schema.fields, values, "", values, errs)
    errs.extend(resolve(e, values))
    _validate_fields(e, e.schema, values, values, "", errs)
    for r in e.schema.rules:
        _eval_rule(e, r, r.id or "", None, values, None, errs)
    return ValidationResult(errors=errs)


def result_ok(r: ValidationResult) -> bool:
    return len(r.errors) == 0


def result_blocking(r: ValidationResult) -> bool:
    return any(e.severity != SchemaFieldSeverity.WARNING for e in r.errors)


def _verr(
    path: str,
    code: ErrorCode,
    constraint: str,
    expected: Value | None,
    actual: Value | None,
) -> ValidationError:
    return ValidationError(
        path=path,
        code=code,
        constraint=constraint,
        expected=expected,
        actual=actual,
        severity=SchemaFieldSeverity.ERROR,
        message=render_message(code, expected, actual),
    )


def _type_err(path: str, want: str, val: Native) -> ValidationError:
    return _verr(path, ErrorCode.TYPE_MISMATCH, "", str_v(want), from_native(val))


def _mask(errs: list[ValidationError], *, secret: bool) -> list[ValidationError]:
    if not secret:
        return errs
    for e in errs:
        e.actual = None
        if e.code in MESSAGE_TEMPLATES:
            e.message = render_message(e.code, e.expected, None)
    return errs


def _check_immutable(
    e: Engine,
    fields: list[SchemaField],
    scope: NativeStruct,
    prefix: str,
    root: NativeStruct,
    errs: list[ValidationError],
) -> None:
    for f in fields:
        path = join_path(prefix, f.name)
        if not field_is_active(e, f, root, path, None):
            continue
        if f.immutable:
            if f.name in scope:
                dv = default_value(f)
                cur = scope[f.name]
                if dv is not None and not native_equals(cur, dv):
                    try:
                        expected = canonical_value(f, dv)
                    except CanonicalError:
                        expected = from_native(dv)
                    err = _verr(
                        path,
                        ErrorCode.IMMUTABLE_MODIFIED,
                        "immutable",
                        expected,
                        from_native(cur),
                    )
                    errs.extend(_mask([err], secret=f.secret))
            continue
        cur = scope.get(f.name)
        if f.object is not None and f.object.schema is not None and isinstance(cur, dict):
            _check_immutable(e, f.object.schema.fields, cur, path, root, errs)
        if f.list is not None and len(f.list.items) >= 1 and isinstance(cur, list):
            for i, el in enumerate(cur):
                it = list_item_def(f.list, i)
                if (
                    it is not None
                    and it.object is not None
                    and it.object.schema is not None
                    and isinstance(el, dict)
                ):
                    _check_immutable(e, it.object.schema.fields, el, f"{path}[{i}]", root, errs)
        if f.map is not None and f.map.value_schema is not None and isinstance(cur, dict):
            for k in sorted(cur):
                el = cur[k]
                if isinstance(el, dict):
                    _check_immutable(
                        e, f.map.value_schema.fields, el, join_path(path, k), root, errs
                    )


def _validate_fields(
    e: Engine,
    schema: Schema,
    scope: NativeStruct,
    root: NativeStruct,
    prefix: str,
    errs: list[ValidationError],
) -> None:
    inactive: set[str] = set()
    declared: set[str] = set()
    for f in schema.fields:
        declared.add(f.name)
        if (f.when or "") != "" and not field_is_active(e, f, root, f.name, None):
            inactive.add(f.name)

    if schema.strict:
        errs.extend(
            _verr(
                join_path(prefix, key),
                ErrorCode.UNKNOWN_FIELD,
                "strict",
                None,
                from_native(scope[key]),
            )
            for key in sorted(scope)
            if key not in declared
        )

    present = sum(1 for key in scope if key not in inactive)
    if schema.min_properties is not None and present < schema.min_properties:
        errs.append(
            _verr(
                prefix,
                ErrorCode.MIN_PROPERTIES_VIOLATED,
                "min_properties",
                uint64_v(schema.min_properties),
                uint64_v(present),
            ),
        )
    if schema.max_properties is not None and present > schema.max_properties:
        errs.append(
            _verr(
                prefix,
                ErrorCode.MAX_PROPERTIES_VIOLATED,
                "max_properties",
                uint64_v(schema.max_properties),
                uint64_v(present),
            ),
        )

    for f in schema.fields:
        if f.name in inactive:
            continue
        _validate_one(
            e,
            f,
            scope.get(f.name),
            f.name in scope,
            join_path(prefix, f.name),
            root,
            None,
            errs,
        )


def _validate_one(
    e: Engine,
    f: SchemaField,
    val: Native,
    exists: bool,
    path: str,
    root: NativeStruct,
    index: int | None,
    errs: list[ValidationError],
) -> None:
    if not exists:
        if f.required:
            errs.append(_verr(path, ErrorCode.REQUIRED_MISSING, "required", None, None))
        return
    if val is None:
        if f.required:
            errs.append(_verr(path, ErrorCode.REQUIRED_MISSING, "required", None, None))
        elif not f.nullable:
            errs.append(_verr(path, ErrorCode.NOT_NULLABLE, "nullable", None, null_v()))
        return

    errs.extend(_mask(_check_kind(e, f, val, path, root), secret=f.secret))
    for r in f.rules:
        _eval_rule(e, r, path, val, root, index, errs)


def _eval_rule(
    e: Engine,
    r: SchemaFieldRule,
    path: str,
    this_val: Native,
    root: NativeStruct,
    index: int | None,
    errs: list[ValidationError],
) -> None:
    vars_: dict[str, Native] = {"this": this_val, "root": root}
    if index is not None:
        vars_["index"] = index
    value, err = e.eval(r.expr, vars_)
    if err is not None:
        ve = expr_err(path, r.expr, f"rule: {err}")
        ve.rule_id = r.id
        errs.append(ve)
        return
    if value is True:
        return
    sev = (
        SchemaFieldSeverity.ERROR
        if r.severity in (None, SchemaFieldSeverity.UNSPECIFIED)
        else r.severity
    )
    errs.append(
        ValidationError(
            path=path,
            code=ErrorCode.RULE_VIOLATED,
            expr=r.expr,
            rule_id=r.id,
            severity=sev,
            message=r.message,
        ),
    )


def _check_kind(
    e: Engine,
    f: SchemaField,
    val: Native,
    path: str,
    root: NativeStruct,
) -> list[ValidationError]:
    if f.float is not None:
        k = f.float
        return _check_number(
            path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multiple_of, k.in_, k.not_in
        )
    if f.double is not None:
        k = f.double
        return _check_number(
            path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multiple_of, k.in_, k.not_in
        )
    if f.int32 is not None:
        k = f.int32
        return _check_int(
            path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multiple_of, k.in_, k.not_in, *_INT32
        )
    if f.int64 is not None:
        k = f.int64
        return _check_int(
            path, val, k.const, k.gt, k.gte, k.lt, k.lte, k.multiple_of, k.in_, k.not_in, *_INT64
        )
    if f.uint32 is not None:
        k = f.uint32
        return _check_uint(
            path,
            val,
            k.const,
            k.gt,
            k.gte,
            k.lt,
            k.lte,
            k.multiple_of,
            k.in_,
            k.not_in,
            _UINT32_MAX,
        )
    if f.uint64 is not None:
        k = f.uint64
        return _check_uint(
            path,
            val,
            k.const,
            k.gt,
            k.gte,
            k.lt,
            k.lte,
            k.multiple_of,
            k.in_,
            k.not_in,
            _UINT64_MAX,
        )
    if f.bool is not None:
        if not isinstance(val, bool):
            return [_type_err(path, "bool", val)]
        if f.bool.const is not None and val is not f.bool.const:
            return [
                _verr(
                    path,
                    ErrorCode.CONST_MISMATCH,
                    "const",
                    from_native(f.bool.const),
                    from_native(val),
                ),
            ]
        return []
    if f.string is not None:
        if not isinstance(val, str):
            return [_type_err(path, "string", val)]
        return _check_string(e, path, val, f.string)
    if f.bytes is not None:
        if not isinstance(val, bytes):
            return [_type_err(path, "bytes", val)]
        return _check_bytes(path, val, f.bytes)
    if f.choice is not None:
        return _check_choice(e, path, val, f.choice, root)
    if f.duration is not None:
        return _check_duration(path, val, f.duration)
    if f.timestamp is not None:
        return _check_timestamp(path, val, f.timestamp)
    if f.list is not None:
        if not isinstance(val, list):
            return [_type_err(path, "list", val)]
        return _check_list(e, path, val, f.list, root)
    if f.object is not None:
        if not isinstance(val, dict):
            return [_type_err(path, "object", val)]
        sub: list[ValidationError] = []
        if f.object.schema is not None:
            _validate_fields(e, f.object.schema, val, root, path, sub)
            for r in f.object.schema.rules:
                _eval_rule(e, r, path, val, root, None, sub)
        return sub
    if f.map is not None:
        if not isinstance(val, dict):
            return [_type_err(path, "map", val)]
        return _check_map(e, path, val, f.map, root)
    if f.one_of is not None:
        if not isinstance(val, dict):
            return [_type_err(path, "object", val)]
        return _check_one_of(e, path, val, f.one_of, root)
    if f.ref is not None:
        return _check_ref(e, path, val, f.ref, root)
    return []  # computed / json


def _check_number(
    path: str,
    val: Native,
    cst: float | None,
    gt: float | None,
    gte: float | None,
    lt: float | None,
    lte: float | None,
    mul: float | None,
    in_set: list[float],
    not_in: list[float],
) -> list[ValidationError]:
    n = as_float(val)
    if n is None:
        return [_type_err(path, "number", val)]
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, expected: Value) -> None:
        out.append(_verr(path, code, constraint, expected, double_v(n)))

    if cst is not None and n != cst:
        add(ErrorCode.CONST_MISMATCH, "const", double_v(cst))
    if gt is not None and n <= gt:
        add(ErrorCode.GT_VIOLATED, "gt", double_v(gt))
    if gte is not None and n < gte:
        add(ErrorCode.GTE_VIOLATED, "gte", double_v(gte))
    if lt is not None and n >= lt:
        add(ErrorCode.LT_VIOLATED, "lt", double_v(lt))
    if lte is not None and n > lte:
        add(ErrorCode.LTE_VIOLATED, "lte", double_v(lte))
    if in_set and n not in in_set:
        add(ErrorCode.NOT_IN_ALLOWED_SET, "in", list_v(*(double_v(x) for x in in_set)))
    if not_in and n in not_in:
        add(ErrorCode.IN_FORBIDDEN_SET, "not_in", list_v(*(double_v(x) for x in not_in)))
    if mul is not None and mul != 0 and n % mul != 0:
        add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", double_v(mul))
    return out


def _check_int(
    path: str,
    val: Native,
    cst: int | None,
    gt: int | None,
    gte: int | None,
    lt: int | None,
    lte: int | None,
    mul: int | None,
    in_set: list[int],
    not_in: list[int],
    min_v: int,
    max_v: int,
) -> list[ValidationError]:
    n = as_int(val)
    if n is None:
        return [_type_err(path, "integer", val)]
    if not min_v <= n <= max_v:
        return [_type_err(path, f"integer in [{min_v}, {max_v}]", val)]
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, expected: Value) -> None:
        out.append(_verr(path, code, constraint, expected, int64_v(n)))

    if cst is not None and n != cst:
        add(ErrorCode.CONST_MISMATCH, "const", int64_v(cst))
    if gt is not None and n <= gt:
        add(ErrorCode.GT_VIOLATED, "gt", int64_v(gt))
    if gte is not None and n < gte:
        add(ErrorCode.GTE_VIOLATED, "gte", int64_v(gte))
    if lt is not None and n >= lt:
        add(ErrorCode.LT_VIOLATED, "lt", int64_v(lt))
    if lte is not None and n > lte:
        add(ErrorCode.LTE_VIOLATED, "lte", int64_v(lte))
    if in_set and n not in in_set:
        add(ErrorCode.NOT_IN_ALLOWED_SET, "in", list_v(*(int64_v(x) for x in in_set)))
    if not_in and n in not_in:
        add(ErrorCode.IN_FORBIDDEN_SET, "not_in", list_v(*(int64_v(x) for x in not_in)))
    if mul is not None and mul != 0 and n % mul != 0:
        add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", int64_v(mul))
    return out


def _check_uint(
    path: str,
    val: Native,
    cst: int | None,
    gt: int | None,
    gte: int | None,
    lt: int | None,
    lte: int | None,
    mul: int | None,
    in_set: list[int],
    not_in: list[int],
    max_v: int,
) -> list[ValidationError]:
    n = as_uint(val)
    if n is None:
        return [_type_err(path, "unsigned integer", val)]
    if n > max_v:
        return [_type_err(path, f"unsigned integer <= {max_v}", val)]
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, expected: Value) -> None:
        out.append(_verr(path, code, constraint, expected, uint64_v(n)))

    if cst is not None and n != cst:
        add(ErrorCode.CONST_MISMATCH, "const", uint64_v(cst))
    if gt is not None and n <= gt:
        add(ErrorCode.GT_VIOLATED, "gt", uint64_v(gt))
    if gte is not None and n < gte:
        add(ErrorCode.GTE_VIOLATED, "gte", uint64_v(gte))
    if lt is not None and n >= lt:
        add(ErrorCode.LT_VIOLATED, "lt", uint64_v(lt))
    if lte is not None and n > lte:
        add(ErrorCode.LTE_VIOLATED, "lte", uint64_v(lte))
    if in_set and n not in in_set:
        add(ErrorCode.NOT_IN_ALLOWED_SET, "in", list_v(*(uint64_v(x) for x in in_set)))
    if not_in and n in not_in:
        add(ErrorCode.IN_FORBIDDEN_SET, "not_in", list_v(*(uint64_v(x) for x in not_in)))
    if mul is not None and mul != 0 and n % mul != 0:
        add(ErrorCode.MULTIPLE_OF_VIOLATED, "multiple_of", uint64_v(mul))
    return out


def _check_string(
    e: Engine,
    path: str,
    s: str,
    k: SchemaFieldString,
) -> list[ValidationError]:
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, expected: Value) -> None:
        out.append(_verr(path, code, constraint, expected, str_v(s)))

    n = len(s)
    if k.const is not None and s != k.const:
        add(ErrorCode.CONST_MISMATCH, "const", str_v(k.const))
    if k.len is not None and n != k.len:
        add(ErrorCode.LEN_MISMATCH, "len", uint64_v(k.len))
    if k.min_len is not None and n < k.min_len:
        add(ErrorCode.MIN_LEN_VIOLATED, "min_len", uint64_v(k.min_len))
    if k.max_len is not None and n > k.max_len:
        add(ErrorCode.MAX_LEN_VIOLATED, "max_len", uint64_v(k.max_len))
    if k.pattern is not None:
        re_ = e.regexps.get(k.pattern)
        if re_ is not None and re_.search(s) is None:
            add(ErrorCode.PATTERN_MISMATCH, "pattern", str_v(k.pattern))
    if k.in_ and s not in k.in_:
        add(ErrorCode.NOT_IN_ALLOWED_SET, "in", list_v(*(str_v(x) for x in k.in_)))
    if k.not_in and s in k.not_in:
        add(ErrorCode.IN_FORBIDDEN_SET, "not_in", list_v(*(str_v(x) for x in k.not_in)))
    fmt = k.format or ""
    if fmt != "":
        check = e.formats.get(Format(fmt))
        if check is None:
            add(ErrorCode.UNSUPPORTED_FORMAT, "format", str_v(fmt))
        elif not check(s):
            add(ErrorCode.FORMAT_MISMATCH, "format", str_v(fmt))
    return out


def _check_bytes(path: str, b: bytes, k: SchemaFieldBytes) -> list[ValidationError]:
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, expected: Value) -> None:
        out.append(_verr(path, code, constraint, expected, bytes_v(b)))

    n = len(b)
    if k.const is not None and b != k.const:
        add(ErrorCode.CONST_MISMATCH, "const", bytes_v(k.const))
    if k.len is not None and n != k.len:
        add(ErrorCode.LEN_MISMATCH, "len", uint64_v(k.len))
    if k.min_len is not None and n < k.min_len:
        add(ErrorCode.MIN_LEN_VIOLATED, "min_len", uint64_v(k.min_len))
    if k.max_len is not None and n > k.max_len:
        add(ErrorCode.MAX_LEN_VIOLATED, "max_len", uint64_v(k.max_len))
    if k.prefix and not b.startswith(k.prefix):
        add(ErrorCode.PREFIX_MISMATCH, "prefix", bytes_v(k.prefix))
    if k.suffix and not b.endswith(k.suffix):
        add(ErrorCode.SUFFIX_MISMATCH, "suffix", bytes_v(k.suffix))
    if k.in_ and b not in k.in_:
        add(ErrorCode.NOT_IN_ALLOWED_SET, "in", list_v(*(bytes_v(x) for x in k.in_)))
    if k.not_in and b in k.not_in:
        add(ErrorCode.IN_FORBIDDEN_SET, "not_in", list_v(*(bytes_v(x) for x in k.not_in)))
    return out


def _check_choice(
    e: Engine,
    path: str,
    val: Native,
    k: SchemaFieldChoice,
    root: NativeStruct,
) -> list[ValidationError]:
    if k.open:
        return []
    actual = from_native(val)
    src = k.options_expr or ""
    if src != "":
        value, err = e.eval(src, {"root": root})
        if err is not None:
            return [expr_err(path, src, f"options_expr: {err}")]
        allowed = value if isinstance(value, list) else []
        if any(native_equals(val, a) for a in allowed):
            return []
        return [
            _verr(
                path,
                ErrorCode.CHOICE_NOT_ALLOWED,
                "options_expr",
                list_v(*(from_native(a) for a in allowed)),
                actual,
            ),
        ]
    expected: list[Value] = []
    for o in k.options:
        if native_equals(val, to_native(o.value)):
            return []
        if o.value is not None:
            expected.append(o.value)
    return [_verr(path, ErrorCode.CHOICE_NOT_ALLOWED, "options", list_v(*expected), actual)]


def _as_duration(val: Native) -> dt.timedelta | None:
    if isinstance(val, dt.timedelta):
        return val
    if isinstance(val, str):
        return parse_go_duration(val)
    return None


def _check_duration(path: str, val: Native, k: SchemaFieldDuration) -> list[ValidationError]:
    d = _as_duration(val)
    if d is None:
        return [_type_err(path, "duration", val)]
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, bound: dt.timedelta) -> None:
        out.append(_verr(path, code, constraint, duration_v(bound), duration_v(d)))

    if k.gt is not None and d <= k.gt:
        add(ErrorCode.GT_VIOLATED, "gt", k.gt)
    if k.gte is not None and d < k.gte:
        add(ErrorCode.GTE_VIOLATED, "gte", k.gte)
    if k.lt is not None and d >= k.lt:
        add(ErrorCode.LT_VIOLATED, "lt", k.lt)
    if k.lte is not None and d > k.lte:
        add(ErrorCode.LTE_VIOLATED, "lte", k.lte)
    return out


def _as_timestamp(val: Native) -> dt.datetime | None:
    if isinstance(val, dt.datetime):
        return val
    if isinstance(val, str):
        return parse_rfc3339(val)
    return None


def _check_timestamp(path: str, val: Native, k: SchemaFieldTimestamp) -> list[ValidationError]:
    ts = _as_timestamp(val)
    if ts is None:
        return [_type_err(path, "timestamp (RFC3339)", val)]
    out: list[ValidationError] = []

    def add(code: ErrorCode, constraint: str, bound: dt.datetime) -> None:
        out.append(_verr(path, code, constraint, timestamp_v(bound), timestamp_v(ts)))

    if k.gt is not None and ts <= k.gt:
        add(ErrorCode.GT_VIOLATED, "gt", k.gt)
    if k.gte is not None and ts < k.gte:
        add(ErrorCode.GTE_VIOLATED, "gte", k.gte)
    if k.lt is not None and ts >= k.lt:
        add(ErrorCode.LT_VIOLATED, "lt", k.lt)
    if k.lte is not None and ts > k.lte:
        add(ErrorCode.LTE_VIOLATED, "lte", k.lte)
    return out


def _check_list(
    e: Engine,
    path: str,
    arr: list[Native],
    l: SchemaFieldList,  # noqa: E741
    root: NativeStruct,
) -> list[ValidationError]:
    if is_tuple(l):
        out: list[ValidationError] = []
        want = len(l.items)
        if len(arr) != want:
            out.append(
                _verr(
                    path,
                    ErrorCode.LIST_COUNT_MISMATCH,
                    "tuple",
                    int64_v(want),
                    int64_v(len(arr)),
                ),
            )
        sub: list[ValidationError] = []
        for i in range(min(len(arr), want)):
            _validate_one(e, l.items[i], arr[i], True, f"{path}[{i}]", root, i, sub)  # noqa: FBT003
        return out + sub

    out = []
    n = len(arr)
    if l.min_items is not None and n < l.min_items:
        out.append(
            _verr(
                path, ErrorCode.MIN_ITEMS_VIOLATED, "min_items", uint64_v(l.min_items), uint64_v(n)
            ),
        )
    if l.max_items is not None and n > l.max_items:
        out.append(
            _verr(
                path, ErrorCode.MAX_ITEMS_VIOLATED, "max_items", uint64_v(l.max_items), uint64_v(n)
            ),
        )
    ce = l.count_expr or ""
    if ce != "":
        value, err = e.eval(ce, {"root": root})
        want_n = as_int(value) if err is None else None
        if err is not None:
            out.append(expr_err(path, ce, f"count_expr: {err}"))
        elif want_n is None or want_n < 0:
            out.append(expr_err(path, ce, "count_expr: want a non-negative int"))
        elif want_n != n:
            out.append(
                _verr(
                    path, ErrorCode.LIST_COUNT_MISMATCH, "count_expr", int64_v(want_n), uint64_v(n)
                ),
            )
    if l.unique:
        seen: set[str] = set()
        for i, el in enumerate(arr):
            key = _unique_key(el)
            if key in seen:
                out.append(
                    _verr(f"{path}[{i}]", ErrorCode.NOT_UNIQUE, "unique", None, from_native(el)),
                )
            seen.add(key)
    if len(l.items) >= 1:
        item = l.items[0]
        sub2: list[ValidationError] = []
        for i, el in enumerate(arr):
            _validate_one(e, item, el, True, f"{path}[{i}]", root, i, sub2)  # noqa: FBT003
        out.extend(sub2)
    return out


def _unique_key(v: Native) -> str:
    return json.dumps(_jsonable(v), separators=(",", ":"), sort_keys=False)


def _jsonable(v: Native) -> object:
    if isinstance(v, (bytes, dt.timedelta, dt.datetime)):
        return display_string(v)
    if isinstance(v, list):
        return [_jsonable(el) for el in v]
    if isinstance(v, dict):
        return {k: _jsonable(x) for k, x in v.items()}
    return v


def _check_map(
    e: Engine,
    path: str,
    m: NativeStruct,
    k: SchemaFieldMap,
    root: NativeStruct,
) -> list[ValidationError]:
    out: list[ValidationError] = []
    n = len(m)
    if k.min_entries is not None and n < k.min_entries:
        out.append(
            _verr(
                path,
                ErrorCode.MIN_ENTRIES_VIOLATED,
                "min_entries",
                uint64_v(k.min_entries),
                uint64_v(n),
            ),
        )
    if k.max_entries is not None and n > k.max_entries:
        out.append(
            _verr(
                path,
                ErrorCode.MAX_ENTRIES_VIOLATED,
                "max_entries",
                uint64_v(k.max_entries),
                uint64_v(n),
            ),
        )
    vs = k.value_schema
    if vs is None:
        return out
    for key in sorted(m):
        vpath = join_path(path, key)
        el = m[key]
        if not isinstance(el, dict):
            out.append(_type_err(vpath, "object", el))
            continue
        sub: list[ValidationError] = []
        _validate_fields(e, vs, el, root, vpath, sub)
        for r in vs.rules:
            _eval_rule(e, r, vpath, el, root, None, sub)
        out.extend(sub)
    return out


def _check_one_of(
    e: Engine,
    path: str,
    m: NativeStruct,
    oo: SchemaFieldOneOf,
    root: NativeStruct,
) -> list[ValidationError]:
    disc = m.get(oo.discriminator)
    if not isinstance(disc, str) or disc == "":
        return [
            _verr(
                path,
                ErrorCode.DISCRIMINATOR_MISSING,
                "discriminator",
                str_v(oo.discriminator),
                None,
            ),
        ]
    variant = oo.variants.get(disc)
    if variant is None:
        keys = sorted(oo.variants)
        return [
            _verr(
                path,
                ErrorCode.UNKNOWN_VARIANT,
                "variants",
                list_v(*(str_v(x) for x in keys)),
                str_v(disc),
            ),
        ]
    sub: list[ValidationError] = []
    _validate_fields(e, variant, m, root, path, sub)
    for r in variant.rules:
        _eval_rule(e, r, path, m, root, None, sub)
    return sub


def _check_ref(
    e: Engine,
    path: str,
    val: Native,
    ref: SchemaFieldRef,
    root: NativeStruct,
) -> list[ValidationError]:
    key = ref_def_key(ref)
    def_ = e.schema.defs.get(key)
    if def_ is None:
        label = key
        if ref.id is not None:
            ns = f"{ref.id.namespace}/" if ref.id.namespace else ""
            ver = f"@{ref.id.version}" if ref.id.version else ""
            label = f"{ns}{ref.id.name}{ver} (unlinked identity-ref — call link)"
        return [_verr(path, ErrorCode.UNKNOWN_REF, "ref", str_v(label), None)]
    if not isinstance(val, dict):
        return [_type_err(path, "object", val)]
    sub: list[ValidationError] = []
    _validate_fields(e, def_, val, root, path, sub)
    for r in def_.rules:
        _eval_rule(e, r, path, val, root, None, sub)
    return sub
