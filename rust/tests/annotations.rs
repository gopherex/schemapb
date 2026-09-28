use prost::Message;
use schemapb::engine::Engine;
use schemapb::gen::schemapb::{Baked, ResolveReport, Schema, StructValue};
use schemapb::value::struct_to_native;

#[test]
fn annotations_conformance() {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../conformance/golden/annotations.json");
    let doc: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(path).unwrap()).unwrap();
    let schema: Schema = serde_json::from_value(doc["schema"].clone()).unwrap();
    assert_eq!(
        Schema::decode(schema.encode_to_vec().as_slice()).unwrap(),
        schema
    );
    assert_eq!(
        serde_json::from_value::<Schema>(serde_json::to_value(&schema).unwrap()).unwrap(),
        schema
    );
    let engine = Engine::compile(schema.clone(), std::collections::HashMap::new()).unwrap();
    let input: StructValue = serde_json::from_value(doc["input"].clone()).unwrap();
    let outcome = engine.bake_detailed(&mut struct_to_native(Some(&input)));
    assert!(outcome.result.errors.is_empty());
    assert_eq!(
        outcome.report,
        Some(serde_json::from_value::<ResolveReport>(doc["report"].clone()).unwrap())
    );
    let baked = outcome.baked.unwrap();
    assert_eq!(
        baked,
        serde_json::from_value::<Baked>(doc["baked"].clone()).unwrap()
    );
    assert_eq!(
        baked.masked(),
        serde_json::from_value::<StructValue>(doc["masked"].clone()).unwrap()
    );
    assert_eq!(baked.schema, Some(schema));
}
