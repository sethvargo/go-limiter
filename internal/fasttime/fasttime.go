// +build !windows

// Package fasttime gets wallclock time, but super fast.
package fasttime

import (
	_ "unsafe"
)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// Now returns a monotonic clock value. The actual value will differ across
// systems, but that's okay because we generally only care about the deltas.
func Now() uint64 {
	return uint64(nanotime())
}
