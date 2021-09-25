package memorystore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/sethvargo/go-limiter/internal/fasttime"
)

func TestFillRate(t *testing.T) {
	s, _ := New(&Config{
		Tokens:   65535,
		Interval: time.Second,
	})

	for i := 0; i < 20; i++ {
		limit, remaining, _, _, _ := s.Take(context.Background(), "asd")
		if remaining < limit-uint64(i)-1 {
			t.Errorf("invalid remaining: run: %d limit: %d remaining: %d", i, limit, remaining)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func testKey(tb testing.TB) string {
	tb.Helper()

	var b [512]byte
	if _, err := rand.Read(b[:]); err != nil {
		tb.Fatalf("failed to generate random string: %v", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(b[:]))
	return digest[:32]
}

func TestStore_Exercise(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s, err := New(&Config{
		Tokens:        5,
		Interval:      3 * time.Second,
		SweepInterval: 24 * time.Hour,
		SweepMinTTL:   24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(ctx); err != nil {
			t.Fatal(err)
		}
	})

	key := testKey(t)

	// Get when no config exists
	{
		limit, remaining, err := s.(*store).Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}

		if got, want := limit, uint64(0); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(0); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
	}

	// Take with no key configuration - this should use the default values
	{
		limit, remaining, reset, ok, err := s.Take(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("expected ok")
		}
		if got, want := limit, uint64(5); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(4); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := time.Until(time.Unix(0, int64(reset))), 3*time.Second; got > want {
			t.Errorf("expected %v to less than %v", got, want)
		}
	}

	// Get the value
	{
		limit, remaining, err := s.(*store).Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := limit, uint64(5); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(4); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
	}

	// Now set a value
	{
		if err := s.Set(ctx, key, 11, 5*time.Second); err != nil {
			t.Fatal(err)
		}
	}

	// Get the value again
	{
		limit, remaining, err := s.(*store).Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := limit, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
	}

	// Take again, this should use the new values
	{
		limit, remaining, reset, ok, err := s.Take(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("expected ok")
		}
		if got, want := limit, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(10); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := time.Until(time.Unix(0, int64(reset))), 5*time.Second; got > want {
			t.Errorf("expected %v to less than %v", got, want)
		}
	}

	// Get the value again
	{
		limit, remaining, err := s.(*store).Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := limit, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(10); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
	}

	// Burst and take
	{
		if err := s.Burst(ctx, key, 5); err != nil {
			t.Fatal(err)
		}

		limit, remaining, reset, ok, err := s.Take(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("expected ok")
		}
		if got, want := limit, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(14); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := time.Until(time.Unix(0, int64(reset))), 5*time.Second; got > want {
			t.Errorf("expected %v to less than %v", got, want)
		}
	}

	// Get the value one final time
	{
		limit, remaining, err := s.(*store).Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := limit, uint64(11); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
		if got, want := remaining, uint64(14); got != want {
			t.Errorf("expected %v to be %v", got, want)
		}
	}
}

func TestStore_Take(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

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
			t.Cleanup(func() {
				if err := s.Close(ctx); err != nil {
					t.Fatal(err)
				}
			})

			type result struct {
				limit, remaining uint64
				reset            time.Duration
				ok               bool
				err              error
			}

			// Take twice everything from the bucket.
			takeCh := make(chan *result, 2*tc.tokens)
			for i := uint64(1); i <= 2*tc.tokens; i++ {
				go func() {
					limit, remaining, reset, ok, err := s.Take(ctx, key)
					takeCh <- &result{limit, remaining, time.Duration(fasttime.Now() - reset), ok, err}
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
				if err := result.err; err != nil {
					t.Fatal(err)
				}

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
			_, _, _, ok, err := s.Take(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
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
