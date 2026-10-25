package query

import (
	"slices"
	"testing"
)

// TestAlbumPath checks the split of photoquery.py:930-941: str.replace of
// "//" left to right without overlaps, then str.split on "/". The wanted
// parts are what Python gives for each path.
func TestAlbumPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		fold caseFold
		want []string
	}{
		{"one title", "A", false, []string{"A"}},
		{"folder and album", "F/A", false, []string{"F", "A"}},
		{"escaped slash", "A//B", false, []string{"A/B"}},
		{"three slashes", "A///B", false, []string{"A/", "B"}},
		{"four slashes", "A////B", false, []string{"A//B"}},
		{"leading slash", "/A", false, []string{"", "A"}},
		{"trailing slash", "A/", false, []string{"A", ""}},
		{"empty", "", false, []string{""}},
		{"NUL inside a part", "F\x00G/A", false, []string{"F\x00G", "A"}},
		{"case kept", "Folder/Album", false, []string{"Folder", "Album"}},
		{"ignore case", "Folder/ALBUM//X", true, []string{"folder", "album/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseAlbumPath(tt.in, tt.fold); !slices.Equal(got, tt.want) {
				t.Errorf("parseAlbumPath(%q, %v) = %q, want %q", tt.in, tt.fold, got, tt.want)
			}
		})
	}
}
