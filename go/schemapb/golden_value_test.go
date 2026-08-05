package schemapb_test

// Value goldens: the As conversion matrix (value-as.json) and value path
// lookup over the baked kitchen-sink values (value-lookup.json). The
// matrix pins the one shared rule — a conversion succeeds iff the value is
// represented in the target exactly — and lookup pins the error-path
// dialect with (at, segment, reason) failure triples.

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/gopherex/schemapb/go/schemapb"
)

// asCase runs one source value against one target kind.
type asCase struct {
	value  *schemapb.Value
	target schemapb.FieldKind
}

// asGoldenMatrix covers every conversion rule at least once.
func asGoldenMatrix() []asCase {
	deepList := schemapb.ListV(schemapb.StrV("a"), schemapb.Int64V(1))
	structV := schemapb.StructV(map[string]*schemapb.Value{"k": schemapb.BoolV(true)})

	return []asCase{
		// numeric cross-kind, exact
		{schemapb.UInt32V(5), schemapb.KindInt64},
		{schemapb.Int32V(-7), schemapb.KindDouble},
		{schemapb.DoubleV(3.0), schemapb.KindInt64},
		{schemapb.Int64V(42), schemapb.KindDouble},
		{schemapb.Int64V(300), schemapb.KindInt32},
		{schemapb.Int64V(300), schemapb.KindUInt32},
		{schemapb.DoubleV(2.5), schemapb.KindFloat},
		{schemapb.FloatV(2.5), schemapb.KindDouble},
		{schemapb.UInt64V(42), schemapb.KindInt32},
		// numeric cross-kind, lossy -> refused
		{schemapb.UInt64V(math.MaxUint64), schemapb.KindInt64},
		{schemapb.Int64V(-1), schemapb.KindUInt64},
		{schemapb.Int64V(-1), schemapb.KindUInt32},
		{schemapb.DoubleV(3.5), schemapb.KindInt64},
		{schemapb.Int64V(1 << 53), schemapb.KindDouble},   // 2^53 itself round-trips
		{schemapb.Int64V(1<<53 + 1), schemapb.KindDouble}, // 2^53+1 does not
		{schemapb.DoubleV(0.1), schemapb.KindFloat},
		{schemapb.Int64V(math.MaxInt64), schemapb.KindDouble},
		// non-numeric: strict own kind, no parsing
		{schemapb.StrV("5"), schemapb.KindInt64},
		{schemapb.StrV("hello"), schemapb.KindString},
		{schemapb.Int64V(5), schemapb.KindString},
		{schemapb.BoolV(true), schemapb.KindBool},
		{schemapb.BoolV(true), schemapb.KindInt64},
		{schemapb.Int64V(1), schemapb.KindBool},
		{schemapb.BytesV([]byte{0xDE, 0xAD}), schemapb.KindBytes},
		{schemapb.StrV("3q0=" /* base64 of the same */), schemapb.KindBytes},
		{schemapb.DurationV(90 * time.Second), schemapb.KindDuration},
		{schemapb.DurationV(90 * time.Second), schemapb.KindInt64},
		{schemapb.TimestampV(time.Unix(1700000000, 0).UTC()), schemapb.KindTimestamp},
		// null converts to nothing
		{schemapb.NullV(), schemapb.KindInt64},
		{schemapb.NullV(), schemapb.KindString},
		// containers: own kind only
		{deepList, schemapb.KindList},
		{deepList, schemapb.KindStruct},
		{structV, schemapb.KindStruct},
		{structV, schemapb.KindList},
	}
}

// applyAs runs the conversion and re-encodes the result as a wire Value of
// the target kind (or nil when refused).
func applyAs(v *schemapb.Value, target schemapb.FieldKind) *schemapb.Value {
	asWire := func() (*schemapb.Value, bool) {
		//nolint:exhaustive // field-only kinds have no value counterpart
		switch target {
		case schemapb.KindBool:
			if x, ok := schemapb.As[bool](v); ok {
				return schemapb.BoolV(x), true
			}
		case schemapb.KindInt32:
			if x, ok := schemapb.As[int32](v); ok {
				return schemapb.Int32V(x), true
			}
		case schemapb.KindInt64:
			if x, ok := schemapb.As[int64](v); ok {
				return schemapb.Int64V(x), true
			}
		case schemapb.KindUInt32:
			if x, ok := schemapb.As[uint32](v); ok {
				return schemapb.UInt32V(x), true
			}
		case schemapb.KindUInt64:
			if x, ok := schemapb.As[uint64](v); ok {
				return schemapb.UInt64V(x), true
			}
		case schemapb.KindFloat:
			if x, ok := schemapb.As[float32](v); ok {
				return schemapb.FloatV(x), true
			}
		case schemapb.KindDouble:
			if x, ok := schemapb.As[float64](v); ok {
				return schemapb.DoubleV(x), true
			}
		case schemapb.KindString:
			if x, ok := schemapb.As[string](v); ok {
				return schemapb.StrV(x), true
			}
		case schemapb.KindBytes:
			if x, ok := schemapb.As[[]byte](v); ok {
				return schemapb.BytesV(x), true
			}
		case schemapb.KindDuration:
			if x, ok := schemapb.As[time.Duration](v); ok {
				return schemapb.DurationV(x), true
			}
		case schemapb.KindTimestamp:
			if x, ok := schemapb.As[time.Time](v); ok {
				return schemapb.TimestampV(x), true
			}
		case schemapb.KindList:
			if items, ok := v.AsList(); ok {
				return schemapb.ListV(items...), true
			}
		case schemapb.KindStruct:
			if fields, ok := v.AsStruct(); ok {
				return schemapb.StructV(fields), true
			}
		}

		return nil, false
	}

	res, ok := asWire()
	if !ok {
		return nil
	}

	return res
}

func TestGoldenValueAs(t *testing.T) {
	t.Parallel()

	type asGoldenCase struct {
		Value  json.RawMessage `json:"value"`
		Target string          `json:"target"`
		Result json.RawMessage `json:"result,omitempty"`
	}

	marshalValue := func(v *schemapb.Value) json.RawMessage {
		raw, err := protojson.MarshalOptions{}.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}

		return raw
	}

	cases := make([]asGoldenCase, 0, len(asGoldenMatrix()))

	for _, c := range asGoldenMatrix() {
		gc := asGoldenCase{Value: marshalValue(c.value), Target: string(c.target)}
		if res := applyAs(c.value, c.target); res != nil {
			gc.Result = marshalValue(res)
		}

		cases = append(cases, gc)
	}

	out, err := json.MarshalIndent(map[string]any{"cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "value-as.json", append(out, '\n'))
}

// goldenValuePaths is the shared value-lookup case table.
var goldenValuePaths = []string{
	// resolving
	"name",
	"flag",
	"replicas",
	"replicas[0]",
	"replicas[0].name",
	"replicas[0].weight",
	"endpoint_pair[0]",
	"endpoint_pair[1]",
	"tablespaces.main.location",
	"annotations.tier",
	"timeout",
	"magic",
	// failing
	"nope",
	"replicas[1]",
	"replicas[0].nope",
	"name.x",
	"replicas.name",
	"endpoint_pair[0][0]",
	"replicas[x]",
	"a..b",
	"a[0]x",
	"",
}

func TestGoldenValueLookup(t *testing.T) {
	t.Parallel()

	type lookupErr struct {
		At      string `json:"at"`
		Segment string `json:"segment"`
		Reason  string `json:"reason"`
	}

	type lookupCase struct {
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value,omitempty"`
		Error *lookupErr      `json:"error,omitempty"`
	}

	e := goldenEngine(t, goldenSchema(t))

	baked, res, err := e.Bake(validInput())
	if err != nil || len(res.GetErrors()) > 0 {
		t.Fatalf("bake: %v / %v", err, res.GetErrors())
	}

	values := baked.GetValues()
	cases := make([]lookupCase, 0, len(goldenValuePaths))

	for _, path := range goldenValuePaths {
		c := lookupCase{Path: path}

		v, lerr := values.Lookup(path)
		if lerr != nil {
			var vle *schemapb.ValueLookupError
			if !errors.As(lerr, &vle) {
				t.Fatalf("Lookup(%q): non-ValueLookupError %v", path, lerr)
			}

			c.Error = &lookupErr{At: vle.At, Segment: vle.Segment, Reason: string(vle.Reason)}
		} else {
			raw, mErr := protojson.MarshalOptions{}.Marshal(v)
			if mErr != nil {
				t.Fatal(mErr)
			}

			c.Value = raw
		}

		cases = append(cases, c)
	}

	out, err := json.MarshalIndent(map[string]any{"cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "value-lookup.json", append(out, '\n'))
}
