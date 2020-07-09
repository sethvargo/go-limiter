package benchmarks

import (
	"math"
	"testing"
	"time"

	"github.com/didip/tollbooth/v6/limiter"
)

func BenchmarkTollbooth(b *testing.B) {
	cases := []struct {
		name        string
		tokens      uint64
		sweepMinTTL time.Duration
	}{
		{
			name:        "memory",
			tokens:      math.MaxUint64,
			sweepMinTTL: time.Duration(math.MaxInt64),
		},
		{
			name:        "sweep",
			tokens:      SessionDefaultTokens,
			sweepMinTTL: SessionMinTTL,
		},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				// Note: tollbooth doesn't support any granularity lower than 1 second
				instance := limiter.New(nil).
					SetMax(float64(tc.tokens)).
					SetTokenBucketExpirationTTL(tc.sweepMinTTL)
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					instance.LimitReached(testSessionID(b, i))
				}
			})

			b.Run("parallel", func(b *testing.B) {
				// Note: tollbooth doesn't support any granularity lower than 1 second
				instance := limiter.New(nil).
					SetMax(float64(tc.tokens)).
					SetTokenBucketExpirationTTL(tc.sweepMinTTL)
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						instance.LimitReached(testSessionID(b, i))
					}
				})
			})
		})
	}
}
