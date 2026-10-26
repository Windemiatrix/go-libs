package photosdb

import (
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// ImportInfo returns the import sessions outside the trash, osxphotos
// import_info (photosdb/photosdb.py:669-679, 3187-3210), in UUID order. The
// slice is new on every call, but its values are shared by all calls: do not
// modify them.
func (db *DB) ImportInfo() []*albuminfo.ImportInfo { return slices.Clone(db.importInfo) }

// Import returns the import session (ZKIND 1506) with this UUID, one in the
// trash included, as osxphotos ImportInfo(db, uuid) does (albuminfo.py:300-318).
// The value is shared: do not modify it.
func (db *DB) Import(uuid string) (*albuminfo.ImportInfo, bool) {
	i, ok := slices.BinarySearchFunc(db.imports, uuid, func(s *albuminfo.ImportInfo, u string) int {
		return strings.Compare(s.UUID, u)
	})
	if !ok {
		return nil, false
	}
	return db.imports[i], true
}

// photos must be in UUID order. Upstream orders the photos of a session by
// Z_FOK_IMPORTSESSION, which is NULL in every schema read here (IMPORT_FOK,
// _constants.py:134-253), so the UUID decides (albuminfo.py:331-340).
func loadImports(rows []sqlread.Album, photos []*photoinfo.PhotoInfo) (all, outsideTrash []*albuminfo.ImportInfo) {
	photosOf := map[string][]string{}
	for _, p := range photos {
		if p.ImportUUID != "" {
			photosOf[p.ImportUUID] = append(photosOf[p.ImportUUID], p.UUID)
		}
	}
	sessions := slices.DeleteFunc(slices.Clone(rows), func(r sqlread.Album) bool { return r.Kind != sqlread.KindImportSession })
	slices.SortFunc(sessions, func(a, b sqlread.Album) int { return strings.Compare(a.UUID, b.UUID) })
	for _, r := range sessions {
		s := &albuminfo.ImportInfo{
			UUID:         r.UUID,
			Title:        nfc(r.Title.String),
			CreationDate: optionalTime(r.Created),
			StartDate:    optionalTime(r.Start),
			EndDate:      optionalTime(r.End),
			PhotoUUIDs:   photosOf[r.UUID],
		}
		all = append(all, s)
		if !trashed(r) { // photosdb.py:3202
			outsideTrash = append(outsideTrash, s)
		}
	}
	return all, outsideTrash
}
