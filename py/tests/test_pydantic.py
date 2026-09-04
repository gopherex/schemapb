"""Reflect conformance: the pydantic mirror model must produce the exact
Schema the Go reference reflected into conformance/golden/reflect.json
(field order included)."""

from __future__ import annotations

import datetime as dt
import json
from pathlib import Path
from typing import Annotated, Any, Literal

import pydantic
import pytest
from pydantic import BaseModel, Field

import schemapb as spb
import schemapb.pydantic as sp
from schemapb import builder as b
from schemapb._gen.schemapb import Schema

GOLDEN = Path(__file__).parent.parent.parent / "conformance" / "golden"


class MirrorBase(BaseModel):
    base: str


class MirrorNested(BaseModel):
    on: bool


class MirrorNode(BaseModel):
    next: MirrorNode | None = None


class MirrorModel(MirrorBase):
    name: Annotated[str, Field(min_length=1, max_length=64, description="display name")]
    mail: Annotated[str, sp.fmt("email")]
    slug: Annotated[str, Field(pattern="^[a-z-]+$")]
    mode: Literal["fast", "safe"]
    level: Literal[1, 2, 3]
    count: Annotated[sp.Int32, Field(ge=0, le=100)]
    big: Annotated[int, Field(gt=-10, lt=10)]
    port: Annotated[sp.UInt32, Field(le=65535)]
    total: sp.UInt64
    ratio: Annotated[sp.Float32, Field(ge=0)]
    score: Annotated[float, Field(lt=1)]
    flag: bool
    opt: str | None = None
    must: int | None
    tags: Annotated[list[str], Field(min_length=1, max_length=5)]
    pair: tuple[str, str]
    blob: Annotated[bytes, Field(max_length=16)]
    magic: Annotated[bytes, sp.exact_len(4)]
    limits: dict[str, int]
    extra: dict[str, MirrorNested]
    nested: MirrorNested
    when: dt.datetime
    wait: dt.timedelta
    raw: Any
    anything: Any
    chain: MirrorNode
    gone: Annotated[str, Field(exclude=True)] = ""


def test_reflect_mirror_matches_golden() -> None:
    got = sp.reflect(MirrorModel, spb.make_id("conformance", "mirror", spb.Version.of(1, 0, 0)))
    want = Schema().from_json((GOLDEN / "reflect.json").read_text())
    assert json.loads(got.to_json()) == json.loads(want.to_json())


def test_reflect_loud_failures() -> None:
    class BadUnion(BaseModel):
        x: int | str

    with pytest.raises(ValueError, match="unsupported union"):
        sp.reflect(BadUnion, spb.make_id("t", "r", spb.Version.of(1, 0, 0)))

    class BadKey(BaseModel):
        x: dict[int, str]

    with pytest.raises(ValueError, match="map key"):
        sp.reflect(BadKey, spb.make_id("t", "r", spb.Version.of(1, 0, 0)))


def test_reflect_override() -> None:
    class SecretName(str):
        __slots__ = ()

    class Params(BaseModel):
        model_config = pydantic.ConfigDict(arbitrary_types_allowed=True)

        token: SecretName
        wait: dt.timedelta

    schema = sp.reflect(
        Params,
        spb.make_id("t", "r", spb.Version.of(1, 0, 0)),
        overrides={
            SecretName: lambda n: b.str_(n).secret().done(),
            # Overrides beat the stdlib branches too.
            dt.timedelta: lambda n: b.int64(n).gte(0).done(),
        },
    )
    token = spb.lookup_path(schema, "token")
    assert token.secret
    assert spb.kind_name(token) == "string"
    wait = spb.lookup_path(schema, "wait")
    assert spb.kind_name(wait) == "int64"


def test_pydantic_version_floor() -> None:
    assert pydantic.VERSION >= "2"
