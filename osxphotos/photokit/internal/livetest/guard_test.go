package livetest

import (
	"errors"
	"strings"
	"testing"
)

func TestLiveReason(t *testing.T) {
	env := func(live string) func(string) string {
		return func(key string) string {
			if key == "PHOTOSYNC_LIVE" {
				return live
			}
			return ""
		}
	}
	const userReason = "only as phototest"
	tests := []struct {
		name, live, user, want string
		err                    error
	}{
		{"variable not set", "", "phototest", "PHOTOSYNC_LIVE", nil},
		{"variable 0", "0", "phototest", "PHOTOSYNC_LIVE", nil},
		{"may run", "1", "phototest", "", nil},
		{"other user", "1", "someone", userReason, nil},
		{"empty user", "1", "", userReason, nil},
		{"lookup error", "1", "phototest", "lookup failed", errors.New("lookup failed")},
		{"user before variable", "", "someone", userReason, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := liveReason(env(tt.live), tt.user, tt.err)
			if (got == "") != (tt.want == "") || !strings.Contains(got, tt.want) {
				t.Errorf("liveReason = %q, want a reason with %q", got, tt.want)
			}
			if tt.user == "someone" && strings.Contains(got, tt.user) {
				t.Errorf("liveReason = %q names the user", got)
			}
		})
	}
}
