//! Schema DESCRIPTOR validation, mirroring the Go reference descriptor.go.

use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::{ErrorCode, Schema, ValidationError, ValidationResult};
use crate::value::SchemaField;

/// A malformed schema descriptor (programmatic failure, principle 5).
#[derive(Debug)]
pub struct SchemaError {
    pub result: ValidationResult,
}

impl std::fmt::Display for SchemaError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "schemapb: invalid schema")?;
        for e in &self.result.errors {
            if e.path.is_empty() {
                write!(f, "; {}", e.message)?;
            } else {
                write!(f, "; {}: {}", e.path, e.message)?;
            }
        }
        Ok(())
    }
}

impl std::error::Error for SchemaError {}

#[must_use]
pub fn schema_err(path: &str, msg: &str) -> ValidationError {
    ValidationError {
        path: path.to_owned(),
        code: ErrorCode::InvalidSchema.into(),
        severity: crate::gen::schemapb::schema::field::Severity::Error.into(),
        message: msg.to_owned(),
        ..Default::default()
    }
}

#[must_use]
pub fn join_path(prefix: &str, name: &str) -> String {
    if prefix.is_empty() {
        name.to_owned()
    } else {
        format!("{prefix}.{name}")
    }
}

/// The schemas directly embedded in a field (not Refs).
#[must_use]
pub fn nested_schemas(f: &SchemaField) -> Vec<&Schema> {
    let mut out = Vec::new();
    match f.kind.as_ref() {
        Some(K::Object(o)) => {
            if let Some(s) = o.schema.as_ref() {
                out.push(s);
            }
        }
        Some(K::OneOf(oo)) => out.extend(oo.variants.values()),
        Some(K::Map(mp)) => {
            if let Some(s) = mp.value_schema.as_ref() {
                out.push(s);
            }
        }
        Some(K::List(l)) => {
            for it in &l.items {
                out.extend(nested_schemas(it));
            }
        }
        _ => {}
    }
    out
}

/// Structural well-formedness of the descriptor (empty = fine).
#[must_use]
pub fn check_descriptor(s: &Schema) -> Vec<ValidationError> {
    let mut errs = Vec::new();
    if s.id.as_ref().is_none_or(|id| id.name.is_empty()) {
        errs.push(schema_err("id.name", "schema identity name is required"));
    }
    errs.extend(check_fields(&s.fields, ""));
    for (name, def) in &s.defs {
        errs.extend(check_fields(&def.fields, &format!("$defs.{name}")));
    }
    errs.extend(check_ref_targets(&s.fields, s, ""));
    for (name, def) in &s.defs {
        errs.extend(check_ref_targets(&def.fields, s, &format!("$defs.{name}")));
    }
    errs
}

#[allow(clippy::too_many_lines)] // flat exhaustive descriptor checks per kind
fn check_fields(fields: &[SchemaField], prefix: &str) -> Vec<ValidationError> {
    let mut errs = Vec::new();
    let mut seen = std::collections::HashSet::new();
    for f in fields {
        let path = join_path(prefix, &f.name);
        if f.name.is_empty() {
            errs.push(schema_err(prefix, "field name is required"));
            continue;
        }
        if !seen.insert(f.name.clone()) {
            errs.push(schema_err(&path, "duplicate field name"));
        }
        let Some(kind) = f.kind.as_ref() else {
            errs.push(schema_err(&path, "field kind is required"));
            continue;
        };
        for (i, r) in f.rules.iter().enumerate() {
            if r.expr.is_empty() {
                errs.push(schema_err(&path, &format!("rule[{i}]: empty expression")));
            }
        }
        match kind {
            K::Computed(c) if c.expr.is_empty() => {
                errs.push(schema_err(&path, "computed field: empty expression"));
            }
            K::OneOf(oo) => {
                if oo.discriminator.is_empty() {
                    errs.push(schema_err(&path, "oneof field: discriminator is required"));
                }
                if oo.variants.is_empty() {
                    errs.push(schema_err(
                        &path,
                        "oneof field: at least one variant is required",
                    ));
                }
            }
            K::Ref(r) if r.target.is_none() => {
                errs.push(schema_err(&path, "ref field: target is required"));
            }
            K::List(l) => {
                if l.items.is_empty() {
                    errs.push(schema_err(
                        &path,
                        "list field: at least one item definition is required",
                    ));
                }
                if l.items.len() > 1
                    && (l.min_items.is_some()
                        || l.max_items.is_some()
                        || l.unique
                        || l.count_expr.as_deref().is_some_and(|s| !s.is_empty()))
                {
                    errs.push(schema_err(
                        &path,
                        "tuple list (multiple item definitions) cannot combine with min_items/max_items/unique/count_expr",
                    ));
                }
            }
            K::Choice(ch) => {
                if !ch.open
                    && ch.options.is_empty()
                    && ch.options_expr.as_deref().unwrap_or("").is_empty()
                {
                    errs.push(schema_err(
                        &path,
                        "choice field: a closed choice requires options or options_expr",
                    ));
                }
                for (i, o) in ch.options.iter().enumerate() {
                    if o.value.is_none() {
                        errs.push(schema_err(
                            &path,
                            &format!("choice option[{i}]: value is required"),
                        ));
                    }
                }
            }
            K::Map(mp) => {
                if let (Some(min), Some(max)) = (mp.min_entries, mp.max_entries) {
                    if min > max {
                        errs.push(schema_err(
                            &path,
                            "map field: min_entries must be <= max_entries",
                        ));
                    }
                }
            }
            _ => {}
        }
        for child in nested_schemas(f) {
            errs.extend(check_fields(&child.fields, &path));
        }
    }
    errs
}

fn check_ref_targets(fields: &[SchemaField], root: &Schema, prefix: &str) -> Vec<ValidationError> {
    use crate::gen::schemapb::schema::field::r#ref::Target;
    let mut errs = Vec::new();
    for f in fields {
        let path = join_path(prefix, &f.name);
        if let Some(K::Ref(r)) = f.kind.as_ref() {
            if let Some(Target::Name(name)) = r.target.as_ref() {
                if !name.is_empty() && !root.defs.contains_key(name) {
                    errs.push(schema_err(
                        &path,
                        &format!("ref {name:?} is not defined in schema defs"),
                    ));
                }
            }
        }
        if let Some(K::List(l)) = f.kind.as_ref() {
            errs.extend(check_ref_targets(&l.items, root, &format!("{path}[]")));
            continue;
        }
        for child in nested_schemas(f) {
            errs.extend(check_ref_targets(&child.fields, root, &path));
        }
    }
    errs
}
