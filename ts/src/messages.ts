/**
 * The spec's shared message-template set, read from the `error_spec` option
 * on each ErrorCode value in errors.proto (conformance/golden/messages.json
 * is the Go export of the same set; the conformance suite compares them).
 * Placeholders {expected} and {actual} substitute display forms; a missing
 * value renders "…". RULE_VIOLATED / EXPR_ERROR / INVALID_SCHEMA carry free
 * text and are not templated.
 */

import { getOption, hasOption } from "@bufbuild/protobuf";
import {
  ErrorCode,
  ErrorCodeSchema,
  type ErrorSpec,
  error_spec,
  type ValidationError,
} from "./gen/schemapb/errors_pb.js";
import type { Value } from "./gen/schemapb/value_pb.js";
import { displayString } from "./render.js";
import { toNative } from "./value.js";

const errorSpecs: ReadonlyMap<ErrorCode, ErrorSpec> = new Map(
  ErrorCodeSchema.values
    .filter((v) => hasOption(v, error_spec))
    .map((v) => [v.number as ErrorCode, getOption(v, error_spec)]),
);

/** The ErrorSpec declared for code in errors.proto; undefined for UNSPECIFIED. */
export function specOf(code: ErrorCode): ErrorSpec | undefined {
  return errorSpecs.get(code);
}

export const messageTemplates: ReadonlyMap<ErrorCode, string> = new Map(
  [...errorSpecs]
    .filter(([, spec]) => spec.template !== "")
    .map(([code, spec]) => [code, spec.template]),
);

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
  return fillTemplate(tpl, expected, actual, displayValue);
}

function fillTemplate(
  tpl: string,
  expected: Value | undefined,
  actual: Value | undefined,
  display: (v: Value) => string,
): string {
  const sub = (s: string, ph: string, v: Value | undefined): string => {
    if (!s.includes(ph)) {
      return s;
    }
    const disp = v === undefined ? "…" : display(v);
    return s.replaceAll(ph, disp);
  };
  return sub(sub(tpl, "{expected}", expected), "{actual}", actual);
}

/** The spec's display form of a value, used by the default messages. */
export function displayValue(v: Value): string {
  return displayString(toNative(v));
}

/**
 * A message catalog for one locale, the unit of translation. Keys are
 * messageKey values: an ErrorCode name ("ERROR_CODE_GTE_VIOLATED") for
 * templated codes, the rule id for RULE_VIOLATED. Templates use the
 * {expected} and {actual} placeholders of ErrorSpec.template; the English
 * catalog is conformance/golden/messages.json.
 */
export type Catalog = Readonly<Record<string, string>>;

/** The catalog key that translates err. */
export function messageKey(err: ValidationError): string {
  if (err.code === ErrorCode.RULE_VIOLATED && err.ruleId) {
    return err.ruleId;
  }
  return `ERROR_CODE_${ErrorCode[err.code]}`;
}

/**
 * Renders err from catalog, substituting the typed values with display
 * (default displayValue; pass a locale-aware one for numbers and times). A
 * key absent from the catalog falls back to err.message.
 */
export function formatMessage(
  err: ValidationError,
  catalog: Catalog,
  display: (v: Value) => string = displayValue,
): string {
  const tpl = catalog[messageKey(err)];
  if (tpl === undefined) {
    return err.message;
  }
  return fillTemplate(tpl, err.expected, err.actual, display);
}
