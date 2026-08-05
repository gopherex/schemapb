//! Bake / merge / render, mirroring the Go reference bake.go + render.go.

use crate::compute::{field_is_active, ref_def_key, select_variant};
use crate::engine::Engine;
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::{Baked, Schema, StructValue, ValidationResult, Value};
use crate::render::{display_string, render_field, RenderContext, RenderField, RenderGroup};
use crate::validate::{result_blocking, validate};
use crate::value::{
    canonical_struct, canonical_value, from_native, struct_to_native, Native, NativeStruct,
    SchemaField,
};

pub struct BakeOutcome {
    pub result: ValidationResult,
    pub baked: Option<Baked>,
}

/// Validate + resolve, then seal in canonical wire form.
pub(crate) fn bake(e: &Engine, values: &mut NativeStruct) -> BakeOutcome {
    let result = validate(e, values);
    if result_blocking(&result) {
        return BakeOutcome {
            result,
            baked: None,
        };
    }
    let baked = Baked {
        schema: Some(e.schema.clone()),
        values: Some(canonical_engine_struct(e, values)),
    };
    BakeOutcome {
        result,
        baked: Some(baked),
    }
}

fn canonical_engine_struct(e: &Engine, values: &NativeStruct) -> StructValue {
    let fields = values
        .iter()
        .map(|(name, val)| {
            let f = e.schema.fields.iter().find(|x| &x.name == name);
            (name.clone(), canonical_top(e, f, val))
        })
        .collect();
    StructValue { fields }
}

fn canonical_top(e: &Engine, f: Option<&SchemaField>, val: &Native) -> Value {
    let Some(f) = f else {
        return from_native(val);
    };
    match f.kind.as_ref() {
        Some(K::Ref(r)) => {
            if let (Some(def), Some(m)) = (e.schema.defs.get(&ref_def_key(r)), val.as_struct()) {
                if let Ok(v) = canonical_struct(def, m) {
                    return v;
                }
            }
            from_native(val)
        }
        Some(K::OneOf(oo)) => {
            if let (Some(variant), Some(m)) = (select_variant(oo, val), val.as_struct()) {
                if let Ok(v) = canonical_struct(variant, m) {
                    return v;
                }
            }
            from_native(val)
        }
        _ => canonical_value(f, val).unwrap_or_else(|_| from_native(val)),
    }
}

/// Layers overrides onto a baked form and re-seals on this engine.
#[must_use]
pub(crate) fn merge(
    e: &Engine,
    baked: &Baked,
    overrides: &StructValue,
    replace_lists: bool,
) -> BakeOutcome {
    let base = struct_to_native(baked.values.as_ref());
    let over = struct_to_native(Some(overrides));
    let mut merged = merge_structs(&base, &over, replace_lists);
    bake(e, &mut merged)
}

fn merge_structs(dst: &NativeStruct, src: &NativeStruct, replace_lists: bool) -> NativeStruct {
    let mut out = dst.clone();
    for (k, sv) in src {
        match (out.get(k), sv) {
            (Some(Native::Struct(dm)), Native::Struct(sm)) => {
                out.insert(
                    k.clone(),
                    Native::Struct(merge_structs(dm, sm, replace_lists)),
                );
            }
            (Some(Native::List(dl)), Native::List(sl)) if !replace_lists => {
                let mut joined = dl.clone();
                joined.extend(sl.iter().cloned());
                out.insert(k.clone(), Native::List(joined));
            }
            _ => {
                out.insert(k.clone(), sv.clone());
            }
        }
    }
    out
}

/// Whether the baked schema is identical in content to `s`.
#[must_use]
pub(crate) fn baked_matches(baked: &Baked, s: &Schema) -> bool {
    baked.schema.as_ref() == Some(s)
}

/// Renders a schema-carried Mustache template against resolved values.
#[must_use]
pub(crate) fn render(e: &Engine, name: &str, values: &NativeStruct) -> Option<String> {
    e.render_template(name, &build_render_context(e, values))
}

/// Renders a Baked snapshot with a template of its embedded schema.
#[must_use]
pub(crate) fn render_baked(e: &Engine, baked: &Baked, name: &str) -> Option<String> {
    render(e, name, &struct_to_native(baked.values.as_ref()))
}

/// The contract render context; inactive fields excluded entirely.
#[must_use]
pub(crate) fn build_render_context(e: &Engine, values: &NativeStruct) -> RenderContext {
    let mut fields: Vec<RenderField> = Vec::new();
    let mut groups: Vec<RenderGroup> = Vec::new();
    let mut group_idx: std::collections::HashMap<String, usize> = std::collections::HashMap::new();

    for f in &e.schema.fields {
        if !f.when.as_deref().unwrap_or("").is_empty()
            && !field_is_active(e, f, values, &f.name, None)
        {
            continue;
        }
        let rf = render_field(f, values);
        fields.push(rf.clone());
        let g = f.group.clone().unwrap_or_default();
        let i = *group_idx.entry(g.clone()).or_insert_with(|| {
            groups.push(RenderGroup {
                name: g,
                fields: Vec::new(),
            });
            groups.len() - 1
        });
        groups[i].fields.push(rf);
    }

    let display = values
        .iter()
        .map(|(name, val)| {
            (
                name.clone(),
                if val.is_null() {
                    String::new()
                } else {
                    display_string(val)
                },
            )
        })
        .collect();
    RenderContext {
        fields,
        groups,
        values: display,
    }
}

impl Baked {
    /// Whether the embedded schema is identical in content to `s`.
    #[must_use]
    pub fn matches(&self, s: &Schema) -> bool {
        baked_matches(self, s)
    }
}
