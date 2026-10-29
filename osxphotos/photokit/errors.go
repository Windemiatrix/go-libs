package photokit

import (
	"errors"
	"fmt"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Status is PHAuthorizationStatus of the process for read-write access to the
// Photos library (photokit.py:120-121, 139-160).
type Status int

// The values of PHAuthorizationStatus.
const (
	StatusNotDetermined Status = iota // the user has not been asked
	StatusRestricted
	StatusDenied
	StatusAuthorized
	StatusLimited // a selection of photos; PhotoKit on macOS reports it, Photos does not offer it
)

var statusNames = [...]string{"not determined", "restricted", "denied", "authorized", "limited"}

// String returns Status(N) for a value without a constant.
func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return fmt.Sprintf("Status(%d)", int(s))
	}
	return statusNames[s]
}

var (
	// ErrNotSupported is returned by every call of a build without the
	// PhotoKit bridge.
	ErrNotSupported = bridge.ErrNotSupported
	// ErrNotAuthorized is wrapped by the error of Open when the process may
	// not access the whole library (PhotoKitAuthError, photokit.py:174-175,
	// 1200-1204).
	ErrNotAuthorized = errors.New("photokit: access to the Photos library is not authorized")
)
