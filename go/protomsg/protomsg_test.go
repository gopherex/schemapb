package protomsg_test

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/gopherex/schemapb/go/protomsg"
	"github.com/gopherex/schemapb/go/schemapb"
)

const testFile = `
name: "test/v1/test.proto"
package: "test.v1"
syntax: "proto3"
dependency: ["google/protobuf/timestamp.proto", "google/protobuf/duration.proto",
  "google/protobuf/wrappers.proto", "google/protobuf/struct.proto",
  "google/protobuf/field_mask.proto", "google/protobuf/any.proto"]
enum_type { name: "Tier" value { name: "TIER_UNSPECIFIED" number: 0 } value { name: "TIER_GOLD" number: 1 } }
message_type {
  name: "Item"
  field { name: "sku" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING }
}
message_type {
  name: "Order"
  field { name: "id" number: 1 label: LABEL_OPTIONAL type: TYPE_INT32 }
  field { name: "note" number: 2 label: LABEL_OPTIONAL type: TYPE_STRING proto3_optional: true oneof_index: 0 }
  field { name: "tier" number: 3 label: LABEL_OPTIONAL type: TYPE_ENUM type_name: ".test.v1.Tier" }
  field { name: "items" number: 4 label: LABEL_REPEATED type: TYPE_MESSAGE type_name: ".test.v1.Item" }
  field { name: "by_sku" number: 5 label: LABEL_REPEATED type: TYPE_MESSAGE type_name: ".test.v1.Order.BySkuEntry" }
  field { name: "at" number: 6 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Timestamp" }
  field { name: "ttl" number: 7 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Duration" }
  field { name: "limit" number: 8 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.UInt32Value" }
  field { name: "meta" number: 9 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Struct" }
  field { name: "mask" number: 10 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.FieldMask" }
  field { name: "blob" number: 11 label: LABEL_OPTIONAL type: TYPE_BYTES }
  field { name: "any" number: 12 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Any" }
  field { name: "item" number: 13 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".test.v1.Item" }
  nested_type {
    name: "BySkuEntry"
    field { name: "key" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING }
    field { name: "value" number: 2 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".test.v1.Item" }
    options { map_entry: true }
  }
  oneof_decl { name: "_note" }
}
`

//nolint:ireturn // protoreflect exposes descriptors only as interfaces
func orderDesc(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	fdp := &descriptorpb.FileDescriptorProto{}
	if err := prototext.Unmarshal([]byte(testFile), fdp); err != nil {
		t.Fatal(err)
	}

	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}

	return fd.Messages().ByName("Order")
}

func set(m *dynamicpb.Message, name string, v protoreflect.Value) {
	m.Set(m.Descriptor().Fields().ByName(protoreflect.Name(name)), v)
}

func msgV(m proto.Message) protoreflect.Value { return protoreflect.ValueOfMessage(m.ProtoReflect()) }

func item(md protoreflect.MessageDescriptor, sku string) *dynamicpb.Message {
	it := dynamicpb.NewMessage(md)
	set(it, "sku", protoreflect.ValueOfString(sku))

	return it
}

func TestValuesMapping(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemMD := desc.Fields().ByName("items").Message()
	order := dynamicpb.NewMessage(desc)

	set(order, "tier", protoreflect.ValueOfEnum(1))
	list := order.Mutable(desc.Fields().ByName("items")).List()
	list.Append(protoreflect.ValueOfMessage(item(itemMD, "a")))

	mp := order.Mutable(desc.Fields().ByName("by_sku")).Map()
	mp.Set(protoreflect.ValueOfString("k").MapKey(), protoreflect.ValueOfMessage(item(itemMD, "b")))
	set(order, "at", msgV(timestamppb.New(time.Unix(10, 5).UTC())))
	set(order, "ttl", msgV(durationpb.New(1500*time.Millisecond)))
	set(order, "limit", msgV(wrapperspb.UInt32(7)))

	meta, _ := structpb.NewStruct(map[string]any{"a": 1.5, "b": nil, "c": []any{"x"}})
	set(order, "meta", msgV(meta))
	set(order, "mask", msgV(&fieldmaskpb.FieldMask{Paths: []string{"a.b"}}))

	got, err := protomsg.Values(order)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"id":     int64(0),
		"tier":   "TIER_GOLD",
		"items":  []any{map[string]any{"sku": "a"}},
		"by_sku": map[string]any{"k": map[string]any{"sku": "b"}},
		"at":     time.Unix(10, 5).UTC(),
		"ttl":    1500 * time.Millisecond,
		"limit":  uint64(7),
		"meta":   map[string]any{"a": 1.5, "b": nil, "c": []any{"x"}},
		"mask":   []any{"a.b"},
		"blob":   []byte{},
	}

	if !equal(got, want) {
		t.Fatalf("Values:\n got %#v\nwant %#v", got, want)
	}

	set(order, "tier", protoreflect.ValueOfEnum(9))
	set(order, "any", msgV(&anypb.Any{}))

	if _, err := protomsg.Values(order); !errors.Is(err, protomsg.ErrUnsupported) {
		t.Fatalf("Any: want ErrUnsupported, got %v", err)
	}
}

// equal compares through the canonical wire form, which normalizes the
// nil-vs-empty bytes distinction protoreflect does not guarantee.
func equal(a, b map[string]any) bool {
	wa, errA := schemapb.StructFromGo(a)
	wb, errB := schemapb.StructFromGo(b)

	return errA == nil && errB == nil && proto.Equal(wa, wb)
}

// orderSchema is what the generator emits for Order (subset).
func orderSchema(t *testing.T) string {
	t.Helper()

	schema := schemapb.NewSchema(schemapb.ID("test.v1", "Order", schemapb.Version{})).
		Fields(
			schemapb.Int32("id").Gte(1),
			schemapb.Str("note").MinLen(3),
			schemapb.List("items", schemapb.Ref("item", "test.v1.Item")).MinItems(2),
			schemapb.MapOf("by_sku", schemapb.Ref("value", "test.v1.Item")),
			schemapb.Ref("item", "test.v1.Item"),
		).
		Def("test.v1.Item", schemapb.Str("sku").MinLen(2)).
		MustBuild()

	raw, err := proto.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func TestValidatorFieldNumbersAndStatus(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemMD := desc.Fields().ByName("items").Message()
	order := dynamicpb.NewMessage(desc)

	set(order, "note", protoreflect.ValueOfString("x"))
	order.Mutable(desc.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "a")))
	order.Mutable(desc.Fields().ByName("by_sku")).Map().
		Set(protoreflect.ValueOfString("sku").MapKey(), protoreflect.ValueOfMessage(item(itemMD, "b")))
	set(order, "item", protoreflect.ValueOfMessage(item(itemMD, "c")))

	v := protomsg.New(orderSchema(t))

	err := v.Validate(order)

	var res *schemapb.ValidationResult
	if !errors.As(err, &res) {
		t.Fatalf("want *ValidationResult error, got %v", err)
	}

	if res.GetSchema().GetName() != "Order" || res.GetSchema().GetNamespace() != "test.v1" {
		t.Fatalf("schema identity not set: %v", res.GetSchema())
	}

	// path -> field numbers per segment (0 = no number).
	want := map[string][]uint32{
		"id":             {1},
		"note":           {2},
		"items":          {4},
		"items[0].sku":   {4, 0, 1},
		`by_sku.sku.sku`: {5, 0, 1},
		"item.sku":       {13, 1},
	}

	got := map[string][]uint32{}

	for _, e := range res.GetErrors() {
		var nums []uint32
		for _, s := range e.GetPathSegments() {
			nums = append(nums, s.GetFieldNumber())
		}

		got[e.GetPath()] = nums
	}

	for path, nums := range want {
		if !slicesEqual(got[path], nums) {
			t.Errorf("%s: field numbers %v, want %v (all: %v)", path, got[path], nums, got)
		}
	}

	st, ok := protomsg.Status(err)
	if !ok || st.GetCode() != 3 {
		t.Fatalf("Status: %v %v", st, ok)
	}

	back, ok := protomsg.FromStatus(st)
	if !ok || !proto.Equal(back, res) {
		t.Fatalf("FromStatus round trip: %v", back)
	}
}

func slicesEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestValidatorOK(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	order := dynamicpb.NewMessage(desc)
	set(order, "id", protoreflect.ValueOfInt32(1))

	itemMD := desc.Fields().ByName("items").Message()
	items := order.Mutable(desc.Fields().ByName("items")).List()
	items.Append(protoreflect.ValueOfMessage(item(itemMD, "aa")))
	items.Append(protoreflect.ValueOfMessage(item(itemMD, "bb")))

	if err := protomsg.New(orderSchema(t)).Validate(order); err != nil {
		t.Fatal(err)
	}
}
