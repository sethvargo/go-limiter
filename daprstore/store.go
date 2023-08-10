// Package daprstore defines an DAPR state based store for limiting.
package daprstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"sync"
	"sync/atomic"
	"time"

	dapr "github.com/dapr/go-sdk/client"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/internal/fasttime"
)

const (
	stateStoreName = `statestore`
	daprPort       = "3500"
)

var port string
var _ limiter.Store = (*store)(nil)
var client dapr.Client

func init() {
	if port = os.Getenv("DAPR_GRPC_PORT"); len(port) == 0 {
		port = daprPort
	}
	client, _ = dapr.NewClientWithPort(port)
}

type store struct {
	tokens   uint64
	interval time.Duration

	sweepInterval time.Duration
	sweepMinTTL   uint64

	client   dapr.Client
	dataLock sync.RWMutex

	stopped uint32
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

	s := &store{
		tokens:   tokens,
		interval: interval,

		client: client,
		stopCh: make(chan struct{}),
	}
	go s.purge()
	return s, nil
}

// Take attempts to remove a token from the named key. If the take is
// successful, it returns true, otherwise false. It also returns the configured
// limit, remaining tokens, and reset time.
func (s *store) Take(ctx context.Context, key string) (uint64, uint64, uint64, bool, error) {
	// If the store is stopped, all requests are rejected.
	if atomic.LoadUint32(&s.stopped) == 1 {
		return 0, 0, 0, false, limiter.ErrStopped
	}

	item, err := client.GetState(ctx, stateStoreName, key, nil)
	if err != nil {
		return 0, 0, 0, false, err
	}
	eTag := item.Etag
	b := bucket{}
	binary.Read(bytes.NewBuffer(item.Value), binary.BigEndian, &b)
	b.take()
	binary.Write(bytes.NewBuffer(item.Value), binary.BigEndian, &b)
	stateOptions := dapr.StateOptions{
		Concurrency: dapr.StateConcurrencyFirstWrite,
		Consistency: dapr.StateConsistencyEventual,
	}
	eTag2 := dapr.ETag{Value: eTag}
	item2 := &dapr.SetStateItem{
		Key:     item.Key,
		Value:   item.Value,
		Etag:    &eTag2,
		Options: &stateOptions}
	err = client.SaveBulkState(ctx, stateStoreName, item2)
	if err != nil {
		return 0, 0, 0, false, err
	}
	return b.maxTokens, b.availableTokens, b.lastTick, true, nil
}

// Get retrieves the information about the key, if any exists.
func (s *store) Get(ctx context.Context, key string) (uint64, uint64, error) {
	// If the store is stopped, all requests are rejected.
	if atomic.LoadUint32(&s.stopped) == 1 {
		return 0, 0, limiter.ErrStopped
	}

	// Acquire a read lock first - this allows other to concurrently check limits
	// without taking a full lock.
	s.dataLock.RLock()
	// if b, ok := s.data[key]; ok {
	// 	s.dataLock.RUnlock()
	// 	return b.get()
	// }
	s.dataLock.RUnlock()

	return 0, 0, nil
}

// Set configures the bucket-specific tokens and interval.
func (s *store) Set(ctx context.Context, key string, tokens uint64, interval time.Duration) error {
	s.dataLock.Lock()
	// b := newBucket(tokens, interval)
	// s.data[key] = b
	s.dataLock.Unlock()
	return nil
}

// Burst adds the provided value to the bucket's currently available tokens.
func (s *store) Burst(ctx context.Context, key string, tokens uint64) error {
	s.dataLock.Lock()
	// if b, ok := s.data[key]; ok {
	// 	b.lock.Lock()
	// 	s.dataLock.Unlock()
	// 	b.availableTokens = b.availableTokens + tokens
	// 	b.lock.Unlock()
	// 	return nil
	// }

	// If we got this far, there's no current record for the key.
	// b := newBucket(s.tokens+tokens, s.interval)
	// s.data[key] = b
	s.dataLock.Unlock()
	return nil
}

// Close stops the memory limiter and cleans up any outstanding
// sessions. You should always call Close() as it releases the memory consumed
// by the map AND releases the tickers.
func (s *store) Close(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&s.stopped, 0, 1) {
		return nil
	}

	// Close the channel to prevent future purging.
	close(s.stopCh)

	// Delete all the things.

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

		s.dataLock.Lock()
		// now := fasttime.Now()
		// for k, b := range s.data {
		// 	b.lock.Lock()
		// 	lastTime := b.startTime + (b.lastTick * uint64(b.interval))
		// 	b.lock.Unlock()

		// 	if now-lastTime > s.sweepMinTTL {
		// 		delete(s.data, k)
		// 	}
		// }
		s.dataLock.Unlock()
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
	lock sync.Mutex
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
	b.lock.Lock()
	defer b.lock.Unlock()

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
	currTick := tick(b.startTime, now, b.interval)

	tokens = b.maxTokens
	reset = b.startTime + ((currTick + 1) * uint64(b.interval))

	b.lock.Lock()
	defer b.lock.Unlock()

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

// tick is the total number of times the current interval has occurred between
// when the time started (start) and the current time (curr). For example, if
// the start time was 12:30pm and it's currently 1:00pm, and the interval was 5
// minutes, tick would return 6 because 1:00pm is the 6th 5-minute tick. Note
// that tick would return 5 at 12:59pm, because it hasn't reached the 6th tick
// yet.
func tick(start, curr uint64, interval time.Duration) uint64 {
	return (curr - start) / uint64(interval.Nanoseconds())
}
