// Package fake is the bridge of the tests of photokit.
package fake

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Bridge answers with its fields; the zero value answers status 0 at once.
// RequestDelay longer than the timeout gives context.DeadlineExceeded at
// once, so that a test does not wait for it. Hold, when not nil, keeps
// RequestAuthorization waiting until it is closed or the timeout passes, as an
// open dialog does.
type Bridge struct {
	Status, RequestStatus int
	StatusErr, RequestErr error
	RequestDelay          time.Duration
	Hold                  chan struct{}

	mu    sync.Mutex
	calls []string
}

var _ bridge.Bridge = (*Bridge)(nil)

// Calls returns a line per call so far, in order. It may be called while a
// call runs: photokit calls RequestAuthorization from a goroutine.
func (b *Bridge) Calls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.calls)
}

func (b *Bridge) log(call string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, call)
}

// AuthorizationStatus implements bridge.Bridge.
func (b *Bridge) AuthorizationStatus() (int, error) {
	b.log("AuthorizationStatus")
	return b.Status, b.StatusErr
}

// RequestAuthorization implements bridge.Bridge.
func (b *Bridge) RequestAuthorization(timeout time.Duration) (int, error) {
	b.log("RequestAuthorization " + timeout.String())
	if b.Hold != nil {
		select {
		case <-b.Hold:
		case <-time.After(timeout):
			return 0, context.DeadlineExceeded
		}
	}
	if b.RequestDelay > timeout {
		return 0, context.DeadlineExceeded
	}
	time.Sleep(b.RequestDelay)
	return b.RequestStatus, b.RequestErr
}
