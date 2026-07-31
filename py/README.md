# schemapb (Python)

Native Python implementation of the schemapb contract: a runtime,
proto-defined form/config schema descriptor with validation, CEL-computed
values and Mustache rendering. Behaviour is pinned by the cross-language
conformance suite (`conformance/golden` in the repository root); the Go
implementation is the reference.

```python
import schemapb as spb

engine = spb.compile_schema(schema)          # throws SchemaError on defects
result = spb.validate(engine, values)        # ValidationResult (data, not exceptions)
outcome = spb.bake(engine, values)           # canonical Baked snapshot
text = spb.render(engine, "conf", values)    # Mustache template from the schema
```

Runtime dependencies: `betterproto2` (protobuf dataclasses), `cel-python`
(CEL evaluation), `chevron` (Mustache). Install with plain
`pip install schemapb`.

Known deviations (tracked):
- durations carry microsecond resolution (`datetime.timedelta`), not the
  contract's nanoseconds; sub-microsecond durations round.
