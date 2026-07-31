/**
 * The compiled engine: every CEL expression, regex pattern and Mustache
 * template of a schema compiled exactly once, up front. A bad schema fails
 * here as a SchemaError, never later at evaluation time. Mirrors the Go
 * reference engine.go.
 */

import {
  type CelEnv,
  type CelInput,
  type CelResult,
  CelScalar,
  type CelValue,
  celEnv,
  isCelError,
  isCelList,
  isCelMap,
  isCelUint,
  mapType,
  parse,
  plan,
} from "@bufbuild/cel";
import { strings } from "@bufbuild/cel/ext";
import type { Expr, ParsedExpr } from "@bufbuild/cel-spec/cel/expr/syntax_pb.js";
import { isMessage } from "@bufbuild/protobuf";
import { isReflectMessage } from "@bufbuild/protobuf/reflect";
import { DurationSchema, TimestampSchema } from "@bufbuild/protobuf/wkt";
import Mustache from "mustache";
import { checkDescriptor, joinPath, nestedSchemas, SchemaError, schemaErr } from "./descriptor.js";
import type { CompileOptions, FormatFunc, FormatRegistry } from "./format.js";
import { coreFormats } from "./format.js";
import type { ValidationError } from "./gen/schemapb/errors_pb.js";
import type { Schema, Schema_Field } from "./gen/schemapb/schema_pb.js";
import type { Format } from "./typed.js";
import type { Native, NativeStruct } from "./value.js";

type Vars = {
  this: typeof CelScalar.DYN;
  root: ReturnType<typeof mapType<typeof CelScalar.STRING, typeof CelScalar.DYN>>;
  index: typeof CelScalar.INT;
};

type Plan = (bindings: { this: CelInput; root: CelInput; index: bigint }) => CelResult;

/** Variable bindings for one evaluation. */
export interface EvalVars {
  this?: Native;
  root: NativeStruct;
  index?: bigint;
}

/** The result of one evaluation: a native value or an error message. */
export type EvalOutcome = { ok: true; value: Native } | { ok: false; error: string };

let sharedEnv: CelEnv<Vars> | undefined;

function env(): CelEnv<Vars> {
  sharedEnv ??= celEnv({
    variables: {
      this: CelScalar.DYN,
      root: mapType(CelScalar.STRING, CelScalar.DYN),
      index: CelScalar.INT,
    },
    funcs: strings,
  });
  return sharedEnv;
}

/** An immutable compiled schema. */
export class Engine {
  readonly schema: Schema;
  readonly formats: FormatRegistry;
  readonly regexps: Map<string, RegExp>;
  readonly #plans: Map<string, Plan>;
  readonly #asts: Map<string, ParsedExpr>;

  private constructor(
    schema: Schema,
    formats: FormatRegistry,
    regexps: Map<string, RegExp>,
    plans: Map<string, Plan>,
    asts: Map<string, ParsedExpr>,
  ) {
    this.schema = schema;
    this.formats = formats;
    this.regexps = regexps;
    this.#plans = plans;
    this.#asts = asts;
  }

  /**
   * Compiles a schema: descriptor checks, all expressions and patterns and
   * templates, static rejection of top-level computed cycles. Throws
   * SchemaError on any defect.
   */
  static compile(schema: Schema, opts: CompileOptions = {}): Engine {
    const errs: ValidationError[] = [...checkDescriptor(schema)];

    const formats = coreFormats();
    const extra =
      opts.formats instanceof Map
        ? opts.formats
        : new Map(Object.entries(opts.formats ?? {}) as [Format, FormatFunc][]);
    for (const [name, fn] of extra) {
      formats.set(name, fn);
    }

    const plans = new Map<string, Plan>();
    const asts = new Map<string, ParsedExpr>();
    for (const [src, path] of schemaExprs(schema)) {
      if (plans.has(src)) {
        continue;
      }
      try {
        const ast = parse(src);
        asts.set(src, ast);
        plans.set(src, plan(env(), ast) as Plan);
      } catch (e) {
        errs.push(schemaErr(path, `cel: ${e instanceof Error ? e.message : String(e)}`));
      }
    }

    const regexps = new Map<string, RegExp>();
    for (const [pattern, path] of schemaPatterns(schema)) {
      if (regexps.has(pattern)) {
        continue;
      }
      try {
        regexps.set(pattern, new RegExp(pattern, "u"));
      } catch (e) {
        errs.push(schemaErr(path, `pattern: ${e instanceof Error ? e.message : String(e)}`));
      }
    }

    for (const [name, src] of Object.entries(schema.templates)) {
      try {
        Mustache.parse(src);
      } catch (e) {
        errs.push(
          schemaErr(`templates.${name}`, `mustache: ${e instanceof Error ? e.message : String(e)}`),
        );
      }
    }

    const engine = new Engine(schema, formats, regexps, plans, asts);
    if (errs.length === 0) {
      errs.push(...engine.checkComputedCycles());
    }
    if (errs.length > 0) {
      throw new SchemaError(errs);
    }
    return engine;
  }

  /**
   * Runs a precompiled CEL plan (a sandboxed expression language — no access
   * to the JS runtime; this is NOT JavaScript eval). The result is in the
   * native value model.
   */
  eval(src: string, vars: EvalVars): EvalOutcome {
    const prg = this.#plans.get(src);
    if (prg === undefined) {
      return { ok: false, error: `expression not compiled: ${src}` };
    }
    const out = prg({
      this: nativeToCel(vars.this ?? null),
      root: nativeToCel(vars.root),
      index: vars.index ?? 0n,
    });
    if (isCelError(out)) {
      return { ok: false, error: out.message };
    }
    return { ok: true, value: celToNative(out) };
  }

  /** Runs a compiled expression and requires a boolean result. */
  evalBool(src: string, vars: EvalVars): { ok: boolean; error?: string } {
    const res = this.eval(src, vars);
    if (!res.ok) {
      return { ok: false, error: res.error };
    }
    if (typeof res.value !== "boolean") {
      return { ok: false, error: `expression yields ${typeof res.value}, want bool` };
    }
    return { ok: res.value };
  }

  /**
   * The dotted root paths a compiled expression reads (root.a -> "a",
   * root["a"] -> "a").
   */
  exprDeps(src: string): string[] {
    const ast = this.#asts.get(src);
    const root = ast?.expr;
    if (root === undefined) {
      return [];
    }
    const deps: string[] = [];
    walkExpr(root, deps);
    return deps;
  }

  /** Statically rejects cycles between top-level Computed fields. */
  private checkComputedCycles(): ValidationError[] {
    const computed = new Map<string, Schema_Field>();
    for (const f of this.schema.fields) {
      if (f.kind.case === "computed") {
        computed.set(f.name, f);
      }
    }
    if (computed.size === 0) {
      return [];
    }
    const deps = new Map<string, string[]>();
    for (const [name, f] of computed) {
      if (f.kind.case !== "computed") {
        continue;
      }
      deps.set(
        name,
        this.exprDeps(f.kind.value.expr).filter((d) => d !== name && computed.has(d)),
      );
    }
    const color = new Map<string, number>();
    const errs: ValidationError[] = [];
    const visit = (n: string): boolean => {
      const c = color.get(n) ?? 0;
      if (c === 1) {
        return false;
      }
      if (c === 2) {
        return true;
      }
      color.set(n, 1);
      for (const d of deps.get(n) ?? []) {
        if (!visit(d)) {
          return false;
        }
      }
      color.set(n, 2);
      return true;
    };
    for (const name of computed.keys()) {
      if (color.get(name) !== 2 && !visit(name)) {
        errs.push(schemaErr(name, "computed field cycle"));
      }
    }
    return errs;
  }
}

// =============================================================================
// Expression / pattern collection
// =============================================================================

function schemaExprs(s: Schema): Map<string, string> {
  const out = new Map<string, string>();
  const add = (src: string | undefined, path: string): void => {
    if (src !== undefined && src !== "" && !out.has(src)) {
      out.set(src, path);
    }
  };
  const walkSchema = (sub: Schema, prefix: string): void => {
    sub.rules.forEach((r, i) => {
      add(r.expr, `${prefix}#rule[${i}]`);
    });
  };
  const walkFields = (fields: Schema_Field[], prefix: string): void => {
    for (const f of fields) {
      const path = joinPath(prefix, f.name);
      add(f.when, `${path}#when`);
      add(f.normalize, `${path}#normalize`);
      f.rules.forEach((r, i) => {
        add(r.expr, `${path}#rule[${i}]`);
      });
      const kind = f.kind;
      if (kind.case === "computed") {
        add(kind.value.expr, `${path}#computed`);
      }
      if (kind.case === "choice") {
        add(kind.value.optionsExpr, `${path}#options`);
      }
      if (kind.case === "list") {
        add(kind.value.countExpr, `${path}#count`);
        walkFields(kind.value.items, `${path}[]`);
      }
      for (const child of nestedSchemas(f)) {
        walkSchema(child, path);
        walkFields(child.fields, path);
      }
    }
  };
  walkSchema(s, "");
  walkFields(s.fields, "");
  for (const [name, def] of Object.entries(s.defs)) {
    walkSchema(def, `$defs.${name}`);
    walkFields(def.fields, `$defs.${name}`);
  }
  return out;
}

function schemaPatterns(s: Schema): Map<string, string> {
  const out = new Map<string, string>();
  const walkFields = (fields: Schema_Field[], prefix: string): void => {
    for (const f of fields) {
      const path = joinPath(prefix, f.name);
      if (f.kind.case === "string") {
        const pattern = f.kind.value.pattern ?? "";
        if (pattern !== "" && !out.has(pattern)) {
          out.set(pattern, `${path}#pattern`);
        }
      }
      if (f.kind.case === "list") {
        walkFields(f.kind.value.items, `${path}[]`);
      }
      for (const child of nestedSchemas(f)) {
        walkFields(child.fields, path);
      }
    }
  };
  walkFields(s.fields, "");
  for (const [name, def] of Object.entries(s.defs)) {
    walkFields(def.fields, `$defs.${name}`);
  }
  return out;
}

// =============================================================================
// CEL <-> native conversion
// =============================================================================

function nativeToCel(x: Native): CelInput {
  if (x === null || typeof x === "boolean" || typeof x === "bigint" || typeof x === "number") {
    return x;
  }
  if (typeof x === "string" || x instanceof Uint8Array) {
    return x;
  }
  if (isMessage(x, DurationSchema) || isMessage(x, TimestampSchema)) {
    return x;
  }
  if (Array.isArray(x)) {
    return x.map(nativeToCel);
  }
  const out: Record<string, CelInput> = {};
  for (const [k, v] of Object.entries(x)) {
    out[k] = nativeToCel(v);
  }
  return out;
}

function celToNative(v: CelValue): Native {
  if (v === null || typeof v === "boolean" || typeof v === "bigint" || typeof v === "number") {
    return v;
  }
  if (typeof v === "string" || v instanceof Uint8Array) {
    return v;
  }
  if (isCelUint(v)) {
    return v.value;
  }
  if (isCelList(v)) {
    return [...v].map(celToNative);
  }
  if (isCelMap(v)) {
    const out: NativeStruct = {};
    for (const [k, el] of v) {
      const key = isCelUint(k) ? k.value.toString(10) : String(k);
      out[key] = celToNative(el);
    }
    return out;
  }
  if (isReflectMessage(v)) {
    const msg = v.message;
    if (isMessage(msg, DurationSchema) || isMessage(msg, TimestampSchema)) {
      return msg;
    }
    return null;
  }
  return null;
}

// =============================================================================
// AST dependency walk
// =============================================================================

function walkExpr(x: Expr, deps: string[]): void {
  const sel = selectPath(x);
  if (sel !== undefined) {
    if (sel !== "") {
      deps.push(sel);
    }
    return;
  }
  const kind = x.exprKind;
  switch (kind.case) {
    case "selectExpr":
      if (kind.value.operand !== undefined) {
        walkExpr(kind.value.operand, deps);
      }
      break;
    case "callExpr":
      if (kind.value.target !== undefined) {
        walkExpr(kind.value.target, deps);
      }
      for (const a of kind.value.args) {
        walkExpr(a, deps);
      }
      break;
    case "listExpr":
      for (const el of kind.value.elements) {
        walkExpr(el, deps);
      }
      break;
    case "structExpr":
      for (const entry of kind.value.entries) {
        if (entry.keyKind.case === "mapKey") {
          walkExpr(entry.keyKind.value, deps);
        }
        if (entry.value !== undefined) {
          walkExpr(entry.value, deps);
        }
      }
      break;
    case "comprehensionExpr": {
      const c = kind.value;
      for (const part of [c.iterRange, c.accuInit, c.loopCondition, c.loopStep, c.result]) {
        if (part !== undefined) {
          walkExpr(part, deps);
        }
      }
      break;
    }
    default:
      break;
  }
}

/** Resolves a select/index chain rooted at `root` to a dotted path. */
function selectPath(x: Expr): string | undefined {
  const kind = x.exprKind;
  switch (kind.case) {
    case "identExpr":
      return kind.value.name === "root" ? "" : undefined;
    case "selectExpr": {
      const operand = kind.value.operand;
      if (operand === undefined) {
        return undefined;
      }
      const base = selectPath(operand);
      if (base === undefined) {
        return undefined;
      }
      return base === "" ? kind.value.field : `${base}.${kind.value.field}`;
    }
    case "callExpr": {
      if (kind.value.function !== "_[_]" || kind.value.args.length !== 2) {
        return undefined;
      }
      const [operand, keyExpr] = kind.value.args;
      if (operand === undefined || keyExpr === undefined) {
        return undefined;
      }
      const base = selectPath(operand);
      if (base === undefined) {
        return undefined;
      }
      const constKind = keyExpr.exprKind;
      if (constKind.case !== "constExpr" || constKind.value.constantKind.case !== "stringValue") {
        return undefined;
      }
      const key = constKind.value.constantKind.value;
      return base === "" ? key : `${base}.${key}`;
    }
    default:
      return undefined;
  }
}
