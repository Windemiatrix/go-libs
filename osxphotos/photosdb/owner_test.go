package photosdb

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// The names of _db_hashed_person_id (_photosdb_process_comments.py:52-89),
// the last row of an ID winning as there. It is the query of
// sqlread.personIndex, so it holds the reading of the rows, not the rule:
// templates.json ({photo.owner}) and the owner key of query.json are the
// independent references once they exist.
func ownerNames(t *testing.T, ro *sql.DB) map[string]string {
	t.Helper()
	rows, err := ro.QueryContext(t.Context(), `
		SELECT ZINVITEEHASHEDPERSONID AS ID, ZINVITEEFIRSTNAME AS F, ZINVITEELASTNAME AS L, ZINVITEEFULLNAME AS N
		FROM ZCLOUDSHAREDALBUMINVITATIONRECORD WHERE ID IS NOT NULL AND ID != '' AND NOT (F IS NULL AND L IS NULL)
		UNION
		SELECT ZCLOUDOWNERHASHEDPERSONID, ZCLOUDOWNERFIRSTNAME, ZCLOUDOWNERLASTNAME, ZCLOUDOWNERFULLNAME
		FROM ZGENERICALBUM WHERE ZCLOUDOWNERHASHEDPERSONID IS NOT NULL AND ZCLOUDOWNERHASHEDPERSONID != ''
		AND NOT (ZCLOUDOWNERFIRSTNAME IS NULL AND ZCLOUDOWNERLASTNAME IS NULL)
		ORDER BY 1, 2, 3, 4`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	names := map[string]string{}
	for rows.Next() {
		var id string
		var first, last, full sql.NullString
		if err := rows.Scan(&id, &first, &last, &full); err != nil {
			t.Fatal(err)
		}
		names[id] = norm.NFC.String(full.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

// The IDs that the second source of names may hold
// (_photosdb_process_comments.py:155-187); an archive needs decoding, which
// TestOpenOwner checks on literals.
func ownerParticipantIDs(t *testing.T, ro *sql.DB) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	if count(t, ro, "SELECT COUNT(*) FROM pragma_table_info('ZSHAREPARTICIPANT') WHERE name IN ('ZHASHEDPERSONID', 'ZNAMECOMPONENTS')") != 2 {
		return ids
	}
	for _, id := range testlib.Rows(t, ro, "SELECT ZHASHEDPERSONID FROM ZSHAREPARTICIPANT "+
		"WHERE ZHASHEDPERSONID IS NOT NULL AND ZHASHEDPERSONID != '' AND ZNAMECOMPONENTS IS NOT NULL") {
		ids[id] = true
	}
	return ids
}

// Upstream: photoinfo.py:1441-1459. The messages hold UUIDs and numbers only.
func TestOwner(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	named := 0
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				names, participants := ownerNames(t, ro), ownerParticipantIDs(t, ro)
				withID, withName, fallback, compared := 0, 0, 0, 0
				for _, p := range db.Photos() {
					if p.Owner != "" {
						withName++
					}
					if p.CloudOwnerHashedID == "" {
						if p.Owner != "" {
							t.Errorf("photo %s: an owner name without an owner ID", p.UUID)
						}
						continue
					}
					withID++
					want, found := names[p.CloudOwnerHashedID]
					if !found && participants[p.CloudOwnerHashedID] {
						fallback++
						continue
					}
					if p.Owner != want {
						t.Errorf("photo %s: Owner differs from the name of its owner ID in the database (found %v)", p.UUID, found)
					} else if want != "" {
						compared++
					}
				}
				t.Logf("%d photos: %d with an owner ID, %d with an owner name, %d of them compared, %d named from ZSHAREPARTICIPANT only",
					len(db.Photos()), withID, withName, compared, fallback)
				mu.Lock()
				defer mu.Unlock()
				named += compared
			})
		}
	})
	if named == 0 {
		t.Error("no photo of any library has an owner name compared with the database: the test is vacuous")
	}
}

// Upstream: _photosdb_process_comments.py:52-89, 155-187; photoinfo.py:1441-1459.
func TestOpenOwner(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	// The cases need distinct photos only; this helper picks them in ZUUID order.
	photos := photosWithoutComments(t, lib, 5)
	archive := sevenPartArchive(t)
	ids := []string{"'TEST-HASH-1'", "'TEST-HASH-2'", "'TEST-HASH-3'", "'TEST-HASH-UNKNOWN'", "''"}
	stmts := []string{
		"INSERT INTO ZCLOUDSHAREDALBUMINVITATIONRECORD (Z_PK, ZINVITEEHASHEDPERSONID, ZINVITEEFIRSTNAME, ZINVITEEFULLNAME) " +
			"VALUES (900001, 'TEST-HASH-1', 'Person', 'Cafe\U00000301'), (900002, 'TEST-HASH-3', 'Person', NULL), (900003, '', 'Person', 'Person E')",
		fmt.Sprintf("INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES "+
			"(900001, 'TEST-HASH-2', X'%x'), (900002, 'TEST-HASH-3', X'%x')", archive, archive),
	}
	for i, u := range photos {
		stmts = append(stmts, "UPDATE ZASSET SET ZCLOUDOWNERHASHEDPERSONID = "+ids[i]+" WHERE ZUUID = '"+u+"'")
	}
	change(t, lib, stmts...)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// TEST-HASH-3 has a row without a full name: upstream then skips its
	// participant and has None. Upstream leaves out a row of the ID ''
	// (_photosdb_process_comments.py:60-61).
	for i, want := range []string{"Caf\U000000e9", "Jos\U000000e9 Part2", "", "", ""} {
		p, ok := db.Photo(photos[i])
		if !ok {
			t.Fatalf("photo %s not found", photos[i])
		}
		if p.Owner != want {
			t.Errorf("case %d, photo %s: Owner differs from the literal of the case", i, p.UUID)
		}
	}
}
