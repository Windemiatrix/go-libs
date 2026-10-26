package photosdb

import (
	"cmp"
	"database/sql"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// AlbumPhotos returns the photos of a in the order of the album, those in the
// trash included: osxphotos AlbumInfo.photos (albuminfo.py:165-187). A photo
// comes as many times as it has membership rows; nil when a has no photos.
// The slice is new on every call, but its values are shared: do not modify
// them.
func (db *DB) AlbumPhotos(a *albuminfo.AlbumInfo) []*photoinfo.PhotoInfo {
	var photos []*photoinfo.PhotoInfo
	for _, u := range db.albumPhotos[a.UUID] {
		photos = append(photos, db.byUUID[u])
	}
	// Python sorted is stable, with reverse=True as well (albuminfo.py:176-184).
	switch a.SortOrder {
	case albuminfo.AlbumSortNewestFirst:
		slices.SortStableFunc(photos, func(p, q *photoinfo.PhotoInfo) int { return q.Date.Compare(p.Date) })
	case albuminfo.AlbumSortOldestFirst:
		slices.SortStableFunc(photos, func(p, q *photoinfo.PhotoInfo) int { return p.Date.Compare(q.Date) })
	case albuminfo.AlbumSortTitle:
		slices.SortStableFunc(photos, func(p, q *photoinfo.PhotoInfo) int { return strings.Compare(p.Title, q.Title) })
	}
	return photos
}

// PhotoIndex returns the position of the first photo of AlbumPhotos(a) with
// the UUID of p: osxphotos AlbumInfo.photo_index (albuminfo.py:280-287). It
// returns -1, false when p is not in a, where osxphotos raises ValueError.
func (db *DB) PhotoIndex(a *albuminfo.AlbumInfo, p *photoinfo.PhotoInfo) (int, bool) {
	i := slices.IndexFunc(db.AlbumPhotos(a), func(q *photoinfo.PhotoInfo) bool { return q.UUID == p.UUID })
	return i, i >= 0
}

// Upstream sorts the pairs of position and photo UUID of an album
// (albuminfo.py:53, 171-174). A NULL position comes before the numbers, where
// upstream fails comparing None with an int. The photos are those of
// PhotoUUIDs, as loadAlbums takes them.
func membershipOrder(members []sqlread.Member, assets []sqlread.Asset, byUUID map[string]*photoinfo.PhotoInfo) map[string][]string {
	photoOf := make(map[int64]string, len(assets))
	for _, a := range assets {
		if a.HasAttributes && byUUID[a.UUID.String] != nil {
			photoOf[a.PK] = a.UUID.String
		}
	}
	rows := slices.DeleteFunc(slices.Clone(members), func(m sqlread.Member) bool { return photoOf[m.AssetPK] == "" })
	slices.SortFunc(rows, func(m, n sqlread.Member) int {
		return cmp.Or(compareNullsFirst(m.Order, n.Order), strings.Compare(photoOf[m.AssetPK], photoOf[n.AssetPK]))
	})
	out := map[string][]string{}
	for _, m := range rows {
		out[m.AlbumUUID] = append(out[m.AlbumUUID], photoOf[m.AssetPK])
	}
	return out
}

func compareNullsFirst(a, b sql.NullInt64) int {
	switch {
	case a.Valid == b.Valid:
		return cmp.Compare(a.Int64, b.Int64)
	case a.Valid:
		return 1
	}
	return -1
}
