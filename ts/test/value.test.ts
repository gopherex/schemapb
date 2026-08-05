/**
 * Value conformance: the As conversion matrix (value-as.json) and value
 * path lookup over the baked kitchen-sink values (value-lookup.json).
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fromJson, toJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { type ValueAsTarget, valueAs } from "../src/as.js";
import { StructValueSchema, type Value, ValueSchema } from "../src/gen/schemapb/value_pb.js";
import {
  boolV,
  bytesV,
  doubleV,
  durationV,
  floatV,
  int32V,
  int64V,
  listV,
  structV,
  strV,
  timestampV,
  uint32V,
  uint64V,
} from "../src/value.js";
import { lookupValue, ValueLookupError } from "../src/value_lookup.js";

const goldenDir = join(__dirname, "..", "..", "conformance", "golden");

function golden(name: string): unknown {
  return JSON.parse(readFileSync(join(goldenDir, name), "utf8"));
}

/** Re-encodes a successful extraction as a wire Value of the target kind. */
function reEncode(v: Value, target: ValueAsTarget): Value | undefined {
  switch (target) {
    case "bool": {
      const x = valueAs(v, "bool");
      return x === undefined ? undefined : boolV(x);
    }
    case "int32": {
      const x = valueAs(v, "int32");
      return x === undefined ? undefined : int32V(x);
    }
    case "int64": {
      const x = valueAs(v, "int64");
      return x === undefined ? undefined : int64V(x);
    }
    case "uint32": {
      const x = valueAs(v, "uint32");
      return x === undefined ? undefined : uint32V(x);
    }
    case "uint64": {
      const x = valueAs(v, "uint64");
      return x === undefined ? undefined : uint64V(x);
    }
    case "float": {
      const x = valueAs(v, "float");
      return x === undefined ? undefined : floatV(x);
    }
    case "double": {
      const x = valueAs(v, "double");
      return x === undefined ? undefined : doubleV(x);
    }
    case "string": {
      const x = valueAs(v, "string");
      return x === undefined ? undefined : strV(x);
    }
    case "bytes": {
      const x = valueAs(v, "bytes");
      return x === undefined ? undefined : bytesV(x);
    }
    case "duration": {
      const x = valueAs(v, "duration");
      return x === undefined ? undefined : durationV(x);
    }
    case "timestamp": {
      const x = valueAs(v, "timestamp");
      return x === undefined ? undefined : timestampV(x);
    }
    case "list": {
      const x = valueAs(v, "list");
      return x === undefined ? undefined : listV(...x);
    }
    case "struct": {
      const x = valueAs(v, "struct");
      return x === undefined ? undefined : structV(x);
    }
    default:
      return undefined;
  }
}

interface AsCase {
  value: unknown;
  target: ValueAsTarget;
  result?: unknown;
}

describe("valueAs conformance", () => {
  const cases = (golden("value-as.json") as { cases: AsCase[] }).cases;

  it.each(cases)("$target <- $value", (c) => {
    const v = fromJson(ValueSchema, c.value as Parameters<typeof fromJson>[1]);
    const got = reEncode(v, c.target);
    if (c.result === undefined) {
      expect(got).toBeUndefined();
    } else {
      expect(got).toBeDefined();
      expect(toJson(ValueSchema, got as Value)).toEqual(c.result);
    }
  });
});

interface LookupCase {
  path: string;
  value?: unknown;
  error?: { at: string; segment: string; reason: string };
}

describe("value lookup conformance", () => {
  const values = fromJson(
    StructValueSchema,
    golden("full-baked.json") as Parameters<typeof fromJson>[1],
  );
  const cases = (golden("value-lookup.json") as { cases: LookupCase[] }).cases;

  it.each(cases)("$path", (c) => {
    if (c.error !== undefined) {
      try {
        lookupValue(values, c.path);
        expect.fail(`lookupValue(${c.path}) resolved, want ${c.error.reason}`);
      } catch (err) {
        expect(err).toBeInstanceOf(ValueLookupError);
        const le = err as ValueLookupError;
        expect({ at: le.at, segment: le.segment, reason: le.reason }).toEqual(c.error);
      }
      return;
    }
    expect(toJson(ValueSchema, lookupValue(values, c.path))).toEqual(c.value);
  });
});
