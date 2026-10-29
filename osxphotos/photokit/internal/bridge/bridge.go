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
	// FetchAssets and FetchBurst find hidden assets too and return them in
	// the order of PhotoKit; none found is not an error. FetchBurst returns
	// every frame with all, otherwise the selected ones and the key photo.
	FetchAssets(ids []string) ([]AssetRecord, error)
	FetchBurst(burstID string, all bool) ([]AssetRecord, error)
}

// AssetRecord is a PHAsset as the bridge reads it; photokit makes an Asset of
// it. A date is nil without a value, the coordinates are nil without a
// location.
type AssetRecord struct {
	LocalIdentifier         string
	MediaType               int
	MediaSubtypes           uint64
	BurstIdentifier         string
	SourceType              int
	PixelWidth, PixelHeight int
	CreationDate            *time.Time
	ModificationDate        *time.Time
	Latitude, Longitude     *float64
	Duration                time.Duration
	Favorite, Hidden        bool
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
