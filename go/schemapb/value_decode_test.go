package schemapb_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

type (
	decodeLabel    string
	decodePort     uint16
	decodeKey      string
	DecodeEmbedded struct {
		Name string `json:"name"`
	}
)

type decodeHiddenEmbedded struct {
	Public bool `json:"public"`
}
type decodeItem struct {
	Port decodePort    `json:"port"`
	Wait time.Duration `json:"wait"`
}
type decodeConfig struct {
	*DecodeEmbedded
	decodeHiddenEmbedded
	Label   decodeLabel               `json:"label"`
	Big     uint64                    `json:"big"`
	Items   []decodeItem              `json:"items"`
	ByName  map[decodeKey]*decodeItem `json:"by_name"`
	Pair    [2]int8                   `json:"pair"`
	Blob    []byte                    `json:"blob"`
	Magic   [2]byte                   `json:"magic"`
	At      time.Time                 `json:"timestamp"`
	Opt     *string                   `json:"opt"`
	Raw     json.RawMessage           `json:"raw"`
	Any     any                       `json:"anything"`
	Missing string                    `json:"missing"`
	Gone    string                    `json:"-"`
}

func TestDecodeNativeTree(t *testing.T) {
	t.Parallel()

	item := sp.StructV(map[string]*sp.Value{"port": sp.UInt32V(65535), "wait": sp.DurationV(2 * time.Second)})
	timestamp := time.Date(2026, 9, 27, 1, 2, 3, 456, time.UTC)
	values := &sp.StructValue{Fields: map[string]*sp.Value{
		"name": sp.StrV("service"), "public": sp.BoolV(true), "label": sp.StrV("main"), "big": sp.UInt64V(math.MaxUint64),
		"items": sp.ListV(item), "by_name": sp.StructV(map[string]*sp.Value{"a": item, "none": sp.NullV()}),
		"pair": sp.ListV(sp.Int32V(-128), sp.Int64V(127)), "blob": sp.BytesV([]byte{1, 2}), "magic": sp.BytesV([]byte{3, 4}),
		"timestamp": sp.TimestampV(timestamp), "opt": sp.NullV(), "raw": sp.StructV(map[string]*sp.Value{"big": sp.UInt64V(math.MaxUint64)}),
		"anything": sp.StructV(map[string]*sp.Value{"duration": sp.DurationV(time.Second), "bytes": sp.BytesV([]byte{5})}),
	}}

	got := decodeConfig{Missing: "old", Gone: "old"}
	if err := (&sp.Baked{Values: values}).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.DecodeEmbedded == nil || got.Name != "service" || !got.Public || got.Label != "main" || got.Big != math.MaxUint64 || got.Missing != "" || got.Gone != "" || got.Opt != nil {
		t.Fatalf("bad decode: %+v", got)
	}

	if len(got.Items) != 1 || got.Items[0] != (decodeItem{Port: 65535, Wait: 2 * time.Second}) || *got.ByName["a"] != got.Items[0] || got.ByName["none"] != nil {
		t.Fatal("nested values differ")
	}

	if got.Pair != [2]int8{-128, 127} || !reflect.DeepEqual(got.Blob, []byte{1, 2}) || got.Magic != [2]byte{3, 4} || !got.At.Equal(timestamp) {
		t.Fatal("native types differ")
	}

	if string(got.Raw) != `{"big":18446744073709551615}` {
		t.Fatalf("raw JSON lost integer precision: %s", got.Raw)
	}

	native := got.Any.(map[string]any)
	if native["duration"] != time.Second {
		t.Fatalf("native duration: %T", native["duration"])
	}

	got.Blob[0] = 9
	native["bytes"].([]byte)[0] = 9
	got.ByName["a"].Port = 1

	if values.Fields["blob"].GetBytesValue()[0] != 1 || values.Fields["anything"].GetStructValue().Fields["bytes"].GetBytesValue()[0] != 5 || item.GetStructValue().Fields["port"].GetUint32Value() != 65535 {
		t.Fatal("decode aliases wire data")
	}
}

func TestDecodeAtomicFailure(t *testing.T) {
	t.Parallel()

	type target struct {
		A        map[string]string `json:"a"`
		Segments []decodeItem      `json:"segments"`
	}

	oldMap := map[string]string{"keep": "old"}
	oldItems := []decodeItem{{Port: 7}}
	dst := target{A: oldMap, Segments: oldItems}
	source := &sp.StructValue{Fields: map[string]*sp.Value{
		"a":        sp.StructV(map[string]*sp.Value{"new": sp.StrV("new")}),
		"segments": sp.ListV(sp.StructV(map[string]*sp.Value{"port": sp.StrV("secret-payload")})),
	}}
	err := source.Decode(&dst)

	var de *sp.DecodeError
	if !errors.As(err, &de) || de.Path != "segments[0].port" || strings.Contains(err.Error(), "secret-payload") {
		t.Fatalf("decode error: %v", err)
	}

	if !reflect.DeepEqual(dst, target{A: oldMap, Segments: oldItems}) || len(oldMap) != 1 || oldItems[0].Port != 7 {
		t.Fatal("failed decode changed target")
	}
}

func TestDecodeFailures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		value  *sp.Value
		target any
	}{
		{"narrow signed", sp.Int32V(128), new(int8)},
		{"narrow unsigned", sp.UInt32V(65536), new(uint16)},
		{"negative unsigned", sp.Int32V(-1), new(uint64)},
		{"inexact float", sp.Int64V(1<<53 + 1), new(float64)},
		{"fractional integer", sp.DoubleV(1.5), new(int)},
		{"duration is not int", sp.DurationV(time.Second), new(int64)},
		{"no string coercion", sp.StrV("30s"), new(time.Duration)},
		{"array length", sp.ListV(sp.StrV("a")), new([2]string)},
		{"byte array length", sp.BytesV([]byte{1}), new([2]byte)},
		{"list is not bytes", sp.ListV(sp.UInt32V(1)), new([]byte)},
		{"list is not byte array", sp.ListV(sp.UInt32V(1)), new([1]byte)},
		{"nonstring map key", sp.StructV(nil), new(map[int]string)},
		{"null scalar", sp.NullV(), new(bool)},
		{"invalid timestamp", &sp.Value{Kind: &sp.Value_TimestampValue{TimestampValue: &timestamppb.Timestamp{Seconds: math.MaxInt64}}}, new(time.Time)},
		{"duration overflow", &sp.Value{Kind: &sp.Value_DurationValue{DurationValue: &durationpb.Duration{Seconds: 315576000000}}}, new(time.Duration)},
		{"nonempty interface", sp.StrV("x"), new(error)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeOf(tc.target).Elem(), Tag: `json:"value"`}})
			dst := reflect.New(typ)
			err := (&sp.StructValue{Fields: map[string]*sp.Value{"value": tc.value}}).Decode(dst.Interface())

			var de *sp.DecodeError
			if !errors.As(err, &de) || de.Path != "value" {
				t.Fatalf("expected value error, got %v", err)
			}
		})
	}

	for _, dst := range []any{nil, 42, (*decodeConfig)(nil)} {
		if err := (&sp.StructValue{}).Decode(dst); err == nil {
			t.Fatal("invalid target accepted")
		}
	}

	var b *sp.Baked
	if err := b.Decode(new(decodeConfig)); err == nil {
		t.Fatal("nil baked accepted")
	}

	type unknown struct {
		X string `json:"x"`
	}

	if err := (&sp.StructValue{Fields: map[string]*sp.Value{"other": sp.StrV("x")}}).Decode(new(unknown)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

type decodeText string

func (d *decodeText) UnmarshalText(data []byte) error {
	if string(data) == "bad-secret" {
		return errors.New("bad-secret")
	}

	*d = decodeText(strings.ToUpper(string(data)))

	return nil
}

type decodeJSON struct {
	Count uint64 `json:"count"`
}

func (d *decodeJSON) UnmarshalJSON(data []byte) error {
	type plain decodeJSON
	return json.Unmarshal(data, (*plain)(d))
}

func TestDecodeCustomTypes(t *testing.T) {
	t.Parallel()

	type config struct {
		Text decodeText `json:"text"`
		JSON decodeJSON `json:"json"`
	}

	s := &sp.StructValue{Fields: map[string]*sp.Value{"text": sp.StrV("hello"), "json": sp.StructV(map[string]*sp.Value{"count": sp.UInt64V(math.MaxUint64)})}}

	var got config
	if err := s.Decode(&got); err != nil || got.Text != "HELLO" || got.JSON.Count != math.MaxUint64 {
		t.Fatalf("custom decode: %+v %v", got, err)
	}

	s.Fields["text"] = sp.StrV("bad-secret")
	if err := s.Decode(&got); err == nil || strings.Contains(err.Error(), "bad-secret") || got.Text != "HELLO" {
		t.Fatalf("custom failure: %v", err)
	}
}

// The exact same cross-language scalar matrix constrains the Go struct decoder.
func TestDecodeValueAsConformance(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(goldenDir + "/value-as.json")
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Cases []struct {
			Value  json.RawMessage `json:"value"`
			Target string          `json:"target"`
			Result json.RawMessage `json:"result"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	targets := map[string]struct {
		typ   reflect.Type
		field sp.FieldDef
	}{
		"bool": {reflect.TypeFor[bool](), sp.Bool("value")}, "int32": {reflect.TypeFor[int32](), sp.Int32("value")},
		"int64": {reflect.TypeFor[int64](), sp.Int64("value")}, "uint32": {reflect.TypeFor[uint32](), sp.UInt32("value")},
		"uint64": {reflect.TypeFor[uint64](), sp.UInt64("value")}, "float": {reflect.TypeFor[float32](), sp.Float("value")},
		"double": {reflect.TypeFor[float64](), sp.Double("value")}, "string": {reflect.TypeFor[string](), sp.Str("value")},
		"bytes": {reflect.TypeFor[[]byte](), sp.Bytes("value")}, "duration": {reflect.TypeFor[time.Duration](), sp.Duration("value")},
		"timestamp": {reflect.TypeFor[time.Time](), sp.Timestamp("value")},
		"list":      {reflect.TypeFor[[]any](), sp.List("value")}, "struct": {reflect.TypeFor[map[string]any](), sp.JSON("value")},
	}

	for i, tc := range doc.Cases {
		t.Run(strconv.Itoa(i)+"_"+tc.Target, func(t *testing.T) {
			t.Parallel()

			target, ok := targets[tc.Target]
			if !ok {
				t.Fatalf("unhandled target %s", tc.Target)
			}

			var value sp.Value
			if err := protojson.Unmarshal(tc.Value, &value); err != nil {
				t.Fatal(err)
			}

			dst := reflect.New(reflect.MapOf(reflect.TypeFor[string](), target.typ))

			err := (&sp.StructValue{Fields: map[string]*sp.Value{"value": &value}}).Decode(dst.Interface())
			if len(tc.Result) == 0 {
				if err == nil {
					t.Fatal("lossy conversion accepted")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			got, err := sp.CanonicalValue(target.field.Done(), dst.Elem().MapIndex(reflect.ValueOf("value")).Interface())
			if err != nil {
				t.Fatal(err)
			}

			var want sp.Value
			if err := protojson.Unmarshal(tc.Result, &want); err != nil {
				t.Fatal(err)
			}

			if !proto.Equal(got, &want) {
				t.Fatalf("got %v want %v", got, &want)
			}
		})
	}
}
