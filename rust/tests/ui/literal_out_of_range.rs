#[derive(schemapb::Reflect)]
struct Bad {
    #[schemapb(lte = 300)]
    level: u8,
}

fn main() {}
