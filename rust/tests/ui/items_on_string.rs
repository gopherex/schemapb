#[derive(schemapb::Reflect)]
struct Bad {
    #[schemapb(items(min = 1))]
    name: String,
}

fn main() {}
