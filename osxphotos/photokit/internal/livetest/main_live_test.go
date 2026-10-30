//go:build photokit_live

package livetest

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// markerPhotos are the photos of the marker album, read before the first call
// to PhotoKit. The tests get no other photo of the library, so that they
// cannot touch one outside the album.
var markerPhotos []*photoinfo.PhotoInfo

// markerBursts holds the burst of each burst photo of markerPhotos by its
// UUID. Only counts are kept: the frames may lie outside the album.
var markerBursts map[string]burstFrames

// frames are those outside the trash, where PhotoKit finds none. unpicked is
// a frame that FetchBurst leaves out without all: one with a pick type (0 is
// a single image with a burst UUID) that is neither selected, the key photo
// nor the default pick (photosdb.py:3347-3351, _constants.py:676-681).
type burstFrames struct {
	frames   int
	unpicked bool
}

func framesOf(burst []*photoinfo.PhotoInfo) burstFrames {
	var b burstFrames
	for _, f := range burst {
		if !f.InTrash {
			b.frames++
			b.unpicked = b.unpicked || (f.BurstPickType != 0 && !f.BurstSelected && !f.BurstKey && !f.BurstDefaultPick)
		}
	}
	return b
}

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
	if reason := readMarkerPhotos(); reason != "" {
		fmt.Println(reason)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// PhotoKit works on the system library only: a library chosen by a variable
// could pass the guard while PhotoKit acts on another.
func systemLibraryPath() (string, error) {
	path, err := photosdb.SystemLibraryPath()
	if err == nil {
		return path, nil
	}
	// SystemLibraryPath may find no key on macOS 26.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Pictures", "Photos Library.photoslibrary"), nil
}

func readMarkerPhotos() (reason string) {
	path, err := systemLibraryPath()
	if err != nil {
		return "livetest: no home directory"
	}
	// Open asks for a ctx canceled on SIGINT and SIGTERM, so that an
	// interrupted Open still removes its copy of the database.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	lib, err := photosdb.Open(ctx, path, photosdb.Options{})
	stop()
	if err != nil {
		return openFailure(err)
	}
	album := markerOf(lib.AlbumInfo())
	if reason = libraryReason(len(lib.Photos()), album != nil); reason != "" {
		return reason
	}
	markerPhotos = lib.AlbumPhotos(album)
	markerBursts = map[string]burstFrames{}
	for _, p := range markerPhotos {
		if p.Burst {
			markerBursts[p.UUID] = framesOf(append([]*photoinfo.PhotoInfo{p}, lib.BurstPhotos(p)...))
		}
	}
	return ""
}
