//! serde between Rust values and the native model: exact kinds, absence,
//! schema fit and decode hints.

use std::collections::BTreeMap;
use std::time::{Duration, UNIX_EPOCH};

use schemapb::build::*;
use schemapb::native::{decode, encode, fit};
use schemapb::value::{Native, NativeStruct};
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Debug, PartialEq)]
struct Sample {
    big: u64,
    small: i8,
    none: Option<i64>,
    some: Option<i64>,
    bytes: Vec<u8>,
    by_id: BTreeMap<u32, String>,
    wait: Duration,
    kind: Kind,
}

#[derive(Serialize, Deserialize, Debug, PartialEq)]
enum Kind {
    Plain,
    Sized(u32),
}

fn sample() -> Sample {
    Sample {
        big: u64::MAX,
        small: -3,
        none: None,
        some: Some(7),
        bytes: vec![1, 2],
        by_id: BTreeMap::from([(5, "five".to_owned())]),
        wait: Duration::from_millis(1500),
        kind: Kind::Sized(9),
    }
}

#[test]
fn encode_keeps_kinds_and_leaves_none_out() {
    let Native::Struct(m) = encode(&sample()).unwrap() else {
        panic!("a struct encodes as an object");
    };
    assert_eq!(m["big"], Native::Uint(u64::MAX));
    assert_eq!(m["small"], Native::Int(-3));
    assert!(
        !m.contains_key("none"),
        "None is absent, so a default can apply"
    );
    assert_eq!(m["some"], Native::Int(7));
    assert_eq!(
        m["by_id"],
        Native::Struct(NativeStruct::from([(
            "5".to_owned(),
            Native::Str("five".to_owned())
        )]))
    );
    assert_eq!(
        m["kind"],
        Native::Struct(NativeStruct::from([("Sized".to_owned(), Native::Uint(9))]))
    );
}

#[test]
fn fit_reshapes_what_serde_cannot_express() {
    let schema = schema("t", "fit")
        .field(bytes("bytes"))
        .field(duration("wait"))
        .field(timestamp("at"))
        .field(list("blobs", bytes("blob")))
        .done();
    let mut m = NativeStruct::new();
    m.insert("bytes".into(), encode(&vec![1_u8, 2]).unwrap());
    m.insert("wait".into(), encode(&Duration::from_millis(1500)).unwrap());
    m.insert(
        "at".into(),
        encode(&(UNIX_EPOCH + Duration::from_secs(10))).unwrap(),
    );
    m.insert("blobs".into(), encode(&vec![vec![3_u8]]).unwrap());
    fit(&schema, &mut m);
    assert_eq!(m["bytes"], Native::Bytes(vec![1, 2]));
    assert_eq!(
        m["wait"],
        Native::Duration(pbjson_types::Duration {
            seconds: 1,
            nanos: 500_000_000
        })
    );
    assert_eq!(
        m["at"],
        Native::Timestamp(pbjson_types::Timestamp {
            seconds: 10,
            nanos: 0
        })
    );
    assert_eq!(m["blobs"], Native::List(vec![Native::Bytes(vec![3])]));
}

#[test]
fn decode_round_trips_and_follows_type_hints() {
    let schema = schema("t", "sample")
        .field(bytes("bytes"))
        .field(duration("wait"))
        .done();
    let Native::Struct(mut m) = encode(&sample()).unwrap() else {
        unreachable!()
    };
    fit(&schema, &mut m);
    assert_eq!(decode::<Sample>(Native::Struct(m)).unwrap(), sample());

    // A proto Duration reads its protoJSON form; std's reads its struct.
    let d = Native::Duration(pbjson_types::Duration {
        seconds: 5400,
        nanos: 0,
    });
    assert_eq!(
        decode::<Duration>(d.clone()).unwrap(),
        Duration::from_mins(90)
    );
    assert_eq!(
        decode::<pbjson_types::Duration>(d).unwrap(),
        pbjson_types::Duration {
            seconds: 5400,
            nanos: 0
        }
    );
}

#[test]
fn decode_refuses_what_the_rust_type_cannot_hold() {
    let negative = Native::Duration(pbjson_types::Duration {
        seconds: -1,
        nanos: 0,
    });
    assert!(decode::<Duration>(negative)
        .unwrap_err()
        .0
        .contains("negative"));
    assert!(decode::<u8>(Native::Uint(300)).is_err());
    assert!(decode::<Kind>(Native::Str("Missing".into())).is_err());
}
