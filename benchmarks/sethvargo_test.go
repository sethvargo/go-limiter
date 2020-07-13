package benchmarks

import (
	"math"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sethvargo/go-limiter/memorystore"
	"github.com/sethvargo/go-limiter/redisstore"
)

func BenchmarkSethVargoMemory(b *testing.B) {
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
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					store.Take(testSessionID(b, i))
				}
				b.StopTimer()
				store.Close()
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
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						store.Take(testSessionID(b, i))
					}
				})
				b.StopTimer()
				store.Close()
			})
		})
	}
}

func BenchmarkSethVargoRedis(b *testing.B) {
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
					Interval:        tc.interval,
					Tokens:          tc.tokens,
					InitialPoolSize: 64,
					MaxPoolSize:     128,
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
					b.Fatal(err)
				}
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					store.Take(testSessionID(b, i))
				}
				b.StopTimer()
				store.Close()
			})

			b.Run("parallel", func(b *testing.B) {

				store, err := redisstore.New(&redisstore.Config{
					Interval:        tc.interval,
					Tokens:          tc.tokens,
					InitialPoolSize: 64,
					MaxPoolSize:     128,
					AuthPassword:    pass,
					DialFunc: func() (net.Conn, error) {
						conn, err := net.Dial("tcp", host)
						if err != nil {
							return nil, err
						}
						return conn, nil
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for i := 0; pb.Next(); i++ {
						store.Take(testSessionID(b, i))
					}
				})
				b.StopTimer()
				store.Close()
			})
		})
	}
}
