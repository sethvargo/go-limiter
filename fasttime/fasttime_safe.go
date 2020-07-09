// +build purego

package fasttime

import "time"

// Now returns the current unix time.
func Now() uint64 {
	return time.Now().UnixNano()
}
