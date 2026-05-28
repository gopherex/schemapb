# @stroppy-io/schemapb-react

Headless React hook for building forms from a [`schemapb`](../schemapb) schema.
It owns the **logic** — form state plus the shared WASM engine
(validate / compute / bake, and the `when` / `options_expr` / `count_expr`
helpers) — and leaves **rendering entirely to you**. No styles, no components,
works with any design system.

```bash
npm i @stroppy-io/schemapb-react @stroppy-io/schemapb react
```

## Usage

```tsx
import { useSchemaForm } from "@stroppy-io/schemapb-react";
import type { Schema } from "@stroppy-io/schemapb";

function ConfigForm({ schema }: { schema: Schema }) {
  const form = useSchemaForm({ schema, initialValues: {} });

  if (!form.ready) return <p>Loading…</p>; // WASM loads on first use

  const submit = form.handleSubmit(
    (baked) => save(baked.values),       // valid: sealed snapshot
    (errors) => console.warn(errors),    // invalid: errors by field path
  );

  return (
    <form onSubmit={submit}>
      {/* your own components, wired via register(path) */}
      <input {...textProps(form.register("name"))} />
      {form.errors.name && <span>{form.errors.name.message}</span>}

      {/* conditional fields: hide when the `when` gate is false */}
      {form.fieldActive("tls_cert") && (
        <input {...textProps(form.register("tls_cert"))} />
      )}

      {/* dynamic enum options come from the schema + current form */}
      <select {...selectProps(form.register("version"))}>
        {form.enumOptions("version").map((v) => <option key={v} value={v}>{v}</option>)}
      </select>

      <button type="submit">Save</button>
    </form>
  );
}
```

`textProps`/`selectProps` are your tiny adapters from `FieldProps`
(`{ name, value, error, onChange }`) to your component's props.

## API — `useSchemaForm(options)`

Options: `{ schema, initialValues?, mode?: "onChange" | "onSubmit", engine? }`.
Pass `engine` (a pre-loaded `Schemapb`) for SSR or a single app-wide instance;
otherwise the shared engine auto-loads on first use.

Returns:

| | |
|---|---|
| `ready` | WASM engine loaded |
| `values` / `computed` | raw inputs / resolved (defaults + Computed) |
| `errors` | `Record<fieldPath, FieldError>` |
| `isValid` | no blocking errors |
| `setValue(path, v)` / `setValues(v)` / `reset(v?)` | mutate state (dotted/indexed paths) |
| `register(path)` | `{ name, value, error, onChange }` for a controlled input |
| `getError(path)` | error for a field |
| `fieldActive(path)` | `when` gate result |
| `enumOptions(path)` / `listCount(path)` | dynamic enum options / list length |
| `handleSubmit(onValid, onInvalid?)` | validates + bakes; calls back with the sealed `Baked` or the errors |

The engine is the same Go code as the server, compiled to WASM — so the form
validates and computes **identically** to backend validation.
