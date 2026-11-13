package schemapb_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/gopherex/schemapb/go/schemapb"
)

// feature plays a consumer's enum-like key type.
type feature string

// collections exercises the validator's collection vocabulary: `unique`,
// `dive` (constraints on items / map values) and `keys ... endkeys`
// (constraints on map keys), plus min/max on a map.
type collections struct {
	Tags     []string         `json:"tags"     validate:"unique"`
	Modes    []string         `json:"modes"    validate:"dive,oneof=fast safe"`
	Names    []string         `json:"names"    validate:"min=1,dive,required"`
	Features map[feature]bool `json:"features" validate:"min=1,max=2,dive,keys,oneof=calls chat,endkeys"`
	Limits   map[string]int64 `json:"limits"   validate:"dive,keys,min=2,endkeys,gte=0"`
}

func compiled(t *testing.T) *schemapb.Engine {
	t.Helper()

	s, err := schemapb.ReflectType[collections](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	e, err := schemapb.Compile(s)
	if err != nil {
		t.Fatal(err)
	}

	return e
}

func validValue() map[string]any {
	return map[string]any{
		"tags":     []any{"a", "b"},
		"modes":    []any{"fast"},
		"names":    []any{"x"},
		"features": map[string]any{"calls": true, "chat": false},
		"limits":   map[string]any{"cpu": int64(2)},
	}
}

func TestReflectCollectionTags(t *testing.T) {
	t.Parallel()

	e := compiled(t)
	for _, tc := range []struct {
		name string
		set  func(v map[string]any)
		want []string // path:code, all must appear; none means valid
	}{
		{name: "valid", set: func(map[string]any) {}},
		{
			name: "unique list refuses a repeated item", set: func(v map[string]any) { v["tags"] = []any{"a", "a"} },
			want: []string{"tags[1]:ERROR_CODE_NOT_UNIQUE"},
		},
		{
			name: "dive constrains items", set: func(v map[string]any) { v["modes"] = []any{"fast", "reckless"} },
			want: []string{"modes[1]:ERROR_CODE_CHOICE_NOT_ALLOWED"},
		},
		{
			name: "min before dive bounds the list", set: func(v map[string]any) { v["names"] = []any{} },
			want: []string{"names:ERROR_CODE_MIN_ITEMS_VIOLATED"},
		},
		{name: "keys refuse an unknown key", set: func(v map[string]any) {
			v["features"] = map[string]any{"calls": true, "komeet": true}
		}, want: []string{"features.komeet:ERROR_CODE_CHOICE_NOT_ALLOWED"}},
		{
			name: "min bounds map entries", set: func(v map[string]any) { v["features"] = map[string]any{} },
			want: []string{"features:ERROR_CODE_MIN_ENTRIES_VIOLATED"},
		},
		{name: "keys and values both constrained", set: func(v map[string]any) {
			v["limits"] = map[string]any{"x": int64(-1)}
		}, want: []string{"limits.x:ERROR_CODE_MIN_LEN_VIOLATED", "limits.x:ERROR_CODE_GTE_VIOLATED"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := validValue()
			tc.set(v)

			errs := e.Validate(v).GetErrors()

			got := make([]string, 0, len(errs))
			for _, verr := range errs {
				got = append(got, verr.GetPath()+":"+verr.GetCode().String())
			}

			if len(tc.want) == 0 && len(got) > 0 {
				t.Fatalf("errors %v, want none", got)
			}

			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Fatalf("missing %s in %v", w, got)
				}
			}
		})
	}
}

// Constraints after `dive` belong to the items: `dive,required` does not make
// the list itself required.
func TestReflectDiveDoesNotConstrainTheCollection(t *testing.T) {
	t.Parallel()

	type model struct {
		Names []string `json:"names,omitempty" validate:"dive,required"`
	}

	s, err := schemapb.ReflectType[model](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	if f := s.GetFields()[0]; f.GetRequired() {
		t.Fatalf("names is required: %v", f)
	}
}

func TestReflectCollectionTagErrors(t *testing.T) {
	t.Parallel()

	type item struct {
		Name string `json:"name"`
	}

	type uniqueByField struct {
		X []item `json:"x" validate:"unique=Name"`
	}

	type uniqueMap struct {
		X map[string]string `json:"x" validate:"unique"`
	}

	type keysOnList struct {
		X []string `json:"x" validate:"dive,keys,min=1,endkeys"`
	}

	type keysUnclosed struct {
		X map[string]string `json:"x" validate:"dive,keys,min=1"`
	}

	type keysWithoutDive struct {
		X map[string]string `json:"x" validate:"keys,min=1,endkeys"`
	}

	for name, try := range map[string]func() error{
		"unique by a struct field is not representable": func() error {
			_, err := schemapb.ReflectType[uniqueByField](testID(t))
			return err
		},
		"unique map values are not representable": func() error {
			_, err := schemapb.ReflectType[uniqueMap](testID(t))
			return err
		},
		"keys on a list": func() error {
			_, err := schemapb.ReflectType[keysOnList](testID(t))
			return err
		},
		"keys without endkeys": func() error {
			_, err := schemapb.ReflectType[keysUnclosed](testID(t))
			return err
		},
		"keys without dive": func() error {
			_, err := schemapb.ReflectType[keysWithoutDive](testID(t))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := try(); err == nil || !strings.Contains(err.Error(), "validate tag") {
				t.Fatalf("err = %v, want a loud validate tag error", err)
			}
		})
	}
}
