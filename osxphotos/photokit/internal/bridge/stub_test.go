//go:build !(darwin && cgo)

package bridge

import (
	"errors"
	"testing"
	"time"
)

func TestStubNotSupported(t *testing.T) {
	b := New()
	if _, err := b.AuthorizationStatus(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("AuthorizationStatus: err = %v, want ErrNotSupported", err)
	}
	if _, err := b.RequestAuthorization(time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("RequestAuthorization: err = %v, want ErrNotSupported", err)
	}
}
