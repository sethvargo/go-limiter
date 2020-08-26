// Package noopstore defines a storage system for limiting that always allows
// requests. It's an empty store useful for testing or development.
package noopstore

import (
	"context"

	"github.com/sethvargo/go-limiter"
)

var _ limiter.Store = (*store)(nil)
var _ limiter.StoreWithContext = (*store)(nil)

type store struct{}

func New() (limiter.StoreWithContext, error) {
	return &store{}, nil
}

// Take always allows the request.
func (s *store) Take(_ string) (uint64, uint64, uint64, bool, error) {
	return 0, 0, 0, true, nil
}

// TakeWithContext always allows the request.
func (s *store) TakeWithContext(_ context.Context, _ string) (uint64, uint64, uint64, bool, error) {
	return 0, 0, 0, true, nil
}

// Close does nothing.
func (s *store) Close() error {
	return nil
}

// CloseWithContext does nothing.
func (s *store) CloseWithContext(_ context.Context) error {
	return nil
}
