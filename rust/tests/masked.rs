use schemapb::gen::schemapb::{Baked, StructValue};

#[test]
fn masked_conformance() {
    let path =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../conformance/golden/masked.json");
    let cases: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(path).unwrap()).unwrap();
    for case in cases.as_array().unwrap() {
        let baked: Baked = serde_json::from_value(case["baked"].clone()).unwrap();
        let before = baked.clone();
        let want: StructValue = serde_json::from_value(case["masked"].clone()).unwrap();
        let mut out = baked.masked();
        assert_eq!(out, want, "{}", case["name"]);
        assert_eq!(baked, before);
        out.fields.clear();
        assert_eq!(baked, before);
        assert_eq!(baked.masked(), want);
    }
}
