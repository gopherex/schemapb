//! Reflect conformance: the derive mirror model must produce the exact
//! Schema the Go reference reflected into conformance/golden/reflect.json
//! (field order included). Requires `--features derive`.
#![cfg(feature = "derive")]

use std::collections::HashMap;

use schemapb::gen::schemapb::{Schema, SchemaIdentity};
use schemapb::reflect::{reflect_schema, ReflectField};
use schemapb::Reflect;

fn golden(name: &str) -> String {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../conformance/golden")
        .join(name);
    std::fs::read_to_string(path).unwrap()
}

fn mirror_id() -> SchemaIdentity {
    SchemaIdentity {
        namespace: "conformance".into(),
        name: "mirror".into(),
        version: "v1.0.0".into(),
    }
}

#[derive(Reflect)]
#[allow(dead_code)]
struct MirrorNested {
    on: bool,
}

#[derive(Reflect)]
#[allow(dead_code)]
struct MirrorNode {
    next: Option<Box<Self>>,
}

/// Inheritance/embedding has no Rust analogue; the flattened field is
/// declared first, like the base declares it.
#[derive(Reflect)]
#[allow(dead_code)]
struct MirrorModel {
    base: String,
    #[schemapb(desc = "display name", min = 1, max = 64)]
    name: String,
    #[schemapb(format = "email")]
    mail: String,
    #[schemapb(pattern = "^[a-z-]+$")]
    slug: String,
    #[schemapb(oneof("fast", "safe"))]
    mode: String,
    #[schemapb(oneof(1, 2, 3))]
    level: i64,
    #[schemapb(gte = 0, lte = 100)]
    count: i32,
    #[schemapb(gt = -10, lt = 10)]
    big: i64,
    #[schemapb(max = 65535)]
    port: u16,
    total: u64,
    #[schemapb(gte = 0)]
    ratio: f32,
    #[schemapb(lt = 1)]
    score: f64,
    flag: bool,
    opt: Option<String>,
    #[schemapb(required)]
    must: Option<i64>,
    #[schemapb(min = 1, max = 5)]
    tags: Vec<String>,
    pair: [String; 2],
    #[schemapb(max = 16)]
    blob: Vec<u8>,
    magic: [u8; 4],
    limits: HashMap<String, i64>,
    extra: HashMap<String, MirrorNested>,
    nested: MirrorNested,
    when: pbjson_types::Timestamp,
    wait: pbjson_types::Duration,
    raw: serde_json::Value,
    anything: serde_json::Value,
    chain: MirrorNode,
    #[schemapb(skip)]
    gone: String,
}

#[test]
fn mirror_matches_golden() {
    let got = reflect_schema::<MirrorModel>(mirror_id()).unwrap();
    let want: Schema = serde_json::from_str(&golden("reflect.json")).unwrap();
    assert_eq!(
        serde_json::to_value(&got).unwrap(),
        serde_json::to_value(&want).unwrap()
    );
}

#[test]
fn override_is_a_trait_impl() {
    struct SecretName(#[allow(dead_code)] String);

    impl ReflectField for SecretName {
        fn field(name: &str) -> schemapb::gen::schemapb::schema::Field {
            let mut f = String::field(name);
            f.secret = true;
            f
        }
    }

    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Params {
        token: SecretName,
    }

    let s = reflect_schema::<Params>(mirror_id()).unwrap();
    let token = s.lookup_path("token").unwrap();
    assert!(token.secret);
    assert_eq!(token.kind_name(), "string");
}

#[test]
#[should_panic(expected = "does not apply")]
fn misapplied_constraint_is_loud() {
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Bad {
        #[schemapb(gte = 3)]
        flag: bool,
    }

    let _ = reflect_schema::<Bad>(mirror_id());
}

/// Native trait overrides express the same attributes as Go's generic tags.
#[test]
fn reflect_attributes_match_go_tags() {
    use schemapb::gen::schemapb::schema::{field::Kind, Field};
    use schemapb::gen::schemapb::StructValue;

    struct Token;
    impl ReflectField for Token {
        fn field(name: &str) -> Field {
            let mut f = String::field(name);
            f.secret = true;
            if let Some(Kind::String(k)) = f.kind.as_mut() {
                k.default = Some("token".into());
                k.min_len = Some(1);
            }
            f
        }
    }
    struct Workers;
    impl ReflectField for Workers {
        fn field(name: &str) -> Field {
            let mut f = i32::field(name);
            if let Some(Kind::Int32(k)) = f.kind.as_mut() {
                k.default = Some(4);
                k.gte = Some(1);
                k.lte = Some(64);
            }
            f
        }
    }
    struct Enabled;
    impl ReflectField for Enabled {
        fn field(name: &str) -> Field {
            let mut f = bool::field(name);
            if let Some(Kind::Bool(k)) = f.kind.as_mut() {
                k.default = Some(false);
            }
            f
        }
    }
    struct Timeout;
    impl ReflectField for Timeout {
        fn field(name: &str) -> Field {
            let mut f = pbjson_types::Duration::field(name);
            if let Some(Kind::Duration(k)) = f.kind.as_mut() {
                k.default = Some(pbjson_types::Duration {
                    seconds: 30,
                    nanos: 0,
                });
            }
            f
        }
    }
    struct Empty;
    impl ReflectField for Empty {
        fn field(name: &str) -> Field {
            let mut f = String::field(name);
            if let Some(Kind::String(k)) = f.kind.as_mut() {
                k.default = Some(String::new());
            }
            f
        }
    }
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Model {
        token: Token,
        workers: Workers,
        enabled: Enabled,
        timeout: Timeout,
        empty: Option<Empty>,
    }
    let schema = reflect_schema::<Model>(SchemaIdentity {
        namespace: "conformance".into(),
        name: "reflect_attributes".into(),
        version: "v1.0.0".into(),
    })
    .unwrap();
    let want: Schema = serde_json::from_str(&golden("reflect-attributes.json")).unwrap();
    assert_eq!(schema, want);
    let e = schemapb::engine::Engine::compile(schema, schemapb::formats::FormatRegistry::new())
        .unwrap();
    let outcome = e.bake(&mut schemapb::value::NativeStruct::new());
    assert!(outcome.result.errors.is_empty());
    let values: StructValue =
        serde_json::from_str(&golden("reflect-attributes-baked.json")).unwrap();
    assert_eq!(outcome.baked.unwrap().values, Some(values));
}
