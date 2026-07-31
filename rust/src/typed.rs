//! Typed identifier domains (newtypes) and the opaque semver [`Version`].
//! An invalid version is unrepresentable (principle 2); validation and
//! ordering delegate to the `semver` crate.

use crate::gen::schemapb::SchemaIdentity;

macro_rules! string_domain {
    ($(#[$doc:meta] $name:ident),+ $(,)?) => {
        $(
            #[$doc]
            #[derive(Debug, Clone, PartialEq, Eq, Hash)]
            pub struct $name(pub String);

            impl From<&str> for $name {
                fn from(s: &str) -> Self {
                    Self(s.to_owned())
                }
            }

            impl std::fmt::Display for $name {
                fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                    f.write_str(&self.0)
                }
            }
        )+
    };
}

string_domain!(
    /// A schema identity's grouping namespace.
    Namespace,
    /// A schema identity's name, unique within its namespace.
    SchemaName,
    /// One field's name inside a schema.
    FieldName,
    /// A reusable sub-schema name in the root defs map.
    DefName,
    /// A render template name carried by a schema.
    TemplateName,
    /// The stable id of an authored validation rule.
    RuleId,
    /// A field's informative section label.
    GroupName,
    /// A `OneOf` variant key (the discriminator value).
    VariantKey,
    /// A string-format registry identifier ("email", "k8s.quantity").
    Format,
);

/// An always-valid semver value; the zero version means "unversioned" and
/// serializes to the empty string. Constructible only through
/// [`Version::of`] / [`Version::parse`].
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Version(Option<semver::Version>);

impl Version {
    #[must_use]
    pub const fn zero() -> Self {
        Self(None)
    }

    #[must_use]
    pub const fn of(major: u64, minor: u64, patch: u64) -> Self {
        Self(Some(semver::Version::new(major, minor, patch)))
    }

    /// Parses "v1.2.3" / "1.2.3-rc.1"; the empty string is the zero version.
    pub fn parse(s: &str) -> Result<Self, semver::Error> {
        if s.is_empty() {
            return Ok(Self(None));
        }
        let trimmed = s.strip_prefix('v').unwrap_or(s);
        semver::Version::parse(trimmed).map(|v| Self(Some(v)))
    }

    #[must_use]
    pub const fn is_zero(&self) -> bool {
        self.0.is_none()
    }

    /// Semver precedence; the zero version sorts before any real version.
    #[must_use]
    pub fn compare(&self, other: &Self) -> std::cmp::Ordering {
        match (&self.0, &other.0) {
            (None, None) => std::cmp::Ordering::Equal,
            (None, Some(_)) => std::cmp::Ordering::Less,
            (Some(_), None) => std::cmp::Ordering::Greater,
            (Some(a), Some(b)) => a.cmp(b),
        }
    }
}

impl std::fmt::Display for Version {
    /// Canonical wire form ("v1.2.3", "" when unversioned).
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.0.as_ref().map_or(Ok(()), |v| write!(f, "v{v}"))
    }
}

/// Builds a schema identity handle: declare it once next to the schema and
/// reuse the same value everywhere the identity is needed.
#[must_use]
pub fn id(ns: &str, name: &str, ver: &Version) -> SchemaIdentity {
    SchemaIdentity {
        namespace: ns.to_owned(),
        name: name.to_owned(),
        version: ver.to_string(),
    }
}
