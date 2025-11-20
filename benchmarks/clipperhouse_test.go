package benchmarks

import (
	"math"
	"testing"
	"time"

	"github.com/clipperhouse/rate"
)

func BenchmarkClipperhouseMemory(b *testing.B) {
	cases := []struct {
		name          string
		tokens        uint64
		interval      time.Duration
		sweepInterval time.Duration
		sweepMinTTL   time.Duration
	}{
		{
			name:          "memory",
			tokens:        math.MaxUint64,
			interval:      time.Duration(math.MaxInt64),
			sweepInterval: time.Duration(math.MaxInt64),
			sweepMinTTL:   time.Duration(math.MaxInt64),
		},
	}

	for _, tc := range cases {
		keyFunc := func(i int) int {
			return i % NumSessions
		}
		limit := rate.NewLimit(int64(tc.tokens), tc.interval)

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				limiter := rate.NewLimiter(keyFunc, limit)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					limiter.Allow(i)
				}
				b.StopTimer()
			})

			b.Run("parallel", func(b *testing.B) {
				limiter := rate.NewLimiter(keyFunc, limit)
				b.ReportAllocs()
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						limiter.Allow(i)
					}
				})
				b.StopTimer()
			})
		})
	}
}
