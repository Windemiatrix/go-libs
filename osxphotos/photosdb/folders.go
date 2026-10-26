package photosdb

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
)

// FolderInfo returns the top-level folders outside the trash, osxphotos
// folder_info (photosdb/photosdb.py:570-589), in UUID order. The slice is new
// on every call, but its values are shared by all calls: do not modify them.
func (db *DB) FolderInfo() []*albuminfo.FolderInfo { return slices.Clone(db.topFolders) }

// FolderTitles returns the titles of FolderInfo in its order, repeats kept:
// osxphotos folders (photosdb/photosdb.py:591-610).
func (db *DB) FolderTitles() []string {
	titles := make([]string, len(db.topFolders))
	for i, f := range db.topFolders {
		titles[i] = f.Title
	}
	return titles
}

// Folder returns the folder with this UUID, one in the trash included, as
// osxphotos FolderInfo(db, uuid) does (albuminfo.py:401-409). The value is
// shared: do not modify it.
func (db *DB) Folder(uuid string) (*albuminfo.FolderInfo, bool) {
	return findFolder(db.folders, uuid)
}

// FolderList returns the folders that enclose a, outermost first: osxphotos
// AlbumInfo.folder_list (albuminfo.py:203-213, photosdb/photosdb.py:3091-3123);
// nil for an album at the top level and for a project, as osxphotos
// ProjectInfo.folder_list is always empty (albuminfo.py:385-392).
func (db *DB) FolderList(a *albuminfo.AlbumInfo) []*albuminfo.FolderInfo {
	if _, ok := db.Project(a.UUID); ok {
		return nil
	}
	var list []*albuminfo.FolderInfo
	// Open has checked every chain of parents, so the walk ends.
	for f, ok := db.Folder(a.ParentUUID); ok; f, ok = db.Folder(f.ParentUUID) {
		list = append(list, f)
	}
	slices.Reverse(list)
	return list
}

func findFolder(folders []*albuminfo.FolderInfo, uuid string) (*albuminfo.FolderInfo, bool) {
	i, ok := slices.BinarySearchFunc(folders, uuid, func(f *albuminfo.FolderInfo, u string) int {
		return strings.Compare(f.UUID, u)
	})
	if !ok {
		return nil, false
	}
	return folders[i], true
}

// The chain of parents of every folder, not only of one above an album, must
// end below the root folder or at a row without a parent: a cycle of folders
// without albums would otherwise reach the caller as a graph of Subfolders
// that never ends. Upstream fails on a missing parent of any row
// (photosdb.py:1977-1980) and on a cycle only above an album.
func loadFolders(rows []sqlread.Album, albums []*albuminfo.AlbumInfo) (all, top []*albuminfo.FolderInfo, err error) {
	byPK := make(map[int64]sqlread.Album, len(rows))
	var rootPK int64
	var folderRows []sqlread.Album
	for _, r := range rows {
		byPK[r.PK] = r
		switch r.Kind {
		case sqlread.KindRootFolder:
			rootPK = r.PK
		case sqlread.KindFolder:
			folderRows = append(folderRows, r)
		}
	}
	slices.SortFunc(folderRows, func(a, b sqlread.Album) int { return strings.Compare(a.UUID, b.UUID) })

	all = make([]*albuminfo.FolderInfo, 0, len(folderRows))
	badParents := 0
	for _, r := range folderRows {
		if _, ok := folderNames(r, byPK, rootPK); !ok {
			badParents++
			continue
		}
		f := &albuminfo.FolderInfo{
			UUID:             r.UUID,
			CloudGUID:        r.CloudGUID.String,
			Title:            nfc(r.Title.String),
			LibraryListOrder: libraryListOrder(r, byPK, rootPK),
		}
		if r.Parent.Valid && r.Parent.Int64 != rootPK { // albuminfo.py:459-465
			f.ParentUUID = byPK[r.Parent.Int64].UUID
		}
		all = append(all, f)
	}
	if badParents > 0 {
		return nil, nil, fmt.Errorf("photosdb: %d folders whose parent folders do not lead to the root folder", badParents)
	}

	// Subfolders is not sorted: the appends follow folderRows, which is in UUID order.
	for i, r := range folderRows {
		if trashed(r) {
			continue
		}
		if r.Parent.Valid && r.Parent.Int64 == rootPK { // photosdb.py:582-588
			top = append(top, all[i])
		}
		if parent, ok := findFolder(all, all[i].ParentUUID); ok { // albuminfo.py:483-489
			parent.Subfolders = append(parent.Subfolders, all[i])
		}
	}
	for _, a := range albums {
		if a.InTrash {
			continue
		}
		if parent, ok := findFolder(all, a.ParentUUID); ok { // albuminfo.py:436-442
			parent.Albums = append(parent.Albums, a)
		}
	}
	return all, top, nil
}
