// Package query selects and orders photos in memory: a subset of osxphotos
// QueryOptions. It does no I/O.
package query

import (
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Options selects photos. The zero value selects every photo.
type Options struct {
	Album             []string // full paths "Folder/Subfolder/Album"
	FromDate, ToDate  *time.Time
	Photos, Movies    *bool // nil: both
	SharedLibrary     *bool
	IncludeTrashed    bool
	TrashedOnly       bool
	BurstSelectedOnly bool // extension: not in osxphotos — the burst filter that osxphotos photos() always applies
}

// OSXPhotosDefaults returns the selection of `osxphotos query`: trashed
// photos and unselected burst frames are left out.
func OSXPhotosDefaults() Options {
	return Options{BurstSelectedOnly: true}
}

// Apply returns a new slice with the photos of in ordered by (Date,
// DateAdded, UUID) ascending, as osxphotos (photoquery.py:888-898); a nil
// DateAdded sorts as 1970-01-01T00:00:00Z. in is not modified; the elements
// are the same pointers. albums is the album list that Options.Album paths
// resolve against. Apply only sorts for now: opts and albums do not change the
// result.
func Apply(opts Options, in []*photoinfo.PhotoInfo, albums []*albuminfo.AlbumInfo) []*photoinfo.PhotoInfo {
	out := slices.Clone(in)
	slices.SortStableFunc(out, comparePhotos)
	return out
}

// epoch is DEFAULT_DATE of osxphotos, the sort key of a nil DateAdded.
var epoch = time.Unix(0, 0).UTC()

// comparePhotos orders by the moments of Date and DateAdded, then by UUID
// byte by byte (as Python compares ASCII strings).
func comparePhotos(a, b *photoinfo.PhotoInfo) int {
	if c := a.Date.Compare(b.Date); c != 0 {
		return c
	}
	if c := dateAdded(a).Compare(dateAdded(b)); c != 0 {
		return c
	}
	return strings.Compare(a.UUID, b.UUID)
}

func dateAdded(p *photoinfo.PhotoInfo) time.Time {
	if p.DateAdded == nil {
		return epoch
	}
	return *p.DateAdded
}
