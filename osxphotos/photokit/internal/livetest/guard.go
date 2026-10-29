package livetest

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

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

// A test library holds hundreds of photos and the marker album; a library in
// daily use holds far more and has no such album.
const (
	markerAlbum   = "__photosync_test__"
	maxLivePhotos = 5000
)

// libraryReason returns why the live tests must not touch the library, or ""
// when they may. The size is checked first: told only that the album is
// missing, the owner could add it to their own library.
func libraryReason(photos int, hasMarker bool) string {
	switch {
	case photos > maxLivePhotos:
		return fmt.Sprintf("livetest: the library has more than %d photos", maxLivePhotos)
	case !hasMarker:
		return "livetest: the library has no album " + markerAlbum
	}
	return ""
}

func markerOf(albums []*albuminfo.AlbumInfo) *albuminfo.AlbumInfo {
	for _, a := range albums {
		if a.Title == markerAlbum {
			return a
		}
	}
	return nil
}

// The error of photosdb names the library path, and the output is read by
// others: only the class of the error is printed. On the first run the read
// itself may raise the Photos dialog, so a refusal is told apart.
func openFailure(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "livetest: no access to the system Photos library: allow Photos for Terminal"
	case errors.Is(err, fs.ErrNotExist):
		return "livetest: no library at the system library path"
	case errors.Is(err, photosdb.ErrUnsupportedSchema):
		return "livetest: the schema of the system Photos library is not supported"
	}
	return "livetest: cannot read the system Photos library"
}
