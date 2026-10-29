package livetest

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
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

func TestLibraryReason(t *testing.T) {
	const album = "__photosync_test__"
	tests := []struct {
		name      string
		photos    int
		hasMarker bool
		want      string
	}{
		{"empty test library", 0, true, ""},
		{"at the limit", 5000, true, ""},
		{"over the limit", 5001, true, "more than 5000"},
		{"no marker album", 10, false, album},
		{"the size before the album", 5001, false, "more than 5000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := libraryReason(tt.photos, tt.hasMarker)
			if (got == "") != (tt.want == "") || !strings.Contains(got, tt.want) {
				t.Errorf("libraryReason(%d, %t) = %q, want a reason with %q", tt.photos, tt.hasMarker, got, tt.want)
			}
			if tt.photos > 5000 && strings.Contains(got, album) {
				t.Errorf("libraryReason(%d, %t) = %q names the album to a library too large", tt.photos, tt.hasMarker, got)
			}
			// A library over the limit is most likely the owner's, and its exact
			// size is personal data.
			if tt.photos > 5000 && strings.Contains(got, strconv.Itoa(tt.photos)) {
				t.Errorf("libraryReason(%d, %t) = %q prints the size of the library", tt.photos, tt.hasMarker, got)
			}
		})
	}
}

func TestMarkerOf(t *testing.T) {
	marker := &albuminfo.AlbumInfo{UUID: "2", Title: "__photosync_test__"}
	other := &albuminfo.AlbumInfo{UUID: "3", Title: "Holidays"}
	tests := []struct {
		name   string
		albums []*albuminfo.AlbumInfo
		want   *albuminfo.AlbumInfo
	}{
		{"marker album", []*albuminfo.AlbumInfo{other, marker}, marker},
		{"another title", []*albuminfo.AlbumInfo{other}, nil},
		{"no albums", nil, nil},
	}
	for _, tt := range tests {
		if got := markerOf(tt.albums); got != tt.want {
			t.Errorf("%s: markerOf = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestOpenFailure(t *testing.T) {
	const path = "/x/y.photoslibrary"
	for _, tt := range []struct {
		err  error
		want string
	}{
		{fs.ErrPermission, "no access to the system Photos library"},
		{fs.ErrNotExist, "no library at the system library path"},
		{photosdb.ErrUnsupportedSchema, "schema of the system Photos library is not supported"},
		{errors.New("disk failed"), "cannot read the system Photos library"},
	} {
		err := fmt.Errorf("open %s: %w", path, tt.err)
		got := openFailure(err)
		if !strings.HasPrefix(got, "livetest: ") || !strings.Contains(got, tt.want) {
			t.Errorf("openFailure(%v) = %q, want a reason with %q", err, got, tt.want)
		}
		if strings.Contains(got, path) {
			t.Errorf("openFailure(%v) = %q names the path", err, got)
		}
	}
}
