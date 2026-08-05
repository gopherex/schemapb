//! The conformance runner: identical results to the Go reference goldens.

use std::collections::BTreeMap;

use schemapb::engine::Engine;
use schemapb::gen::schemapb::{ErrorCode, Schema, StructValue, ValidationResult};
use schemapb::messages::template;
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
    Engine::compile(schema, formats).unwrap()
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
    let outcome = e.bake(&mut values);
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
    let got = e.validate(&mut values);
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
    let outcome = e.bake(&mut values);
    let baked = outcome.baked.expect("baked");
    let conf = e.render_baked(&baked, "conf").expect("conf");
    let report = e.render_baked(&baked, "report").expect("report");
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

/// Lookup conformance: every case in lookup.json must resolve to the same
/// kind, or fail with the same (at, segment, reason) triple, as Go.
#[test]
fn lookup_cases() {
    let schema: Schema = serde_json::from_str(&golden("full-schema.json")).unwrap();
    let doc: serde_json::Value = serde_json::from_str(&golden("lookup.json")).unwrap();

    for case in doc["cases"].as_array().unwrap() {
        let path = case["path"].as_str().unwrap();
        match schema.lookup_path(path) {
            Ok(f) => {
                assert!(
                    case["error"].is_null(),
                    "lookup {path:?}: resolved, want error"
                );
                assert_eq!(
                    f.kind_name(),
                    case["kind"].as_str().unwrap(),
                    "lookup {path:?}: kind"
                );
            }
            Err(e) => {
                let want = &case["error"];
                assert!(
                    !want.is_null(),
                    "lookup {path:?}: failed with {e}, want kind"
                );
                assert_eq!(e.at, want["at"].as_str().unwrap(), "lookup {path:?}: at");
                assert_eq!(
                    e.segment,
                    want["segment"].as_str().unwrap(),
                    "lookup {path:?}: segment"
                );
                assert_eq!(
                    e.reason.as_str(),
                    want["reason"].as_str().unwrap(),
                    "lookup {path:?}: reason"
                );
            }
        }
    }
}

/// Field names must be identifiers and not CEL reserved words.
#[test]
fn field_name_rules() {
    use schemapb::gen::schemapb::schema::field::{Kind, String as StringKind};
    use schemapb::gen::schemapb::{schema::Field, SchemaIdentity};

    let with_name = |name: &str| Schema {
        id: Some(SchemaIdentity {
            namespace: "t".into(),
            name: "names".into(),
            ..Default::default()
        }),
        fields: vec![Field {
            name: name.into(),
            kind: Some(Kind::String(<StringKind as Default>::default())),
            ..Default::default()
        }],
        ..Default::default()
    };

    for bad in ["a.b", "my-field", "1st", "in", "true", "while"] {
        assert!(
            !schemapb::descriptor::check_descriptor(&with_name(bad)).is_empty(),
            "field name {bad:?} accepted, want descriptor error"
        );
    }

    for good in ["snake_case", "camelCase", "_x", "a1"] {
        assert!(
            schemapb::descriptor::check_descriptor(&with_name(good)).is_empty(),
            "field name {good:?} rejected"
        );
    }
}

/// Value-as conformance: every case in value-as.json must convert (and
/// re-encode as the target's wire kind) or refuse exactly as Go does.
#[test]
fn value_as_cases() {
    use schemapb::gen::schemapb::Value;
    use schemapb::value::{
        bool_v, bytes_v, double_v, duration_v, float_v, int32_v, int64_v, list_v, str_v, struct_v,
        timestamp_v, uint32_v, uint64_v,
    };

    let re_encode = |v: &Value, target: &str| -> Option<Value> {
        match target {
            "bool" => v.get::<bool>().map(bool_v),
            "int32" => v.get::<i32>().map(int32_v),
            "int64" => v.get::<i64>().map(int64_v),
            "uint32" => v.get::<u32>().map(uint32_v),
            "uint64" => v.get::<u64>().map(uint64_v),
            "float" => v.get::<f32>().map(float_v),
            "double" => v.get::<f64>().map(double_v),
            "string" => v.get::<&str>().map(str_v),
            "bytes" => v.get::<Vec<u8>>().map(bytes_v),
            "duration" => v.get::<pbjson_types::Duration>().map(duration_v),
            "timestamp" => v.get::<pbjson_types::Timestamp>().map(timestamp_v),
            "list" => v.as_list().map(|items| list_v(items.to_vec())),
            "struct" => v.as_struct().map(|fields| struct_v(fields.clone())),
            _ => None,
        }
    };

    let doc: serde_json::Value = serde_json::from_str(&golden("value-as.json")).unwrap();

    for case in doc["cases"].as_array().unwrap() {
        let v: Value = serde_json::from_value(case["value"].clone()).unwrap();
        let target = case["target"].as_str().unwrap();
        let got = re_encode(&v, target);

        match got {
            None => assert!(
                case["result"].is_null(),
                "as {target} <- {}: refused, want {}",
                case["value"],
                case["result"]
            ),
            Some(res) => {
                assert!(
                    !case["result"].is_null(),
                    "as {target} <- {}: got value, want refusal",
                    case["value"]
                );
                let want: Value = serde_json::from_value(case["result"].clone()).unwrap();
                assert_eq!(res, want, "as {target} <- {}", case["value"]);
            }
        }
    }
}

/// Value-lookup conformance over the baked kitchen-sink values.
#[test]
fn value_lookup_cases() {
    use schemapb::gen::schemapb::Value;

    let values: StructValue = serde_json::from_str(&golden("full-baked.json")).unwrap();
    let doc: serde_json::Value = serde_json::from_str(&golden("value-lookup.json")).unwrap();

    for case in doc["cases"].as_array().unwrap() {
        let path = case["path"].as_str().unwrap();
        match values.lookup(path) {
            Ok(v) => {
                assert!(
                    case["error"].is_null(),
                    "lookup {path:?}: resolved, want error"
                );
                let want: Value = serde_json::from_value(case["value"].clone()).unwrap();
                assert_eq!(*v, want, "lookup {path:?}");
            }
            Err(e) => {
                let want = &case["error"];
                assert!(
                    !want.is_null(),
                    "lookup {path:?}: failed with {e}, want value"
                );
                assert_eq!(e.at, want["at"].as_str().unwrap(), "lookup {path:?}: at");
                assert_eq!(
                    e.segment,
                    want["segment"].as_str().unwrap(),
                    "lookup {path:?}: segment"
                );
                assert_eq!(
                    e.reason.as_str(),
                    want["reason"].as_str().unwrap(),
                    "lookup {path:?}: reason"
                );
            }
        }
    }
}
