import datetime as dt

import pytest

import schemapb as spb
from schemapb import builder as b
from schemapb._gen.schemapb import ErrorCode, SchemaFieldResultType
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
