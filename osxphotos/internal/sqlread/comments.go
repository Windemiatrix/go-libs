package sqlread

import (
	"context"
	"database/sql"
	"errors"
)

// Comment is one row of the query of _db_comments_uuid
// (_photosdb_process_comments.py:95-118).
type Comment struct {
	PhotoUUID      sql.NullString  // ZASSET.ZUUID of ZCOMMENTEDASSET or ZLIKEDASSET
	IsLike, IsMine sql.NullInt64   // ZISLIKE, ZISMYCOMMENT
	Date           sql.NullFloat64 // ZCOMMENTDATE
	Text           sql.NullString  // ZCOMMENTTEXT
	User           sql.NullString  // as Album.Owner, of ZCOMMENTERHASHEDPERSONID
	UserComponents [][]byte        // as Album.OwnerComponents
}

// The query of _db_comments_uuid without its SQL comments, with ZASSET, the
// asset table of Photos 6 and later (_photosdb_process_comments.py:95-110).
const commentQuery = "SELECT ZASSET.ZUUID, ZCLOUDSHAREDCOMMENT.ZISLIKE, ZCLOUDSHAREDCOMMENT.ZCOMMENTDATE, ZCLOUDSHAREDCOMMENT.ZCOMMENTTEXT, " +
	"ZCLOUDSHAREDCOMMENT.ZCOMMENTERHASHEDPERSONID, ZCLOUDSHAREDCOMMENT.ZISMYCOMMENT FROM ZCLOUDSHAREDCOMMENT JOIN ZASSET ON " +
	"ZASSET.Z_PK = ZCLOUDSHAREDCOMMENT.ZCOMMENTEDASSET OR ZASSET.Z_PK = ZCLOUDSHAREDCOMMENT.ZLIKEDASSET "

// Comments returns the rows of commentQuery in ZCLOUDSHAREDCOMMENT.Z_PK order,
// which upstream leaves to SQLite; nil when a table or column of it is missing.
func (d *DB) Comments(ctx context.Context) ([]Comment, error) {
	err := d.CheckColumns(ctx, map[string][]string{"ZCLOUDSHAREDCOMMENT": {"Z_PK", "ZISLIKE", "ZCOMMENTDATE", "ZCOMMENTTEXT", "ZCOMMENTERHASHEDPERSONID", "ZISMYCOMMENT", "ZCOMMENTEDASSET", "ZLIKEDASSET"}})
	if errors.Is(err, ErrMissingColumn) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	people, err := d.personIndex(ctx)
	if err != nil {
		return nil, err
	}
	var out []Comment
	err = d.readRows(ctx, "comments", commentQuery+"ORDER BY ZCLOUDSHAREDCOMMENT.Z_PK", func(rows *sql.Rows) error {
		var c Comment
		var user sql.NullString
		if err := rows.Scan(&c.PhotoUUID, &c.IsLike, &c.Date, &c.Text, &user, &c.IsMine); err != nil {
			return err
		}
		c.User, c.UserComponents = people.name(user.String)
		out = append(out, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
