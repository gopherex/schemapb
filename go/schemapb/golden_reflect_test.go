package schemapb_test

// The Reflect conformance golden: the MIRROR MODEL. Every language that
// implements native-model reflection (Go reflect, Python pydantic,
// TypeScript zod, Rust derive) declares this same logical model in its own
// type system and must produce the byte-identical Schema below
// (reflect.json). Field order is declaration order and part of the
// contract.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gopherex/schemapb/go/schemapb"
)

// mirrorBase embeds without a json tag: fields flatten.
type mirrorBase struct {
	Base string `json:"base"`
}

type mirrorNested struct {
	On bool `json:"on"`
}

// mirrorNode is self-referential: the cycle degrades to JSON.
type mirrorNode struct {
	Next *mirrorNode `json:"next"`
}

type mirrorModel struct {
	mirrorBase

	Name   string                  `desc:"display name"  json:"name"                validate:"required,min=1,max=64"`
	Mail   string                  `json:"mail"          validate:"email"`
	Slug   string                  `json:"slug"          pattern:"^[a-z-]+$"`
	Mode   string                  `json:"mode"          validate:"oneof=fast safe"`
	Level  int                     `json:"level"         validate:"oneof=1 2 3"`
	Count  int32                   `json:"count"         validate:"gte=0,lte=100"`
	Big    int64                   `json:"big"           validate:"gt=-10,lt=10"`
	Port   uint16                  `json:"port"          validate:"max=65535"`
	Total  uint64                  `json:"total"`
	Ratio  float32                 `json:"ratio"         validate:"gte=0"`
	Score  float64                 `json:"score"         validate:"lt=1"`
	Flag   bool                    `json:"flag"`
	Opt    *string                 `json:"opt,omitempty"`
	Must   *int64                  `json:"must"          validate:"required"`
	Tags   []string                `json:"tags"          validate:"min=1,max=5"`
	Pair   [2]string               `json:"pair"`
	Blob   []byte                  `json:"blob"          validate:"max=16"`
	Magic  [4]byte                 `json:"magic"`
	Limits map[string]int64        `json:"limits"`
	Extra  map[string]mirrorNested `json:"extra"`
	Nested mirrorNested            `json:"nested"`
	When   time.Time               `json:"when"`
	Wait   time.Duration           `json:"wait"`
	Raw    json.RawMessage         `json:"raw"`
	Any    any                     `json:"anything"`
	Chain  mirrorNode              `json:"chain"`
	Gone   string                  `json:"-"`

	hidden string //nolint:unused // unexported fields are skipped
}

func TestGoldenReflect(t *testing.T) {
	t.Parallel()

	s, err := schemapb.ReflectType[mirrorModel](
		schemapb.ID("conformance", "mirror", schemapb.Ver(1, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "reflect.json", stableJSON(t, s))
}
