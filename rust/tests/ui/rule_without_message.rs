#[derive(schemapb::Reflect)]
struct Bad {
    #[schemapb(rule("this > 0"))]
    count: i64,
}

fn main() {}
