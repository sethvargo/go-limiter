// Package noopstore defines a storage system for limiting that always allows
// requests. It's an empty store useful for testing or development.
package noopstore

import "github.com/sethvargo/go-limiter"

var _ limiter.Store = (*store)(nil)

type store struct{}

// Take always allows the request.
func (s *store) Take(_ string) (uint64, uint64, uint64, bool) {
	return 0, 0, 0, true
}

// Close does nothing.
func (s *store) Close() error {
	return nil
}
