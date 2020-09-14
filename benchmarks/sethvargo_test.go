package benchmarks

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/sethvargo/go-limiter/memorystore"
	"github.com/sethvargo/go-redisstore"
)

func BenchmarkSethVargoMemory(b *testing.B) {
	ctx := context.Background()

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
		{
			name:          "sweep",
			tokens:        SessionDefaultTokens,
			interval:      SessionDefaultInterval,
			sweepInterval: SessionSweepInterval,
			sweepMinTTL:   SessionMinTTL,
		},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				store, err := memorystore.New(&memorystore.Config{
					SweepInterval: tc.sweepInterval,
					SweepMinTTL:   tc.sweepMinTTL,
					Interval:      tc.interval,
					Tokens:        tc.tokens,
				})
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := store.Close(ctx); err != nil {
						b.Fatal(err)
					}
				})
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					store.Take(ctx, testSessionID(b, i))
				}
				b.StopTimer()
			})

			b.Run("parallel", func(b *testing.B) {
				store, err := memorystore.New(&memorystore.Config{
					SweepInterval: tc.sweepInterval,
					SweepMinTTL:   tc.sweepMinTTL,
					Interval:      tc.interval,
					Tokens:        tc.tokens,
				})
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := store.Close(ctx); err != nil {
						b.Fatal(err)
					}
				})
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						store.Take(ctx, testSessionID(b, i))
					}
				})
				b.StopTimer()
			})
		})
	}
}

func BenchmarkSethVargoRedis(b *testing.B) {
	ctx := context.Background()

	host := os.Getenv("REDIS_HOST")
	if host == "" {
		b.Fatal("missing REDIS_HOST")
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
			name:     "redis",
			tokens:   math.MaxUint64,
			interval: time.Duration(math.MaxInt64),
		},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {

				store, err := redisstore.New(&redisstore.Config{
					Tokens:   tc.tokens,
					Interval: tc.interval,
					Dial: func() (redis.Conn, error) {
						return redis.Dial("tcp", host+":"+port,
							redis.DialPassword(pass))
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := store.Close(ctx); err != nil {
						b.Fatal(err)
					}
				})
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					store.Take(ctx, testSessionID(b, i))
				}
				b.StopTimer()
			})

			b.Run("parallel", func(b *testing.B) {

				store, err := redisstore.New(&redisstore.Config{
					Tokens:   tc.tokens,
					Interval: tc.interval,
					Dial: func() (redis.Conn, error) {
						return redis.Dial("tcp", host+":"+port,
							redis.DialPassword(pass))
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := store.Close(ctx); err != nil {
						b.Fatal(err)
					}
				})
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						store.Take(ctx, testSessionID(b, i))
					}
				})
				b.StopTimer()
			})
		})
	}
}
