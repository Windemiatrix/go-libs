//go:build darwin && cgo

package bridge

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"
)

type recordingSink struct {
	writes, progress int
	err              error
}

func (s *recordingSink) Write([]byte) error {
	s.writes++
	return s.err
}

func (s *recordingSink) Progress(float64) { s.progress++ }

func TestRequestWrite(t *testing.T) {
	s := &recordingSink{err: errors.New("disk full")}
	r := &cgoRequest{sink: s}
	r.write([]byte("a"))
	r.write([]byte("b"))
	if s.writes != 1 {
		t.Errorf("Write called %d times after it failed once, want 1", s.writes)
	}
}

func TestRequestFinish(t *testing.T) {
	for _, tt := range []struct {
		name             string
		canceled, failed bool
		domain           string
		code             int64
		want             error
	}{
		{"success", false, false, "", 0, nil},
		{"NSError", false, true, "PHPhotosErrorDomain", 3164, &PhotoKitError{"PHPhotosErrorDomain", 3164}},
		{"NSError of code 0", false, true, "NSCocoaErrorDomain", 0, &PhotoKitError{"NSCocoaErrorDomain", 0}},
		{"domain not UTF-8", false, true, "\xff", 1, &PhotoKitError{"", 1}},
		{"canceled with an NSError", true, true, "PHPhotosErrorDomain", 3072, ErrCanceled},
		{"canceled without an NSError", true, false, "", 0, ErrCanceled},
		{"no image data", false, false, "", 3, ErrNoResource},
		{"canceled without image data", true, false, "", 3, ErrCanceled},
		{"exception in a handler", false, false, "", 1, errors.New("photokit: Objective-C exception: ")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &cgoRequest{done: make(chan error, 1)}
			r.canceled.Store(tt.canceled)
			r.finish(tt.failed, tt.domain, tt.code)
			if got := <-r.done; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("finish(%t, %q, %d) = %v, want %v", tt.failed, tt.domain, tt.code, got, tt.want)
			}
		})
	}
}

func TestChangeResult(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001/L0/001"
	for _, tt := range []struct {
		name       string
		failed     bool
		domain     string
		code       int64
		id, wantID string
		wantErr    error
	}{
		{"refused", true, "PHPhotosErrorDomain", 3072, "", "", &PhotoKitError{"PHPhotosErrorDomain", 3072}},
		{"failed without an NSError", true, "", 0, "", "", &PhotoKitError{"", 0}},
		{"domain not UTF-8", true, "\xff", 1, "", "", &PhotoKitError{"", 1}},
		{"failed with an identifier", true, "PHPhotosErrorDomain", 3300, id, "", &PhotoKitError{"PHPhotosErrorDomain", 3300}},
		{"created", false, "", 0, id, id, nil},
		{"identifier not UTF-8", false, "", 0, "\xff", "", errors.New("photokit: a string from PhotoKit is not valid UTF-8")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := changeResult(tt.failed, tt.domain, tt.code, tt.id)
			if got != tt.wantID || !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("changeResult(%t, %q, %d, %q) = %q, %v, want %q, %v", tt.failed, tt.domain, tt.code, tt.id, got, err, tt.wantID, tt.wantErr)
			}
		})
	}
}

// A string that breaks the rule of bridge.h is a bug of photokit: it must
// raise before performChanges, which the tests must not reach.
const brokenRule = "photokit: Objective-C exception: NSInvalidArgumentException"

func TestImportBeforeChange(t *testing.T) {
	r := ImportRecord{Photo: "/x/IMG_0001.JPG", Albums: []string{"\xff"}}
	if id, err := New().Import(r, time.Second); id != "" || err == nil || err.Error() != brokenRule {
		t.Errorf("Import with an album not UTF-8 = %q, %v, want %q", id, err, brokenRule)
	}
}

// Each call breaks the rule with one string, and PhotoKit has nothing the others
// name, so a deleted check fails here without changing the library.
func TestCollectionChangesBeforeChange(t *testing.T) {
	b := New()
	titleID, titleErr := b.CreateCollection("\xff", "Trips", true, time.Second)
	parentID, parentErr := b.CreateCollection("Trips", "\xff", false, time.Second)
	if titleID != "" || parentID != "" {
		t.Errorf("CreateCollection = %q and %q, want no identifier", titleID, parentID)
	}
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"title not UTF-8", titleErr},
		{"parent not UTF-8", parentErr},
		{"new title not UTF-8", b.RenameCollection("Trips", "\xff", time.Second)},
		{"collection not UTF-8", b.RenameCollection("\xff", "Trips", time.Second)},
		{"one collection to delete not UTF-8", b.DeleteCollections([]string{"\xff"}, time.Second)},
		{"album not UTF-8", b.AlbumAssets("\xff", []string{"x"}, true, time.Second)},
		{"asset not UTF-8", b.AlbumAssets("Trips", []string{"\xff"}, false, time.Second)},
		{"asset to delete not UTF-8", b.DeleteAssets([]string{"\xff"}, time.Second)},
		// Favorite, not Hidden: a broken check must not reach a dialog.
		{"asset to update not UTF-8", b.UpdateAsset("\xff", PatchRecord{HasFavorite: true}, time.Second)},
	} {
		if tt.err == nil || tt.err.Error() != brokenRule {
			t.Errorf("%s: error %v, want %q", tt.name, tt.err, brokenRule)
		}
	}
}

// The calls return before PhotoKit, which the tests must not reach.
func TestCollectionChangesNone(t *testing.T) {
	b := New()
	if err := b.DeleteCollections(nil, time.Second); err != nil {
		t.Errorf("DeleteCollections(nil) = %v, want nil", err)
	}
	if err := b.DeleteAssets(nil, time.Second); err != nil {
		t.Errorf("DeleteAssets(nil) = %v, want nil", err)
	}
	if err := b.AlbumAssets("\xff", nil, true, time.Second); err != nil {
		t.Errorf("AlbumAssets with no asset = %v, want nil", err)
	}
}

// The handle is far above those RequestResource gives; a call after the
// completion must not panic, as the Value of a deleted cgo.Handle does.
func TestRequestHandlers(t *testing.T) {
	const h = 1 << 40
	s := &recordingSink{}
	paths := 0
	r := &cgoRequest{sink: s, path: func(string) { paths++ }, done: make(chan error, 1)}
	requests.Store(uintptr(h), r)
	pk_go_write(h, nil, 0)
	pk_go_progress(h, 0.5)
	pk_go_video(h, nil)
	pk_go_done(h, 0, nil)
	if !bool(pk_go_progress(h, 1)) {
		t.Error("pk_go_progress of an ended request does not ask to stop")
	}
	pk_go_write(h, nil, 0)
	pk_go_image(h, nil, 0, nil)
	pk_go_video(h, nil)
	if err := <-r.done; err != nil {
		t.Errorf("Done = %v, want nil", err)
	}
	if s.writes != 0 || s.progress != 1 {
		t.Errorf("sink got %d writes and %d progress values, want 0 and 1", s.writes, s.progress)
	}
	if paths != 0 {
		t.Errorf("path called %d times for a NULL path and after the completion, want 0", paths)
	}
	defer func() {
		if recover() == nil {
			t.Error("a second pk_go_done did not panic")
		}
	}()
	pk_go_done(h, 0, nil)
}

// An image request runs on a global queue and holds its thread until PhotoKit
// returns, so it has no id for cancelImageRequest: and only the answer of
// pk_go_progress stops it.
func TestRequestStop(t *testing.T) {
	const h = 1<<40 + 1
	r := &cgoRequest{sink: &recordingSink{}, done: make(chan error, 1)}
	requests.Store(uintptr(h), r)
	defer requests.Delete(uintptr(h))
	if bool(pk_go_progress(h, 0.5)) {
		t.Error("pk_go_progress before Cancel asks to stop")
	}
	r.Cancel()
	r.Cancel()
	if !bool(pk_go_progress(h, 0.6)) {
		t.Error("pk_go_progress after Cancel does not ask to stop")
	}
}

func TestRequestCancel(t *testing.T) {
	runs := 0
	r := &cgoRequest{cancel: func() { runs++ }}
	r.Cancel()
	r.Cancel()
	if runs != 1 {
		t.Errorf("two calls of Cancel ran the cancel %d times, want 1", runs)
	}
}

// The source is longer than the limit, so that a copy without the check
// returns bytes instead of reading past the buffer.
func TestMetadataBytes(t *testing.T) {
	b := make([]byte, 1<<20+1)
	b[0] = 'b'
	p := unsafe.Pointer(&b[0]) //nolint:gosec // G103: a Go buffer of known length stands for the plist of bridge.m
	got := metadataBytes(p, 1<<20)
	if !bytes.Equal(got, b[:1<<20]) {
		t.Errorf("metadataBytes of 1 MiB = %d bytes, want a copy of 1 MiB", len(got))
	}
	b[0] = 'c'
	if len(got) > 0 && got[0] != 'b' {
		t.Error("metadataBytes shares its result with the buffer of bridge.m, which is freed after pk_go_image")
	}
	if got := metadataBytes(p, 1<<20+1); got != nil {
		t.Errorf("metadataBytes over 1 MiB = %d bytes, want nil", len(got))
	}
	if got := metadataBytes(nil, 0); got != nil {
		t.Errorf("metadataBytes(nil) = %v, want nil", got)
	}
}

// The reads raise with the reason of the exception, not its name as the
// changes do.
const brokenRuleOfRead = "photokit: Objective-C exception: "

// An id that breaks the rule of bridge.h must make the call raise, not return
// nothing as for an asset that is not found.
func TestReadsBeforePhotoKit(t *testing.T) {
	known := func() (n int) {
		requests.Range(func(_, _ any) bool {
			n++
			return true
		})
		return n
	}
	before := known()
	b := New()
	image, imageErr := b.RequestImage("\xff", 0, nil)
	resource, resourceErr := b.RequestResource("\xff", 1, nil)
	video, videoErr := b.RequestVideo("\xff", 1, nil)
	if image != nil || resource != nil || video != nil {
		t.Errorf("requests of an id not UTF-8 = %v, %v, %v; want none", image, resource, video)
	}
	if after := known(); after != before {
		t.Errorf("%d requests known after three that did not start, want %d", after, before)
	}
	resources, resourcesErr := b.AssetResources("\xff")
	assets, assetsErr := b.FetchAssets([]string{"\xff"})
	bursts, burstErr := b.FetchBurst("\xff", false)
	if resources != nil || assets != nil || bursts != nil {
		t.Errorf("AssetResources, FetchAssets and FetchBurst of an id not UTF-8 = %v, %v, %v; want none", resources, assets, bursts)
	}
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"RequestImage", imageErr},
		{"RequestResource", resourceErr},
		{"RequestVideo", videoErr},
		{"AssetResources", resourcesErr},
		{"FetchAssets", assetsErr},
		{"FetchBurst", burstErr},
	} {
		if tt.err == nil || !strings.HasPrefix(tt.err.Error(), brokenRuleOfRead) {
			t.Errorf("%s of an id not UTF-8: error %v, want the prefix %q", tt.name, tt.err, brokenRuleOfRead)
		}
	}
}

// The codes are literals, so that a new order of the enum in bridge.h fails
// here.
func TestCodeError(t *testing.T) {
	if err := codeError(0, ""); err != nil {
		t.Errorf("codeError(0) = %v, want nil", err)
	}
	if err := codeError(2, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("codeError(2) = %v, want context.DeadlineExceeded", err)
	}
	if err := codeError(3, ""); !errors.Is(err, ErrNoResource) {
		t.Errorf("codeError(3) = %v, want ErrNoResource", err)
	}
	if err := codeError(4, ""); !errors.Is(err, ErrNoAlbum) {
		t.Errorf("codeError(4) = %v, want ErrNoAlbum", err)
	}
	if err := codeError(5, ""); !errors.Is(err, ErrNoAsset) {
		t.Errorf("codeError(5) = %v, want ErrNoAsset", err)
	}
	for _, tt := range []struct{ name, message, want string }{
		{"valid", "IMG_0001.HEIC caf\U000000e9", "photokit: Objective-C exception: IMG_0001.HEIC caf\U000000e9"},
		{"empty", "", "photokit: Objective-C exception: "},
		{"invalid bytes", "IMG_\xff\xfe.HEIC", "photokit: a string from PhotoKit is not valid UTF-8"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := codeError(1, tt.message); err == nil || err.Error() != tt.want {
				t.Errorf("codeError(1, %q) = %v, want %q", tt.message, err, tt.want)
			}
		})
	}
}

func TestDate(t *testing.T) {
	if got := date(false, 1); got != nil {
		t.Errorf("date(false, 1) = %v, want nil", got)
	}
	for _, tt := range []struct {
		sec  float64
		want time.Time
	}{
		{0, time.Unix(0, 0)},
		{1700000000.123456, time.UnixMicro(1700000000123456)},
		{-1.5, time.UnixMicro(-1500000)},
		{1700000000.0000007, time.UnixMicro(1700000000000001)},
	} {
		if got := date(true, tt.sec); got == nil || !got.Equal(tt.want) {
			t.Errorf("date(true, %v) = %v, want %v", tt.sec, got, tt.want)
		}
	}
}

func TestDuration(t *testing.T) {
	for _, tt := range []struct {
		sec  float64
		want time.Duration
	}{
		{0, 0},
		{1.5, 1500 * time.Millisecond},
		{1.0000000006, time.Second + time.Nanosecond},
	} {
		if got := duration(tt.sec); got != tt.want {
			t.Errorf("duration(%v) = %v, want %v", tt.sec, got, tt.want)
		}
	}
}

// With nil ids the calls return nothing, and an id that is not UTF-8 raises:
// both before PhotoKit, which the tests must not reach.
func TestIdentifierPairsBeforePhotoKit(t *testing.T) {
	b := New()
	for name, call := range map[string]func([]string) ([]IdentifierPair, error){"CloudIdentifiers": b.CloudIdentifiers, "LocalIdentifiers": b.LocalIdentifiers} {
		if got, err := call(nil); got != nil || err != nil {
			t.Errorf("%s(nil) = %v, %v, want nil, nil", name, got, err)
		}
		if got, err := call([]string{"\xff"}); got != nil || err == nil || err.Error() != brokenRule {
			t.Errorf("%s of an id not UTF-8 = %v, %v, want %q", name, got, err, brokenRule)
		}
	}
}

// The call returns before PhotoKit reads the library, which the tests must not
// reach.
func TestFetchAssetsNone(t *testing.T) {
	if got, err := New().FetchAssets(nil); got != nil || err != nil {
		t.Errorf("FetchAssets(nil) = %v, %v, want nil, nil", got, err)
	}
}
