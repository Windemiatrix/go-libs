package photosdb

import (
	"database/sql"
	"maps"
	"slices"
	"sync"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestPersons(t *testing.T) {
	t.Parallel()

	db := &DB{photos: []*photoInfo{
		{UUID: "A-1", Persons: []string{"Person A", "Person B", "Person B", "_UNKNOWN_"}},
		{UUID: "A-2", InTrash: true, Persons: []string{"Person A"}},
		{UUID: "A-3", Burst: true, BurstPickType: 2, Persons: []string{"Person C"}},
		{UUID: "A-4"},
	}}
	want := []string{"Person A", "Person B", "Person C", "_UNKNOWN_"}
	got := db.Persons()
	if !slices.Equal(got, want) {
		t.Errorf("Persons() has %d names, want %d", len(got), len(want))
	}
	got[0] = "changed"
	if again := db.Persons(); !slices.Equal(again, want) {
		t.Error("a change of the slice of Persons() reached the next call")
	}
	counts := db.PersonCounts()
	if want := map[string]int{"Person A": 2, "Person B": 2, "Person C": 1, "_UNKNOWN_": 1}; !maps.Equal(counts, want) {
		t.Errorf("PersonCounts() has %d names, want %d", len(counts), len(want))
	}
	counts["Person A"] = 99
	if db.PersonCounts()["Person A"] != 2 {
		t.Error("a change of the map of PersonCounts() reached the next call")
	}

	empty := &DB{photos: []*photoInfo{{UUID: "A-1"}}}
	if got := empty.Persons(); got != nil {
		t.Errorf("Persons() of a library without faces has %d names, want nil", len(got))
	}
	if got := empty.PersonCounts(); got == nil || len(got) != 0 {
		t.Errorf("PersonCounts() of a library without faces: nil %v, %d names; want an empty map", got == nil, len(got))
	}
}

// Expected: the faces of the assets that Open reads (those with a
// ZADDITIONALASSETATTRIBUTES row), by direct SQL.
func TestPersonsLibraries(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	faces, withPersons := 0, 0
	t.Run("libraries", func(t *testing.T) {
		for _, name := range testlib.Libraries(t) {
			lib := testlib.OpenPristine(t, name)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				want, named := personSQL(t, lib, schemaByID(t, db.Info().SchemaID))
				n := 0
				for _, c := range want {
					n += c
				}
				if got := db.PersonCounts(); !maps.Equal(got, want) {
					t.Errorf("PersonCounts() has %d names, want %d names and %d faces", len(got), len(want), n)
				}
				if got := db.Persons(); !slices.Equal(got, slices.Sorted(maps.Keys(want))) {
					t.Errorf("Persons() has %d names, want %d", len(got), len(want))
				}
				for _, p := range db.Photos() {
					if p.HasPersons != named[p.UUID] {
						t.Errorf("%s: HasPersons %v, want %v", p.UUID, p.HasPersons, named[p.UUID])
					}
				}
				t.Logf("%d names, %d faces, %d photos with a named person", len(want), n, len(named))
				mu.Lock()
				defer mu.Unlock()
				faces += n
				withPersons += len(named)
			})
		}
	})
	if faces == 0 {
		t.Error("no library has a face of a person: the test is vacuous")
	}
	if withPersons == 0 {
		t.Error("no library has a photo with a named person: the HasPersons check is vacuous")
	}
}

func personSQL(t *testing.T, lib string, s schema.Schema) (counts map[string]int, named map[string]bool) {
	t.Helper()
	rows, err := roDB(t, lib).QueryContext(t.Context(), "SELECT a.ZUUID, p.ZFULLNAME FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f."+s.FacePersonColumn+
		" JOIN ZASSET a ON a.Z_PK = f."+s.FaceAssetColumn+" JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK") //nolint:gosec // G202: the column names are constants of internal/schema
	if err != nil {
		t.Fatal(err)
	}
	counts, named = map[string]int{}, map[string]bool{}
	for rows.Next() {
		var uuid string
		var name sql.NullString
		if err := rows.Scan(&uuid, &name); err != nil {
			t.Fatal(err)
		}
		key := norm.NFC.String(name.String)
		if key == "" {
			key = "_UNKNOWN_"
		} else {
			named[uuid] = true
		}
		counts[key]++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	return counts, named
}
