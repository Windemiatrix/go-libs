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
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
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

// Photo a has Dog twice, so that the counts are of records, not of photos
// (_photosdb_process_searchinfo.py:386, 398). Photo b has dog, a label of its
// own whose normalized string is that of Dog: each map counts on its own. The
// Beach of a is normalized to shore, so that the reader is seen to take
// normalized_string rather than lower(content). The foreign asset has Cat, so
// that the labels of an asset the library lacks are counted, as upstream counts
// them.
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
			"(1, 1500, 'Dog', 'dog'), (2, 1500, 'Beach', 'beach'), (3, 5, 'Townsville', 'townsville'), (4, 1500, 'Cat', 'cat'), "+
			"(5, 1500, 'dog', 'dog'), (6, 1500, 'Beach', 'shore')",
		"INSERT INTO ga (assetid, groupid) VALUES (1, 1), (1, 6), (1, 1), (1, 3), (2, 2), (2, 5), (3, 4)",
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
		{"normalized labels of a", sa.Normalized().Labels(), []string{"dog", "dog", "shore"}},
		{"labels of b", db.SearchInfoOf(b).Labels(), []string{"Beach", "dog"}},
		{"normalized labels of b", db.SearchInfoOf(b).Normalized().Labels(), []string{"beach", "dog"}},
		{"labels of a photo without terms", db.SearchInfoOf(c).Labels(), nil},
		{"labels of the search database", db.Labels(), []string{"Beach", "Cat", "Dog", "dog"}},
		{"normalized labels of the search database", db.LabelsNormalized(), []string{"beach", "cat", "dog", "shore"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if sa.City() != "Townsville" || db.SearchInfoOf(c).City() != "" {
		t.Errorf("City of a, c = %q, %q; want Townsville and none", sa.City(), db.SearchInfoOf(c).City())
	}
	if len(db.search) != 3 {
		t.Errorf("terms kept for %d assets, want 3: those of an asset the library lacks too", len(db.search))
	}
	// Upstream would give Beach 1 and the normalized dog 1: its shared try starts
	// both lists anew when either label is new (_photosdb_process_searchinfo.py:264-269).
	if got, want := db.LabelCounts(), map[string]int{"Beach": 2, "Cat": 1, "Dog": 2, "dog": 1}; !maps.Equal(got, want) {
		t.Errorf("LabelCounts = %v, want %v", got, want)
	}
	if got, want := db.LabelCountsNormalized(), map[string]int{"beach": 1, "cat": 1, "dog": 3, "shore": 1}; !maps.Equal(got, want) {
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
	psiLabels, psiNormalized := []string{"Beach", "Dog", "Dog"}, []string{"dog", "dog", "shore"}
	tests := []struct {
		name             string
		setup            []func(t *testing.T, lib string)
		want, normalized []string
	}{
		{"psi.sqlite", []func(*testing.T, string){psi}, psiLabels, psiNormalized},
		{"leo.sqlite without psi.sqlite", []func(*testing.T, string){leo}, []string{"Fox"}, []string{"fox"}},
		{"leo.sqlite when psi.sqlite is empty", []func(*testing.T, string){leo, emptyPSI}, []string{"Fox"}, []string{"fox"}},
		{"psi.sqlite before leo.sqlite", []func(*testing.T, string){leo, psi}, psiLabels, psiNormalized},
		{"no search database", nil, nil, nil},
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
			s := db.SearchInfoOf(&photoinfo.PhotoInfo{UUID: a})
			if got := s.Labels(); !slices.Equal(got, tt.want) {
				t.Errorf("labels = %q, want %q", got, tt.want)
			}
			if got := s.Normalized().Labels(); !slices.Equal(got, tt.normalized) {
				t.Errorf("normalized labels = %q, want %q", got, tt.normalized)
			}
		})
	}

	// Open looks for the search directory beside the database it reads, not in
	// Options.LibraryPath (_photosdb_process_searchinfo.py:154).
	t.Run("a database file with another library path", func(t *testing.T) {
		t.Parallel()
		lib := testlib.OpenPristine(t, "synthetic")
		psi(t, lib)
		db, err := Open(t.Context(), filepath.Join(lib, "database", "Photos.sqlite"), Options{TempDir: t.TempDir(), LibraryPath: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if got := db.SearchInfoOf(&photoinfo.PhotoInfo{UUID: a}).Labels(); !slices.Equal(got, psiLabels) {
			t.Errorf("labels = %q, want %q", got, psiLabels)
		}
	})
}

// A search database that cannot be read fails Open, as upstream fails, and so
// does one whose status cannot be read (searchSize); the messages hold no value
// of the database.
func TestSearchDatabaseErrors(t *testing.T) {
	t.Parallel()

	// A file inside gives the directory a size, so that it is taken as a database.
	directory := func(name string) func(t *testing.T, lib string) {
		return func(t *testing.T, lib string) {
			dir := filepath.Join(lib, "database", "search", name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	tests := []struct {
		name        string
		setup       func(t *testing.T, lib string)
		unsupported bool
		want        string
	}{
		{"psi.sqlite is a directory", directory("psi.sqlite"), false, "not a regular file"},
		{"leo.sqlite is a directory", directory("leo.sqlite"), false, "not a regular file"},
		{"the search directory cannot be read", func(t *testing.T, lib string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads a directory without permissions")
			}
			writeSearchDB(t, lib, "psi.sqlite", searchPSITables...)
			dir := filepath.Join(lib, "database", "search")
			if err := os.Chmod(dir, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // G302: a directory needs the search bit to be removed
		}, false, "permission denied"},
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
// all of them, as upstream keeps them. The messages hold counts only.
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
				switch lib.name {
				case "synthetic":
					if len(terms) != 0 {
						t.Errorf("%d terms, want none: the library has no search database", len(terms))
					}
				case "Test-26.1": // its psi.sqlite holds no terms
				default:
					if len(terms) == 0 {
						t.Error("no terms: the search database was not read")
					}
				}
				records, labelRecords := 0, 0
				for _, r := range db.search {
					records += len(r)
					labelRecords += len(searchinfo.New(r).Labels())
				}
				counted := 0
				for _, n := range db.LabelCounts() {
					counted += n
				}
				if records != len(terms) || counted != labelRecords {
					t.Errorf("%d records of %d terms; %d labels counted of %d", records, len(terms), counted, labelRecords)
				}
				t.Logf("%d terms of %d assets; %d labels, %d distinct", len(terms), len(db.search), labelRecords, len(db.Labels()))
				labels.Add(int64(len(db.Labels())))
			})
		}
	})
	if labels.Load() == 0 {
		t.Error("no label in any library: the test is vacuous")
	}
}
