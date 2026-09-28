//! Sections: a struct field whose type is a derived struct (not
//! `Option<T>`) is optional with an implicit empty object default, so an
//! absent section still resolves its inner defaults. Mirrors the Go
//! reference (`go/schemapb/reflect_section_test.go`). Requires
//! `--features derive`.
#![cfg(feature = "derive")]

use schemapb::engine::Engine;
use schemapb::formats::FormatRegistry;
use schemapb::gen::schemapb::schema::{field::Kind, Field};
use schemapb::gen::schemapb::{ErrorCode, Schema, SchemaIdentity, StructValue, ValidationResult};
use schemapb::reflect::{reflect_schema, ReflectField};
use schemapb::value::{Native, NativeStruct};
use schemapb::Reflect;

fn id() -> SchemaIdentity {
    SchemaIdentity {
        namespace: "t".into(),
        name: "section".into(),
        version: "v1.0.0".into(),
    }
}

// Inner defaults come from the native override mechanism (a trait impl),
// like Go's `schemapb:"default=..."` tags.
struct Host;
impl ReflectField for Host {
    fn field(name: &str) -> Field {
        let mut f = String::field(name);
        if let Some(Kind::String(k)) = f.kind.as_mut() {
            k.default = Some("localhost".into());
        }
        f
    }
}

struct Port;
impl ReflectField for Port {
    fn field(name: &str) -> Field {
        let mut f = i64::field(name);
        if let Some(Kind::Int64(k)) = f.kind.as_mut() {
            k.default = Some(5432);
        }
        f
    }
}

struct Realm;
impl ReflectField for Realm {
    fn field(name: &str) -> Field {
        let mut f = String::field(name);
        if let Some(Kind::String(k)) = f.kind.as_mut() {
            k.default = Some("main".into());
        }
        f
    }
}

#[derive(Reflect)]
#[allow(dead_code)]
struct SectionDb {
    host: Host,
    port: Port,
}

#[derive(Reflect)]
#[allow(dead_code)]
struct SectionAuth {
    token: String,
    realm: Realm,
}

const fn object_default(f: &Field) -> Option<&StructValue> {
    match f.kind.as_ref() {
        Some(Kind::Object(o)) => o.default.as_ref(),
        _ => None,
    }
}

fn bake(schema: Schema, mut input: NativeStruct) -> (ValidationResult, Option<NativeStruct>) {
    let e = Engine::compile(schema, FormatRegistry::new()).unwrap();
    let outcome = e.bake(&mut input);
    let values = outcome
        .baked
        .map(|b| schemapb::value::struct_to_native(b.values.as_ref()));
    (outcome.result, values)
}

fn has_error(res: &ValidationResult, path: &str, code: ErrorCode) -> bool {
    res.errors
        .iter()
        .any(|e| e.path == path && e.code == code as i32)
}

#[test]
fn section_optional_with_inner_defaults() {
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Config {
        db: SectionDb,
    }

    let schema = reflect_schema::<Config>(id()).unwrap();
    let f = schema.lookup_path("db").unwrap();
    assert!(!f.required);
    assert!(object_default(f).is_some());

    let (res, values) = bake(schema, NativeStruct::new());
    assert!(res.errors.is_empty(), "{res:?}");
    let want = NativeStruct::from([(
        "db".to_owned(),
        Native::Struct(NativeStruct::from([
            ("host".to_owned(), Native::Str("localhost".into())),
            ("port".to_owned(), Native::Int(5432)),
        ])),
    )]);
    assert_eq!(values, Some(want));
}

#[test]
fn section_required_inner_field_reported_inside() {
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Config {
        auth: SectionAuth,
    }

    let (res, _) = bake(reflect_schema::<Config>(id()).unwrap(), NativeStruct::new());
    assert!(
        has_error(&res, "auth.token", ErrorCode::RequiredMissing),
        "{res:?}"
    );
    assert!(
        !has_error(&res, "auth", ErrorCode::RequiredMissing),
        "{res:?}"
    );
}

#[test]
fn section_explicit_required() {
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Config {
        #[schemapb(required)]
        db: SectionDb,
    }

    let schema = reflect_schema::<Config>(id()).unwrap();
    let f = schema.lookup_path("db").unwrap();
    assert!(f.required);
    assert!(object_default(f).is_none());

    let (res, _) = bake(schema, NativeStruct::new());
    assert!(has_error(&res, "db", ErrorCode::RequiredMissing), "{res:?}");
}

#[test]
fn option_section_unchanged() {
    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Config {
        db: Option<SectionDb>,
    }

    let schema = reflect_schema::<Config>(id()).unwrap();
    let f = schema.lookup_path("db").unwrap();
    assert!(!f.required);
    assert!(f.nullable);
    assert!(object_default(f).is_none());

    let (res, values) = bake(schema, NativeStruct::new());
    assert!(res.errors.is_empty(), "{res:?}");
    assert_eq!(values, Some(NativeStruct::new()));
}

#[test]
fn overrides_and_items_are_not_sections() {
    struct Opaque;
    impl ReflectField for Opaque {
        fn field(name: &str) -> Field {
            SectionDb::field(name)
        }
    }

    #[derive(Reflect)]
    #[allow(dead_code)]
    struct Config {
        opaque: Opaque,
        dbs: Vec<SectionDb>,
        boxed: Box<SectionDb>,
    }

    let schema = reflect_schema::<Config>(id()).unwrap();
    let opaque = schema.lookup_path("opaque").unwrap();
    assert!(opaque.required);
    assert!(object_default(opaque).is_none());

    let dbs = schema.lookup_path("dbs").unwrap();
    let Some(Kind::List(list)) = dbs.kind.as_ref() else {
        panic!("dbs is not a list: {dbs:?}");
    };
    assert!(list.items[0].required);
    assert!(object_default(&list.items[0]).is_none());

    // Box<T> is a plain value to serde: it stays a section.
    let boxed = schema.lookup_path("boxed").unwrap();
    assert!(!boxed.required);
    assert!(object_default(boxed).is_some());
}
