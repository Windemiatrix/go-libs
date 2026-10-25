package photosdb

import (
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// Query returns the photos that osxphotos PhotosDB.query(options) returns
// (photosdb/photosdb.py:3396-3402): query.Apply over every photo and album of
// the library. The slice is new on every call, but its values are shared by
// all calls: do not modify them.
func (db *DB) Query(o query.Options) []*photoinfo.PhotoInfo {
	return query.Apply(o, db.photos, db.albums)
}

// Duplicates is query.Duplicates over every photo of the library, those in
// the trash and the unselected burst frames included, in ZUUID order, as
// osxphotos PhotoInfo.duplicates (photoinfo.py:1426-1439); it reads the whole
// catalog on every call. The values are shared: do not modify them.
func (db *DB) Duplicates(p *photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
	return query.Duplicates(p, db.photos)
}
