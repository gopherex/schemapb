package schemapb_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	schemapb "github.com/gopherex/schemapb/go/schemapb"
)

// full-coverage.json is the structural companion of the kitchen-sink golden:
// a document in which EVERY field of EVERY message of the proto contract is
// populated (both arms of each oneof across instances). It is not a valid
// runnable schema — constraints deliberately conflict — it exists so that
// every port's protoJSON decoder sees every field spelled out, and so the
// reflection test below fails the moment a new proto field is added without
// extending the goldens.

// u64 / i64 / i32 / f32 / f64 / b / str return pointers for optional scalars.
func ptr[T any](v T) *T { return &v }

// coverageSchema populates every Schema / Field / kind message field.
func coverageSchema() *schemapb.Schema {
	dur := durationpb.New(1500 * time.Millisecond)
	ts := timestamppb.New(fixedInstant())

	return &schemapb.Schema{
		Id: &schemapb.SchemaIdentity{
			Namespace: "conformance",
			Name:      "field_coverage",
			Version:   "v9.9.9-rc.1",
		},
		Description:   ptr("every proto field populated"),
		Strict:        true,
		Coerce:        true,
		MinProperties: ptr(uint64(0)),
		MaxProperties: ptr(uint64(99)),
		Templates:     map[string]string{"tpl": "{{name}}"},
		Defs: map[string]*schemapb.Schema{
			"def_a": {
				Id:     &schemapb.SchemaIdentity{Name: "def_a"},
				Fields: []*schemapb.Schema_Field{{Name: "x", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}}},
			},
		},
		Rules: []*schemapb.Schema_Field_Rule{{
			Expr:     "true",
			Message:  "form rule",
			Id:       ptr("form-rule"),
			Severity: ptr(schemapb.SeverityWarning),
		}},
		Fields: []*schemapb.Schema_Field{
			{
				Name:        "float_all",
				Description: ptr("float with every constraint"),
				Nullable:    true,
				Required:    true,
				Immutable:   true,
				Group:       ptr("grp"),
				Unit:        ptr("MB"),
				Title:       ptr("Float"),
				Deprecated:  true,
				Secret:      true,
				Normalize:   ptr("this"),
				When:        ptr("true"),
				Examples:    []*schemapb.Value{schemapb.FloatV(1)},
				Rules: []*schemapb.Schema_Field_Rule{{
					Expr: "true", Message: "field rule", Id: ptr("fr"), Severity: ptr(schemapb.SeverityError),
				}},
				Kind: &schemapb.Schema_Field_Float_{Float: &schemapb.Schema_Field_Float{
					Default: ptr(float32(1)), Const: ptr(float32(2)), Gt: ptr(float32(3)),
					Gte: ptr(float32(4)), Lt: ptr(float32(5)), Lte: ptr(float32(6)),
					In: []float32{7}, NotIn: []float32{8}, MultipleOf: ptr(float32(9)),
				}},
			},
			{Name: "double_all", Kind: &schemapb.Schema_Field_Double_{Double: &schemapb.Schema_Field_Double{
				Default: ptr(1.0), Const: ptr(2.0), Gt: ptr(3.0), Gte: ptr(4.0), Lt: ptr(5.0), Lte: ptr(6.0),
				In: []float64{7}, NotIn: []float64{8}, MultipleOf: ptr(9.0),
			}}},
			{Name: "int32_all", Kind: &schemapb.Schema_Field_Int32_{Int32: &schemapb.Schema_Field_Int32{
				Default: ptr(int32(1)), Const: ptr(int32(2)), Gt: ptr(int32(3)), Gte: ptr(int32(4)),
				Lt: ptr(int32(5)), Lte: ptr(int32(6)), In: []int32{7}, NotIn: []int32{8}, MultipleOf: ptr(int32(9)),
			}}},
			{Name: "int64_all", Kind: &schemapb.Schema_Field_Int64_{Int64: &schemapb.Schema_Field_Int64{
				Default: ptr(int64(1)), Const: ptr(int64(2)), Gt: ptr(int64(3)), Gte: ptr(int64(4)),
				Lt: ptr(int64(5)), Lte: ptr(int64(6)), In: []int64{7}, NotIn: []int64{8}, MultipleOf: ptr(int64(9)),
			}}},
			{Name: "uint32_all", Kind: &schemapb.Schema_Field_Uint32{Uint32: &schemapb.Schema_Field_UInt32{
				Default: ptr(uint32(1)), Const: ptr(uint32(2)), Gt: ptr(uint32(3)), Gte: ptr(uint32(4)),
				Lt: ptr(uint32(5)), Lte: ptr(uint32(6)), In: []uint32{7}, NotIn: []uint32{8}, MultipleOf: ptr(uint32(9)),
			}}},
			{Name: "uint64_all", Kind: &schemapb.Schema_Field_Uint64{Uint64: &schemapb.Schema_Field_UInt64{
				Default: ptr(uint64(1)), Const: ptr(uint64(2)), Gt: ptr(uint64(3)), Gte: ptr(uint64(4)),
				Lt: ptr(uint64(5)), Lte: ptr(uint64(6)), In: []uint64{7}, NotIn: []uint64{8}, MultipleOf: ptr(uint64(9)),
			}}},
			{Name: "bool_all", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{
				Default: ptr(true), Const: ptr(false),
			}}},
			{Name: "string_all", Kind: &schemapb.Schema_Field_String_{String_: &schemapb.Schema_Field_String{
				Default: ptr("d"), Const: ptr("c"), Len: ptr(uint64(1)), MinLen: ptr(uint64(2)),
				MaxLen: ptr(uint64(3)), Pattern: ptr("^x$"), In: []string{"a"}, NotIn: []string{"b"},
				Format: ptr("email"),
			}}},
			{Name: "bytes_all", Kind: &schemapb.Schema_Field_Bytes_{Bytes: &schemapb.Schema_Field_Bytes{
				Default: []byte{1}, Const: []byte{2}, Len: ptr(uint64(1)), MinLen: ptr(uint64(2)),
				MaxLen: ptr(uint64(3)), Prefix: []byte{3}, Suffix: []byte{4},
				In: [][]byte{{5}}, NotIn: [][]byte{{6}},
			}}},
			{Name: "choice_all", Kind: &schemapb.Schema_Field_Choice_{Choice: &schemapb.Schema_Field_Choice{
				Options: []*schemapb.Schema_Field_Choice_Option{{
					Value:       schemapb.StrV("opt"),
					Label:       "Opt",
					Description: "an option",
					Deprecated:  true,
				}},
				Default:     schemapb.StrV("opt"),
				Open:        true,
				OptionsExpr: ptr(`["opt"]`),
			}}},
			{Name: "duration_all", Kind: &schemapb.Schema_Field_Duration_{Duration: &schemapb.Schema_Field_Duration{
				Default: dur, Gt: dur, Gte: dur, Lt: dur, Lte: dur,
			}}},
			{Name: "timestamp_all", Kind: &schemapb.Schema_Field_Timestamp_{Timestamp: &schemapb.Schema_Field_Timestamp{
				Default: ts, Gt: ts, Gte: ts, Lt: ts, Lte: ts,
			}}},
			{Name: "list_all", Kind: &schemapb.Schema_Field_List_{List: &schemapb.Schema_Field_List{
				Items: []*schemapb.Schema_Field{
					{Name: "item", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}},
				},
				MinItems: ptr(uint64(1)), MaxItems: ptr(uint64(2)), Unique: true, CountExpr: ptr("1"),
			}}},
			{Name: "object_all", Kind: &schemapb.Schema_Field_Object_{Object: &schemapb.Schema_Field_Object{
				Schema: &schemapb.Schema{
					Id:     &schemapb.SchemaIdentity{Name: "nested"},
					Fields: []*schemapb.Schema_Field{{Name: "y", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}}},
				},
			}}},
			{Name: "map_all", Kind: &schemapb.Schema_Field_Map_{Map: &schemapb.Schema_Field_Map{
				ValueSchema: &schemapb.Schema{
					Id:     &schemapb.SchemaIdentity{Name: "map_value"},
					Fields: []*schemapb.Schema_Field{{Name: "z", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}}},
				},
				MinEntries: ptr(uint64(1)), MaxEntries: ptr(uint64(2)),
			}}},
			{Name: "oneof_all", Kind: &schemapb.Schema_Field_OneOf_{OneOf: &schemapb.Schema_Field_OneOf{
				Discriminator: "type",
				Variants: map[string]*schemapb.Schema{
					"v1": {
						Id:     &schemapb.SchemaIdentity{Name: "variant"},
						Fields: []*schemapb.Schema_Field{{Name: "w", Kind: &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}}},
					},
				},
			}}},
			{Name: "ref_by_name", Kind: &schemapb.Schema_Field_Ref_{Ref: &schemapb.Schema_Field_Ref{
				Target: &schemapb.Schema_Field_Ref_Name{Name: "def_a"},
			}}},
			{Name: "ref_by_id", Kind: &schemapb.Schema_Field_Ref_{Ref: &schemapb.Schema_Field_Ref{
				Target: &schemapb.Schema_Field_Ref_Id{Id: &schemapb.SchemaIdentity{
					Namespace: "conformance", Name: "def_a", Version: "v1.0.0",
				}},
			}}},
			{Name: "computed_all", Kind: &schemapb.Schema_Field_Computed_{Computed: &schemapb.Schema_Field_Computed{
				Expr:   "1",
				Result: ptr(schemapb.ResultInt64),
			}}},
			{Name: "json_all", Kind: &schemapb.Schema_Field_Json_{Json: &schemapb.Schema_Field_Json{
				Default: schemapb.NullV(),
			}}},
		},
	}
}

// coverageAllValues is one list containing every Value variant.
func coverageAllValues() *schemapb.ListValue {
	return &schemapb.ListValue{Items: []*schemapb.Value{
		schemapb.NullV(),
		schemapb.BoolV(true),
		schemapb.Int32V(1),
		schemapb.Int64V(2),
		schemapb.UInt32V(3),
		schemapb.UInt64V(4),
		schemapb.FloatV(1.5),
		schemapb.DoubleV(2.5),
		schemapb.StrV("s"),
		schemapb.BytesV([]byte{0xFF}),
		schemapb.DurationV(1500 * time.Millisecond),
		schemapb.TimestampV(fixedInstant()),
		schemapb.ListV(schemapb.BoolV(false)),
		schemapb.StructV(map[string]*schemapb.Value{"k": schemapb.NullV()}),
	}}
}

// coverageDoc lists every top-level message of the coverage document in a
// fixed order (both SchemaRef arms via the two Filled instances).
func coverageDoc() []struct {
	Key string
	Msg proto.Message
} {
	schema := coverageSchema()
	values := &schemapb.StructValue{Fields: map[string]*schemapb.Value{"bool_all": schemapb.BoolV(true)}}

	return []struct {
		Key string
		Msg proto.Message
	}{
		{"schema", schema},
		{"allValues", coverageAllValues()},
		{"filledInline", &schemapb.Filled{
			Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: coverageSchema()}},
			Values: values,
		}},
		{"filledById", &schemapb.Filled{
			Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Id{Id: schema.GetId()}},
			Values: values,
		}},
		{"baked", &schemapb.Baked{Schema: coverageSchema(), Values: values}},
		{"validationResult", &schemapb.ValidationResult{Errors: []*schemapb.ValidationError{{
			Path:       "float_all",
			Code:       schemapb.ErrorCode_ERROR_CODE_GTE_VIOLATED,
			Expected:   schemapb.FloatV(4),
			Actual:     schemapb.FloatV(1),
			Constraint: "gte",
			Expr:       "true",
			RuleId:     ptr("fr"),
			Severity:   schemapb.SeverityWarning,
			Message:    "must be >= 4",
		}}}},
	}
}

// fixedInstant is the single fixed timestamp of the coverage document.
func fixedInstant() time.Time {
	return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
}

func TestGoldenFieldCoverage(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	buf.WriteString("{\n")

	doc := coverageDoc()
	for i, entry := range doc {
		raw, err := protojson.Marshal(entry.Msg)
		if err != nil {
			t.Fatal(err)
		}

		var indented bytes.Buffer
		if err := json.Indent(&indented, raw, "  ", "  "); err != nil {
			t.Fatal(err)
		}

		buf.WriteString("  \"" + entry.Key + "\": ")
		buf.Write(indented.Bytes())

		if i < len(doc)-1 {
			buf.WriteString(",")
		}

		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	checkGolden(t, "full-coverage.json", buf.Bytes())
}

// TestProtoFieldCoverage walks every message descriptor of the contract and
// fails if any field is not populated somewhere in the coverage document: a
// new proto field cannot land without extending the goldens.
func TestProtoFieldCoverage(t *testing.T) {
	t.Parallel()

	want := map[string]bool{}

	var collect func(protoreflect.MessageDescriptors)

	collect = func(msgs protoreflect.MessageDescriptors) {
		for i := range msgs.Len() {
			m := msgs.Get(i)
			if m.IsMapEntry() {
				continue
			}

			for j := range m.Fields().Len() {
				want[string(m.Fields().Get(j).FullName())] = false
			}

			collect(m.Messages())
		}
	}

	for _, fileDesc := range []protoreflect.FileDescriptor{
		schemapb.File_schemapb_value_proto.ParentFile(),
		schemapb.File_schemapb_schema_proto.ParentFile(),
		schemapb.File_schemapb_errors_proto.ParentFile(),
		schemapb.File_schemapb_runtime_proto.ParentFile(),
	} {
		collect(fileDesc.Messages())
	}

	var mark func(m protoreflect.Message)

	mark = func(m protoreflect.Message) {
		m.Range(func(fieldDesc protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			want[string(fieldDesc.FullName())] = true

			switch {
			case fieldDesc.IsMap():
				mapVal := fieldDesc.MapValue()
				if mapVal.Kind() == protoreflect.MessageKind {
					v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
						mark(mv.Message())

						return true
					})
				}
			case fieldDesc.IsList() && fieldDesc.Kind() == protoreflect.MessageKind:
				list := v.List()
				for i := range list.Len() {
					mark(list.Get(i).Message())
				}
			case fieldDesc.Kind() == protoreflect.MessageKind:
				mark(v.Message())
			}

			return true
		})
	}

	for _, entry := range coverageDoc() {
		mark(entry.Msg.ProtoReflect())
	}

	var missing []string

	for name, seen := range want {
		if !seen {
			missing = append(missing, name)
		}
	}

	slices.Sort(missing)

	if len(missing) > 0 {
		t.Errorf("proto fields not populated in full-coverage.json:\n  %v", missing)
	}
}
