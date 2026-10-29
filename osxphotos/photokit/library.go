package photokit

import (
	"fmt"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Library is the system Photos library of the user (PhotoLibrary,
// photokit.py:1185-1207).
type Library struct{ b bridge.Bridge }

// Open returns the library when the process is authorized and
// ErrNotAuthorized otherwise, for StatusLimited too: a limited library shows
// only the selected items. It does not ask the user and does not wait: call
// RequestAuthorization first. Upstream asks in the constructor
// (photokit.py:1199-1204).
func Open() (*Library, error) { return open(bridge.New()) }

func open(b bridge.Bridge) (*Library, error) {
	s, err := authorizationStatus(b)
	if err != nil {
		return nil, err
	}
	if s != StatusAuthorized {
		return nil, fmt.Errorf("%w: status %s", ErrNotAuthorized, s)
	}
	return &Library{b}, nil
}
