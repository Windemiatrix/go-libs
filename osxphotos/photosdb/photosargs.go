package photosdb

import (
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// PhotosOptions are the arguments of osxphotos PhotosDB.photos()
// (photosdb/photosdb.py:3227-3262); the zero value is photos() without them.
type PhotosOptions struct {
	Keywords, UUID, Persons, Albums []string  // any of the values; Albums are titles
	NoImages, NoMovies              bool      // images=False, movies=False
	FromDate, ToDate                time.Time // Date at or after, before; the zero time is not set
	InTrash                         bool      // only the photos in the trash; otherwise only those outside it
}

// PhotosWith returns the photos that osxphotos photos(…) returns with o
// (photosdb/photosdb.py:3264-3362), in the order of query.Sort. Keywords and
// Persons are put in NFC, Albums are not, as upstream; Albums are looked for
// among user albums, those in the trash included. The slice is new on every
// call, but its values are shared: do not modify them.
func (db *DB) PhotosWith(o PhotosOptions) []*photoinfo.PhotoInfo {
	photos := db.Query(query.Options{
		UUID: o.UUID, Keyword: o.Keywords, Person: o.Persons,
		OnlyPhotos: o.NoMovies, OnlyMovies: o.NoImages,
		FromDate: o.FromDate, ToDate: o.ToDate,
		DeletedOnly: o.InTrash,
	})
	if len(o.Albums) == 0 {
		return photos
	}
	titled := map[string]bool{}
	for _, a := range db.albums {
		if slices.Contains(o.Albums, a.Title) {
			titled[a.UUID] = true
		}
	}
	return slices.DeleteFunc(photos, func(p *photoinfo.PhotoInfo) bool {
		return !slices.ContainsFunc(p.AlbumUUIDs, func(id string) bool { return titled[id] })
	})
}
