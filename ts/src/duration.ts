/**
 * Go-compatible duration formatting and parsing. The spec's display form for
 * durations is Go's `time.Duration.String()` ("5m0s", "1.5s", "300ms") and
 * string inputs accept Go's `time.ParseDuration` syntax ("2h45m", "-1.5s").
 */

import { create } from "@bufbuild/protobuf";
import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";

const NANOS = 1_000_000_000n;

/** Total nanoseconds of a wkt Duration. */
export function durationNanos(d: Duration): bigint {
  return d.seconds * NANOS + BigInt(d.nanos);
}

/** Builds a wkt Duration from total nanoseconds. */
export function durationFromNanos(total: bigint): Duration {
  let seconds = total / NANOS;
  let nanos = total % NANOS;
  if (nanos < 0n && seconds > 0n) {
    seconds -= 1n;
    nanos += NANOS;
  }
  return create(DurationSchema, { seconds, nanos: Number(nanos) });
}

/** Total nanoseconds of a wkt Timestamp (epoch-relative). */
export function timestampNanos(t: Timestamp): bigint {
  return t.seconds * NANOS + BigInt(t.nanos);
}

/** Builds a wkt Timestamp from epoch milliseconds. */
export function timestampFromMillis(ms: number): Timestamp {
  const seconds = BigInt(Math.floor(ms / 1000));
  const nanos = Math.round((ms - Number(seconds) * 1000) * 1e6);
  return create(TimestampSchema, { seconds, nanos });
}

/**
 * Formats a duration exactly like Go's `time.Duration.String()`.
 */
export function formatGoDuration(d: Duration): string {
  let v = durationNanos(d);
  const neg = v < 0n;
  if (neg) {
    v = -v;
  }

  if (v === 0n) {
    return "0s";
  }

  let out: string;
  if (v < NANOS) {
    // Sub-second: ns, µs or ms with fraction.
    if (v < 1000n) {
      out = `${v}ns`;
    } else if (v < 1_000_000n) {
      out = `${frac(v, 1000n)}µs`;
    } else {
      out = `${frac(v, 1_000_000n)}ms`;
    }
  } else {
    const minuteNs = 60_000_000_000n;
    let s = `${frac(v % minuteNs, NANOS)}s`;
    let rest = v / minuteNs; // whole minutes
    if (rest > 0n) {
      s = `${rest % 60n}m${s}`;
      rest /= 60n;
      if (rest > 0n) {
        s = `${rest}h${s}`;
      }
    }
    out = s;
  }
  return neg ? `-${out}` : out;
}

/** value/unit with trailing-zero-trimmed fraction (Go's fmtFrac). */
function frac(v: bigint, unit: bigint): string {
  const whole = v / unit;
  let fraction = (v % unit).toString().padStart(unit.toString().length - 1, "0");
  fraction = fraction.replace(/0+$/, "");
  return fraction === "" ? `${whole}` : `${whole}.${fraction}`;
}

const UNIT_NANOS = new Map<string, bigint>([
  ["ns", 1n],
  ["us", 1000n],
  ["µs", 1000n],
  ["μs", 1000n],
  ["ms", 1_000_000n],
  ["s", 1_000_000_000n],
  ["m", 60_000_000_000n],
  ["h", 3_600_000_000_000n],
]);

const DURATION_RE = /^([+-])?((\d+(\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h))+$/;
const PART_RE = /(\d+(?:\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h)/g;

/**
 * Parses Go `time.ParseDuration` syntax. Returns undefined on invalid input.
 */
export function parseGoDuration(s: string): Duration | undefined {
  if (s === "0") {
    return create(DurationSchema);
  }
  if (!DURATION_RE.test(s)) {
    return undefined;
  }
  const neg = s.startsWith("-");
  let total = 0n;
  for (const m of s.matchAll(PART_RE)) {
    const num = m[1];
    const unit = m[2];
    if (num === undefined || unit === undefined) {
      return undefined;
    }
    const unitNs = UNIT_NANOS.get(unit);
    if (unitNs === undefined) {
      return undefined;
    }
    const [whole = "0", fracDigits = ""] = num.split(".");
    total += BigInt(whole) * unitNs;
    if (fracDigits !== "") {
      // Fractional part scaled into the unit without float error.
      total += (BigInt(fracDigits) * unitNs) / 10n ** BigInt(fracDigits.length);
    }
  }
  return durationFromNanos(neg ? -total : total);
}

/**
 * Formats a timestamp exactly like Go's `t.Format(time.RFC3339)` in UTC:
 * whole seconds, no fractional part, trailing Z.
 */
export function formatRfc3339(t: Timestamp): string {
  const date = new Date(Number(t.seconds) * 1000);
  return `${date.toISOString().slice(0, 19)}Z`;
}

const RFC3339_RE =
  /^(\d{4})-(\d{2})-(\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$/;

/** Parses an RFC3339 string into a Timestamp. Undefined on invalid input. */
export function parseRfc3339(s: string): Timestamp | undefined {
  if (!RFC3339_RE.test(s)) {
    return undefined;
  }
  const ms = Date.parse(s);
  if (Number.isNaN(ms)) {
    return undefined;
  }
  return timestampFromMillis(ms);
}
