//go:build darwin && cgo

package photokit

import (
	"errors"
	"os"
	"testing"
)

// RequestAuthorization is never called here: without
// NSPhotoLibraryUsageDescription macOS ends the process, and with it the call
// waits for a dialog nobody answers on a runner.
func TestCGOAuthorizationStatus(t *testing.T) {
	s, err := AuthorizationStatus()
	if err != nil {
		t.Fatalf("AuthorizationStatus: %v", err)
	}
	t.Logf("authorization status: %s", s)
	// The runner never grants Photos access; another Mac may have granted it to
	// its Terminal, so the value is pinned on the runner only.
	if os.Getenv("GITHUB_ACTIONS") == "true" && s != StatusNotDetermined {
		t.Errorf("status on the runner = %s, want %s", s, StatusNotDetermined)
	}
	_, err = Open()
	if s == StatusAuthorized && err != nil {
		t.Errorf("Open with status %s: %v", s, err)
	}
	if s != StatusAuthorized && !errors.Is(err, ErrNotAuthorized) {
		t.Errorf("Open with status %s: err = %v, want ErrNotAuthorized", s, err)
	}
}
