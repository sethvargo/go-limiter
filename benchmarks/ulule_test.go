package benchmarks

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

func BenchmarkUlule(b *testing.B) {
	cases := []struct {
		name     string
		tokens   uint64
		interval time.Duration
	}{
		{
			name:     "memory",
			tokens:   math.MaxUint64,
			interval: time.Duration(math.MaxInt64),
		},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				ctx := context.Background()
				store := memory.NewStore()
				instance := limiter.New(store, limiter.Rate{
					Limit:  int64(tc.tokens),
					Period: tc.interval,
				})
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					instance.Get(ctx, testSessionID(b, i))
				}
			})

			b.Run("parallel", func(b *testing.B) {
				ctx := context.Background()
				store := memory.NewStore()
				instance := limiter.New(store, limiter.Rate{
					Limit:  int64(tc.tokens),
					Period: tc.interval,
				})
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						instance.Get(ctx, testSessionID(b, i))
					}
				})
			})
		})
	}
}
