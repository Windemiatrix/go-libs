//go:build darwin && cgo

package bridge

import (
	"context"
	"errors"
	"testing"
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
