package photosdb

import (
	"fmt"
	"slices"
	"strings"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
)

// One root folder is required (photosdb.py:1958-1967). An empty or repeated
// ZUUID is an error, as for assets in load: it would merge the members of two
// albums. So is a chain of parents that does not end, a parent missing from
// the rows (photosdb.py:1977-1980) or a cycle, where the recursion of
// upstream fails (photosdb.py:2944-2965).
func loadAlbums(rows []sqlread.Album, albumPhotos map[string][]string) ([]*albuminfo.AlbumInfo, error) {
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
		a := albumOf(r, albumPhotos)
		a.FolderNames, a.LibraryListOrder = folders, libraryListOrder(r, byPK, rootPK)
		if r.Parent.Valid && r.Parent.Int64 != rootPK { // albuminfo.py:228-234
			a.ParentUUID = byPK[r.Parent.Int64].UUID
		}
		out = append(out, a)
	}
	if badParents > 0 {
		return nil, fmt.Errorf("photosdb: %d albums whose parent folders do not lead to the root folder", badParents)
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int { return strings.Compare(a.UUID, b.UUID) })
	return out, nil
}

// osxphotos ProjectInfo inherits these from AlbumInfo (albuminfo.py:368-392).
func albumOf(r sqlread.Album, albumPhotos map[string][]string) *albuminfo.AlbumInfo {
	return &albuminfo.AlbumInfo{
		UUID:         r.UUID,
		CloudGUID:    r.CloudGUID.String,
		Title:        nfc(r.Title.String),
		Owner:        ownerName(r),
		PhotoUUIDs:   sortedSet(slices.Clone(albumPhotos[r.UUID])),
		CreationDate: optionalTime(r.Created),
		StartDate:    optionalTime(r.Start),
		EndDate:      optionalTime(r.End),
		InTrash:      trashed(r),
		SortOrder:    sortOrder(r),
	}
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

// Upstream: albuminfo.py:243-256.
func sortOrder(r sqlread.Album) albuminfo.AlbumSortOrder {
	if !r.SortKey.Valid {
		return albuminfo.AlbumSortUnknown
	}
	switch r.SortKey.Int64 {
	case 0:
		return albuminfo.AlbumSortManual
	case 5:
		return albuminfo.AlbumSortTitle
	case 1:
		if r.SortAscending.Valid {
			switch r.SortAscending.Int64 {
			case 0:
				return albuminfo.AlbumSortNewestFirst
			case 1:
				return albuminfo.AlbumSortOldestFirst
			}
		}
	}
	return albuminfo.AlbumSortUnknown
}

// What upstream returns when it fails to read the order (albuminfo.py:278, 513).
const noListOrder = 9_999_999

// Upstream: albuminfo.py:266-278, 501-513. A row without an order drops the
// list of its own parents, as the exception of int(None) does there. The
// chain of parents of r must end: folderNames has checked it.
func libraryListOrder(r sqlread.Album, byPK map[int64]sqlread.Album, rootPK int64) []int {
	var order []int
	for {
		if !r.ListOrder.Valid {
			order = append(order, noListOrder)
			break
		}
		order = append(order, int(r.ListOrder.Int64))
		if !r.Parent.Valid || r.Parent.Int64 == rootPK {
			break
		}
		r = byPK[r.Parent.Int64]
	}
	slices.Reverse(order)
	return order
}

// trashed is one rule for albums and folders: Python None != 0 holds, so a
// NULL state counts as trashed (photosdb.py:1936).
func trashed(r sqlread.Album) bool { return !r.Trashed.Valid || r.Trashed.Int64 != 0 }

// Upstream: shareparticipant.py:90-117, 153-187; _photosdb_process_comments.py:172-187.
func ownerName(r sqlread.Album) string {
	for _, archive := range r.OwnerComponents {
		if name := personName(archive); name != "" {
			return name
		}
	}
	return nfc(r.Owner.String)
}

func personName(archive []byte) string {
	keyed, _ := plistAny(archive).(map[string]any)
	objects, _ := keyed["$objects"].([]any)
	top, _ := keyed["$top"].(map[string]any)
	object := func(ref any) any {
		uid, ok := ref.(plist.UID)
		if !ok || uint64(uid) >= uint64(len(objects)) || objects[uid] == "$null" {
			return nil
		}
		return objects[uid]
	}
	person, _ := object(top["root"]).(map[string]any)
	data, _ := object(person["NSnameComponentsPrivate"]).(map[string]any)
	var parts []string
	for _, key := range []string{"NSgivenName", "NSfamilyName"} {
		if s, _ := object(data[key]).(string); s != "" {
			parts = append(parts, s)
		}
	}
	return nfc(strings.Join(parts, " "))
}
