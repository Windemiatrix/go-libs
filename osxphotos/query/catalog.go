package query

import (
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Catalog is what Apply selects from: the photos and albums of a library and
// the data that PhotoInfo does not hold, which PhotosDB reads from other rows
// (photoquery.py:656-664). A nil map means that no photo has the data.
type Catalog struct {
	Photos   []*photoinfo.PhotoInfo
	Albums   []*albuminfo.AlbumInfo
	Comments map[string]bool // uuid of every photo with a comment (DB.Comments)
	Likes    map[string]bool // uuid of every photo with a like (DB.Likes)
}

// ApplyCatalog is Apply over c.Photos and c.Albums, with c.Comments and
// c.Likes for HasComment, NoComment, HasLikes and NoLikes. c is not modified.
func ApplyCatalog(o Options, c Catalog) []*photoinfo.PhotoInfo {
	before, after := filters(o, c)
	var out []*photo
	for _, p := range c.Photos {
		if inSelection(o, p) && passes(before, p) {
			out = append(out, p)
		}
	}
	if o.Duplicate {
		out = withDuplicates(out, c.Photos, o)
	}
	out = slices.DeleteFunc(out, func(p *photo) bool { return !passes(after, p) })
	for _, f := range o.Function {
		out = f(out)
	}
	if o.BurstPhotos { // after the functions, as photoquery.py:863 (ref #640)
		out = withBursts(out, c.Photos, o.MissingBursts)
	}
	slices.SortStableFunc(out, comparePhotos)
	if o.NewestFirst { // the keys of Sort are unique, so this is sorted(reverse=True)
		slices.Reverse(out)
	}
	return out
}
