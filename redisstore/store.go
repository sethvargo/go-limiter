// Package redisstore defines a redis-backed storage system for limiting.
package redisstore

import (
	"crypto/sha1"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sethvargo/go-limiter"
)

var _ limiter.Store = (*store)(nil)

type store struct {
	tokens   uint64
	interval time.Duration
	rate     float64
	ttl      uint64
	pool     *pool

	luaScript    string
	luaScriptSHA string

	stopped uint32
}

// Config is used as input to New. It defines the behavior of the storage
// system.
type Config struct {
	// Tokens is the number of tokens to allow per interval. The default value is
	// 1.
	Tokens uint64

	// Interval is the time interval upon which to enforce rate limiting. The
	// default value is 1 second.
	Interval time.Duration

	// TTL is the amount of time a key should exist without changes before
	// purging. The default is 10 x interval.
	TTL uint64

	// InitialPoolSize and MaxPoolSize determine the initial and maximum number of
	// pool connections. The default values are 5 and 100 respectively.
	InitialPoolSize uint64
	MaxPoolSize     uint64

	// DialFunc is a function that creates a connection to the Redis
	// server.
	DialFunc func() (net.Conn, error)

	// AuthUsername and AuthPassword are optional authentication information.
	AuthUsername string
	AuthPassword string
}

// New uses a Redis instance to back a rate limiter that to limit the number of
// permitted events over an interval.
func New(c *Config) (limiter.Store, error) {
	if c == nil {
		c = new(Config)
	}

	tokens := uint64(1)
	if c.Tokens > 0 {
		tokens = c.Tokens
	}

	interval := 1 * time.Second
	if c.Interval > 0 {
		interval = c.Interval
	}

	rate := float64(interval) / float64(tokens)

	ttl := 10 * uint64(interval.Seconds())
	if c.TTL > 0 {
		ttl = c.TTL
	}
	if ttl == 0 {
		return nil, fmt.Errorf("ttl cannot be 0")
	}

	initialPoolSize := uint64(5)
	if c.InitialPoolSize > 0 {
		initialPoolSize = c.InitialPoolSize
	}

	maxPoolSize := uint64(5)
	if c.InitialPoolSize > 0 {
		maxPoolSize = c.MaxPoolSize
	}

	dialFunc := c.DialFunc
	if dialFunc == nil {
		return nil, fmt.Errorf("missing DialFunc")
	}

	luaScript := fmt.Sprintf(string(luaTemplate),
		tokens, interval, rate, ttl)
	luaScriptSHA := fmt.Sprintf("%x", sha1.Sum([]byte(luaScript)))

	pool, err := newPool(&poolConfig{
		initial:  initialPoolSize,
		max:      maxPoolSize,
		dialFunc: dialFunc,
		username: c.AuthUsername,
		password: c.AuthPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to setup connection pool: %w", err)
	}

	client, err := pool.get()
	if err != nil {
		return nil, fmt.Errorf("failed to get client to configure lua: %w", err)
	}

	if _, err := client.do("SCRIPT", "LOAD", luaScript); err != nil {
		if closeErr := client.release(pool); err != nil {
			return nil, fmt.Errorf("failed to prime script: %v, but then failed to close client: %w", err, closeErr)
		}
		return nil, fmt.Errorf("failed to prime script: %v", err)
	}

	if err := client.release(pool); err != nil {
		return nil, fmt.Errorf("failed to close client: %w", err)
	}

	s := &store{
		tokens:   tokens,
		interval: interval,
		rate:     rate,
		ttl:      ttl,
		pool:     pool,

		luaScript:    luaScript,
		luaScriptSHA: luaScriptSHA,
	}
	return s, nil
}

// Take attempts to remove a token from the named key. If the take is
// successful, it returns true, otherwise false. It also returns the configured
// limit, remaining tokens, and reset time, if one was found. Any errors
// connecting to the store or parsing the return value are considered failures
// and fail the take.
func (s *store) Take(key string) (tokens uint64, remaining uint64, next uint64, ok bool, retErr error) {
	// If the store is stopped, all requests are rejected.
	if atomic.LoadUint32(&s.stopped) == 1 {
		return 0, 0, 0, false, limiter.ErrStopped
	}

	// Get a client from the pool.
	c, err := s.pool.get()
	if err != nil {
		retErr = fmt.Errorf("failed to get redis client from pool: %w", err)
		return 0, 0, 0, false, retErr
	}
	defer func() {
		if err := c.release(s.pool); err != nil {
			retErr = fmt.Errorf("failed to release pool: %v, original error: %w", err, retErr)
		}
	}()

	now := uint64(time.Now().UTC().UnixNano())
	nowStr := strconv.FormatUint(now, 10)

	resp, err := c.do("EVAL", s.luaScript, "1", key, nowStr)
	if err != nil {
		retErr = fmt.Errorf("failed to EVAL script: %w", err)
		return 0, 0, 0, false, retErr
	}

	a := resp.array()
	if len(a) < 3 {
		retErr = fmt.Errorf("response has less than 3 values: %#v", a)
		return 0, 0, 0, false, retErr
	}

	tokens, next, ok = a[0].uint64(), a[1].uint64(), a[2].uint64() == 1
	return s.tokens, tokens, next, ok, nil
}

// Close stops the memory limiter and cleans up any outstanding sessions. You
// should absolutely always call Close() as it releases any open network
// connections.
func (s *store) Close() error {
	if !atomic.CompareAndSwapUint32(&s.stopped, 0, 1) {
		return nil
	}

	// Close the connection pool.
	s.pool.close()
	return nil
}
