package photosdb

import (
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Comments returns the comments on p, a photo of a shared album: osxphotos
// comments (photoinfo.py:1671-1677, _photosdb_process_comments.py:95-152),
// oldest first, those of one date in ZCLOUDSHAREDCOMMENT.Z_PK order; nil
// without one. The slice is shared: do not modify it.
func (db *DB) Comments(p *photoinfo.PhotoInfo) []photoinfo.CommentInfo { return db.comments[p.UUID] }

// Likes returns the likes of p as Comments returns its comments: osxphotos
// likes (photoinfo.py:1679-1685).
func (db *DB) Likes(p *photoinfo.PhotoInfo) []photoinfo.LikeInfo { return db.likes[p.UUID] }

// Upstream: _photosdb_process_comments.py:120-150, an unread date sorts as
// DEFAULT_DATETIME_LOCAL (photos_datetime.py:19-21). One stable sort keeps each
// photo's order. An author is named once, not decoded again for each row.
func loadComments(rows []sqlread.Comment) (map[string][]photoinfo.CommentInfo, map[string][]photoinfo.LikeInfo) {
	type dated struct {
		sqlread.Comment
		date *time.Time
		key  time.Time
	}
	list := make([]dated, len(rows))
	for i, r := range rows {
		date, key := optionalTime(r.Date), time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		if date != nil {
			*date = date.In(time.Local)
			key = *date
		}
		list[i] = dated{r, date, key}
	}
	slices.SortStableFunc(list, func(a, b dated) int { return a.key.Compare(b.key) })
	comments, likes, names := map[string][]photoinfo.CommentInfo{}, map[string][]photoinfo.LikeInfo{}, map[string]string{}
	for _, r := range list {
		if _, ok := names[r.UserID.String]; !ok {
			names[r.UserID.String] = personName(r.User, r.UserComponents)
		}
		id, user, mine, text := r.PhotoUUID.String, names[r.UserID.String], r.IsMine.Valid && r.IsMine.Int64 != 0, nfc(r.Text.String)
		switch {
		case r.IsLike.Valid && r.IsLike.Int64 != 0:
			likes[id] = append(likes[id], photoinfo.LikeInfo{Datetime: r.date, User: user, IsMine: mine})
		case text != "":
			comments[id] = append(comments[id], photoinfo.CommentInfo{Datetime: r.date, User: user, IsMine: mine, Text: text})
		}
	}
	return comments, likes
}
