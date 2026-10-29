//go:build photokit_live

package livetest

import (
	"fmt"
	"os"
	"os/user"
	"testing"
)

// The guard is here and not in each test, so that a test cannot forget it:
// no call to PhotoKit happens before it passes.
func TestMain(m *testing.M) {
	username := ""
	u, err := user.Current()
	if err == nil {
		username = u.Username
	}
	if reason := liveReason(os.Getenv, username, err); reason != "" {
		fmt.Println(reason)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
