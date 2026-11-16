#[derive(schemapb::Reflect, serde::Serialize)]
#[serde(untagged)]
enum Bad {
    A { x: i64 },
}

fn main() {}
