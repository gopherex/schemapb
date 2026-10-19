// Package protomsg validates typed protobuf messages against a schemapb
// Schema. It is the runtime support of the code protoc-gen-go-schemapb
// generates: the generator translates field options into a Schema at
// generation time, the generated code embeds it and calls a Validator.
//
// A message is presented to the engine as a plain value tree (Values):
//
//	field name        proto field name (not the JSON name)
//	implicit presence always present, zero value included
//	explicit presence present only when set (optional, message, oneof member)
//	repeated / map    always present, possibly empty; map keys as strings
//	enum              value name (string); an unknown number stays int64
//	int32 kinds       int64        uint32 kinds  uint64
//	float / double    float64      bytes         []byte
//	Timestamp         time.Time    Duration      time.Duration
//	wrappers          the wrapped scalar, absent when unset
//	Struct / Value / ListValue     their JSON-shaped value
//	Empty             empty object  FieldMask    list of path strings
//	Any               unsupported (ErrUnsupported)
//
// Error paths name proto fields; each key segment that names a proto field
// carries its wire field number (PathSegment.field_number), and the result
// carries the schema identity, so a client holding only the proto files can
// map every error back to the field it concerns.
package protomsg
