"""Schema DESCRIPTOR validation, mirroring the Go reference descriptor.go."""

from __future__ import annotations

from schemapb._gen.schemapb import (
    ErrorCode,
    Schema,
    SchemaField,
    SchemaFieldSeverity,
    ValidationError,
    ValidationResult,
)


class SchemaError(Exception):
    """A malformed schema descriptor (programmatic failure, principle 5)."""

    def __init__(self, errors: list[ValidationError]) -> None:
        text = "; ".join(e.message if e.path == "" else f"{e.path}: {e.message}" for e in errors)
        super().__init__(f"schemapb: invalid schema; {text}")
        self.result = ValidationResult(errors=errors)


def schema_err(path: str, msg: str) -> ValidationError:
    return ValidationError(
        path=path,
        code=ErrorCode.INVALID_SCHEMA,
        severity=SchemaFieldSeverity.ERROR,
        message=msg,
    )


def join_path(prefix: str, name: str) -> str:
    return name if prefix == "" else f"{prefix}.{name}"


def nested_schemas(f: SchemaField) -> list[Schema]:
    out: list[Schema] = []
    if f.object is not None and f.object.schema is not None:
        out.append(f.object.schema)
    if f.one_of is not None:
        out.extend(f.one_of.variants.values())
    if f.map is not None and f.map.value_schema is not None:
        out.append(f.map.value_schema)
    if f.list is not None:
        for it in f.list.items:
            out.extend(nested_schemas(it))
    return out


def check_descriptor(s: Schema) -> list[ValidationError]:
    errs: list[ValidationError] = []
    if s.id is None or s.id.name == "":
        errs.append(schema_err("id.name", "schema identity name is required"))
    errs.extend(_check_fields(s.fields, ""))
    for name, def_ in s.defs.items():
        errs.extend(_check_fields(def_.fields, f"$defs.{name}"))
    errs.extend(_check_ref_targets(s.fields, s.defs, ""))
    for name, def_ in s.defs.items():
        errs.extend(_check_ref_targets(def_.fields, s.defs, f"$defs.{name}"))
    return errs


def _check_fields(fields: list[SchemaField], prefix: str) -> list[ValidationError]:
    errs: list[ValidationError] = []
    seen: set[str] = set()
    for f in fields:
        path = join_path(prefix, f.name)
        if f.name == "":
            errs.append(schema_err(prefix, "field name is required"))
            continue
        if f.name in seen:
            errs.append(schema_err(path, "duplicate field name"))
        seen.add(f.name)
        if _kind_case(f) is None:
            errs.append(schema_err(path, "field kind is required"))
            continue
        for i, r in enumerate(f.rules):
            if r.expr == "":
                errs.append(schema_err(path, f"rule[{i}]: empty expression"))
        if f.computed is not None and f.computed.expr == "":
            errs.append(schema_err(path, "computed field: empty expression"))
        if f.one_of is not None:
            if f.one_of.discriminator == "":
                errs.append(schema_err(path, "oneof field: discriminator is required"))
            if len(f.one_of.variants) == 0:
                errs.append(schema_err(path, "oneof field: at least one variant is required"))
        if f.ref is not None and f.ref.name is None and f.ref.id is None:
            errs.append(schema_err(path, "ref field: target is required"))
        if f.list is not None:
            if len(f.list.items) == 0:
                errs.append(
                    schema_err(path, "list field: at least one item definition is required")
                )
            if len(f.list.items) > 1 and (
                f.list.min_items is not None
                or f.list.max_items is not None
                or f.list.unique
                or (f.list.count_expr or "") != ""
            ):
                errs.append(
                    schema_err(
                        path,
                        "tuple list (multiple item definitions) cannot combine with"
                        " min_items/max_items/unique/count_expr",
                    ),
                )
        if f.choice is not None:
            ch = f.choice
            if not ch.open and len(ch.options) == 0 and (ch.options_expr or "") == "":
                errs.append(
                    schema_err(
                        path, "choice field: a closed choice requires options or options_expr"
                    ),
                )
            for i, o in enumerate(ch.options):
                if o.value is None:
                    errs.append(schema_err(path, f"choice option[{i}]: value is required"))
        if (
            f.map is not None
            and f.map.min_entries is not None
            and f.map.max_entries is not None
            and f.map.min_entries > f.map.max_entries
        ):
            errs.append(schema_err(path, "map field: min_entries must be <= max_entries"))
        for child in nested_schemas(f):
            errs.extend(_check_fields(child.fields, path))
    return errs


_KIND_ATTRS = (
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


def _kind_case(f: SchemaField) -> str | None:
    for attr in _KIND_ATTRS:
        if getattr(f, attr) is not None:
            return attr
    return None


def _check_ref_targets(
    fields: list[SchemaField],
    root_defs: dict[str, Schema],
    prefix: str,
) -> list[ValidationError]:
    errs: list[ValidationError] = []
    for f in fields:
        path = join_path(prefix, f.name)
        if (
            f.ref is not None
            and f.ref.id is None
            and (f.ref.name or "") != ""
            and f.ref.name not in root_defs
        ):
            errs.append(schema_err(path, f'ref "{f.ref.name}" is not defined in schema defs'))
        if f.list is not None:
            errs.extend(_check_ref_targets(f.list.items, root_defs, f"{path}[]"))
            continue
        for child in nested_schemas(f):
            errs.extend(_check_ref_targets(child.fields, root_defs, path))
    return errs
