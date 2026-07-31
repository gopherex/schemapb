"""Bake / merge / render, mirroring the Go reference bake.go + render.go."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING

import chevron

from schemapb._gen.schemapb import (
    Baked,
    Filled,
    Schema,
    SchemaField,
    StructValue,
    ValidationResult,
    Value,
)
from schemapb.compute import field_is_active, ref_def_key, select_variant
from schemapb.render import RenderField, display_string, render_field
from schemapb.validate import result_blocking, validate
from schemapb.value import (
    CanonicalError,
    Native,
    NativeStruct,
    canonical_struct,
    canonical_value,
    from_native,
    struct_to_native,
)

if TYPE_CHECKING:
    from schemapb.engine import Engine


@dataclass(slots=True)
class BakeOutcome:
    result: ValidationResult
    baked: Baked | None = None


def bake(e: Engine, values: NativeStruct) -> BakeOutcome:
    """Validate + resolve, then seal in canonical wire form."""
    result = validate(e, values)
    if result_blocking(result):
        return BakeOutcome(result=result)
    return BakeOutcome(
        result=result,
        baked=Baked(schema=e.schema, values=_canonical_engine_struct(e, values)),
    )


def _canonical_engine_struct(e: Engine, values: NativeStruct) -> StructValue:
    by_name = {f.name: f for f in e.schema.fields}
    fields = {name: _canonical_top(e, by_name.get(name), val) for name, val in values.items()}
    return StructValue(fields=fields)


def _canonical_top(e: Engine, f: SchemaField | None, val: Native) -> Value:
    if f is None:
        return from_native(val)
    if f.ref is not None:
        def_ = e.schema.defs.get(ref_def_key(f.ref))
        if def_ is not None and isinstance(val, dict):
            return canonical_struct(def_, val)
        return from_native(val)
    if f.one_of is not None:
        sel = select_variant(f.one_of, val)
        if sel is not None:
            return canonical_struct(sel[0], sel[1])
        return from_native(val)
    try:
        return canonical_value(f, val)
    except CanonicalError:
        return from_native(val)


def merge(e: Engine, baked: Baked, overrides: StructValue, *, replace_lists: bool) -> BakeOutcome:
    base = struct_to_native(baked.values)
    over = struct_to_native(overrides)
    return bake(e, _merge_structs(base, over, replace_lists=replace_lists))


def _merge_structs(dst: NativeStruct, src: NativeStruct, *, replace_lists: bool) -> NativeStruct:
    out: NativeStruct = dict(dst)
    for k, sv in src.items():
        dv = out.get(k)
        if isinstance(dv, dict) and isinstance(sv, dict):
            out[k] = _merge_structs(dv, sv, replace_lists=replace_lists)
            continue
        if not replace_lists and isinstance(dv, list) and isinstance(sv, list):
            out[k] = [*dv, *sv]
            continue
        out[k] = sv
    return out


def baked_matches(baked: Baked, s: Schema) -> bool:
    return baked.schema is not None and baked.schema == s


def filled_bake(filled: Filled, compile_fn: Callable[[Schema], Engine]) -> BakeOutcome:
    src = filled.schema
    if src is None or src.schema is None:
        msg = "schemapb: Filled bake requires an inline schema (id refs resolve via a registry)"
        raise ValueError(msg)
    return bake(compile_fn(src.schema), struct_to_native(filled.values))


def render(e: Engine, name: str, values: NativeStruct) -> str | None:
    tmpl = e.schema.templates.get(name)
    if tmpl is None:
        return None
    return str(chevron.render(tmpl, build_render_context(e, values)))


def render_baked(e: Engine, baked: Baked, name: str) -> str | None:
    return render(e, name, struct_to_native(baked.values))


def build_render_context(e: Engine, values: NativeStruct) -> dict[str, object]:
    """The contract render context; inactive fields excluded entirely."""
    fields: list[RenderField] = []
    group_fields: list[list[RenderField]] = []
    group_names: list[str] = []
    group_idx: dict[str, int] = {}

    for f in e.schema.fields:
        if (f.when or "") != "" and not field_is_active(e, f, values, f.name, None):
            continue
        rf = render_field(f, values)
        fields.append(rf)
        g = f.group or ""
        i = group_idx.get(g)
        if i is None:
            i = len(group_names)
            group_idx[g] = i
            group_names.append(g)
            group_fields.append([])
        group_fields[i].append(rf)
    groups: list[dict[str, object]] = [
        {"name": name, "fields": flds} for name, flds in zip(group_names, group_fields, strict=True)
    ]

    display = {name: "" if val is None else display_string(val) for name, val in values.items()}
    return {"fields": fields, "groups": groups, "values": display}
