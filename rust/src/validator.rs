//! `Validator`: a compiled schema checking [`Values`], with outcomes as
//! plain Rust data — the entry point for an application that never touches
//! the proto types.

use std::fmt;

use crate::descriptor::SchemaError;
use crate::engine::Engine;
use crate::formats::FormatRegistry;
use crate::gen::schemapb::schema::field::Severity as PbSeverity;
use crate::gen::schemapb::{ErrorCode, Schema, ValidationError, ValidationResult};
use crate::value::{to_native, Native};
use crate::values::Values;

/// A compiled schema. Compilation checks the whole schema (structure, CEL,
/// patterns, templates), so every defect surfaces once, here.
pub struct Validator {
    engine: Engine,
}

/// How much a violation weighs.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Severity {
    /// Blocks: the values are invalid.
    Error,
    /// Reported, but the values stay valid.
    Warning,
}

/// One failed check.
#[derive(Debug, Clone, PartialEq)]
pub struct Violation {
    /// The value's path: `estimate`, `labels[2]`, `limits.max`.
    pub path: String,
    /// The stable machine code; branch on it, never on `message`.
    pub code: ErrorCode,
    /// The constraint that failed (`gte`, `pattern`, ...); empty for a rule.
    pub constraint: String,
    /// The id of the failed rule, when it has one.
    pub rule_id: Option<String>,
    pub severity: Severity,
    /// The bound or set the value missed (`gte` → its bound).
    pub expected: Option<Native>,
    /// The offending value; `None` for a secret field.
    pub actual: Option<Native>,
    /// English text for people (see `messages` for translation keys).
    pub message: String,
}

/// Everything a check found, errors and warnings, in the spec's order.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Report {
    pub violations: Vec<Violation>,
}

/// The values are invalid: the violations that block, warnings included.
#[derive(Debug, Clone, PartialEq)]
pub struct Invalid {
    pub report: Report,
}

impl Report {
    #[must_use]
    pub fn is_valid(&self) -> bool {
        self.errors().next().is_none()
    }

    pub fn errors(&self) -> impl Iterator<Item = &Violation> {
        self.violations
            .iter()
            .filter(|v| v.severity == Severity::Error)
    }

    pub fn warnings(&self) -> impl Iterator<Item = &Violation> {
        self.violations
            .iter()
            .filter(|v| v.severity == Severity::Warning)
    }

    /// The violations at `path`.
    pub fn at<'a>(&'a self, path: &'a str) -> impl Iterator<Item = &'a Violation> {
        self.violations.iter().filter(move |v| v.path == path)
    }

    fn from_result(result: &ValidationResult) -> Self {
        Self {
            violations: result.errors.iter().map(violation).collect(),
        }
    }

    fn into_outcome(self) -> Result<Self, Invalid> {
        if self.is_valid() {
            Ok(self)
        } else {
            Err(Invalid { report: self })
        }
    }
}

fn violation(e: &ValidationError) -> Violation {
    Violation {
        path: e.path.clone(),
        code: ErrorCode::try_from(e.code).unwrap_or(ErrorCode::Unspecified),
        constraint: e.constraint.clone(),
        rule_id: e.rule_id.clone(),
        severity: if e.severity == PbSeverity::Warning as i32 {
            Severity::Warning
        } else {
            Severity::Error
        },
        expected: e.expected.as_ref().map(|v| to_native(Some(v))),
        actual: e.actual.as_ref().map(|v| to_native(Some(v))),
        message: e.message.clone(),
    }
}

impl fmt::Display for Violation {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.path.is_empty() {
            f.write_str(&self.message)
        } else {
            write!(f, "{}: {}", self.path, self.message)
        }
    }
}

impl fmt::Display for Invalid {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("invalid values")?;
        for v in self.report.errors() {
            write!(f, "; {v}")?;
        }
        Ok(())
    }
}

impl std::error::Error for Invalid {}

/// A stored schema that does not load.
#[derive(Debug)]
pub enum LoadError {
    /// Not a schemapb schema in protoJSON form.
    Json(serde_json::Error),
    /// A schema that does not compile.
    Schema(SchemaError),
}

impl fmt::Display for LoadError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Json(e) => write!(f, "schemapb: schema json: {e}"),
            Self::Schema(e) => e.fmt(f),
        }
    }
}

impl std::error::Error for LoadError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Json(e) => Some(e),
            Self::Schema(e) => Some(e),
        }
    }
}

impl Validator {
    /// Compiles `schema` with the core string formats.
    pub fn new(schema: Schema) -> Result<Self, SchemaError> {
        Self::with_formats(schema, FormatRegistry::new())
    }

    /// Compiles `schema` with extension string formats.
    pub fn with_formats(schema: Schema, formats: FormatRegistry) -> Result<Self, SchemaError> {
        Engine::compile(schema, formats).map(|engine| Self { engine })
    }

    /// Loads a schema stored with [`Self::schema_json`].
    pub fn from_json(json: &str) -> Result<Self, LoadError> {
        let schema: Schema = serde_json::from_str(json).map_err(LoadError::Json)?;
        Self::new(schema).map_err(LoadError::Schema)
    }

    /// The schema in protoJSON, the form to store and load it with.
    #[must_use]
    pub fn schema_json(&self) -> String {
        serde_json::to_string(&self.engine.schema).unwrap_or_default()
    }

    #[must_use]
    pub const fn schema(&self) -> &Schema {
        &self.engine.schema
    }

    /// The compiled engine, for rendering, baking and the rest of its API.
    #[must_use]
    pub const fn engine(&self) -> &Engine {
        &self.engine
    }

    /// Checks `values` without changing them: every error and warning.
    #[must_use]
    pub fn check(&self, values: &Values) -> Report {
        let mut copy = values.as_native().clone();
        Report::from_result(&self.engine.validate(&mut copy))
    }

    /// `Ok` with the warnings when `values` are valid, `Err` otherwise.
    pub fn validate(&self, values: &Values) -> Result<Report, Invalid> {
        self.check(values).into_outcome()
    }

    /// Validates and, only when valid, completes `values` in place:
    /// defaults, coercion to the declared kinds, normalization, computed
    /// fields. Invalid values stay exactly as they were.
    pub fn resolve(&self, values: &mut Values) -> Result<Report, Invalid> {
        let mut copy = values.as_native().clone();
        let report = Report::from_result(&self.engine.validate(&mut copy)).into_outcome()?;
        *values = Values::from(copy);
        Ok(report)
    }
}

impl fmt::Debug for Validator {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Validator")
            .field("schema", &self.engine.schema.id)
            .finish_non_exhaustive()
    }
}
