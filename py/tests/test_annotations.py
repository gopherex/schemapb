"""Opaque annotations survive serialization and never change engine behavior."""

import json
from pathlib import Path

from schemapb import Baked, Schema, StructValue, compile_schema, masked, struct_to_native
from schemapb._gen.schemapb import ResolveReport


def test_annotations() -> None:
    doc = json.loads(
        (Path(__file__).parents[2] / "conformance/golden/annotations.json").read_text()
    )
    schema = Schema().from_json(json.dumps(doc["schema"]))
    assert Schema().parse(bytes(schema)) == schema
    assert Schema().from_json(schema.to_json()) == schema
    engine = compile_schema(schema)
    outcome = engine.bake_detailed(
        struct_to_native(StructValue().from_json(json.dumps(doc["input"])))
    )
    assert not outcome.result.errors
    assert outcome.baked == Baked().from_json(json.dumps(doc["baked"]))
    assert outcome.report == ResolveReport().from_json(json.dumps(doc["report"]))
    assert outcome.baked is not None
    assert masked(outcome.baked) == StructValue().from_json(json.dumps(doc["masked"]))
    assert schema == Schema().from_json(json.dumps(doc["schema"]))
