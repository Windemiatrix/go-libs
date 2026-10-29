package photokit

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

func TestOpen(t *testing.T) {
	t.Run("authorized", func(t *testing.T) {
		b := &fake.Bridge{Status: int(StatusAuthorized)}
		lib, err := open(b)
		if err != nil {
			t.Fatal(err)
		}
		if lib == nil || lib.b != bridge.Bridge(b) {
			t.Errorf("open = %v, want a library on the bridge it was given", lib)
		}
	})
	for _, s := range []Status{StatusNotDetermined, StatusRestricted, StatusDenied, StatusLimited} {
		t.Run(s.String(), func(t *testing.T) {
			b := &fake.Bridge{Status: int(s), RequestStatus: int(StatusAuthorized)}
			lib, err := open(b)
			if lib != nil || !errors.Is(err, ErrNotAuthorized) {
				t.Fatalf("open = %v, %v; want ErrNotAuthorized", lib, err)
			}
			if !strings.Contains(err.Error(), s.String()) {
				t.Errorf("err = %v, want the status %s in it", err, s)
			}
			if want := []string{"AuthorizationStatus"}; !slices.Equal(b.Calls(), want) {
				t.Errorf("calls = %q, want %q: Open never asks the user", b.Calls(), want)
			}
		})
	}
	t.Run("not supported", func(t *testing.T) {
		lib, err := open(&fake.Bridge{StatusErr: bridge.ErrNotSupported})
		if lib != nil || !errors.Is(err, ErrNotSupported) {
			t.Errorf("open = %v, %v; want ErrNotSupported", lib, err)
		}
	})
}

func TestExportedFunctions(t *testing.T) {
	s, err := AuthorizationStatus()
	wantS, wantErr := authorizationStatus(bridge.New())
	if s != wantS || fmt.Sprint(err) != fmt.Sprint(wantErr) {
		t.Errorf("AuthorizationStatus() = %v, %v; want %v, %v", s, err, wantS, wantErr)
	}
	lib, err := Open()
	wantLib, wantOpenErr := open(bridge.New())
	if (lib == nil) != (wantLib == nil) || fmt.Sprint(err) != fmt.Sprint(wantOpenErr) {
		t.Errorf("Open() = %v, %v; want %v, %v", lib, err, wantLib, wantOpenErr)
	}
	// Only the stub may be asked: PhotoKit would show a dialog, and macOS ends
	// a test binary that has no NSPhotoLibraryUsageDescription.
	if errors.Is(wantErr, ErrNotSupported) {
		if _, err := RequestAuthorization(t.Context()); !errors.Is(err, ErrNotSupported) {
			t.Errorf("RequestAuthorization: err = %v, want ErrNotSupported", err)
		}
	}
}
