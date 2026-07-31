"""The compiled engine: every CEL expression, regex pattern and Mustache
template compiled once, up front; conversions between the native model and
celpy's celtypes. Mirrors the Go reference engine.go.
"""

from __future__ import annotations

import datetime as dt
import re
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

import celpy
import chevron
from celpy import celtypes
from lark import Token, Tree

from schemapb.descriptor import (
    SchemaError,
    check_descriptor,
    join_path,
    nested_schemas,
    schema_err,
)
from schemapb.formats import FormatRegistry, core_formats
from schemapb.render import ascii_lower, ascii_upper

if TYPE_CHECKING:
    from schemapb._gen.schemapb import Schema, SchemaField, ValidationError
    from schemapb.value import Native

# The strings-extension subset registered as custom functions (celpy has no
# built-in strings extension; the rest of the extension is a tracked gap).
_CUSTOM_FUNCTIONS: dict[str, Any] = {
    "lowerAscii": lambda s: celtypes.StringType(ascii_lower(str(s))),
    "upperAscii": lambda s: celtypes.StringType(ascii_upper(str(s))),
    "trim": lambda s: celtypes.StringType(str(s).strip()),
}


@dataclass(slots=True)
class _Compiled:
    program: Any
    tree: Tree[Token]


@dataclass(slots=True)
class Engine:
    """An immutable compiled schema."""

    schema: Schema
    formats: FormatRegistry
    regexps: dict[str, re.Pattern[str]]
    _programs: dict[str, _Compiled] = field(default_factory=dict)

    def eval(self, src: str, vars_: dict[str, Native]) -> tuple[Native, str | None]:
        """Runs a precompiled CEL plan (sandboxed expression language, not
        Python eval). Returns (value, error).
        """
        compiled = self._programs.get(src)
        if compiled is None:
            return None, f"expression not compiled: {src}"
        activation = {
            "this": native_to_cel(vars_.get("this")),
            "root": native_to_cel(vars_.get("root", {})),
            "index": celtypes.IntType(_as_index(vars_.get("index", 0))),
        }
        try:
            out = compiled.program.evaluate(activation)
        except celpy.CELEvalError as e:
            return None, str(e.args[0]) if e.args else str(e)
        return cel_to_native(out), None

    def eval_bool(self, src: str, vars_: dict[str, Native]) -> tuple[bool, str | None]:
        value, err = self.eval(src, vars_)
        if err is not None:
            return False, err
        if not isinstance(value, bool):
            return False, f"expression yields {type(value).__name__}, want bool"
        return value, None

    def expr_deps(self, src: str) -> list[str]:
        compiled = self._programs.get(src)
        if compiled is None:
            return []
        deps: list[str] = []
        _walk_deps(compiled.tree, deps)
        return deps


def _as_index(v: object) -> int:
    return v if isinstance(v, int) else 0


def compile_schema(
    schema: Schema,
    formats: dict[str, Any] | None = None,
) -> Engine:
    """Compiles a schema; throws SchemaError on any defect."""
    errs: list[ValidationError] = list(check_descriptor(schema))

    registry = core_formats()
    for name, fn in (formats or {}).items():
        registry[name] = fn  # type: ignore[index]

    env = celpy.Environment()
    programs: dict[str, _Compiled] = {}
    for src, path in _schema_exprs(schema).items():
        try:
            tree = env.compile(src)
            programs[src] = _Compiled(env.program(tree, functions=_CUSTOM_FUNCTIONS), tree)
        except celpy.CELParseError as e:
            errs.append(schema_err(path, f"cel: {e}"))

    regexps: dict[str, re.Pattern[str]] = {}
    for pattern, path in _schema_patterns(schema).items():
        try:
            regexps[pattern] = re.compile(pattern)
        except re.error as e:
            errs.append(schema_err(path, f"pattern: {e}"))

    for name, src in schema.templates.items():
        try:
            chevron.render(src, {})
        except Exception as e:  # noqa: BLE001 - chevron raises plain Exception subclasses
            errs.append(schema_err(f"templates.{name}", f"mustache: {e}"))

    engine = Engine(schema=schema, formats=registry, regexps=regexps, _programs=programs)
    if not errs:
        errs.extend(_check_computed_cycles(engine))
    if errs:
        raise SchemaError(errs)
    return engine


# =============================================================================
# Expression / pattern collection
# =============================================================================


def _schema_exprs(s: Schema) -> dict[str, str]:
    out: dict[str, str] = {}

    def add(src: str | None, path: str) -> None:
        if src and src not in out:
            out[src] = path

    def walk_schema(sub: Schema, prefix: str) -> None:
        for i, r in enumerate(sub.rules):
            add(r.expr, f"{prefix}#rule[{i}]")

    def walk_fields(fields: list[SchemaField], prefix: str) -> None:
        for f in fields:
            path = join_path(prefix, f.name)
            add(f.when, f"{path}#when")
            add(f.normalize, f"{path}#normalize")
            for i, r in enumerate(f.rules):
                add(r.expr, f"{path}#rule[{i}]")
            if f.computed is not None:
                add(f.computed.expr, f"{path}#computed")
            if f.choice is not None:
                add(f.choice.options_expr, f"{path}#options")
            if f.list is not None:
                add(f.list.count_expr, f"{path}#count")
                walk_fields(f.list.items, f"{path}[]")
            for child in nested_schemas(f):
                walk_schema(child, path)
                walk_fields(child.fields, path)

    walk_schema(s, "")
    walk_fields(s.fields, "")
    for name, def_ in s.defs.items():
        walk_schema(def_, f"$defs.{name}")
        walk_fields(def_.fields, f"$defs.{name}")
    return out


def _schema_patterns(s: Schema) -> dict[str, str]:
    out: dict[str, str] = {}

    def walk_fields(fields: list[SchemaField], prefix: str) -> None:
        for f in fields:
            path = join_path(prefix, f.name)
            if f.string is not None and (f.string.pattern or "") != "":
                out.setdefault(f.string.pattern or "", f"{path}#pattern")
            if f.list is not None:
                walk_fields(f.list.items, f"{path}[]")
            for child in nested_schemas(f):
                walk_fields(child.fields, path)

    walk_fields(s.fields, "")
    for name, def_ in s.defs.items():
        walk_fields(def_.fields, f"$defs.{name}")
    return out


def _check_computed_cycles(e: Engine) -> list[ValidationError]:
    computed = {f.name: f for f in e.schema.fields if f.computed is not None}
    if not computed:
        return []
    deps: dict[str, list[str]] = {}
    for name, f in computed.items():
        assert f.computed is not None  # noqa: S101 - filtered above
        deps[name] = [d for d in e.expr_deps(f.computed.expr) if d != name and d in computed]
    color: dict[str, int] = {}
    errs: list[ValidationError] = []

    def visit(n: str) -> bool:
        c = color.get(n, 0)
        if c == 1:
            return False
        if c == 2:
            return True
        color[n] = 1
        for d in deps.get(n, []):
            if not visit(d):
                return False
        color[n] = 2
        return True

    errs.extend(
        schema_err(name, "computed field cycle")
        for name in computed
        if color.get(name) != 2 and not visit(name)
    )
    return errs


# =============================================================================
# CEL <-> native conversion
# =============================================================================


def native_to_cel(x: Native) -> Any:  # noqa: ANN401 - celtypes has no common base
    if x is None:
        return None
    if isinstance(x, bool):
        return celtypes.BoolType(x)
    if isinstance(x, int):
        return celtypes.IntType(x)
    if isinstance(x, float):
        return celtypes.DoubleType(x)
    if isinstance(x, str):
        return celtypes.StringType(x)
    if isinstance(x, bytes):
        return celtypes.BytesType(x)
    if isinstance(x, dt.timedelta):
        return celtypes.DurationType(x)
    if isinstance(x, dt.datetime):
        return celtypes.TimestampType(x)
    if isinstance(x, list):
        return celtypes.ListType([native_to_cel(el) for el in x])
    return celtypes.MapType(
        {celtypes.StringType(k): native_to_cel(v) for k, v in x.items()},
    )


def cel_to_native(v: Any) -> Native:  # noqa: ANN401
    if v is None or isinstance(
        v, celtypes.NoneType if hasattr(celtypes, "NoneType") else type(None)
    ):
        return None
    if isinstance(v, celtypes.BoolType):
        return bool(v)
    if isinstance(v, (celtypes.IntType, celtypes.UintType)):
        return int(v)
    if isinstance(v, celtypes.DoubleType):
        return float(v)
    if isinstance(v, celtypes.StringType):
        return str(v)
    if isinstance(v, celtypes.BytesType):
        return bytes(v)
    if isinstance(v, celtypes.DurationType):
        return dt.timedelta(seconds=v.total_seconds())
    if isinstance(v, celtypes.TimestampType):
        return dt.datetime.fromtimestamp(v.timestamp(), tz=dt.UTC)
    if isinstance(v, celtypes.ListType):
        return [cel_to_native(el) for el in v]
    if isinstance(v, celtypes.MapType):
        return {str(k): cel_to_native(el) for k, el in v.items()}
    if isinstance(v, bool):
        return v
    if isinstance(v, int):
        return v
    if isinstance(v, float):
        return v
    if isinstance(v, str):
        return v
    return None


# =============================================================================
# AST dependency walk (celpy compiles to a lark Tree)
# =============================================================================


def _walk_deps(node: object, deps: list[str]) -> None:
    if not isinstance(node, Tree):
        return
    path = _select_path(node)
    if path is not None:
        if path != "":
            deps.append(path)
        return
    for child in node.children:
        _walk_deps(child, deps)


def _select_path(node: Tree[Token]) -> str | None:
    """Resolves a member_dot / member_index chain rooted at `root`."""
    if node.data == "member":
        head = node.children[0] if node.children else None
        return _select_path(head) if isinstance(head, Tree) else None
    if node.data == "primary":
        child = node.children[0] if node.children else None
        if isinstance(child, Tree) and child.data == "ident":
            token = child.children[0]
            return "" if str(token) == "root" else None
        return None
    if node.data == "member_dot":
        first = node.children[0]
        base = _select_path(first) if isinstance(first, Tree) else None
        if base is None:
            return None
        name = str(node.children[1])
        return name if base == "" else f"{base}.{name}"
    if node.data == "member_index":
        head = node.children[0]
        base = _select_path(head) if isinstance(head, Tree) else None
        if base is None:
            return None
        key = _string_literal(node.children[1])
        if key is None:
            return None
        return key if base == "" else f"{base}.{key}"
    if node.data == "ident":
        token = node.children[0]
        return "" if str(token) == "root" else None
    return None


def _string_literal(node: Any) -> str | None:  # noqa: ANN401
    if isinstance(node, Tree):
        for child in node.children:
            found = _string_literal(child)
            if found is not None:
                return found
        return None
    text = str(node)
    if len(text) >= 2 and text[0] in "'\"" and text[-1] == text[0]:
        return text[1:-1]
    return None
