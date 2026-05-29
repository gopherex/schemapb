package emit

import (
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

func TestEmitRoundtripSignatures(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, _ := Emit(f)
	src := string(out)
	for _, want := range []string{
		"func (c *InfraDiskV1) ToValues() (*structpb.Struct, error)",
		"func FromValuesInfraDiskV1(st *structpb.Struct) (*InfraDiskV1, error)",
		"func (c *InfraDiskV1) ToFilled() (*schemapb.Filled, error)",
		"func (c *InfraDiskV1) ToBaked() (*schemapb.Baked, error)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}
