# Errors

Validation never throws for invalid data: it returns a `ValidationResult`
proto (`schemapb/errors.proto`). The proto files are the whole contract — a
client in any language reads an error without this library.

## ValidationError

| field | meaning |
|---|---|
| `code` | stable `ErrorCode`; the contract |
| `path`, `path_segments` | where: field names, list indexes, map keys. A key segment naming a proto field carries `field_number` when the value was a typed proto message |
| `expected`, `actual` | typed `Value`s; their shape per code is in the code's `ErrorSpec`. `actual` is absent on `secret` fields |
| `constraint` | the structured constraint that failed (`gte`, `pattern`, ...) |
| `rule_id`, `expr` | the CEL rule that failed or errored |
| `severity` | `ERROR` blocks; `WARNING` does not |
| `key` | the failure concerns the map **key** that ends `path` (`Map.key_field`), not its value |
| `message` | English text rendered from the template; informative, not contract |

`ValidationResult.schema` is set by typed-message validators (`protomsg`):
the identity of the schema the message was validated against.

## ErrorSpec: the codes document themselves

Every `ErrorCode` value carries an `error_spec` option:

```proto
ERROR_CODE_GTE_VIOLATED = 12 [(error_spec) = {
  summary: "The value is less than the inclusive lower bound."
  template: "must be >= {expected}"
  constraints: "gte"
  expected: PAYLOAD_SCALAR
  actual: PAYLOAD_SCALAR
}];
```

Read it from the enum value descriptor (`schemapb.SpecOf(code)` in Go,
`specOf(code)` in TypeScript, `getOption(value, error_spec)` with any
protobuf runtime that keeps options). `RULE_VIOLATED`, `EXPR_ERROR` and
`INVALID_SCHEMA` have no template: their message is the rule author's text,
the evaluator's or the compiler's.

## Translation

Messages are rendered from a catalog for one locale:

- key: the `ErrorCode` name (`ERROR_CODE_GTE_VIOLATED`), or the rule `id`
  for `RULE_VIOLATED` (`MessageKey` / `messageKey`);
- value: a template with `{expected}` and `{actual}`;
- a missing key falls back to `message`, so a partial translation degrades
  to English, never to nothing.

The English catalog is `conformance/golden/messages.json`; start a
translation from it.

```go
ru := schemapb.Catalog{"ERROR_CODE_GTE_VIOLATED": "должно быть не меньше {expected}"}
text := ru.Message(err, nil) // nil => DisplayValue; pass a locale-aware formatter
```

```ts
const ru = { ERROR_CODE_GTE_VIOLATED: "должно быть не меньше {expected}" };
formatMessage(err, ru, (v) => displayValue(v));
```

Give rules an `id` so their messages can be translated. Field labels for a
message come from the schema (`title`, `description`), found by `path`.

## Transport

`protomsg.Status(err)` (Go) packs the result into a `google.rpc.Status` with
code `INVALID_ARGUMENT` and the `ValidationResult` as its only detail — the
shape gRPC and Connect carry. `protomsg.FromStatus` unpacks it.
