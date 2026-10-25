package golden

import (
	"slices"
	"testing"
)

// TestKeysCoverFields: a key added to fields.txt without a rule is red.
func TestKeysCoverFields(t *testing.T) {
	t.Parallel()
	fields := fieldsTxt(t)
	photo, album := PhotoKeys(), AlbumKeys()
	if !slices.IsSorted(photo) || !slices.IsSorted(album) {
		t.Errorf("PhotoKeys %v and AlbumKeys %v must be sorted", photo, album)
	}
	if !slices.Contains(photo, "uuid") {
		t.Error("uuid is not in PhotoKeys")
	}
	for _, k := range photo {
		if slices.Contains(album, k) {
			t.Errorf("key %s is in PhotoKeys and in AlbumKeys", k)
		}
	}
	all := slices.Concat(photo, album)
	slices.Sort(all)
	want := slices.Clone(fields)
	slices.Sort(want)
	if !slices.Equal(all, want) {
		t.Errorf("PhotoKeys and AlbumKeys give %d keys, fields.txt has %d\n got %v\nwant %v", len(all), len(want), all, want)
	}
	if want := []string{"album_info", "albums", "burst_album_info", "burst_albums", "folders"}; !slices.Equal(album, want) {
		t.Errorf("AlbumKeys = %v, want %v", album, want)
	}
}
