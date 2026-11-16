#[derive(schemapb::Reflect)]
struct Bad {
    #[schemapb(gte = 3)]
    flag: bool,
}

fn main() {}
