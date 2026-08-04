"""schemapb — native Python implementation of the schemapb contract."""

from schemapb._gen.schemapb import (
    Baked,
    ErrorCode,
    Filled,
    ListValue,
    NullValue,
    Schema,
    SchemaField,
    SchemaIdentity,
    SchemaRef,
    StructValue,
    ValidationError,
    ValidationResult,
    Value,
)
from schemapb.bake import (
    BakeOutcome,
    bake,
    baked_matches,
    build_render_context,
    filled_bake,
    merge,
    render,
    render_baked,
)
from schemapb.compute import choice_options, field_is_active, list_count, list_item_def, resolve
from schemapb.descriptor import SchemaError, check_descriptor
from schemapb.duration import format_go_duration, format_rfc3339, parse_go_duration, parse_rfc3339
from schemapb.engine import Engine, compile_schema
from schemapb.formats import FormatRegistry, core_formats
from schemapb.lookup import LookupReason, SchemaLookupError, list_items, lookup, lookup_path
from schemapb.messages import MESSAGE_TEMPLATES, render_message
from schemapb.registry import InMemoryRegistry, RegistryError, identity_key, link
from schemapb.render import display_string, kind_name, native_equals
from schemapb.typed import (
    FORMAT_EMAIL,
    Format,
    Version,
    make_id,
)
from schemapb.validate import result_blocking, result_ok, validate
from schemapb.value import (
    Native,
    NativeStruct,
    canonical_value,
    from_native,
    struct_from_native,
    struct_to_native,
    to_native,
)

__all__ = [  # noqa: RUF022 - grouped by module
    "Baked",
    "ErrorCode",
    "Filled",
    "ListValue",
    "NullValue",
    "Schema",
    "SchemaField",
    "SchemaIdentity",
    "SchemaRef",
    "StructValue",
    "ValidationError",
    "ValidationResult",
    "Value",
    "BakeOutcome",
    "bake",
    "baked_matches",
    "build_render_context",
    "filled_bake",
    "merge",
    "render",
    "render_baked",
    "choice_options",
    "field_is_active",
    "list_count",
    "resolve",
    "SchemaError",
    "check_descriptor",
    "format_go_duration",
    "format_rfc3339",
    "parse_go_duration",
    "parse_rfc3339",
    "Engine",
    "compile_schema",
    "kind_name",
    "list_item_def",
    "list_items",
    "lookup",
    "lookup_path",
    "FormatRegistry",
    "core_formats",
    "MESSAGE_TEMPLATES",
    "render_message",
    "InMemoryRegistry",
    "RegistryError",
    "identity_key",
    "link",
    "display_string",
    "native_equals",
    "FORMAT_EMAIL",
    "Format",
    "LookupReason",
    "SchemaLookupError",
    "Version",
    "make_id",
    "result_blocking",
    "result_ok",
    "validate",
    "Native",
    "NativeStruct",
    "canonical_value",
    "from_native",
    "struct_from_native",
    "struct_to_native",
    "to_native",
]
