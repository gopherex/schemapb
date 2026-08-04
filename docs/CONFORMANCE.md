# Conformance

The four implementations share no runtime. What keeps them identical is this
suite: the **Go implementation is the reference** — it writes golden
fixtures, and TypeScript, Python and Rust must reproduce them exactly
(protoJSON compared as messages, rendered text compared byte-for-byte).

## Golden files (`conformance/golden/`)

| File | What it pins |
|---|---|
| `full-schema.json` | The kitchen-sink schema: every proto field of the contract populated (protoJSON). Ports load it as their test input. |
| `full-baked.json` | `Bake` of a valid input against the full schema: defaults seeded, `"256"`-style coercions applied, computed values resolved, canonical typed `Value` forms. |
| `full-errors.json` | `Validate` of a deliberately broken input: every error with `path`, `ErrorCode`, expected/actual, severity — in the deterministic order the spec requires. |
| `full-rendered.txt` | Mustache render of the schema's template against the resolved values — byte-equal, including escaping and Go-style duration display (`5m0s`). |
| `messages.json` | The spec-owned message-template set, per `ErrorCode`. |
| `lookup.json` | Schema path lookup over the kitchen-sink schema: each case pins either the resolved field's kind or the failing `(at, segment, reason)` triple. Error message texts are NOT pinned — each language words its lookup error idiomatically; the triple is the contract. |
| `full-coverage.json` | Which contract features the kitchen-sink schema exercises — a checklist that the goldens stay exhaustive. |

The valid/broken inputs are defined in `go/schemapb/golden_test.go` and
mirrored verbatim in each port's conformance test
(`ts/test/conformance.test.ts`, `py/tests/test_conformance.py`,
`rust/tests/conformance.rs`).

## Regenerating

Only after an intentional behaviour change, from `go/`:

```sh
go test ./schemapb -run Golden -update
```

Then run every other language's suite (`make test-ts test-py test-rust`) —
each must go green against the new goldens before the change lands. A golden
diff in review **is** the cross-language behaviour diff.

## What byte-equality forces

- deterministic error ordering and identical message texts,
- one display form per value kind (RFC 3339 whole-second timestamps,
  Go-style durations, base64 bytes, sorted struct keys),
- ASCII-only casing helpers (locale-independent),
- identical CEL semantics including the strings extension,
- identical Mustache escaping and render-context shape,
- secret masking that re-renders messages instead of leaking values.
