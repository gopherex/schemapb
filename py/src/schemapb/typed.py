"""Typed identifier domains and the opaque semver Version.

NewType gives nominal separation under mypy strict (principle 1); Version is
constructible only through parse/of — an invalid version is unrepresentable
(principle 2).
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import NewType

from schemapb._gen.schemapb import SchemaIdentity

Namespace = NewType("Namespace", str)
SchemaName = NewType("SchemaName", str)
FieldName = NewType("FieldName", str)
DefName = NewType("DefName", str)
TemplateName = NewType("TemplateName", str)
RuleId = NewType("RuleId", str)
GroupName = NewType("GroupName", str)
VariantKey = NewType("VariantKey", str)
Format = NewType("Format", str)

FORMAT_EMAIL = Format("email")
FORMAT_URL = Format("url")
FORMAT_UUID = Format("uuid")
FORMAT_IPV4 = Format("ipv4")
FORMAT_IPV6 = Format("ipv6")
FORMAT_IP = Format("ip")
FORMAT_HOSTNAME = Format("hostname")
FORMAT_DATE = Format("date")
FORMAT_TIME = Format("time")
FORMAT_DATETIME = Format("datetime")

_SEMVER_RE = re.compile(
    r"^v?(0|[1-9]\d*)(?:\.(0|[1-9]\d*))?(?:\.(0|[1-9]\d*))?"
    r"(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?"
    r"(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$",
)


class VersionError(ValueError):
    """An invalid semver string."""


@dataclass(frozen=True, slots=True)
class Version:
    """An always-valid semver value; the zero Version means "unversioned"."""

    _s: str = field(default="")

    @staticmethod
    def of(major: int, minor: int, patch: int) -> Version:
        return Version(f"v{major}.{minor}.{patch}")

    @staticmethod
    def parse(s: str) -> Version:
        if s == "":
            return Version()
        m = _SEMVER_RE.match(s)
        if m is None:
            msg = f"schemapb: invalid version {s!r}"
            raise VersionError(msg)
        major, minor, patch, pre, build = m.groups()
        out = f"v{major}.{minor or 0}.{patch or 0}"
        if pre:
            out += f"-{pre}"
        if build:
            out += f"+{build}"
        return Version(out)

    @property
    def is_zero(self) -> bool:
        return self._s == ""

    def __str__(self) -> str:
        return self._s

    def __lt__(self, other: Version) -> bool:
        return self.compare(other) < 0

    def __le__(self, other: Version) -> bool:
        return self.compare(other) <= 0

    def __gt__(self, other: Version) -> bool:
        return self.compare(other) > 0

    def __ge__(self, other: Version) -> bool:
        return self.compare(other) >= 0

    def compare(self, other: Version) -> int:
        if self._s == other._s:
            return 0
        if self._s == "":
            return -1
        if other._s == "":
            return 1
        return _cmp(_key(self._s), _key(other._s))


def _key(s: str) -> _Key:
    m = _SEMVER_RE.match(s)
    if m is None:  # unreachable: constructed values are valid
        msg = f"schemapb: corrupt version {s!r}"
        raise VersionError(msg)
    major, minor, patch, pre, _build = m.groups()
    nums = (int(major), int(minor or 0), int(patch or 0))
    if pre is None:
        return nums, None
    parts: list[tuple[int, int | str]] = [
        (0, int(p)) if p.isdigit() else (1, p) for p in pre.split(".")
    ]
    return nums, tuple(parts)


_Key = tuple[tuple[int, int, int], "tuple[tuple[int, int | str], ...] | None"]


def _cmp(a: _Key, b: _Key) -> int:
    # Semver precedence: a release outranks any of its prereleases.
    (an, ap), (bn, bp) = a, b
    if an != bn:
        return -1 if an < bn else 1
    if ap == bp:
        return 0
    if ap is None:
        return 1
    if bp is None:
        return -1
    return -1 if ap < bp else 1


def make_id(ns: str, name: str, ver: Version) -> SchemaIdentity:
    """Builds an identity handle: declare once, reuse everywhere."""
    return SchemaIdentity(namespace=ns, name=name, version=str(ver))
