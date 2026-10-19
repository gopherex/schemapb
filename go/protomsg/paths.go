package protomsg

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/gopherex/schemapb/go/schemapb"
)

// annotateFieldNumbers walks each error path over md and records the wire
// field number on every key segment that names a proto field.
func annotateFieldNumbers(md protoreflect.MessageDescriptor, res *schemapb.ValidationResult) {
	for _, err := range res.GetErrors() {
		numberPath(md, err.GetPathSegments())
	}
}

func numberPath(md protoreflect.MessageDescriptor, segments []*schemapb.PathSegment) {
	cur := md // message whose fields the next key names; nil = not a message

	var container protoreflect.FieldDescriptor // list/map whose element is next

	for _, seg := range segments {
		if container != nil {
			// The segment addresses an element: a list index or a map key.
			cur = elementMessage(container)
			container = nil

			continue
		}

		key, ok := seg.GetSegment().(*schemapb.PathSegment_Key)
		if !ok || cur == nil {
			return
		}

		field := cur.Fields().ByName(protoreflect.Name(key.Key))
		if field == nil {
			return
		}

		number := uint32(field.Number()) //nolint:gosec // field numbers are positive and < 2^29
		seg.FieldNumber = &number

		switch {
		case field.IsList() || field.IsMap():
			container, cur = field, nil
		case field.Message() != nil && !isWellKnown(field.Message()):
			cur = field.Message()
		default:
			cur = nil
		}
	}
}

//nolint:ireturn // protoreflect exposes descriptors only as interfaces
func elementMessage(field protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	if field.IsMap() {
		field = field.MapValue()
	}

	if msg := field.Message(); msg != nil && !isWellKnown(msg) {
		return msg
	}

	return nil
}

// isWellKnown reports google.protobuf messages, which map to scalars or
// JSON-shaped values rather than to objects of their fields.
func isWellKnown(md protoreflect.MessageDescriptor) bool {
	return md.ParentFile().Package() == "google.protobuf"
}
