package schemapb_test

// The lookup conformance golden: a fixed table of schema paths resolved
// against the kitchen-sink schema (full-schema.json). Every implementation
// must produce the same kind for resolving paths and the same
// (at, segment, reason) triple for failing ones. Messages are NOT pinned —
// each language words its LookupError idiomatically; the triple is the
// contract.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/gopherex/schemapb/go/schemapb"
)

// goldenLookupPaths is the shared case table (mirrored in every port's
// conformance test).
var goldenLookupPaths = []string{
	// resolving
	"name",
	"flag",
	"logging",
	"logging.collector",
	"replicas",
	"endpoint_pair",
	"tablespaces",
	"backup",
	"data_volume",
	"data_volume.size_gb",
	"primary_endpoint.host",
	"cache",
	"wal_level",
	"timeout",
	// failing
	"nope",
	"logging.nope",
	"replicas.name",
	"tablespaces.location",
	"backup.url",
	"name.x",
	"logging.collector.x",
	"cache.x",
	"",
}

type lookupGoldenCase struct {
	Path  string           `json:"path"`
	Kind  string           `json:"kind,omitempty"`
	Error *lookupGoldenErr `json:"error,omitempty"`
}

type lookupGoldenErr struct {
	At      string `json:"at"`
	Segment string `json:"segment"`
	Reason  string `json:"reason"`
}

func TestGoldenLookup(t *testing.T) {
	t.Parallel()

	s := goldenSchema(t)
	cases := make([]lookupGoldenCase, 0, len(goldenLookupPaths))

	for _, path := range goldenLookupPaths {
		c := lookupGoldenCase{Path: path}

		f, err := s.LookupPath(path)
		if err != nil {
			var lerr *schemapb.LookupError
			if !errors.As(err, &lerr) {
				t.Fatalf("LookupPath(%q): non-LookupError %v", path, err)
			}

			c.Error = &lookupGoldenErr{
				At:      lerr.At,
				Segment: string(lerr.Segment),
				Reason:  string(lerr.Reason),
			}
		} else {
			c.Kind = string(schemapb.KindName(f))
		}

		cases = append(cases, c)
	}

	out, err := json.MarshalIndent(map[string]any{"cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	checkGolden(t, "lookup.json", append(out, '\n'))
}
