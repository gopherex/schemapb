//! Schema-driven masking of wire snapshots, without resolution or CEL.
use std::collections::HashMap;

use crate::compute::{list_item_def, ref_def_key};
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::value::Kind as V;
use crate::gen::schemapb::{Baked, Schema, StructValue, Value};
use crate::value::SchemaField;

impl Baked {
    /// Independent display values; present secrets become the string "***".
    #[must_use]
    pub fn masked(&self) -> StructValue {
        let mut out = self.values.clone().unwrap_or_default();
        let empty_defs = HashMap::new();
        let defs = self.schema.as_ref().map_or(&empty_defs, |s| &s.defs);
        mask_struct(self.schema.as_ref(), &mut out, defs);
        out
    }
}

fn hidden() -> Value {
    Value {
        kind: Some(V::StringValue("***".into())),
    }
}

fn mask_struct(schema: Option<&Schema>, values: &mut StructValue, defs: &HashMap<String, Schema>) {
    let Some(schema) = schema else {
        for value in values.fields.values_mut() {
            *value = hidden();
        }
        return;
    };
    for field in &schema.fields {
        if let Some(value) = values.fields.get_mut(&field.name) {
            mask_value(field, value, defs);
        }
    }
}

fn mask_object(schema: Option<&Schema>, value: &mut Value, defs: &HashMap<String, Schema>) {
    if let (Some(schema), Some(V::StructValue(values))) = (schema, &mut value.kind) {
        mask_struct(Some(schema), values, defs);
    } else {
        *value = hidden();
    }
}

fn mask_value(field: &SchemaField, value: &mut Value, defs: &HashMap<String, Schema>) {
    if field.secret {
        *value = hidden();
        return;
    }
    if matches!(value.kind, None | Some(V::NullValue(_))) {
        return;
    }
    match field.kind.as_ref() {
        Some(K::Object(o)) => mask_object(o.schema.as_ref(), value, defs),
        Some(K::Ref(r)) => mask_object(defs.get(&ref_def_key(r)), value, defs),
        Some(K::OneOf(oo)) => {
            let schema = if let Some(V::StructValue(values)) = &value.kind {
                values
                    .fields
                    .get(&oo.discriminator)
                    .and_then(|v| match &v.kind {
                        Some(V::StringValue(disc)) => oo.variants.get(disc),
                        _ => None,
                    })
            } else {
                None
            };
            mask_object(schema, value, defs);
        }
        Some(K::List(list)) => {
            if let Some(V::ListValue(values)) = &mut value.kind {
                for (i, item) in values.items.iter_mut().enumerate() {
                    if let Some(def) = list_item_def(list, i) {
                        mask_value(def, item, defs);
                    }
                }
            } else {
                *value = hidden();
            }
        }
        Some(K::Map(mp)) => {
            if let Some(V::StructValue(values)) = &mut value.kind {
                for item in values.fields.values_mut() {
                    if let Some(def) = &mp.value_field {
                        mask_value(def, item, defs);
                    } else if mp.value_schema.is_some()
                        && !matches!(item.kind, Some(V::NullValue(_)))
                    {
                        mask_object(mp.value_schema.as_ref(), item, defs);
                    }
                }
            } else {
                *value = hidden();
            }
        }
        _ => {}
    }
}
