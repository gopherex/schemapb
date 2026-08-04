//! Schema path lookup: resolving a dot path ("a.b.c") to the field it
//! addresses.
//!
//! Paths address FIELDS, not values, so they carry no list indices or map
//! keys: lookup descends through Object fields and resolves Refs against
//! root $defs; every other kind is terminal — a path may END on a
//! list/map/oneof field but cannot continue through one.
//!
//! Failures point at the exact segment that broke ("no field b in a",
//! never "a.b.c not found").

use crate::compute::ref_def_key;
use crate::descriptor::join_path;
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::Schema;
use crate::render::kind_name;
use crate::value::SchemaField;

/// Why a schema path failed to resolve. The wire strings are stable spec
/// values shared by all implementations (conformance).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LookupReason {
    EmptyPath,
    NotFound,
    NotTraversable,
    AmbiguousOneOf,
    UnknownRef,
}

impl LookupReason {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::EmptyPath => "empty_path",
            Self::NotFound => "not_found",
            Self::NotTraversable => "not_traversable",
            Self::AmbiguousOneOf => "ambiguous_oneof",
            Self::UnknownRef => "unknown_ref",
        }
    }
}

/// Pinpoints the failing segment of a schema path: `at` is the resolved
/// parent path ("" for root), `segment` the name that failed, `kind` the
/// kind of the offending field (set for the traversal reasons).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LookupError {
    pub at: String,
    pub segment: String,
    pub reason: LookupReason,
    pub kind: &'static str,
}

impl std::fmt::Display for LookupError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let where_ = if self.at.is_empty() {
            "root".to_owned()
        } else {
            format!("{:?}", self.at)
        };

        match self.reason {
            LookupReason::EmptyPath => write!(f, "schemapb: lookup: empty path"),
            LookupReason::NotFound => {
                write!(f, "schemapb: lookup: no field {:?} in {where_}", self.segment)
            }
            LookupReason::AmbiguousOneOf => write!(
                f,
                "schemapb: lookup: cannot descend into oneof {:?} in {where_}: the variant depends on a discriminator value",
                self.segment
            ),
            LookupReason::UnknownRef => write!(
                f,
                "schemapb: lookup: ref {:?} in {where_} points to a def that does not exist",
                self.segment
            ),
            LookupReason::NotTraversable => write!(
                f,
                "schemapb: lookup: cannot descend into {:?} in {where_} (kind {})",
                self.segment, self.kind
            ),
        }
    }
}

impl std::error::Error for LookupError {}

fn err(reason: LookupReason, at: &str, segment: &str, kind: &'static str) -> LookupError {
    LookupError {
        at: at.to_owned(),
        segment: segment.to_owned(),
        reason,
        kind,
    }
}

/// Resolves a field path within the schema, one segment per field name.
/// Returns the addressed field, or a `LookupError` naming the exact
/// segment that failed.
pub fn lookup<'a>(s: &'a Schema, segments: &[&str]) -> Result<&'a SchemaField, LookupError> {
    if segments.is_empty() {
        return Err(err(LookupReason::EmptyPath, "", "", ""));
    }

    let mut cur: Option<&Schema> = Some(s);
    let mut parent = String::new();

    for (i, seg) in segments.iter().enumerate() {
        let f = cur
            .and_then(|c| c.fields.iter().find(|c| c.name == *seg))
            .ok_or_else(|| err(LookupReason::NotFound, &parent, seg, ""))?;

        if i == segments.len() - 1 {
            return Ok(f);
        }

        match f.kind.as_ref() {
            Some(K::Object(o)) => cur = o.schema.as_ref(),
            Some(K::Ref(r)) => match s.defs.get(&ref_def_key(r)) {
                Some(def) => cur = Some(def),
                None => return Err(err(LookupReason::UnknownRef, &parent, seg, "ref")),
            },
            Some(K::OneOf(_)) => {
                return Err(err(LookupReason::AmbiguousOneOf, &parent, seg, "oneof"));
            }
            _ => {
                return Err(err(
                    LookupReason::NotTraversable,
                    &parent,
                    seg,
                    kind_name(f),
                ))
            }
        }

        parent = join_path(&parent, seg);
    }

    unreachable!()
}

/// `lookup` over a dot-separated path ("a.b.c"). Field names are
/// identifiers (enforced by descriptor validation), so the dot is never
/// part of a name.
pub fn lookup_path<'a>(s: &'a Schema, path: &str) -> Result<&'a SchemaField, LookupError> {
    if path.is_empty() {
        return Err(err(LookupReason::EmptyPath, "", "", ""));
    }

    let segments: Vec<&str> = path.split('.').collect();
    lookup(s, &segments)
}
