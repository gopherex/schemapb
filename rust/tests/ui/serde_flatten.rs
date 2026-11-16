#[derive(schemapb::Reflect, serde::Serialize)]
struct Inner {
    a: i64,
}

#[derive(schemapb::Reflect, serde::Serialize)]
struct Bad {
    #[serde(flatten)]
    inner: Inner,
}

fn main() {}
