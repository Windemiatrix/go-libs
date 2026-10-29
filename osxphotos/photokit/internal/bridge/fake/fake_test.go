package fake

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestBridge(t *testing.T) {
	t.Run("answers with its fields", func(t *testing.T) {
		statusErr, requestErr := errors.New("status failed"), errors.New("request failed")
		b := &Bridge{Status: 2, StatusErr: statusErr, RequestStatus: 3, RequestErr: requestErr}
		if s, err := b.AuthorizationStatus(); s != 2 || !errors.Is(err, statusErr) {
			t.Errorf("AuthorizationStatus = %d, %v; want 2, %v", s, err, statusErr)
		}
		if s, err := b.RequestAuthorization(time.Minute); s != 3 || !errors.Is(err, requestErr) {
			t.Errorf("RequestAuthorization = %d, %v; want 3, %v", s, err, requestErr)
		}
		if want := []string{"AuthorizationStatus", "RequestAuthorization 1m0s"}; !slices.Equal(b.Calls(), want) {
			t.Errorf("calls = %q, want %q", b.Calls(), want)
		}
	})
	t.Run("hold closed", func(t *testing.T) {
		b := &Bridge{Hold: make(chan struct{}), RequestStatus: 3}
		close(b.Hold)
		if s, err := b.RequestAuthorization(time.Hour); s != 3 || err != nil {
			t.Errorf("RequestAuthorization = %d, %v; want 3, nil", s, err)
		}
	})
	t.Run("hold open", func(t *testing.T) {
		b := &Bridge{Hold: make(chan struct{}), RequestStatus: 3}
		if _, err := b.RequestAuthorization(10 * time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
	})
	t.Run("delay longer than the timeout", func(t *testing.T) {
		start := time.Now()
		_, err := (&Bridge{RequestDelay: time.Hour, RequestStatus: 3}).RequestAuthorization(time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
			t.Errorf("err = %v after %v, want context.DeadlineExceeded at once", err, time.Since(start))
		}
	})
	t.Run("delay equal to the timeout", func(t *testing.T) {
		b := &Bridge{RequestDelay: time.Millisecond, RequestStatus: 3}
		if s, err := b.RequestAuthorization(time.Millisecond); s != 3 || err != nil {
			t.Errorf("RequestAuthorization = %d, %v; want 3, nil", s, err)
		}
	})
}
