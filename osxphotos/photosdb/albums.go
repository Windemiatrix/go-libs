package photosdb

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Albums returns every user album of the library, those in the trash
// included, ordered by UUID: the albums that PhotoInfo.AlbumUUIDs refers to.
// osxphotos album_info, without the albums in the trash, is AlbumInfo. Shared
// albums are not read. The slice is new on every call, but its values are
// shared by all calls: do not modify them.
// extension: not in osxphotos — osxphotos lists no album in the trash.
func (db *DB) Albums() []*albuminfo.AlbumInfo { return slices.Clone(db.albums) }

// One root folder is required (photosdb.py:1958-1967). An empty or repeated
// ZUUID is an error, as for assets in load: it would merge the members of two
// albums. So is a chain of parents that does not end, a parent missing from
// the rows (photosdb.py:1977-1980) or a cycle, where the recursion of
// upstream fails (photosdb.py:2944-2965).
func loadAlbums(rows []sqlread.Album, members []sqlread.Member, assets []sqlread.Asset,
	byUUID map[string]*photoinfo.PhotoInfo) ([]*albuminfo.AlbumInfo, error) {
	byPK := make(map[int64]sqlread.Album, len(rows))
	seen := make(map[string]bool, len(rows))
	var roots, badUUID int
	var rootPK int64
	for _, r := range rows {
		byPK[r.PK] = r
		if r.Kind == sqlread.KindRootFolder {
			roots++
			rootPK = r.PK
		}
		if r.UUID == "" || seen[r.UUID] {
			badUUID++
		}
		seen[r.UUID] = true
	}
	if roots != 1 {
		return nil, fmt.Errorf("photosdb: %d root folders in ZGENERICALBUM, want 1", roots)
	}
	if badUUID > 0 {
		return nil, fmt.Errorf("photosdb: %d albums and folders with an empty or repeated ZGENERICALBUM.ZUUID", badUUID)
	}

	var out []*albuminfo.AlbumInfo
	byAlbum := map[string]*albuminfo.AlbumInfo{}
	badParents := 0
	for _, r := range rows {
		if r.Kind != sqlread.KindUserAlbum { // photosdb.py:612-622, 3187-3209
			continue
		}
		folders, ok := folderNames(r, byPK, rootPK)
		if !ok {
			badParents++
			continue
		}
		a := &albuminfo.AlbumInfo{
			UUID:         r.UUID,
			CloudGUID:    r.CloudGUID.String,
			Title:        nfc(r.Title.String),
			FolderNames:  folders,
			CreationDate: optionalTime(r.Created),
			StartDate:    optionalTime(r.Start),
			EndDate:      optionalTime(r.End),
			InTrash:      trashed(r),
		}
		if r.Parent.Valid && r.Parent.Int64 != rootPK { // albuminfo.py:228-234
			a.ParentUUID = byPK[r.Parent.Int64].UUID
		}
		out = append(out, a)
		byAlbum[a.UUID] = a
	}
	if badParents > 0 {
		return nil, fmt.Errorf("photosdb: %d albums whose parent folders do not lead to the root folder", badParents)
	}

	photoOf := make(map[int64]string, len(assets))
	for _, a := range assets {
		if a.HasAttributes && byUUID[a.UUID.String] != nil {
			photoOf[a.PK] = a.UUID.String
		}
	}
	for _, m := range members {
		a, isAlbum := byAlbum[m.AlbumUUID]
		p, isPhoto := photoOf[m.AssetPK]
		if isAlbum && isPhoto {
			a.PhotoUUIDs = append(a.PhotoUUIDs, p)
		}
	}
	for _, a := range out {
		a.PhotoUUIDs = sortedSet(a.PhotoUUIDs)
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int { return strings.Compare(a.UUID, b.UUID) })
	return out, nil
}

// Upstream: photosdb.py:1987-1991, 2944-2965, 3013-3050. A chain with more
// steps than there are rows is a cycle.
func folderNames(album sqlread.Album, byPK map[int64]sqlread.Album, rootPK int64) (names []string, ok bool) {
	parent := album.Parent
	for steps := 0; parent.Valid && parent.Int64 != rootPK; steps++ {
		f, found := byPK[parent.Int64]
		if !found || steps == len(byPK) {
			return nil, false
		}
		names = append(names, nfc(f.Title.String))
		parent = f.Parent
	}
	slices.Reverse(names)
	return names, true
}

// trashed is one rule for albums and folders: Python None != 0 holds, so a
// NULL state counts as trashed (photosdb.py:1936).
func trashed(r sqlread.Album) bool { return !r.Trashed.Valid || r.Trashed.Int64 != 0 }
