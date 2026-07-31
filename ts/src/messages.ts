/**
 * The spec's shared message-template set (see conformance/golden/
 * messages.json — written by the Go reference; this table must match it,
 * the conformance suite compares them). Placeholders {expected} and {actual}
 * substitute display forms; a missing value renders "…". RULE_VIOLATED /
 * EXPR_ERROR / INVALID_SCHEMA carry free text and are not templated.
 */

import { ErrorCode } from "./gen/schemapb/errors_pb.js";
import type { Value } from "./gen/schemapb/value_pb.js";
import { displayString } from "./render.js";
import { toNative } from "./value.js";

export const messageTemplates: ReadonlyMap<ErrorCode, string> = new Map([
  [ErrorCode.TYPE_MISMATCH, "expected {expected}"],
  [ErrorCode.REQUIRED_MISSING, "required"],
  [ErrorCode.NOT_NULLABLE, "must not be null"],
  [ErrorCode.UNKNOWN_FIELD, "unknown field"],
  [ErrorCode.IMMUTABLE_MODIFIED, "immutable: cannot be changed"],

  [ErrorCode.CONST_MISMATCH, "must equal {expected}"],
  [ErrorCode.GT_VIOLATED, "must be > {expected}"],
  [ErrorCode.GTE_VIOLATED, "must be >= {expected}"],
  [ErrorCode.LT_VIOLATED, "must be < {expected}"],
  [ErrorCode.LTE_VIOLATED, "must be <= {expected}"],
  [ErrorCode.NOT_IN_ALLOWED_SET, "must be one of {expected}"],
  [ErrorCode.IN_FORBIDDEN_SET, "must not be one of {expected}"],
  [ErrorCode.MULTIPLE_OF_VIOLATED, "must be a multiple of {expected}"],

  [ErrorCode.LEN_MISMATCH, "length must be exactly {expected}"],
  [ErrorCode.MIN_LEN_VIOLATED, "length must be at least {expected}"],
  [ErrorCode.MAX_LEN_VIOLATED, "length must be at most {expected}"],
  [ErrorCode.PATTERN_MISMATCH, "must match pattern {expected}"],
  [ErrorCode.FORMAT_MISMATCH, "must be a valid {expected}"],
  [ErrorCode.PREFIX_MISMATCH, "must start with {expected}"],
  [ErrorCode.SUFFIX_MISMATCH, "must end with {expected}"],
  [ErrorCode.UNSUPPORTED_FORMAT, "format {expected} is not supported by this implementation"],

  [ErrorCode.CHOICE_NOT_ALLOWED, "must be one of {expected}"],

  [ErrorCode.MIN_ITEMS_VIOLATED, "must have at least {expected} items"],
  [ErrorCode.MAX_ITEMS_VIOLATED, "must have at most {expected} items"],
  [ErrorCode.NOT_UNIQUE, "must be unique"],
  [ErrorCode.LIST_COUNT_MISMATCH, "must have exactly {expected} items"],

  [ErrorCode.MIN_ENTRIES_VIOLATED, "must have at least {expected} entries"],
  [ErrorCode.MAX_ENTRIES_VIOLATED, "must have at most {expected} entries"],

  [ErrorCode.MIN_PROPERTIES_VIOLATED, "must have at least {expected} properties"],
  [ErrorCode.MAX_PROPERTIES_VIOLATED, "must have at most {expected} properties"],

  [ErrorCode.DISCRIMINATOR_MISSING, "discriminator {expected} must be a non-empty string"],
  [ErrorCode.UNKNOWN_VARIANT, "unknown variant {actual}"],
  [ErrorCode.UNKNOWN_REF, "unknown $ref {expected}"],
]);

/** Renders a code's template with the typed values. */
export function renderMessage(
  code: ErrorCode,
  expected: Value | undefined,
  actual: Value | undefined,
): string {
  const tpl = messageTemplates.get(code);
  if (tpl === undefined) {
    return "";
  }
  const sub = (s: string, ph: string, v: Value | undefined): string => {
    if (!s.includes(ph)) {
      return s;
    }
    const disp = v === undefined ? "…" : displayString(toNative(v));
    return s.replaceAll(ph, disp);
  };
  return sub(sub(tpl, "{expected}", expected), "{actual}", actual);
}
