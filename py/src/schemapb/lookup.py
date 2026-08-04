"""Schema path lookup: resolving a dot path ("a.b.c") to the field it
addresses.

Paths address FIELDS, not values, so they carry no list indices or map
keys: lookup descends through Object fields and resolves Refs against root
$defs; every other kind is terminal - a path may END on a list/map/oneof
field but cannot continue through one.

Failures point at the exact segment that broke ("no field b in a", never
"a.b.c not found").
"""

from __future__ import annotations

import json
from enum import StrEnum
from typing import TYPE_CHECKING

from schemapb.compute import ref_def_key
from schemapb.descriptor import join_path
from schemapb.render import kind_name

if TYPE_CHECKING:
    from schemapb._gen.schemapb import Schema, SchemaField

__all__ = ["LookupReason", "SchemaLookupError", "lookup", "lookup_path"]


class LookupReason(StrEnum):
    """Stable spec strings shared by all implementations (conformance)."""

    EMPTY_PATH = "empty_path"
    NOT_FOUND = "not_found"
    NOT_TRAVERSABLE = "not_traversable"
    AMBIGUOUS_ONE_OF = "ambiguous_oneof"
    UNKNOWN_REF = "unknown_ref"


class SchemaLookupError(Exception):
    """Pinpoints the failing segment of a schema path.

    ``at`` is the resolved parent path ("" for root), ``segment`` the name
    that failed, ``kind`` the kind of the offending field (set for the
    traversal reasons).
    """

    def __init__(
        self, reason: LookupReason, at: str = "", segment: str = "", kind: str = ""
    ) -> None:
        self.at = at
        self.segment = segment
        self.reason = reason
        self.kind = kind
        super().__init__(self._message())

    def _message(self) -> str:
        where = json.dumps(self.at) if self.at else "root"
        seg = json.dumps(self.segment)
        if self.reason is LookupReason.EMPTY_PATH:
            return "schemapb: lookup: empty path"
        if self.reason is LookupReason.NOT_FOUND:
            return f"schemapb: lookup: no field {seg} in {where}"
        if self.reason is LookupReason.AMBIGUOUS_ONE_OF:
            return (
                f"schemapb: lookup: cannot descend into oneof {seg} in {where}: "
                "the variant depends on a discriminator value"
            )
        if self.reason is LookupReason.UNKNOWN_REF:
            return f"schemapb: lookup: ref {seg} in {where} points to a def that does not exist"
        return f"schemapb: lookup: cannot descend into {seg} in {where} (kind {self.kind})"


def lookup(s: Schema, *segments: str) -> SchemaField:
    """Resolve a field path within the schema, one segment per field name.

    Returns the addressed field or raises ``SchemaLookupError`` naming the exact
    segment that failed.
    """
    if not segments:
        raise SchemaLookupError(LookupReason.EMPTY_PATH)

    cur: Schema | None = s
    parent = ""
    for i, seg in enumerate(segments):
        f = next((c for c in cur.fields if c.name == seg), None) if cur is not None else None
        if f is None:
            raise SchemaLookupError(LookupReason.NOT_FOUND, parent, seg)
        if i == len(segments) - 1:
            return f

        if f.object is not None:
            cur = f.object.schema
        elif f.ref is not None:
            def_ = s.defs.get(ref_def_key(f.ref))
            if def_ is None:
                raise SchemaLookupError(LookupReason.UNKNOWN_REF, parent, seg, "ref")
            cur = def_
        elif f.one_of is not None:
            raise SchemaLookupError(LookupReason.AMBIGUOUS_ONE_OF, parent, seg, "oneof")
        else:
            raise SchemaLookupError(LookupReason.NOT_TRAVERSABLE, parent, seg, kind_name(f))

        parent = join_path(parent, seg)

    msg = "unreachable"
    raise AssertionError(msg)


def lookup_path(s: Schema, path: str) -> SchemaField:
    """``lookup`` over a dot-separated path ("a.b.c").

    Field names are identifiers (enforced by descriptor validation), so the
    dot is never part of a name.
    """
    if path == "":
        raise SchemaLookupError(LookupReason.EMPTY_PATH)
    return lookup(s, *path.split("."))
