package protomsg_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/gopherex/schemapb/go/protomsg"
)

func TestDurationRange(t *testing.T) {
	t.Parallel()

	const maxSeconds = math.MaxInt64 / int64(time.Second)

	maxNanos := math.MaxInt64 % int64(time.Second)

	cases := []struct {
		seconds, nanos int64
		ok             bool
	}{
		{1, 500, true},
		{-1, -500, true},
		{maxSeconds, maxNanos, true},
		{maxSeconds, maxNanos + 1, false},
		{-maxSeconds, -maxNanos - 1, true}, // exactly math.MinInt64
		{-maxSeconds, -maxNanos - 2, false},
		{maxSeconds + 1, 0, false},
		{1, -1, false},
		{-1, 1, false},
		{0, int64(time.Second), false},
	}

	for _, tc := range cases {
		d, err := protomsg.Duration(tc.seconds, tc.nanos)
		if tc.ok != (err == nil) {
			t.Errorf("duration(%d, %d) = %v, %v; want ok=%v", tc.seconds, tc.nanos, d, err, tc.ok)
		}

		if err != nil && !errors.Is(err, protomsg.ErrOutOfRange) {
			t.Errorf("duration(%d, %d): %v is not ErrOutOfRange", tc.seconds, tc.nanos, err)
		}

		if err == nil && d != time.Duration(tc.seconds)*time.Second+time.Duration(tc.nanos) {
			t.Errorf("duration(%d, %d) = %v", tc.seconds, tc.nanos, d)
		}
	}
}
