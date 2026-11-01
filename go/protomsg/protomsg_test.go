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

//nolint:ireturn,nolintlint // protoreflect exposes descriptors only as interfaces
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

	set(order, "ttl", msgV(&durationpb.Duration{Seconds: 1 << 62}))

	if _, err := protomsg.Values(order); !errors.Is(err, protomsg.ErrOutOfRange) {
		t.Fatalf("huge Duration: want ErrOutOfRange, got %v", err)
	}

	set(order, "ttl", msgV(durationpb.New(time.Second)))
	set(order, "tier", protoreflect.ValueOfEnum(9))

	packed := item(itemMD, "inner")

	raw, err := proto.Marshal(packed)
	if err != nil {
		t.Fatal(err)
	}

	set(order, "any", msgV(&anypb.Any{TypeUrl: "type.googleapis.com/test.v1.Item", Value: raw}))

	got, err = protomsg.Values(order)
	if err != nil {
		t.Fatal(err)
	}

	if a, _ := got["any"].(map[string]any); len(a) != 1 || a["@type"] != "type.googleapis.com/test.v1.Item" {
		t.Fatalf("Any of an unlisted type: got %v, want only @type", got["any"])
	}

	got, err = protomsg.Values(order, protomsg.WithAnyTypes(dynamicpb.NewMessageType(itemMD)))
	if err != nil {
		t.Fatal(err)
	}

	if a, _ := got["any"].(map[string]any); a["sku"] != "inner" || a["@type"] != "type.googleapis.com/test.v1.Item" {
		t.Fatalf("Any of a listed type: got %v", got["any"])
	}
}

// resolveSchema derives values: a default, nested normalize, a computed id.
func resolveSchema(t *testing.T, computed string) string {
	t.Helper()

	schema := schemapb.NewSchema(schemapb.ID("test.v1", "Order", schemapb.Version{})).
		Fields(
			schemapb.Computed("id", computed).Result(schemapb.ResultInt64),
			schemapb.Str("note").Default("n/a"),
			schemapb.List("items", schemapb.Ref("item", "test.v1.Item")),
			schemapb.MapOf("by_sku", schemapb.Ref("value", "test.v1.Item")),
			schemapb.Duration("ttl").Default(30*time.Second),
		).
		Def("test.v1.Item", schemapb.Str("sku").MinLen(1).Normalize("this.upperAscii()")).
		MustBuild()

	raw, err := proto.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func TestResolveWritesBack(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemMD := desc.Fields().ByName("items").Message()
	order := dynamicpb.NewMessage(desc)
	order.Mutable(desc.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "a")))
	order.Mutable(desc.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "b")))
	order.Mutable(desc.Fields().ByName("by_sku")).Map().
		Set(protoreflect.ValueOfString("k").MapKey(), protoreflect.ValueOfMessage(item(itemMD, "c")))

	res, err := protomsg.New(resolveSchema(t, "size(root.items)")).Resolve(order)
	if err != nil || res.Blocking() {
		t.Fatalf("Resolve: %v %v", err, res.GetErrors())
	}

	get := func(name string) protoreflect.Value { return order.Get(desc.Fields().ByName(protoreflect.Name(name))) }

	if get("id").Int() != 2 || get("note").String() != "n/a" {
		t.Errorf("computed/default not written: id=%v note=%q", get("id"), get("note").String())
	}

	if sku := get("items").List().Get(1).Message().Get(itemMD.Fields().ByName("sku")).String(); sku != "B" {
		t.Errorf("list item normalize not written: %q", sku)
	}

	entry := get("by_sku").Map().Get(protoreflect.ValueOfString("k").MapKey()).Message()
	if sku := entry.Get(itemMD.Fields().ByName("sku")).String(); sku != "C" {
		t.Errorf("map value normalize not written: %q", sku)
	}

	ttl := get("ttl").Message()
	if ttl.Get(ttl.Descriptor().Fields().ByName("seconds")).Int() != 30 {
		t.Errorf("duration default not written: %v", ttl)
	}
}

func TestResolveLeavesMessageOnFailure(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemMD := desc.Fields().ByName("items").Message()

	blocked := dynamicpb.NewMessage(desc)
	blocked.Mutable(desc.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "")))
	before := proto.Clone(blocked)

	res, err := protomsg.New(resolveSchema(t, "size(root.items)")).Resolve(blocked)
	if err != nil || !res.Blocking() {
		t.Fatalf("want a blocking result, got %v %v", err, res.GetErrors())
	}

	if !proto.Equal(before, blocked) {
		t.Error("a blocking Resolve modified the message")
	}

	misfit := dynamicpb.NewMessage(desc)
	misfit.Mutable(desc.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "a")))
	before = proto.Clone(misfit)

	// 2^40 does not fit the int32 id.
	if _, err := protomsg.New(resolveSchema(t, "1099511627776")).Resolve(misfit); !errors.Is(err, protomsg.ErrWriteBack) {
		t.Fatalf("want ErrWriteBack, got %v", err)
	}

	if !proto.Equal(before, misfit) {
		t.Error("a failed write-back modified the message")
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

// fullNative is every native kind Order can hold, as Values emits it
// (the Any entry named by protomsg.AnyTypeKey).
func fullNative() map[string]any {
	return map[string]any{
		"id":     int64(1),
		"note":   "abc",
		"tier":   "TIER_GOLD",
		"items":  []any{map[string]any{"sku": "a"}},
		"by_sku": map[string]any{"k": map[string]any{"sku": "b"}},
		"at":     time.Unix(10, 5).UTC(),
		"ttl":    1500 * time.Millisecond,
		"limit":  uint64(7),
		"meta":   map[string]any{"a": 1.5, "b": nil, "c": []any{"x"}},
		"mask":   []any{"a.b"},
		"blob":   []byte{1, 2},
		"item":   map[string]any{"sku": "c"},
		"any":    map[string]any{protomsg.AnyTypeKey: "type.googleapis.com/test.v1.Item", "sku": "inner"},
	}
}

func fullOrder(t *testing.T, desc protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()

	itemMD := desc.Fields().ByName("items").Message()
	order := dynamicpb.NewMessage(desc)

	set(order, "id", protoreflect.ValueOfInt32(1))
	set(order, "note", protoreflect.ValueOfString("abc"))
	set(order, "tier", protoreflect.ValueOfEnum(1))

	list := order.Mutable(desc.Fields().ByName("items")).List()
	list.Append(protoreflect.ValueOfMessage(item(itemMD, "a")))

	order.Mutable(desc.Fields().ByName("by_sku")).Map().
		Set(protoreflect.ValueOfString("k").MapKey(), protoreflect.ValueOfMessage(item(itemMD, "b")))

	set(order, "at", msgV(timestamppb.New(time.Unix(10, 5).UTC())))
	set(order, "ttl", msgV(durationpb.New(1500*time.Millisecond)))
	set(order, "limit", msgV(wrapperspb.UInt32(7)))

	meta, err := structpb.NewStruct(map[string]any{"a": 1.5, "b": nil, "c": []any{"x"}})
	if err != nil {
		t.Fatal(err)
	}

	set(order, "meta", msgV(meta))
	set(order, "mask", msgV(&fieldmaskpb.FieldMask{Paths: []string{"a.b"}}))
	set(order, "blob", protoreflect.ValueOfBytes([]byte{1, 2}))
	set(order, "item", msgV(item(itemMD, "c")))

	packed, err := proto.Marshal(item(itemMD, "inner"))
	if err != nil {
		t.Fatal(err)
	}

	set(order, "any", msgV(&anypb.Any{TypeUrl: "type.googleapis.com/test.v1.Item", Value: packed}))

	return order
}

func TestWriteBackBuildsFromNative(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemType := dynamicpb.NewMessageType(desc.Fields().ByName("items").Message())

	order := dynamicpb.NewMessage(desc)
	if err := protomsg.WriteBack(order, fullNative(), protomsg.WithAnyTypes(itemType)); err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(order, fullOrder(t, desc)) {
		t.Fatalf("WriteBack:\n got %v", order)
	}

	// The round trip: Values then WriteBack is the identity.
	values, err := protomsg.Values(fullOrder(t, desc), protomsg.WithAnyTypes(itemType))
	if err != nil {
		t.Fatal(err)
	}

	back := dynamicpb.NewMessage(desc)
	if err := protomsg.WriteBack(back, values, protomsg.WithAnyTypes(itemType)); err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(back, fullOrder(t, desc)) {
		t.Fatalf("round trip:\n got %v", back)
	}
}

func TestWriteBackMergesIntoAnExistingMessage(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	fields := desc.Fields()
	itemMD := fields.ByName("items").Message()
	itemType := dynamicpb.NewMessageType(itemMD)

	order := dynamicpb.NewMessage(desc)
	set(order, "id", protoreflect.ValueOfInt32(7))
	order.Mutable(fields.ByName("items")).List().Append(protoreflect.ValueOfMessage(item(itemMD, "keep")))

	packed, err := proto.Marshal(item(itemMD, "old"))
	if err != nil {
		t.Fatal(err)
	}

	set(order, "any", msgV(&anypb.Any{TypeUrl: "type.googleapis.com/test.v1.Item", Value: packed}))

	if err := protomsg.WriteBack(order, map[string]any{
		"note":   "new",
		"tier":   int64(1), // integers are accepted as in the TS bridge
		"items":  []any{map[string]any{"sku": "z"}},
		"by_sku": map[string]any{"k": map[string]any{"sku": "y"}},
		"any":    map[string]any{protomsg.AnyTypeKey: "type.googleapis.com/test.v1.Item", "sku": "rewritten"},
	}, protomsg.WithAnyTypes(itemType)); err != nil {
		t.Fatal(err)
	}

	if order.Get(fields.ByName("id")).Int() != 7 {
		t.Error("a key absent from values changed the field")
	}

	if order.Get(fields.ByName("note")).String() != "new" || order.Get(fields.ByName("tier")).Enum() != 1 {
		t.Errorf("note/tier not written: %v %v", order.Get(fields.ByName("note")), order.Get(fields.ByName("tier")))
	}

	rewritten := itemType.New()
	anyMsg := order.Get(fields.ByName("any")).Message()

	if err := proto.Unmarshal(anyMsg.Get(anyMsg.Descriptor().Fields().ByName("value")).Bytes(), rewritten.Interface()); err != nil {
		t.Fatal(err)
	}

	if rewritten.Get(itemMD.Fields().ByName("sku")).String() != "rewritten" {
		t.Errorf("Any not repacked: %v", rewritten)
	}

	if err := protomsg.WriteBack(order, map[string]any{"note": nil}); err != nil {
		t.Fatal(err)
	}

	if order.Has(fields.ByName("note")) {
		t.Error("null did not clear the field")
	}
}

func TestWriteBackMisfitAndUnlistedAny(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	fields := desc.Fields()

	order := dynamicpb.NewMessage(desc)
	set(order, "id", protoreflect.ValueOfInt32(7))
	before := proto.Clone(order)

	// 2^40 does not fit the int32 id: nothing is stored.
	if err := protomsg.WriteBack(order, map[string]any{"id": int64(1) << 40, "note": "x"}); !errors.Is(err, protomsg.ErrWriteBack) {
		t.Fatalf("want ErrWriteBack, got %v", err)
	}

	if !proto.Equal(before, order) {
		t.Error("a failed write-back modified the message")
	}

	// An unlisted Any type was seen only by its "@type": the field is left as is.
	if err := protomsg.WriteBack(order, map[string]any{
		"any": map[string]any{protomsg.AnyTypeKey: "type.googleapis.com/test.v1.Item", "sku": "no"},
	}); err != nil {
		t.Fatal(err)
	}

	if order.Has(fields.ByName("any")) {
		t.Error("an unlisted Any set the field")
	}
}

func TestResolveRepacksAny(t *testing.T) {
	t.Parallel()

	desc := orderDesc(t)
	itemMD := desc.Fields().ByName("items").Message()
	itemType := dynamicpb.NewMessageType(itemMD)

	schema := schemapb.NewSchema(schemapb.ID("test.v1", "Order", schemapb.Version{})).
		Fields(schemapb.OneOf("any", "@type").
			Variant("type.googleapis.com/test.v1.Item", schemapb.Str("sku").Pattern("^[A-Z]+$").Normalize("this.upperAscii()"))).
		MustBuild()

	raw, err := proto.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	packed, err := proto.Marshal(item(itemMD, "abc"))
	if err != nil {
		t.Fatal(err)
	}

	order := dynamicpb.NewMessage(desc)
	set(order, "any", msgV(&anypb.Any{TypeUrl: "type.googleapis.com/test.v1.Item", Value: packed}))

	res, err := protomsg.New(string(raw), protomsg.WithAnyTypes(itemType)).Resolve(order)
	if err != nil || res.Blocking() {
		t.Fatalf("Resolve: %v %v", err, res.GetErrors())
	}

	anyMsg := order.Get(desc.Fields().ByName("any")).Message()
	stored := itemType.New()

	if err := proto.Unmarshal(anyMsg.Get(anyMsg.Descriptor().Fields().ByName("value")).Bytes(), stored.Interface()); err != nil {
		t.Fatal(err)
	}

	// The validated (normalized) value is the stored one.
	if sku := stored.Get(itemMD.Fields().ByName("sku")).String(); sku != "ABC" {
		t.Fatalf("Any not repacked: stored sku %q", sku)
	}
}
