# Displaying baked values

Go exposes `(*Baked).Masked() *StructValue`. TypeScript and Python export
`masked(baked)`, and Rust exposes `Baked::masked() -> StructValue`.

These APIs return an independent wire-value copy for instance state, cards,
logs or other displays. They read the schema embedded in the snapshot and do
not compile it, execute CEL, normalize, apply defaults or change the snapshot.
No native/JSON conversion occurs: public values keep their exact wire kinds
and numeric precision. The work is a deep copy and a schema traversal.

Every **present** field marked `secret` is replaced with a string value
containing exactly `***`, regardless of its original type. A secret object,
list or map is replaced in its entirety. Explicit null on a secret field is
also masked; absent fields remain absent. Nonsecret nulls remain null.
Masking applies to inactive fields too: `when` controls resolution and
validation, not whether a stored secret can be displayed.

For nonsecret containers, masking descends through Object, homogeneous lists,
positional tuples, both Map value forms, local and linked identity Ref, and
the selected OneOf variant. A OneOf's variant is selected before its
potentially secret discriminator is masked. Ref lookup uses the embedded
root definitions; no registry or network access is required. Recursive
references traverse only the finite values present in the snapshot.

Fields not declared by the schema are copied unchanged. Free-form JSON has
no child field descriptors: mark the JSON field itself secret to hide it.
Masking does not infer sensitivity from names or track the provenance of
computed/copied values; those destination fields need their own secret flag.

A normal Baked snapshot contains its resolved schema. For incomplete snapshots,
a missing schema masks every top-level value, and an unresolved Ref, unknown
OneOf variant or incompatible container shape masks that whole field. This
prevents an unresolved schema branch from silently exposing its contents.
An absent values message yields an empty StructValue; Go also accepts a nil
Baked receiver.

The result is a **display projection**, not a new valid configuration: a
secret numeric/container field now contains a string. Do not feed it back to
Bake or use it for execution. Send the returned StructValue to the UI, rather
than sending the original Baked alongside it; the embedded schema can itself
contain secret defaults.

```go
visible := baked.Masked()
// visible.Fields["password"].GetStringValue() == "***"
// baked.Values still contains the original typed values.
```

The shared `conformance/golden/masked.json` cases pin exact output across all
four languages. The runners also check that output mutations cannot affect
the source snapshot or subsequent masking calls.
