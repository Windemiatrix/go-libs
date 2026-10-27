package photosdb

import (
	"database/sql"
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

// Upstream: _photosdb_process_comments.py:120-150. One stable sort of all rows
// gives each photo the order of the stable sort of its own list.
func loadComments(rows []sqlread.Comment) (map[string][]photoinfo.CommentInfo, map[string][]photoinfo.LikeInfo) {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b sqlread.Comment) int { return commentDate(a.Date).Compare(commentDate(b.Date)) })
	comments, likes := map[string][]photoinfo.CommentInfo{}, map[string][]photoinfo.LikeInfo{}
	for _, r := range rows {
		id, date, user := r.PhotoUUID.String, commentDate(r.Date), personName(r.User, r.UserComponents)
		mine, text := r.IsMine.Valid && r.IsMine.Int64 != 0, nfc(r.Text.String)
		switch {
		case r.IsLike.Valid && r.IsLike.Int64 != 0:
			likes[id] = append(likes[id], photoinfo.LikeInfo{Datetime: date, User: user, IsMine: mine})
		case text != "":
			comments[id] = append(comments[id], photoinfo.CommentInfo{Datetime: date, User: user, IsMine: mine, Text: text})
		}
	}
	return comments, likes
}

// photos_datetime_local with default=True (photos_datetime.py:19-21, 70-89).
func commentDate(v sql.NullFloat64) time.Time {
	if t := optionalTime(v); t != nil {
		return t.In(time.Local)
	}
	return time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
}
