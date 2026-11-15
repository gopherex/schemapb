//! A bound holds only when its comparison is true: NaN violates every bound,
//! an infinity only the bound on its side; finite rejects NaN and the
//! infinities. NaN and the infinities display in protoJSON spelling.

use schemapb::engine::Engine;
use schemapb::gen::schemapb::{ErrorCode, Schema};
use schemapb::render::display_string;
use schemapb::value::{Native, NativeStruct};

const NAN: f64 = f64::NAN;
const INF: f64 = f64::INFINITY;

fn engine() -> Engine {
    let schema: Schema = serde_json::from_value(serde_json::json!({
        "id": {"namespace": "t", "name": "nonfinite", "version": "v1.0.0"},
        "fields": [
            {"name": "gt", "double": {"gt": 0}},
            {"name": "gte", "double": {"gte": 0}},
            {"name": "lt", "float": {"lt": 10}},
            {"name": "lte", "float": {"lte": 10}},
            {"name": "fin", "double": {"finite": true}},
            {"name": "fin32", "float": {"finite": true, "gte": 1}},
            {"name": "cst", "double": {"const": "Infinity"}},
            {"name": "i", "int64": {"gt": "0", "lte": "3"}}
        ]
    }))
    .unwrap();
    Engine::compile(schema, schemapb::formats::FormatRegistry::new()).unwrap()
}

#[test]
fn float_bounds_and_finite() {
    let e = engine();
    let cases: &[(&str, Native, &[ErrorCode])] = &[
        ("gt", Native::Double(NAN), &[ErrorCode::GtViolated]),
        ("gte", Native::Double(NAN), &[ErrorCode::GteViolated]),
        ("lt", Native::Double(NAN), &[ErrorCode::LtViolated]),
        ("lte", Native::Double(NAN), &[ErrorCode::LteViolated]),
        ("gte", Native::Double(INF), &[]),
        ("gte", Native::Double(-INF), &[ErrorCode::GteViolated]),
        ("lte", Native::Double(INF), &[ErrorCode::LteViolated]),
        ("gte", Native::Double(0.0), &[]),
        ("fin", Native::Double(1e308), &[]),
        ("fin", Native::Double(NAN), &[ErrorCode::NotFinite]),
        ("fin", Native::Double(INF), &[ErrorCode::NotFinite]),
        ("fin", Native::Double(-INF), &[ErrorCode::NotFinite]),
        (
            "fin32",
            Native::Double(NAN),
            &[ErrorCode::NotFinite, ErrorCode::GteViolated],
        ),
        ("cst", Native::Double(INF), &[]),
        ("cst", Native::Double(NAN), &[ErrorCode::ConstMismatch]),
        ("i", Native::Int(0), &[ErrorCode::GtViolated]),
        ("i", Native::Int(3), &[]),
        ("i", Native::Int(4), &[ErrorCode::LteViolated]),
    ];
    for (path, val, want) in cases {
        let mut values = NativeStruct::from([((*path).to_owned(), val.clone())]);
        let got: Vec<i32> = e
            .validate(&mut values)
            .errors
            .iter()
            .filter(|x| x.path == *path)
            .map(|x| x.code)
            .collect();
        let want: Vec<i32> = want.iter().map(|c| *c as i32).collect();
        assert_eq!(got, want, "{path} = {val:?}");
    }
}

#[test]
fn non_finite_display() {
    let cases = [
        (Native::Double(NAN), "NaN"),
        (Native::Double(INF), "Infinity"),
        (Native::Double(-INF), "-Infinity"),
        (Native::Double(1.5), "1.5"),
        (
            Native::List(vec![Native::Double(-INF), Native::Double(2.0)]),
            r#"["-Infinity",2]"#,
        ),
    ];
    for (v, want) in cases {
        assert_eq!(display_string(&v), want, "{v:?}");
    }
}
