package photosdb

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Album returns the user album with this UUID, one in the trash included, as
// osxphotos AlbumInfo(db, uuid) does (albuminfo.py:150-158). The value is
// shared: do not modify it.
func (db *DB) Album(uuid string) (*albuminfo.AlbumInfo, bool) {
	// db.albums is in strings.Compare order of UUID, as loadAlbums sorts it.
	i, ok := slices.BinarySearchFunc(db.albums, uuid, func(a *albuminfo.AlbumInfo, u string) int {
		return strings.Compare(a.UUID, u)
	})
	if !ok {
		return nil, false
	}
	return db.albums[i], true
}

// AlbumInfo returns the user albums outside the trash, osxphotos album_info
// (photosdb/photosdb.py:612-622, 3187-3210), in UUID order. The slice is new
// on every call, but its values are shared by all calls: do not modify them.
func (db *DB) AlbumInfo() []*albuminfo.AlbumInfo {
	var out []*albuminfo.AlbumInfo
	for _, a := range db.albums {
		if !a.InTrash {
			out = append(out, a)
		}
	}
	return out
}

// AlbumTitles returns the titles of AlbumInfo, sorted, each once: osxphotos
// albums (photosdb/photosdb.py:639-650, 3212-3225).
func (db *DB) AlbumTitles() []string { return sortedTitles(db.AlbumInfo()) }

// AlbumCounts returns the number of photos of the albums of AlbumInfo by
// title, albums of one title added up: osxphotos albums_as_dict
// (photosdb/photosdb.py:511-526).
func (db *DB) AlbumCounts() map[string]int {
	counts := map[string]int{}
	for _, a := range db.AlbumInfo() {
		counts[a.Title] += len(a.PhotoUUIDs)
	}
	return counts
}

// AlbumInfoOf returns the albums of p outside the trash, sorted by title, then
// by UUID: osxphotos PhotoInfo.album_info (photoinfo.py:655-662, 2022-2034).
func (db *DB) AlbumInfoOf(p *photoinfo.PhotoInfo) []*albuminfo.AlbumInfo {
	return db.albumsOf(p.AlbumUUIDs)
}

// AlbumTitlesOf returns the titles of AlbumInfoOf(p), sorted, each once:
// osxphotos PhotoInfo.albums (photoinfo.py:633-644).
func (db *DB) AlbumTitlesOf(p *photoinfo.PhotoInfo) []string {
	return sortedTitles(db.AlbumInfoOf(p))
}

// BurstAlbumInfoOf returns the albums outside the trash of p and of the key
// photos of its burst, each once, in the order of AlbumInfoOf: osxphotos
// burst_album_info (photoinfo.py:664-671).
func (db *DB) BurstAlbumInfoOf(p *photoinfo.PhotoInfo) []*albuminfo.AlbumInfo {
	uuids := slices.Clone(p.AlbumUUIDs)
	for _, f := range db.BurstPhotos(p) {
		if f.BurstKey {
			uuids = append(uuids, f.AlbumUUIDs...)
		}
	}
	return db.albumsOf(sortedSet(uuids))
}

// BurstAlbumTitlesOf returns the titles of BurstAlbumInfoOf(p), sorted, each
// once: osxphotos burst_albums (photoinfo.py:646-653).
func (db *DB) BurstAlbumTitlesOf(p *photoinfo.PhotoInfo) []string {
	return sortedTitles(db.BurstAlbumInfoOf(p))
}

// albumsOf sorts by title as osxphotos does (photoinfo.py:658-662); the UUID
// breaks ties, where upstream keeps the order of the membership rows. Open
// leaves no UUID that is not an album in the lists.
func (db *DB) albumsOf(uuids []string) []*albuminfo.AlbumInfo {
	var out []*albuminfo.AlbumInfo
	for _, u := range uuids { // photoinfo.py:2029-2034
		if a, ok := db.Album(u); ok && !a.InTrash {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b *albuminfo.AlbumInfo) int {
		return cmp.Or(strings.Compare(a.Title, b.Title), strings.Compare(a.UUID, b.UUID))
	})
	return out
}

// Upstream makes a set of the titles (photosdb.py:3224-3225, photoinfo.py:640-643).
func sortedTitles(albums []*albuminfo.AlbumInfo) []string {
	var titles []string
	for _, a := range albums {
		titles = append(titles, a.Title)
	}
	slices.Sort(titles)
	return slices.Compact(titles)
}
