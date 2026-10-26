package sqlread

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func ownersByPK(t *testing.T, path string) map[int64]sql.NullString {
	t.Helper()
	albums, err := openDB(t, path).Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]sql.NullString{}
	for _, a := range albums {
		out[a.PK] = a.Owner
	}
	return out
}

// Upstream runs its query without ORDER BY (_photosdb_process_comments.py:52-89),
// and an empty or NULL owner ID has no owner there (albuminfo.py:122-133).
func upstreamOwners(t *testing.T, path string) map[int64]sql.NullString {
	t.Helper()
	ro := openRO(t, path)
	each := func(q string, scan func(*sql.Rows) error) {
		rows, err := ro.QueryContext(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := scan(rows); err != nil {
				t.Fatal(err)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]sql.NullString{}
	each(hashedPersonQuery, func(rows *sql.Rows) error {
		var id string
		var first, last, full sql.NullString
		err := rows.Scan(&id, &first, &last, &full)
		names[id] = full
		return err
	})
	out := map[int64]sql.NullString{}
	each("SELECT Z_PK, ZCLOUDOWNERHASHEDPERSONID FROM ZGENERICALBUM WHERE ZUUID IS NOT NULL AND ZKIND IN (2, 1505, 1506, 1508, 3999, 4000)",
		func(rows *sql.Rows) error {
			var pk int64
			var id sql.NullString
			err := rows.Scan(&pk, &id)
			out[pk] = names[id.String]
			return err
		})
	return out
}

// The names are synthetic: the test writes them over the rows of the library.
// Any rows that Albums reads do: the owner does not depend on the kind
// (albuminfo.py:113-133). Photos 11 and later keep no names in these columns
// (_photosdb_process_comments.py:91-93), so the library is one of Photos 8.
func TestAlbumsOwner(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testlib.OpenPristine(t, "Test-Cloud-13.1"), "database", "Photos.sqlite")
	w := openWriter(t, path)
	const row = "SELECT Z_PK FROM ZGENERICALBUM WHERE ZUUID IS NOT NULL AND ZKIND IN (2, 1505, 1506, 1508, 3999, 4000) " +
		"ORDER BY Z_PK LIMIT 1 OFFSET "
	set := func(offset, person, first, last, full string) string {
		return "UPDATE ZGENERICALBUM SET ZCLOUDOWNERHASHEDPERSONID = " + person + ", ZCLOUDOWNERFIRSTNAME = " + first +
			", ZCLOUDOWNERLASTNAME = " + last + ", ZCLOUDOWNERFULLNAME = " + full + " WHERE Z_PK = (" + row + offset + ")"
	}
	changed := map[int64]struct {
		name string
		want sql.NullString
	}{
		count(t, w, row+"0"): {"of three rows of one ID the last of the UNION wins", sql.NullString{String: "TEST-NAME-2", Valid: true}},
		count(t, w, row+"1"): {"a person without a first and a last name has no name", sql.NullString{}},
		count(t, w, row+"2"): {"the last row of an ID wins even without a full name", sql.NullString{}},
		count(t, w, row+"3"): {"an empty owner ID has no owner", sql.NullString{}},
		count(t, w, row+"4"): {"a NULL owner ID has no owner", sql.NullString{}},
	}
	exec(t, w,
		set("0", "'TEST-P1'", "'TEST-F'", "NULL", "'TEST-NAME-1'"),
		set("1", "'TEST-P2'", "NULL", "NULL", "'TEST-NAME-X'"),
		set("2", "'TEST-P3'", "'TEST-G'", "NULL", "NULL"),
		set("3", "''", "'TEST-F'", "NULL", "'TEST-NAME-E'"),
		set("4", "NULL", "'TEST-F'", "NULL", "'TEST-NAME-N'"),
		"INSERT INTO ZCLOUDSHAREDALBUMINVITATIONRECORD (Z_PK, ZINVITEEHASHEDPERSONID, ZINVITEEFIRSTNAME, ZINVITEELASTNAME, ZINVITEEFULLNAME) VALUES "+
			"(900001, 'TEST-P1', 'TEST-F', NULL, 'TEST-NAME-0'), (900002, 'TEST-P2', NULL, NULL, 'TEST-NAME-X'), "+
			"(900003, 'TEST-P3', NULL, 'TEST-L', 'TEST-NAME-3'), (900004, '', 'TEST-F', NULL, 'TEST-NAME-E'), "+
			"(900005, 'TEST-P1', 'TEST-F', NULL, 'TEST-NAME-2')",
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	owners, want := ownersByPK(t, path), upstreamOwners(t, path)
	if len(owners) != len(want) {
		t.Errorf("%d albums, the upstream query gives %d", len(owners), len(want))
	}
	for pk, up := range want {
		if got := owners[pk]; got != up {
			t.Errorf("row %d: Owner %v, the upstream query gives %v", pk, got, up)
		}
		c, ok := changed[pk]
		switch {
		case ok && up != c.want:
			t.Errorf("%s: the upstream query gives %v, want %v", c.name, up, c.want)
		case !ok && strings.HasPrefix(owners[pk].String, "TEST-"):
			t.Errorf("row %d: Owner %v, a name of the changed rows", pk, owners[pk])
		}
	}
}
