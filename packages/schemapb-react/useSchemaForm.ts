import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  schemapb,
  type BakedJson,
  type FieldErrorJson,
  type Schema,
  type Schemapb,
} from "@stroppy-io/schemapb";
import { errorsByField, getPath, setPath } from "./helpers.ts";

type Values = Record<string, unknown>;

export interface UseSchemaFormOptions {
  /** The schema to validate/compute against. */
  schema: Schema;
  /** Initial form values (defaults are filled by compute). */
  initialValues?: Values;
  /** When to (re)validate. "onChange" (default) runs on every edit; "onSubmit"
   *  only validates in handleSubmit. */
  mode?: "onChange" | "onSubmit";
  /** Supply a pre-loaded engine instead of auto-loading the shared one. Useful
   *  for SSR, a single app-wide instance, or custom WASM loading. */
  engine?: Schemapb;
}

/** Props to spread onto a controlled input. Headless: you decide the element. */
export interface FieldProps {
  name: string;
  value: unknown;
  error?: FieldErrorJson;
  onChange: (eventOrValue: unknown) => void;
}

export interface UseSchemaFormResult {
  /** Whether the WASM engine has loaded. While false, validate/compute no-op. */
  ready: boolean;
  /** Current (raw) form values. */
  values: Values;
  /** Resolved values: inputs + schema defaults + evaluated Computed fields. */
  computed: Values;
  /** Validation errors indexed by field path. */
  errors: Record<string, FieldErrorJson>;
  /** True when there are no blocking validation errors. */
  isValid: boolean;

  /** Set a single field by dotted/indexed path (e.g. "addr.zip", "tags.0"). */
  setValue: (path: string, value: unknown) => void;
  /** Replace all values. */
  setValues: (values: Values) => void;
  /** Reset to initialValues (or the provided values). */
  reset: (values?: Values) => void;

  /** Error for a field path, if any. */
  getError: (path: string) => FieldErrorJson | undefined;
  /** Controlled-input props for a field path (RHF-style). */
  register: (path: string) => FieldProps;

  /** Whether a field is active for the current form (its `when` gate). */
  fieldActive: (path: string) => boolean;
  /** Allowed enum values for a field given the current form. */
  enumOptions: (path: string) => number[];
  /** Required list length for a field given the current form. */
  listCount: (path: string) => number;

  /** Validate (and on success, bake) the form. Returns a submit handler:
   *  calls onValid(baked) when valid, else onInvalid(errors). */
  handleSubmit: (
    onValid: (baked: BakedJson) => void,
    onInvalid?: (errors: Record<string, FieldErrorJson>) => void,
  ) => (e?: { preventDefault?: () => void }) => void;
}

// extractValue pulls a value out of an onChange arg: a DOM-ish event
// ({target:{value|checked}}) or a raw value passed directly.
function extractValue(arg: unknown): unknown {
  if (arg && typeof arg === "object" && "target" in arg) {
    const t = (arg as { target: { value?: unknown; checked?: unknown; type?: string } }).target;
    if (t.type === "checkbox") return t.checked;
    return t.value;
  }
  return arg;
}

/**
 * Headless form controller for a schemapb schema. Holds form state and runs the
 * shared WASM engine (validate/compute/bake + when/options/count helpers). You
 * render the fields with your own components — this hook owns only the logic.
 */
export function useSchemaForm(opts: UseSchemaFormOptions): UseSchemaFormResult {
  const { schema, initialValues, mode = "onChange", engine } = opts;

  const [sp, setSp] = useState<Schemapb | null>(engine ?? null);
  const [values, setValuesState] = useState<Values>(initialValues ?? {});
  const [computed, setComputed] = useState<Values>(initialValues ?? {});
  const [errors, setErrors] = useState<Record<string, FieldErrorJson>>({});

  // Load the (cached) WASM engine once, unless one was injected.
  useEffect(() => {
    if (engine) {
      setSp(engine);
      return;
    }
    let live = true;
    schemapb().then((inst) => {
      if (live) setSp(inst);
    });
    return () => {
      live = false;
    };
  }, [engine]);

  // Recompute validation + derived values whenever inputs change (onChange mode).
  useEffect(() => {
    if (!sp || mode !== "onChange") return;
    const v = sp.validate(schema, values);
    setErrors(errorsByField(v.errors));
    const c = sp.compute(schema, values);
    setComputed(c.values as Values);
  }, [sp, schema, values, mode]);

  const setValue = useCallback((path: string, value: unknown) => {
    setValuesState((prev) => setPath(prev, path, value));
  }, []);

  const setValues = useCallback((next: Values) => setValuesState(next), []);

  const reset = useCallback(
    (next?: Values) => setValuesState(next ?? initialValues ?? {}),
    [initialValues],
  );

  const getError = useCallback((path: string) => errors[path], [errors]);

  const register = useCallback(
    (path: string): FieldProps => ({
      name: path,
      value: getPath(values, path),
      error: errors[path],
      onChange: (eventOrValue) => setValue(path, extractValue(eventOrValue)),
    }),
    [values, errors, setValue],
  );

  const fieldActive = useCallback(
    (path: string) => {
      if (!sp) return true;
      try {
        return sp.fieldActive(schema, path, values);
      } catch {
        return true;
      }
    },
    [sp, schema, values],
  );

  const enumOptions = useCallback(
    (path: string) => {
      if (!sp) return [];
      try {
        return sp.enumOptions(schema, path, values);
      } catch {
        return [];
      }
    },
    [sp, schema, values],
  );

  const listCount = useCallback(
    (path: string) => {
      if (!sp) return 0;
      try {
        return sp.listCount(schema, path, values);
      } catch {
        return 0;
      }
    },
    [sp, schema, values],
  );

  // Keep a live ref so the submit handler always sees current values/engine.
  const ref = useRef({ sp, schema, values });
  ref.current = { sp, schema, values };

  const handleSubmit = useCallback<UseSchemaFormResult["handleSubmit"]>(
    (onValid, onInvalid) => (e) => {
      e?.preventDefault?.();
      const { sp: engine, schema: sc, values: vals } = ref.current;
      if (!engine) return;
      const r = engine.bake(sc, vals);
      const errMap = errorsByField(r.errors);
      setErrors(errMap);
      if (r.baked) onValid(r.baked);
      else onInvalid?.(errMap);
    },
    [],
  );

  const isValid = useMemo(() => Object.keys(errors).length === 0, [errors]);

  return {
    ready: sp !== null,
    values,
    computed,
    errors,
    isValid,
    setValue,
    setValues,
    reset,
    getError,
    register,
    fieldActive,
    enumOptions,
    listCount,
    handleSubmit,
  };
}
