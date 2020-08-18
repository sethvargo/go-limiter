package redisstore

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
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

	if testing.Short() {
		t.Skipf("skipping (short)")
	}

	host := os.Getenv("REDIS_HOST")
	if host == "" {
		t.Fatal("missing REDIS_HOST")
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	pass := os.Getenv("REDIS_PASS")

	cases := []struct {
		name     string
		tokens   uint64
		interval time.Duration
	}{
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
				Interval:        tc.interval,
				Tokens:          tc.tokens,
				InitialPoolSize: 32,
				MaxPoolSize:     32,
				AuthPassword:    pass,
				DialFunc: func() (net.Conn, error) {
					conn, err := net.Dial("tcp", host+":"+port)
					if err != nil {
						return nil, err
					}
					return conn, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()

			type result struct {
				limit, remaining uint64
				reset            time.Duration
				ok               bool
				err              error
			}

			// Take twice everything
			takeCh := make(chan *result, 2*tc.tokens)
			for i := uint64(1); i <= 2*tc.tokens; i++ {
				go func() {
					limit, remaining, reset, ok, err := s.Take(key)
					takeCh <- &result{limit, remaining, time.Until(time.Unix(0, int64(reset))), ok, err}
				}()
			}

			// Accumulate and sort results, since they could come in any order
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

			// Wait for entries again
			time.Sleep(tc.interval)

			// Verify we can take once more
			_, _, _, ok, err := s.Take(key)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Errorf("expected %t to be %t", ok, true)
			}
		})
	}
}
