"""Display masking uses the same wire fixtures in every language."""

import json
from copy import deepcopy
from datetime import timedelta
from pathlib import Path

import pytest

from schemapb import Baked, StructValue, Value, masked

CASES = json.loads((Path(__file__).parents[2] / "conformance/golden/masked.json").read_text())


@pytest.mark.parametrize("case", CASES, ids=[c["name"] for c in CASES])
def test_masked(case: dict[str, object]) -> None:
    baked = Baked().from_json(json.dumps(case["baked"]))
    before = deepcopy(baked)
    want = StructValue().from_json(json.dumps(case["masked"]))
    out = masked(baked)
    assert out == want
    assert baked == before
    for value in out.fields.values():
        mutate(value)
    assert baked == before
    assert masked(baked) == want


def mutate(value: Value) -> None:
    if value.struct_value is not None:
        for item in value.struct_value.fields.values():
            mutate(item)
    if value.list_value is not None:
        for item in value.list_value.items:
            mutate(item)
    if value.duration_value is not None:
        value.duration_value += timedelta(seconds=1)
    if value.timestamp_value is not None:
        value.timestamp_value += timedelta(seconds=1)
    value.string_value = "changed"
