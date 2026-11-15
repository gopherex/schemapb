import datetime as dt

import pytest

import schemapb as spb
from schemapb import builder as b
from schemapb._gen.schemapb import ErrorCode, SchemaFieldResultType
from schemapb.render import display_string
from schemapb.typed import Version, make_id
from schemapb.value import str_v


def test_builder_end_to_end() -> None:
    _schema, engine = (
        b.new_schema(make_id("infra", "pg", Version.of(1, 0, 0)))
        .fields(
            b.int64("shared_buffers").gte(16).default(128).unit("MB").group("Memory"),
            b.bool_("autovacuum").default(v=True).group("Vacuum"),
            b.choice("wal_level")
            .opt(str_v("minimal"), "Minimal")
            .opt(str_v("replica"), "Replica")
            .default(str_v("replica")),
            b.computed("cache", "root.shared_buffers * 3").result(SchemaFieldResultType.INT64),
            b.duration("timeout").default(dt.timedelta(minutes=5)),
            b.str_("mode").default("Fast").normalize("this.lowerAscii()").in_("fast", "slow"),
        )
        .rules(b.rule("int(root.shared_buffers) < 100000", "too big").warn())
        .build()
    )

    res = spb.validate(engine, {"shared_buffers": 8})
    assert any(e.path == "shared_buffers" and e.code == ErrorCode.GTE_VIOLATED for e in res.errors)

    outcome = spb.bake(engine, {})
    assert outcome.baked is not None
    values = outcome.baked.values.fields
    assert values["shared_buffers"].int64_value == 128
    assert values["cache"].int64_value == 384
    assert values["mode"].string_value == "fast"
    assert spb.choice_options(engine, "wal_level", {}) == ["minimal", "replica"]


def test_broken_schema_raises() -> None:
    with pytest.raises(spb.SchemaError):
        b.new_schema(make_id("t", "cycle", Version.of(0, 1, 0))).fields(
            b.computed("a", "root.b + 1"),
            b.computed("b", "root.a + 1"),
        ).build()


_NAN, _INF = float("nan"), float("inf")


@pytest.fixture(scope="module")
def nonfinite_engine() -> spb.Engine:
    _schema, engine = (
        b.new_schema(make_id("t", "nonfinite", Version.of(1, 0, 0)))
        .fields(
            b.double("gt").gt(0),
            b.double("gte").gte(0),
            b.float_("lt").lt(10),
            b.float_("lte").lte(10),
            b.double("fin").finite(),
            b.float_("fin32").finite().gte(1),
            b.int64("i").gt(0).lte(3),
        )
        .build()
    )
    return engine


# A bound holds only when its comparison is true: NaN violates every bound,
# an infinity only the bound on its side; finite rejects NaN and the infinities.
@pytest.mark.parametrize(
    ("path", "val", "want"),
    [
        ("gt", _NAN, [ErrorCode.GT_VIOLATED]),
        ("gte", _NAN, [ErrorCode.GTE_VIOLATED]),
        ("lt", _NAN, [ErrorCode.LT_VIOLATED]),
        ("lte", _NAN, [ErrorCode.LTE_VIOLATED]),
        ("gte", _INF, []),
        ("gte", -_INF, [ErrorCode.GTE_VIOLATED]),
        ("lte", _INF, [ErrorCode.LTE_VIOLATED]),
        ("gte", 0.0, []),
        ("fin", 1e308, []),
        ("fin", _NAN, [ErrorCode.NOT_FINITE]),
        ("fin", _INF, [ErrorCode.NOT_FINITE]),
        ("fin", -_INF, [ErrorCode.NOT_FINITE]),
        ("fin32", _NAN, [ErrorCode.NOT_FINITE, ErrorCode.GTE_VIOLATED]),
        ("i", 0, [ErrorCode.GT_VIOLATED]),
        ("i", 3, []),
        ("i", 4, [ErrorCode.LTE_VIOLATED]),
    ],
)
def test_float_bounds_and_finite(
    nonfinite_engine: spb.Engine, path: str, val: float, want: list[ErrorCode]
) -> None:
    res = spb.validate(nonfinite_engine, {path: val})
    assert [e.code for e in res.errors if e.path == path] == want


# NaN and the infinities display in protoJSON spelling, as JSON strings inside containers.
@pytest.mark.parametrize(
    ("v", "want"),
    [
        (_NAN, "NaN"),
        (_INF, "Infinity"),
        (-_INF, "-Infinity"),
        (1.5, "1.5"),
        ([-_INF, 2.0], '["-Infinity",2]'),
    ],
)
def test_non_finite_display(v: spb.Native, want: str) -> None:
    assert display_string(v) == want
