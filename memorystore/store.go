// Package memorystore defines an in-memory storage system for limiting.
package memorystore

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/internal/fasttime"
)

var _ limiter.Store = (*store)(nil)

type store struct {
	tokens   uint64
	interval time.Duration

	sweepInterval time.Duration
	sweepMinTTL   uint64

	// data holds the per-key buckets. It is a sync.Map because the access
	// pattern is create-once, read-many: a bucket is created on a key's first
	// Take and then read on every subsequent Take. sync.Map serves those reads
	// from a read-only snapshot without mutating shared state, avoiding the
	// per-call atomic writes an RWMutex makes to a single counter (which bounces
	// that cache line across cores under load).
	data sync.Map // map[string]*bucket

	stopped atomic.Bool
	stopCh  chan struct{}
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

	// SweepInterval is the rate at which to run the garbage collection on stale
	// entries. Setting this to a low value will optimize memory consumption, but
	// will likely reduce performance and increase lock contention. Setting this
	// to a high value will maximum throughput, but will increase the memory
	// footprint. This can be tuned in combination with SweepMinTTL to control how
	// long stale entries are kept. The default value is 6 hours.
	SweepInterval time.Duration

	// SweepMinTTL is the minimum amount of time a session must be inactive before
	// clearing it from the entries. There's no validation, but this should be at
	// least as high as your rate limit, or else the data store will purge records
	// before their limit is applied. The default value is 12 hours.
	SweepMinTTL time.Duration

	// InitialAlloc previously pre-sized the in-memory map. It is retained for
	// backward compatibility but no longer has any effect: the store now uses a
	// sync.Map, which does not support pre-sizing.
	InitialAlloc int

	// DisablePurge disables the purge operation. WARNING: this will cause
	// unbounded memory growth. Do not enable unless you have a fixed number of
	// buckets.
	DisablePurge bool
}

// New creates an in-memory rate limiter that uses a bucketing model to limit
// the number of permitted events over an interval. It's optimized for runtime
// and memory efficiency.
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

	sweepInterval := 6 * time.Hour
	if c.SweepInterval > 0 {
		sweepInterval = c.SweepInterval
	}

	sweepMinTTL := 12 * time.Hour
	if c.SweepMinTTL > 0 {
		sweepMinTTL = c.SweepMinTTL
	}

	s := &store{
		tokens:   tokens,
		interval: interval,

		sweepInterval: sweepInterval,
		sweepMinTTL:   uint64(sweepMinTTL),

		stopCh: make(chan struct{}),
	}

	if !c.DisablePurge {
		go s.purge()
	}

	return s, nil
}

// Take attempts to remove a token from the named key. If the take is
// successful, it returns true, otherwise false. It also returns the configured
// limit, remaining tokens, and reset time.
func (s *store) Take(ctx context.Context, key string) (uint64, uint64, uint64, bool, error) {
	// If the store is stopped, all requests are rejected.
	if s.stopped.Load() {
		return 0, 0, 0, false, limiter.ErrStopped
	}

	// The common case is an existing bucket, which sync.Map serves from its
	// read-only path without mutating shared state.
	if v, ok := s.data.Load(key); ok {
		return v.(*bucket).take()
	}

	// First time we've seen this key (or it was garbage collected). Create a
	// bucket and store it, deferring to the winner if another goroutine raced us.
	b := newBucket(s.tokens, s.interval)
	actual, _ := s.data.LoadOrStore(key, b)
	return actual.(*bucket).take()
}

// Get retrieves the information about the key, if any exists.
func (s *store) Get(ctx context.Context, key string) (uint64, uint64, error) {
	// If the store is stopped, all requests are rejected.
	if s.stopped.Load() {
		return 0, 0, limiter.ErrStopped
	}

	// Acquire a read lock first - this allows other to concurrently check limits
	// without taking a full lock.
	if v, ok := s.data.Load(key); ok {
		return v.(*bucket).get()
	}

	return 0, 0, nil
}

// Set configures the bucket-specific tokens and interval.
func (s *store) Set(ctx context.Context, key string, tokens uint64, interval time.Duration) error {
	// A non-positive interval would divide by zero in tick(). Fall back to the
	// store's configured interval, mirroring New.
	if interval <= 0 {
		interval = s.interval
	}

	s.data.Store(key, newBucket(tokens, interval))
	return nil
}

// Burst adds the provided value to the bucket's currently available tokens.
func (s *store) Burst(ctx context.Context, key string, tokens uint64) error {
	if v, ok := s.data.Load(key); ok {
		v.(*bucket).burst(tokens)
		return nil
	}

	// No current record for the key. Create one pre-filled with the burst; if
	// another goroutine raced us, burst onto the winner instead.
	b := newBucket(s.tokens+tokens, s.interval)
	if actual, loaded := s.data.LoadOrStore(key, b); loaded {
		actual.(*bucket).burst(tokens)
	}
	return nil
}

// Close stops the memory limiter and cleans up any outstanding
// sessions. You should always call Close() as it releases the memory consumed
// by the map AND releases the tickers.
func (s *store) Close(ctx context.Context) error {
	if !s.stopped.CompareAndSwap(false, true) {
		return nil
	}

	// Close the channel to prevent future purging.
	close(s.stopCh)

	// Delete all the things.
	s.data.Range(func(k, _ any) bool {
		s.data.Delete(k)
		return true
	})
	return nil
}

// purge continually iterates over the map and purges old values on the provided
// sweep interval. Earlier designs used a go-function-per-item expiration, but
// it actually generated *more* lock contention under normal use. The most
// performant option with real-world data was a global garbage collection on a
// fixed interval.
func (s *store) purge() {
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}

		now := fasttime.Now()
		s.data.Range(func(k, v any) bool {
			b := v.(*bucket)
			b.lock.RLock()
			lastTime := b.startTime + (b.lastTick * uint64(b.interval))
			b.lock.RUnlock()

			// There's a very rare edge case where the server clock is reset between
			// the call to fasttime.Now() above and when this bucket is locked. This
			// is more likely when there are many buckets, since this function will
			// take longer to run.
			lastTime = min(lastTime, now)

			if now-lastTime > s.sweepMinTTL {
				s.data.Delete(k)
			}
			return true
		})
	}
}

// bucket is an internal wrapper around a taker.
type bucket struct {
	// startTime is the number of nanoseconds from unix epoch when this bucket was
	// initially created.
	startTime uint64

	// maxTokens is the maximum number of tokens permitted on the bucket at any
	// time. The number of available tokens will never exceed this value.
	maxTokens uint64

	// interval is the time at which ticking should occur.
	interval time.Duration

	// availableTokens is the current point-in-time number of tokens remaining.
	availableTokens uint64

	// lastTick is the last clock tick, used to re-calculate the number of tokens
	// on the bucket.
	lastTick uint64

	// lock guards the mutable fields.
	lock sync.RWMutex
}

// newBucket creates a new bucket from the given tokens and interval.
func newBucket(tokens uint64, interval time.Duration) *bucket {
	b := &bucket{
		startTime:       fasttime.Now(),
		maxTokens:       tokens,
		availableTokens: tokens,
		interval:        interval,
	}
	return b
}

// get returns information about the bucket.
func (b *bucket) get() (tokens uint64, remaining uint64, retErr error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	tokens = b.maxTokens
	remaining = b.availableTokens
	return
}

// take attempts to remove a token from the bucket. If there are no tokens
// available and the clock has ticked forward, it recalculates the number of
// tokens and retries. It returns the limit, remaining tokens, time until
// refresh, and whether the take was successful.
func (b *bucket) take() (tokens uint64, remaining uint64, reset uint64, ok bool, retErr error) {
	// Capture the current request time, current tick, and amount of time until
	// the bucket resets.
	now := fasttime.Now()

	b.lock.Lock()
	defer b.lock.Unlock()

	// If the current time is before the start time, it means the server clock was
	// reset to an earlier time. In that case, rebase to 0.
	if now < b.startTime {
		b.startTime = now
		b.lastTick = 0
	}

	currTick := tick(b.startTime, now, b.interval)

	tokens = b.maxTokens
	reset = b.startTime + ((currTick + 1) * uint64(b.interval))

	// If we're on a new tick since last assessment, perform
	// a full reset up to maxTokens.
	if b.lastTick < currTick {
		b.availableTokens = b.maxTokens
		b.lastTick = currTick
	}

	if b.availableTokens > 0 {
		b.availableTokens--
		ok = true
		remaining = b.availableTokens
	}

	return
}

// burst adds the specified number of tokens to the bucket's available tokens in
// a thread-safe manner. The addition saturates at math.MaxUint64 so a large
// burst cannot overflow and wrap to a smaller value.
func (b *bucket) burst(tokens uint64) {
	b.lock.Lock()
	if b.availableTokens > math.MaxUint64-tokens {
		b.availableTokens = math.MaxUint64
	} else {
		b.availableTokens += tokens
	}
	b.lock.Unlock()
}

// tick is the total number of times the current interval has occurred between
// when the time started (start) and the current time (curr). For example, if
// the start time was 12:30pm and it's currently 1:00pm, and the interval was 5
// minutes, tick would return 6 because 1:00pm is the 6th 5-minute tick. Note
// that tick would return 5 at 12:59pm, because it hasn't reached the 6th tick
// yet.
func tick(start, curr uint64, interval time.Duration) uint64 {
	return (curr - start) / uint64(interval.Nanoseconds())
}
