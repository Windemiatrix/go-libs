package photosdb

import (
	"database/sql"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
)

const (
	// "Rene" with a combining acute accent, and the same name in NFC.
	personNFD, personNFC = "Rene\U00000301 Example", "Ren\U000000e9 Example"
	unknownName          = "_UNKNOWN_"
)

func personDB(t *testing.T, in tables) *DB {
	t.Helper()
	c, err := load(in)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	persons, byUUID, personsOf := loadPersons(in, c)
	return &DB{photos: c.photos, byUUID: c.byUUID, persons: persons, personByUUID: byUUID, personsOf: personsOf}
}

func schemaOfLibrary(t *testing.T, db *DB) schema.Schema {
	t.Helper()
	s, ok := schema.ForModel(db.Info().ModelVersion)
	if !ok {
		t.Fatal("no schema for the model of the library")
	}
	return s
}

// checkPersonInfoOf holds the invariant between PersonInfoOf and p.Persons,
// which loadPersons and groupFaces build from the same rows of Faces.
func checkPersonInfoOf(t *testing.T, db *DB) (withFaces, faces int) {
	t.Helper()
	for i, p := range db.Photos() {
		got := db.PersonInfoOf(p)
		names := make([]string, len(got))
		for k, pi := range got {
			names[k] = pi.Name
		}
		if !slices.Equal(names, p.Persons) {
			t.Errorf("photo %d: PersonInfoOf has %d persons and Persons %d, or their names differ", i, len(names), len(p.Persons))
		}
		faces += len(got)
		if len(got) > 0 {
			withFaces++
		}
	}
	return withFaces, faces
}

// The expected values are literals: the persons of photosdb.py:1767-1840 and
// personinfo.py:35-108 from built rows.
func TestLoadPersonInfo(t *testing.T) {
	t.Parallel()

	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1")},
		persons: []sqlread.Person{
			{PK: 1, UUID: nullString("P-2"), FullName: nullString(personNFD), DisplayName: nullString(personNFD), KeyAssetUUID: nullString("A-1"),
				FaceCount: nullInt(3), KeyFace: nullInt(11), Type: nullInt(1), ManualOrder: nullInt(2)},
			{PK: 2, UUID: nullString("P-1"), KeyAssetUUID: nullString("A-9"), Type: nullInt(2)},
			{PK: 3, UUID: nullString("P-3"), FullName: nullString(""), DisplayName: nullString("Pat"), KeyFace: nullInt(7)},
			{PK: 4, UUID: nullString("P-4"), FullName: nullString("Pat Example")},
			{PK: 5, UUID: nullString("D"), FullName: nullString("First Example")},
			{PK: 6, UUID: nullString("D"), FullName: nullString("Second Example")},
			{PK: 7},
		},
		feedback: []sqlread.Feedback{
			{PK: 1, PersonPK: 1, Type: nullInt(0)}, {PK: 2, PersonPK: 1, Type: nullInt(1)},
			{PK: 3, PersonPK: 2, Type: sql.NullInt64{}},
			{PK: 4, PersonPK: 3, Type: nullInt(2)},
			{PK: 5, PersonPK: 4, Type: nullInt(1)}, {PK: 6, PersonPK: 4, Type: nullInt(0)},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	got, byUUID, _ := loadPersons(in, c)
	want := []personinfo.PersonInfo{
		{UUID: "P-2", Name: personNFC, DisplayName: personNFC, KeyFace: 11, FaceCount: 3, KeyPhotoUUID: "A-1", Favorite: true, SortOrder: 2},
		{UUID: "P-1", Name: unknownName},
		{UUID: "P-3", Name: unknownName, DisplayName: "Pat", KeyFace: 7, FeatureLess: true},
		{UUID: "P-4", Name: "Pat Example", FeatureLess: true},
		{UUID: "D", Name: "First Example"},
		{UUID: "D", Name: "Second Example"},
		{Name: unknownName},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("persons:\n%+v\nwant:\n%+v", got, want)
	}
	if p := byUUID["D"]; p.Name != "Second Example" || byUUID["P-3"] != want[2] || byUUID[""] != want[6] || len(byUUID) != 6 {
		t.Errorf("byUUID has %d persons, D is %q; want 6 with the empty UUID, D the row with the highest Z_PK", len(byUUID), p.Name)
	}
}

func TestPersonInfoOf(t *testing.T) {
	t.Parallel()

	face := func(pk, person int64, uuid string, name sql.NullString) sqlread.Face {
		return sqlread.Face{PK: pk, AssetPK: 1, AssetUUID: nullString("A-1"), PersonPK: person, PersonUUID: nullString(uuid), FullName: name}
	}

	t.Run("order", func(t *testing.T) {
		t.Parallel()
		db := personDB(t, tables{
			assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2")},
			persons: []sqlread.Person{
				{PK: 1, UUID: nullString("P-1")},
				{PK: 2, UUID: nullString("P-9")},
				{PK: 3, UUID: nullString("P-5"), FullName: nullString("Zed Example")},
				{PK: 4, UUID: nullString("P-7"), FullName: nullString("Alex Example")},
			},
			faces: []sqlread.Face{
				face(1, 2, "P-9", sql.NullString{}), face(2, 1, "P-1", sql.NullString{}), face(3, 4, "P-7", nullString("Alex Example")),
				face(4, 3, "P-5", nullString("Zed Example")), face(5, 1, "P-1", sql.NullString{}),
			},
		})
		a1, a2 := db.byUUID["A-1"], db.byUUID["A-2"]
		if first := db.PersonInfoOf(a1); len(first) > 0 {
			first[0].UUID = "changed"
		}
		var got []string
		for _, p := range db.PersonInfoOf(a1) {
			got = append(got, p.UUID)
		}
		// _UNKNOWN_ sorts after capitalized names in byte order, as upstream sorts by name (photoinfo.py:608-611).
		if want := []string{"P-7", "P-5", "P-1", "P-1", "P-9"}; !slices.Equal(got, want) {
			t.Errorf("PersonInfoOf = %q, want %q", got, want)
		}
		if none := db.PersonInfoOf(a2); len(none) != 0 {
			t.Errorf("PersonInfoOf of a photo without a face has %d persons", len(none))
		}
		if withFaces, faces := checkPersonInfoOf(t, db); withFaces != 1 || faces != 5 {
			t.Errorf("%d photos with faces and %d faces; want 1 and 5", withFaces, faces)
		}

		all := db.PersonInfo()
		var uuids []string
		for _, p := range all {
			uuids = append(uuids, p.UUID)
		}
		if want := []string{"P-1", "P-9", "P-5", "P-7"}; !slices.Equal(uuids, want) {
			t.Errorf("PersonInfo = %q, want %q: the order of Z_PK", uuids, want)
		}
		all[0].Name = "changed"
		if db.PersonInfo()[0].Name == "changed" {
			t.Error("PersonInfo shares its slice")
		}
		if p, ok := db.Person("P-7"); !ok || p.Name != "Alex Example" {
			t.Error("Person(P-7) is not the person Alex Example")
		}
		if _, ok := db.Person("P-0"); ok {
			t.Error("Person(P-0) found a person")
		}
	})

	// Two rows of one ZPERSONUUID: the face is of the row of its Z_PK
	// (photoinfo.py:605-611; personinfo.py:35).
	t.Run("repeated UUID", func(t *testing.T) {
		t.Parallel()
		db := personDB(t, tables{
			assets: []sqlread.Asset{baseAsset(1, "A-1")},
			persons: []sqlread.Person{
				{PK: 1, UUID: nullString("P-D"), FullName: nullString("First Example")},
				{PK: 2, UUID: nullString("P-D"), FullName: nullString("Second Example")},
			},
			faces: []sqlread.Face{face(1, 1, "P-D", nullString("First Example"))},
		})
		got := db.PersonInfoOf(db.byUUID["A-1"])
		if len(got) != 1 || got[0].Name != "First Example" {
			t.Errorf("PersonInfoOf has %d persons, the first is First Example %v; want 1, true", len(got), len(got) > 0 && got[0].Name == "First Example")
		}
		if p, _ := db.Person("P-D"); p.Name != "Second Example" {
			t.Error("Person(P-D) is not the row with the highest Z_PK")
		}
		checkPersonInfoOf(t, db)
	})

	// load skips an asset without attributes before its ZUUID is checked, so it may share the ZUUID of a photo.
	t.Run("asset without attributes", func(t *testing.T) {
		t.Parallel()
		skipped := baseAsset(2, "A-1")
		skipped.HasAttributes = false
		db := personDB(t, tables{
			assets:  []sqlread.Asset{baseAsset(1, "A-1"), skipped},
			persons: []sqlread.Person{{PK: 1, UUID: nullString("P-1"), FullName: nullString("Pat Example")}},
			faces:   []sqlread.Face{{PK: 1, AssetPK: 2, AssetUUID: nullString("A-1"), PersonPK: 1, PersonUUID: nullString("P-1"), FullName: nullString("Pat Example")}},
		})
		if got := db.PersonInfoOf(db.byUUID["A-1"]); len(got) != 0 {
			t.Errorf("PersonInfoOf has %d persons, want 0: the face is of another asset", len(got))
		}
		checkPersonInfoOf(t, db)
	})

	// Go sorts up to 12 elements by insertion, which is stable: 13 faces show the third key.
	t.Run("face order", func(t *testing.T) {
		t.Parallel()
		var faces []sqlread.Face
		for pk := int64(1); pk <= 12; pk++ {
			faces = append(faces, face(pk, 1+pk%2, "P-D", sql.NullString{}))
		}
		faces = append(faces, face(13, 3, "P-A", nullString("Alex Example")))
		db := personDB(t, tables{
			assets: []sqlread.Asset{baseAsset(1, "A-1")},
			persons: []sqlread.Person{
				{PK: 1, UUID: nullString("P-D"), KeyFace: nullInt(1)},
				{PK: 2, UUID: nullString("P-D"), KeyFace: nullInt(2)},
				{PK: 3, UUID: nullString("P-A"), FullName: nullString("Alex Example")},
			},
			faces: faces,
		})
		var got []int64
		for _, p := range db.PersonInfoOf(db.byUUID["A-1"]) {
			got = append(got, p.KeyFace)
		}
		if want := []int64{0, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1}; !slices.Equal(got, want) {
			t.Errorf("KeyFace in PersonInfoOf = %v, want %v: equal names in the order of the faces", got, want)
		}
	})
}

// The oracle is the database: the counts of the same rows by SQL. The messages
// hold counts, not the names of a library.
func TestLoadPersonInfoLibraries(t *testing.T) {
	t.Parallel()

	// Not a JOIN: an asset with two attribute rows would count twice.
	const withAttributes = " WHERE EXISTS (SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"
	var mu sync.Mutex
	var people, faces int
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				s := schemaOfLibrary(t, db)
				var named, keyed, favorite, featureLess int
				for _, p := range db.PersonInfo() {
					if p.Name != unknownName {
						named++
					}
					if p.KeyPhotoUUID != "" {
						keyed++
					}
					if p.Favorite {
						favorite++
					}
					if p.FeatureLess {
						featureLess++
					}
				}
				withFaces, found := checkPersonInfoOf(t, db)
				for _, c := range []struct {
					name  string
					got   int
					query string
				}{
					{"persons", len(db.PersonInfo()), "SELECT COUNT(*) FROM ZPERSON"},
					{"named persons", named, "SELECT COUNT(*) FROM ZPERSON WHERE ZFULLNAME IS NOT NULL AND ZFULLNAME != ''"},
					{"favorites", favorite, "SELECT COUNT(*) FROM ZPERSON WHERE ZTYPE = 1"},
					{"persons featured less", featureLess, "SELECT COUNT(*) FROM ZPERSON p WHERE " +
						"(SELECT u.ZTYPE FROM ZUSERFEEDBACK u WHERE u.ZPERSON = p.Z_PK ORDER BY u.Z_PK LIMIT 1) != 0"},
					{"persons with a key photo", keyed, "SELECT COUNT(*) FROM ZPERSON p JOIN ZDETECTEDFACE kf ON kf.Z_PK = p.ZKEYFACE " +
						"JOIN ZASSET a ON a.Z_PK = kf." + s.FaceAssetColumn + withAttributes},
					{"faces", found, "SELECT COUNT(*) FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn +
						" JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn + withAttributes},
				} {
					if want := count(t, ro, c.query); c.got != want {
						t.Errorf("%s: %d, want %d", c.name, c.got, want)
					}
				}
				t.Logf("%d persons: %d named, %d with a key photo, %d favorites, %d featured less; %d faces on %d photos",
					len(db.PersonInfo()), named, keyed, favorite, featureLess, found, withFaces)
				mu.Lock()
				defer mu.Unlock()
				people += len(db.PersonInfo())
				faces += found
			})
		}
	})
	if people == 0 || faces == 0 {
		t.Errorf("%d persons and %d faces in all libraries; want some of both: the test is vacuous", people, faces)
	}
}

// A person with two ZUSERFEEDBACK rows of different ZTYPE is what the lowest
// Z_PK decides (personinfo.py:99-108), so the rows are written.
func TestLoadPersonInfoFeedback(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "Test-13.0.0")
	change(t, lib,
		"DELETE FROM ZUSERFEEDBACK",
		"INSERT INTO ZUSERFEEDBACK (Z_PK, ZPERSON, ZTYPE) SELECT 1, MAX(Z_PK), 1 FROM ZPERSON",
		"INSERT INTO ZUSERFEEDBACK (Z_PK, ZPERSON, ZTYPE) SELECT 2, MIN(Z_PK), 0 FROM ZPERSON",
		"INSERT INTO ZUSERFEEDBACK (Z_PK, ZPERSON, ZTYPE) SELECT 3, MIN(Z_PK), 1 FROM ZPERSON",
	)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// PersonInfo is in Z_PK order: the first person has the lowest Z_PK.
	all := db.PersonInfo()
	if len(all) < 2 {
		t.Fatalf("Test-13.0.0 has %d persons, want two or more: the case is vacuous", len(all))
	}
	if first, last := all[0], all[len(all)-1]; first.FeatureLess || !last.FeatureLess {
		t.Errorf("FeatureLess of the first person %v, of the last %v; want false (rows 0, then 1) and true", first.FeatureLess, last.FeatureLess)
	}
}

// Test-13.0.0 has persons with faces. The changes are what an application
// meets in a library that Photos is changing: a face whose person is gone, a
// person without a name and a person without a key face. The rows are chosen
// by integer keys, so that no text of the library goes into SQL.
func TestLoadPersonInfoDamaged(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "Test-13.0.0")
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s := schemaOfLibrary(t, before)
	var photo *photoInfo
	for _, p := range before.Photos() {
		if len(p.Persons) > 0 {
			photo = p
			break
		}
	}
	ro := roDB(t, lib)
	keyed := count(t, ro, "SELECT COALESCE(MIN(p.Z_PK), 0) FROM ZPERSON p JOIN ZDETECTEDFACE kf ON kf.Z_PK = p.ZKEYFACE "+
		"JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = kf."+s.FaceAssetColumn)
	named := count(t, ro, "SELECT COALESCE(MIN(Z_PK), 0) FROM ZPERSON WHERE ZFULLNAME != '' AND Z_PK != "+strconv.Itoa(keyed))
	if photo == nil || keyed == 0 || named == 0 {
		t.Fatal("Test-13.0.0 has no photo with a face, no person with a key photo or no other person with a name: the case is vacuous")
	}
	// PersonInfo is in Z_PK order.
	index := func(pk int) int { return count(t, ro, "SELECT COUNT(*) FROM ZPERSON WHERE Z_PK < "+strconv.Itoa(pk)) }
	namedAt, keyedAt := index(named), index(keyed)
	n, asset := len(photo.Persons), strconv.FormatInt(photo.PK, 10)
	change(t, lib,
		"UPDATE ZDETECTEDFACE SET "+s.FacePersonColumn+" = 999999999 WHERE Z_PK = (SELECT MIN(f.Z_PK) FROM ZDETECTEDFACE f "+
			"JOIN ZPERSON p ON p.Z_PK = f."+s.FacePersonColumn+" WHERE f."+s.FaceAssetColumn+" = "+asset+")",
		"UPDATE ZPERSON SET ZFULLNAME = NULL, ZDISPLAYNAME = 'Pat Example' WHERE Z_PK = "+strconv.Itoa(named),
		"UPDATE ZPERSON SET ZKEYFACE = NULL WHERE Z_PK = "+strconv.Itoa(keyed),
	)

	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	checkPersonInfoOf(t, db)
	after := db.byUUID[photo.UUID]
	want := count(t, roDB(t, lib), "SELECT COUNT(*) FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f."+s.FacePersonColumn+
		" WHERE f."+s.FaceAssetColumn+" = "+asset)
	if want != n-1 || len(db.PersonInfoOf(after)) != want || len(after.Persons) != want {
		t.Errorf("the photo had %d faces; now %d by SQL, %d in PersonInfoOf, %d in Persons; want one face less in all", n, want,
			len(db.PersonInfoOf(after)), len(after.Persons))
	}
	persons := db.PersonInfo()
	if p := persons[namedAt]; p.Name != unknownName || p.DisplayName != "Pat Example" {
		t.Errorf("the person without a name: Name is _UNKNOWN_ %v, DisplayName is Pat Example %v; want both", p.Name == unknownName, p.DisplayName == "Pat Example")
	}
	if p := persons[keyedAt]; p.KeyFace != 0 || p.KeyPhotoUUID != "" {
		t.Errorf("the person without a key face: KeyFace %d, KeyPhotoUUID empty %v; want 0, true", p.KeyFace, p.KeyPhotoUUID == "")
	}
}
