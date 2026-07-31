//! Identity-addressed schema registry and identity-ref linking.

use std::collections::HashMap;

use crate::descriptor::nested_schemas;
use crate::gen::schemapb::schema::field::r#ref::Target;
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::{Schema, SchemaIdentity};
use crate::value::SchemaField;

/// A registry misuse (missing name, conflicting identity, broken link).
#[derive(Debug)]
pub struct RegistryError(pub String);

impl std::fmt::Display for RegistryError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for RegistryError {}

/// NUL-joined identity key (never collides with local def names).
#[must_use]
pub fn identity_key(id: Option<&SchemaIdentity>) -> String {
    id.map_or_else(
        || "\0\0".to_owned(),
        |id| format!("{}\0{}\0{}", id.namespace, id.name, id.version),
    )
}

/// In-memory identity-addressed store; strict put + explicit replace.
#[derive(Default)]
pub struct InMemoryRegistry {
    m: HashMap<String, Schema>,
}

impl InMemoryRegistry {
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Stores a schema under its own identity; a DIFFERENT schema under a
    /// taken identity is rejected (idempotent re-put is fine).
    pub fn put(&mut self, s: Schema) -> Result<(), RegistryError> {
        let Some(id) = s.id.as_ref().filter(|id| !id.name.is_empty()) else {
            return Err(RegistryError(
                "schemapb: schema identity requires a name".to_owned(),
            ));
        };
        let key = identity_key(Some(id));
        if let Some(existing) = self.m.get(&key) {
            if existing != &s {
                return Err(RegistryError(format!(
                    "schemapb: identity already registered with different content: {}",
                    id.name
                )));
            }
        }
        self.m.insert(key, s);
        Ok(())
    }

    /// Stores the schema unconditionally.
    pub fn put_replace(&mut self, s: Schema) -> Result<(), RegistryError> {
        let Some(id) = s.id.as_ref().filter(|id| !id.name.is_empty()) else {
            return Err(RegistryError(
                "schemapb: schema identity requires a name".to_owned(),
            ));
        };
        let key = identity_key(Some(id));
        self.m.insert(key, s);
        Ok(())
    }

    #[must_use]
    pub fn get(&self, id: &SchemaIdentity) -> Option<&Schema> {
        self.m.get(&identity_key(Some(id)))
    }
}

fn collect_id_refs(fields: &[SchemaField], out: &mut HashMap<String, SchemaIdentity>) {
    for f in fields {
        if let Some(K::Ref(r)) = f.kind.as_ref() {
            if let Some(Target::Id(id)) = r.target.as_ref() {
                out.insert(identity_key(Some(id)), id.clone());
            }
        }
        if let Some(K::List(l)) = f.kind.as_ref() {
            collect_id_refs(&l.items, out);
            continue;
        }
        for child in nested_schemas(f) {
            collect_id_refs(&child.fields, out);
        }
    }
}

/// Pulls every identity-ref into the root defs (transitively); returns a
/// linked clone.
pub fn link(s: &Schema, reg: &InMemoryRegistry) -> Result<Schema, RegistryError> {
    let mut root = s.clone();
    loop {
        let mut ids = HashMap::new();
        collect_id_refs(&root.fields, &mut ids);
        for def in root.defs.values() {
            collect_id_refs(&def.fields, &mut ids);
        }
        let mut added = false;
        for (key, id) in ids {
            if root.defs.contains_key(&key) {
                continue;
            }
            let Some(resolved) = reg.get(&id) else {
                return Err(RegistryError(format!(
                    "schemapb: link: cannot resolve schema {}",
                    id.name
                )));
            };
            let mut cloned = resolved.clone();
            for (k, d) in std::mem::take(&mut cloned.defs) {
                root.defs.entry(k).or_insert(d);
            }
            root.defs.insert(key, cloned);
            added = true;
        }
        if !added {
            return Ok(root);
        }
    }
}
