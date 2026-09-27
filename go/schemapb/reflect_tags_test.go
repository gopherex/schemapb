package schemapb_test

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

func TestReflectAttributes(t *testing.T) {
	t.Parallel()

	type config struct {
		Token   string        `json:"token"   schemapb:"secret=true;default=key;description=access token"`
		Workers int32         `json:"workers" schemapb:"default=4;gte=1;lte=64"`
		Timeout time.Duration `json:"timeout" schemapb:"default=1m30s;gt=0s"`
		Enabled bool          `json:"enabled" schemapb:"default=false"`
		Empty   string        `json:"empty"   schemapb:"default="`
		Unset   *string       `json:"unset"`
		Label   string        `json:"label"   schemapb:"default=\" a;b=\\\"c\\\" \";in=[\" a;b=\\\"c\\\" \",\"other\"]"`
		Blob    []byte        `json:"blob"    schemapb:"default=AQI=;min_len=1"`
		When    time.Time     `json:"when"    schemapb:"default=2026-09-27T10:20:30Z"`
		Mode    string        `json:"mode"    schemapb:"default={\"stringValue\":\"safe\"}"                             validate:"oneof=fast safe"`
		Nested  []struct {
			Value int64 `json:"value" schemapb:"default=0"`
		} `json:"nested,omitempty"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, _ := s.LookupPath("token")
	if !f.Secret || f.GetDescription() != "access token" {
		t.Fatal(f)
	}

	f, _ = s.LookupPath("empty")
	if f.GetString_().Default == nil {
		t.Fatal("empty default lost presence")
	}

	f, _ = s.LookupPath("unset")
	if f.GetString_().Default != nil {
		t.Fatal("absent default became present")
	}

	b, r, err := s.Bake(map[string]any{"nested": []any{map[string]any{}}})
	if err != nil || !r.Ok() {
		t.Fatalf("bake: %v %v", err, r)
	}

	var got config
	if err := b.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Token != "key" || got.Workers != 4 || got.Timeout != 90*time.Second || got.Enabled || got.Empty != "" || got.Label != " a;b=\"c\" " || got.Mode != "safe" || got.Unset != nil || len(got.Nested) != 1 || got.Nested[0].Value != 0 {
		t.Fatalf("unexpected decoded config: workers=%d timeout=%v", got.Workers, got.Timeout)
	}

	if !reflect.DeepEqual(got.Blob, []byte{1, 2}) || !got.When.Equal(time.Date(2026, 9, 27, 10, 20, 30, 0, time.UTC)) {
		t.Fatal("native types lost")
	}
}

func TestReflectTagErrors(t *testing.T) {
	t.Parallel()

	for _, tag := range []string{
		"unknown=1", "default=nope", "secret=maybe", "default=2147483648", "default=null",
		"name=other", "string={}", "gte=1;gte=2", "description", "description=\"unterminated",
		"in=[1,2}", "rules=[{\"unknown\":true}]", "in=null",
	} {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			typ := reflect.StructOf([]reflect.StructField{{Name: "Count", Type: reflect.TypeFor[int32](), Tag: reflect.StructTag(`json:"count" schemapb:` + strconv.Quote(tag))}})

			_, err := sp.Reflect(typ, testID(t))
			if err == nil || !strings.Contains(err.Error(), "Count") {
				t.Fatalf("expected field error, got %v", err)
			}
		})
	}

	type noContainerDefault struct {
		Items []string `json:"items" schemapb:"default=[]"`
	}

	if _, err := sp.ReflectType[noContainerDefault](testID(t)); err == nil {
		t.Fatal("list default accepted")
	}
}

func TestSetFieldAttributeUsesDescriptors(t *testing.T) {
	t.Parallel()

	f := sp.Str("token").MinLen(2).Secret().Done()
	if err := sp.SetFieldAttribute(f, "maxLen", "20"); err != nil {
		t.Fatal(err)
	}

	if err := sp.SetFieldAttribute(f, "secret", "false"); err != nil {
		t.Fatal(err)
	}

	if err := sp.SetFieldAttribute(f, "rules", `[{"expr":"this != ''","message":"required; again"}]`); err != nil {
		t.Fatal(err)
	}

	if f.Secret || f.GetString_().GetMinLen() != 2 || f.GetString_().GetMaxLen() != 20 || len(f.Rules) != 1 {
		t.Fatal(f)
	}

	before := proto.Clone(f)
	if err := sp.SetFieldAttribute(f, "min_len", "not-an-int"); err == nil || !proto.Equal(f, before) {
		t.Fatal("failed attribute mutated field")
	}

	if err := sp.SetFieldAttribute(nil, "secret", "true"); err == nil {
		t.Fatal("nil field accepted")
	}

	broken := &sp.Schema_Field{Kind: &sp.Schema_Field_String_{}}
	if err := sp.SetFieldAttribute(broken, "default", "x"); err == nil {
		t.Fatal("nil kind accepted")
	}
}

func TestReflectFieldTagHooks(t *testing.T) {
	t.Parallel()

	type domain string

	type config struct {
		Token  domain `default:"project" json:"token" schemapb:"secret=false;default=builtin" secret:"true"`
		Nested struct {
			Token domain `default:"nested" json:"token"`
		} `json:"nested"`
		Skip domain `json:"-"`
	}

	template := sp.Str("token").Secret().Default("base").Done()
	before := proto.Clone(template)

	var order []string

	s, err := sp.ReflectType[config](testID(t),
		sp.WithType(reflect.TypeFor[domain](), func(sp.FieldName) *sp.Schema_Field { return template }),
		sp.WithFieldTags(func(sf reflect.StructField, f *sp.Schema_Field) error {
			order = append(order, sf.Name)
			if raw, ok := sf.Tag.Lookup("default"); ok {
				return sp.SetFieldAttribute(f, "default", raw)
			}

			return nil
		}),
		sp.WithFieldTags(func(sf reflect.StructField, f *sp.Schema_Field) error {
			if sf.Tag.Get("secret") == "true" {
				f.Secret = true
			}

			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(template, before) {
		t.Fatal("type override template was mutated")
	}

	f, _ := s.LookupPath("token")
	if !f.Secret || f.GetString_().GetDefault() != "project" {
		t.Fatal(f)
	}

	f, _ = s.LookupPath("nested.token")
	if f.GetString_().GetDefault() != "nested" {
		t.Fatal(f)
	}

	if !reflect.DeepEqual(order, []string{"Token", "Token", "Nested"}) {
		t.Fatal(order)
	}

	sentinel := errors.New("project hook failed")

	_, err = sp.ReflectType[config](testID(t), sp.WithFieldTags(func(reflect.StructField, *sp.Schema_Field) error { return sentinel }))
	if !errors.Is(err, sentinel) {
		t.Fatalf("hook error lost: %v", err)
	}
}
