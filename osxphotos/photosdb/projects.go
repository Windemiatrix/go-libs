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
func (db *DB) ProjectInfo() []*albuminfo.AlbumInfo {
	var out []*albuminfo.AlbumInfo
	for _, a := range db.projects {
		if !a.InTrash {
			out = append(out, a)
		}
	}
	return out
}

// Project returns the project with this UUID, one in the trash included, as
// osxphotos ProjectInfo(db, uuid) does (albuminfo.py:368). The value is
// shared: do not modify it.
func (db *DB) Project(uuid string) (*albuminfo.AlbumInfo, bool) {
	i, ok := slices.BinarySearchFunc(db.projects, uuid, func(a *albuminfo.AlbumInfo, u string) int {
		return strings.Compare(a.UUID, u)
	})
	if !ok {
		return nil, false
	}
	return db.projects[i], true
}

// ProjectInfoOf returns the projects of p outside the trash, sorted by title,
// then by UUID: osxphotos PhotoInfo.project_info (photoinfo.py:696-703,
// 2022-2034). Upstream fails on a project without a title, which sorts first
// here.
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

// Projects inherit parent, sort_order and library_list_order of AlbumInfo,
// with no folders above them (albuminfo.py:229-234, 237-278, 374-392). The
// parent is any row of ZGENERICALBUM, as upstream reads all of them; a parent
// that is not a row is an error, as there (photosdb.py:1977-1980), and so is
// one without ZUUID, as for albums. rows have passed loadAlbums: one root
// folder, no empty or repeated ZUUID.
func loadProjects(rows []sqlread.Album, albumPhotos map[string][]string) ([]*albuminfo.AlbumInfo, error) {
	var rootPK int64
	for _, r := range rows {
		if r.Kind == sqlread.KindRootFolder {
			rootPK = r.PK
		}
	}
	var out []*albuminfo.AlbumInfo
	badParents := 0
	for _, r := range rows {
		if r.Kind != sqlread.KindProject {
			continue
		}
		order := noListOrder
		if r.ListOrder.Valid {
			order = intOf(r.ListOrder)
		}
		a := &albuminfo.AlbumInfo{
			UUID:             r.UUID,
			CloudGUID:        r.CloudGUID.String,
			Title:            nfc(r.Title.String),
			PhotoUUIDs:       sortedSet(slices.Clone(albumPhotos[r.UUID])),
			CreationDate:     optionalTime(r.Created),
			StartDate:        optionalTime(r.Start),
			EndDate:          optionalTime(r.End),
			InTrash:          trashed(r),
			SortOrder:        sortOrder(r),
			LibraryListOrder: []int{order},
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
		return nil, fmt.Errorf("photosdb: %d projects whose ZPARENTFOLDER is not a row of ZGENERICALBUM with a ZUUID", badParents)
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int { return strings.Compare(a.UUID, b.UUID) })
	return out, nil
}
