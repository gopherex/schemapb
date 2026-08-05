"""Value conformance: the As matrix (value-as.json) and value path lookup
(value-lookup.json).

Python's type tokens are int/float/str/bytes/timedelta/datetime/list/dict:
the language has no int32/uint32/float32, so range-constrained golden
targets map onto the unbounded ``int`` (resp. double ``float``) token. The
exactness core is asserted for every case; refusals that exist ONLY
because of the target's range (impossible to express with an unbounded
token) are listed explicitly below.
"""

from __future__ import annotations

import datetime as dt
import json
from pathlib import Path
from typing import Any

import betterproto2
import pytest

import schemapb as spb
from schemapb._gen.schemapb import StructValue, Value

GOLDEN = Path(__file__).parent.parent.parent / "conformance" / "golden"

# (source-json, target) pairs whose golden refusal is range-only: Python's
# unbounded int / double-view float extracts them anyway.
RANGE_ONLY_REFUSALS = {
    ('{"uint64Value": "18446744073709551615"}', "int64"),
    ('{"int64Value": "-1"}', "uint64"),
    ('{"int64Value": "-1"}', "uint32"),
    ('{"doubleValue": 0.1}', "float"),
}

_INT_TARGETS = {"int32", "int64", "uint32", "uint64"}
_TOKENS: dict[str, type] = {
    "bool": bool,
    "float": float,
    "double": float,
    "string": str,
    "bytes": bytes,
    "duration": dt.timedelta,
    "timestamp": dt.datetime,
    "list": list,
    "struct": dict,
}


def _cases(name: str) -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = json.loads((GOLDEN / name).read_text())["cases"]
    return cases


@pytest.mark.parametrize(
    "case", _cases("value-as.json"), ids=lambda c: f"{c['target']}<-{json.dumps(c['value'])}"
)
def test_value_as_conformance(case: dict[str, Any]) -> None:
    v = Value().from_json(json.dumps(case["value"]))
    target: str = case["target"]
    token = int if target in _INT_TARGETS else _TOKENS[target]
    got = spb.value_as(v, token)

    if (json.dumps(case["value"]), target) in RANGE_ONLY_REFUSALS:
        assert got is not None, "range-only refusal: unbounded token must extract"
        return

    if "result" not in case:
        assert got is None, f"want refusal, got {got!r}"
        return

    want = Value().from_json(json.dumps(case["result"]))
    if target in _INT_TARGETS:
        assert got == _int_of(want)
    elif target == "struct":
        assert isinstance(got, dict)
        assert {k: x.to_json() for k, x in got.items()} == {
            k: x.to_json()
            for k, x in (want.struct_value.fields if want.struct_value else {}).items()
        }
    elif target == "list":
        assert isinstance(got, list)
        assert [x.to_json() for x in got] == [
            x.to_json() for x in (want.list_value.items if want.list_value else [])
        ]
    else:
        assert got == _scalar_of(want)


def _int_of(v: Value) -> int:
    n = spb.value_as(v, int)
    assert n is not None
    return n


def _scalar_of(v: Value) -> object:
    _, val = betterproto2.which_one_of(v, "kind")
    return val


@pytest.mark.parametrize("case", _cases("value-lookup.json"), ids=lambda c: repr(c["path"]))
def test_value_lookup_conformance(case: dict[str, Any]) -> None:
    values = StructValue().from_json((GOLDEN / "full-baked.json").read_text())
    if "error" in case:
        with pytest.raises(spb.ValueLookupError) as exc:
            spb.value_lookup(values, case["path"])
        err = exc.value
        got = {"at": err.at, "segment": err.segment, "reason": err.reason.value}
        assert got == case["error"]
    else:
        v = spb.value_lookup(values, case["path"])
        assert json.loads(v.to_json()) == case["value"]
