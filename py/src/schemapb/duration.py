"""Go-compatible duration formatting/parsing and RFC3339 timestamps.

The spec's display form is Go's time.Duration.String(); string inputs accept
Go's time.ParseDuration syntax. Python timedeltas carry microseconds — the
sub-microsecond tail of the contract rounds (documented deviation).
"""

from __future__ import annotations

import datetime as dt
import re

_UNIT_US = {
    "ns": 1e-3,
    "us": 1.0,
    "µs": 1.0,
    "μs": 1.0,
    "ms": 1e3,
    "s": 1e6,
    "m": 60e6,
    "h": 3600e6,
}
_DURATION_RE = re.compile(r"^[+-]?((\d+(\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h))+$")
_PART_RE = re.compile(r"(\d+(?:\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h)")


def duration_micros(d: dt.timedelta) -> int:
    return round(d.total_seconds() * 1e6)


def format_go_duration(d: dt.timedelta) -> str:
    v = duration_micros(d)
    neg = v < 0
    v = abs(v)
    if v == 0:
        return "0s"
    if v < 1000:
        out = f"{_frac(v, 1)}µs" if v >= 1 else f"{v * 1000}ns"
    elif v < 1_000_000:
        out = f"{_frac(v, 1000)}ms"
    else:
        minute_us = 60_000_000
        s = f"{_frac(v % minute_us, 1_000_000)}s"
        rest = v // minute_us
        if rest > 0:
            s = f"{rest % 60}m{s}"
            rest //= 60
            if rest > 0:
                s = f"{rest}h{s}"
        out = s
    return f"-{out}" if neg else out


def _frac(v: int, unit: int) -> str:
    whole = v // unit
    if unit == 1:
        return str(whole)
    frac = str(v % unit).rjust(len(str(unit)) - 1, "0").rstrip("0")
    return f"{whole}.{frac}" if frac else str(whole)


def parse_go_duration(s: str) -> dt.timedelta | None:
    if s == "0":
        return dt.timedelta()
    if not _DURATION_RE.match(s):
        return None
    neg = s.startswith("-")
    total_us = 0.0
    for num, unit in _PART_RE.findall(s):
        total_us += float(num) * _UNIT_US[unit]
    micros = round(total_us)
    return dt.timedelta(microseconds=-micros if neg else micros)


_RFC3339_RE = re.compile(
    r"^(\d{4})-(\d{2})-(\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$",
)


def format_rfc3339(t: dt.datetime) -> str:
    """Go's t.Format(time.RFC3339) in UTC: whole seconds, trailing Z."""
    return t.astimezone(dt.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_rfc3339(s: str) -> dt.datetime | None:
    if not _RFC3339_RE.match(s):
        return None
    try:
        return dt.datetime.fromisoformat(s.replace("z", "Z")).astimezone(dt.UTC)
    except ValueError:
        return None
