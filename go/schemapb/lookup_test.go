package schemapb_test

import (
	"errors"
	"testing"

	"github.com/gopherex/schemapb/go/schemapb"
)

// Cases the kitchen-sink golden cannot host: a broken ref (the golden
// schema must stay valid), typed-segment Lookup, error texts, name rules.

func lookupFixture(t *testing.T) *schemapb.Schema {
	t.Helper()

	s, err := schemapb.NewSchema(schemapb.ID("t", "lookup", schemapb.Ver(1, 0, 0))).
		Fields(
			schemapb.Str("name"),
			schemapb.Object("cfg", schemapb.Bool("on")),
		).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestLookupTypedSegments(t *testing.T) {
	t.Parallel()

	s := lookupFixture(t)

	f, err := s.Lookup(schemapb.FieldName("cfg"), schemapb.FieldName("on"))
	if err != nil {
		t.Fatal(err)
	}

	if got := schemapb.KindName(f); got != schemapb.KindBool {
		t.Fatalf("kind = %q, want bool", got)
	}
}

func TestLookupErrorText(t *testing.T) {
	t.Parallel()

	s := lookupFixture(t)

	_, err := s.LookupPath("cfg.off")

	var lerr *schemapb.LookupError
	if !errors.As(err, &lerr) {
		t.Fatalf("want LookupError, got %v", err)
	}

	if lerr.At != "cfg" || lerr.Segment != "off" || lerr.Reason != schemapb.LookupNotFound {
		t.Fatalf("wrong error fields: %+v", lerr)
	}

	const want = `schemapb: lookup: no field "off" in "cfg"`
	if lerr.Error() != want {
		t.Fatalf("error text = %q, want %q", lerr.Error(), want)
	}
}

func TestLookupUnknownRef(t *testing.T) {
	t.Parallel()

	// Built by hand: builders/Compile would refuse the dangling ref.
	s := &schemapb.Schema{
		Fields: []*schemapb.Schema_Field{{
			Name: "vol",
			Kind: &schemapb.Schema_Field_Ref_{Ref: &schemapb.Schema_Field_Ref{
				Target: &schemapb.Schema_Field_Ref_Name{Name: "missing"},
			}},
		}},
	}

	_, err := s.LookupPath("vol.path")

	var lerr *schemapb.LookupError
	if !errors.As(err, &lerr) || lerr.Reason != schemapb.LookupUnknownRef {
		t.Fatalf("want unknown_ref, got %v", err)
	}
}

func TestLookupEmptyPath(t *testing.T) {
	t.Parallel()

	s := lookupFixture(t)

	for _, try := range []func() (*schemapb.Schema_Field, error){
		func() (*schemapb.Schema_Field, error) { return s.Lookup() },
		func() (*schemapb.Schema_Field, error) { return s.LookupPath("") },
	} {
		_, err := try()

		var lerr *schemapb.LookupError
		if !errors.As(err, &lerr) || lerr.Reason != schemapb.LookupEmptyPath {
			t.Fatalf("want empty_path, got %v", err)
		}
	}
}

func TestFieldNameRules(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"a.b", "my-field", "1st", "in", "true", "while"} {
		s := &schemapb.Schema{
			Id: schemapb.ID("t", "names", schemapb.Ver(1, 0, 0)),
			Fields: []*schemapb.Schema_Field{{
				Name: bad,
				Kind: &schemapb.Schema_Field_String_{String_: &schemapb.Schema_Field_String{}},
			}},
		}

		if _, err := schemapb.Compile(s); err == nil {
			t.Errorf("field name %q compiled, want descriptor error", bad)
		}
	}

	// snake_case and camelCase both stay legal.
	for _, good := range []string{"snake_case", "camelCase", "_x", "a1"} {
		s := &schemapb.Schema{
			Id: schemapb.ID("t", "names", schemapb.Ver(1, 0, 0)),
			Fields: []*schemapb.Schema_Field{{
				Name: good,
				Kind: &schemapb.Schema_Field_String_{String_: &schemapb.Schema_Field_String{}},
			}},
		}

		if _, err := schemapb.Compile(s); err != nil {
			t.Errorf("field name %q rejected: %v", good, err)
		}
	}
}
