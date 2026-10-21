package testlib

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// libCase is what the migrations of one library must leave in its copy.
type libCase struct {
	assets  []string // markers in ZADDITIONALASSETATTRIBUTES.ZTITLE, without the "mig:" prefix
	albums  []string // markers in ZGENERICALBUM.ZTITLE, without the "mig:" prefix
	skipped int      // assets that lose their ZADDITIONALASSETATTRIBUTES row
	checks  []check
}

// check is a query that returns one value; NULL is rendered as "NULL".
type check struct {
	query string
	want  string
}

// pk is the ZASSET primary key of the asset marked "mig:<marker>".
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

// burstChecks takes the album membership table of the schema and its album column.
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

// expected lists every library that has migrations. A library that is not
// here must have no migrations directory and gets no markers.
var expected = map[string]libCase{
	"Test-13.0.0": {
		assets: slices.Concat(assetCloudGUIDMarkers, fingerprintMarkers, burstMarkers),
		albums: albumCloudGUIDMarkers,
		checks: slices.Concat(assetCloudGUIDChecks(), albumCloudGUIDChecks(),
			fingerprintChecks("ZMASTERFINGERPRINT"), burstChecks("Z_28ASSETS", "Z_28ALBUMS")),
	},
	"Test-Cloud-13.1": {
		assets: []string{"asset-cloud-guid-null", "asset-cloud-guid-empty", "cloud-master-local", "cloud-master-none"},
		albums: []string{"album-cloud-guid-null"},
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
		assets: slices.Concat(resourcesMarkers, rawMarkers, []string{"trash"}),
		checks: slices.Concat(resourcesChecks(), rawChecks(), []check{
			{"SELECT ZTRASHEDSTATE || '|' || ZTRASHEDDATE FROM ZASSET WHERE Z_PK = " + pk("trash"), "1|694224000"},
		}),
	},
	"Test-14.6.0": {
		assets: []string{"shared-library-member", "shared-library-null", "tzoffset-null",
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
		assets: []string{"syndicated-not-saved", "syndicated-saved", "shared-album-photo"},
		checks: []check{
			{"SELECT ZSYNDICATIONIDENTIFIER || '|' || ZSYNDICATIONHISTORY FROM ZADDITIONALASSETATTRIBUTES " +
				"WHERE ZTITLE = 'mig:syndicated-not-saved'", "TEST-SYND-0001|0"},
			{"SELECT ZSYNDICATIONIDENTIFIER FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:syndicated-saved'", "TEST-SYND-0002"},
			{"SELECT ZSYNDICATIONHISTORY FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:syndicated-saved'", "NULL"},
			{"SELECT ZCLOUDBATCHPUBLISHDATE FROM ZASSET WHERE Z_PK = " + pk("shared-album-photo"), "694224000"},
		},
	},
	"Test-16.0.0-beta": {
		assets:  []string{"date-modified-raw-value", "date-modified-raw-null"},
		skipped: 1,
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
		assets: slices.Concat(assetCloudGUIDMarkers, fingerprintMarkers, burstMarkers, resourcesMarkers),
		albums: albumCloudGUIDMarkers,
		checks: slices.Concat(assetCloudGUIDChecks(), albumCloudGUIDChecks(),
			fingerprintChecks("ZORIGINALSTABLEHASH"), burstChecks("Z_33ASSETS", "Z_33ALBUMS"), resourcesChecks()),
	},
	"Test-27.0_DevBeta": {
		assets: fingerprintMarkers,
		checks: fingerprintChecks("ZORIGINALSTABLEHASH"),
	},
}

const (
	assetMarkersQuery = "SELECT substr(ZTITLE, 5) FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE LIKE 'mig:%' ORDER BY 1"
	albumMarkersQuery = "SELECT substr(ZTITLE, 5) FROM ZGENERICALBUM WHERE ZTITLE LIKE 'mig:%' ORDER BY 1"
	assetCountQuery   = "SELECT COUNT(*) FROM ZASSET"
	skippedQuery      = "SELECT COUNT(*) FROM ZASSET a WHERE NOT EXISTS " +
		"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"
)

// TestMigrations applies the migrations of every test library to a copy and
// checks the markers and values they leave, and that the databases in
// testdata stay byte for byte as they were.
func TestMigrations(t *testing.T) {
	dir := librariesDir(t)
	before := hashTree(t, dir)

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
			db := openDB(t, Open(t, name))
			pristine := openDB(t, OpenPristine(t, name))

			wantAssets := slices.Sorted(slices.Values(want.assets))
			if got := rowStrings(t, db, assetMarkersQuery); !slices.Equal(got, wantAssets) {
				t.Errorf("asset markers:\n got %q\nwant %q", got, wantAssets)
			}
			wantAlbums := slices.Sorted(slices.Values(want.albums))
			if got := rowStrings(t, db, albumMarkersQuery); !slices.Equal(got, wantAlbums) {
				t.Errorf("album markers:\n got %q\nwant %q", got, wantAlbums)
			}
			if got, was := count(t, db, assetCountQuery), count(t, pristine, assetCountQuery); got != was {
				t.Errorf("assets: %d after migrations, %d before", got, was)
			}
			if got, was := count(t, db, skippedQuery), count(t, pristine, skippedQuery); got != was+want.skipped {
				t.Errorf("assets without additional attributes: %d after migrations, %d before, want %d more",
					got, was, want.skipped)
			}
			for _, c := range want.checks {
				if got := scalar(t, db, c.query); got != c.want {
					t.Errorf("%s\n got %q\nwant %q", c.query, got, c.want)
				}
			}
		})
	}

	after := hashTree(t, dir)
	for _, p := range slices.Sorted(maps.Keys(before)) {
		if before[p] != after[p] {
			t.Errorf("%s changed or disappeared during the test (SHA-256 before %q, after %q)", p, before[p], after[p])
		}
	}
	for _, p := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared during the test", p)
		}
	}
}

var (
	migrationName = regexp.MustCompile(`^[0-9]{3}_[a-z0-9-]+\.sql$`)
	insertWord    = regexp.MustCompile(`(?i)\binsert\b`)
)

// TestMigrationFiles checks the layout of every migrations directory: only
// files NNN_<case>.sql numbered from 001 without gaps, no INSERT, and each
// file parses into at least one statement.
func TestMigrationFiles(t *testing.T) {
	dir := librariesDir(t)
	for _, name := range Libraries(t) {
		mdir := filepath.Join(dir, name, "migrations")
		entries, err := os.ReadDir(mdir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Errorf("%s: empty migrations directory", mdir)
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

// TestMigrate runs migrate on a small database whose trigger calls the Core
// Data functions, so it also checks that their stand-ins are registered.
func TestMigrate(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string // empty: no error
		want    string // value of t.v after migrate
	}{
		{
			name: "applies files in name order",
			files: map[string]string{
				"002_b.sql": "-- rows: any\nDELETE FROM t WHERE id = 99;\nUPDATE t SET v = v || 'c' WHERE id = 1;\n",
				"001_a.sql": "-- header\nUPDATE t\nSET v = 'b'\nWHERE id = 1;\n",
			},
			want: "bc",
		},
		{
			name: "statement that changes no rows rolls everything back",
			files: map[string]string{
				"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1;\n",
				"002_b.sql": "-- header\nUPDATE t SET v = 'x' WHERE id = 99;\n",
			},
			wantErr: "002_b.sql:2:",
			want:    "a",
		},
		{
			name:    "invalid statement",
			files:   map[string]string{"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1;\nUPDATE missing SET v = 1;\n"},
			wantErr: "001_a.sql:2:",
			want:    "a",
		},
		{
			name:    "unterminated statement",
			files:   map[string]string{"001_a.sql": "UPDATE t SET v = 'b' WHERE id = 1\n"},
			wantErr: "001_a.sql:",
			want:    "a",
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
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			if got := scalar(t, db, "SELECT v FROM t WHERE id = 1"); got != tt.want {
				t.Errorf("t.v = %q, want %q", got, tt.want)
			}
		})
	}
}

// newTestDB creates a database with one row and a trigger like the ones in
// Photos.sqlite that call Core Data functions.
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

// writeFiles writes the files into a temporary directory and returns their
// paths in name order.
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

func openDB(t *testing.T, lib string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(lib, "database", "Photos.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// scalar returns the single value of query; NULL is "NULL", no row is "<no row>".
func scalar(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	var v sql.NullString
	err := db.QueryRowContext(t.Context(), query).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "<no row>"
	case err != nil:
		t.Fatalf("%s: %v", query, err)
	case !v.Valid:
		return "NULL"
	}
	return v.String
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	s := scalar(t, db, query)
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("%s: %q is not a number", query, s)
	}
	return n
}

func rowStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return out
}

// hashTree returns the SHA-256 of every file under dir, keyed by relative path.
func hashTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	// os.Root keeps every open inside dir, so a symlink cannot lead the walk out of it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := root.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		sums[filepath.FromSlash(p)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}
