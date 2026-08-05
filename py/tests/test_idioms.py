"""The idiomatic surface: Engine methods and Version ordering operators
delegate to the same machinery the functional forms use."""

from __future__ import annotations

import schemapb as spb
from schemapb import builder as b


def _engine() -> spb.Engine:
    _, engine = (
        b.new_schema(spb.make_id("t", "idioms", spb.Version.of(1, 0, 0)))
        .fields(
            b.str_("name").required().min_len(1),
            b.int64("replicas").default(1).gte(1),
            b.computed("memory_mb", "root.replicas * 256"),
        )
        .template("conf", "{{values.name}}: {{values.memory_mb}}MB")
        .build()
    )
    return engine


def test_engine_methods() -> None:
    engine = _engine()

    result = engine.validate({"name": "svc"})
    assert result.errors == []

    outcome = engine.bake({"name": "svc"})
    assert outcome.baked is not None

    assert engine.render_baked(outcome.baked, "conf") == "svc: 256MB"

    merged = engine.merge(outcome.baked, spb.struct_from_native({"replicas": 3}))
    assert merged.baked is not None
    assert engine.render_baked(merged.baked, "conf") == "svc: 768MB"


def test_version_ordering() -> None:
    v1, v2 = spb.Version.of(1, 2, 3), spb.Version.of(1, 10, 0)
    assert v1 < v2
    assert v2 > v1
    assert v1 <= spb.Version.parse("v1.2.3")
    assert spb.Version() < v1  # zero (unversioned) sorts first
