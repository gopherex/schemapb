package protomsg

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/gopherex/schemapb/go/schemapb"
)

// Status converts a validation error into a google.rpc.Status with code
// INVALID_ARGUMENT whose single detail is the packed ValidationResult, the
// form gRPC and Connect carry in error details. ok is false when err holds
// no ValidationResult.
func Status(err error) (*status.Status, bool) {
	var res *schemapb.ValidationResult
	if !errors.As(err, &res) {
		return nil, false
	}

	detail, aerr := anypb.New(res)
	if aerr != nil {
		return nil, false
	}

	return &status.Status{
		Code:    int32(code.Code_INVALID_ARGUMENT),
		Message: res.Error(),
		Details: []*anypb.Any{detail},
	}, true
}

// FromStatus extracts the ValidationResult carried in st's details.
func FromStatus(st *status.Status) (*schemapb.ValidationResult, bool) {
	for _, detail := range st.GetDetails() {
		res := &schemapb.ValidationResult{}
		if detail.MessageIs(res) && detail.UnmarshalTo(res) == nil {
			return res, true
		}
	}

	return nil, false
}
