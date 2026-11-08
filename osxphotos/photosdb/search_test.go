package photosdb

import (
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// An asset of psi.sqlite that the synthetic library does not have.
const searchForeignUUID = "00000000-0000-0000-0000-0000000000F1"

// The tables and columns that the readers of upstream name
// (_photosdb_process_searchinfo.py:201-218, 294-310).
var (
	searchPSITables = []string{
		"CREATE TABLE assets (uuid_0 INTEGER, uuid_1 INTEGER)",
		"CREATE TABLE groups (category INTEGER, owning_groupid INTEGER, content_string TEXT, normalized_string TEXT, lookup_identifier TEXT)",
		"CREATE TABLE ga (assetid INTEGER, groupid INTEGER)",
	}
	searchLeoTables = []string{
		"CREATE TABLE lexicon (lexeme_id INTEGER, type INTEGER, category INTEGER, content TEXT)",
		"CREATE TABLE items (identifier TEXT, lexeme_ids BLOB, type INTEGER)",
	}
)

func writeSearchDB(t *testing.T, lib, name string, stmts ...string) {
	t.Helper()
	path := filepath.Join(lib, "database", "search", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err := db.ExecContext(t.Context(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// uuid_0 and uuid_1 of psi.sqlite are the bytes of the UUID as two
// little-endian signed integers (_photosdb_process_searchinfo.py:405-415).
func psiHalves(t *testing.T, uuid string) (lo, hi int64) {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(uuid, "-", ""))
	if err != nil || len(b) != 16 {
		t.Fatal("not a UUID")
	}
	return int64(binary.LittleEndian.Uint64(b[:8])), int64(binary.LittleEndian.Uint64(b[8:]))
}

// Photo a has the label Dog twice, the label Beach and a city; photo b the
// label Beach; the foreign asset the label Cat.
func psiFixture(t *testing.T, a, b string) []string {
	t.Helper()
	var assets []string
	for i, u := range []string{a, b, searchForeignUUID} {
		lo, hi := psiHalves(t, u)
		assets = append(assets, fmt.Sprintf("(%d, %d, %d)", i+1, lo, hi))
	}
	return append(slices.Clone(searchPSITables),
		"INSERT INTO assets (rowid, uuid_0, uuid_1) VALUES "+strings.Join(assets, ", "),
		"INSERT INTO groups (rowid, category, content_string, normalized_string) VALUES "+
			"(1, 1500, 'Dog', 'dog'), (2, 1500, 'Beach', 'beach'), (3, 5, 'Townsville', 'townsville'), (4, 1500, 'Cat', 'cat')",
		"INSERT INTO ga (assetid, groupid) VALUES (1, 1), (1, 2), (1, 1), (1, 3), (2, 2), (3, 4)",
	)
}

// leo.sqlite keeps the identifier in lower case; the reader makes it upper case.
func leoFixture(uuid string) []string {
	return append(slices.Clone(searchLeoTables),
		"INSERT INTO lexicon (lexeme_id, type, category, content) VALUES (1, 1, 4000, 'Fox')",
		"INSERT INTO items (identifier, lexeme_ids, type) VALUES ('"+strings.ToLower(uuid)+"', X'01000000', 1)",
	)
}

func searchPhotos(t *testing.T) []*photoinfo.PhotoInfo {
	t.Helper()
	db, err := Open(t.Context(), testlib.OpenPristine(t, "synthetic"), Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// The readers give UUIDs in upper case (_photosdb_process_searchinfo.py:316, 405-415).
	var photos []*photoinfo.PhotoInfo
	for _, p := range db.Photos() {
		if p.UUID == strings.ToUpper(p.UUID) {
			photos = append(photos, p)
		}
	}
	if len(photos) < 3 {
		t.Fatalf("the synthetic library has %d photos with an upper-case UUID, want 3 or more: the case is vacuous", len(photos))
	}
	return photos
}

// The expectations are literals; the database is written into a copy of the
// synthetic library, which has none.
func TestSearchInfoOf(t *testing.T) {
	t.Parallel()

	photos := searchPhotos(t)
	a, b, c := photos[0], photos[1], photos[2]
	lib := testlib.OpenPristine(t, "synthetic")
	writeSearchDB(t, lib, "psi.sqlite", psiFixture(t, a.UUID, b.UUID)...)
	tmp := t.TempDir()
	before := snapshotTree(t, lib)
	db, err := Open(t.Context(), lib, Options{TempDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	assertTree(t, "psi.sqlite", before, snapshotTree(t, lib))
	assertNoCopies(t, tmp)

	sa := db.SearchInfoOf(a)
	for _, tc := range []struct {
		name      string
		got, want []string
	}{
		{"labels of a", sa.Labels(), []string{"Beach", "Dog", "Dog"}},
		{"normalized labels of a", sa.Normalized().Labels(), []string{"beach", "dog", "dog"}},
		{"labels of b", db.SearchInfoOf(b).Labels(), []string{"Beach"}},
		{"labels of a photo without terms", db.SearchInfoOf(c).Labels(), nil},
		// The foreign asset is no photo of the library: its label Cat is left out.
		{"labels of the library", db.Labels(), []string{"Beach", "Dog"}},
		{"normalized labels of the library", db.LabelsNormalized(), []string{"beach", "dog"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if sa.City() != "Townsville" || db.SearchInfoOf(c).City() != "" {
		t.Errorf("City of a, c = %q, %q; want Townsville and none", sa.City(), db.SearchInfoOf(c).City())
	}
	if len(db.search) != 2 {
		t.Errorf("terms kept for %d photos, want 2: those of an asset the library lacks are not kept", len(db.search))
	}
	// labels_as_dict counts records, the label Dog of a twice (_photosdb_process_searchinfo.py:261-269, 386).
	if got, want := db.LabelCounts(), map[string]int{"Beach": 2, "Dog": 2}; !maps.Equal(got, want) {
		t.Errorf("LabelCounts = %v, want %v", got, want)
	}
	if got, want := db.LabelCountsNormalized(), map[string]int{"beach": 2, "dog": 2}; !maps.Equal(got, want) {
		t.Errorf("LabelCountsNormalized = %v, want %v", got, want)
	}

	none, err := Open(t.Context(), testlib.OpenPristine(t, "synthetic"), Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(none.search) != 0 || none.Labels() != nil || len(none.LabelCounts()) != 0 || none.SearchInfoOf(a).All() != nil {
		t.Errorf("a library without a search database: terms of %d photos, %d labels", len(none.search), len(none.Labels()))
	}
}

// _photosdb_process_searchinfo.py:154-181: psi.sqlite when it is not empty,
// else leo.sqlite, else nothing.
func TestSearchDatabases(t *testing.T) {
	t.Parallel()

	photos := searchPhotos(t)
	a, b := photos[0].UUID, photos[1].UUID
	psi := func(t *testing.T, lib string) { writeSearchDB(t, lib, "psi.sqlite", psiFixture(t, a, b)...) }
	leo := func(t *testing.T, lib string) { writeSearchDB(t, lib, "leo.sqlite", leoFixture(a)...) }
	emptyPSI := func(t *testing.T, lib string) {
		if err := os.WriteFile(filepath.Join(lib, "database", "search", "psi.sqlite"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		setup []func(t *testing.T, lib string)
		want  []string
	}{
		{"psi.sqlite", []func(*testing.T, string){psi}, []string{"Beach", "Dog", "Dog"}},
		{"leo.sqlite without psi.sqlite", []func(*testing.T, string){leo}, []string{"Fox"}},
		{"leo.sqlite when psi.sqlite is empty", []func(*testing.T, string){leo, emptyPSI}, []string{"Fox"}},
		{"psi.sqlite before leo.sqlite", []func(*testing.T, string){leo, psi}, []string{"Beach", "Dog", "Dog"}},
		{"no search database", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			for _, setup := range tt.setup {
				setup(t, lib)
			}
			tmp := t.TempDir()
			db, err := Open(t.Context(), lib, Options{TempDir: tmp})
			if err != nil {
				t.Fatal(err)
			}
			assertNoCopies(t, tmp)
			p := &photoinfo.PhotoInfo{UUID: a}
			if got := db.SearchInfoOf(p).Labels(); !slices.Equal(got, tt.want) {
				t.Errorf("labels = %q, want %q", got, tt.want)
			}
			if want := strings.ToLower(strings.Join(tt.want, ",")); strings.Join(db.SearchInfoOf(p).Normalized().Labels(), ",") != want {
				t.Errorf("normalized labels = %q, want %q", db.SearchInfoOf(p).Normalized().Labels(), want)
			}
		})
	}
}

// A search database that cannot be read fails Open, as upstream fails; the
// messages hold no value of the database.
func TestSearchDatabaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T, lib string)
		unsupported bool
		want        string
	}{
		{"psi.sqlite is a directory", func(t *testing.T, lib string) {
			dir := filepath.Join(lib, "database", "search", "psi.sqlite")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, false, "not a regular file"},
		{"psi.sqlite is not a database", func(t *testing.T, lib string) {
			writeSearchDB(t, lib, "leo.sqlite", searchLeoTables...)
			if err := os.WriteFile(filepath.Join(lib, "database", "search", "psi.sqlite"), []byte(strings.Repeat("SECRET-CONTENT\n", 300)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, false, "not a database"},
		{"psi.sqlite without ga", func(t *testing.T, lib string) { writeSearchDB(t, lib, "psi.sqlite", searchPSITables[:2]...) }, true, "table ga"},
		{"leo.sqlite without items", func(t *testing.T, lib string) { writeSearchDB(t, lib, "leo.sqlite", searchLeoTables[:1]...) }, true, "table items"},
		{"a value of another type", func(t *testing.T, lib string) {
			writeSearchDB(t, lib, "psi.sqlite", append(slices.Clone(searchPSITables),
				"INSERT INTO assets (rowid, uuid_0, uuid_1) VALUES (1, 'SECRET-U', 1)",
				"INSERT INTO groups (rowid, category, content_string, normalized_string) VALUES (1, 1500, 'SECRET-L', 'secret-l')",
				"INSERT INTO ga (assetid, groupid) VALUES (1, 1)")...)
		}, false, "unexpected value type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			tt.setup(t, lib)
			tmp := t.TempDir()
			db, err := Open(t.Context(), lib, Options{TempDir: tmp})
			if db != nil || err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "SECRET") {
				t.Errorf("Open = %v, %v; want nil and an error with %q, without a value", db, err, tt.want)
			}
			if errors.Is(err, ErrUnsupportedSchema) != tt.unsupported {
				t.Errorf("errors.Is(%v, ErrUnsupportedSchema) = %v, want %v", err, !tt.unsupported, tt.unsupported)
			}
			assertNoCopies(t, tmp)
		})
	}
}

// Not vacuous: the search databases of upstream hold labels (its
// tests/test_*.py check them). The oracle of the terms kept is the reader:
// those of the photos of the library. The messages hold counts only.
func TestSearchLibraries(t *testing.T) {
	t.Parallel()

	var labels atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				terms, err := openSearch(t.Context(), filepath.Join(lib.path, "database", "Photos.sqlite"), t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				ofPhotos := 0
				for _, term := range terms {
					if _, ok := db.Photo(term.UUID); ok {
						ofPhotos++
					}
				}
				records, photoLabels := 0, 0
				for _, p := range db.Photos() {
					records += len(db.search[p.UUID])
					photoLabels += len(db.SearchInfoOf(p).Labels())
				}
				counted := 0
				for _, n := range db.LabelCounts() {
					counted += n
				}
				if records != ofPhotos || counted != photoLabels || len(db.Labels()) != len(db.LabelCounts()) {
					t.Errorf("%d records of %d terms of photos; %d labels counted of %d; %d labels, %d counts",
						records, ofPhotos, counted, photoLabels, len(db.Labels()), len(db.LabelCounts()))
				}
				if lib.name == "synthetic" && len(terms) != 0 {
					t.Errorf("synthetic: %d terms, want none: it has no search database", len(terms))
				}
				t.Logf("%d terms, %d of assets the library lacks; %d photos with terms; %d labels of photos, %d distinct",
					len(terms), len(terms)-ofPhotos, len(db.search), photoLabels, len(db.Labels()))
				labels.Add(int64(len(db.Labels())))
			})
		}
	})
	if labels.Load() == 0 {
		t.Error("no label in any library: the test is vacuous")
	}
}
