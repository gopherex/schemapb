//! `Validate`: a Rust type checked against the schema its definition
//! declares — the `validator`-crate workflow with the whole schemapb engine
//! behind it (CEL rules, defaults, normalization, computed fields).
//!
//! `#[derive(Validate)]` (feature `derive`) implements it together with
//! [`ReflectField`]; the schema is compiled once per type, on first use.

use std::any::TypeId;
use std::collections::HashMap;
use std::sync::{Mutex, OnceLock, PoisonError};

use serde::de::DeserializeOwned;
use serde::Serialize;

use crate::gen::schemapb::{ErrorCode, SchemaIdentity};
use crate::native::{decode, encode, fit};
use crate::reflect::{reflect_schema, ReflectField};
use crate::validator::{Invalid, Report, Severity, Validator, Violation};
use crate::value::Native;
use crate::values::Values;

/// A type whose values are validated against the schema its definition
/// declares.
pub trait Validate: ReflectField + Serialize + Sized + 'static {
    /// The identity of the type's schema (the derive uses the crate name
    /// and the type name).
    fn schema_id() -> SchemaIdentity;

    /// The compiled schema, built once per type.
    ///
    /// # Panics
    ///
    /// When the type's attributes declare a schema that does not compile
    /// (a CEL rule or a pattern with an error): a defect of the type's
    /// definition, reported with the schema errors on first use.
    #[must_use]
    fn validator() -> &'static Validator {
        cached::<Self>()
    }

    /// The value as field values, shaped by the schema.
    fn values(&self) -> Result<Values, Invalid> {
        let native = encode(self).map_err(|e| encoding(&e.0))?;
        let Native::Struct(mut m) = native else {
            return Err(encoding("a Validate type serializes as an object"));
        };
        fit(Self::validator().schema(), &mut m);
        Ok(Values::from(m))
    }

    /// Every error and warning of this value.
    #[must_use]
    fn check(&self) -> Report {
        match self.values() {
            Ok(values) => Self::validator().check(&values),
            Err(invalid) => invalid.report,
        }
    }

    /// `Ok` with the warnings when the value is valid, `Err` otherwise.
    fn validate(&self) -> Result<Report, Invalid> {
        Self::validator().validate(&self.values()?)
    }

    /// The value completed by its schema — defaults for absent (`None`)
    /// fields, normalization, computed fields — when valid.
    fn resolved(&self) -> Result<Self, Invalid>
    where
        Self: DeserializeOwned,
    {
        let mut values = self.values()?;
        Self::validator().resolve(&mut values)?;
        decode(Native::Struct(values.into_native())).map_err(|e| encoding(&e.0))
    }
}

fn encoding(message: &str) -> Invalid {
    Invalid {
        report: Report {
            violations: vec![Violation {
                path: String::new(),
                code: ErrorCode::TypeMismatch,
                constraint: String::new(),
                rule_id: None,
                severity: Severity::Error,
                expected: None,
                actual: None,
                message: format!("schemapb: {message}"),
            }],
        },
    }
}

/// One compiled validator per type, generic types included (a static in a
/// generic function would be shared by every instantiation).
fn cached<T: Validate>() -> &'static Validator {
    static CACHE: OnceLock<Mutex<HashMap<TypeId, &'static Validator>>> = OnceLock::new();
    let cache = CACHE.get_or_init(Mutex::default);
    let mut map = cache.lock().unwrap_or_else(PoisonError::into_inner);
    map.entry(TypeId::of::<T>()).or_insert_with(|| {
        let schema = reflect_schema::<T>(T::schema_id())
            .unwrap_or_else(|e| panic!("schemapb: {}: {e}", std::any::type_name::<T>()));
        let validator = Validator::new(schema)
            .unwrap_or_else(|e| panic!("schemapb: {}: {e}", std::any::type_name::<T>()));
        Box::leak(Box::new(validator))
    })
}
