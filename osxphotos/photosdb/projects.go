package photosdb

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// ProjectInfo returns the projects (cards, calendars, slideshows: ZKIND 1508)
// outside the trash, osxphotos project_info (photosdb/photosdb.py:681-691,
// 3187-3210), in UUID order. A project is an AlbumInfo without folders, as
// osxphotos ProjectInfo (albuminfo.py:368-392); AlbumPhotos gives its photos.
// The slice is new on every call, but its values are shared by all calls: do
// not modify them.
func (db *DB) ProjectInfo() []*albuminfo.AlbumInfo { return outsideTrash(db.projects) }

// Project returns the project with this UUID, one in the trash included, as
// osxphotos ProjectInfo(db, uuid) does (albuminfo.py:368). The value is
// shared: do not modify it.
func (db *DB) Project(uuid string) (*albuminfo.AlbumInfo, bool) { return findAlbum(db.projects, uuid) }

// ProjectInfoOf returns the projects of p outside the trash, sorted by title,
// then by UUID: osxphotos PhotoInfo.project_info (photoinfo.py:696-703,
// 2022-2034). A project without a title sorts first; upstream fails when it
// compares its None title with another.
func (db *DB) ProjectInfoOf(p *photoinfo.PhotoInfo) []*albuminfo.AlbumInfo {
	var out []*albuminfo.AlbumInfo
	for _, u := range p.ProjectUUIDs {
		if a, ok := db.Project(u); ok && !a.InTrash {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int {
		return cmp.Or(strings.Compare(a.Title, b.Title), strings.Compare(a.UUID, b.UUID))
	})
	return out
}

// Projects and shared albums inherit parent, sort_order and
// library_list_order of AlbumInfo, with no folders above them
// (albuminfo.py:229-234, 237-278, 374-392; photosdb.py:1992-1994). The parent
// is any row of ZGENERICALBUM, as upstream reads all of them; a parent that is
// not a row is an error, as there (photosdb.py:1977-1980), and so is one
// without ZUUID, as for albums. rows have passed loadAlbums (one root folder),
// but the CollectionShare rows of ZSHARE among them have not, so an empty or
// repeated ZUUID, which would merge the photos of two albums, is checked here.
func loadFolderless(rows []sqlread.Album, albumPhotos map[string][]string, kind int64, plural string) ([]*albuminfo.AlbumInfo, error) {
	var rootPK int64
	seen, badUUID := map[string]bool{"": true}, 0
	for _, r := range rows {
		if r.Kind == sqlread.KindRootFolder {
			rootPK = r.PK
		}
		if seen[r.UUID] {
			badUUID++
		}
		seen[r.UUID] = true
	}
	if badUUID > 0 {
		return nil, fmt.Errorf("photosdb: %d %s with an empty ZUUID or the ZUUID of another album", badUUID, plural)
	}
	var out []*albuminfo.AlbumInfo
	badParents := 0
	for _, r := range rows {
		if r.Kind != kind {
			continue
		}
		a := albumOf(r, albumPhotos)
		a.LibraryListOrder = []int{noListOrder}
		if r.ListOrder.Valid {
			a.LibraryListOrder = []int{intOf(r.ListOrder)}
		}
		if r.Parent.Valid && r.Parent.Int64 != rootPK {
			if !r.ParentUUID.Valid {
				badParents++
			}
			a.ParentUUID = r.ParentUUID.String
		}
		out = append(out, a)
	}
	if badParents > 0 {
		return nil, fmt.Errorf("photosdb: %d %s whose ZPARENTFOLDER is not a row of ZGENERICALBUM with a ZUUID", badParents, plural)
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int { return strings.Compare(a.UUID, b.UUID) })
	return out, nil
}
