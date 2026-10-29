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
	if got, err := b.FetchAssets([]string{"id"}); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("FetchAssets = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.FetchBurst("id", true); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("FetchBurst = %v, %v; want ErrNotSupported", got, err)
	}
}
