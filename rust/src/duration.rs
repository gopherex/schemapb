//! Go-compatible duration formatting/parsing and RFC3339 timestamps.

use pbjson_types::{Duration, Timestamp};

const NANOS: i128 = 1_000_000_000;

#[must_use]
pub fn duration_nanos(d: &Duration) -> i128 {
    i128::from(d.seconds) * NANOS + i128::from(d.nanos)
}

#[must_use]
#[allow(clippy::cast_possible_truncation)]
pub const fn duration_from_nanos(total: i128) -> Duration {
    let mut seconds = total / NANOS;
    let mut nanos = total % NANOS;
    if nanos < 0 && seconds > 0 {
        seconds -= 1;
        nanos += NANOS;
    }
    Duration {
        seconds: seconds as i64,
        nanos: nanos as i32,
    }
}

#[must_use]
pub fn timestamp_nanos(t: &Timestamp) -> i128 {
    i128::from(t.seconds) * NANOS + i128::from(t.nanos)
}

/// Formats a duration exactly like Go's `time.Duration.String()`.
#[must_use]
pub fn format_go_duration(d: &Duration) -> String {
    let mut v = duration_nanos(d);
    let neg = v < 0;
    if neg {
        v = -v;
    }
    if v == 0 {
        return "0s".to_owned();
    }
    let out = if v < NANOS {
        if v < 1_000 {
            format!("{v}ns")
        } else if v < 1_000_000 {
            format!("{}µs", frac(v, 1_000))
        } else {
            format!("{}ms", frac(v, 1_000_000))
        }
    } else {
        let minute = 60 * NANOS;
        let mut s = format!("{}s", frac(v % minute, NANOS));
        let mut rest = v / minute;
        if rest > 0 {
            s = format!("{}m{s}", rest % 60);
            rest /= 60;
            if rest > 0 {
                s = format!("{rest}h{s}");
            }
        }
        s
    };
    if neg {
        format!("-{out}")
    } else {
        out
    }
}

fn frac(v: i128, unit: i128) -> String {
    let whole = v / unit;
    let width = unit.to_string().len() - 1;
    let fraction = format!("{:0width$}", v % unit);
    let trimmed = fraction.trim_end_matches('0');
    if trimmed.is_empty() {
        whole.to_string()
    } else {
        format!("{whole}.{trimmed}")
    }
}

/// Parses Go `time.ParseDuration` syntax. `None` on invalid input.
#[must_use]
pub fn parse_go_duration(s: &str) -> Option<Duration> {
    if s == "0" {
        return Some(Duration::default());
    }
    let (neg, mut rest) = s
        .strip_prefix('-')
        .map_or_else(|| (false, s.strip_prefix('+').unwrap_or(s)), |r| (true, r));
    if rest.is_empty() {
        return None;
    }
    let mut total: i128 = 0;
    while !rest.is_empty() {
        let num_len = rest
            .find(|c: char| !c.is_ascii_digit() && c != '.')
            .unwrap_or(rest.len());
        if num_len == 0 {
            return None;
        }
        let (num, tail) = rest.split_at(num_len);
        let unit_len = tail
            .find(|c: char| c.is_ascii_digit() || c == '.')
            .unwrap_or(tail.len());
        let (unit, next) = tail.split_at(unit_len);
        let unit_ns: i128 = match unit {
            "ns" => 1,
            "us" | "µs" | "μs" => 1_000,
            "ms" => 1_000_000,
            "s" => NANOS,
            "m" => 60 * NANOS,
            "h" => 3_600 * NANOS,
            _ => return None,
        };
        let (whole, frac_digits) = num.split_once('.').unwrap_or((num, ""));
        let whole_n: i128 = if whole.is_empty() {
            0
        } else {
            whole.parse().ok()?
        };
        total += whole_n * unit_ns;
        if !frac_digits.is_empty() {
            let frac_n: i128 = frac_digits.parse().ok()?;
            total += frac_n * unit_ns / 10_i128.pow(u32::try_from(frac_digits.len()).ok()?);
        }
        rest = next;
    }
    Some(duration_from_nanos(if neg { -total } else { total }))
}

/// Formats a timestamp like Go's `t.Format(time.RFC3339)` in UTC: whole
/// seconds, trailing Z.
#[must_use]
pub fn format_rfc3339(t: &Timestamp) -> String {
    chrono::DateTime::from_timestamp(t.seconds, 0).map_or_else(String::new, |dt| {
        dt.format("%Y-%m-%dT%H:%M:%SZ").to_string()
    })
}

/// Parses an RFC3339 string into a Timestamp. `None` on invalid input.
#[must_use]
pub fn parse_rfc3339(s: &str) -> Option<Timestamp> {
    let dt = chrono::DateTime::parse_from_rfc3339(s).ok()?;
    Some(Timestamp {
        seconds: dt.timestamp(),
        nanos: i32::try_from(dt.timestamp_subsec_nanos()).unwrap_or(0),
    })
}
