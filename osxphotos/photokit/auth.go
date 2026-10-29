package photokit

import (
	"context"
	"fmt"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// PHOTOKIT_REQUEST_TIMEOUT of upstream (photokit.py:196-203); a deadline of
// ctx replaces it.
const requestTimeout = 5 * time.Minute

// AuthorizationStatus is the status of this process. It never shows a dialog
// (check_photokit_authorization, photokit.py:113-121).
func AuthorizationStatus() (Status, error) { return authorizationStatus(bridge.New()) }

// RequestAuthorization asks the user for read-write access once and returns
// the status they chose (request_photokit_authorization, photokit.py:124-162;
// requestAuthorizationForAccessLevel:handler:, the API that replaced the
// requestAuthorization: upstream calls). The process needs
// NSPhotoLibraryUsageDescription in its Info.plist: without it macOS ends the
// process. It returns ctx.Err() as soon as ctx is done, and
// context.DeadlineExceeded after 5 minutes when ctx has no deadline (the 300 s
// of PHOTOKIT_REQUEST_TIMEOUT, photokit.py:196-203). The request itself keeps
// waiting in the background until the user answers or that deadline passes,
// and the late answer is dropped.
func RequestAuthorization(ctx context.Context) (Status, error) {
	return requestAuthorization(ctx, bridge.New())
}

func authorizationStatus(b bridge.Bridge) (Status, error) {
	return toStatus(b.AuthorizationStatus())
}

func requestAuthorization(ctx context.Context, b bridge.Bridge) (Status, error) {
	if err := ctx.Err(); err != nil {
		return StatusNotDetermined, err
	}
	timeout := requestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	type result struct {
		status int
		err    error
	}
	// The bridge cannot be interrupted: the buffer lets the goroutine end when
	// the bridge returns, after nobody waits for the answer.
	done := make(chan result, 1)
	go func() {
		s, err := b.RequestAuthorization(timeout)
		done <- result{s, err}
	}()
	select {
	case <-ctx.Done():
		return StatusNotDetermined, ctx.Err()
	case r := <-done:
		return toStatus(r.status, r.err)
	}
}

// A newer macOS may add a status; it is reported, not guessed.
func toStatus(s int, err error) (Status, error) {
	switch {
	case err != nil:
		return StatusNotDetermined, err
	case s < 0 || s >= len(statusNames):
		return StatusNotDetermined, fmt.Errorf("photokit: unknown authorization status %d", s)
	}
	return Status(s), nil
}
