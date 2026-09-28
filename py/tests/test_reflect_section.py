"""Sections: a field annotated with a nested model (not ``X | None``, no
default) is optional with an implicit empty object default, so an absent
section still resolves its inner defaults. Mirrors the Go reference
(go/schemapb/reflect_section_test.go)."""

from __future__ import annotations

from typing import TYPE_CHECKING, Annotated

import pydantic
from pydantic import BaseModel

import schemapb as spb
import schemapb.pydantic as sp
from schemapb import builder as b
from schemapb._gen.schemapb import ErrorCode, Schema

if TYPE_CHECKING:
    from collections.abc import Callable

    from schemapb._gen.schemapb import SchemaField, ValidationResult


class Host(str):
    __slots__ = ()


class Port(int):
    __slots__ = ()


class Realm(str):
    __slots__ = ()


# Pydantic defaults are not reflected; the inner defaults come from the
# native override mechanism, like Go's `schemapb:"default=..."` tags.
OVERRIDES: dict[type, Callable[[str], SchemaField]] = {
    Host: lambda n: b.str_(n).default("localhost").done(),
    Port: lambda n: b.int64(n).default(5432).done(),
    Realm: lambda n: b.str_(n).default("main").done(),
}


class SectionDB(BaseModel):
    model_config = pydantic.ConfigDict(arbitrary_types_allowed=True)

    host: Host
    port: Port


class SectionAuth(BaseModel):
    model_config = pydantic.ConfigDict(arbitrary_types_allowed=True)

    token: str
    realm: Realm


def _reflect(model: type[BaseModel]) -> Schema:
    return sp.reflect(
        model, spb.make_id("t", "section", spb.Version.of(1, 0, 0)), overrides=OVERRIDES
    )


def _has_error(res: ValidationResult, path: str, code: ErrorCode) -> bool:
    return any(e.path == path and e.code == code for e in res.errors)


def test_section_optional_with_inner_defaults() -> None:
    class Config(BaseModel):
        db: SectionDB

    schema = _reflect(Config)
    f = spb.lookup_path(schema, "db")
    assert not f.required
    assert f.object is not None
    assert f.object.default is not None

    outcome = spb.compile_schema(schema).bake({})
    assert outcome.result.errors == []
    assert outcome.baked is not None
    assert spb.struct_to_native(outcome.baked.values) == {"db": {"host": "localhost", "port": 5432}}


def test_section_required_inner_field_reported_inside() -> None:
    class Config(BaseModel):
        auth: SectionAuth

    res = spb.compile_schema(_reflect(Config)).bake({}).result
    assert _has_error(res, "auth.token", ErrorCode.REQUIRED_MISSING)
    assert not _has_error(res, "auth", ErrorCode.REQUIRED_MISSING)


def test_section_explicit_required() -> None:
    class Config(BaseModel):
        db: Annotated[SectionDB, sp.required()]

    schema = _reflect(Config)
    f = spb.lookup_path(schema, "db")
    assert f.required
    assert f.object is not None
    assert f.object.default is None

    res = spb.compile_schema(schema).bake({}).result
    assert _has_error(res, "db", ErrorCode.REQUIRED_MISSING)


def test_optional_section_unchanged() -> None:
    class Config(BaseModel):
        db: SectionDB | None = None
        nullable: SectionDB | None

    schema = _reflect(Config)
    db = spb.lookup_path(schema, "db")
    assert not db.required
    assert db.nullable
    assert db.object is not None
    assert db.object.default is None

    # `X | None` without a default: required and nullable, no default.
    nullable = spb.lookup_path(schema, "nullable")
    assert nullable.required
    assert nullable.nullable
    assert nullable.object is not None
    assert nullable.object.default is None

    outcome = spb.compile_schema(schema).bake({"nullable": None})
    assert outcome.result.errors == []
    assert outcome.baked is not None
    assert spb.struct_to_native(outcome.baked.values) == {"nullable": None}


def test_defaulted_section_is_not_materialized() -> None:
    class Config(BaseModel):
        db: SectionDB = SectionDB(host=Host("h"), port=Port(1))

    f = spb.lookup_path(_reflect(Config), "db")
    assert not f.required
    assert f.object is not None
    assert f.object.default is None


def test_overridden_section_is_not_a_section() -> None:
    class Config(BaseModel):
        db: SectionDB

    schema = sp.reflect(
        Config,
        spb.make_id("t", "section", spb.Version.of(1, 0, 0)),
        overrides={SectionDB: lambda n: b.json_(n).done()},
    )
    f = spb.lookup_path(schema, "db")
    assert f.required
    assert spb.kind_name(f) == "json"


def test_nested_model_items_are_not_sections() -> None:
    class Config(BaseModel):
        dbs: list[SectionDB]

    schema = _reflect(Config)
    dbs = spb.lookup_path(schema, "dbs")
    assert dbs.list is not None
    item = dbs.list.items[0]
    assert item.required
    assert item.object is not None
    assert item.object.default is None
