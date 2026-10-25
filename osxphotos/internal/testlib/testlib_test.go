package testlib

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type libCase struct {
	assetMarkers           []string
	albumMarkers           []string
	assetsLosingAttributes int
	checks                 []check
}

type check struct {
	query string
	want  string
}

func pk(marker string) string {
	return "(SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:" + marker + "')"
}

var (
	assetCloudGUIDMarkers = []string{"asset-cloud-guid-1", "asset-cloud-guid-dup-a", "asset-cloud-guid-dup-b"}
	albumCloudGUIDMarkers = []string{"album-cloud-guid", "folder-cloud-guid"}
	fingerprintMarkers    = []string{"fingerprint-value", "fingerprint-null"}
	burstMarkers          = []string{"burst-key", "burst-selected", "burst-none", "burst-unselected", "burst-default", "burst-null"}
	resourcesMarkers      = []string{"resources-original", "resources-missing"}
	rawMarkers            = []string{"resources-raw"} // only libraries that have a RAW resource
)

func assetCloudGUIDChecks() []check {
	return []check{
		{"SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = " + pk("asset-cloud-guid-1"), "TEST-GUID-0001"},
		{"SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = " + pk("asset-cloud-guid-dup-a"), "TEST-GUID-0002"},
		{"SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = " + pk("asset-cloud-guid-dup-b"), "TEST-GUID-0002"},
		// The library has no iCloud: every other asset keeps a NULL GUID.
		{"SELECT COUNT(*) FROM ZASSET WHERE ZCLOUDASSETGUID IS NOT NULL", "3"},
	}
}

func albumCloudGUIDChecks() []check {
	return []check{
		{"SELECT ZCLOUDGUID FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZTITLE = 'mig:album-cloud-guid'", "TEST-ALBUM-GUID-0001"},
		{"SELECT ZCLOUDGUID FROM ZGENERICALBUM WHERE ZKIND = 4000 AND ZTITLE = 'mig:folder-cloud-guid'", "TEST-FOLDER-GUID-0001"},
	}
}

func fingerprintChecks(column string) []check {
	return []check{
		{"SELECT " + column + " FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:fingerprint-value'", "TEST-FP-0001"},
		{"SELECT " + column + " FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:fingerprint-null'", "NULL"},
	}
}

func burstChecks(table, albumColumn string) []check {
	pick := func(marker string) string {
		return "SELECT ZAVALANCHEPICKTYPE FROM ZASSET WHERE Z_PK = " + pk(marker)
	}
	nonKey := "SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE IN " +
		"('mig:burst-selected', 'mig:burst-none', 'mig:burst-unselected', 'mig:burst-default', 'mig:burst-null')"
	return []check{
		{"SELECT COUNT(*) FROM ZASSET WHERE ZAVALANCHEUUID = 'TEST-BURST-0001'", "6"},
		{"SELECT COUNT(*) FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK " +
			"WHERE x.ZTITLE LIKE 'mig:burst-%' AND a.ZAVALANCHEUUID = 'TEST-BURST-0001' AND a.ZKIND = 0", "6"},
		{pick("burst-key"), "16"},
		{pick("burst-selected"), "8"},
		{pick("burst-none"), "0"},
		{pick("burst-unselected"), "2"},
		{pick("burst-default"), "4"},
		{pick("burst-null"), "NULL"},
		{"SELECT COUNT(*) > 0 FROM " + table + " m JOIN ZGENERICALBUM g ON g.Z_PK = m." + albumColumn +
			" WHERE m.Z_3ASSETS = " + pk("burst-key") + " AND g.ZKIND = 2 AND g.ZTRASHEDSTATE = 0", "1"},
		{"SELECT COUNT(*) FROM " + table + " WHERE Z_3ASSETS IN (" + nonKey + ")", "0"},
	}
}

func resourcesChecks() []check {
	return []check{
		{"SELECT ZKIND FROM ZASSET WHERE Z_PK = " + pk("resources-original"), "0"},
		{"SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZASSET = " + pk("resources-original") +
			" AND (ZLOCALAVAILABILITY IS NOT 1 OR ZREMOTEAVAILABILITY IS NOT 1)", "0"},
		{"SELECT COUNT(*) > 0 FROM ZINTERNALRESOURCE WHERE ZASSET = " + pk("resources-original") +
			" AND ZDATASTORESUBTYPE = 1 AND ZDATALENGTH = 1000001", "1"},
		{"SELECT ZKIND FROM ZASSET WHERE Z_PK = " + pk("resources-missing"), "0"},
		{"SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZASSET = " + pk("resources-missing") +
			" AND (ZLOCALAVAILABILITY IS NOT 0 OR ZREMOTEAVAILABILITY IS NOT 1)", "0"},
		{"SELECT COUNT(*) > 0 FROM ZINTERNALRESOURCE WHERE ZASSET = " + pk("resources-missing") +
			" AND ZDATASTORESUBTYPE = 1", "1"},
	}
}

func rawChecks() []check {
	return []check{
		{"SELECT COUNT(*) > 0 FROM ZINTERNALRESOURCE WHERE ZASSET = " + pk("resources-raw") +
			" AND ZDATASTORESUBTYPE = 17 AND ZDATALENGTH = 2000002", "1"},
	}
}

var expected = map[string]libCase{
	"Test-13.0.0": {
		assetMarkers: slices.Concat(assetCloudGUIDMarkers, fingerprintMarkers, burstMarkers),
		albumMarkers: albumCloudGUIDMarkers,
		checks: slices.Concat(assetCloudGUIDChecks(), albumCloudGUIDChecks(),
			fingerprintChecks("ZMASTERFINGERPRINT"), burstChecks("Z_28ASSETS", "Z_28ALBUMS")),
	},
	"Test-Cloud-13.1": {
		assetMarkers: []string{"asset-cloud-guid-null", "asset-cloud-guid-empty", "cloud-master-local", "cloud-master-none"},
		albumMarkers: []string{"album-cloud-guid-null"},
		checks: []check{
			{"SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = " + pk("asset-cloud-guid-null"), "NULL"},
			{"SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = " + pk("asset-cloud-guid-empty"), ""},
			{"SELECT ZCLOUDLOCALSTATE || '|' || ZCLOUDMASTERGUID FROM ZCLOUDMASTER WHERE Z_PK = " +
				"(SELECT ZMASTER FROM ZASSET WHERE Z_PK = " + pk("cloud-master-local") + ")", "1|TEST-MASTER-GUID-0001"},
			{"SELECT ZMASTER FROM ZASSET WHERE Z_PK = " + pk("cloud-master-none"), "NULL"},
			{"SELECT ZCLOUDGUID FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZTITLE = 'mig:album-cloud-guid-null'", "NULL"},
		},
	},
	"Test-14.0.0": {
		assetMarkers: slices.Concat(resourcesMarkers, rawMarkers, []string{"trash"}),
		checks: slices.Concat(resourcesChecks(), rawChecks(), []check{
			{"SELECT ZTRASHEDSTATE || '|' || ZTRASHEDDATE FROM ZASSET WHERE Z_PK = " + pk("trash"), "1|694224000"},
		}),
	},
	"Test-14.6.0": {
		assetMarkers: []string{"shared-library-member", "shared-library-null", "tzoffset-null",
			"original-filename-null", "original-filename-empty", "date-created-null"},
		checks: []check{
			{"SELECT ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE || '|' || ZLIBRARYSCOPESHARESTATE || '|' || ZLIBRARYSCOPE " +
				"FROM ZASSET WHERE Z_PK = " + pk("shared-library-member"), "1|2|7"},
			{"SELECT COALESCE(ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE, ZLIBRARYSCOPESHARESTATE, ZLIBRARYSCOPE) " +
				"FROM ZASSET WHERE Z_PK = " + pk("shared-library-null"), "NULL"},
			{"SELECT ZTIMEZONEOFFSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:tzoffset-null'", "NULL"},
			{"SELECT ZORIGINALFILENAME FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:original-filename-null'", "NULL"},
			{"SELECT ZORIGINALFILENAME FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:original-filename-empty'", ""},
			{"SELECT ZDATECREATED FROM ZASSET WHERE Z_PK = " + pk("date-created-null"), "NULL"},
		},
	},
	"Test-15.4.1": {
		assetMarkers: []string{"syndicated-not-saved", "syndicated-saved", "shared-album-photo"},
		checks: []check{
			{"SELECT ZSYNDICATIONIDENTIFIER || '|' || ZSYNDICATIONHISTORY FROM ZADDITIONALASSETATTRIBUTES " +
				"WHERE ZTITLE = 'mig:syndicated-not-saved'", "TEST-SYND-0001|0"},
			{"SELECT ZSYNDICATIONIDENTIFIER FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:syndicated-saved'", "TEST-SYND-0002"},
			{"SELECT ZSYNDICATIONHISTORY FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:syndicated-saved'", "NULL"},
			{"SELECT ZCLOUDBATCHPUBLISHDATE FROM ZASSET WHERE Z_PK = " + pk("shared-album-photo"), "694224000"},
		},
	},
	"Test-16.0.0-beta": {
		assetMarkers:           []string{"date-modified-raw-value", "date-modified-raw-null"},
		assetsLosingAttributes: 1,
		checks: []check{
			{"SELECT ZMODIFICATIONDATE || '|' || ZADJUSTMENTTIMESTAMP FROM ZASSET WHERE Z_PK = " +
				pk("date-modified-raw-value"), "694224000|694310400"},
			{"SELECT ZMODIFICATIONDATE FROM ZASSET WHERE Z_PK = " + pk("date-modified-raw-null"), "NULL"},
			// The skipped asset is marked by its file name and has no additional attributes row.
			{"SELECT COUNT(*) FROM ZASSET a WHERE a.ZFILENAME = 'mig-skipped-asset.jpg' AND NOT EXISTS " +
				"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)", "1"},
		},
	},
	"Test-26.1": {
		assetMarkers: slices.Concat(assetCloudGUIDMarkers, fingerprintMarkers, burstMarkers, resourcesMarkers),
		albumMarkers: albumCloudGUIDMarkers,
		checks: slices.Concat(assetCloudGUIDChecks(), albumCloudGUIDChecks(),
			fingerprintChecks("ZORIGINALSTABLEHASH"), burstChecks("Z_33ASSETS", "Z_33ALBUMS"), resourcesChecks()),
	},
	"Test-27.0_DevBeta": {
		assetMarkers: fingerprintMarkers,
		checks:       fingerprintChecks("ZORIGINALSTABLEHASH"),
	},
}

const (
	assetMarkersQuery      = "SELECT substr(ZTITLE, 5) FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE LIKE 'mig:%' ORDER BY 1"
	albumMarkersQuery      = "SELECT substr(ZTITLE, 5) FROM ZGENERICALBUM WHERE ZTITLE LIKE 'mig:%' ORDER BY 1"
	assetCountQuery        = "SELECT COUNT(*) FROM ZASSET"
	withoutAttributesQuery = "SELECT COUNT(*) FROM ZASSET a WHERE NOT EXISTS " +
		"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"
)

func TestMigrations(t *testing.T) {
	dir := RepoDir(t, "libraries")
	names := Libraries(t)
	if len(names) == 0 {
		t.Fatalf("no test libraries in %s", dir)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			want, ok := expected[name]
			if !ok {
				if _, err := os.Stat(filepath.Join(dir, name, "migrations")); err == nil {
					t.Fatalf("%s has a migrations directory but no entry in the expected table of this test", name)
				}
			}
			db := OpenDB(t, dbFile(Open(t, name)))
			pristine := OpenDB(t, dbFile(OpenPristine(t, name)))

			wantAssets := slices.Sorted(slices.Values(want.assetMarkers))
			if got := Rows(t, db, assetMarkersQuery); !slices.Equal(got, wantAssets) {
				t.Errorf("asset markers:\n got %q\nwant %q", got, wantAssets)
			}
			wantAlbums := slices.Sorted(slices.Values(want.albumMarkers))
			if got := Rows(t, db, albumMarkersQuery); !slices.Equal(got, wantAlbums) {
				t.Errorf("album markers:\n got %q\nwant %q", got, wantAlbums)
			}
			if got, was := Count(t, db, assetCountQuery), Count(t, pristine, assetCountQuery); got != was {
				t.Errorf("assets: %d after migrations, %d before", got, was)
			}
			if got, was := Count(t, db, withoutAttributesQuery), Count(t, pristine, withoutAttributesQuery); got != was+want.assetsLosingAttributes {
				t.Errorf("assets without additional attributes: %d after migrations, %d before, want %d more",
					got, was, want.assetsLosingAttributes)
			}
			for _, c := range want.checks {
				if got := Scalar(t, db, c.query); got != c.want {
					t.Errorf("%s\n got %q\nwant %q", c.query, got, c.want)
				}
			}
		})
	}
}

func dbFile(lib string) string { return filepath.Join(lib, "database", "Photos.sqlite") }

func TestSynthetic(t *testing.T) {
	t.Parallel()
	base := OpenDB(t, dbFile(OpenPristine(t, "synthetic")))
	db := OpenDB(t, dbFile(Synthetic(t, 3)))
	for _, q := range []string{
		"SELECT COUNT(*) FROM ZASSET",
		"SELECT COUNT(DISTINCT ZUUID) FROM ZASSET",
		"SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES",
		"SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET AND a.ZADDITIONALATTRIBUTES = x.Z_PK",
		"SELECT COUNT(*) FROM ZCLOUDMASTER",
		"SELECT COUNT(*) FROM ZASSET a JOIN ZCLOUDMASTER m ON m.Z_PK = a.ZMASTER",
		"SELECT COUNT(*) FROM ZINTERNALRESOURCE",
		"SELECT COUNT(*) FROM ZINTERNALRESOURCE r JOIN ZASSET a ON a.Z_PK = r.ZASSET",
	} {
		if got, want := Count(t, db, q), 3*Count(t, base, q); got != want || want == 0 {
			t.Errorf("%s: %d, want %d (three times the synthetic library, not 0)", q, got, want)
		}
	}
	if n := Count(t, db, "SELECT COUNT(*) FROM (SELECT ZMASTER FROM ZASSET WHERE ZMASTER IS NOT NULL GROUP BY ZMASTER HAVING COUNT(*) > 1)"); n != 0 {
		t.Errorf("%d cloud masters are shared by several assets", n)
	}
}

// The reference data in testdata/golden/synthetic was made from this content:
// change the hashes only together with a new golden-regen run and that data.
func TestPinnedSynthetic(t *testing.T) {
	t.Parallel()
	db := OpenDB(t, dbFile(OpenPristine(t, "synthetic")))
	want := map[string]string{
		"ZADDITIONALASSETATTRIBUTES": "a0feab3f0ba6eb1fa1cfd9cdeae1d8e839d6a97deee0fa31e72c2397d144f69e",
		"ZASSET":                     "6d2d9b87de17ab1f947578323dc1adc468ab6e378bbcaa617fb3f3b32a4640eb",
		"ZCLOUDMASTER":               "0b16088332ad364ce67bf0430bcccfc8cbd138fcca4f2898f9e6a65bbee5fa77",
		"ZGENERICALBUM":              "0edff7f44c9549940cca5fb5ece6afd2ee993e93e1ca41aa3f7491f5b533a8e8",
		"ZINTERNALRESOURCE":          "a0c7073c0e59cfe254990ca61fdf0426231418c89d7a1b93747336597f063080",
		"ZSHARE":                     "91ccadb230eadb3630ded0ca16977f6d9b1d4bb1472effcebf4de6bc1e32780e",
		"Z_33ASSETS":                 "185264c4115e8f4d0030fefd689b1a8cb69c3d7b6007361aa60d8e716dc7e044",
		"Z_METADATA":                 "1499d1544d311f49bf443419b78f525da0f1027e769fea8bce2ee62b9b4d0816",
		"Z_PRIMARYKEY":               "217ea8c9942c73296138c4e5449e5bf4e3d9436fcf8167d8169729bd98136e46",
		// The root node of an empty R-tree.
		"Z_RT_Asset_boundedByRect_node": "c5916478df6da12198b3bd5af9491dbdc3015da888acd225e33aec67e6309393",
	}
	for _, table := range slices.Sorted(maps.Keys(want)) {
		if got := hashRows(t, db, `SELECT * FROM "`+table+`" ORDER BY rowid`); got != want[table] {
			t.Errorf("SHA-256 of %s:\n got %s\nwant %s", table, got, want[table])
		}
	}
	tables, err := ordinaryTables(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if _, pinned := want[table]; !pinned {
			if n := Count(t, db, `SELECT COUNT(*) FROM "`+table+`"`); n != 0 {
				t.Errorf("%s has %d rows, want 0 (a table without a pinned hash must be empty)", table, n)
			}
		}
	}
	if n := Count(t, db, "PRAGMA freelist_count"); n != 0 {
		t.Errorf("freelist_count %d, want 0 (free pages can keep old data)", n)
	}
	if got, want := hashRows(t, db, "SELECT type, name, sql FROM sqlite_master ORDER BY type, name"),
		"b74f6dfd6522b50741b090d7c8f699dcbb8221ff6863fcbbc32bfd5d5a3d8c17"; got != want {
		t.Errorf("SHA-256 of sqlite_master:\n got %s\nwant %s", got, want)
	}
}

func hashRows(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			s := fmt.Sprint(v)
			if b, ok := v.([]byte); ok {
				s = hex.EncodeToString(b)
			}
			_, _ = fmt.Fprintf(h, "%T:%s\x00", v, s)
		}
		_, _ = h.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

var (
	migrationName = regexp.MustCompile(`^[0-9]{3}_[a-z0-9-]+\.sql$`)
	insertWord    = regexp.MustCompile(`(?i)\binsert\b`)
)

func TestMigrationFiles(t *testing.T) {
	dir := RepoDir(t, "libraries")
	for _, name := range Libraries(t) {
		for _, sub := range []string{"migrations", "reference"} {
			checkSQLDir(t, filepath.Join(dir, name, sub))
		}
	}
}

func checkSQLDir(t *testing.T, mdir string) {
	t.Helper()
	entries, err := os.ReadDir(mdir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Errorf("%s: empty directory", mdir)
	}
	for i, e := range entries {
		p := filepath.Join(mdir, e.Name())
		if !e.Type().IsRegular() || !migrationName.MatchString(e.Name()) {
			t.Errorf("%s: only regular files named NNN_<case>.sql are allowed", p)
			continue
		}
		if prefix := fmt.Sprintf("%03d_", i+1); !strings.HasPrefix(e.Name(), prefix) {
			t.Errorf("%s: want the number %s (files are numbered from 001 without gaps)", p, prefix)
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if insertWord.Match(src) {
			t.Errorf("%s: INSERT is not allowed in migrations", p)
		}
		stmts, err := parse(string(src))
		switch {
		case err != nil:
			t.Errorf("%s: %v", p, err)
		case len(stmts) == 0:
			t.Errorf("%s: no statements", p)
		}
	}
}

func TestParse(t *testing.T) {
	src := "-- header\n" +
		"\n" +
		"UPDATE t SET v = 1\n" +
		"  WHERE id = 1;\n" +
		"-- rows: any\n" +
		"DELETE FROM t WHERE id = 2;\n" +
		"-- rows: any\n" +
		"\n" +
		"DELETE FROM t WHERE id = 3;\n" +
		"-- rows: any\n" +
		"-- another comment\n" +
		"DELETE FROM t WHERE id = 4;\n"
	got, err := parse(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []statement{
		{sql: "UPDATE t SET v = 1\nWHERE id = 1;", line: 3},
		{sql: "DELETE FROM t WHERE id = 2;", line: 6, anyRows: true},
		// "-- rows: any" counts only on the line right before the statement.
		{sql: "DELETE FROM t WHERE id = 3;", line: 9},
		{sql: "DELETE FROM t WHERE id = 4;", line: 12},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parse:\n got %+v\nwant %+v", got, want)
	}

	if _, err := parse("UPDATE t SET v = 1;\nUPDATE t SET v = 2\n"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("unterminated statement: got error %v, want one about line 2", err)
	}
}

func TestMigrate(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantErr   string
		wantValue string
	}{
		{
			name: "applies files in name order",
			files: map[string]string{
				"002_b.sql": "-- rows: any\nDELETE FROM t WHERE id = 99;\nUPDATE t SET v = v || 'c' WHERE id = 1;\n",
				"001_a.sql": "-- header\nUPDATE t\nSET v = 'b'\nWHERE id = 1;\n",
			},
			wantValue: "bc",
		},
		{
			name: "statement that changes no rows rolls everything back",
			files: map[string]string{
				"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1;\n",
				"002_b.sql": "-- header\nUPDATE t SET v = 'x' WHERE id = 99;\n",
			},
			wantErr:   "002_b.sql:2:",
			wantValue: "a",
		},
		{
			name:      "invalid statement",
			files:     map[string]string{"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1;\nUPDATE missing SET v = 1;\n"},
			wantErr:   "001_a.sql:2:",
			wantValue: "a",
		},
		{
			name:      "unterminated statement",
			files:     map[string]string{"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1\n"},
			wantErr:   "001_a.sql:",
			wantValue: "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := newTestDB(t)
			err := migrate(t.Context(), dbPath, writeFiles(t, tt.files))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("migrate: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("migrate: got error %v, want one containing %q", err, tt.wantErr)
			}
			if got := Scalar(t, OpenDB(t, dbPath), "SELECT v FROM t WHERE id = 1"); got != tt.wantValue {
				t.Errorf("t.v = %q, want %q", got, tt.wantValue)
			}
		})
	}
}

// The trigger of newTestDB calls the Core Data functions like those of
// Photos.sqlite, so TestMigrate also covers their stand-ins.
func newTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	trigger := "CREATE TRIGGER t_core_data AFTER UPDATE ON t FOR EACH ROW BEGIN SELECT "
	for i, name := range coreDataFunctions {
		if i > 0 {
			trigger += ", "
		}
		trigger += name + "('T', 1, NEW.id, 'V', NEW.v)"
	}
	trigger += "; END"
	for _, stmt := range []string{
		"CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)",
		"INSERT INTO t (id, v) VALUES (1, 'a')",
		trigger,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func writeFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(files[name]), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}
