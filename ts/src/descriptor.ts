/**
 * Schema DESCRIPTOR validation (not form values): structural checks mirroring
 * the Go reference descriptor.go. Problems surface as a SchemaError carrying
 * the proto ValidationResult with code INVALID_SCHEMA.
 */

import { create } from "@bufbuild/protobuf";
import type { ValidationError, ValidationResult } from "./gen/schemapb/errors_pb.js";
import {
  ErrorCode,
  ValidationErrorSchema,
  ValidationResultSchema,
} from "./gen/schemapb/errors_pb.js";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import { Schema_Field_Severity } from "./gen/schemapb/schema_pb.js";

/** A malformed schema descriptor (programmatic failure, principle 5). */
export class SchemaError extends Error {
  readonly result: ValidationResult;

  constructor(errors: ValidationError[]) {
    const text = errors
      .map((e) => (e.path === "" ? e.message : `${e.path}: ${e.message}`))
      .join("; ");
    super(`schemapb: invalid schema; ${text}`);
    this.name = "SchemaError";
    this.result = create(ValidationResultSchema, { errors });
  }
}

/** Builds one descriptor-level ValidationError. */
export function schemaErr(path: string, msg: string): ValidationError {
  return create(ValidationErrorSchema, {
    path,
    code: ErrorCode.INVALID_SCHEMA,
    severity: Schema_Field_Severity.ERROR,
    message: msg,
  });
}

export function joinPath(prefix: string, name: string): string {
  return prefix === "" ? name : `${prefix}.${name}`;
}

/** The schemas directly embedded in a field (not Refs). */
export function nestedSchemas(f: Schema_Field): Schema[] {
  const out: Schema[] = [];
  const kind = f.kind;
  switch (kind.case) {
    case "object":
      if (kind.value.schema !== undefined) {
        out.push(kind.value.schema);
      }
      break;
    case "oneOf":
      out.push(...Object.values(kind.value.variants));
      break;
    case "map":
      if (kind.value.valueSchema !== undefined) {
        out.push(kind.value.valueSchema);
      }
      break;
    case "list":
      for (const it of kind.value.items) {
        out.push(...nestedSchemas(it));
      }
      break;
    default:
      break;
  }
  return out;
}

/**
 * Verifies the schema descriptor is well-formed; returns the problems
 * (empty = fine). Expression compilation errors are added by compile().
 */
export function checkDescriptor(s: Schema): ValidationError[] {
  const errs: ValidationError[] = [];
  if ((s.id?.name ?? "") === "") {
    errs.push(schemaErr("id.name", "schema identity name is required"));
  }
  errs.push(...checkFields(s.fields, ""));
  for (const [name, def] of Object.entries(s.defs)) {
    errs.push(...checkFields(def.fields, `$defs.${name}`));
  }
  errs.push(...checkRefTargets(s.fields, s.defs, ""));
  for (const [name, def] of Object.entries(s.defs)) {
    errs.push(...checkRefTargets(def.fields, s.defs, `$defs.${name}`));
  }
  return errs;
}

function checkFields(fields: Schema_Field[], prefix: string): ValidationError[] {
  const errs: ValidationError[] = [];
  const seen = new Set<string>();
  for (const f of fields) {
    const path = joinPath(prefix, f.name);
    if (f.name === "") {
      // List item fields are anonymous but checkFields never walks items
      // directly (mirrors the Go reference).
      errs.push(schemaErr(prefix, "field name is required"));
      continue;
    }
    if (seen.has(f.name)) {
      errs.push(schemaErr(path, "duplicate field name"));
    }
    seen.add(f.name);
    if (f.kind.case === undefined) {
      errs.push(schemaErr(path, "field kind is required"));
      continue;
    }
    f.rules.forEach((r, i) => {
      if (r.expr === "") {
        errs.push(schemaErr(path, `rule[${i}]: empty expression`));
      }
    });
    const kind = f.kind;
    switch (kind.case) {
      case "computed":
        if (kind.value.expr === "") {
          errs.push(schemaErr(path, "computed field: empty expression"));
        }
        break;
      case "oneOf":
        if (kind.value.discriminator === "") {
          errs.push(schemaErr(path, "oneof field: discriminator is required"));
        }
        if (Object.keys(kind.value.variants).length === 0) {
          errs.push(schemaErr(path, "oneof field: at least one variant is required"));
        }
        break;
      case "ref":
        if (kind.value.target.case === undefined) {
          errs.push(schemaErr(path, "ref field: target is required"));
        }
        break;
      case "list": {
        const l = kind.value;
        if (l.items.length === 0) {
          errs.push(schemaErr(path, "list field: at least one item definition is required"));
        }
        if (
          l.items.length > 1 &&
          (l.minItems !== undefined ||
            l.maxItems !== undefined ||
            l.unique ||
            (l.countExpr ?? "") !== "")
        ) {
          errs.push(
            schemaErr(
              path,
              "tuple list (multiple item definitions) cannot combine with min_items/max_items/unique/count_expr",
            ),
          );
        }
        break;
      }
      case "choice": {
        const ch = kind.value;
        if (!ch.open && ch.options.length === 0 && (ch.optionsExpr ?? "") === "") {
          errs.push(
            schemaErr(path, "choice field: a closed choice requires options or options_expr"),
          );
        }
        ch.options.forEach((o, i) => {
          if (o.value === undefined) {
            errs.push(schemaErr(path, `choice option[${i}]: value is required`));
          }
        });
        break;
      }
      case "map": {
        const mp = kind.value;
        if (
          mp.minEntries !== undefined &&
          mp.maxEntries !== undefined &&
          mp.minEntries > mp.maxEntries
        ) {
          errs.push(schemaErr(path, "map field: min_entries must be <= max_entries"));
        }
        break;
      }
      default:
        break;
    }
    for (const child of nestedSchemas(f)) {
      errs.push(...checkFields(child.fields, path));
    }
  }
  return errs;
}

function checkRefTargets(
  fields: Schema_Field[],
  rootDefs: Record<string, Schema>,
  prefix: string,
): ValidationError[] {
  const errs: ValidationError[] = [];
  for (const f of fields) {
    const path = joinPath(prefix, f.name);
    if (f.kind.case === "ref") {
      const target = f.kind.value.target;
      if (target.case === "name" && target.value !== "" && !(target.value in rootDefs)) {
        errs.push(
          schemaErr(path, `ref ${JSON.stringify(target.value)} is not defined in schema defs`),
        );
      }
    }
    if (f.kind.case === "list") {
      errs.push(...checkRefTargets(f.kind.value.items, rootDefs, `${path}[]`));
      continue;
    }
    for (const child of nestedSchemas(f)) {
      errs.push(...checkRefTargets(child.fields, rootDefs, path));
    }
  }
  return errs;
}
