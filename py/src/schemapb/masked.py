"""Schema-driven display masking of wire snapshots, without resolution or CEL."""

from copy import deepcopy

import betterproto2

from schemapb._gen.schemapb import Baked, Schema, SchemaField, StructValue, Value
from schemapb.compute import list_item_def, ref_def_key


def masked(baked: Baked) -> StructValue:
    """Return independent values with every present secret replaced by ``***``."""
    out = deepcopy(baked.values) if baked.values is not None else StructValue()
    _mask_struct(baked.schema, out, baked.schema.defs if baked.schema else {})
    return out


def _hidden() -> Value:
    return Value(string_value="***")


def _mask_struct(schema: Schema | None, values: StructValue, defs: dict[str, Schema]) -> None:
    if schema is None:
        for key in values.fields:
            values.fields[key] = _hidden()
        return
    for field in schema.fields:
        if field.name in values.fields:
            values.fields[field.name] = _mask_value(field, values.fields[field.name], defs)


def _mask_object(schema: Schema | None, value: Value, defs: dict[str, Schema]) -> Value:
    if schema is None or value.struct_value is None:
        return _hidden()
    _mask_struct(schema, value.struct_value, defs)
    return value


def _mask_value(field: SchemaField, value: Value, defs: dict[str, Schema]) -> Value:
    if field.secret:
        return _hidden()
    if betterproto2.which_one_of(value, "kind")[0] in ("", "null_value"):
        return value
    if field.object is not None:
        return _mask_object(field.object.schema, value, defs)
    if field.ref is not None:
        return _mask_object(defs.get(ref_def_key(field.ref)), value, defs)
    if field.one_of is not None:
        disc = (
            value.struct_value.fields.get(field.one_of.discriminator)
            if value.struct_value
            else None
        )
        schema = (
            field.one_of.variants.get(disc.string_value)
            if disc and disc.string_value is not None
            else None
        )
        return _mask_object(schema, value, defs)
    return _mask_collections(field, value, defs)


def _mask_collections(field: SchemaField, value: Value, defs: dict[str, Schema]) -> Value:
    if field.list is not None:
        if value.list_value is None:
            return _hidden()
        for i, item in enumerate(value.list_value.items):
            item_def = list_item_def(field.list, i)
            if item_def is not None:
                value.list_value.items[i] = _mask_value(item_def, item, defs)
    if field.map is not None:
        if value.struct_value is None:
            return _hidden()
        for key, item in value.struct_value.fields.items():
            if field.map.value_field is not None:
                value.struct_value.fields[key] = _mask_value(field.map.value_field, item, defs)
            elif field.map.value_schema is not None and item.null_value is None:
                value.struct_value.fields[key] = _mask_object(field.map.value_schema, item, defs)
    return value
