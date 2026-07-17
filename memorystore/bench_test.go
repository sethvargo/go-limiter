package memorystore

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/sethvargo/go-limiter"
)

// benchStore builds a store with effectively unlimited tokens and no ticking or
// sweeping during the run, so benchmarks measure the steady-state Take path
// rather than refills or purges.
func benchStore(tb testing.TB) limiter.Store {
	tb.Helper()

	s, err := New(&Config{
		Tokens:        1 << 62,
		Interval:      time.Hour,
		SweepInterval: time.Hour,
		SweepMinTTL:   time.Hour,
		DisablePurge:  true,
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := s.Close(context.Background()); err != nil {
			tb.Fatal(err)
		}
	})
	return s
}

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "key-" + strconv.Itoa(i)
	}
	return keys
}

// BenchmarkTake_serial measures the raw per-call cost with no contention.
func BenchmarkTake_serial(b *testing.B) {
	ctx := context.Background()
	s := benchStore(b)
	if _, _, _, _, err := s.Take(ctx, "key"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Take(ctx, "key")
	}
}

// BenchmarkTake_parallel_hotKey measures contention when every goroutine hits
// the same bucket (one aggressive client).
func BenchmarkTake_parallel_hotKey(b *testing.B) {
	ctx := context.Background()
	s := benchStore(b)
	if _, _, _, _, err := s.Take(ctx, "key"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Take(ctx, "key")
		}
	})
}

// BenchmarkTake_parallel_manyKeys measures contention when goroutines spread
// across many distinct buckets (many clients), stressing the shared map lock.
func BenchmarkTake_parallel_manyKeys(b *testing.B) {
	ctx := context.Background()
	s := benchStore(b)
	keys := benchKeys(10000)
	for _, k := range keys {
		if _, _, _, _, err := s.Take(ctx, k); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Take(ctx, keys[i%len(keys)])
			i++
		}
	})
}
