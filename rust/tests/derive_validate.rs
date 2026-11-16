//! `#[derive(Validate)]`: the validator-crate workflow on the schemapb
//! engine — typed attributes, serde names, enums, Rust types, defaults.
#![cfg(feature = "derive")]

use std::collections::{BTreeSet, HashMap};
use std::net::IpAddr;
use std::num::NonZeroU32;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use schemapb::{Email, ErrorCode, Reflect, Validate};
use serde::{Deserialize, Serialize};

fn codes(report: &schemapb::Report) -> Vec<(String, ErrorCode)> {
    report.errors().map(|e| (e.path.clone(), e.code)).collect()
}

#[derive(Validate, Serialize, Deserialize, Debug, Clone, PartialEq)]
#[serde(rename_all = "camelCase")]
#[schemapb(rule(
    "root.minReplicas <= root.maxReplicas",
    message = "min above max",
    id = "range"
))]
struct Deployment {
    #[schemapb(min = 1, max = 32, pattern = "^[a-z][a-z0-9-]*$")]
    name: String,
    owner: Email,
    #[schemapb(gte = 1, lte = 64, default = 4)]
    workers: Option<u32>,
    min_replicas: u32,
    max_replicas: u32,
    #[schemapb(min = 1, unique, items(pattern = "^[a-z]+$", max = 16))]
    tags: Vec<String>,
    #[schemapb(keys(pattern = "^[a-z]+$"), values(gte = 0))]
    quotas: HashMap<String, i64>,
    #[schemapb(
        rule("this != 0.0", message = "ratio must not be zero", id = "ratio"),
        finite
    )]
    ratio: f64,
    #[serde(rename = "listen")]
    #[schemapb(desc = "bind address")]
    address: IpAddr,
    #[schemapb(gte = Duration::from_secs(1), lte = Duration::from_secs(3600), default = Duration::from_secs(30))]
    timeout: Option<Duration>,
    #[schemapb(default = Tier::Standard)]
    tier: Option<Tier>,
    #[serde(skip)]
    #[allow(dead_code)]
    cache: Vec<u64>,
}

#[derive(Reflect, Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
enum Tier {
    Standard,
    HighMemory,
}

fn deployment() -> Deployment {
    Deployment {
        name: "api".into(),
        owner: "ops@example.com".into(),
        workers: None,
        min_replicas: 1,
        max_replicas: 3,
        tags: vec!["web".into()],
        quotas: HashMap::from([("cpu".into(), 4)]),
        ratio: 0.5,
        address: "10.0.0.1".parse().unwrap(),
        timeout: None,
        tier: None,
        cache: vec![1, 2],
    }
}

#[test]
fn a_valid_value_passes() {
    assert!(deployment().validate().unwrap().is_valid());
}

#[test]
fn violations_use_the_serde_names() {
    let mut d = deployment();
    d.name = "API!".into();
    d.owner = "not-an-email".into();
    d.workers = Some(0);
    d.min_replicas = 5;
    d.tags = vec!["Web".into(), "Web".into()];
    d.quotas = HashMap::from([("CPU".into(), -1)]);
    d.ratio = 0.0;
    d.timeout = Some(Duration::from_millis(10));

    let report = d.check();
    let got = codes(&report);
    for want in [
        ("name", ErrorCode::PatternMismatch),
        ("owner", ErrorCode::FormatMismatch),
        ("workers", ErrorCode::GteViolated),
        ("tags[0]", ErrorCode::PatternMismatch),
        ("tags[1]", ErrorCode::NotUnique),
        ("quotas.CPU", ErrorCode::GteViolated),
        ("ratio", ErrorCode::RuleViolated),
        ("timeout", ErrorCode::GteViolated),
        // A schema rule with an id reports at its id.
        ("range", ErrorCode::RuleViolated),
    ] {
        assert!(
            got.contains(&(want.0.to_owned(), want.1)),
            "{want:?} missing from {got:?}"
        );
    }
    assert!(report
        .errors()
        .any(|e| e.rule_id.as_deref() == Some("range")));
    assert!(d.validate().is_err());

    let schema = Deployment::validator().schema();
    let listen = schema.lookup_path("listen").unwrap();
    assert_eq!(listen.description.as_deref(), Some("bind address"));
    assert!(
        schema.lookup_path("cache").is_err(),
        "serde(skip) is not in the schema"
    );
    assert!(schema.lookup_path("minReplicas").is_ok());
}

#[test]
fn resolved_fills_defaults_of_absent_fields() {
    let d = deployment().resolved().unwrap();
    assert_eq!(d.workers, Some(4));
    assert_eq!(d.timeout, Some(Duration::from_secs(30)));
    assert_eq!(d.tier, Some(Tier::Standard));
    assert_eq!(
        d.cache,
        Vec::<u64>::new(),
        "skipped fields take their serde default"
    );

    let mut set = deployment();
    set.workers = Some(8);
    set.tier = Some(Tier::HighMemory);
    let kept = set.resolved().unwrap();
    assert_eq!((kept.workers, kept.tier), (Some(8), Some(Tier::HighMemory)));
}

#[test]
fn enums_are_choices_with_their_wire_names() {
    let schema = Deployment::validator().schema();
    let tier = schema.lookup_path("tier").unwrap();
    assert_eq!(tier.kind_name(), "choice");
    let json = serde_json::to_string(&Deployment::validator().schema().fields).unwrap();
    assert!(json.contains("high_memory"), "{json}");
}

/// An internally tagged enum is a `OneOf`: each variant's fields validate.
#[derive(Reflect, Serialize, Deserialize, Debug, PartialEq)]
#[serde(tag = "kind", rename_all = "lowercase")]
enum Storage {
    Disk {
        #[schemapb(gte = 1)]
        size_gb: u32,
    },
    S3 {
        #[schemapb(pattern = "^[a-z0-9.-]+$")]
        bucket: String,
    },
    Memory,
}

#[derive(Validate, Serialize, Deserialize, Debug, PartialEq)]
struct Volume {
    storage: Storage,
    backups: Vec<Storage>,
}

#[test]
fn tagged_enums_validate_their_variant() {
    let ok = Volume {
        storage: Storage::Disk { size_gb: 10 },
        backups: vec![
            Storage::S3 {
                bucket: "b.example".into(),
            },
            Storage::Memory,
        ],
    };
    assert!(ok.validate().is_ok());
    assert_eq!(ok.resolved().unwrap(), ok);

    let bad = Volume {
        storage: Storage::Disk { size_gb: 0 },
        backups: vec![Storage::S3 {
            bucket: "Bad Bucket".into(),
        }],
    };
    let got = codes(&bad.check());
    assert!(
        got.contains(&("storage.size_gb".into(), ErrorCode::GteViolated)),
        "{got:?}"
    );
    assert!(
        got.contains(&("backups[0].bucket".into(), ErrorCode::PatternMismatch)),
        "{got:?}"
    );
}

/// Rust types map to their honest kinds and ranges.
#[derive(Validate, Serialize, Deserialize, Debug, PartialEq)]
struct Types {
    level: u8,
    shift: i8,
    count: NonZeroU32,
    initial: char,
    #[schemapb(max = 4)]
    blob: Vec<u8>,
    magic: [u8; 2],
    ids: BTreeSet<u64>,
    pair: (String, i64),
    at: SystemTime,
    #[schemapb(lte = Duration::from_secs(60))]
    wait: Duration,
    nested: Inner,
    maybe: Option<Inner>,
}

#[derive(Reflect, Serialize, Deserialize, Debug, PartialEq)]
struct Inner {
    #[schemapb(default = "x")]
    label: Option<String>,
}

fn types() -> Types {
    Types {
        level: 200,
        shift: -5,
        count: NonZeroU32::new(3).unwrap(),
        initial: 'a',
        blob: vec![1, 2, 3],
        magic: [0xCA, 0xFE],
        ids: BTreeSet::from([1, 2]),
        pair: ("a".into(), 1),
        at: UNIX_EPOCH + Duration::from_secs(1_700_000_000),
        wait: Duration::from_millis(1500),
        nested: Inner { label: None },
        maybe: None,
    }
}

#[test]
fn rust_types_round_trip_exactly() {
    let t = types();
    assert!(t.validate().is_ok(), "{:?}", t.check());
    let back = t.resolved().unwrap();
    assert_eq!(
        back.nested.label.as_deref(),
        Some("x"),
        "a section resolves its defaults"
    );
    assert_eq!(back.maybe, None);
    assert_eq!(
        Types {
            nested: Inner { label: None },
            ..back
        },
        types()
    );

    let schema = Types::validator().schema();
    for (path, kind) in [
        ("level", "uint32"),
        ("blob", "bytes"),
        ("magic", "bytes"),
        ("ids", "list"),
        ("pair", "list"),
        ("at", "timestamp"),
        ("wait", "duration"),
        ("initial", "string"),
    ] {
        assert_eq!(
            schema.lookup_path(path).unwrap().kind_name(),
            kind,
            "{path}"
        );
    }
}

#[test]
fn values_beyond_the_rust_range_never_validate() {
    let values = types()
        .values()
        .unwrap()
        .with("level", 300_u64)
        .with("count", 0_u64)
        .with("initial", "ab");
    let got = codes(&Types::validator().check(&values));
    for want in [
        ("level", ErrorCode::LteViolated),
        ("count", ErrorCode::GtViolated),
        ("initial", ErrorCode::LenMismatch),
    ] {
        assert!(
            got.contains(&(want.0.to_owned(), want.1)),
            "{want:?} missing from {got:?}"
        );
    }

    let mut over = types();
    over.blob = vec![0; 5];
    over.wait = Duration::from_secs(61);
    let got = codes(&over.check());
    assert!(
        got.contains(&("blob".into(), ErrorCode::MaxLenViolated)),
        "{got:?}"
    );
    assert!(
        got.contains(&("wait".into(), ErrorCode::LteViolated)),
        "{got:?}"
    );
}

/// Each instantiation of a generic type has its own schema.
#[derive(Validate, Serialize, Deserialize)]
struct Page<T> {
    #[schemapb(max = 2)]
    items: Vec<T>,
}

#[test]
fn generic_types_compile_per_instantiation() {
    let strings = Page {
        items: vec!["a".to_owned()],
    };
    let numbers = Page { items: vec![1_i64] };
    assert!(strings.validate().is_ok());
    assert!(numbers.validate().is_ok());
    let kinds = |s: &schemapb::gen::schemapb::Schema| {
        s.lookup_path("items").unwrap().items()[0].kind_name()
    };
    assert_eq!(kinds(Page::<String>::validator().schema()), "string");
    assert_eq!(kinds(Page::<i64>::validator().schema()), "int64");
    assert!(Page {
        items: vec![1_i64; 3]
    }
    .validate()
    .is_err());
}

#[derive(Validate, Serialize, Deserialize)]
#[schemapb(namespace = "tracker", name = "issue", version = "v2.0.0", strict)]
struct Issue {
    #[serde(rename = "type")]
    r#type: String,
}

#[test]
fn identity_and_container_attributes() {
    let id = Deployment::schema_id();
    assert_eq!(
        (id.namespace.as_str(), id.name.as_str()),
        ("schemapb", "Deployment")
    );
    let id = Issue::schema_id();
    assert_eq!(
        (id.namespace.as_str(), id.name.as_str(), id.version.as_str()),
        ("tracker", "issue", "v2.0.0")
    );
    assert!(Issue::validator().schema().strict);
    let values = Issue {
        r#type: "bug".into(),
    }
    .values()
    .unwrap()
    .with("extra", 1_i64);
    let got = codes(&Issue::validator().check(&values));
    assert_eq!(got, vec![("extra".to_owned(), ErrorCode::UnknownField)]);
}

#[derive(Reflect, Serialize, Deserialize, Debug, PartialEq)]
#[serde(transparent)]
struct Slug(String);

#[derive(Validate, Serialize, Deserialize)]
struct WithSlug {
    #[schemapb(pattern = "^[a-z-]+$")]
    slug: String,
    alias: Slug,
}

#[test]
fn transparent_newtypes_take_the_inner_shape() {
    let schema = WithSlug::validator().schema();
    assert_eq!(schema.lookup_path("alias").unwrap().kind_name(), "string");
    let bad = WithSlug {
        slug: "Bad".into(),
        alias: Slug("ok".into()),
    };
    assert_eq!(
        codes(&bad.check()),
        vec![("slug".to_owned(), ErrorCode::PatternMismatch)]
    );
}

#[derive(Validate, Serialize)]
struct BrokenRule {
    #[schemapb(rule("this >", message = "broken"))]
    n: i64,
}

#[test]
#[should_panic(expected = "BrokenRule")]
fn a_schema_that_does_not_compile_names_its_type() {
    let _ = BrokenRule::validator();
}

#[cfg(feature = "uuid")]
#[test]
fn uuid_fields_carry_their_format() {
    #[derive(Validate, Serialize)]
    struct Ref {
        id: uuid::Uuid,
    }
    assert_eq!(
        Ref::validator()
            .schema()
            .lookup_path("id")
            .unwrap()
            .kind_name(),
        "string"
    );
    assert!(Ref {
        id: uuid::Uuid::nil()
    }
    .validate()
    .is_ok());
    let values = schemapb::Values::new().with("id", "nope");
    assert_eq!(
        codes(&Ref::validator().check(&values)),
        vec![("id".to_owned(), ErrorCode::FormatMismatch)]
    );
}

#[cfg(feature = "url")]
#[test]
fn url_fields_carry_their_format() {
    #[derive(Validate, Serialize)]
    struct Link {
        href: url::Url,
    }
    let link = Link {
        href: "https://example.com/a".parse().unwrap(),
    };
    assert!(link.validate().is_ok());
}

#[test]
fn chrono_types_are_timestamps_and_dates() {
    #[derive(Validate, Serialize, Deserialize, Debug, PartialEq)]
    struct When {
        at: chrono::DateTime<chrono::Utc>,
        day: chrono::NaiveDate,
    }
    let w = When {
        at: chrono::DateTime::from_timestamp(1_700_000_000, 0).unwrap(),
        day: chrono::NaiveDate::from_ymd_opt(2026, 11, 16).unwrap(),
    };
    assert!(w.validate().is_ok(), "{:?}", w.check());
    assert_eq!(w.resolved().unwrap(), w);
    let schema = When::validator().schema();
    assert_eq!(schema.lookup_path("at").unwrap().kind_name(), "timestamp");
}
