"""Identity-addressed schema registry and identity-ref linking."""

from __future__ import annotations

import copy
from typing import TYPE_CHECKING

from schemapb.descriptor import nested_schemas

if TYPE_CHECKING:
    from schemapb._gen.schemapb import Schema, SchemaField, SchemaIdentity
    from schemapb.typed import Version


class RegistryError(Exception):
    """A registry misuse (missing name, conflicting identity, broken link)."""


def identity_key(id_: SchemaIdentity | None) -> str:
    if id_ is None:
        return "\x00\x00"
    return f"{id_.namespace}\x00{id_.name}\x00{id_.version}"


class InMemoryRegistry:
    """In-memory identity-addressed store; strict put + explicit replace."""

    def __init__(self) -> None:
        self._m: dict[str, Schema] = {}

    def put(self, s: Schema) -> None:
        if s.id is None or s.id.name == "":
            msg = "schemapb: schema identity requires a name"
            raise RegistryError(msg)
        key = identity_key(s.id)
        existing = self._m.get(key)
        if existing is not None and existing != s:
            msg = f"schemapb: identity already registered with different content: {s.id.name}"
            raise RegistryError(msg)
        self._m[key] = s

    def put_replace(self, s: Schema) -> None:
        if s.id is None or s.id.name == "":
            msg = "schemapb: schema identity requires a name"
            raise RegistryError(msg)
        self._m[identity_key(s.id)] = s

    def get(self, id_: SchemaIdentity) -> Schema | None:
        return self._m.get(identity_key(id_))

    def list(
        self,
        namespace: str | None = None,
        name: str | None = None,
        version: Version | None = None,
        name_contains: str | None = None,
    ) -> list[Schema]:
        out: list[Schema] = []
        for s in self._m.values():
            sid = s.id
            if namespace is not None and (sid is None or sid.namespace != namespace):
                continue
            if name is not None and (sid is None or sid.name != name):
                continue
            if version is not None and (sid is None or sid.version != str(version)):
                continue
            if name_contains is not None and (
                sid is None or name_contains.lower() not in sid.name.lower()
            ):
                continue
            out.append(s)
        return out


def _collect_id_refs(fields: list[SchemaField], out: dict[str, SchemaIdentity]) -> None:
    for f in fields:
        if f.ref is not None and f.ref.id is not None:
            out[identity_key(f.ref.id)] = f.ref.id
        if f.list is not None:
            _collect_id_refs(f.list.items, out)
            continue
        for child in nested_schemas(f):
            _collect_id_refs(child.fields, out)


def link(s: Schema, reg: InMemoryRegistry) -> Schema:
    """Pulls every identity-ref into the root defs (transitively); returns a
    linked deep copy.
    """
    root = copy.deepcopy(s)
    while True:
        ids: dict[str, SchemaIdentity] = {}
        _collect_id_refs(root.fields, ids)
        for def_ in root.defs.values():
            _collect_id_refs(def_.fields, ids)
        added = False
        for key, id_ in ids.items():
            if key in root.defs:
                continue
            resolved = reg.get(id_)
            if resolved is None:
                msg = f"schemapb: link: cannot resolve schema {id_.name}"
                raise RegistryError(msg)
            cloned = copy.deepcopy(resolved)
            for k, d in cloned.defs.items():
                root.defs.setdefault(k, d)
            cloned.defs = {}
            root.defs[key] = cloned
            added = True
        if not added:
            break
    return root
