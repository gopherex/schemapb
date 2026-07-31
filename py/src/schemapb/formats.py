"""The spec's string-format registry: mandatory core formats, extensible per
engine; an unknown format fails validation loudly (UNSUPPORTED_FORMAT).
"""

from __future__ import annotations

import datetime as dt
import re
from collections.abc import Callable
from typing import TypeAlias

from schemapb.typed import Format

FormatFunc: TypeAlias = Callable[[str], bool]
FormatRegistry: TypeAlias = "dict[Format, FormatFunc]"

_UUID_RE = re.compile(r"^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$", re.IGNORECASE)
_HOSTNAME_RE = re.compile(
    r"^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$",
    re.IGNORECASE,
)
_IPV4_RE = re.compile(r"^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$")
_EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+$")
_DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
_TIME_RE = re.compile(r"^\d{2}:\d{2}:\d{2}$")
_RFC3339_RE = re.compile(
    r"^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$",
)
_URL_RE = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*://\S+$|^/\S*$")


def _is_ipv4(s: str) -> bool:
    m = _IPV4_RE.match(s)
    if m is None:
        return False
    for octet in m.groups():
        if len(octet) > 1 and octet.startswith("0"):
            return False
        if int(octet) > 255:
            return False
    return True


def _is_ipv6(s: str) -> bool:
    if ":" not in s or " " in s:
        return False
    if s.count("::") > 1:
        return False
    groups = [g for g in re.split(r"::|:", s) if g != ""]
    if len(groups) > 8 or ("::" not in s and len(groups) != 8):
        return False
    return all(re.fullmatch(r"[0-9a-fA-F]{1,4}", g) or _is_ipv4(g) for g in groups)


def _is_date(s: str) -> bool:
    if not _DATE_RE.match(s):
        return False
    try:
        dt.date.fromisoformat(s)
    except ValueError:
        return False
    return True


def _is_time(s: str) -> bool:
    if not _TIME_RE.match(s):
        return False
    h, m, sec = (int(p) for p in s.split(":"))
    return h <= 23 and m <= 59 and sec <= 59


def _is_datetime(s: str) -> bool:
    if not _RFC3339_RE.match(s):
        return False
    try:
        dt.datetime.fromisoformat(s.replace("z", "Z"))
    except ValueError:
        return False
    return True


def core_formats() -> FormatRegistry:
    return {
        Format("email"): lambda s: _EMAIL_RE.match(s) is not None,
        Format("url"): lambda s: _URL_RE.match(s) is not None,
        Format("uuid"): lambda s: _UUID_RE.match(s) is not None,
        Format("ipv4"): _is_ipv4,
        Format("ipv6"): lambda s: _is_ipv6(s) and not _is_ipv4(s),
        Format("ip"): lambda s: _is_ipv4(s) or _is_ipv6(s),
        Format("hostname"): lambda s: len(s) <= 253 and _HOSTNAME_RE.match(s) is not None,
        Format("date"): _is_date,
        Format("time"): _is_time,
        Format("datetime"): _is_datetime,
    }
