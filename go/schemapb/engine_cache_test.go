// The cache is unexported: these tests count its entries directly.
//
//nolint:testpackage // reaches the unexported engine cache
package schemapb

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

func engineCacheLen() int {
	n := 0

	engineCache.Range(func(any, any) bool {
		n++

		return true
	})

	return n
}

func probeSchema(i int) *Schema {
	return NewSchema(ID("probe", SchemaName(fmt.Sprintf("s%d", i)), MustVersion("v1"))).
		Fields(
			Str("name").Default("x"),
			Int64("n").Gte(0).Rules(Rule("this >= 0", "n must be non-negative")),
			Computed("twice", "root.n * 2").Result(ResultInt64),
		).MustBuild()
}

// The cache lives as long as the schemas it serves: a program that builds
// schemas as it goes must get the memory back, compiled programs included.
//
//nolint:paralleltest // counts the process-wide cache; a parallel test would add to it
func TestEngineCacheFollowsSchemaLifetime(t *testing.T) {
	const n = 200

	before := engineCacheLen()

	for i := range n {
		s := probeSchema(i)
		if _, err := s.Validate(map[string]any{"name": "x", "n": 1}); err != nil {
			t.Fatal(err)
		}
	}

	// Cleanups run after the collector finds the schemas unreachable; give
	// them a few cycles.
	deadline := time.Now().Add(5 * time.Second)
	for engineCacheLen() > before+n/10 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	if got := engineCacheLen(); got > before+n/10 {
		t.Fatalf("engine cache kept %d entries of %d collected schemas", got-before, n)
	}
}

// A live schema keeps one engine: the same pointer compiles once, and the
// cached engine still knows its schema.
func TestEngineCacheReusesTheLiveSchemasEngine(t *testing.T) {
	t.Parallel()

	s := probeSchema(999)

	first, err := s.engine()
	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()

	second, err := s.engine()
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("a live schema was compiled twice")
	}

	if first.Schema() != s {
		t.Fatal("the cached engine lost its live schema")
	}

	runtime.KeepAlive(s)
}
