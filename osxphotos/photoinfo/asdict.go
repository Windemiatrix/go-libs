package photoinfo

import "time"

// Upstream has DEFAULT_DATETIME_LOCAL where the database has no date
// (_photosdb_process_comments.py:130, photos_datetime.py:19-21).
func orNoDate(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
}

// CommentInfo and LikeInfo hold "" where upstream has None
// (_photosdb_process_comments.py:125-128), which json writes as null.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AsDict is asdict of CommentInfo (commentinfo.py:19-20): datetime a
// time.Time, 1970-01-01 00:00 in time.Local for a nil Datetime; user nil for ""
// (None upstream).
func (c CommentInfo) AsDict() map[string]any {
	return map[string]any{"datetime": orNoDate(c.Datetime), "user": strOrNil(c.User), "ismine": c.IsMine, "text": c.Text}
}

// AsDict is asdict of LikeInfo (commentinfo.py:31-32), as CommentInfo.AsDict
// without text.
func (l LikeInfo) AsDict() map[string]any {
	return map[string]any{"datetime": orNoDate(l.Datetime), "user": strOrNil(l.User), "ismine": l.IsMine}
}
