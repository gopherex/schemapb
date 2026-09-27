import { create } from "@bufbuild/protobuf";
import { type PathSegment, PathSegmentSchema } from "./gen/schemapb/value_pb.js";

/** Parses the escaped canonical path, including arbitrary quoted map keys. */
export function pathSegments(path: string): PathSegment[] {
  const out: PathSegment[] = [];
  const tokens = /(?:^|\.)([^.[\]]+)|\[("(?:\\.|[^"\\])*"|\d+)\]/gy;
  let offset = 0;
  while (offset < path.length) {
    tokens.lastIndex = offset;
    const m = tokens.exec(path);
    if (!m) return [];
    const name = m[1];
    const bracket = m[2] ?? "";
    out.push(
      create(PathSegmentSchema, {
        segment:
          name !== undefined
            ? { case: "key", value: name }
            : bracket.startsWith('"')
              ? { case: "key", value: JSON.parse(bracket) as string }
              : { case: "index", value: BigInt(bracket) },
      }),
    );
    offset = tokens.lastIndex;
  }
  return out;
}

/** UTF-8 lexical order, independent of JS's UTF-16 sort ordering. */
export function sortedKeys(value: object): string[] {
  return Object.keys(value).sort((a, b) => {
    const aa = Array.from(a),
      bb = Array.from(b);
    for (let i = 0; i < Math.min(aa.length, bb.length); i++) {
      const d = (aa[i]?.codePointAt(0) ?? 0) - (bb[i]?.codePointAt(0) ?? 0);
      if (d) return d;
    }
    return aa.length - bb.length;
  });
}
