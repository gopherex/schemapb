# schemapb (Python)

Native Python implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

## Install

```sh
pip install schemapb
```

Requires Python ≥ 3.11. Runtime dependencies: `betterproto2` (protobuf
dataclasses), `cel-python` (CEL evaluation), `chevron` (Mustache).

## Quickstart

```python
import schemapb as spb

# build() fully compiles: every schema defect raises SchemaError.
schema, engine = (
    spb.new_schema(spb.make_id("shared", "service", spb.Version.of(1, 0, 0)))
    .fields(
        spb.str_("name").required().min_len(1),
        spb.int64("replicas").default(1).gte(1).lte(9),
        spb.computed("memory_mb", "root.replicas * 256"),
    )
    .template("conf", "{{values.name}}: {{values.memory_mb}}MB")
    .build()  # schema is a plain proto message — ship it anywhere
)

result = engine.validate(values)      # ValidationResult — errors as data
outcome = engine.bake(values)         # canonical Baked snapshot
text = engine.render("conf", values)  # Mustache template from the schema

# Module-level forms of every operation stay importable for functional
# style: spb.validate(engine, values) etc.
```

Schemas arriving over the wire compile with `spb.compile_schema(schema)`.

## Reflecting a pydantic model (`pip install schemapb[pydantic]`)

```python
import schemapb.pydantic as sp

schema = sp.reflect(MyModel, spb.make_id("shared", "service", spb.Version.of(1, 0, 0)))
```

Field types, `X | None`, defaults, `Field(...)` constraints,
`Literal[...]` choices, nested models and `dict[str, V]` maps all carry
over; markers (`sp.Int32`, `sp.UInt64`, `sp.exact_len`, `sp.fmt`) close
the gaps Python's type system cannot express. Pinned by
conformance/golden/reflect.json.

## Using schemapb types from your own protos

If your `.proto` files embed schemapb messages and you generate them with
betterproto2, your gen tree will contain its own `schemapb/` package.
Replace it with a one-line shim so your generated code uses this package's
classes:

```python
# <your gen root>/schemapb/__init__.py
from schemapb.pb import *  # noqa: F403
```

`schemapb.pb` is the public, stable re-export of the generated protobuf
types (the internal `_gen` layout is not a stability promise).

## Development

From the repository root: `make configure` once, then `make lint-py`
(Ruff `select = ALL` + strict mypy) / `make test-py` (pytest). Tooling runs
through a pinned `uv` in `./bin`; consumers still install with plain pip.

Known deviations (tracked):

- durations carry microsecond resolution (`datetime.timedelta`), not the
  contract's nanoseconds; sub-microsecond durations round.
- no CEL evaluation cost limit: `cel-python` does not expose one (the Go
  reference has `WithCostLimit`).
- `value_as` type tokens are Python's own: `int` is unbounded (covers the
  full int64 and uint64 wire ranges) and there is no float32 token — the
  range-constrained refusals other implementations report cannot be
  expressed with these tokens.
