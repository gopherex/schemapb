#[derive(schemapb::Reflect)]
struct Bad {
    #[schemapb(pattern = "^[0-9]+$")]
    count: i64,
}

fn main() {}
