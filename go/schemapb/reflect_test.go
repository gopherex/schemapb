package schemapb_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gopherex/schemapb/go/schemapb"
)

func TestReflectLoudFailures(t *testing.T) {
	t.Parallel()

	type badName struct {
		X string `json:"my-field"` //nolint:tagliatelle // the invalid name IS the test
	}

	type badTag struct {
		X string `json:"x" validate:"min=abc"`
	}

	type badMapKey struct {
		X map[int]string `json:"x"`
	}

	type badKind struct {
		X chan int `json:"x"`
	}

	for name, try := range map[string]func() error{
		"non-identifier json name": func() error {
			_, err := schemapb.ReflectType[badName](testID(t))
			return err
		},
		"malformed tag value": func() error {
			_, err := schemapb.ReflectType[badTag](testID(t))
			return err
		},
		"non-string map key": func() error {
			_, err := schemapb.ReflectType[badMapKey](testID(t))
			return err
		},
		"unsupported kind": func() error {
			_, err := schemapb.ReflectType[badKind](testID(t))
			return err
		},
	} {
		if err := try(); err == nil {
			t.Errorf("%s: reflected silently, want loud error", name)
		}
	}
}

func testID(t *testing.T) *schemapb.SchemaIdentity {
	t.Helper()
	return schemapb.ID("t", "reflect", schemapb.Ver(1, 0, 0))
}

// SecretName plays a consumer's domain type overridden via WithType.
type secretName string

func TestReflectWithTypeOverride(t *testing.T) {
	t.Parallel()

	type params struct {
		Token secretName    `json:"token"`
		Wait  time.Duration `json:"wait"`
	}

	s, err := schemapb.ReflectType[params](testID(t),
		schemapb.WithType(reflect.TypeFor[secretName](), func(n schemapb.FieldName) *schemapb.Schema_Field {
			return schemapb.Str(n).Secret().Done()
		}),
		// Overrides beat the stdlib branches too.
		schemapb.WithType(reflect.TypeFor[time.Duration](), func(n schemapb.FieldName) *schemapb.Schema_Field {
			return schemapb.Int64(n).Gte(0).Done()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	token, err := s.LookupPath("token")
	if err != nil || !token.GetSecret() || schemapb.KindName(token) != schemapb.KindString {
		t.Fatalf("token override not applied: %v %v", token, err)
	}

	wait, err := s.LookupPath("wait")
	if err != nil || schemapb.KindName(wait) != schemapb.KindInt64 {
		t.Fatalf("stdlib override not applied: %v %v", wait, err)
	}
}

type asMarshaler struct{ V int }

func (asMarshaler) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func TestReflectMarshalerBecomesJSON(t *testing.T) {
	t.Parallel()

	type wrap struct {
		M asMarshaler `json:"m"`
	}

	s, err := schemapb.ReflectType[wrap](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.LookupPath("m")
	if err != nil || schemapb.KindName(f) != schemapb.KindJSON {
		t.Fatalf("marshaler type should reflect as JSON, got %v %v", schemapb.KindName(f), err)
	}
}

func TestReflectPointerNullable(t *testing.T) {
	t.Parallel()

	type params struct {
		Opt  *string `json:"opt"`
		Must *string `json:"must" validate:"required"`
	}

	s, err := schemapb.ReflectType[params](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	opt, _ := s.LookupPath("opt")
	if opt.GetRequired() || !opt.GetNullable() {
		t.Fatalf("*T must be optional+nullable, got %+v", opt)
	}

	must, _ := s.LookupPath("must")
	if !must.GetRequired() || !must.GetNullable() {
		t.Fatalf(`validate:"required" pointer must be required+nullable, got %+v`, must)
	}
}

func TestReflectSchemaValidates(t *testing.T) {
	t.Parallel()

	// The reflected schema is a working engine: coercion on, constraints live.
	s, err := schemapb.ReflectType[mirrorModel](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	e, err := schemapb.Compile(s)
	if err != nil {
		t.Fatal(err)
	}

	res := e.Validate(map[string]any{
		"name": "", // MIN_LEN
		"mode": "reckless",
	})

	codes := make([]string, 0, len(res.GetErrors()))
	for _, verr := range res.GetErrors() {
		codes = append(codes, verr.GetPath()+":"+verr.GetCode().String())
	}

	joined := strings.Join(codes, ",")
	for _, want := range []string{"name:ERROR_CODE_MIN_LEN_VIOLATED", "mode:ERROR_CODE_CHOICE_NOT_ALLOWED", "must:ERROR_CODE_REQUIRED_MISSING"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %s", want, joined)
		}
	}
}
