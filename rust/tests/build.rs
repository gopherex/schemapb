//! The builder mirrors the Go one: the kitchen-sink schema written with it
//! equals the Go-built golden (conformance/golden/full-schema.json).

use std::time::Duration;

use schemapb::build::*;
use schemapb::duration::parse_rfc3339;
use schemapb::gen::schemapb::Schema;
use schemapb::typed::Version;
use schemapb::Values;

fn golden(name: &str) -> String {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../conformance/golden")
        .join(name);
    std::fs::read_to_string(path).unwrap()
}

fn at(rfc3339: &str) -> pbjson_types::Timestamp {
    parse_rfc3339(rfc3339).unwrap()
}

#[allow(clippy::too_many_lines)]
fn kitchen_sink() -> Schema {
    // The Go builder leaves coercion off; this one turns it on by default.
    let endpoint = schema("conformance", "endpoint")
        .version(&Version::of(2, 1, 0))
        .coerce(false)
        .field(string("host").required())
        .field(int64("port").default(5432))
        .done();
    let endpoint_id = endpoint.id.clone().unwrap();

    schema("conformance", "kitchen_sink")
        .version(&Version::of(1, 0, 0))
        .desc("conformance kitchen sink: every kind, every attribute")
        .strict()
        .coerce(true)
        .min_props(1)
        .max_props(64)
        .def(
            "volume",
            [
                string("path").required().into(),
                int64("size_gb").default(10).gt(0).into(),
            ],
        )
        .def_schema("endpoint", endpoint.clone())
        .field(float("f32").default(0.5).gt(0.0).lt(1.0).forbidden([0.25]).finite())
        .field(double("f64").equals(1.5).default(1.5).multiple_of(0.5).gte(0.0).finite())
        .field(int32("i32").default(8).gte(0).lte(64).multiple_of(2).allowed([2, 4, 8, 16, 32, 64]))
        .field(
            int64("i64")
                .default(128)
                .gte(16)
                .lte(1 << 40)
                .unit("MB")
                .group("numbers")
                .title("Big number")
                .desc("an int64 with everything"),
        )
        .field(uint32("u32").allowed([1, 2, 4]).default(2))
        .field(uint64("u64").forbidden([0]).default(1))
        .field(bool("flag").default(true).group("flags"))
        .field(bool("pinned").equals(true).default(true))
        .field(
            string("name")
                .default("main")
                .min_len(1)
                .max_len(32)
                .pattern("^[a-z][a-z0-9-]*$")
                .example("main")
                .example("analytics"),
        )
        .field(
            string("mode")
                .allowed(["fast", "slow"])
                .forbidden(["legacy"])
                .default("Fast")
                .normalize("this.lowerAscii()"),
        )
        .field(string("exact").len(4).default("abcd"))
        .field(string("mail").format("email").required())
        .field(string("token").min_len(8).secret().required().deprecated())
        .field(
            bytes("license")
                .min_len(4)
                .max_len(64)
                .prefix(b"LIC-")
                .suffix(b"-END")
                .default(b"LIC-x-END"),
        )
        .field(
            bytes("magic")
                .equals(&[0xDE, 0xAD])
                .allowed([&[0xDE, 0xAD][..], &[0xBE, 0xEF][..]])
                .forbidden([&[0x00][..]]),
        )
        .field(
            choice("wal_level")
                .opt_described("minimal", "Minimal", "bare minimum WAL", true)
                .opt("replica", "Replica")
                .opt("logical", "Logical")
                .default("replica")
                .group("choices"),
        )
        .field(choice("cpu").int_opts([2, 4, 8]).default(4_i64))
        .field(choice("region").str_opts(["eu", "us"]).open())
        .field(choice("compression").options_expr(
            r#"root.wal_level == "logical" ? ["lz4", "zstd"] : ["off"]"#,
        ))
        .field(
            duration("timeout")
                .default(Duration::from_secs(300))
                .gt(Duration::ZERO)
                .gte(Duration::from_secs(1))
                .lt(Duration::from_secs(7200))
                .lte(Duration::from_secs(3600)),
        )
        .field(
            timestamp("not_before")
                .gt(at("2025-12-31T23:59:59Z"))
                .gte(at("2026-01-01T00:00:00Z"))
                .lt(at("2031-01-01T00:00:00Z"))
                .lte(at("2030-12-31T23:59:59Z"))
                .default(at("2026-06-01T12:00:00Z")),
        )
        .field(tuple(
            "endpoint_pair",
            [
                string("host").min_len(1).into(),
                int64("port").gte(1).lte(65535).into(),
            ],
        ))
        .field(int64("replica_count").default(2).gte(0).lte(10))
        .field(
            list(
                "replicas",
                object("")
                    .field(string("name").required())
                    .field(int64("weight").default(1))
                    .rule(rule("true", "always fine")),
            )
            .min_items(0)
            .max_items(10)
            .unique()
            .count("int(root.replica_count)"),
        )
        .field(
            object("logging")
                .field(bool("collector").default(false))
                .field(string("directory").default("log"))
                .strict()
                .min_props(0)
                .max_props(8)
                .object_rule(rule(r#"root.logging.directory != """#, "dir required").id("log-dir")),
        )
        .field(
            map("tablespaces")
                .field(string("location").required())
                .field(duration("ttl"))
                .strict()
                .min_entries(0)
                .max_entries(16)
                .value_rule(rule("true", "map value rule")),
        )
        .field(
            map_of("limits", int64("value").gte(0))
                .key(string("key").pattern("^[a-z]+$"))
                .min_entries(0)
                .max_entries(8),
        )
        .field(
            map_of("ports", string("value").min_len(1))
                .key(int32("key").gt(0).rule(rule("this != 22", "ssh port").id("no-ssh"))),
        )
        .field(computed("label_keys", "root.labels.map(k, k).sort()"))
        .field(computed("flag_text", "string(root.flag)"))
        .field(map_of(
            "labels",
            string("value")
                .pattern("^[a-z]+$")
                .rule(rule(r#"this != "forbidden""#, "label value forbidden")),
        ))
        .field(
            oneof("backup", "type")
                .variant("s3", [string("bucket").required().into()])
                .variant_schema("endpoint", endpoint),
        )
        .field(ref_def("data_volume", "volume"))
        .field(ref_id("primary_endpoint", endpoint_id))
        .field(
            computed("cache", "root.i64 * 3")
                .result(ResultType::Int64)
                .group("numbers"),
        )
        .field(json("annotations").default(Values::new().with("team", "storage").with("tier", 1_i64)))
        .field(
            int64("debug_level")
                .when(r#"root.mode == "slow""#)
                .default(1)
                .immutable()
                .nullable(),
        )
        .rule(rule("int(root.i64) >= int(root.i32)", "i64 must cover i32").id("cross"))
        .rule(rule("int(root.replica_count) < 8", "many replicas").warn())
        .rule(rule("true", "explicit severity").error())
        .rule(rule("this == root", "form rules bind this to the form").id("this-form"))
        .required_when("annotations", r#"root.mode == "slow""#)
        .required_unless("mail", "false")
        .forbidden_when("u64", r#"root.mode == "legacy-off""#)
        .template(
            "conf",
            "name = {{values.name}}\nwal_level = {{values.wal_level}}\ncache = {{values.cache}}\n",
        )
        .template(
            "report",
            "{{#groups}}[{{name}}]\n{{#fields}}{{#set}}{{name}}={{{value}}}{{#label}} ({{label}}){{/label}}\n{{/set}}{{/fields}}{{/groups}}",
        )
        .done()
}

#[test]
fn builder_matches_the_go_kitchen_sink() {
    let want: Schema = serde_json::from_str(&golden("full-schema.json")).unwrap();
    let got = kitchen_sink();
    if got != want {
        let got = serde_json::to_value(&got).unwrap();
        let want = serde_json::to_value(&want).unwrap();
        assert_eq!(
            serde_json::to_string_pretty(&got).unwrap(),
            serde_json::to_string_pretty(&want).unwrap()
        );
    }
}

#[test]
fn builder_schema_compiles() {
    let mut formats = schemapb::formats::FormatRegistry::new();
    formats.insert("x.nonempty".to_owned(), Box::new(|v: &str| !v.is_empty()));
    schemapb::Validator::with_formats(kitchen_sink(), formats).unwrap();
}
