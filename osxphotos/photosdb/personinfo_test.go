package photosdb

import (
	"cmp"
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
	return &DB{photos: c.photos, byUUID: c.byUUID, personData: loadPersons(in, c)}
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
		if n := len(db.FaceInfoOf(p)); n != len(got) {
			t.Errorf("photo %d: FaceInfoOf has %d faces and PersonInfoOf %d persons", i, n, len(got))
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
	d := loadPersons(in, c)
	got, byUUID := d.persons, d.personByUUID
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
	if d.personPK["D"] != 6 || d.personPK["P-3"] != 3 || len(d.personPK) != 6 {
		t.Errorf("personPK of D is %d, of P-3 %d, %d UUIDs; want 6, 3 and 6", d.personPK["D"], d.personPK["P-3"], len(d.personPK))
	}
}

// The expected values are literals: the faces of
// _photosdb_process_faceinfo.py:186-331 from built rows. HasSmile, Manual and
// InTrash differ in some face for every pair, so that a swap of two flags
// fails. The A-9 face is of an asset outside the catalog, the face 10 of an
// asset without attributes that shares the ZUUID of A-1.
func TestLoadFaceInfo(t *testing.T) {
	t.Parallel()

	skipped := baseAsset(3, "A-1")
	skipped.HasAttributes = false
	in := tables{
		assets:  []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), skipped},
		persons: []sqlread.Person{{PK: 1, UUID: nullString("P-2"), FullName: nullString(personNFD)}, {PK: 2, UUID: nullString("P-1")}},
		faces: []sqlread.Face{
			{
				PK: 7, AssetPK: 1, AssetUUID: nullString("A-1"), UUID: nullString("F-7"), PersonPK: 1, PersonUUID: nullString("P-2"), FullName: nullString(personNFD),
				AgeType: nullInt(2), EyeMakeupType: nullInt(1), EyeState: nullInt(3), FacialHairType: nullInt(4), GenderType: nullInt(5),
				GlassesType: nullInt(6), HairColorType: nullInt(7), HasSmile: nullInt(1), InTrash: nullInt(0), LipMakeupType: nullInt(8),
				Manual: nullInt(0), SmileType: nullInt(9), SourceHeight: nullInt(3000), SourceWidth: nullInt(4000),
				CenterX: nullFloat(0.25), CenterY: nullFloat(0.5), Quality: nullFloat(0.75), Roll: nullFloat(0.125), Size: nullFloat(0.2),
			},
			{PK: 8, AssetPK: 1, AssetUUID: nullString("A-1"), PersonPK: 2, PersonUUID: nullString("P-1"), HasSmile: nullInt(0), InTrash: nullInt(0), Manual: nullInt(1)},
			{PK: 9, AssetPK: 99, AssetUUID: nullString("A-9"), PersonPK: 1, PersonUUID: nullString("P-2"), Quality: nullFloat(0.75)},
			{PK: 10, AssetPK: 3, AssetUUID: nullString("A-1"), PersonPK: 1, PersonUUID: nullString("P-2"), Quality: nullFloat(0.9)},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	d := loadPersons(in, c)
	face7 := personinfo.FaceInfo{
		PK: 7, UUID: "F-7", Name: personNFC, AssetUUID: "A-1", PersonUUID: "P-2",
		CenterX: 0.25, CenterY: 0.5, Size: 0.2, Quality: 0.75, SourceWidth: 4000, SourceHeight: 3000,
		HasSmile: true, Manual: false, InTrash: false,
		AgeType: 2, EyeMakeupType: 1, EyeState: 3, FacialHairType: 4, GenderType: 5, GlassesType: 6, HairColorType: 7, LipMakeupType: 8, SmileType: 9,
		Roll: 0.125,
	}
	// A person without a name has the name "" on a face, which sorts first (_photosdb_process_faceinfo.py:283; photoinfo.py:622).
	face8 := personinfo.FaceInfo{PK: 8, AssetUUID: "A-1", PersonUUID: "P-1", Manual: true}
	if want := map[string][]personinfo.FaceInfo{"A-1": {face8, face7}}; !reflect.DeepEqual(d.facesOf, want) {
		t.Errorf("facesOf:\n%+v\nwant:\n%+v", d.facesOf, want)
	}
	var faces []int64
	for _, f := range d.personFaces[1] {
		faces = append(faces, f.PK)
	}
	// The faces of assets outside the catalog are faces of the person too (_photosdb_process_faceinfo.py:228-230, 326-329).
	if want := []int64{10, 7, 9}; !slices.Equal(faces, want) {
		t.Errorf("faces of P-2 %v, want %v: by quality, descending, then Z_PK", faces, want)
	}
	if want := map[int64][]string{1: {"A-1"}, 2: {"A-1"}}; !reflect.DeepEqual(d.personPhotos, want) {
		t.Errorf("personPhotos = %v, want %v", d.personPhotos, want)
	}
}

// Upstream keys the photos and the faces of a person by ZPERSON.Z_PK
// (personinfo.py:58-81); the methods take the ZPERSONUUID of Person.
func TestFaces(t *testing.T) {
	t.Parallel()

	face := func(pk, person int64, asset string, name sql.NullString, quality float64) sqlread.Face {
		assetPK := map[string]int64{"A-1": 1, "A-2": 2, "A-3": 3}[asset]
		return sqlread.Face{PK: pk, AssetPK: assetPK, AssetUUID: nullString(asset), PersonPK: person, FullName: name, Quality: nullFloat(quality)}
	}
	pks := func(faces []personinfo.FaceInfo) []int64 {
		var out []int64
		for _, f := range faces {
			out = append(out, f.PK)
		}
		return out
	}
	uuids := func(photos []*photoInfo) []string {
		var out []string
		for _, p := range photos {
			out = append(out, p.UUID)
		}
		return out
	}

	t.Run("order", func(t *testing.T) {
		t.Parallel()
		zed, alex := nullString("Zed Example"), nullString("Alex Example")
		db := personDB(t, tables{
			assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), baseAsset(3, "A-3")},
			persons: []sqlread.Person{
				{PK: 1, UUID: nullString("P-1"), FullName: zed}, {PK: 2, UUID: nullString("P-2")},
				{PK: 3, UUID: nullString("P-3"), FullName: alex}, {PK: 4, UUID: nullString("P-4")},
			},
			faces: []sqlread.Face{
				face(1, 1, "A-1", zed, 0.5), face(2, 2, "A-1", sql.NullString{}, 0.5), face(3, 3, "A-1", alex, 0.5),
				face(4, 2, "A-1", sql.NullString{}, 0.5), face(5, 1, "A-3", zed, 0.9), face(6, 1, "A-2", zed, 0.5),
				face(7, 1, "A-3", zed, 0.25), face(8, 1, "A-2", zed, 0.5),
			},
		})
		a1 := db.byUUID["A-1"]
		if got, want := pks(db.FaceInfoOf(a1)), []int64{2, 4, 3, 1}; !slices.Equal(got, want) {
			t.Errorf("FaceInfoOf = %v, want %v: by name, then Z_PK", got, want)
		}
		if got := db.FaceInfoOf(db.byUUID["A-2"]); len(got) != 2 || got[0].PK != 6 {
			t.Errorf("FaceInfoOf(A-2) has %d faces; want 6 and 8", len(got))
		}
		if got, want := pks(db.PersonFaces("P-1")), []int64{5, 1, 6, 8, 7}; !slices.Equal(got, want) {
			t.Errorf("PersonFaces = %v, want %v: by quality, descending, then Z_PK", got, want)
		}
		if got, want := uuids(db.PersonPhotos("P-1")), []string{"A-1", "A-2", "A-3"}; !slices.Equal(got, want) {
			t.Errorf("PersonPhotos = %v, want %v: each photo once, by UUID", got, want)
		}
		for name, n := range map[string]int{
			"FaceInfoOf of a photo without a face":   len(db.FaceInfoOf(&photoInfo{UUID: "A-0"})),
			"PersonFaces of a person without a face": len(db.PersonFaces("P-4")), "PersonPhotos of a person without a face": len(db.PersonPhotos("P-4")),
			"PersonFaces of an unknown UUID": len(db.PersonFaces("P-0")), "PersonPhotos of an unknown UUID": len(db.PersonPhotos("P-0")),
		} {
			if n != 0 {
				t.Errorf("%s: %d, want 0", name, n)
			}
		}
		faces, personFaces, photos := db.FaceInfoOf(a1), db.PersonFaces("P-1"), db.PersonPhotos("P-1")
		faces[0].PK, personFaces[0].PK, photos[0] = 99, 99, nil
		if db.FaceInfoOf(a1)[0].PK == 99 || db.PersonFaces("P-1")[0].PK == 99 || db.PersonPhotos("P-1")[0] == nil {
			t.Error("FaceInfoOf, PersonFaces or PersonPhotos shares its slice")
		}
	})

	// Two rows of one ZPERSONUUID: the UUID stands for the row of Person, the
	// highest Z_PK.
	t.Run("repeated UUID", func(t *testing.T) {
		t.Parallel()
		db := personDB(t, tables{
			assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2")},
			persons: []sqlread.Person{
				{PK: 1, UUID: nullString("P-D"), FullName: nullString("First Example")},
				{PK: 2, UUID: nullString("P-D"), FullName: nullString("Second Example")},
			},
			faces: []sqlread.Face{face(1, 1, "A-1", nullString("First Example"), 0.5), face(2, 2, "A-2", nullString("Second Example"), 0.5)},
		})
		if got := uuids(db.PersonPhotos("P-D")); !slices.Equal(got, []string{"A-2"}) {
			t.Errorf("PersonPhotos = %v, want [A-2]: the photos of the row with the highest Z_PK", got)
		}
		if got := pks(db.PersonFaces("P-D")); !slices.Equal(got, []int64{2}) {
			t.Errorf("PersonFaces = %v, want [2]", got)
		}
	})

	// Go sorts up to 12 elements by insertion, which is stable: 13 faces show the Z_PK order of equal qualities.
	t.Run("equal qualities", func(t *testing.T) {
		t.Parallel()
		var faces []sqlread.Face
		for pk := int64(1); pk <= 13; pk++ {
			faces = append(faces, face(pk, 1, "A-1", sql.NullString{}, float64(pk%2)))
		}
		db := personDB(t, tables{
			assets:  []sqlread.Asset{baseAsset(1, "A-1")},
			persons: []sqlread.Person{{PK: 1, UUID: nullString("P-1")}},
			faces:   faces,
		})
		if got, want := pks(db.PersonFaces("P-1")), []int64{1, 3, 5, 7, 9, 11, 13, 2, 4, 6, 8, 10, 12}; !slices.Equal(got, want) {
			t.Errorf("PersonFaces = %v, want %v", got, want)
		}
		if got, want := pks(db.FaceInfoOf(db.byUUID["A-1"])), []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}; !slices.Equal(got, want) {
			t.Errorf("FaceInfoOf = %v, want %v", got, want)
		}
	})
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
				onPhoto := map[personinfo.PersonInfo]bool{}
				for _, p := range db.Photos() {
					for _, pi := range db.PersonInfoOf(p) {
						onPhoto[pi] = true
					}
				}
				var personFaces int
				for uuid, person := range db.personByUUID {
					faces := db.PersonFaces(uuid)
					personFaces += len(faces)
					if !slices.IsSortedFunc(faces, func(a, b personinfo.FaceInfo) int { return cmp.Compare(b.Quality, a.Quality) }) {
						t.Error("the faces of a person are not by quality, descending")
					}
					if photos := len(db.PersonPhotos(uuid)); (photos > 0) != onPhoto[person] {
						t.Errorf("a person has %d photos in PersonPhotos; is in PersonInfoOf of a photo: %v", photos, onPhoto[person])
					}
				}
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
					// Assets without attributes included; a repeated ZPERSONUUID is the row with the highest Z_PK, as Person.
					{"faces of persons", personFaces, "SELECT COUNT(*) FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn +
						" JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn +
						" WHERE p.Z_PK = (SELECT MAX(q.Z_PK) FROM ZPERSON q WHERE q.ZPERSONUUID IS p.ZPERSONUUID)"},
				} {
					if want := count(t, ro, c.query); c.got != want {
						t.Errorf("%s: %d, want %d", c.name, c.got, want)
					}
				}
				t.Logf("%d persons: %d named, %d with a key photo, %d favorites, %d featured less; %d faces on %d photos, %d faces of persons",
					len(db.PersonInfo()), named, keyed, favorite, featureLess, found, withFaces, personFaces)
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
