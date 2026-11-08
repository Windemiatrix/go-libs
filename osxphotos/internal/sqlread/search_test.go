package sqlread

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

const (
	psiUUIDA = "00112233-4455-6677-8899-AABBCCDDEEFF"
	psiUUIDB = "01020304-0506-0708-090A-0B0C0D0E0F10"
	leoUUID  = "ABCDEF01-2345-6789-ABCD-EF0123456789"
)

// The tables and columns of psi.sqlite that the query of upstream names
// (_photosdb_process_searchinfo.py:201-218).
var psiSchema = []string{
	"CREATE TABLE assets (uuid_0 INTEGER, uuid_1 INTEGER)",
	"CREATE TABLE groups (category INTEGER, owning_groupid INTEGER, content_string TEXT, normalized_string TEXT, lookup_identifier TEXT)",
	"CREATE TABLE ga (rowid INTEGER, assetid INTEGER, groupid INTEGER)",
}

// The halves are the little-endian signed integers of the bytes 00 11 .. EE FF
// and 01 02 .. 10, whose UUIDs are psiUUIDA and psiUUIDB.
var psiAssets = fmt.Sprintf("INSERT INTO assets (rowid, uuid_0, uuid_1) VALUES (1, %d, %d), (2, %d, %d)",
	int64(0x7766554433221100), int64(-0x11223344556678), int64(0x0807060504030201), int64(0x100F0E0D0C0B0A09))

// Groups 1 and 4 end in NUL, 2 is decomposed, 3 and 8 hold NULL strings, 5 has
// a NULL category, 4 and 9 are placeholders (9 with the final newline that the
// $ of Python allows) and 7 is a near miss of the placeholder.
const psiGroups = "INSERT INTO groups (rowid, category, content_string, normalized_string) VALUES " +
	"(1, 1500, 'Beach' || char(0), 'beach' || char(0)), " +
	"(2, 1500, 'Caf' || char(101, 769), 'caf' || char(101, 769)), " +
	"(3, 5, 'Paris', NULL), " +
	"(4, 1500, 'PGPlaceParkSearchableText' || char(0), 'pgplaceparksearchabletext' || char(0)), " +
	"(5, NULL, 'Orphan', 'orphan'), " +
	"(6, 1103, 'Holiday', 'holiday'), " +
	"(7, 1201, 'PGSearchableText', 'pgsearchabletext'), " +
	"(8, 2, NULL, NULL), " +
	"(9, 1500, 'PGParkSearchableText' || char(10), 'pgparksearchabletext' || char(10))"

// rowid is declared and filled against the insertion order, so only ORDER BY ga.rowid (_photosdb_process_searchinfo.py:201-218) gives this order.
// Two assets share the group 6.
const psiGA = "INSERT INTO ga (rowid, assetid, groupid) VALUES " +
	"(10, 2, 9), (9, 1, 8), (8, 1, 7), (7, 2, 6), (6, 1, 6), (5, 1, 5), (4, 1, 4), (3, 2, 3), (2, 2, 2), (1, 1, 1)"

func dropSQL(name string) string {
	if table, col, ok := strings.Cut(name, "."); ok {
		return "ALTER TABLE " + table + " DROP COLUMN " + col
	}
	return "DROP TABLE " + name
}

func psiDB(t *testing.T, stmts ...string) *DB {
	t.Helper()
	return openDB(t, makeDB(t, append(slices.Clone(psiSchema), stmts...)...))
}

func TestUUIDOf(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		uuid0, uuid1 int64
		want         string
	}{
		{0x7766554433221100, -0x11223344556678, psiUUIDA},
		{0x0807060504030201, 0x100F0E0D0C0B0A09, psiUUIDB},
		{0, 0, "00000000-0000-0000-0000-000000000000"},
		{-1, -1, "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF"},
	} {
		if got := uuidOf(tt.uuid0, tt.uuid1); got != tt.want {
			t.Errorf("uuidOf(%d, %d) = %s, want %s", tt.uuid0, tt.uuid1, got, tt.want)
		}
	}
}

func TestSearchTerms(t *testing.T) {
	t.Parallel()

	t.Run("terms", func(t *testing.T) {
		t.Parallel()
		got, err := psiDB(t, psiAssets, psiGroups, psiGA).SearchTerms(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		want := []SearchTerm{
			{1, psiUUIDA, 1500, "Beach", "beach"},
			{2, psiUUIDB, 1500, "Caf\U000000e9", "caf\U000000e9"},
			{3, psiUUIDB, 5, "Paris", ""},
			{6, psiUUIDA, 1103, "Holiday", "holiday"},
			{7, psiUUIDB, 1103, "Holiday", "holiday"},
			{8, psiUUIDA, 1201, "PGSearchableText", "pgsearchabletext"},
			{9, psiUUIDA, 2, "", ""},
		}
		if !slices.Equal(got, want) {
			t.Errorf("SearchTerms = %+v\nwant %+v", got, want)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		t.Parallel()
		got, err := psiDB(t).SearchTerms(t.Context())
		if len(got) != 0 || err != nil {
			t.Errorf("SearchTerms = %v, %v; want none, nil", got, err)
		}
	})

	t.Run("missing table or column", func(t *testing.T) {
		t.Parallel()
		for _, missing := range []string{"ga", "groups", "assets", "ga.groupid", "ga.assetid", "groups.category",
			"groups.content_string", "groups.normalized_string", "assets.uuid_0", "assets.uuid_1"} {
			_, err := psiDB(t, dropSQL(missing)).SearchTerms(t.Context())
			if !errors.Is(err, ErrMissingColumn) {
				t.Errorf("without %s: error %v, want ErrMissingColumn", missing, err)
			}
		}
	})

	t.Run("value type", func(t *testing.T) {
		t.Parallel()
		_, err := psiDB(t,
			"INSERT INTO assets (rowid, uuid_0, uuid_1) VALUES (1, NULL, 1)",
			"INSERT INTO groups (rowid, category, content_string, normalized_string) VALUES (1, 1, 'SECRET', 'secret')",
			"INSERT INTO ga (rowid, assetid, groupid) VALUES (1, 1, 1)",
		).SearchTerms(t.Context())
		if err == nil || !strings.Contains(err.Error(), "row 1: unexpected value type") || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("error %v, want row 1: unexpected value type without the value", err)
		}
	})
}

// A lexeme of each case: unmapped (2), without content (4 of type 2, 7 NULL),
// a placeholder ending in NUL (5), one whose lower case composes only under NFC
// (8), without category (9), without id (the blob holds 0) and absent from the
// lexicon (99).
const leoLexicon = "INSERT INTO lexicon (lexeme_id, type, category, content) VALUES " +
	"(1, 1, 4000, 'Beach' || char(0)), " +
	"(2, 1, 2030, 'Unmapped'), " +
	"(3, 1, 1070, 'Monday'), " +
	"(4, 2, 4000, 'Typed'), " +
	"(5, 1, 4000, 'PGMeaningDinnerSearchableText' || char(0)), " +
	"(6, 1, 2090, 'CAF' || char(69, 769)), " +
	"(7, 1, 4000, NULL), " +
	"(8, 1, 4000, 'J' || char(780)), " +
	"(9, 1, NULL, 'NoCategory'), " +
	"(NULL, 1, 4000, 'NoID')"

// The lexeme ids 1, 3, 2, 4, 5, 6, 7, 8, 9, 0 and 99, little-endian, and a tail of two bytes.
const leoBlob = "X'" + "01000000" + "03000000" + "02000000" + "04000000" + "05000000" +
	"06000000" + "07000000" + "08000000" + "09000000" + "00000000" + "63000000" + "FFFF" + "'"

// rowid is declared and filled against the insertion order, so only ORDER BY rowid (_photosdb_process_searchinfo.py:305-310) gives this order.
// The items of type 2, without an identifier and with an empty one give no term.
const leoItems = "INSERT INTO items (rowid, identifier, lexeme_ids, type) VALUES " +
	"(5, 'second-id', X'01000000', 1), (4, NULL, X'01000000', 1), (3, '', X'01000000', 1), (2, 'other', X'01000000', 2), " +
	"(1, 'abcdef01-2345-6789-abcd-ef0123456789', " + leoBlob + ", 1)"

var leoSchema = []string{
	"CREATE TABLE lexicon (lexeme_id INTEGER, type INTEGER, category INTEGER, content TEXT)",
	"CREATE TABLE items (rowid INTEGER, identifier TEXT, lexeme_ids BLOB, type INTEGER)",
}

func leoCategoryValues() map[int]bool {
	values := map[int]bool{}
	for _, v := range leoCategories {
		values[v] = true
	}
	return values
}

// LEO_CATEGORY_TO_PHOTOS8 has 73 entries, each mapped to a category of its own
// (_photosdb_process_searchinfo.py:45-119).
func TestLeoCategories(t *testing.T) {
	t.Parallel()

	if n, distinct := len(leoCategories), len(leoCategoryValues()); n != 73 || distinct != n {
		t.Errorf("leoCategories: %d entries, %d distinct values; want 73 of each", n, distinct)
	}
}

func leoDB(t *testing.T, stmts ...string) *DB {
	t.Helper()
	return openDB(t, makeDB(t, append(slices.Clone(leoSchema), stmts...)...))
}

func TestLeoTerms(t *testing.T) {
	t.Parallel()

	t.Run("terms", func(t *testing.T) {
		t.Parallel()
		got, err := leoDB(t, leoLexicon, leoItems).LeoTerms(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		want := []SearchTerm{
			{1, leoUUID, 1500, "Beach", "beach"},
			{2, leoUUID, 1070, "Monday", "monday"},
			{3, leoUUID, 5, "CAF\U000000c9", "caf\U000000e9"},
			{4, leoUUID, 1500, "J\U0000030C", "\U000001F0"},
			{5, "SECOND-ID", 1500, "Beach", "beach"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("LeoTerms = %+v\nwant %+v", got, want)
		}
	})

	// Upstream keeps in db_searchinfo_uuid only the last item of an identifier
	// that has records (_photosdb_process_searchinfo.py:353-354); whether the
	// reader must too waits for the count of repeats in the test libraries.
	t.Run("repeated identifier", func(t *testing.T) {
		t.Parallel()
		got, err := leoDB(t, leoLexicon, "INSERT INTO items (rowid, identifier, lexeme_ids, type) VALUES "+
			"(2, 'abc', X'03000000', 1), (1, 'ABC', X'01000000', 1)").LeoTerms(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		want := []SearchTerm{
			{1, "ABC", 1500, "Beach", "beach"},
			{2, "ABC", 1070, "Monday", "monday"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("LeoTerms = %+v\nwant %+v", got, want)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		t.Parallel()
		got, err := leoDB(t).LeoTerms(t.Context())
		if len(got) != 0 || err != nil {
			t.Errorf("LeoTerms = %v, %v; want none, nil", got, err)
		}
	})

	t.Run("missing table or column", func(t *testing.T) {
		t.Parallel()
		for _, missing := range []string{"lexicon", "items", "lexicon.lexeme_id", "lexicon.type", "lexicon.category",
			"lexicon.content", "items.identifier", "items.lexeme_ids", "items.type"} {
			_, err := leoDB(t, dropSQL(missing)).LeoTerms(t.Context())
			if !errors.Is(err, ErrMissingColumn) {
				t.Errorf("without %s: error %v, want ErrMissingColumn", missing, err)
			}
		}
	})

	// A scan of items cannot fail (any value converts to text or bytes), so its error is a failed query.
	t.Run("items query", func(t *testing.T) {
		t.Parallel()
		_, err := openDB(t, makeDB(t, leoSchema[0],
			"CREATE TABLE items (identifier TEXT PRIMARY KEY, lexeme_ids BLOB, type INTEGER) WITHOUT ROWID")).LeoTerms(t.Context())
		if err == nil || !strings.Contains(err.Error(), "leo items") {
			t.Errorf("error %v, want a failed query of leo items", err)
		}
	})

	// Upstream ignores a NULL lexeme_id, so only the text of row 2 fails.
	t.Run("value type", func(t *testing.T) {
		t.Parallel()
		_, err := leoDB(t, "INSERT INTO lexicon (lexeme_id, type, category, content) VALUES (NULL, 1, 4000, 'x'), ('SECRET', 1, 4000, 'x')").LeoTerms(t.Context())
		if err == nil || !strings.Contains(err.Error(), "row 2: unexpected value type") || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("error %v, want row 2: unexpected value type without the value", err)
		}
	})
}

func TestLexemeIDs(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		blob []byte
		want []uint32
	}{
		{"nil", nil, nil},
		{"little-endian", []byte{1, 0, 0, 0, 0, 1, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF}, []uint32{1, 256, math.MaxUint32}},
		{"tail", []byte{1, 0, 0, 0, 9, 9}, []uint32{1}},
		{"short", []byte{1, 2, 3}, nil},
	} {
		if got := lexemeIDs(tt.blob); !slices.Equal(got, tt.want) {
			t.Errorf("%s: lexemeIDs = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func hasFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The oracle is the database: the kept rows are those with a category and
// without a placeholder key. The GLOB is wider than the pattern of the reader,
// which takes only letters and digits between the two words.
func checkPSI(t *testing.T, path string) int {
	t.Helper()
	terms, err := openDB(t, path).SearchTerms(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ro := openRO(t, path)
	const kept = " FROM ga JOIN groups ON groups.rowid = ga.groupid JOIN assets ON ga.assetid = assets.rowid" +
		" WHERE groups.category IS NOT NULL AND COALESCE(groups.content_string, '') NOT GLOB 'PG*SearchableText'"
	assets, labels := map[string]bool{}, int64(0)
	for i, term := range terms {
		assets[term.UUID] = true
		if term.Category == 1500 {
			labels++
		}
		if i > 0 && terms[i-1].RowID >= term.RowID {
			t.Errorf("SearchTerms: term %d out of ga.rowid order", i+1)
		}
	}
	for _, c := range []struct {
		name, query string
		got         int64
	}{
		{"terms", "SELECT COUNT(*)" + kept, int64(len(terms))},
		{"assets", "SELECT COUNT(DISTINCT ga.assetid)" + kept, int64(len(assets))},
		{"labels", "SELECT COUNT(*)" + kept + " AND groups.category = 1500", labels},
	} {
		if want := count(t, ro, c.query); c.got != want {
			t.Errorf("SearchTerms: %d %s, want %d", c.got, c.name, want)
		}
	}
	t.Logf("SearchTerms: %d terms, %d assets, %d labels", len(terms), len(assets), labels)
	return len(terms)
}

func TestSearchTermsLibraries(t *testing.T) {
	t.Parallel()

	var libraries, terms atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, name := range testlib.Libraries(t) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				dir := filepath.Join(testlib.OpenPristine(t, name), "database", "search")
				psi, leo := filepath.Join(dir, "psi.sqlite"), filepath.Join(dir, "leo.sqlite")
				switch {
				case hasFile(psi):
					libraries.Add(1)
					terms.Add(int64(checkPSI(t, psi)))
				case hasFile(leo):
					libraries.Add(1)
					got, err := openDB(t, leo).LeoTerms(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					if len(got) == 0 {
						t.Error("LeoTerms: no term")
					}
					values := leoCategoryValues()
					for _, term := range got {
						if !values[term.Category] {
							t.Errorf("LeoTerms: category %d is no value of leoCategories", term.Category)
							break
						}
					}
					// Upstream keeps the terms of only the last item of an identifier
					// (_photosdb_process_searchinfo.py:353-354); the count tells whether that matters.
					repeated := count(t, openRO(t, leo), "SELECT COUNT(*) - COUNT(DISTINCT upper(identifier)) FROM items "+
						"WHERE type = 1 AND identifier IS NOT NULL AND identifier <> ''")
					t.Logf("LeoTerms: %d terms, %d items repeat an identifier", len(got), repeated)
					terms.Add(int64(len(got)))
				default:
					t.Log("no search database")
				}
			})
		}
	})
	if libraries.Load() == 0 || terms.Load() == 0 {
		t.Errorf("%d libraries with a search database, %d terms in all of them; want some of both: the test is vacuous", libraries.Load(), terms.Load())
	}
}
