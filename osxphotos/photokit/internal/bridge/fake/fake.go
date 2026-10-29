// Package fake is the bridge of the tests of photokit.
package fake

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Bridge answers with its fields; the zero value answers status 0 at once.
// RequestDelay longer than the timeout gives context.DeadlineExceeded at
// once, so that a test does not wait for it. Hold, when not nil, keeps
// RequestAuthorization waiting until it is closed or the timeout passes, as an
// open dialog does. FetchAssets finds the Assets whose LocalIdentifier is one
// of the ids or one of them followed by "/", as PhotoKit finds "UUID/L0/001" by
// its UUID; FetchBurst finds those of the burst whatever all is. Both keep the
// order of Assets and return FetchErr when it is set. AssetResources returns
// Resources of the id exactly as given, and ResourcesErr.
//
// RequestResource returns bridge.ErrNoResource when Data has no entry for
// id+"/"+type, whatever Resources says. From a goroutine it writes the entry in
// chunks of Chunk bytes (0: one chunk), each followed by Progress with the
// share written; after a failed Write it writes no more and waits for Cancel,
// as PhotoKit does not stop by itself. Then it waits for HoldRequest to be
// closed when it is not nil, whatever Cancel, as a completion that never comes.
// Done delivers DoneErr, or bridge.ErrCanceled after Cancel.
type Bridge struct {
	Status, RequestStatus int
	StatusErr, RequestErr error
	RequestDelay          time.Duration
	Hold                  chan struct{}
	Assets                []bridge.AssetRecord
	FetchErr              error
	Resources             map[string][]bridge.ResourceRecord
	ResourcesErr          error
	Data                  map[string][]byte
	Chunk                 int
	DoneErr               error
	HoldRequest           chan struct{}

	mu    sync.Mutex
	calls []string
	sink  bridge.ResourceSink
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

// FetchAssets implements bridge.Bridge.
func (b *Bridge) FetchAssets(ids []string) ([]bridge.AssetRecord, error) {
	b.log("FetchAssets " + strings.Join(ids, " "))
	return b.except(func(r bridge.AssetRecord) bool {
		return !slices.ContainsFunc(ids, func(id string) bool {
			return r.LocalIdentifier == id || strings.HasPrefix(r.LocalIdentifier, id+"/")
		})
	})
}

// FetchBurst implements bridge.Bridge.
func (b *Bridge) FetchBurst(burstID string, all bool) ([]bridge.AssetRecord, error) {
	b.log(fmt.Sprintf("FetchBurst %s %t", burstID, all))
	return b.except(func(r bridge.AssetRecord) bool { return r.BurstIdentifier != burstID })
}

// AssetResources implements bridge.Bridge.
func (b *Bridge) AssetResources(id string) ([]bridge.ResourceRecord, error) {
	b.log("AssetResources " + id)
	return b.Resources[id], b.ResourcesErr
}

// RequestResource implements bridge.Bridge.
func (b *Bridge) RequestResource(id string, typ int, sink bridge.ResourceSink) (bridge.Request, error) {
	b.log(fmt.Sprintf("RequestResource %s %d", id, typ))
	data, ok := b.Data[id+"/"+strconv.Itoa(typ)]
	if !ok {
		return nil, bridge.ErrNoResource
	}
	b.mu.Lock()
	b.sink = sink
	b.mu.Unlock()
	r := &request{b: b, id: id, canceled: make(chan struct{}), done: make(chan error, 1)}
	go func() { r.done <- r.run(data, sink) }()
	return r, nil
}

// LastSink returns the sink of the latest RequestResource, nil before one.
func (b *Bridge) LastSink() bridge.ResourceSink {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sink
}

type request struct {
	b        *Bridge
	id       string
	once     sync.Once
	canceled chan struct{}
	done     chan error
}

func (r *request) Cancel() {
	r.once.Do(func() {
		r.b.log("Cancel " + r.id)
		close(r.canceled)
	})
}

func (r *request) Done() <-chan error { return r.done }

func (r *request) run(data []byte, sink bridge.ResourceSink) error {
	chunk := r.b.Chunk
	if chunk <= 0 {
		chunk = max(len(data), 1)
	}
	written := 0
	for c := range slices.Chunk(data, chunk) {
		if r.isCanceled() {
			return bridge.ErrCanceled
		}
		if sink.Write(c) != nil {
			<-r.canceled
			return bridge.ErrCanceled
		}
		written += len(c)
		sink.Progress(float64(written) / float64(len(data)))
	}
	if r.b.HoldRequest != nil {
		<-r.b.HoldRequest
	}
	if r.isCanceled() {
		return bridge.ErrCanceled
	}
	return r.b.DoneErr
}

func (r *request) isCanceled() bool {
	select {
	case <-r.canceled:
		return true
	default:
		return false
	}
}

func (b *Bridge) except(drop func(bridge.AssetRecord) bool) ([]bridge.AssetRecord, error) {
	if b.FetchErr != nil {
		return nil, b.FetchErr
	}
	return slices.DeleteFunc(slices.Clone(b.Assets), drop), nil
}
