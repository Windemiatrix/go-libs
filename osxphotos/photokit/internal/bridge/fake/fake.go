// Package fake is the bridge of the tests of photokit.
package fake

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
//
// RequestImage logs the id and the version and returns bridge.ErrNoResource
// when Images has no entry for id+"/"+version. An entry without Data delivers
// bridge.ErrNoResource by Done without Write and Info, as PhotoKit does when
// it gives no data; otherwise it runs as RequestResource does, with the data
// in one chunk, and gives the Record to Info after the data.
//
// RequestVideo logs the id and the version and returns bridge.ErrNoResource
// when Videos has no entry for id+"/"+version. Otherwise it runs as
// RequestResource does without data, and gives the entry to path unless it is
// "", as PhotoKit gives no file of an AVComposition.
//
// ExportComposition logs the id and the version. An entry of Videos for
// id+"/"+version other than "" is the file of an AVURLAsset: it runs as
// RequestVideo does and gives the entry to file. Otherwise, from a goroutine,
// it delivers bridge.ErrNoComposition by Done when Compositions has no entry
// for id+"/"+version, as PhotoKit tells it only in its handler. Otherwise,
// before it returns, it keeps the directory of path and its mode for
// LastRenderDir and writes the entry into path, which must not exist, with
// mode 0644; then from a goroutine it delivers the error of the write, if any,
// or runs as RequestResource does without data.
//
// Import keeps the record and the timeout for LastImport and, when HoldImport
// is not nil, waits for it to be closed or the timeout to pass, as Hold does;
// then it returns ImportID and ImportErr.
//
// Collections returns CollectionRecords and CollectionsErr. The four changes
// of collections, DeleteAssets and UpdateAsset log their arguments, keep the
// timeout for LastTimeout, the slice of DeleteCollections, AlbumAssets or
// DeleteAssets as given for LastIDs and the record of UpdateAsset for
// LastPatch, and wait for HoldChange as Import does for HoldImport; then they
// return ChangeErr, and CreateCollection CreatedID when ChangeErr is nil.
// UpdateAsset logs the names of the fields its record sets.
//
// CloudIdentifiers and LocalIdentifiers log the number of ids and return
// PairsErr when it is set; otherwise the Pairs, in their order, whose Local is
// one of the ids or one of them followed by "/", as FetchAssets finds Assets,
// or whose Cloud is one of the cloudIDs.
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
	Images                map[string]ImageData
	Videos                map[string]string
	Compositions          map[string][]byte
	DoneErr               error
	HoldRequest           chan struct{}
	ImportID              string
	ImportErr             error
	HoldImport            chan struct{}
	CollectionRecords     []bridge.CollectionRecord
	CollectionsErr        error
	CreatedID             string
	ChangeErr             error
	HoldChange            chan struct{}
	Pairs                 []bridge.IdentifierPair
	PairsErr              error

	mu          sync.Mutex
	calls       []string
	sink        bridge.ResourceSink
	lastImport  bridge.ImportRecord
	lastTimeout time.Duration
	lastIDs     []string
	lastPatch   bridge.PatchRecord
	renderDir   string
	renderMode  fs.FileMode
}

var _ bridge.Bridge = (*Bridge)(nil)

// ImageData is what RequestImage gives for an entry of Images.
type ImageData struct {
	Data   []byte
	Record bridge.ImageRecord
}

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
	return b.start(id, sink, func(r *request) error { return r.run(data, b.Chunk, sink, nil) }), nil
}

// RequestImage implements bridge.Bridge.
func (b *Bridge) RequestImage(id string, version int, sink bridge.ImageSink) (bridge.Request, error) {
	b.log(fmt.Sprintf("RequestImage %s %d", id, version))
	img, ok := b.Images[id+"/"+strconv.Itoa(version)]
	if !ok {
		return nil, bridge.ErrNoResource
	}
	return b.start(id, sink, func(r *request) error {
		if len(img.Data) == 0 {
			return bridge.ErrNoResource
		}
		return r.run(img.Data, 0, sink, func() { sink.Info(img.Record) })
	}), nil
}

// RequestVideo implements bridge.Bridge.
func (b *Bridge) RequestVideo(id string, version int, path func(string)) (bridge.Request, error) {
	b.log(fmt.Sprintf("RequestVideo %s %d", id, version))
	p, ok := b.Videos[id+"/"+strconv.Itoa(version)]
	if !ok {
		return nil, bridge.ErrNoResource
	}
	return b.start(id, nil, func(r *request) error {
		return r.run(nil, 0, nil, func() {
			if p != "" {
				path(p)
			}
		})
	}), nil
}

// ExportComposition implements bridge.Bridge.
func (b *Bridge) ExportComposition(id string, version int, path string, file func(string)) (bridge.Request, error) {
	b.log(fmt.Sprintf("ExportComposition %s %d", id, version))
	key := id + "/" + strconv.Itoa(version)
	if p := b.Videos[key]; p != "" {
		return b.start(id, nil, func(r *request) error { return r.run(nil, 0, nil, func() { file(p) }) }), nil
	}
	err := bridge.ErrNoComposition
	if data, ok := b.Compositions[key]; ok {
		err = b.render(path, data)
	}
	return b.start(id, nil, func(r *request) error {
		if err != nil {
			return err
		}
		return r.run(nil, 0, nil, nil)
	}), nil
}

// The file is written before ExportComposition returns, so that a cancel never
// races the removal of its directory.
func (b *Bridge) render(path string, data []byte) error {
	fi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.renderDir, b.renderMode = filepath.Dir(path), fi.Mode()
	b.mu.Unlock()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G302, G304: the mode AVFoundation leaves under the default umask, so that a test tells the copy from the file of the session, at a path photokit made
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return cmp.Or(err, f.Close())
}

// LastRenderDir returns the directory of the path of the latest
// ExportComposition that had an entry of Compositions and its mode, "" and 0
// before one.
func (b *Bridge) LastRenderDir() (string, fs.FileMode) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.renderDir, b.renderMode
}

func (b *Bridge) start(id string, sink bridge.ResourceSink, run func(*request) error) *request {
	b.mu.Lock()
	b.sink = sink
	b.mu.Unlock()
	r := &request{b: b, id: id, canceled: make(chan struct{}), done: make(chan error, 1)}
	go func() { r.done <- run(r) }()
	return r
}

// LastSink returns the sink of the latest RequestResource or RequestImage, nil
// before one and after RequestVideo and ExportComposition.
func (b *Bridge) LastSink() bridge.ResourceSink {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sink
}

// Import implements bridge.Bridge.
func (b *Bridge) Import(r bridge.ImportRecord, timeout time.Duration) (string, error) {
	b.log(fmt.Sprintf("Import %s %d albums", cmp.Or(r.OriginalFilename, filepath.Base(cmp.Or(r.Photo, r.Video))), len(r.Albums)))
	b.mu.Lock()
	b.lastImport, b.lastTimeout = r, timeout
	b.mu.Unlock()
	if b.HoldImport != nil {
		select {
		case <-b.HoldImport:
		case <-time.After(timeout):
			return "", context.DeadlineExceeded
		}
	}
	return b.ImportID, b.ImportErr
}

// LastImport returns the record and the timeout of the latest Import, zero
// values before one.
func (b *Bridge) LastImport() (bridge.ImportRecord, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastImport, b.lastTimeout
}

// Collections implements bridge.Bridge.
func (b *Bridge) Collections() ([]bridge.CollectionRecord, error) {
	b.log("Collections")
	return b.CollectionRecords, b.CollectionsErr
}

// CreateCollection implements bridge.Bridge.
func (b *Bridge) CreateCollection(title, parent string, folder bool, timeout time.Duration) (string, error) {
	if err := b.change(fmt.Sprintf("CreateCollection %s %s folder=%t", title, parent, folder), nil, timeout); err != nil {
		return "", err
	}
	return b.CreatedID, nil
}

// RenameCollection implements bridge.Bridge.
func (b *Bridge) RenameCollection(id, title string, timeout time.Duration) error {
	return b.change(fmt.Sprintf("RenameCollection %s %s", id, title), nil, timeout)
}

// DeleteCollections implements bridge.Bridge.
func (b *Bridge) DeleteCollections(ids []string, timeout time.Duration) error {
	return b.change(fmt.Sprintf("DeleteCollections %d", len(ids)), ids, timeout)
}

// AlbumAssets implements bridge.Bridge.
func (b *Bridge) AlbumAssets(album string, assets []string, add bool, timeout time.Duration) error {
	return b.change(fmt.Sprintf("AlbumAssets %s %d add=%t", album, len(assets), add), assets, timeout)
}

// DeleteAssets implements bridge.Bridge.
func (b *Bridge) DeleteAssets(ids []string, timeout time.Duration) error {
	return b.change(fmt.Sprintf("DeleteAssets %d", len(ids)), ids, timeout)
}

// UpdateAsset implements bridge.Bridge.
func (b *Bridge) UpdateAsset(id string, r bridge.PatchRecord, timeout time.Duration) error {
	var set []string
	for _, f := range []struct {
		name string
		has  bool
	}{{"date", r.HasCreationDate}, {"favorite", r.HasFavorite}, {"hidden", r.HasHidden}, {"location", r.HasLocation}} {
		if f.has {
			set = append(set, f.name)
		}
	}
	b.mu.Lock()
	b.lastPatch = r
	b.mu.Unlock()
	return b.change("UpdateAsset "+id+" "+strings.Join(set, " "), nil, timeout)
}

// LastPatch returns the record of the latest UpdateAsset, the zero value before one.
func (b *Bridge) LastPatch() bridge.PatchRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastPatch
}

// CloudIdentifiers implements bridge.Bridge.
func (b *Bridge) CloudIdentifiers(ids []string) ([]bridge.IdentifierPair, error) {
	return b.pairs("CloudIdentifiers", ids, func(p bridge.IdentifierPair, id string) bool {
		return p.Local == id || strings.HasPrefix(p.Local, id+"/")
	})
}

// LocalIdentifiers implements bridge.Bridge.
func (b *Bridge) LocalIdentifiers(cloudIDs []string) ([]bridge.IdentifierPair, error) {
	return b.pairs("LocalIdentifiers", cloudIDs, func(p bridge.IdentifierPair, id string) bool { return p.Cloud == id })
}

func (b *Bridge) pairs(call string, ids []string, named func(bridge.IdentifierPair, string) bool) ([]bridge.IdentifierPair, error) {
	b.log(fmt.Sprintf("%s %d", call, len(ids)))
	if b.PairsErr != nil {
		return nil, b.PairsErr
	}
	return slices.DeleteFunc(slices.Clone(b.Pairs), func(p bridge.IdentifierPair) bool {
		return !slices.ContainsFunc(ids, func(id string) bool { return named(p, id) })
	}), nil
}

// LastTimeout returns the timeout of the latest Import or change, 0 before one.
func (b *Bridge) LastTimeout() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastTimeout
}

// LastIDs returns what the slice given to the latest change holds now: the ids
// of DeleteCollections and DeleteAssets, the assets of AlbumAssets, nil for the
// others and before one.
func (b *Bridge) LastIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.lastIDs)
}

// ids is kept as given, not copied, so that LastIDs shows a caller that
// changed the slice it passed.
func (b *Bridge) change(call string, ids []string, timeout time.Duration) error {
	b.mu.Lock()
	b.calls, b.lastIDs, b.lastTimeout = append(b.calls, call), ids, timeout
	b.mu.Unlock()
	if b.HoldChange != nil {
		select {
		case <-b.HoldChange:
		case <-time.After(timeout):
			return context.DeadlineExceeded
		}
	}
	return b.ChangeErr
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

func (r *request) run(data []byte, chunk int, sink bridge.ResourceSink, info func()) error {
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
	if info != nil {
		info()
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
