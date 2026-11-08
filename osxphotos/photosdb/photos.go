package photosdb

import (
	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// DB is a Photos library read into memory.
type DB struct {
	info   Info
	photos []*photoinfo.PhotoInfo // ZUUID order
	byUUID map[string]*photoinfo.PhotoInfo
	albums []*albuminfo.AlbumInfo            // user albums, trashed ones included, ZUUID order
	bursts map[string][]*photoinfo.PhotoInfo // frames of the bursts by BurstUUID, ZUUID order

	sharedAlbums []*albuminfo.AlbumInfo // shared albums (ZKIND 1505), trashed ones included, ZUUID order
	projects     []*albuminfo.AlbumInfo // projects, those in the trash included, ZUUID order

	albumPhotos map[string][]string // photo UUIDs of each album, shared album and project by its UUID, in membership order, repeats kept

	unusedKeywords []string // NFC titles of the ZKEYWORD rows no photo has, sorted, repeats kept

	folders    []*albuminfo.FolderInfo // every folder, those in the trash included, ZUUID order
	topFolders []*albuminfo.FolderInfo // osxphotos folder_info, ZUUID order

	imports    []*albuminfo.ImportInfo // every import session, those in the trash included, ZUUID order
	importInfo []*albuminfo.ImportInfo // osxphotos import_info, ZUUID order

	momentShares  map[string]*photoinfo.ShareInfo // by photo UUID
	libraryShares map[string]*photoinfo.ShareInfo // by photo UUID

	participants map[string][]photoinfo.ShareParticipant // by photo UUID, in ZSHAREPARTICIPANT.Z_PK order
	comments     map[string][]photoinfo.CommentInfo      // by photo UUID, oldest first
	likes        map[string][]photoinfo.LikeInfo         // by photo UUID, oldest first
	commented    map[string]bool                         // the keys of comments, for query.Catalog
	liked        map[string]bool                         // the keys of likes, for query.Catalog

	cloudMeta map[string][]byte // the ZDATA of ZCLOUDMASTERMEDIAMETADATA by photo UUID, decoded by CloudMetadataOf

	search map[string][]searchinfo.Record // the terms of psi.sqlite or leo.sqlite by asset UUID, also of an asset Photos.sqlite lacks

	renderer template.Renderer // of RenderTemplate: one Renderer is one process of osxphotos, whose {counter} grows from photo to photo

	personData
	momentData
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
