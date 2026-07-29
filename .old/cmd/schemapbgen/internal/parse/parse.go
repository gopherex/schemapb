// Package parse loads a schemapb.Schema from protojson.
package parse

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
)

// FromJSONFile reads a protojson-encoded Schema from path.
func FromJSONFile(path string) (*schemapb.Schema, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}
