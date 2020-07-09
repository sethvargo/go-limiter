package httplimit_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
)

func TestNewMiddleware(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tokens   uint64
		interval time.Duration
	}{
		{
			name:     "millisecond",
			tokens:   5,
			interval: 500 * time.Millisecond,
		},
		{
			name:     "second",
			tokens:   3,
			interval: time.Second,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := memorystore.New(&memorystore.Config{
				Tokens:   tc.tokens,
				Interval: tc.interval,
			})
			if err != nil {
				t.Fatal(err)
			}

			middleware, err := httplimit.NewMiddleware(store, httplimit.IPKeyFunc())
			if err != nil {
				t.Fatal(err)
			}

			doWork := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				fmt.Fprintf(w, "hello world")
			})

			server := httptest.NewServer(middleware.Handle(doWork))
			defer server.Close()

			client := server.Client()

			for i := uint64(0); i < tc.tokens; i++ {
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}

				limit, err := strconv.ParseUint(resp.Header.Get(httplimit.HeaderRateLimitLimit), 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := limit, tc.tokens; got != want {
					t.Errorf("limit: expected %d to be %d", got, want)
				}

				reset, err := time.Parse(time.RFC1123, resp.Header.Get(httplimit.HeaderRateLimitReset))
				if err != nil {
					t.Fatal(err)
				}
				if got, want := time.Until(reset), tc.interval; got > want {
					t.Errorf("reset: expected %d to be less than %d", got, want)
				}

				remaining, err := strconv.ParseUint(resp.Header.Get(httplimit.HeaderRateLimitRemaining), 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := remaining, tc.tokens-uint64(i)-1; got != want {
					t.Errorf("remaining: expected %d to be %d", got, want)
				}
			}

			// Should be limited
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := resp.StatusCode, http.StatusTooManyRequests; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}

			limit, err := strconv.ParseUint(resp.Header.Get(httplimit.HeaderRateLimitLimit), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := limit, tc.tokens; got != want {
				t.Errorf("limit: expected %d to be %d", got, want)
			}

			reset, err := time.Parse(time.RFC1123, resp.Header.Get(httplimit.HeaderRateLimitReset))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := time.Until(reset), tc.interval; got > want {
				t.Errorf("reset: expected %d to be less than %d", got, want)
			}

			remaining, err := strconv.ParseUint(resp.Header.Get(httplimit.HeaderRateLimitRemaining), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := remaining, uint64(0); got != want {
				t.Errorf("remaining: expected %d to be %d", got, want)
			}
		})
	}
}
