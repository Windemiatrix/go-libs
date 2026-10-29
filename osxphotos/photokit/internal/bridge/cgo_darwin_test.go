//go:build darwin && cgo

package bridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The codes are literals, so that a new order of the enum in bridge.h fails
// here.
func TestCodeError(t *testing.T) {
	if err := codeError(0, ""); err != nil {
		t.Errorf("codeError(0) = %v, want nil", err)
	}
	if err := codeError(2, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("codeError(2) = %v, want context.DeadlineExceeded", err)
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
