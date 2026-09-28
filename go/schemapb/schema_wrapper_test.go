package schemapb_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

// These wrappers exist only in tests. No runtime/config framework dependency.
type testLive[T any] struct {
	value T
	calls int
}

func (testLive[T]) SchemaInner() reflect.Type  { return reflect.TypeFor[T]() }
func (w *testLive[T]) SchemaDecodeTarget() any { return &w.value }
func (w *testLive[T]) SchemaField(f *sp.Schema_Field) error {
	w.calls++

	if f.Annotations == nil {
		f.Annotations = map[string]*sp.Value{}
	}

	f.Annotations["backplate.live"] = sp.BoolV(true)
	f.Annotations["test.calls"] = sp.Int32V(int32(w.calls))

	return nil
}
func (testLive[T]) MarshalJSON() ([]byte, error) { return []byte(`"wrong shape"`), nil }
func (*testLive[T]) UnmarshalJSON([]byte) error  { return errors.New("JSON hook must not run") }

type testSecret struct{ value string }

func (*testSecret) SchemaInner() reflect.Type            { return reflect.TypeFor[string]() }
func (w *testSecret) SchemaDecodeTarget() any            { return &w.value }
func (*testSecret) SchemaField(f *sp.Schema_Field) error { f.Secret = true; return nil }

type testDecorated string

func (*testDecorated) SchemaField(f *sp.Schema_Field) error { f.Secret = true; return nil }

type WrapperBackplane struct {
	Level testLive[string] `json:"level" schemapb:"default=warn"`
}

type wrapperConfig struct {
	WrapperBackplane
	DB struct {
		DSN      testSecret              `json:"dsn"       schemapb:"secret=false;default=private"`
		MaxConns testLive[int32]         `json:"max_conns" schemapb:"default=10;gte=1"`
		Timeout  testLive[time.Duration] `json:"timeout"   schemapb:"default=5s"`
	} `json:"db"       schemapb:"default={}"`
	Suffix   testLive[testSecret]               `json:"suffix"   schemapb:"default=!"`
	Values   []testLive[int64]                  `json:"values"`
	Tenants  map[string]testLive[wrapperTenant] `json:"tenants"`
	Optional testLive[*string]                  `json:"optional"`
	Outer    *testLive[string]                  `json:"outer"`
}
type wrapperTenant struct {
	Token testSecret      `json:"token"`
	Count testLive[int32] `json:"count" schemapb:"default=2"`
}

func TestSchemaWrapperPipeline(t *testing.T) {
	t.Parallel()

	s, err := sp.ReflectType[wrapperConfig](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{"values": []any{"9007199254740993"}, "tenants": map[string]any{"customer.a": map[string]any{"token": "private"}}, "optional": nil, "outer": nil}

	baked, res, err := s.Bake(input)
	if err != nil || !res.Ok() {
		t.Fatalf("bake: %v %v", res, err)
	}

	var got wrapperConfig
	if err := baked.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Level.value != "warn" || got.DB.DSN.value != "private" || got.DB.MaxConns.value != 10 || got.DB.Timeout.value != 5*time.Second || got.Suffix.value.value != "!" {
		t.Fatalf("unexpected config: %+v", got)
	}

	if got.Values[0].value != 9007199254740993 || got.Tenants["customer.a"].value.Token.value != "private" || got.Tenants["customer.a"].value.Count.value != 2 || got.Optional.value != nil || got.Outer != nil {
		t.Fatal("nested decode failed")
	}

	f, err := s.LookupPath("db.max_conns")
	if err != nil || f.GetInt32().GetGte() != 1 || !f.Annotations["backplate.live"].GetBoolValue() {
		t.Fatalf("schema: %v %v", f, err)
	}

	if f.Annotations["test.calls"].GetInt32Value() != 1 {
		t.Fatal("receiver was not fresh")
	}

	masked := baked.Masked()
	if masked.GetFields()["db"].GetStructValue().GetFields()["dsn"].GetStringValue() != "***" || masked.GetFields()["suffix"].GetStringValue() != "***" {
		t.Fatal("secret decorators lost")
	}

	if masked.GetFields()["tenants"].GetStructValue().GetFields()["customer.a"].GetStructValue().GetFields()["token"].GetStringValue() != "***" {
		t.Fatal("map wrapper schema bypassed")
	}

	second, err := sp.ReflectType[wrapperConfig](testID(t))
	if err != nil || !proto.Equal(s, second) {
		t.Fatal("reflection depends on prior invocation")
	}
}

func TestSchemaWrapperShapes(t *testing.T) {
	t.Parallel()

	type shapes struct {
		Duration        testLive[time.Duration]     `json:"duration"`
		Time            testLive[time.Time]         `json:"time"`
		Blob            testLive[[]byte]            `json:"blob"`
		Array           [2]testLive[int32]          `json:"array"`
		Pointers        map[string]*testLive[int32] `json:"pointers"`
		InnerPointer    testLive[*int32]            `json:"inner_pointer"`
		PointerRequired testLive[*int32]            `json:"pointer_required"  validate:"required"`
		Omitted         testLive[string]            `json:"omitted,omitempty"`
		Tagged          WrapperBackplane            `json:"backplane"`
		Decorated       testDecorated               `json:"decorated"`
	}

	s, err := sp.ReflectType[shapes](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path               string
		kind               sp.FieldKind
		nullable, required bool
	}{
		{"duration", sp.KindDuration, false, true},
		{"time", sp.KindTimestamp, false, true},
		{"blob", sp.KindBytes, false, true},
		{"inner_pointer", sp.KindInt32, true, false},
		{"pointer_required", sp.KindInt32, true, true},
		{"omitted", sp.KindString, false, false},
	} {
		f, e := s.LookupPath(tc.path)
		if e != nil || sp.KindName(f) != tc.kind || f.Nullable != tc.nullable || f.Required != tc.required {
			t.Fatalf("%s: %v %v", tc.path, f, e)
		}
	}

	root, err := sp.ReflectType[testLive[string]](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	if len(root.Fields) != 1 || root.Fields[0].Name != "value" || root.Fields[0].GetString_() == nil || !root.Fields[0].Annotations["backplate.live"].GetBoolValue() {
		t.Fatal("root wrapper bypassed")
	}

	input := sp.MustStructFromGo(map[string]any{"duration": time.Second, "time": time.Unix(123, 456).UTC(), "blob": []byte{1, 2}, "array": []any{int32(3), int32(4)}, "pointers": map[string]any{"x": nil, "y": int32(5)}, "inner_pointer": int32(6), "pointer_required": nil, "backplane": map[string]any{"level": "debug"}, "decorated": "secret"})

	var got shapes
	if err := input.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Duration.value != time.Second || !got.Time.value.Equal(time.Unix(123, 456)) || got.Array[1].value != 4 || got.Pointers["x"] != nil || got.Pointers["y"].value != 5 || *got.InnerPointer.value != 6 || got.PointerRequired.value != nil || got.Tagged.Level.value != "debug" {
		t.Fatal("wrong typed decoding")
	}

	got.Blob.value[0] = 99

	if input.Fields["blob"].GetBytesValue()[0] != 1 {
		t.Fatal("decode aliased snapshot")
	}
}

// A decorator observes tag handlers, and an outer decorator observes the inner.
type testOuter struct{ inner testSecret }

func (*testOuter) SchemaInner() reflect.Type { return reflect.TypeFor[testSecret]() }
func (w *testOuter) SchemaDecodeTarget() any { return &w.inner }
func (*testOuter) SchemaField(f *sp.Schema_Field) error {
	if !f.Secret || f.GetDescription() != "handler" {
		return errors.New("wrong decorator order")
	}

	return nil
}

func TestSchemaWrapperPrecedence(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Value testOuter `desc:"legacy" json:"value" schemapb:"secret=false;description=tag;default=ok"`
	}

	s, err := sp.ReflectType[cfg](testID(t), sp.WithFieldTags(func(_ reflect.StructField, f *sp.Schema_Field) error {
		if f.GetDescription() != "tag" || f.Secret {
			return errors.New("tag handler order")
		}

		f.Description = ptr("handler")

		return nil
	}))
	if err != nil || !s.Fields[0].Secret {
		t.Fatalf("hooks: %v", err)
	}

	overridden, err := sp.ReflectType[struct {
		Value testLive[string] `json:"value" schemapb:"default=7"`
	}](testID(t), sp.WithType(reflect.TypeFor[testLive[string]](), func(n sp.FieldName) *sp.Schema_Field { return sp.Int32(n).Done() }))
	if err != nil || overridden.Fields[0].GetInt32().GetDefault() != 7 || len(overridden.Fields[0].Annotations) != 0 {
		t.Fatalf("override lost precedence: %v", err)
	}
}

type badNilInner struct{}

func (badNilInner) SchemaInner() reflect.Type { return nil }

type (
	badCycleA struct{}
	badCycleB struct{}
)

func (badCycleA) SchemaInner() reflect.Type  { return reflect.TypeFor[badCycleB]() }
func (*badCycleB) SchemaInner() reflect.Type { return reflect.TypeFor[badCycleA]() }

type badConfig struct{}

func (badConfig) SchemaField(*sp.Schema_Field) error { return errors.New("invalid wrapper metadata") }

type badDescriptor string

func (badDescriptor) SchemaField(f *sp.Schema_Field) error { f.Kind = nil; return nil }
func TestSchemaWrapperFailures(t *testing.T) {
	t.Parallel()

	for _, typ := range []reflect.Type{reflect.TypeFor[badNilInner](), reflect.TypeFor[badCycleA](), reflect.TypeFor[badConfig](), reflect.TypeFor[badDescriptor]()} {
		t.Run(typ.Name(), func(t *testing.T) {
			t.Parallel()

			_, err := sp.Reflect(typ, testID(t))
			if err == nil {
				t.Fatal("invalid wrapper accepted")
			}
		})
	}
}

type badTarget[T any] struct{}

func (badTarget[T]) SchemaInner() reflect.Type { return reflect.TypeFor[string]() }
func (*badTarget[T]) SchemaDecodeTarget() any  { var zero T; return zero }

type noInner struct{}

func (*noInner) SchemaDecodeTarget() any { return new(string) }

type (
	decodeCycleA struct{ next *decodeCycleB }
	decodeCycleB struct{ next *decodeCycleA }
)

func (*decodeCycleA) SchemaInner() reflect.Type { return reflect.TypeFor[decodeCycleB]() }
func (*decodeCycleB) SchemaInner() reflect.Type { return reflect.TypeFor[decodeCycleA]() }
func (w *decodeCycleA) SchemaDecodeTarget() any { w.next = new(decodeCycleB); return w.next }
func (w *decodeCycleB) SchemaDecodeTarget() any { w.next = new(decodeCycleA); return w.next }
func TestSchemaDecodeTargetFailures(t *testing.T) {
	t.Parallel()

	input := sp.MustStructFromGo(map[string]any{"value": "private"})

	for _, typ := range []reflect.Type{reflect.TypeFor[badTarget[*string]](), reflect.TypeFor[badTarget[string]](), reflect.TypeFor[badTarget[any]](), reflect.TypeFor[noInner](), reflect.TypeFor[wrongTarget](), reflect.TypeFor[nilDecodeInner](), reflect.TypeFor[selfTarget](), reflect.TypeFor[decodeCycleA]()} {
		t.Run(typ.String(), func(t *testing.T) {
			t.Parallel()

			dst := reflect.New(reflect.StructOf([]reflect.StructField{{Name: "Value", Type: typ, Tag: `json:"value"`}})).Interface()
			err := input.Decode(dst)

			var de *sp.DecodeError
			if !errors.As(err, &de) || de.Path != "value" || strings.Contains(err.Error(), "private") {
				t.Fatalf("bad error: %v", err)
			}
		})
	}
}

func TestSchemaWrapperDecodeAtomic(t *testing.T) {
	t.Parallel()

	type cfg struct {
		A testLive[int64]   `json:"a"`
		Z []testLive[int32] `json:"z"`
	}

	got := cfg{A: testLive[int64]{value: 5}, Z: []testLive[int32]{{value: 7}}}
	input := sp.MustStructFromGo(map[string]any{"a": int64(9), "z": []any{int64(math.MaxInt64)}})
	err := input.Decode(&got)

	var de *sp.DecodeError
	if !errors.As(err, &de) || de.Path != "z[0]" || got.A.value != 5 || got.Z[0].value != 7 {
		t.Fatalf("non-atomic decode: %v %+v", err, got)
	}

	type nulls struct {
		Value testLive[string] `json:"value"`
	}

	var dst nulls
	if err := sp.MustStructFromGo(map[string]any{"value": nil}).Decode(&dst); err == nil {
		t.Fatal("null accepted for nonnullable inner type")
	}
}

// JSON-only wrappers remain supported; the native target hook is optional.
type jsonOnly string

func (jsonOnly) SchemaInner() reflect.Type { return reflect.TypeFor[string]() }
func (v *jsonOnly) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}

	*v = jsonOnly(s)

	return nil
}

func TestSchemaWrapperJSONFallback(t *testing.T) {
	t.Parallel()

	var got struct {
		Value jsonOnly `json:"value"`
	}
	if err := sp.MustStructFromGo(map[string]any{"value": "ok"}).Decode(&got); err != nil || got.Value != "ok" {
		t.Fatalf("JSON fallback: %v", err)
	}
}

type wrongTarget struct{ value int32 }

func (*wrongTarget) SchemaInner() reflect.Type { return reflect.TypeFor[string]() }
func (w *wrongTarget) SchemaDecodeTarget() any { return &w.value }

type nilDecodeInner struct{ value string }

func (*nilDecodeInner) SchemaInner() reflect.Type { return nil }
func (w *nilDecodeInner) SchemaDecodeTarget() any { return &w.value }

type selfTarget struct{}

func (*selfTarget) SchemaInner() reflect.Type { return reflect.TypeFor[selfTarget]() }
func (w *selfTarget) SchemaDecodeTarget() any { return w }

func TestSchemaWrapperAbsent(t *testing.T) {
	t.Parallel()
	// Every hook would fail if invoked; absent and outer-null fields skip them.
	var got struct {
		Absent noInner      `json:"absent"`
		Outer  *wrongTarget `json:"outer"`
	}
	if err := sp.MustStructFromGo(map[string]any{"outer": nil}).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Outer != nil {
		t.Fatal("outer null was materialized")
	}
}

func TestSchemaWrapperExactNumbers(t *testing.T) {
	t.Parallel()

	type target struct {
		U testLive[uint64]  `json:"u"`
		F testLive[float32] `json:"f"`
	}

	var got target
	if err := (&sp.StructValue{Fields: map[string]*sp.Value{"u": sp.UInt64V(math.MaxUint64), "f": sp.FloatV(1.25)}}).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.U.value != math.MaxUint64 || got.F.value != 1.25 {
		t.Fatal("numeric precision changed")
	}

	err := (&sp.StructValue{Fields: map[string]*sp.Value{"u": sp.UInt64V(5), "f": sp.DoubleV(0.1)}}).Decode(&got)

	var de *sp.DecodeError
	if !errors.As(err, &de) || de.Path != "f" || got.U.value != math.MaxUint64 || got.F.value != 1.25 {
		t.Fatalf("lossy conversion or partial write: %v", err)
	}
}

func TestSchemaWrapperNestedErrorPath(t *testing.T) {
	t.Parallel()

	var got struct {
		Tenants map[string]testLive[wrapperTenant] `json:"tenants"`
	}

	err := sp.MustStructFromGo(map[string]any{"tenants": map[string]any{"customer.a": map[string]any{"count": "invalid"}}}).Decode(&got)

	var de *sp.DecodeError
	if !errors.As(err, &de) || de.Path != `tenants["customer.a"].count` {
		t.Fatalf("lost wrapper path: %v", err)
	}
}

type testMarkedByte uint8

func (*testMarkedByte) SchemaField(f *sp.Schema_Field) error { f.Secret = true; return nil }

type testWrappedByte uint8

func (*testWrappedByte) SchemaInner() reflect.Type            { return reflect.TypeFor[uint8]() }
func (w *testWrappedByte) SchemaDecodeTarget() any            { return (*uint8)(w) }
func (*testWrappedByte) SchemaField(f *sp.Schema_Field) error { f.Secret = true; return nil }

func TestSchemaByteElementHooks(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Plain   []byte             `json:"plain"`
		Marked  []testMarkedByte   `json:"marked"`
		Wrapped [2]testWrappedByte `json:"wrapped"`
	}

	schema, err := sp.ReflectType[cfg](testID(t))
	if err != nil {
		t.Fatal(err)
	}

	if schema.Fields[0].GetBytes() == nil || schema.Fields[1].GetList() == nil || schema.Fields[2].GetList() == nil {
		t.Fatal("byte optimization swallowed element hooks")
	}

	baked, res, err := schema.Bake(map[string]any{"plain": []byte{3, 4}, "marked": []any{"1", "2"}, "wrapped": []any{"5", "6"}})
	if err != nil || !res.Ok() {
		t.Fatalf("bake: %v %v", res, err)
	}

	var got cfg
	if err := baked.Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Plain[0] != 3 || got.Marked[1] != 2 || got.Wrapped[1] != 6 {
		t.Fatal("wrong byte decoding")
	}

	masked := baked.Masked()
	for _, name := range []string{"marked", "wrapped"} {
		for _, item := range masked.Fields[name].GetListValue().GetItems() {
			if item.GetStringValue() != "***" {
				t.Fatal("secret byte element leaked")
			}
		}
	}
}
