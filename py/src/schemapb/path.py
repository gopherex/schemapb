"""Canonical escaped value paths shared by diagnostics and resolve events."""

import json
import re

from schemapb._gen.schemapb import PathSegment

_TOKEN = re.compile(r'(?:^|\.)([^.\[\]]+)|\[("(?:\\.|[^"\\])*"|\d+)\]')


def path_segments(path: str) -> list[PathSegment]:
    result: list[PathSegment] = []
    offset = 0
    while offset < len(path):
        match = _TOKEN.match(path, offset)
        if match is None:
            return []
        name, bracket = match.groups()
        if name is not None:
            result.append(PathSegment(key=name))
        elif bracket.startswith('"'):
            result.append(PathSegment(key=json.loads(bracket)))
        else:
            result.append(PathSegment(index=int(bracket)))
        offset = match.end()
    return result
