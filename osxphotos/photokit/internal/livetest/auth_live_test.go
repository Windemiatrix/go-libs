//go:build photokit_live

package livetest

import (
	"context"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
)

func TestLiveAuthorization(t *testing.T) {
	s, err := photokit.AuthorizationStatus()
	if err != nil {
		t.Fatalf("AuthorizationStatus: %v", err)
	}
	t.Logf("authorization status: %s", s)
	if s != photokit.StatusAuthorized {
		// The first request shows a dialog that the owner answers by hand.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		if s, err = photokit.RequestAuthorization(ctx); err != nil {
			t.Fatalf("RequestAuthorization: %v", err)
		}
		t.Logf("status after the request: %s", s)
		if got, err := photokit.AuthorizationStatus(); err != nil || got != s {
			t.Errorf("request returned %s, status afterwards %s, %v", s, got, err)
		}
	}
	if _, err := photokit.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
}
