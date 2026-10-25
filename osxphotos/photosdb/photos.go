package photosdb

import (
	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// DB is a Photos library read into memory.
type DB struct {
	info   Info
	photos []*photoinfo.PhotoInfo // ZUUID order
	byUUID map[string]*photoinfo.PhotoInfo
	albums []*albuminfo.AlbumInfo            // user albums, trashed ones included, ZUUID order
	bursts map[string][]*photoinfo.PhotoInfo // frames of the bursts by BurstUUID, ZUUID order

	folders    []*albuminfo.FolderInfo // every folder, those in the trash included, ZUUID order
	topFolders []*albuminfo.FolderInfo // osxphotos folder_info, ZUUID order
}

// Close releases the DB. It releases nothing today: the copy of the
// database is removed before Open returns. Close may be called any number
// of times and returns nil.
func (db *DB) Close() error { return nil }

// Info describes the library.
func (db *DB) Info() Info { return db.info }

// Photos returns every photo, those in the trash and the unselected burst
// frames included, in the order of osxphotos (query.Sort). Query(query.Options{})
// makes the selection of osxphotos photos() (photosdb/photosdb.py:3269-3276,
// 3346-3358). The slice is new on every call, but its values are shared by all
// calls: do not modify them.
func (db *DB) Photos() []*photoinfo.PhotoInfo {
	return query.Sort(db.photos)
}

// Photo returns the photo with this UUID, without any filter
// (photosdb.py:3364-3376): trashed photos and unselected burst frames are
// found as well. The value is shared: do not modify it.
func (db *DB) Photo(uuid string) (*photoinfo.PhotoInfo, bool) {
	p, ok := db.byUUID[uuid]
	return p, ok
}
