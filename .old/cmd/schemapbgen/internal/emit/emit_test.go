package emit

import (
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

func TestEmitCompiles(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
		schemapb.Enum("level").Values(map[int32]string{0: "minimal", 1: "replica"}).Required(),
		schemapb.Object("wal", schemapb.Bool("enabled").Required()),
	).MustBuild()

	f, err := model.Build(s, "myconfig")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Emit(f)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	src := string(out)
	for _, want := range []string{
		"package myconfig",
		"type InfraDiskV1 struct",
		"SharedBuffers",
		`json:"shared_buffers"`,
		"WalLevel",
		`*string`,
		`json:"wal_level,omitempty"`,
		"type InfraDiskV1_Level int32",
		"type InfraDiskV1_Wal struct",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output missing %q\n---\n%s", want, src)
		}
	}
}

func TestEmitSugar(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, _ := Emit(f)
	src := string(out)
	for _, want := range []string{
		"func (c *InfraDiskV1) GetSharedBuffers() int64",
		"func (c *InfraDiskV1) GetWalLevel() string", // nil-safe deref of *string
		"func NewInfraDiskV1() *InfraDiskV1",
		"func (c *InfraDiskV1) WithSharedBuffers(v int64) *InfraDiskV1",
		"func (c *InfraDiskV1) Clone() *InfraDiskV1",
		"func DefaultInfraDiskV1() *InfraDiskV1",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestEmitSchemaWrap(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, err := Emit(f)
	if err != nil {
		t.Fatal(err)
	}
	src := string(out)
	for _, want := range []string{
		"var _schemaInfraDiskV1Wire = []byte{",
		"func (c *InfraDiskV1) Schema() *schemapb.Schema",
		"func (c *InfraDiskV1) Validate() []*schemapb.FieldError",
		"var InfraDiskV1Identity = &schemapb.SchemaIdentity{",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}
