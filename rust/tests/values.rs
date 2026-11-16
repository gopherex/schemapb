//! A tracker's custom fields: a schema built in code, values kept in an
//! application struct and stored as plain JSON, validated and resolved.

use std::time::{Duration, SystemTime, UNIX_EPOCH};

use schemapb::build::*;
use schemapb::value::Native;
use schemapb::{ErrorCode, Severity, Validator, ValueError, Values};

fn tracker() -> Validator {
    schema("tracker", "project-42")
        .field(string("title").min_len(1).required())
        .field(int64("estimate").gte(0).unit("h"))
        .field(uint64("big"))
        .field(choice("status").str_opts(["open", "done"]).default("open"))
        .field(duration("spent").default(Duration::ZERO))
        .field(timestamp("due"))
        .field(bytes("attachment").max_len(16))
        .field(double("ratio").finite())
        .field(list("labels", string("label").pattern("^[a-z]+$")).unique())
        .field(computed("over", "root.estimate > 40").result(ResultType::Bool))
        .rule(rule("root.estimate < 1000", "estimate too large").id("estimate-cap"))
        .rule(
            rule(
                "root.status != \"done\" || root.estimate > 0",
                "done without estimate",
            )
            .warn(),
        )
        .build()
        .unwrap()
}

/// A struct an application stores in a table, custom fields in `jsonb`.
#[derive(serde::Serialize, serde::Deserialize, PartialEq, Debug)]
struct Issue {
    id: u64,
    custom: Values,
}

#[test]
fn resolve_completes_and_types_the_values() {
    let v = tracker();
    let mut values = Values::new()
        .with("title", "crash on start")
        .with("estimate", 8_i64)
        .with("labels", vec!["bug".to_owned()]);

    let report = v.resolve(&mut values).unwrap();
    assert!(report.is_valid());
    assert_eq!(
        values.get::<String>("status").unwrap().as_deref(),
        Some("open")
    );
    assert_eq!(
        values.get::<Duration>("spent").unwrap(),
        Some(Duration::ZERO)
    );
    assert_eq!(values.get::<bool>("over").unwrap(), Some(false));
    assert_eq!(
        values.get::<Vec<String>>("labels").unwrap(),
        Some(vec!["bug".to_owned()])
    );
    assert_eq!(values.get::<i64>("missing").unwrap(), None);
}

#[test]
fn violations_are_plain_data() {
    let v = tracker();
    let values = Values::new()
        .with("estimate", -1_i64)
        .with("status", "lost")
        .with("labels", vec!["Bug".to_owned(), "Bug".to_owned()])
        .with("ratio", f64::NAN);

    let report = v.check(&values);
    let codes: Vec<(&str, ErrorCode)> =
        report.errors().map(|e| (e.path.as_str(), e.code)).collect();
    for want in [
        ("title", ErrorCode::RequiredMissing),
        ("estimate", ErrorCode::GteViolated),
        ("status", ErrorCode::ChoiceNotAllowed),
        ("labels[1]", ErrorCode::NotUnique),
        ("labels[0]", ErrorCode::PatternMismatch),
        ("ratio", ErrorCode::NotFinite),
    ] {
        assert!(codes.contains(&want), "{want:?} missing from {codes:?}");
    }

    let gte = report.at("estimate").next().unwrap();
    assert_eq!(gte.constraint, "gte");
    assert_eq!(gte.expected, Some(Native::Int(0)));
    assert_eq!(gte.actual, Some(Native::Int(-1)));

    let err = v.validate(&values).unwrap_err();
    assert!(
        err.to_string()
            .starts_with("invalid values; title: required"),
        "{err}"
    );
}

#[test]
fn rules_and_warnings() {
    let v = tracker();
    let values = Values::new()
        .with("title", "x")
        .with("status", "done")
        .with("estimate", 0_i64);
    let report = v.validate(&values).unwrap();
    let warning = report.warnings().next().unwrap();
    assert_eq!(warning.severity, Severity::Warning);
    assert_eq!(warning.message, "done without estimate");

    let capped = Values::new().with("title", "x").with("estimate", 5000_i64);
    let err = v.validate(&capped).unwrap_err();
    assert!(
        err.report
            .errors()
            .any(|e| e.rule_id.as_deref() == Some("estimate-cap")
                && e.code == ErrorCode::RuleViolated)
    );
}

#[test]
fn invalid_values_stay_untouched() {
    let v = tracker();
    let mut values = Values::new().with("estimate", -1_i64);
    let before = values.clone();
    assert!(v.resolve(&mut values).is_err());
    assert_eq!(values, before);
}

#[test]
fn plain_json_round_trips_through_storage() {
    let v = tracker();
    let due = UNIX_EPOCH + Duration::from_hours(500_000);
    let mut custom = Values::new()
        .with("title", "store me")
        .with("estimate", 3_i64)
        .with("big", u64::MAX)
        .with("spent", Duration::from_mins(90))
        .with("due", due)
        .with("attachment", vec![1_u8, 2, 3])
        .with("ratio", 0.5);
    v.resolve(&mut custom).unwrap();

    let issue = Issue { id: 7, custom };
    let json = serde_json::to_string(&issue).unwrap();
    for part in [
        r#""spent":"1h30m0s""#,
        r#""due":"2027-01-15T08:00:00Z""#,
        r#""attachment":"AQID""#,
        &format!(r#""big":{}"#, u64::MAX),
        r#""estimate":3"#,
    ] {
        assert!(json.contains(part), "{part} not in {json}");
    }

    let mut back: Issue = serde_json::from_str(&json).unwrap();
    // Plain JSON carries those kinds as strings; resolving restores them.
    assert_eq!(
        back.custom.raw("spent"),
        Some(&Native::Str("1h30m0s".to_owned()))
    );
    v.resolve(&mut back.custom).unwrap();
    assert_eq!(back, issue);
    assert_eq!(back.custom.get::<SystemTime>("due").unwrap(), Some(due));
    assert_eq!(back.custom.get::<u64>("big").unwrap(), Some(u64::MAX));
    assert_eq!(
        back.custom.get::<Vec<u8>>("attachment").unwrap(),
        Some(vec![1, 2, 3])
    );
}

#[test]
fn non_finite_doubles_round_trip() {
    let values = Values::new().with("ratio", f64::NEG_INFINITY);
    let json = serde_json::to_string(&values).unwrap();
    assert_eq!(json, r#"{"ratio":"-Infinity"}"#);
    let schema = schema("t", "ratio").field(double("ratio")).build().unwrap();
    let mut back: Values = serde_json::from_str(&json).unwrap();
    schema.resolve(&mut back).unwrap();
    assert_eq!(back.get::<f64>("ratio").unwrap(), Some(f64::NEG_INFINITY));
}

#[test]
fn schema_is_stored_and_loaded_as_json() {
    let v = tracker();
    let loaded = Validator::from_json(&v.schema_json()).unwrap();
    assert_eq!(loaded.schema(), v.schema());
    let values = Values::new().with("title", "");
    assert_eq!(loaded.check(&values), v.check(&values));
    assert!(Validator::from_json("{").is_err());
    assert!(Validator::from_json(r#"{"fields":[{"name":"x","int64":{},"string":{}}]}"#).is_err());
}

#[test]
fn typed_reads_never_truncate() {
    let values = Values::new()
        .with("big", u64::MAX)
        .with("list", vec![1_i64, -2])
        .with("text", "x");
    let err: ValueError = values.get::<i64>("big").unwrap_err();
    assert_eq!(err.to_string(), "big: expected i64, found uint");
    assert_eq!(values.get::<Vec<u32>>("list").unwrap_err().key, "list[1]");
    assert_eq!(values.get::<bool>("text").unwrap_err().found, "string");
    assert!(values.get::<Duration>("text").is_err());
}

#[test]
fn deserialize_rejects_a_non_object() {
    assert!(serde_json::from_str::<Values>("[1]").is_err());
    let parsed: Values = serde_json::from_str(r#"{"n":null,"a":[{"k":1.5}]}"#).unwrap();
    assert_eq!(parsed.raw("n"), Some(&Native::Null));
    assert_eq!(
        parsed.get::<Vec<Values>>("a").unwrap().unwrap()[0]
            .get::<f64>("k")
            .unwrap(),
        Some(1.5)
    );
}
