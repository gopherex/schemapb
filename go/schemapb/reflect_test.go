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

func testID(tb testing.TB) *schemapb.SchemaIdentity {
	tb.Helper()
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

// textID is a google/uuid-like [16]byte that encoding/json writes as text.
type textID [16]byte

func (textID) MarshalText() ([]byte, error) {
	return []byte("00000000-0000-0000-0000-000000000000"), nil
}

// textRef marshals as text through a pointer receiver.
type textRef struct{ ID int }

func (*textRef) MarshalText() ([]byte, error) { return []byte("ref"), nil }

// bothMarshalers prefers JSON, as encoding/json does.
type bothMarshalers [4]byte

func (bothMarshalers) MarshalJSON() ([]byte, error) { return []byte("null"), nil }
func (bothMarshalers) MarshalText() ([]byte, error) { return []byte("text"), nil }

// A type encoding/json writes as text reflects as a string, wherever it sits.
func TestReflectTextMarshalerBecomesString(t *testing.T) {
	t.Parallel()

	type wrap struct {
		ID      textID             `json:"id"`
		Ref     textRef            `json:"ref"`
		Both    bothMarshalers     `json:"both"`
		At      time.Time          `json:"at"`
		IDs     []textID           `json:"ids"`
		Pair    [2]textID          `json:"pair"`
		ByName  map[string]textID  `json:"by_name"`
		Refs    map[string]textRef `json:"refs"`
		Pointer *textID            `json:"pointer"`
	}

	s, err := schemapb.ReflectType[wrap](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	item := func(f *schemapb.Schema_Field) *schemapb.Schema_Field { return f.GetList().GetItems()[0] }
	value := func(f *schemapb.Schema_Field) *schemapb.Schema_Field { return f.GetMap().GetValueField() }
	self := func(f *schemapb.Schema_Field) *schemapb.Schema_Field { return f }

	for _, tc := range []struct {
		path string
		pick func(*schemapb.Schema_Field) *schemapb.Schema_Field
		want schemapb.FieldKind
	}{
		{path: "id", pick: self, want: schemapb.KindString},
		{path: "ref", pick: self, want: schemapb.KindString},
		{path: "both", pick: self, want: schemapb.KindJSON},
		{path: "at", pick: self, want: schemapb.KindTimestamp},
		{path: "ids", pick: item, want: schemapb.KindString},
		{path: "pair", pick: item, want: schemapb.KindString},
		{path: "by_name", pick: value, want: schemapb.KindString},
		{path: "refs", pick: value, want: schemapb.KindString},
		{path: "pointer", pick: self, want: schemapb.KindString},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			f, err := s.LookupPath(tc.path)
			if err != nil {
				t.Fatal(err)
			}

			if got := schemapb.KindName(tc.pick(f)); got != tc.want {
				t.Fatalf("%s reflects as %s, want %s", tc.path, got, tc.want)
			}
		})
	}
}

// A marshaling root struct is one value, like the same type in a field.
func TestReflectMarshalingRoot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		typ  reflect.Type
		want schemapb.FieldKind
	}{
		{name: "text marshaler", typ: reflect.TypeFor[textRef](), want: schemapb.KindString},
		{name: "json marshaler", typ: reflect.TypeFor[asMarshaler](), want: schemapb.KindJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, err := schemapb.Reflect(tc.typ, testID(t))
			if err != nil {
				t.Fatal(err)
			}

			if f, err := s.LookupPath("value"); err != nil || schemapb.KindName(f) != tc.want {
				t.Fatalf("root reflects as %v %v, want one %s value", schemapb.KindName(f), err, tc.want)
			}
		})
	}
}

func TestReflectTextMarshalerOverride(t *testing.T) {
	t.Parallel()

	type wrap struct {
		ID textID `json:"id"`
	}

	s, err := schemapb.ReflectType[wrap](testID(t), schemapb.WithType(reflect.TypeFor[textID](),
		func(name schemapb.FieldName) *schemapb.Schema_Field { return schemapb.Bytes(name).Done() }))
	if err != nil {
		t.Fatal(err)
	}

	if f, err := s.LookupPath("id"); err != nil || schemapb.KindName(f) != schemapb.KindBytes {
		t.Fatalf("WithType must win over the text rule, got %v %v", schemapb.KindName(f), err)
	}
}

// textWrapped marshals as text, but its SchemaInner describes it.
type textWrapped struct{}

func (textWrapped) MarshalText() ([]byte, error) { return []byte("1"), nil }
func (textWrapped) SchemaInner() reflect.Type    { return reflect.TypeFor[int64]() }

func TestReflectTextMarshalerWrapper(t *testing.T) {
	t.Parallel()

	type wrap struct {
		W textWrapped `json:"w"`
	}

	s, err := schemapb.ReflectType[wrap](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	if f, err := s.LookupPath("w"); err != nil || schemapb.KindName(f) != schemapb.KindInt64 {
		t.Fatalf("SchemaInner must win over the text rule, got %v %v", schemapb.KindName(f), err)
	}
}

func BenchmarkReflectTextMarshaler(b *testing.B) {
	type wrap struct {
		ID  textID            `json:"id"`
		IDs []textID          `json:"ids"`
		Map map[string]textID `json:"map"`
	}

	id := testID(b)
	for b.Loop() {
		if _, err := schemapb.ReflectType[wrap](id); err != nil {
			b.Fatal(err)
		}
	}
}

func TestReflectBoolDefaultsFalse(t *testing.T) {
	t.Parallel()

	type params struct {
		Insecure bool `json:"insecure"`
		On       bool `json:"on"       schemapb:"default=true"`
		Must     bool `json:"must"     validate:"required"`
	}

	s, err := schemapb.ReflectType[params](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	insecure, _ := s.LookupPath("insecure")
	if insecure.GetRequired() || insecure.GetBool().Default == nil || insecure.GetBool().GetDefault() {
		t.Fatalf("bool without omitempty must default to false, got %+v", insecure)
	}

	on, _ := s.LookupPath("on")
	if !on.GetBool().GetDefault() {
		t.Fatalf("default=true must win, got %+v", on)
	}

	must, _ := s.LookupPath("must")
	if !must.GetRequired() || must.GetBool().Default != nil {
		t.Fatalf(`validate:"required" bool must stay required, got %+v`, must)
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
