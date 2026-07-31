/**
 * The spec's string-format registry: core formats every implementation MUST
 * support, extensible per engine with compile options. An unknown format
 * fails validation loudly (UNSUPPORTED_FORMAT) — never a silent pass.
 */

import type { Format } from "./typed.js";

export type FormatFunc = (s: string) => boolean;

export type FormatRegistry = Map<Format, FormatFunc>;

const UUID_RE = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
const HOSTNAME_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/i;
const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const TIME_RE = /^\d{2}:\d{2}:\d{2}$/;
const RFC3339_FULL_RE = /^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$/;
// The pragmatic address shape of Go's mail.ParseAddress: one @, non-empty
// local part and domain, no whitespace.
const EMAIL_RE = /^[^@\s]+@[^@\s]+$/;

function isIpv4(s: string): boolean {
  const m = IPV4_RE.exec(s);
  if (m === null) {
    return false;
  }
  return m.slice(1).every((oct) => {
    if (oct === undefined || (oct.length > 1 && oct.startsWith("0"))) {
      return false;
    }
    const n = Number(oct);
    return n >= 0 && n <= 255;
  });
}

function isIpv6(s: string): boolean {
  if (!s.includes(":") || s.includes(" ")) {
    return false;
  }
  const doubleColons = s.split("::").length - 1;
  if (doubleColons > 1) {
    return false;
  }
  const groups = s.split(/::|:/).filter((g) => g !== "");
  if (groups.length > 8) {
    return false;
  }
  if (doubleColons === 0 && groups.length !== 8) {
    return false;
  }
  return groups.every((g) => /^[0-9a-f]{1,4}$/i.test(g) || isIpv4(g));
}

function isValidDate(s: string): boolean {
  if (!DATE_RE.test(s)) {
    return false;
  }
  const [y = 0, m = 0, d = 0] = s.split("-").map(Number);
  const date = new Date(Date.UTC(y, m - 1, d));
  return date.getUTCFullYear() === y && date.getUTCMonth() === m - 1 && date.getUTCDate() === d;
}

function isValidTime(s: string): boolean {
  if (!TIME_RE.test(s)) {
    return false;
  }
  const [h = -1, m = -1, sec = -1] = s.split(":").map(Number);
  return h >= 0 && h <= 23 && m >= 0 && m <= 59 && sec >= 0 && sec <= 59;
}

/** A fresh registry with the spec's mandatory core formats. */
export function coreFormats(): FormatRegistry {
  return new Map<Format, FormatFunc>([
    ["email" as Format, (s) => EMAIL_RE.test(s)],
    [
      "url" as Format,
      (s) => {
        try {
          new URL(s);
          return true;
        } catch {
          return s.startsWith("/") && !s.includes(" ");
        }
      },
    ],
    ["uuid" as Format, (s) => UUID_RE.test(s)],
    ["ipv4" as Format, isIpv4],
    ["ipv6" as Format, (s) => isIpv6(s) && !isIpv4(s)],
    ["ip" as Format, (s) => isIpv4(s) || isIpv6(s)],
    ["hostname" as Format, (s) => s.length <= 253 && HOSTNAME_RE.test(s)],
    ["date" as Format, isValidDate],
    ["time" as Format, isValidTime],
    ["datetime" as Format, (s) => RFC3339_FULL_RE.test(s) && !Number.isNaN(Date.parse(s))],
  ]);
}

/** Options for compile(). */
export interface CompileOptions {
  /**
   * Extra format checkers layered over the core registry. Namespaced ids
   * ("k8s.quantity") for extensions.
   */
  formats?: ReadonlyMap<Format, FormatFunc> | Record<string, FormatFunc>;
}
