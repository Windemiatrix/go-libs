package livetest

// The TCC grant to Photos belongs to the Terminal of this account, so the
// live tests run only there, and only when asked for.
const (
	liveUser = "phototest"
	liveEnv  = "PHOTOSYNC_LIVE"
)

// liveReason returns why the live tests must not run, or "" when they may.
// The user is checked first, so that another account is told only that it may
// not run them, not which variable enables them. The reason does not name the
// user, whose login would end up in the log.
func liveReason(getenv func(string) string, username string, lookupErr error) string {
	switch {
	case lookupErr != nil:
		return "livetest: cannot look up the current user: " + lookupErr.Error()
	case username != liveUser:
		return "livetest: the live tests run only as " + liveUser
	case getenv(liveEnv) != "1":
		return "livetest: " + liveEnv + "=1 is not set"
	}
	return ""
}
