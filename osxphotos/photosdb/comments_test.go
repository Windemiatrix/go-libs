package photosdb

import (
	"database/sql"
	"fmt"
	"math"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func commentAt(seconds int64) *time.Time {
	t := time.Unix(coreDataOffset+seconds, 0)
	return &t
}

func sameCommentDate(got, want *time.Time) bool {
	if got == nil || want == nil {
		return got == want
	}
	return got.Equal(*want) && got.Location() == time.Local
}

func sameComments(got, want []photoinfo.CommentInfo) bool {
	return slices.EqualFunc(got, want, func(g, w photoinfo.CommentInfo) bool {
		return sameCommentDate(g.Datetime, w.Datetime) && g.User == w.User && g.IsMine == w.IsMine && g.Text == w.Text
	})
}

func sameLikes(got, want []photoinfo.LikeInfo) bool {
	return slices.EqualFunc(got, want, func(g, w photoinfo.LikeInfo) bool {
		return sameCommentDate(g.Datetime, w.Datetime) && g.User == w.User && g.IsMine == w.IsMine
	})
}

func commentRow(uuid string, like sql.NullInt64, date sql.NullFloat64, text sql.NullString) sqlread.Comment {
	return sqlread.Comment{PhotoUUID: nullString(uuid), IsLike: like, Date: date, Text: text}
}

// Upstream: _photosdb_process_comments.py:120-150.
func TestLoadComments(t *testing.T) {
	t.Parallel()

	later := commentRow("P-1", nullInt(0), nullFloat(694224200), nullString("Second"))
	later.UserID, later.User = nullString("TEST-HASH-1"), nullString("Person A")
	earlier := commentRow("P-1", nullInt(0), nullFloat(694224100), nullString("Cafe\U00000301"))
	earlier.UserID, earlier.UserComponents, earlier.IsMine = nullString("TEST-HASH-2"), [][]byte{sevenPartArchive(t)}, nullInt(1)
	// bool() of upstream: any number but 0 is true, NULL is false.
	likeEarly := commentRow("P-1", nullInt(2), nullFloat(694224100), nullString("A text of a like"))
	likeEarly.IsMine = nullInt(-1)
	rows := []sqlread.Comment{
		later,
		commentRow("P-1", nullInt(0), nullFloat(694224000), nullString("")),
		commentRow("P-1", nullInt(0), nullFloat(694224000), sql.NullString{}),
		commentRow("P-1", nullInt(1), nullFloat(694224300), sql.NullString{}),
		earlier,
		likeEarly,
		commentRow("P-1", sql.NullInt64{}, nullFloat(694224400), nullString("Third")),
		// Upstream sorts the date it reads: 1e12 is past the year 9999, so DEFAULT_DATETIME_LOCAL.
		commentRow("P-1", nullInt(1), nullFloat(1e12), sql.NullString{}),
		commentRow("P-2", nullInt(0), nullFloat(694224000), nullString("A")),
		commentRow("P-2", nullInt(0), nullFloat(694224000), nullString("B")),
		commentRow("P-2", nullInt(1), sql.NullFloat64{}, sql.NullString{}),
		commentRow("P-2", nullInt(1), nullFloat(math.NaN()), sql.NullString{}),
		commentRow("P-2", nullInt(1), nullFloat(1e12), sql.NullString{}),
		commentRow("P-2", nullInt(1), nullFloat(-1e9), sql.NullString{}),
		commentRow("P-3", nullInt(0), nullFloat(694224000), nullString("")),
	}
	// Groups of one date large enough that an unstable sort reorders them.
	byDate := make([][]photoinfo.CommentInfo, 3)
	for i := range 100 {
		seconds := int64(694224000 + 100*(i%3))
		rows = append(rows, commentRow("P-4", nullInt(0), nullFloat(float64(seconds)), nullString(strconv.Itoa(i))))
		byDate[i%3] = append(byDate[i%3], photoinfo.CommentInfo{Datetime: commentAt(seconds), Text: strconv.Itoa(i)})
	}
	db := &DB{}
	db.comments, db.likes = loadComments(rows)

	for _, tt := range []struct {
		uuid     string
		comments []photoinfo.CommentInfo
		likes    []photoinfo.LikeInfo
	}{
		{"P-1",
			[]photoinfo.CommentInfo{
				{Datetime: commentAt(694224100), User: "Jos\U000000e9 Part2", IsMine: true, Text: "Caf\U000000e9"},
				{Datetime: commentAt(694224200), User: "Person A", Text: "Second"},
				{Datetime: commentAt(694224400), Text: "Third"},
			},
			[]photoinfo.LikeInfo{{}, {Datetime: commentAt(694224100), IsMine: true}, {Datetime: commentAt(694224300)}}},
		// list.sort of upstream is stable; a date it cannot read is DEFAULT_DATETIME_LOCAL,
		// later than 1969.
		{"P-2",
			[]photoinfo.CommentInfo{{Datetime: commentAt(694224000), Text: "A"}, {Datetime: commentAt(694224000), Text: "B"}},
			[]photoinfo.LikeInfo{{Datetime: commentAt(-1e9)}, {}, {}, {}}},
		{"P-3", nil, nil},
		{"P-4", slices.Concat(byDate...), nil},
		{"P-unknown", nil, nil},
	} {
		p := &photoInfo{UUID: tt.uuid}
		if got := db.Comments(p); !sameComments(got, tt.comments) || (got == nil) != (tt.comments == nil) {
			t.Errorf("%s: %d comments, want %d with the values of the rows", tt.uuid, len(got), len(tt.comments))
		}
		if got := db.Likes(p); !sameLikes(got, tt.likes) || (got == nil) != (tt.likes == nil) {
			t.Errorf("%s: %d likes, want %d with the values of the rows", tt.uuid, len(got), len(tt.likes))
		}
	}
}

const commentInsert = "INSERT INTO ZCLOUDSHAREDCOMMENT (Z_PK, ZISLIKE, ZCOMMENTDATE, ZCOMMENTTEXT, ZCOMMENTERHASHEDPERSONID, ZISMYCOMMENT, " +
	"ZCOMMENTEDASSET, ZLIKEDASSET) VALUES "

func photosWithoutComments(t *testing.T, lib string, n int) []string {
	t.Helper()
	photos := testlib.Rows(t, roDB(t, lib), fmt.Sprintf("SELECT a.ZUUID FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK "+
		"WHERE NOT EXISTS (SELECT 1 FROM ZCLOUDSHAREDCOMMENT c WHERE a.Z_PK IN (c.ZCOMMENTEDASSET, c.ZLIKEDASSET)) ORDER BY a.ZUUID LIMIT %d", n))
	if len(photos) != n {
		t.Fatalf("%d photos without comments, want %d", len(photos), n)
	}
	return photos
}

func commentAssetPK(photoUUID string) string {
	return "(SELECT Z_PK FROM ZASSET WHERE ZUUID = '" + photoUUID + "')"
}

func TestOpenComments(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	photos := photosWithoutComments(t, lib, 2)
	first, second := commentAssetPK(photos[0]), commentAssetPK(photos[1])
	change(t, lib,
		"INSERT INTO ZCLOUDSHAREDALBUMINVITATIONRECORD (Z_PK, ZINVITEEHASHEDPERSONID, ZINVITEEFIRSTNAME, ZINVITEEFULLNAME) "+
			"VALUES (900001, 'TEST-HASH-1', 'Person', 'Person A')",
		fmt.Sprintf("INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES (900001, 'TEST-HASH-2', X'%x')", sevenPartArchive(t)),
		commentInsert+
			"(900001, 0, 694224200, 'Second', 'TEST-HASH-1', 0, "+first+", NULL), "+
			"(900002, 0, 694224100, 'Cafe\U00000301', 'TEST-HASH-2', 1, "+first+", NULL), "+
			"(900003, 0, 694224000, '', 'TEST-HASH-1', 0, "+first+", NULL), "+
			"(900004, 1, 694224300, NULL, 'TEST-HASH-UNKNOWN', 0, NULL, "+first+"), "+
			"(900005, 1, NULL, NULL, NULL, 1, NULL, "+second+"), "+
			"(900006, 0, 694224500, 'Through the liked asset', NULL, 0, NULL, "+second+"), "+
			"(900007, 1, 694224600, NULL, 'TEST-HASH-1', 0, "+second+", NULL), "+
			"(900008, 1, 694224700, NULL, NULL, 0, "+first+", "+second+"), "+
			"(900010, 0, 694224800, 'Same date A', NULL, 0, "+first+", NULL), "+
			"(900009, 0, 694224800, 'Same date B', NULL, 0, "+first+", NULL)",
	)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	for i, tt := range []struct {
		comments []photoinfo.CommentInfo
		likes    []photoinfo.LikeInfo
	}{
		{
			[]photoinfo.CommentInfo{
				{Datetime: commentAt(694224100), User: "Jos\U000000e9 Part2", IsMine: true, Text: "Caf\U000000e9"},
				{Datetime: commentAt(694224200), User: "Person A", Text: "Second"},
				{Datetime: commentAt(694224800), Text: "Same date B"},
				{Datetime: commentAt(694224800), Text: "Same date A"},
			},
			[]photoinfo.LikeInfo{{Datetime: commentAt(694224300)}, {Datetime: commentAt(694224700)}},
		},
		{
			[]photoinfo.CommentInfo{{Datetime: commentAt(694224500), Text: "Through the liked asset"}},
			[]photoinfo.LikeInfo{{IsMine: true}, {Datetime: commentAt(694224600), User: "Person A"}, {Datetime: commentAt(694224700)}},
		},
	} {
		p, ok := db.Photo(photos[i])
		if !ok {
			t.Fatalf("photo %s not found", photos[i])
		}
		if got := db.Comments(p); !sameComments(got, tt.comments) {
			t.Errorf("photo %s: %d comments, want %d with the values of the rows", p.UUID, len(got), len(tt.comments))
		}
		if got := db.Likes(p); !sameLikes(got, tt.likes) {
			t.Errorf("photo %s: %d likes, want %d with the values of the rows", p.UUID, len(got), len(tt.likes))
		}
	}
}

// Upstream fails on Photos 4 and earlier (_photosdb_process_comments.py:34-39),
// which the port does not read, and on a library without the table or a column
// of its query.
func TestOpenCommentsWithoutTable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		after  []string
		listed int
	}{
		{"a comment", nil, 1},
		{"no ZCLOUDSHAREDCOMMENT", []string{"DROP TABLE ZCLOUDSHAREDCOMMENT"}, 0},
		{"no ZISMYCOMMENT", []string{"ALTER TABLE ZCLOUDSHAREDCOMMENT RENAME COLUMN ZISMYCOMMENT TO ZX"}, 0},
		{"no ZLIKEDASSET", []string{"ALTER TABLE ZCLOUDSHAREDCOMMENT RENAME COLUMN ZLIKEDASSET TO ZX"}, 0},
		{"no ZCOMMENTERHASHEDPERSONID", []string{"ALTER TABLE ZCLOUDSHAREDCOMMENT RENAME COLUMN ZCOMMENTERHASHEDPERSONID TO ZX"}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			photo := photosWithoutComments(t, lib, 1)[0]
			change(t, lib, slices.Concat([]string{
				commentInsert + "(900001, 0, 694224000, 'A comment', NULL, 0, " + commentAssetPK(photo) + ", NULL)",
			}, tt.after)...)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			p, ok := db.Photo(photo)
			if !ok {
				t.Fatalf("photo %s not found", photo)
			}
			if got := len(db.Comments(p)); got != tt.listed {
				t.Errorf("photo %s: %d comments, want %d", photo, got, tt.listed)
			}
		})
	}
}
