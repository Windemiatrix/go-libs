// Package bridge is the boundary between photokit and PhotoKit, so that the
// Go code of photokit is tested without Photos.
package bridge

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrNotSupported is returned by every method of the bridge of a build
	// without PhotoKit; photokit exports it.
	ErrNotSupported = errors.New("photokit: not supported on this platform")
	// ErrNoResource is returned by RequestResource when the asset has no
	// resource of the type; photokit exports it.
	ErrNoResource = errors.New("photokit: the asset has no resource of that type")
	// ErrCanceled is delivered by Done after Cancel, whatever PhotoKit reports
	// then.
	ErrCanceled = errors.New("photokit: the request was canceled")
)

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
	// AssetResources finds a hidden asset too; an unknown id gives none, not an error.
	AssetResources(id string) ([]ResourceRecord, error)
	// RequestResource starts requestDataForAssetResource with
	// networkAccessAllowed for the first resource of type typ of the asset id,
	// a hidden one too; no such asset or resource is ErrNoResource.
	RequestResource(id string, typ int, sink ResourceSink) (Request, error)
}

// ResourceSink receives a resource on a queue of PhotoKit. Write gets the
// chunks in order and does not keep p; after an error of Write the bridge
// calls it no more and the caller cancels the request. Progress gets the share
// received, from 0 to 1.
type ResourceSink interface {
	Write(p []byte) error
	Progress(fraction float64)
}

// Request is a running RequestResource. Done delivers at most once, without
// waiting for a receiver: nil, a *PhotoKitError, or ErrCanceled after Cancel,
// since PhotoKit calls the completion handler after cancelDataRequest: too,
// unless a stalled iCloud download holds it back. A second Cancel does nothing.
type Request interface {
	Cancel()
	Done() <-chan error
}

// PhotoKitError is the NSError of a completion handler by its domain and code
// only: its description may name a file.
type PhotoKitError struct {
	Domain string
	Code   int64
}

// Error implements error.
func (e *PhotoKitError) Error() string { return fmt.Sprintf("%s code %d", e.Domain, e.Code) }

// ResourceRecord is a PHAssetResource as the bridge reads it; a string is ""
// when PhotoKit gives none.
type ResourceRecord struct {
	Type                  int
	OriginalFilename, UTI string
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
