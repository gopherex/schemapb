package schemapb_test

import (
	"errors"
	"testing"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

// invalid-schema.json / invalid-schema-errors.json pin descriptor checks
// across ports: one schema with
// several defects per field, whose INVALID_SCHEMA errors every port must
// report in the same order (field order, then check order). No defs: their
// map has no defined order.
func invalidSchema() *schemapb.Schema {
	str := &schemapb.Schema_Field_String_{String_: &schemapb.Schema_Field_String{}}

	return &schemapb.Schema{
		Id: &schemapb.SchemaIdentity{Name: "invalid"},
		Fields: []*schemapb.Schema_Field{
			{Name: "m", Kind: &schemapb.Schema_Field_Map_{Map: &schemapb.Schema_Field_Map{
				MinEntries:  ptr(uint64(5)),
				MaxEntries:  ptr(uint64(2)),
				ValueSchema: &schemapb.Schema{},
				ValueField:  &schemapb.Schema_Field{Name: "value", Kind: str},
				KeyField: &schemapb.Schema_Field{Name: "key", Kind: &schemapb.Schema_Field_Double_{
					Double: &schemapb.Schema_Field_Double{},
				}},
			}}},
			{Name: "", Kind: str},
			{Name: "dup", Kind: str},
			{Name: "dup", Kind: str},
			{Name: "nokind"},
			{Name: "empty_list", Kind: &schemapb.Schema_Field_List_{List: &schemapb.Schema_Field_List{}}},
			{Name: "closed", Kind: &schemapb.Schema_Field_Choice_{Choice: &schemapb.Schema_Field_Choice{}}},
			{Name: "union", Kind: &schemapb.Schema_Field_OneOf_{OneOf: &schemapb.Schema_Field_OneOf{}}},
			{Name: "ref", Kind: &schemapb.Schema_Field_Ref_{Ref: &schemapb.Schema_Field_Ref{
				Target: &schemapb.Schema_Field_Ref_Name{Name: "missing"},
			}}},
		},
	}
}

func TestGoldenInvalidSchema(t *testing.T) {
	t.Parallel()

	var serr *schemapb.SchemaError
	if err := invalidSchema().CheckDescriptor(); !errors.As(err, &serr) {
		t.Fatalf("want a SchemaError, got %v", err)
	}

	checkGolden(t, "invalid-schema.json", stableJSON(t, invalidSchema()))
	checkGolden(t, "invalid-schema-errors.json", stableJSON(t, serr.Result))
}
