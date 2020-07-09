package benchmarks

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

const (
	// NumSessions is the number of unique sessions to store in the bucket. These
	// are pre-allocated in a map.
	NumSessions = 1000

	// SessionDefaultTokens is the default number of tokens to allow per interval.
	SessionDefaultTokens = 5

	// SessionDefaultInterval is the default ticking interval.
	SessionDefaultInterval = 500 * time.Millisecond

	// SessionMinTTL is the minimum amount of time a session should exist with no
	// requests before cleaning up.
	SessionMinTTL = 100 * time.Nanosecond

	// SessionSweepInterval is the frequency at which sessions should be swept and
	// purged.
	SessionSweepInterval = 1 * time.Millisecond
)

var sessions map[int]string

func init() {
	// Use math/rand since we don't actually need secure crypto here.
	rand.Seed(time.Now().UnixNano())

	data := make([]string, NumSessions)
	for i := 0; i < NumSessions; i++ {
		data[i] = strconv.Itoa(rand.Int())
	}
}

func testSessionID(tb testing.TB, i int) string {
	return sessions[i%NumSessions]
}
