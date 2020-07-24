package memorystore

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"
	"time"
)

func testKey(tb testing.TB) string {
	tb.Helper()

	var b [512]byte
	if _, err := rand.Read(b[:]); err != nil {
		tb.Fatalf("failed to generate random string: %v", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(b[:]))
	return digest[:32]
}

func TestStore_Take(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tokens   uint64
		interval time.Duration
	}{
		{
			name:     "milli",
			tokens:   5,
			interval: 500 * time.Millisecond,
		},
		{
			name:     "second",
			tokens:   10,
			interval: 1 * time.Second,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key := testKey(t)

			s, err := New(&Config{
				Interval:      tc.interval,
				Tokens:        tc.tokens,
				SweepInterval: 24 * time.Hour,
				SweepMinTTL:   24 * time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()

			type result struct {
				limit, remaining uint64
				reset            time.Duration
				ok               bool
			}

			// Take twice everything from the bucket.
			takeCh := make(chan *result, 2*tc.tokens)
			for i := uint64(1); i <= 2*tc.tokens; i++ {
				go func() {
					limit, remaining, reset, ok := s.Take(key)
					takeCh <- &result{limit, remaining, time.Duration(fastnow() - reset), ok}
				}()
			}

			// Accumulate and sort results, since they could come in any order.
			var results []*result
			for i := uint64(1); i <= 2*tc.tokens; i++ {
				select {
				case result := <-takeCh:
					results = append(results, result)
				case <-time.After(5 * time.Second):
					t.Fatal("timeout")
				}
			}
			sort.Slice(results, func(i, j int) bool {
				if results[i].remaining == results[j].remaining {
					return !results[j].ok
				}
				return results[i].remaining > results[j].remaining
			})

			for i, result := range results {
				if got, want := result.limit, tc.tokens; got != want {
					t.Errorf("limit: expected %d to be %d", got, want)
				}
				if got, want := result.reset, tc.interval; got > want {
					t.Errorf("reset: expected %d to be less than %d", got, want)
				}

				// first half should pass, second half should fail
				if uint64(i) < tc.tokens {
					if got, want := result.remaining, tc.tokens-uint64(i)-1; got != want {
						t.Errorf("remaining: expected %d to be %d", got, want)
					}
					if got, want := result.ok, true; got != want {
						t.Errorf("ok: expected %t to be %t", got, want)
					}
				} else {
					if got, want := result.remaining, uint64(0); got != want {
						t.Errorf("remaining: expected %d to be %d", got, want)
					}
					if got, want := result.ok, false; got != want {
						t.Errorf("ok: expected %t to be %t", got, want)
					}
				}
			}

			// Wait for the bucket to have entries again.
			time.Sleep(tc.interval)

			// Verify we can take once more.
			if _, _, _, ok := s.Take(key); !ok {
				t.Errorf("expected %t to be %t", ok, true)
			}
		})
	}
}

func TestBucketedLimiter_tick(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		start    uint64
		curr     uint64
		interval time.Duration
		exp      uint64
	}{
		{
			name:     "no_diff",
			start:    0,
			curr:     0,
			interval: time.Second,
			exp:      0,
		},
		{
			name:     "half",
			start:    0,
			curr:     uint64(500 * time.Millisecond),
			interval: time.Second,
			exp:      0,
		},
		{
			name:     "almost",
			start:    0,
			curr:     uint64(1*time.Second - time.Nanosecond),
			interval: time.Second,
			exp:      0,
		},
		{
			name:     "exact",
			start:    0,
			curr:     uint64(1 * time.Second),
			interval: time.Second,
			exp:      1,
		},
		{
			name:     "multiple",
			start:    0,
			curr:     uint64(50*time.Second - 500*time.Millisecond),
			interval: time.Second,
			exp:      49,
		},
		{
			name:     "short",
			start:    0,
			curr:     uint64(50*time.Second - 500*time.Millisecond),
			interval: time.Millisecond,
			exp:      49500,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, want := tick(tc.start, tc.curr, tc.interval), tc.exp; got != want {
				t.Errorf("expected %v to be %v", got, want)
			}
		})
	}
}

func TestBucketedLimiter_availableTokens(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		last     uint64
		curr     uint64
		max      uint64
		fillRate float64
		exp      uint64
	}{
		{
			name:     "zero",
			last:     0,
			curr:     0,
			max:      1,
			fillRate: 1.0,
			exp:      0,
		},
		{
			name:     "one",
			last:     0,
			curr:     1,
			max:      1,
			fillRate: 1.0,
			exp:      1,
		},
		{
			name:     "max",
			last:     0,
			curr:     5,
			max:      2,
			fillRate: 1.0,
			exp:      2,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, want := availableTokens(tc.last, tc.curr, tc.max, tc.fillRate), tc.exp; got != want {
				t.Errorf("expected %v to be %v", got, want)
			}
		})
	}
}
