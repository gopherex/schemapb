//! The conformance runner: identical results to the Go reference goldens.

use std::collections::BTreeMap;

use schemapb::bake::{bake, render_baked};
use schemapb::engine::{compile, Engine};
use schemapb::gen::schemapb::{ErrorCode, Schema, StructValue, ValidationResult};
use schemapb::messages::template;
use schemapb::validate::validate;
use schemapb::value::{Native, NativeStruct};

fn golden(name: &str) -> String {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../conformance/golden")
        .join(name);
    std::fs::read_to_string(path).unwrap()
}

fn golden_engine() -> Engine {
    let schema: Schema = serde_json::from_str(&golden("full-schema.json")).unwrap();
    let mut formats = schemapb::formats::FormatRegistry::new();
    formats.insert("x.nonempty".into(), Box::new(|v: &str| !v.is_empty()));
    compile(schema, formats).unwrap()
}

fn s(v: &str) -> Native {
    Native::Str(v.to_owned())
}

fn obj(entries: Vec<(&str, Native)>) -> Native {
    Native::Struct(
        entries
            .into_iter()
            .map(|(k, v)| (k.to_owned(), v))
            .collect::<BTreeMap<_, _>>(),
    )
}

/// Mirrors `go/schemapb/golden_test.go` validInput exactly.
fn valid_input() -> NativeStruct {
    let mut m = NativeStruct::new();
    m.insert("i64".into(), s("256")); // coerced
    m.insert("mail".into(), s("dba@corp.io"));
    m.insert("token".into(), s("s3cret-token"));
    m.insert("magic".into(), Native::Bytes(vec![0xde, 0xad]));
    m.insert("replica_count".into(), Native::Int(1));
    m.insert(
        "replicas".into(),
        Native::List(vec![obj(vec![("name", s("r1"))])]),
    );
    m.insert(
        "tablespaces".into(),
        obj(vec![("main", obj(vec![("location", s("/var/lib/ts"))]))]),
    );
    m.insert(
        "backup".into(),
        obj(vec![("type", s("s3")), ("bucket", s("backups"))]),
    );
    m.insert("data_volume".into(), obj(vec![("path", s("/data"))]));
    m.insert("region".into(), s("somewhere-else"));
    m.insert(
        "endpoint_pair".into(),
        Native::List(vec![s("db1"), Native::Int(5432)]),
    );
    m
}

/// Mirrors `go/schemapb/golden_test.go` brokenInput exactly.
fn broken_input() -> NativeStruct {
    let mut m = NativeStruct::new();
    m.insert("f32".into(), Native::Double(0.25));
    m.insert("f64".into(), Native::Double(2.0));
    m.insert("i32".into(), Native::Int(5));
    m.insert("i64".into(), Native::Int(8));
    m.insert("u32".into(), Native::Uint(3));
    m.insert("u64".into(), Native::Uint(0));
    m.insert("pinned".into(), Native::Bool(false));
    m.insert("name".into(), s("Bad Name!"));
    m.insert("mode".into(), s("legacy"));
    m.insert("exact".into(), s("abcde"));
    m.insert("mail".into(), s("not-an-email"));
    m.insert("token".into(), s("short"));
    m.insert("license".into(), Native::Bytes(b"XX".to_vec()));
    m.insert("magic".into(), Native::Bytes(vec![0x00]));
    m.insert("wal_level".into(), s("extreme"));
    m.insert("cpu".into(), Native::Int(3));
    m.insert("timeout".into(), s("3h"));
    m.insert("not_before".into(), s("2020-01-01T00:00:00Z"));
    m.insert("replica_count".into(), Native::Int(2));
    m.insert(
        "replicas".into(),
        Native::List(vec![
            obj(vec![("name", s("r1"))]),
            obj(vec![("name", s("r1"))]),
            obj(vec![("weight", Native::Int(2))]),
        ]),
    );
    m.insert(
        "logging".into(),
        obj(vec![
            ("collector", Native::Bool(true)),
            ("junk", Native::Int(1)),
        ]),
    );
    m.insert("tablespaces".into(), obj(vec![("bad", obj(vec![]))]));
    m.insert("backup".into(), obj(vec![("type", s("tape"))]));
    m.insert(
        "data_volume".into(),
        obj(vec![("path", s("/data")), ("size_gb", Native::Int(0))]),
    );
    m.insert("garbage".into(), Native::Int(1));
    m.insert(
        "endpoint_pair".into(),
        Native::List(vec![s(""), s("not-a-port")]),
    );
    m
}

#[test]
fn bakes_valid_input() {
    let e = golden_engine();
    let mut values = valid_input();
    let outcome = bake(&e, &mut values);
    let hard: Vec<_> = outcome
        .result
        .errors
        .iter()
        .filter(|x| x.code != i32::from(ErrorCode::RuleViolated as u8))
        .collect();
    assert!(hard.is_empty(), "unexpected errors: {hard:#?}");
    let want: StructValue = serde_json::from_str(&golden("full-baked.json")).unwrap();
    let got = outcome.baked.expect("baked").values.expect("values");
    let mut got_keys: Vec<_> = got.fields.keys().collect();
    let mut want_keys: Vec<_> = want.fields.keys().collect();
    got_keys.sort();
    want_keys.sort();
    assert_eq!(got_keys, want_keys);
    for (name, wv) in &want.fields {
        assert_eq!(got.fields.get(name), Some(wv), "field {name}");
    }
}

#[test]
fn broken_input_errors() {
    let e = golden_engine();
    let mut values = broken_input();
    let got = validate(&e, &mut values);
    let want: ValidationResult = serde_json::from_str(&golden("full-errors.json")).unwrap();
    let key = |x: &schemapb::gen::schemapb::ValidationError| format!("{}:{}", x.path, x.code);
    assert_eq!(
        got.errors.iter().map(key).collect::<Vec<_>>(),
        want.errors.iter().map(key).collect::<Vec<_>>(),
    );
    for (g, w) in got.errors.iter().zip(want.errors.iter()) {
        assert_eq!(g, w, "at {}:{}", w.path, w.code);
    }
}

#[test]
fn rendered() {
    let e = golden_engine();
    let mut values = valid_input();
    let outcome = bake(&e, &mut values);
    let baked = outcome.baked.expect("baked");
    let conf = render_baked(&e, &baked, "conf").expect("conf");
    let report = render_baked(&e, &baked, "report").expect("report");
    assert_eq!(format!("{conf}---\n{report}"), golden("full-rendered.txt"));
}

#[test]
fn message_templates() {
    let want: std::collections::BTreeMap<String, String> =
        serde_json::from_str(&golden("messages.json")).unwrap();
    for (name, tpl) in &want {
        let code_name = name.strip_prefix("ERROR_CODE_").unwrap();
        let code = (0..=90)
            .filter_map(|n| ErrorCode::try_from(n).ok())
            .find(|c| {
                format!("{c:?}")
                    .chars()
                    .flat_map(char::to_uppercase)
                    .collect::<String>()
                    == code_name.replace('_', "")
            })
            .unwrap_or_else(|| panic!("unknown code {name}"));
        assert_eq!(template(code), Some(tpl.as_str()), "{name}");
    }
}
