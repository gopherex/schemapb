//! A malformed CEL expression is a schema error, never a crash: schemas
//! come from storage, and one bad rule must not take a process down.

use schemapb::build::*;

#[test]
fn malformed_expressions_are_schema_errors() {
    for expr in [
        "this >",
        "root.a +",
        "((",
        "1 +* 2",
        "this.size( > 1",
        "",
        "]",
    ] {
        let built = std::panic::catch_unwind(|| {
            schema("t", "cel")
                .field(int64("a").rule(rule(expr, "m")))
                .build()
                .map(|_| ())
        });
        match built {
            Ok(Err(e)) => assert!(e.to_string().contains("rule[0]"), "{expr:?}: {e}"),
            Ok(Ok(())) => panic!("{expr:?} compiled"),
            Err(payload) => panic!("{expr:?} panicked: {payload:?}"),
        }
    }
}

#[test]
fn the_spec_functions_evaluate() {
    let v = schema("t", "fns")
        .field(string("name").normalize("this.trim().lowerAscii()"))
        .field(map_of("labels", string("value")))
        .field(computed("keys", "root.labels.map(k, k).sort()"))
        .field(computed("upper", "root.name.upperAscii()"))
        .field(computed("flag", "string(true)"))
        .build()
        .unwrap();
    let mut values = schemapb::Values::new().with("name", "  Ab ").with(
        "labels",
        schemapb::Values::new().with("b", "1").with("a", "2"),
    );
    v.resolve(&mut values).unwrap();
    assert_eq!(values.get::<String>("name").unwrap().as_deref(), Some("ab"));
    assert_eq!(
        values.get::<Vec<String>>("keys").unwrap(),
        Some(vec!["a".into(), "b".into()])
    );
    assert_eq!(
        values.get::<String>("upper").unwrap().as_deref(),
        Some("AB")
    );
    assert_eq!(
        values.get::<String>("flag").unwrap().as_deref(),
        Some("true")
    );
}
