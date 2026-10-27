package sqlread

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

const contributorInsert = "INSERT INTO ZASSETCONTRIBUTOR (Z_PK, ZPARTICIPANT, Z3LIBRARYSCOPEASSETCONTRIBUTORS) VALUES "

func participantInsert(s schema.Schema) string {
	return "INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZACCEPTANCESTATUS, ZISCURRENTUSER, ZEXITSTATE, ZPERMISSION, ZPERSON, " + s.ParticipantShareColumn +
		", ZSHARE, ZEMAILADDRESS, ZPARTICIPANTID, ZPHONENUMBER, ZUSERIDENTIFIER, ZUUID, ZNAMECOMPONENTS) VALUES "
}

func attributesAt(t *testing.T, ro *sql.DB, offset int) (pk int64, assetUUID string) {
	t.Helper()
	err := ro.QueryRowContext(t.Context(), "SELECT x.Z_PK, a.ZUUID FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET "+
		"ORDER BY x.Z_PK LIMIT 1 OFFSET ?", offset).Scan(&pk, &assetUUID)
	if err != nil {
		t.Fatal(err)
	}
	return pk, assetUUID
}

func TestParticipantsValues(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	s := schemaByID(t, "photos11_1")
	ro := openRO(t, path)
	pkA, uuidA := attributesAt(t, ro, 0)
	pkB, uuidB := attributesAt(t, ro, 1)
	w := openWriter(t, path)
	exec(t, w,
		participantInsert(s)+"(900001, 11, 12, 13, 14, 15, 16, 17, 'user@example.com', 'TEST-PID', '+10000000000', 'TEST-UID', 'TEST-PU-1', X'010203')",
		"INSERT INTO ZSHAREPARTICIPANT (Z_PK) VALUES (900002)",
		"INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZNAMECOMPONENTS) VALUES (900003, zeroblob(1048577)), (900004, zeroblob(1048576)), (900005, NULL), (900006, NULL), "+
			"(900007, replace(hex(zeroblob(262145)), '00', '\U0001F600'))",
		// Z_PKs that no participant has, so that a join on the wrong key finds nothing.
		// 900005 has no contributor, the contributor of 900006 has no attributes row.
		contributorInsert+fmt.Sprintf("(800001, 900003, %d), (800002, 900002, %d), (800003, 900001, %d), (800004, 900004, %d), (800005, 900006, 999999999), "+
			"(800006, 900001, %d), (800007, 900001, %d), (800008, 900007, %d)", pkA, pkA, pkB, pkB, pkA, pkB, pkA),
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := openDB(t, path).Participants(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	var inserted []Participant
	for i, p := range got {
		if i > 0 && got[i-1].PK >= p.PK {
			t.Errorf("row %d out of Z_PK order", i+1)
		}
		if p.PK >= 900001 {
			inserted = append(inserted, p)
		}
	}

	ni := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
	ns := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	want := []Participant{
		{
			PK:               900001,
			AcceptanceStatus: ni(11), IsCurrentUser: ni(12), ExitState: ni(13), Permission: ni(14), Person: ni(15), Z54Share: ni(16), Share: ni(17),
			EmailAddress: ns("user@example.com"), ParticipantID: ns("TEST-PID"), PhoneNumber: ns("+10000000000"),
			UserIdentifier: ns("TEST-UID"), UUID: ns("TEST-PU-1"), NameComponents: []byte{1, 2, 3},
			AssetUUIDs: []string{uuidB, uuidA, uuidB},
		},
		{PK: 900002, AssetUUIDs: []string{uuidA}},
		{PK: 900003, AssetUUIDs: []string{uuidA}},
		{PK: 900004, NameComponents: make([]byte, 1<<20), AssetUUIDs: []string{uuidB}},
		{PK: 900006},
		// length() counts the 262145 characters; only the bound in bytes drops the text.
		{PK: 900007, AssetUUIDs: []string{uuidA}},
	}
	if len(inserted) != len(want) {
		t.Fatalf("%d inserted participants, want %d", len(inserted), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(inserted[i], want[i]) {
			t.Errorf("Z_PK %d differs from the inserted row", want[i].PK)
		}
	}
}

func TestParticipantsShareColumn(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	pk, _ := attributesAt(t, openRO(t, path), 0)
	w := openWriter(t, path)
	exec(t, w,
		"INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZPERSON) VALUES (900001, 15)",
		fmt.Sprintf("%s(800001, 900001, %d)", contributorInsert, pk),
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	d := openDB(t, path)
	s := schemaByID(t, "photos11_1")
	s.ParticipantShareColumn = "ZPERSON"
	got, err := d.Participants(t.Context(), s)
	if err != nil || len(got) != 1 || got[0].Z54Share != (sql.NullInt64{Int64: 15, Valid: true}) {
		t.Errorf("%d rows, %v; want one whose Z54Share is the value of ZPERSON", len(got), err)
	}
	s.ParticipantShareColumn = "ZNOSUCHCOLUMN"
	if got, err := d.Participants(t.Context(), s); got != nil || err != nil {
		t.Errorf("a share column the table does not have: %d rows, %v; want nil, nil", len(got), err)
	}
}

func TestParticipantsSchema(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, acceptance string
		after            []string
		rows             int    // inserted participants returned
		err              string // "" for none
	}{
		{"a participant", "1", nil, 1, ""},
		// Upstream catches the error of the query (shareparticipant.py:45-49).
		{"no ZASSETCONTRIBUTOR", "1", []string{"DROP TABLE ZASSETCONTRIBUTOR"}, 0, ""},
		{"a text in ZACCEPTANCESTATUS", "'SECRET-X'", nil, 0, "unexpected value type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := syntheticDB(t)
			pk, _ := attributesAt(t, openRO(t, path), 0)
			w := openWriter(t, path)
			exec(t, w, slices.Concat([]string{
				"INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZACCEPTANCESTATUS) VALUES (900001, " + tt.acceptance + ")",
				fmt.Sprintf("%s(800001, 900001, %d)", contributorInsert, pk),
			}, tt.after)...)
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := openDB(t, path).Participants(t.Context(), schemaByID(t, "photos11_1"))
			if tt.err != "" {
				if got != nil || err == nil || !strings.Contains(err.Error(), ": row ") || !strings.Contains(err.Error(), tt.err) ||
					strings.Contains(err.Error(), "SECRET") {
					t.Errorf("%d rows, error %v; want no rows, row <n>: %s without the value", len(got), err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.rows == 0 && got != nil {
				t.Errorf("%d rows, want nil", len(got))
			}
			if n := len(slices.DeleteFunc(got, func(p Participant) bool { return p.PK != 900001 })); n != tt.rows {
				t.Errorf("%d inserted participants, want %d", n, tt.rows)
			}
		})
	}
}
