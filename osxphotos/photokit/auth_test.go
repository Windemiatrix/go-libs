package photokit

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

func TestAuthorizationStatus(t *testing.T) {
	for _, tt := range []struct {
		raw  int
		want Status
		name string
	}{
		{0, StatusNotDetermined, "not determined"},
		{1, StatusRestricted, "restricted"},
		{2, StatusDenied, "denied"},
		{3, StatusAuthorized, "authorized"},
		{4, StatusLimited, "limited"},
	} {
		got, err := authorizationStatus(&fake.Bridge{Status: tt.raw})
		if err != nil || got != tt.want || got.String() != tt.name {
			t.Errorf("authorizationStatus(%d) = %v (%d), %v; want %s (%d)", tt.raw, got, int(got), err, tt.name, int(tt.want))
		}
	}
	for _, raw := range []int{-1, 5} {
		if _, err := authorizationStatus(&fake.Bridge{Status: raw}); err == nil || !strings.Contains(err.Error(), fmt.Sprint(raw)) {
			t.Errorf("authorizationStatus(%d): err = %v, want an unknown status %d", raw, err, raw)
		}
		if got, want := Status(raw).String(), fmt.Sprintf("Status(%d)", raw); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", raw, got, want)
		}
	}
}

func TestRequestAuthorization(t *testing.T) {
	for _, want := range []Status{StatusAuthorized, StatusDenied} {
		t.Run(want.String(), func(t *testing.T) {
			b := &fake.Bridge{RequestStatus: int(want)}
			got, err := requestAuthorization(t.Context(), b)
			if err != nil || got != want {
				t.Fatalf("requestAuthorization = %v, %v; want %v", got, err, want)
			}
			// Upstream asks twice (photokit.py:156-160); PHOTOKIT_REQUEST_TIMEOUT
			// bounds a request without a deadline.
			if calls := b.Calls(); !slices.Equal(calls, []string{"RequestAuthorization 5m0s"}) {
				t.Errorf("calls = %q, want one request for 5m0s", calls)
			}
		})
	}
	t.Run("bridge timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
		defer cancel()
		b := &fake.Bridge{RequestDelay: 2 * time.Hour}
		got, err := requestAuthorization(ctx, b)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("requestAuthorization = %v, %v; want context.DeadlineExceeded", got, err)
		}
		calls := b.Calls()
		if len(calls) != 1 {
			t.Fatalf("calls = %q, want one", calls)
		}
		d, err := time.ParseDuration(strings.TrimPrefix(calls[0], "RequestAuthorization "))
		if err != nil || d <= 59*time.Minute || d > time.Hour {
			t.Errorf("calls = %q, want the time left to the 1h deadline", calls)
		}
	})
	t.Run("canceled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		b := &fake.Bridge{RequestStatus: int(StatusAuthorized)}
		if _, err := requestAuthorization(ctx, b); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		// A bridge call started by mistake logs from its goroutine after the return.
		time.Sleep(20 * time.Millisecond)
		if calls := b.Calls(); len(calls) != 0 {
			t.Errorf("calls = %q, want none", calls)
		}
	})
	t.Run("bridge error", func(t *testing.T) {
		want := errors.New("bridge failed")
		if _, err := requestAuthorization(t.Context(), &fake.Bridge{RequestErr: want}); !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	})
	t.Run("deadline before the answer", func(t *testing.T) {
		b := &fake.Bridge{Hold: make(chan struct{}), RequestStatus: int(StatusAuthorized)}
		t.Cleanup(func() { close(b.Hold) })
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		if _, err := requestAuthorization(ctx, b); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
	})
}

// Not parallel: it counts the goroutines of the process.
func TestRequestAuthorizationCancelInFlight(t *testing.T) {
	b := &fake.Bridge{Hold: make(chan struct{}), RequestStatus: int(StatusAuthorized)}
	before := runtime.NumGoroutine()
	// A request that ignores ctx.Done() waits for the deadline and ends with
	// DeadlineExceeded, not Canceled.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	time.AfterFunc(10*time.Millisecond, cancel)
	if _, err := requestAuthorization(ctx, b); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	close(b.Hold)
	for deadline := time.Now().Add(time.Second); runtime.NumGoroutine() > before; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines, %d before the request: the late answer was not dropped", runtime.NumGoroutine(), before)
		}
	}
}
