//go:build darwin && cgo

package bridge

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
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

// The handle is far above those RequestResource gives; a call after the
// completion must not panic, as the Value of a deleted cgo.Handle does.
func TestRequestHandlers(t *testing.T) {
	const h = 1 << 40
	s := &recordingSink{}
	r := &cgoRequest{sink: s, done: make(chan error, 1)}
	requests.Store(uintptr(h), r)
	pk_go_write(h, nil, 0)
	pk_go_progress(h, 0.5)
	pk_go_done(h, 0, nil)
	pk_go_progress(h, 1)
	pk_go_write(h, nil, 0)
	if err := <-r.done; err != nil {
		t.Errorf("Done = %v, want nil", err)
	}
	if s.writes != 0 || s.progress != 1 {
		t.Errorf("sink got %d writes and %d progress values, want 0 and 1", s.writes, s.progress)
	}
	defer func() {
		if recover() == nil {
			t.Error("a second pk_go_done did not panic")
		}
	}()
	pk_go_done(h, 0, nil)
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

// The call returns before PhotoKit, which the tests must not reach.
func TestFetchAssetsNone(t *testing.T) {
	if got, err := New().FetchAssets(nil); got != nil || err != nil {
		t.Errorf("FetchAssets(nil) = %v, %v, want nil, nil", got, err)
	}
}
