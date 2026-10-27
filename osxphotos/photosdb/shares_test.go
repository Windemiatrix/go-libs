package photosdb

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func TestShareInfoFields(t *testing.T) {
	t.Parallel()

	var r sqlread.Share
	rv := reflect.ValueOf(&r).Elem()
	for i := range rv.NumField() {
		switch f := rv.Field(i); f.Interface().(type) {
		case int64:
			f.SetInt(1)
		case sql.NullInt64:
			f.Set(reflect.ValueOf(nullInt(int64(100 + i))))
		case sql.NullFloat64:
			f.Set(reflect.ValueOf(nullFloat(float64(86400 * i))))
		case sql.NullString:
			f.Set(reflect.ValueOf(nullString(fmt.Sprintf("TEST-%02d", i))))
		case []byte:
			f.SetBytes([]byte{byte(i)})
		default:
			t.Fatalf("sqlread.Share.%s: type %s", rv.Type().Field(i).Name, f.Type())
		}
	}
	// Upstream does not normalize the title (shareinfo.py:146-147).
	r.Title = nullString("Cafe\U00000301")
	got := reflect.ValueOf(shareInfo(r)).Elem()
	if got.NumField() != rv.NumField() {
		t.Fatalf("ShareInfo has %d fields, sqlread.Share %d", got.NumField(), rv.NumField())
	}
	for i := range rv.NumField() {
		name := rv.Type().Field(i).Name
		if g := got.FieldByName(name); !g.IsValid() || !converted(g.Interface(), rv.Field(i).Interface()) {
			t.Errorf("ShareInfo.%s is not the value of sqlread.Share.%s", name, name)
		}
	}

	if got := shareInfo(sqlread.Share{PK: 2}); !reflect.DeepEqual(got, &photoinfo.ShareInfo{PK: 2}) {
		t.Errorf("a row of NULLs: %+v", *got)
	}
}

func converted(got, row any) bool {
	switch r := row.(type) {
	case int64:
		return got == any(r)
	case sql.NullInt64:
		p, ok := got.(*int)
		return ok && p != nil && int64(*p) == r.Int64
	case sql.NullFloat64:
		d, ok := got.(*time.Time)
		return ok && d != nil && d.Equal(time.Unix(coreDataOffset+int64(r.Float64), 0)) && d.Location() == time.Local
	case sql.NullString:
		return got == any(r.String)
	case []byte:
		b, ok := got.([]byte)
		return ok && bytes.Equal(b, r)
	}
	return false
}

func TestLocalTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    sql.NullFloat64
		want time.Time // the zero time for nil
	}{
		{"NULL", sql.NullFloat64{}, time.Time{}},
		// Upstream leaves 0 unconverted (shareinfo.py:74-75).
		{"0", nullFloat(0), time.Time{}},
		{"-0", nullFloat(math.Copysign(0, -1)), time.Time{}},
		{"NaN", nullFloat(math.NaN()), time.Time{}},
		{"after the year 9999", nullFloat(1e12), time.Time{}},
		{"a second", nullFloat(1), time.Date(2001, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"2023-01-01", nullFloat(694224000), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		got := localTime(tt.v)
		if tt.want.IsZero() {
			if got != nil {
				t.Errorf("%s: %v, want nil", tt.name, got)
			}
			continue
		}
		if got == nil || !got.Equal(tt.want) || got.Location() != time.Local {
			t.Errorf("%s: %v, want %v in time.Local", tt.name, got, tt.want)
		}
	}
}

func TestLoadShares(t *testing.T) {
	t.Parallel()

	asset := func(pk int64, uuid string, moment, scope sql.NullInt64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		a.MomentShare, a.Scope = moment, scope
		return a
	}
	skipped := asset(7, "A-none", nullInt(1), nullInt(2))
	skipped.HasAttributes = false
	moment, library := loadShares([]sqlread.Share{{PK: 0}, {PK: 1}, {PK: 2}}, []sqlread.Asset{
		asset(1, "A-moment", nullInt(1), sql.NullInt64{}),
		asset(2, "A-library", sql.NullInt64{}, nullInt(2)),
		asset(3, "A-both", nullInt(1), nullInt(2)),
		asset(4, "A-none", sql.NullInt64{}, sql.NullInt64{}),
		asset(5, "A-missing", nullInt(99), nullInt(98)),
		asset(6, "A-zero", nullInt(0), nullInt(0)),
		skipped,
	})
	db := &DB{momentShares: moment, libraryShares: library}
	pk := func(s *photoinfo.ShareInfo) int64 {
		if s == nil {
			return -1
		}
		return s.PK
	}
	tests := []struct {
		uuid            string
		moment, library int64 // Z_PK of the share; -1 for nil
	}{
		{"A-moment", 1, -1},
		{"A-library", -1, 2},
		{"A-both", 1, 2},
		{"A-none", -1, -1},
		{"A-missing", -1, -1},
		// The join of upstream finds a row with Z_PK 0 too (shareinfo.py:140-146).
		{"A-zero", 0, 0},
		{"A-unknown", -1, -1},
	}
	for _, tt := range tests {
		p := &photoInfo{UUID: tt.uuid}
		if m, l := pk(db.SharedMomentInfo(p)), pk(db.ShareInfo(p)); m != tt.moment || l != tt.library {
			t.Errorf("%s: SharedMomentInfo %d, ShareInfo %d; want %d, %d", tt.uuid, m, l, tt.moment, tt.library)
		}
	}
}

func TestShareInfoLibraries(t *testing.T) {
	t.Parallel()

	// A share through ZLIBRARYSCOPE is checked whatever the test libraries hold.
	const scoped = "synthetic with a photo in a shared library"
	libs := map[string]string{scoped: testlib.OpenPristine(t, "synthetic")}
	change(t, libs[scoped], "UPDATE ZASSET SET ZLIBRARYSCOPE = (SELECT MIN(Z_PK) FROM ZSHARE) "+
		"WHERE Z_PK = (SELECT MIN(ZASSET) FROM ZADDITIONALASSETATTRIBUTES)")
	for _, name := range testlib.Libraries(t) {
		libs[name] = testlib.OpenPristine(t, name)
	}
	for name, path := range libs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, path)
			hasTable := count(t, src, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'ZSHARE'") == 1
			for _, k := range []struct {
				column string
				get    func(*photoInfo) *photoinfo.ShareInfo
			}{{"ZMOMENTSHARE", db.SharedMomentInfo}, {"ZLIBRARYSCOPE", db.ShareInfo}} {
				linked := 0
				for i, p := range db.Photos() {
					var want *photoinfo.ShareInfo
					if hasTable {
						want = joinedShare(t, src, "SELECT s.Z_PK, s.ZTITLE, s.ZUUID FROM ZSHARE s JOIN ZASSET a ON a."+k.column+
							" = s.Z_PK WHERE a.ZUUID = ?", p.UUID)
					}
					got := k.get(p)
					if !sameShare(got, want) {
						t.Errorf("photo %d: the share differs from the ZSHARE row of ZASSET.%s", i, k.column)
					}
					if got != nil {
						linked++
					}
				}
				t.Logf("%d photos with a share through ZASSET.%s", linked, k.column)
				if name == scoped && linked == 0 {
					t.Errorf("no photo with a share through ZASSET.%s: the case is vacuous", k.column)
				}
			}
		})
	}
}

func sameShare(got, want *photoinfo.ShareInfo) bool {
	if got == nil || want == nil {
		return got == want
	}
	return got.PK == want.PK && got.Title == want.Title && got.UUID == want.UUID
}

func joinedShare(t *testing.T, src *sql.DB, query, photoUUID string) *photoinfo.ShareInfo {
	t.Helper()
	var (
		s         photoinfo.ShareInfo
		title, id sql.NullString
	)
	err := src.QueryRowContext(t.Context(), query, photoUUID).Scan(&s.PK, &title, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	s.Title, s.UUID = title.String, id.String
	return &s
}

// Every key of _NSPersonNameComponentsData (shareparticipant.py:107-117) with
// its own value; the given name in NFD.
func sevenPartArchive(t *testing.T) []byte {
	t.Helper()
	class := func(name string) map[string]any {
		return map[string]any{"$classname": name, "$classes": []any{name, "NSObject"}}
	}
	data := map[string]any{"$class": plist.UID(3), "NSgivenName": plist.UID(5)}
	objects := []any{
		"$null", map[string]any{"$class": plist.UID(4), "NSnameComponentsPrivate": plist.UID(2)}, data,
		class("_NSPersonNameComponentsData"), class("NSPersonNameComponents"), "Jose\U00000301",
	}
	for i, key := range []string{"NSmiddleName", "NSfamilyName", "NSnamePrefix", "NSnameSuffix", "NSnickname", "NSphoneticRepresentation"} {
		data[key] = plist.UID(len(objects))
		objects = append(objects, fmt.Sprintf("Part%d", i+1))
	}
	return marshal(t, map[string]any{
		"$archiver": "NSKeyedArchiver", "$version": 100000,
		"$top": map[string]any{"root": plist.UID(1)}, "$objects": objects,
	}, plist.BinaryFormat)
}

var (
	sevenParts = photoinfo.PersonNameComponents{
		GivenName: "Jos\U000000e9", MiddleName: "Part1", FamilyName: "Part2", NamePrefix: "Part3",
		NameSuffix: "Part4", Nickname: "Part5", PhoneticRepresentation: "Part6",
	}
	// nameArchive with NULL given and family names keeps its middle name.
	middleOnly = photoinfo.PersonNameComponents{MiddleName: "Jos\U000000e9"}
)

func everyValue(pk int64, uuid string) photoinfo.ShareParticipant {
	n := func(v int) *int { return &v }
	return photoinfo.ShareParticipant{
		PK: pk, AcceptanceStatus: n(11), IsCurrentUser: n(12), ExitState: n(13), Permission: n(14),
		Person: n(15), ScopeShare: n(16), Share: n(17),
		EmailAddress: "user@example.com", ParticipantID: "TEST-PID", PhoneNumber: "+10000000000",
		UserIdentifier: "TEST-UID", UUID: uuid, NameComponents: sevenParts,
	}
}

// Upstream fails on an archive that does not unarchive or whose root has no
// NSnameComponentsPrivate, and gives None for a part the archive does not
// have (shareparticipant.py:153-187).
func TestNameComponents(t *testing.T) {
	t.Parallel()

	const null = plist.UID(0)
	noRoot := marshal(t, map[string]any{"$objects": []any{"$null"}}, plist.BinaryFormat)
	noPrivate := marshal(t, map[string]any{"$top": map[string]any{"root": plist.UID(1)}, "$objects": []any{"$null", map[string]any{"$class": null}}},
		plist.BinaryFormat)
	for _, tt := range []struct {
		name    string
		archive []byte
		want    photoinfo.PersonNameComponents
		ok      bool
	}{
		{"every part", sevenPartArchive(t), sevenParts, true},
		{"no given and family name", nameArchive(t, null, null), middleOnly, true},
		{"NULL", nil, photoinfo.PersonNameComponents{}, false},
		{"not a property list", []byte("not a list"), photoinfo.PersonNameComponents{}, false},
		{"no root", noRoot, photoinfo.PersonNameComponents{}, false},
		{"no NSnameComponentsPrivate", noPrivate, photoinfo.PersonNameComponents{}, false},
		{"an archive over the limit", nameArchiveOf(t, plist.UID(3), plist.UID(4), strings.Repeat("a", maxPlistSize)), photoinfo.PersonNameComponents{}, false},
	} {
		if got, ok := nameComponents(tt.archive); got != tt.want || ok != tt.ok {
			t.Errorf("%s: another name, or decoded %v; want decoded %v", tt.name, ok, tt.ok)
		}
	}
}

func TestLoadParticipants(t *testing.T) {
	t.Parallel()

	full, noName := sevenPartArchive(t), nameArchive(t, plist.UID(0), plist.UID(0))
	row := func(asset string, pk int64, archive []byte) sqlread.Participant {
		return sqlread.Participant{AssetUUID: nullString(asset), PK: pk, NameComponents: archive}
	}
	every := row("P-1", 1, full)
	every.AcceptanceStatus, every.IsCurrentUser, every.ExitState, every.Permission = nullInt(11), nullInt(12), nullInt(13), nullInt(14)
	every.Person, every.ScopeShare, every.Share = nullInt(15), nullInt(16), nullInt(17)
	every.EmailAddress, every.ParticipantID, every.PhoneNumber = nullString("user@example.com"), nullString("TEST-PID"), nullString("+10000000000")
	// Upstream passes the strings of the row as they are (shareparticipant.py:47).
	every.UserIdentifier, every.UUID = nullString("TEST-UID"), nullString("Cafe\U00000301")
	db := &DB{participants: loadParticipants([]sqlread.Participant{
		every, row("P-1", 2, noName),
		row("P-2", 3, full), row("P-2", 4, nil),
		row("P-3", 5, []byte("not a list")),
	})}

	want := []photoinfo.ShareParticipant{everyValue(1, "Cafe\U00000301"), {PK: 2, NameComponents: middleOnly}}
	p1 := &photoInfo{UUID: "P-1"}
	if got := db.ShareParticipantInfo(p1); !reflect.DeepEqual(got, want) {
		t.Errorf("P-1: %d participants, not the values of the rows", len(got))
	}
	// photoinfo.py:1646-1649 joins the parts even when they are empty.
	if got := db.ShareParticipants(p1); !slices.Equal(got, []string{"Jos\U000000e9 Part2", " "}) {
		t.Errorf("P-1: %d names, not the given and family names", len(got))
	}
	// One archive that does not decode empties the list (shareparticipant.py:45-49).
	for _, u := range []string{"P-2", "P-3", "P-unknown"} {
		p := &photoInfo{UUID: u}
		if got, names := db.ShareParticipantInfo(p), db.ShareParticipants(p); got != nil || names != nil {
			t.Errorf("%s: %d participants, %d names; want nil", u, len(got), len(names))
		}
	}
}

const participantInsert = "INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZACCEPTANCESTATUS, ZISCURRENTUSER, ZEXITSTATE, ZPERMISSION, ZPERSON, Z66_SHARE, ZSHARE, " +
	"ZEMAILADDRESS, ZPARTICIPANTID, ZPHONENUMBER, ZUSERIDENTIFIER, ZUUID, ZNAMECOMPONENTS) VALUES "

const contributorInsert = "INSERT INTO ZASSETCONTRIBUTOR (Z_PK, ZPARTICIPANT, Z3LIBRARYSCOPEASSETCONTRIBUTORS) VALUES "

func attributesOf(photoUUID string) string {
	return "(SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET WHERE a.ZUUID = '" + photoUUID + "')"
}

// The literal Z66_SHARE checks the column of the synthetic schema, photos11_1.
func TestOpenShareParticipants(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	photos := testlib.Rows(t, roDB(t, lib), "SELECT a.ZUUID FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK "+
		"WHERE NOT EXISTS (SELECT 1 FROM ZASSETCONTRIBUTOR c WHERE c.Z3LIBRARYSCOPEASSETCONTRIBUTORS = x.Z_PK) ORDER BY a.ZUUID LIMIT 3")
	if len(photos) != 3 {
		t.Fatalf("%d photos without a contributor in the synthetic library, want 3", len(photos))
	}
	full, noName := sevenPartArchive(t), nameArchive(t, plist.UID(0), plist.UID(0))
	change(t, lib,
		fmt.Sprintf(participantInsert+"(900001, 11, 12, 13, 14, 15, 16, 17, 'user@example.com', 'TEST-PID', '+10000000000', 'TEST-UID', 'TEST-PU-1', X'%x')", full),
		fmt.Sprintf("INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZNAMECOMPONENTS) VALUES (900002, X'%x'), (900003, NULL), (900004, X'%x')", noName, full),
		// In reverse Z_PK order of the participants; the last one has no attributes row.
		contributorInsert+"(900001, 900002, "+attributesOf(photos[0])+"), (900002, 900001, "+attributesOf(photos[0])+"), "+
			"(900003, 900004, "+attributesOf(photos[1])+"), (900004, 900003, "+attributesOf(photos[1])+"), (900005, 900001, 999999999)",
	)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	want := []photoinfo.ShareParticipant{everyValue(900001, "TEST-PU-1"), {PK: 900002, NameComponents: middleOnly}}
	for i, tt := range []struct {
		want  []photoinfo.ShareParticipant
		names []string
	}{
		{want, []string{"Jos\U000000e9 Part2", " "}},
		{nil, nil}, // a participant without an archive (shareparticipant.py:45-49)
		{nil, nil},
	} {
		p, ok := db.Photo(photos[i])
		if !ok {
			t.Fatalf("photo %s not found", photos[i])
		}
		if got := db.ShareParticipantInfo(p); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("photo %s: %d participants, want %d with the values of the rows", p.UUID, len(got), len(tt.want))
		}
		if got := db.ShareParticipants(p); !slices.Equal(got, tt.names) {
			t.Errorf("photo %s: %d names, want %d", p.UUID, len(got), len(tt.names))
		}
	}
	for _, p := range db.Photos() {
		for _, s := range db.ShareParticipantInfo(p) {
			if s.PK >= 900001 && p.UUID != photos[0] {
				t.Errorf("photo %s: participant %d", p.UUID, s.PK)
			}
		}
	}
}

func TestOpenShareParticipantsSchema(t *testing.T) {
	t.Parallel()

	archive := sevenPartArchive(t)
	for _, tt := range []struct {
		name, acceptance string
		after            []string
		listed           int    // photos with the participant
		err              string // "" for none
	}{
		{"a participant", "1", nil, 1, ""},
		// Upstream catches the error of the query (shareparticipant.py:45-49).
		{"no ZASSETCONTRIBUTOR", "1", []string{"DROP TABLE ZASSETCONTRIBUTOR"}, 0, ""},
		{"a text in ZACCEPTANCESTATUS", "'SECRET-X'", nil, 0, "unexpected value type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			change(t, lib, slices.Concat([]string{
				fmt.Sprintf("INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZACCEPTANCESTATUS, ZNAMECOMPONENTS) VALUES (900001, %s, X'%x')", tt.acceptance, archive),
				contributorInsert + "(900001, 900001, (SELECT MIN(x.Z_PK) FROM ZADDITIONALASSETATTRIBUTES x " +
					"WHERE NOT EXISTS (SELECT 1 FROM ZASSETCONTRIBUTOR c WHERE c.Z3LIBRARYSCOPEASSETCONTRIBUTORS = x.Z_PK)))",
			}, tt.after)...)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if tt.err != "" {
				msg := ""
				if err != nil {
					msg = err.Error()
				}
				if db != nil || !strings.HasPrefix(msg, "sqlread: share participants: row ") || !strings.Contains(msg, tt.err) || strings.Contains(msg, "SECRET") {
					t.Fatalf("Open: a DB %v, the expected error %v, a value in it %v", db != nil, strings.Contains(msg, tt.err), strings.Contains(msg, "SECRET"))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			listed := 0
			for _, p := range db.Photos() {
				if slices.ContainsFunc(db.ShareParticipantInfo(p), func(s photoinfo.ShareParticipant) bool { return s.PK == 900001 }) {
					listed++
				}
			}
			if listed != tt.listed {
				t.Errorf("%d photos with the participant, want %d", listed, tt.listed)
			}
		})
	}
}

// A photo lists every row of the query of upstream, or none when one archive
// does not decode (shareparticipant.py:21-49).
func TestShareParticipantsLibraries(t *testing.T) {
	t.Parallel()

	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)
			s := schemaByID(t, libraryModels[name].id)
			participant := testlib.Rows(t, src, "SELECT name FROM pragma_table_info('ZSHAREPARTICIPANT')")
			if len(participant) > 0 && !slices.Contains(participant, s.ParticipantShareColumn) {
				t.Errorf("ZSHAREPARTICIPANT has no column %s", s.ParticipantShareColumn)
			}
			join := len(participant) > 0 &&
				slices.Contains(testlib.Rows(t, src, "SELECT name FROM pragma_table_info('ZASSETCONTRIBUTOR')"), "Z3LIBRARYSCOPEASSETCONTRIBUTORS")
			listed := 0
			for i, p := range db.Photos() {
				got, want := len(db.ShareParticipantInfo(p)), 0
				if join {
					want = count(t, src, "SELECT COUNT(*) FROM ZSHAREPARTICIPANT p JOIN ZASSETCONTRIBUTOR c ON c.ZPARTICIPANT = p.Z_PK "+
						"JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = c.Z3LIBRARYSCOPEASSETCONTRIBUTORS JOIN ZASSET a ON a.Z_PK = x.ZASSET "+
						"WHERE a.ZUUID = '"+p.UUID+"'")
				}
				if got != 0 && got != want {
					t.Errorf("photo %d: %d participants, want %d or none", i, got, want)
				}
				listed += got
			}
			t.Logf("%d participants of photos", listed)
		})
	}
}
