package schemapb_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	sp "github.com/gopherex/schemapb/go/schemapb"
)

func TestJSONNumber(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, raw string
		field     sp.FieldDef
		want      any
		fail      bool
	}{
		{"above_js_integer", "9007199254740993", sp.Int64("value"), int64(9007199254740993), false},
		{"max_int", "9223372036854775807", sp.Int64("value"), int64(math.MaxInt64), false},
		{"min_int", "-9223372036854775808", sp.Int64("value"), int64(math.MinInt64), false},
		{"max_uint", "18446744073709551615", sp.UInt64("value"), uint64(math.MaxUint64), false},
		{"max_uint_exp", "184467440737095516150e-1", sp.UInt64("value"), uint64(math.MaxUint64), false},
		{"integral_exp", "1.5e1", sp.Int64("value"), int64(15), false},
		{"integral_decimal", "1000.0", sp.Int64("value"), int64(1000), false},
		{"signed_overflow", "9223372036854775808", sp.Int64("value"), nil, true},
		{"signed_underflow", "-9223372036854775809", sp.Int64("value"), nil, true},
		{"unsigned_overflow", "18446744073709551616", sp.UInt64("value"), nil, true},
		{"unsigned_negative", "-1", sp.UInt64("value"), nil, true},
		{"narrow_overflow", "2147483648", sp.Int32("value"), nil, true},
		{"narrow_unsigned_overflow", "4294967296", sp.UInt32("value"), nil, true},
		{"fraction", "1.5", sp.Int64("value"), nil, true},
		{"fraction_exp", "15e-1", sp.Int64("value"), nil, true},
		{"huge_exponent", "1e99999999999999999999", sp.Int64("value"), nil, true},
		{"tiny_exponent", "1e-99999999999999999999", sp.Int64("value"), nil, true},
		{"zero_exponent", "0e99999999999999999999", sp.Int64("value"), int64(0), false},
		{"float32_midpoint", "1.000000059604644775390625", sp.Float("value"), float32(1), false},
		{"float32_direct_rounding", "1.00000005960464477539062500000000001", sp.Float("value"), math.Float32frombits(0x3f800001), false},
		{"float32_even_upper", "1.000000178813934326171875", sp.Float("value"), math.Float32frombits(0x3f800002), false},
		{"float64_midpoint", "1.00000000000000011102230246251565404236316680908203125", sp.Double("value"), float64(1), false},
		{"float64_round", "0.1", sp.Double("value"), 0.1, false},
		{"float32_overflow", "3.5e38", sp.Float("value"), nil, true},
		{"float64_overflow", "1e309", sp.Double("value"), nil, true},
		{"float32_underflow", "1e-100", sp.Float("value"), float32(0), false},
		{"float64_underflow", "-1e-9999", sp.Double("value"), math.Copysign(0, -1), false},
		{"invalid_leading_zero", "01", sp.Int64("value"), nil, true},
		{"invalid_nan", "NaN", sp.Double("value"), nil, true},
		{"not_string", "4", sp.Str("value"), nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := sp.NewSchema(testID(t)).Fields(tc.field).MustBuild()

			b, r, err := s.Bake(map[string]any{"value": json.Number(tc.raw)})
			if err != nil {
				t.Fatal(err)
			}

			if tc.fail {
				if b != nil || r.Ok() {
					t.Fatalf("accepted %s: %v", tc.raw, r)
				}

				return
			}

			if !r.Ok() || b == nil {
				t.Fatalf("rejected %s: %v", tc.raw, r)
			}

			got := b.Values.Fields["value"]

			want, err := sp.CanonicalValue(tc.field.Done(), tc.want)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(stableJSON(t, got), stableJSON(t, want)) {
				t.Fatalf("got %s want %s", stableJSON(t, got), stableJSON(t, want))
			}

			direct, err := sp.CanonicalValue(tc.field.Done(), json.Number(tc.raw))
			if err != nil || !bytes.Equal(stableJSON(t, direct), stableJSON(t, want)) {
				t.Fatalf("direct conversion: %v %v", direct, err)
			}
		})
	}
}

func TestJSONNumberCollections(t *testing.T) {
	t.Parallel()

	decoder := json.NewDecoder(strings.NewReader(`{"ports":[9007199254740993],"limits":{"cpu":18446744073709551615}}`))
	decoder.UseNumber()

	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		t.Fatal(err)
	}

	schema := sp.NewSchema(testID(t)).Fields(sp.List("ports", sp.Int64("")), sp.MapOf("limits", sp.UInt64("value"))).MustBuild()

	b, r, err := schema.Bake(values)
	if err != nil || !r.Ok() {
		t.Fatalf("%v %v", r, err)
	}

	if b.Values.Fields["ports"].GetListValue().Items[0].GetInt64Value() != 9007199254740993 || b.Values.Fields["limits"].GetStructValue().Fields["cpu"].GetUint64Value() != math.MaxUint64 {
		t.Fatal("numeric precision lost")
	}
}
