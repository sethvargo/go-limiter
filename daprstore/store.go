// Package daprstore defines an DAPR state based store for limiting.
package daprstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"strings"
	"sync/atomic"
	"time"

	dapr "github.com/dapr/go-sdk/client"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/internal/fasttime"
)

const DEFAULT_DAPR_PORT = "3500"
const DEFAULT_STATE_STORE_NAME = "statestore"

var port string
var _ limiter.Store = (*store)(nil)
var client dapr.Client

func init() {
	if port = os.Getenv("DAPR_GRPC_PORT"); len(port) == 0 {
		port = DEFAULT_DAPR_PORT
	}
	client, _ = dapr.NewClientWithPort(port)
}

type store struct {
	tokens   uint64
	interval time.Duration

	client         dapr.Client
	stateStoreName string
	stopped        uint32
	stopCh         chan struct{}
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

	// The name of the DAPR state store to be used for the distributed bucket
	StateStoreName string
}

// New creates a DAPR state rate limiter that uses a bucketing model to limit
// the number of permitted events over an interval. All DAPR applications with the same app-id
// can use the same bucket.
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

	stateStoreName := DEFAULT_STATE_STORE_NAME
	if c.StateStoreName != "" {
		stateStoreName = c.StateStoreName
	}

	s := &store{
		tokens:         tokens,
		interval:       interval,
		client:         client,
		stateStoreName: stateStoreName,
		stopCh:         make(chan struct{}),
	}
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
	// Get the current bucket, or create a new one if it doesn't exist.
	item, err := client.GetStateWithConsistency(ctx, s.stateStoreName, key, nil, dapr.StateConsistencyStrong)
	if err != nil {
		return 0, 0, 0, false, err
	}
	var b Bucket
	eTag := item.Etag
	if item.Value == nil {
		b = *newBucket(s.tokens, s.interval)
	} else {
		binary.Read(bytes.NewBuffer(item.Value), binary.BigEndian, &b)
	}

	// Take a token from the bucket.
	tokens, remaining, reset, ok, err := b.take()
	if err != nil {
		return 0, 0, 0, false, err
	}

	// Save the bucket
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, b)

	err = client.SaveStateWithETag(ctx, s.stateStoreName, key, buf.Bytes(), eTag, nil, dapr.WithConcurrency(dapr.StateConcurrencyFirstWrite), dapr.WithConsistency(dapr.StateConsistencyStrong))
	if err != nil {
		if strings.Contains(err.Error(), "etag mismatch") {
			// Server conflict so try it again
			return s.Take(ctx, key)
		}
		return 0, 0, 0, false, err
	}
	return tokens, remaining, reset, ok, nil
}

// Get retrieves the information about the key, if any exists.
func (s *store) Get(ctx context.Context, key string) (uint64, uint64, error) {
	// If the store is stopped, all requests are rejected.
	if atomic.LoadUint32(&s.stopped) == 1 {
		return 0, 0, limiter.ErrStopped
	}

	// Get the current bucket, or create a new one if it doesn't exist.
	item, err := client.GetStateWithConsistency(ctx, s.stateStoreName, key, nil, dapr.StateConsistencyStrong)
	if err != nil {
		return 0, 0, err
	}
	var b Bucket
	if item.Value == nil {
		return 0, 0, nil
	} else {
		binary.Read(bytes.NewBuffer(item.Value), binary.BigEndian, &b)
		return b.get()
	}
}

// Set configures the bucket-specific tokens and interval.
func (s *store) Set(ctx context.Context, key string, tokens uint64, interval time.Duration) error {
	b := newBucket(tokens, interval)
	// Get the current bucket.
	item, err := client.GetStateWithConsistency(ctx, s.stateStoreName, key, nil, dapr.StateConsistencyStrong)
	if err == nil {
		eTag := item.Etag
		// Save the bucket
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, b)

		err = client.SaveStateWithETag(ctx, s.stateStoreName, key, buf.Bytes(), eTag, nil, dapr.WithConcurrency(dapr.StateConcurrencyFirstWrite), dapr.WithConsistency(dapr.StateConsistencyStrong))
		if err != nil {
			if strings.Contains(err.Error(), "etag mismatch") {
				// Server conflict so try it again
				return s.Set(ctx, key, tokens, interval)
			}
		}
	}
	return nil
}

// Burst adds the provided value to the bucket's currently available tokens.
func (s *store) Burst(ctx context.Context, key string, tokens uint64) error {
	var b Bucket
	// Get the current bucket.
	item, err := client.GetStateWithConsistency(ctx, s.stateStoreName, key, nil, dapr.StateConsistencyStrong)
	if err == nil {
		eTag := item.Etag
		if item.Value != nil {
			binary.Read(bytes.NewBuffer(item.Value), binary.BigEndian, &b)
		}
		// Add tokens to the bucket.
		b.AvailableTokens += tokens
		// Save the bucket
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, b)

		err = client.SaveStateWithETag(ctx, s.stateStoreName, key, buf.Bytes(), eTag, nil, dapr.WithConcurrency(dapr.StateConcurrencyFirstWrite), dapr.WithConsistency(dapr.StateConsistencyStrong))
		if err != nil {
			if strings.Contains(err.Error(), "etag mismatch") {
				// Server conflict so try it again
				return s.Burst(ctx, key, tokens)
			}
		}
	}
	return nil
}

// Close stops the dapr limiter and cleans up any outstanding
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

// Bucket is an internal wrapper around a taker.
type Bucket struct {
	// StartTime is the number of nanoseconds from unix epoch when this bucket was
	// initially created.
	StartTime uint64

	// MaxTokens is the maximum number of tokens permitted on the bucket at any
	// time. The number of available tokens will never exceed this value.
	MaxTokens uint64

	// Interval is the time at which ticking should occur.
	Interval time.Duration

	// AvailableTokens is the current point-in-time number of tokens remaining.
	AvailableTokens uint64

	// LastTick is the last clock tick, used to re-calculate the number of tokens
	// on the bucket.
	LastTick uint64
}

// newBucket creates a new bucket from the given tokens and interval.
func newBucket(tokens uint64, interval time.Duration) *Bucket {
	b := &Bucket{
		StartTime:       fasttime.Now(),
		MaxTokens:       tokens,
		AvailableTokens: tokens,
		Interval:        interval,
	}
	return b
}

// get returns information about the bucket.
func (b *Bucket) get() (tokens uint64, remaining uint64, retErr error) {
	//b.Lock.Lock()
	//defer b.Lock.Unlock()

	tokens = b.MaxTokens
	remaining = b.AvailableTokens
	return
}

// take attempts to remove a token from the bucket. If there are no tokens
// available and the clock has ticked forward, it recalculates the number of
// tokens and retries. It returns the limit, remaining tokens, time until
// refresh, and whether the take was successful.
func (b *Bucket) take() (tokens uint64, remaining uint64, reset uint64, ok bool, retErr error) {
	// Capture the current request time, current tick, and amount of time until
	// the bucket resets.
	now := fasttime.Now()
	currTick := tick(b.StartTime, now, b.Interval)

	tokens = b.MaxTokens
	reset = b.StartTime + ((currTick + 1) * uint64(b.Interval))

	//b.Lock.Lock()
	//defer b.Lock.Unlock()

	// If we're on a new tick since last assessment, perform
	// a full reset up to maxTokens.
	if b.LastTick < currTick {
		b.AvailableTokens = b.MaxTokens
		b.LastTick = currTick
	}

	if b.AvailableTokens > 0 {
		b.AvailableTokens--
		ok = true
		remaining = b.AvailableTokens
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
