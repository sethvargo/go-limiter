// Package noopstore defines a storage system for limiting that always allows
// requests. It's an empty store useful for testing or development.
package noopstore

import (
	"context"
	"time"

	"github.com/sethvargo/go-limiter"
)

var _ limiter.Store = (*store)(nil)

type store struct{}

func New() (limiter.Store, error) {
	return &store{}, nil
}

// Take always allows the request.
func (s *store) Take(_ context.Context, _ string) (uint64, uint64, uint64, bool, error) {
	return 0, 0, 0, true, nil
}

// TakeMultiple always allows the request.
func (s *store) TakeMultiple(_ context.Context, _ string, _ uint64) (uint64, uint64, uint64, bool, error) {
	return 0, 0, 0, true, nil
}

// Get does nothing.
func (s *store) Get(_ context.Context, _ string) (uint64, uint64, error) {
	return 0, 0, nil
}

// Set does nothing.
func (s *store) Set(_ context.Context, _ string, _ uint64, _ time.Duration) error {
	return nil
}

// Burst does nothing.
func (s *store) Burst(_ context.Context, _ string, _ uint64) error {
	return nil
}

// Close does nothing.
func (s *store) Close(_ context.Context) error {
	return nil
}
