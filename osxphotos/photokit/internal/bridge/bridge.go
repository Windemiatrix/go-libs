// Package bridge is the boundary between photokit and PhotoKit, so that the
// Go code of photokit is tested without Photos.
package bridge

import (
	"errors"
	"math"
	"time"
)

// ErrNotSupported is returned by every method of the bridge of a build
// without PhotoKit; photokit exports it.
var ErrNotSupported = errors.New("photokit: not supported on this platform")

// Bridge is what photokit needs from PhotoKit. A status is a
// PHAuthorizationStatus value for read-write access.
type Bridge interface {
	AuthorizationStatus() (int, error)
	// RequestAuthorization blocks up to timeout, then returns
	// context.DeadlineExceeded.
	RequestAuthorization(timeout time.Duration) (int, error)
}

// Half of int64 in nanoseconds, so that dispatch_time cannot overflow when it
// adds the current time.
const maxTimeoutMS = math.MaxInt64 / 2 / int64(time.Millisecond)

func timeoutMS(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	// Rounded down, a positive timeout under 1 ms would be 0, which does not
	// wait at all.
	return min(max(d.Milliseconds(), 1), maxTimeoutMS)
}
