// Package query selects and orders photos in memory as osxphotos query does.
// It does no I/O.
package query

import (
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Sort returns a new slice with the photos of in ordered by (Date,
// DateAdded, UUID) ascending, as osxphotos (photoquery.py:888-898); a nil
// DateAdded sorts as 1970-01-01T00:00:00Z. in is not modified; the elements
// are the same pointers.
func Sort(in []*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
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
