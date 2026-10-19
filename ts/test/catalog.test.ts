import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { ErrorCode, ValidationErrorSchema } from "../src/gen/schemapb/errors_pb.js";
import { displayValue, formatMessage, messageKey } from "../src/messages.js";
import { int64V } from "../src/value.js";

describe("catalog", () => {
  const ru = {
    ERROR_CODE_GTE_VIOLATED: "должно быть не меньше {expected} (сейчас {actual})",
    cross: "i64 должно покрывать i32",
  };
  const gte = create(ValidationErrorSchema, {
    code: ErrorCode.GTE_VIOLATED,
    expected: int64V(1n),
    actual: int64V(0n),
    message: "must be >= 1",
  });
  const rule = create(ValidationErrorSchema, {
    code: ErrorCode.RULE_VIOLATED,
    ruleId: "cross",
    message: "i64 must cover i32",
  });
  const untranslated = create(ValidationErrorSchema, {
    code: ErrorCode.REQUIRED_MISSING,
    message: "required",
  });

  it("keys and renders like the Go catalog", () => {
    expect(messageKey(gte)).toBe("ERROR_CODE_GTE_VIOLATED");
    expect(messageKey(rule)).toBe("cross");
    expect(messageKey(untranslated)).toBe("ERROR_CODE_REQUIRED_MISSING");
    expect(formatMessage(gte, ru)).toBe("должно быть не меньше 1 (сейчас 0)");
    expect(formatMessage(rule, ru)).toBe("i64 должно покрывать i32");
    expect(formatMessage(untranslated, ru)).toBe("required");
    expect(formatMessage(gte, ru, (v) => `<${displayValue(v)}>`)).toBe(
      "должно быть не меньше <1> (сейчас <0>)",
    );
  });
});
