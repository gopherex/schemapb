package protomsg

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/gopherex/schemapb/go/schemapb"
)

// SupportPackageIsVersion1 is referenced by generated code to assert, at
// compile time, that it is compatible with this runtime.
const SupportPackageIsVersion1 = true

// Validator validates messages of one type against the Schema embedded by
// the generator. It compiles the schema once, on first use, and is safe for
// concurrent use.
type Validator struct {
	compiled func() (*schemapb.Engine, error)
	mapping  mapping
}

// New returns a Validator for a binary-encoded Schema. Options configure
// the value mapping (WithAnyTypes).
func New(rawSchema string, opts ...Option) *Validator {
	return &Validator{mapping: newMapping(opts), compiled: sync.OnceValues(func() (*schemapb.Engine, error) {
		schema := &schemapb.Schema{}
		if err := proto.Unmarshal([]byte(rawSchema), schema); err != nil {
			return nil, fmt.Errorf("protomsg: decode embedded schema: %w", err)
		}

		return schemapb.Compile(schema)
	})}
}

// Schema returns the embedded schema. It panics if the embedded schema does
// not compile, which means the generated code and this runtime disagree.
func (v *Validator) Schema() *schemapb.Schema {
	engine, err := v.compiled()
	if err != nil {
		panic(err)
	}

	return engine.Schema()
}

// Check validates msg and returns every failure, warnings included. The
// error is programmatic only: the schema does not compile, or msg holds a
// field the value mapping cannot express.
func (v *Validator) Check(msg proto.Message) (*schemapb.ValidationResult, error) {
	res, _, err := v.check(msg)

	return res, err
}

// check validates msg and also returns the resolved value tree: the engine
// applies defaults, normalize and computed values to it in place.
func (v *Validator) check(msg proto.Message) (*schemapb.ValidationResult, map[string]any, error) {
	engine, err := v.compiled()
	if err != nil {
		return nil, nil, err
	}

	values, err := v.mapping.messageValues(msg.ProtoReflect())
	if err != nil {
		return nil, nil, err
	}

	res := engine.Validate(values)
	res.Schema = engine.Schema().GetId()
	annotateFieldNumbers(msg.ProtoReflect().Descriptor(), res)

	return res, values, nil
}

// Resolve validates msg and, when nothing blocks, writes the resolved
// values (defaults, normalize, computed) back into it. On a blocking
// result msg is left untouched. The error is programmatic (see Check), or
// ErrWriteBack when a resolved value does not fit its field; msg is
// unchanged then too.
func (v *Validator) Resolve(msg proto.Message) (*schemapb.ValidationResult, error) {
	res, values, err := v.check(msg)
	if err != nil || res.Blocking() {
		return res, err
	}

	if err := v.mapping.writeBack(msg, values); err != nil {
		return nil, err
	}

	return res, nil
}

// Validate validates msg. It returns nil when nothing blocks (warnings alone
// do not), the *schemapb.ValidationResult as the error otherwise, or a
// programmatic error (see Check).
func (v *Validator) Validate(msg proto.Message) error {
	res, err := v.Check(msg)
	if err != nil {
		return err
	}

	if res.Blocking() {
		return res
	}

	return nil
}
