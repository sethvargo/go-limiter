package benchmarks

import (
	"math"
	"testing"

	"go.uber.org/ratelimit"
)

func BenchmarkUber(b *testing.B) {
	cases := []struct {
		name   string
		tokens int
	}{
		{
			name:   "memory",
			tokens: math.MaxInt64,
		},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				instance := ratelimit.New(int(tc.tokens))
				b.ResetTimer()

				var x string
				for i := 0; i < b.N; i++ {
					x = testSessionID(b, i)
					instance.Take()
				}
				_ = x
			})

			b.Run("parallel", func(b *testing.B) {
				instance := ratelimit.New(int(tc.tokens))
				b.ResetTimer()

				var x string
				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						x = testSessionID(b, i)
						instance.Take()
					}
				})
				_ = x
			})
		})
	}
}
