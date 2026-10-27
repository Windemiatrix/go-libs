package sqlread

import (
	"database/sql"
	"path/filepath"
	"reflect"
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

// The rows are synthetic: the test writes its own entity number, shares,
// participants and names over the synthetic library, so that the CollectionShare
// of reference/006_shared-album.sql and the other ZSHARE rows fall outside it.
func collectionShareDB(t *testing.T, stmts ...string) string {
	t.Helper()
	path := syntheticDB(t)
	w := openWriter(t, path)
	exec(t, w,
		"DELETE FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare'",
		"INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME, Z_SUPER, Z_MAX) VALUES (900, 'CollectionShare', 0, 0)",
		"INSERT INTO ZSHARE (Z_PK, Z_ENT, ZUUID, ZTITLE, ZTRASHEDSTATE, ZCREATIONDATE, ZSTARTDATE, ZENDDATE, ZCUSTOMSORTKEY, ZCUSTOMSORTASCENDING) VALUES "+
			"(900001, 900, 'TEST-SHARE-A', 'TEST-TITLE-A', NULL, 694224000, 694224100, 694224200, 1, 0), "+
			"(900002, 900, 'TEST-SHARE-B', NULL, 1, NULL, NULL, NULL, NULL, NULL), "+
			"(900003, 900, NULL, 'TEST-NO-UUID', 0, NULL, NULL, NULL, NULL, NULL), "+
			"(900004, 901, 'TEST-SHARE-OTHER', 'TEST-OTHER', 0, NULL, NULL, NULL, NULL, NULL)",
		"UPDATE ZASSET SET ZCOLLECTIONSHARE = 900001, ZCLOUDBATCHPUBLISHDATE = CASE Z_PK WHEN 7 THEN 100 ELSE 200 END WHERE Z_PK IN (5, 6, 7)",
		"UPDATE ZASSET SET ZCOLLECTIONSHARE = 900002, ZCLOUDBATCHPUBLISHDATE = NULL WHERE Z_PK = 10",
		"UPDATE ZASSET SET ZCOLLECTIONSHARE = 900003 WHERE Z_PK = 8",
		"UPDATE ZASSET SET ZCOLLECTIONSHARE = 900004 WHERE Z_PK = 9",
		"INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZSHARE, ZROLE, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES "+
			"(899999, 900001, 1, 'TEST-P-UNNAMED', NULL), (900001, 900001, 1, 'TEST-P-NAMED', X'09'), (900002, 900001, 2, 'TEST-P-MEMBER', NULL), "+
			"(900003, 900002, 1, 'TEST-P-FIRST', X'03'), (900004, 900002, 1, 'TEST-P-UNNAMED', X'0102'), "+
			"(900005, 900002, 2, 'TEST-P-UNNAMED', X'0102'), (900006, 900001, 2, 'TEST-P-UNNAMED', X'00')",
		"INSERT INTO ZCLOUDSHAREDALBUMINVITATIONRECORD (Z_PK, ZINVITEEHASHEDPERSONID, ZINVITEEFIRSTNAME, ZINVITEEFULLNAME) VALUES "+
			"(900001, 'TEST-P-NAMED', 'TEST-F', 'TEST-NAME-1')",
	)
	exec(t, w, stmts...)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCollectionShares(t *testing.T) {
	t.Parallel()

	// Upstream reads the trashed state with bool(): NULL is outside the trash (photosdb.py:2906).
	shareA := Album{
		PK: 900001, UUID: "TEST-SHARE-A", Kind: KindSharedAlbum, Title: sql.NullString{String: "TEST-TITLE-A", Valid: true},
		Trashed: sql.NullInt64{Valid: true}, SortKey: sql.NullInt64{Int64: 1, Valid: true}, SortAscending: sql.NullInt64{Valid: true},
		Created: sql.NullFloat64{Float64: 694224000, Valid: true}, Start: sql.NullFloat64{Float64: 694224100, Valid: true},
		End: sql.NullFloat64{Float64: 694224200, Valid: true},
	}
	shareB := Album{PK: 900002, UUID: "TEST-SHARE-B", Kind: KindSharedAlbum, Trashed: sql.NullInt64{Int64: 1, Valid: true}}
	member := func(asset int64, share string, order int64) Member {
		return Member{AssetPK: asset, AlbumUUID: share, Order: sql.NullInt64{Int64: order, Valid: true}, Kind: KindSharedAlbum}
	}
	// A NULL publish date sorts first, a tie goes by Z_PK (photosdb.py:2817-2832).
	wantMembers := []Member{member(10, "TEST-SHARE-B", 0), member(7, "TEST-SHARE-A", 1), member(5, "TEST-SHARE-A", 2), member(6, "TEST-SHARE-A", 3)}

	named := sql.NullString{String: "TEST-NAME-1", Valid: true}
	unnamed := [][]byte{{1, 2}, {1, 2}, {0}}
	tests := []struct {
		name   string
		stmts  []string
		owners func(a, b *Album)
	}{
		{"ZROLE", nil, func(a, b *Album) {
			// A named owner takes the name of the UNION, not its ZNAMECOMPONENTS.
			a.Owner = named
			b.OwnerComponents = unnamed
		}},
		// Photos 12 renames the column (photosdb.py:2845-2847); ZROLE is then not read.
		{"ZCPLROLE", []string{
			"ALTER TABLE ZSHAREPARTICIPANT ADD COLUMN ZCPLROLE INTEGER",
			"UPDATE ZSHAREPARTICIPANT SET ZCPLROLE = 1 WHERE Z_PK IN (900002, 900005)",
		}, func(_, b *Album) { b.OwnerComponents = unnamed }},
		// A missing column gives no owner, as upstream checks the role column and
		// ZNAMECOMPONENTS (photosdb.py:2846-2849; _photosdb_process_comments.py:158-161).
		{"no role column", []string{"ALTER TABLE ZSHAREPARTICIPANT RENAME COLUMN ZROLE TO ZX"}, func(_, _ *Album) {}},
		{"no ZHASHEDPERSONID", []string{"ALTER TABLE ZSHAREPARTICIPANT RENAME COLUMN ZHASHEDPERSONID TO ZX"}, func(_, _ *Album) {}},
		{"no ZNAMECOMPONENTS", []string{"ALTER TABLE ZSHAREPARTICIPANT RENAME COLUMN ZNAMECOMPONENTS TO ZX"}, func(a, _ *Album) { a.Owner = named }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			albums, members, err := openDB(t, collectionShareDB(t, tt.stmts...)).CollectionShares(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			a, b := shareA, shareB
			tt.owners(&a, &b)
			if len(albums) != 2 {
				t.Fatalf("%d shares, want 2", len(albums))
			}
			for i, want := range []Album{a, b} {
				if !reflect.DeepEqual(albums[i], want) {
					t.Errorf("share %d (%s): not the row written (owner %v, %d name archives)", i+1, albums[i].UUID,
						albums[i].Owner.Valid, len(albums[i].OwnerComponents))
				}
			}
			if !reflect.DeepEqual(members, wantMembers) {
				t.Errorf("members %+v, want %+v", members, wantMembers)
			}
		})
	}

	t.Run("no CollectionShare entity", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testlib.OpenPristine(t, "Test-13.0.0"), "database", "Photos.sqlite")
		if n := count(t, openRO(t, path), "SELECT COUNT(*) FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare'"); n != 0 {
			t.Fatal("Test-13.0.0 has the CollectionShare entity; the test needs a library without it")
		}
		albums, members, err := openDB(t, path).CollectionShares(t.Context())
		if albums != nil || members != nil || err != nil {
			t.Errorf("CollectionShares = %d, %d, %v; want nil, nil, nil", len(albums), len(members), err)
		}
	})
}

// A participant of an ID of the UNION is skipped, as upstream does, even when
// the UNION has no full name for it (_photosdb_process_comments.py:172-174);
// the album owner of ZGENERICALBUM also takes the names of ZSHAREPARTICIPANT.
func TestAlbumsOwnerComponents(t *testing.T) {
	t.Parallel()

	const album = "(SELECT Z_PK FROM ZGENERICALBUM WHERE ZKIND = 2 ORDER BY Z_PK LIMIT 1 OFFSET "
	set := func(offset, person string) string {
		return "UPDATE ZGENERICALBUM SET ZCLOUDOWNERHASHEDPERSONID = '" + person + "' WHERE Z_PK = " + album + offset + ")"
	}
	path := collectionShareDB(t,
		set("0", "TEST-P-UNNAMED"), set("1", "TEST-P-NAMED"), set("2", "TEST-P-FIRSTONLY"),
		"INSERT INTO ZCLOUDSHAREDALBUMINVITATIONRECORD (Z_PK, ZINVITEEHASHEDPERSONID, ZINVITEEFIRSTNAME) VALUES (900002, 'TEST-P-FIRSTONLY', 'TEST-F')",
		"INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZSHARE, ZROLE, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES (900007, 900001, 2, 'TEST-P-FIRSTONLY', X'0102')",
	)
	ro := openRO(t, path)
	want := map[int64]Album{
		count(t, ro, "SELECT "+album+"0)"): {OwnerComponents: [][]byte{{1, 2}, {1, 2}, {0}}},
		count(t, ro, "SELECT "+album+"1)"): {Owner: sql.NullString{String: "TEST-NAME-1", Valid: true}},
		count(t, ro, "SELECT "+album+"2)"): {},
	}
	if len(want) != 3 {
		t.Fatalf("%d user albums, want 3", len(want))
	}
	albums, err := openDB(t, path).Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, a := range albums {
		w, ok := want[a.PK]
		if !ok {
			continue
		}
		found++
		if a.Owner != w.Owner || !reflect.DeepEqual(a.OwnerComponents, w.OwnerComponents) {
			t.Errorf("album %d: Owner %v, %d name archives; not the owner written", a.PK, a.Owner.Valid, len(a.OwnerComponents))
		}
	}
	if found != len(want) {
		t.Errorf("%d of the %d albums written read", found, len(want))
	}
}
