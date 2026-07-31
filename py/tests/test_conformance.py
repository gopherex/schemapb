"""The conformance runner: identical results to the Go reference goldens."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

import schemapb as spb
from schemapb._gen.schemapb import ErrorCode, Schema, StructValue, ValidationResult

GOLDEN = Path(__file__).parent.parent.parent / "conformance" / "golden"


def golden(name: str) -> str:
    return (GOLDEN / name).read_text()


@pytest.fixture(scope="module")
def engine() -> spb.Engine:
    schema = Schema().from_json(golden("full-schema.json"))
    return spb.compile_schema(schema, formats={"x.nonempty": lambda v: v != ""})


def valid_input() -> spb.NativeStruct:
    return {
        "i64": "256",  # coerced
        "mail": "dba@corp.io",
        "token": "s3cret-token",
        "magic": b"\xde\xad",
        "replica_count": 1,
        "replicas": [{"name": "r1"}],
        "tablespaces": {"main": {"location": "/var/lib/ts"}},
        "backup": {"type": "s3", "bucket": "backups"},
        "data_volume": {"path": "/data"},
        "region": "somewhere-else",
        "endpoint_pair": ["db1", 5432],
    }


def broken_input() -> spb.NativeStruct:
    return {
        "f32": 0.25,
        "f64": 2.0,
        "i32": 5,
        "i64": 8,
        "u32": 3,
        "u64": 0,
        "pinned": False,
        "name": "Bad Name!",
        "mode": "legacy",
        "exact": "abcde",
        "mail": "not-an-email",
        "token": "short",
        "license": b"XX",
        "magic": b"\x00",
        "wal_level": "extreme",
        "cpu": 3,
        "timeout": "3h",
        "not_before": "2020-01-01T00:00:00Z",
        "replica_count": 2,
        "replicas": [{"name": "r1"}, {"name": "r1"}, {"weight": 2}],
        "logging": {"collector": True, "junk": 1},
        "tablespaces": {"bad": {}},
        "backup": {"type": "tape"},
        "data_volume": {"path": "/data", "size_gb": 0},
        "garbage": 1,
        "endpoint_pair": ["", "not-a-port"],
    }


def test_bakes_valid_input(engine: spb.Engine) -> None:
    outcome = spb.bake(engine, valid_input())
    hard = [e for e in outcome.result.errors if e.code != ErrorCode.RULE_VIOLATED]
    assert hard == []
    assert outcome.baked is not None
    want = StructValue().from_json(golden("full-baked.json"))
    got = outcome.baked.values
    assert got is not None
    assert sorted(got.fields) == sorted(want.fields)
    for name in want.fields:
        assert got.fields[name] == want.fields[name], name


def test_broken_input_errors(engine: spb.Engine) -> None:
    got = spb.validate(engine, broken_input())
    want = ValidationResult().from_json(golden("full-errors.json"))

    def key(e: spb.ValidationError) -> str:
        return f"{e.path}:{e.code!r}"

    assert [key(e) for e in got.errors] == [key(e) for e in want.errors]
    for g, w in zip(got.errors, want.errors, strict=True):
        assert g == w, f"{g.path}:{g.code!r}"


def test_rendered(engine: spb.Engine) -> None:
    outcome = spb.bake(engine, valid_input())
    assert outcome.baked is not None
    conf = spb.render_baked(engine, outcome.baked, "conf")
    report = spb.render_baked(engine, outcome.baked, "report")
    assert f"{conf}---\n{report}" == golden("full-rendered.txt")


def test_message_templates() -> None:
    want = json.loads(golden("messages.json"))
    got = {f"ERROR_CODE_{code.name}": tpl for code, tpl in spb.MESSAGE_TEMPLATES.items()}
    assert got == want
