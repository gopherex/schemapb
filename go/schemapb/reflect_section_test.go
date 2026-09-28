package schemapb_test

import (
	"testing"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

// A non-pointer struct field is a section: optional, with an implicit empty
// object default, so an absent section still resolves its inner defaults.

type sectionDB struct {
	Host string `json:"host" schemapb:"default=localhost"`
	Port int64  `json:"port" schemapb:"default=5432"`
}

type sectionAuth struct {
	Token string `json:"token"`
	Realm string `json:"realm" schemapb:"default=main"`
}

func hasError(res *sp.ValidationResult, path string, code sp.ErrorCode) bool {
	for _, e := range res.GetErrors() {
		if e.GetPath() == path && e.GetCode() == code {
			return true
		}
	}

	return false
}

func TestReflectSectionOptionalWithInnerDefaults(t *testing.T) {
	t.Parallel()

	type config struct {
		DB sectionDB `json:"db"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, _ := s.LookupPath("db")
	if f.GetRequired() || f.GetObject().GetDefault() == nil {
		t.Fatalf("section must be optional with an empty default: %v", f)
	}

	baked, res, err := s.Bake(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("absent section: %v %v", res, err)
	}

	var got config
	if err := baked.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.DB.Host != "localhost" || got.DB.Port != 5432 {
		t.Fatalf("inner defaults not applied: %+v", got)
	}
}

func TestReflectSectionRequiredInnerFieldReportedInside(t *testing.T) {
	t.Parallel()

	type config struct {
		Auth sectionAuth `json:"auth"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	_, res, err := s.Bake(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if !hasError(res, "auth.token", sp.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Fatalf("want auth.token REQUIRED_MISSING, got %v", res)
	}

	if hasError(res, "auth", sp.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Fatalf("section itself reported missing: %v", res)
	}
}

func TestReflectSectionValidateRequired(t *testing.T) {
	t.Parallel()

	type config struct {
		DB sectionDB `json:"db" validate:"required"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, _ := s.LookupPath("db")
	if !f.GetRequired() || f.GetObject().GetDefault() != nil {
		t.Fatalf("validate:required section must stay required without default: %v", f)
	}

	_, res, err := s.Bake(nil)
	if err != nil {
		t.Fatal(err)
	}

	if !hasError(res, "db", sp.ErrorCode_ERROR_CODE_REQUIRED_MISSING) {
		t.Fatalf("want db REQUIRED_MISSING, got %v", res)
	}
}

func TestReflectSectionExplicitTagsOverrideImplicit(t *testing.T) {
	t.Parallel()

	type explicit struct {
		DB sectionDB `json:"db" schemapb:"default={}"`
	}

	s, err := sp.ReflectType[explicit](testID(t))
	if err != nil {
		t.Fatalf("explicit default={} must keep working: %v", err)
	}

	if b, res, err := s.Bake(nil); err != nil || !res.Ok() || b == nil {
		t.Fatalf("explicit default: %v %v", res, err)
	}

	type forced struct {
		DB sectionDB `json:"db" schemapb:"required=true"`
	}

	s, err = sp.ReflectType[forced](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, _ := s.LookupPath("db")
	if !f.GetRequired() {
		t.Fatalf("schemapb tag must override implicit optionality: %v", f)
	}

	type invalid struct {
		DB sectionDB `json:"db" schemapb:"default={\"port\":1}"`
	}

	if _, err := sp.ReflectType[invalid](testID(t)); err == nil {
		t.Fatal("explicit non-empty object default must still be rejected")
	}
}

func TestReflectPointerSectionUnchanged(t *testing.T) {
	t.Parallel()

	type config struct {
		DB *sectionDB `json:"db"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, _ := s.LookupPath("db")
	if f.GetRequired() || !f.GetNullable() || f.GetObject().GetDefault() != nil {
		t.Fatalf("pointer section must stay optional, nullable, without default: %v", f)
	}

	baked, res, err := s.Bake(nil)
	if err != nil || !res.Ok() {
		t.Fatalf("absent pointer section: %v %v", res, err)
	}

	var got config
	if err := baked.Decode(&got); err != nil || got.DB != nil {
		t.Fatalf("pointer section materialized: %v %+v", err, got)
	}
}

func TestReflectImmutableSectionKeepsRequired(t *testing.T) {
	t.Parallel()

	type config struct {
		DB sectionDB `json:"db" schemapb:"immutable=true"`
	}

	s, err := sp.ReflectType[config](testID(t))
	if err != nil {
		t.Fatalf("immutable section must still reflect: %v", err)
	}

	f, _ := s.LookupPath("db")
	if !f.GetImmutable() || !f.GetRequired() || f.GetObject().GetDefault() != nil {
		t.Fatalf("immutable section must stay required without implicit default: %v", f)
	}

	type explicit struct {
		DB sectionDB `json:"db" schemapb:"immutable=true;default={}"`
	}

	if _, err := sp.ReflectType[explicit](testID(t)); err == nil {
		t.Fatal("explicit immutable object default must still be rejected")
	}
}
