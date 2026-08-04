"""Lookup conformance (golden lookup.json) + port-local name-rule cases."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

import schemapb as spb
from schemapb._gen.schemapb import Schema, SchemaField, SchemaFieldString, SchemaIdentity

GOLDEN = Path(__file__).parent.parent.parent / "conformance" / "golden"


def _golden_schema() -> Schema:
    return Schema().from_json((GOLDEN / "full-schema.json").read_text())


def _cases() -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = json.loads((GOLDEN / "lookup.json").read_text())["cases"]
    return cases


@pytest.mark.parametrize("case", _cases(), ids=lambda c: repr(c["path"]))
def test_lookup_conformance(case: dict[str, Any]) -> None:
    schema = _golden_schema()
    if "error" in case:
        with pytest.raises(spb.SchemaLookupError) as exc:
            spb.lookup_path(schema, case["path"])
        err = exc.value
        got = {"at": err.at, "segment": err.segment, "reason": err.reason.value}
        assert got == case["error"]
    else:
        assert spb.kind_name(spb.lookup_path(schema, case["path"])) == case["kind"]


def _schema_with_name(name: str) -> Schema:
    return Schema(
        id=SchemaIdentity(namespace="t", name="names"),
        fields=[SchemaField(name=name, string=SchemaFieldString())],
    )


@pytest.mark.parametrize("bad", ["a.b", "my-field", "1st", "in", "true", "while"])
def test_field_name_rejected(bad: str) -> None:
    assert spb.check_descriptor(_schema_with_name(bad))


@pytest.mark.parametrize("good", ["snake_case", "camelCase", "_x", "a1"])
def test_field_name_accepted(good: str) -> None:
    assert spb.check_descriptor(_schema_with_name(good)) == []
