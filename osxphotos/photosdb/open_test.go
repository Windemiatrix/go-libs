package photosdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

type (
	photoInfo     = photoinfo.PhotoInfo
	photoResource = photoinfo.Resource
)

var uuidForm = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// internal/testlib provides the Core Data functions that the triggers call.
func change(t *testing.T, lib string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(lib, "database", "Photos.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	for _, s := range stmts {
		if _, err := db.ExecContext(t.Context(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func count(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

const withoutAttributes = "SELECT COUNT(*) FROM ZASSET a WHERE NOT EXISTS " +
	"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"

func assertNoCopies(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "osxphotos-sqlread-") {
			t.Errorf("%s holds the copy %s", dir, e.Name())
		}
	}
}

type fileState struct {
	mode  fs.FileMode
	size  int64
	mtime time.Time
	sum   [sha256.Size]byte
}

func snapshotTree(t *testing.T, dir string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		st := fileState{mode: info.Mode(), size: info.Size(), mtime: info.ModTime()}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(p) //nolint:gosec // G122: a file of a test library copy
			if err != nil {
				return err
			}
			st.sum = sha256.Sum256(b)
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = st
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertTree(t *testing.T, name string, before, after map[string]fileState) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("%s: %d entries before Open, %d after", name, len(before), len(after))
	}
	for p, st := range before {
		if got, ok := after[p]; !ok || got != st {
			t.Errorf("%s: %s changed by Open", name, p)
		}
	}
	for p := range after {
		if strings.HasSuffix(p, "-shm") {
			t.Errorf("%s: Open left %s", name, p)
		}
	}
}

func TestOpenLibraries(t *testing.T) {
	t.Parallel()

	type library struct {
		name, path string
		model      int
		id         string
	}
	var libs []library
	for _, name := range testlib.Libraries(t) {
		want, ok := libraryModels[name]
		if !ok {
			t.Fatalf("test library %s is not in libraryModels", name)
		}
		libs = append(libs, library{name, testlib.OpenPristine(t, name), want.model, want.id})
	}
	for _, lib := range libs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			before := snapshotTree(t, lib.path)
			db, err := Open(t.Context(), lib.path, Options{TempDir: tmp})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			assertTree(t, lib.name, before, snapshotTree(t, lib.path))
			assertNoCopies(t, tmp)
			info := db.Info()
			version := schemaByID(t, lib.id).PhotosVersion
			if info.ModelVersion != lib.model || info.SchemaID != lib.id || info.PhotosVersion != version ||
				!uuidForm.MatchString(info.LibraryUUID) || info.LibraryPath != lib.path {
				t.Errorf("Info = %+v, want model %d, schema %s, Photos version %v, a library UUID", info, lib.model, lib.id, version)
			}
			src := roDB(t, lib.path)
			skipped := count(t, src, withoutAttributes)
			photos := db.Photos()
			if want := count(t, src, "SELECT COUNT(*) FROM ZASSET") - skipped; len(photos) != want || info.SkippedAssets != skipped {
				t.Errorf("%d photos, %d skipped; want %d, %d", len(photos), info.SkippedAssets, want, skipped)
			}
			for _, p := range photos {
				if got, ok := db.Photo(p.UUID); !ok || got != p {
					t.Fatalf("Photo(%s) is not the photo of Photos", p.UUID)
				}
			}
			if _, ok := db.Photo("no such uuid"); ok {
				t.Error("Photo of an unknown UUID: found")
			}
			if err := db.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		})
	}
}

func TestOpenTempDir(t *testing.T) {
	// Not parallel: a case sets TMPDIR.
	lib := testlib.OpenPristine(t, "synthetic")
	link := filepath.Join(t.TempDir(), "link.photoslibrary")
	if err := os.Symlink(lib, link); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(lib, "resources", "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	sibling := lib + "2"
	if err := os.Mkdir(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := []struct{ name, lib, tmp string }{
		{"the library", lib, lib},
		{"its database directory", lib, filepath.Join(lib, "database")},
		{"a subdirectory", lib, sub},
		{"a link to the library", lib, link},
		{"the library through a link, the directory by its real path", link, filepath.Join(lib, "database")},
	}
	// On a case-insensitive file system the library can be named in another
	// case; the path strings differ, the directory is the same.
	otherCase := filepath.Join(filepath.Dir(lib), strings.ToUpper(filepath.Base(lib)), "database")
	if a, err := os.Stat(otherCase); err == nil {
		if b, err := os.Stat(filepath.Join(lib, "database")); err == nil && os.SameFile(a, b) {
			inside = append(inside, struct{ name, lib, tmp string }{"the library in another case", lib, otherCase})
		}
	}
	if len(inside) == 5 {
		t.Log("the temporary file system is case-sensitive: the library in another case is not checked")
	}
	for _, tt := range inside {
		before := snapshotTree(t, lib)
		_, err := Open(t.Context(), tt.lib, Options{TempDir: tt.tmp})
		if err == nil || !strings.Contains(err.Error(), "inside the library") {
			t.Errorf("%s: Open error %v, want one about a temporary directory inside the library", tt.name, err)
		}
		assertTree(t, tt.name, before, snapshotTree(t, lib))
	}

	db, err := Open(t.Context(), lib, Options{TempDir: sibling})
	if err != nil {
		t.Fatalf("TempDir next to the library: %v", err)
	}
	_ = db.Close()
	assertNoCopies(t, sibling)

	if _, err := Open(t.Context(), lib, Options{TempDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("Open with a missing TempDir: no error")
	}
	if _, err := Open(t.Context(), filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Error("Open of a missing library: no error")
	}

	// "" is os.TempDir(), which is TMPDIR.
	t.Setenv("TMPDIR", filepath.Join(lib, "database"))
	if _, err := Open(t.Context(), lib, Options{}); err == nil || !strings.Contains(err.Error(), "inside the library") {
		t.Errorf("TempDir \"\" with TMPDIR inside the library: error %v", err)
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if _, err := Open(t.Context(), lib, Options{}); err != nil {
		t.Errorf("TempDir \"\": %v", err)
	}
	assertNoCopies(t, tmp)
}

func openPathDB(t *testing.T, path string, o Options) *DB {
	t.Helper()
	if o.TempDir == "" {
		o.TempDir = t.TempDir()
	}
	db, err := Open(t.Context(), path, o)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	return db
}

func openPathDatabase(lib string) string { return filepath.Join(lib, "database", "Photos.sqlite") }

// photosdb.py:353-385, 455.
func TestOpenPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input func(lib string) string
	}{
		{"library directory", func(lib string) string { return lib }},
		{"Photos.sqlite", openPathDatabase},
		{"photos.db", func(lib string) string { return filepath.Join(lib, "database", "photos.db") }},
		{"PHOTOS.DB", func(lib string) string { return filepath.Join(lib, "database", "PHOTOS.DB") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			writeFile(t, filepath.Join(lib, "database", "photos.db"))
			writeFile(t, filepath.Join(lib, "database", "PHOTOS.DB"))
			info := openPathDB(t, tt.input(lib), Options{}).Info()
			if info.LibraryPath != lib || info.DBPath != openPathDatabase(lib) {
				t.Errorf("LibraryPath, DBPath = %q, %q; want %q, %q", info.LibraryPath, info.DBPath, lib, openPathDatabase(lib))
			}
		})
	}
}

// photosdb.py:377-385, 455.
func TestOpenPathsOtherName(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	db := filepath.Join(lib, "database", "db.sqlite")
	if err := os.Rename(openPathDatabase(lib), db); err != nil {
		t.Fatal(err)
	}
	info := openPathDB(t, db, Options{}).Info()
	if info.LibraryPath != lib || info.DBPath != db {
		t.Errorf("LibraryPath, DBPath = %q, %q; want %q, %q", info.LibraryPath, info.DBPath, lib, db)
	}
}

// photosdb.py:349-350, 455.
func TestOpenPathsRelative(t *testing.T) {
	lib := testlib.OpenPristine(t, "synthetic")

	t.Chdir(filepath.Join(lib, "database"))
	info := openPathDB(t, "Photos.sqlite", Options{}).Info()
	if info.LibraryPath != lib || info.DBPath != openPathDatabase(lib) {
		t.Errorf("Photos.sqlite: LibraryPath, DBPath = %q, %q; want %q, %q", info.LibraryPath, info.DBPath, lib, openPathDatabase(lib))
	}

	t.Chdir(filepath.Dir(lib))
	name := filepath.Base(lib)
	info = openPathDB(t, name, Options{}).Info()
	if info.LibraryPath != name || info.DBPath != openPathDatabase(lib) {
		t.Errorf("directory: LibraryPath, DBPath = %q, %q; want %q, %q", info.LibraryPath, info.DBPath, name, openPathDatabase(lib))
	}
}

func TestOpenPathsRefused(t *testing.T) {
	t.Parallel()

	layout := filepath.Join(t.TempDir(), "lib", "database")
	legacy := filepath.Join(layout, "photos.db")
	writeFile(t, legacy)
	other := filepath.Join(layout, "notes.txt")
	writeFile(t, other)
	tests := []struct {
		name, path string
		options    Options
		want       error
	}{
		{"a missing path", filepath.Join(t.TempDir(), "missing"), Options{}, fs.ErrNotExist},
		{"photos.db without Photos.sqlite", legacy, Options{}, fs.ErrNotExist},
		{"a directory without a database", t.TempDir(), Options{}, fs.ErrNotExist},
		{"Options.LibraryPath missing", testlib.OpenPristine(t, "synthetic"),
			Options{LibraryPath: filepath.Join(t.TempDir(), "missing")}, fs.ErrNotExist},
		{"a file that is not a database", other, Options{}, nil},
	}
	for _, tt := range tests {
		tmp := t.TempDir()
		tt.options.TempDir = tmp
		db, err := Open(t.Context(), tt.path, tt.options)
		if db != nil || err == nil || (tt.want != nil && !errors.Is(err, tt.want)) || strings.Contains(err.Error(), "inside the library") {
			t.Errorf("%s: Open = %v, %v; want nil and an error of %v, not about the temporary directory", tt.name, db, err, tt.want)
		}
		assertNoCopies(t, tmp)
	}
}

// A copy of the database inside any library that the paths name would become
// part of that library.
func TestOpenPathsTempDir(t *testing.T) {
	t.Parallel()

	read := testlib.OpenPristine(t, "synthetic")
	elsewhere := testlib.OpenPristine(t, "synthetic")
	realElsewhere, err := filepath.EvalSymlinks(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	link := openPathDatabase(filepath.Join(t.TempDir(), "link.photoslibrary"))
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(openPathDatabase(elsewhere), link); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path string
		options    Options
		library    string
	}{
		{"the directory of a database read from elsewhere", openPathDatabase(read),
			Options{LibraryPath: elsewhere, TempDir: filepath.Join(read, "database")}, read},
		{"Options.LibraryPath", openPathDatabase(read), Options{LibraryPath: elsewhere, TempDir: elsewhere}, elsewhere},
		{"the library derived from Photos.sqlite", openPathDatabase(read), Options{TempDir: read}, read},
		{"the library read while Options.LibraryPath is set", read, Options{LibraryPath: elsewhere, TempDir: read}, read},
		{"the library of Photos.sqlite read while Options.LibraryPath is set", openPathDatabase(read),
			Options{LibraryPath: elsewhere, TempDir: read}, read},
		{"the library a linked Photos.sqlite points into", link, Options{TempDir: filepath.Join(elsewhere, "database")}, realElsewhere},
		{"the library directory whose Photos.sqlite is a link", filepath.Dir(filepath.Dir(link)),
			Options{TempDir: filepath.Join(elsewhere, "database")}, realElsewhere},
	}
	for _, tt := range tests {
		before, beforeElsewhere := snapshotTree(t, read), snapshotTree(t, elsewhere)
		_, err := Open(t.Context(), tt.path, tt.options)
		if err == nil || !strings.HasSuffix(err.Error(), "inside the library "+tt.library) {
			t.Errorf("%s: Open error %v, want one about a temporary directory inside the library %s", tt.name, err, tt.library)
		}
		assertTree(t, tt.name, before, snapshotTree(t, read))
		assertTree(t, tt.name, beforeElsewhere, snapshotTree(t, elsewhere))
	}
}

func TestOpenNotDatabaseMessage(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "lib", "database")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte(strings.Repeat("SECRET-CONTENT\n", 300)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(t.Context(), file, Options{TempDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not a database") || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("Open error %v, want one that says the file is not a database, without its content", err)
	}
}

// photosdb.py:456-458.
func TestOpenLibraryPath(t *testing.T) {
	t.Parallel()

	withOriginals, uuid, dir, file := pathsLibrary(t)
	read := testlib.Open(t, "Test-14.0.0")
	want := filepath.Join(withOriginals, "originals", dir, file)
	tests := []struct {
		name, path  string
		options     Options
		wantLibrary string
		wantPath    string
	}{
		{"without Options.LibraryPath", read, Options{}, read, ""},
		{"a library directory", read, Options{LibraryPath: withOriginals}, withOriginals, want},
		{"Photos.sqlite", openPathDatabase(read), Options{LibraryPath: withOriginals}, withOriginals, want},
	}
	for _, tt := range tests {
		db := openPathDB(t, tt.path, tt.options)
		p, ok := db.Photo(uuid)
		if !ok {
			t.Fatalf("%s: Photo(%s) not found", tt.name, uuid)
		}
		if info := db.Info(); info.LibraryPath != tt.wantLibrary || info.DBPath != openPathDatabase(read) || p.Path != tt.wantPath {
			t.Errorf("%s: LibraryPath, DBPath, Path = %q, %q, %q; want %q, %q, %q",
				tt.name, info.LibraryPath, info.DBPath, p.Path, tt.wantLibrary, openPathDatabase(read), tt.wantPath)
		}
	}
}

func TestOpenRemovesCopy(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name        string
		ctx         context.Context
		edit        []string
		unsupported bool
		want        string
	}{
		{"success", t.Context(), nil, false, ""},
		{"unknown model", t.Context(), []string{"UPDATE Z_METADATA SET Z_PLIST = CAST('" + strings.ReplaceAll(string(modelPlist("<integer>18100</integer>")), "'", "''") + "' AS BLOB)"},
			true, "18100"},
		{"no metadata", t.Context(), []string{"DELETE FROM Z_METADATA"}, false, "Z_METADATA"},
		{"missing column", t.Context(), []string{"ALTER TABLE ZASSET RENAME COLUMN ZCLOUDASSETGUID TO ZX"}, true, "ZASSET.ZCLOUDASSETGUID"},
		{"missing table", t.Context(), []string{"ALTER TABLE ZKEYWORD RENAME TO ZKEYWORDX"}, true, "table ZKEYWORD"},
		{"missing column of a person name", t.Context(), []string{"ALTER TABLE ZCLOUDSHAREDALBUMINVITATIONRECORD RENAME COLUMN ZINVITEEFULLNAME TO ZX"},
			true, "ZCLOUDSHAREDALBUMINVITATIONRECORD.ZINVITEEFULLNAME"},
		{"canceled", canceled, nil, false, "context canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			change(t, lib, tt.edit...)
			tmp := t.TempDir()
			before := snapshotTree(t, lib)
			db, err := Open(tt.ctx, lib, Options{TempDir: tmp})
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("Open: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("Open error %v, want one with %q", err, tt.want)
			case tt.want != "" && db != nil:
				t.Error("Open returned a DB with an error")
			}
			if tt.unsupported != errors.Is(err, ErrUnsupportedSchema) {
				t.Errorf("errors.Is(%v, ErrUnsupportedSchema) = %v, want %v", err, !tt.unsupported, tt.unsupported)
			}
			assertNoCopies(t, tmp)
			assertTree(t, tt.name, before, snapshotTree(t, lib))
		})
	}
}

// TestOpenEmptyDatabase: sqlread opens a zero-length Photos.sqlite as an
// empty database, so Open must still fail and remove the copy.
func TestOpenEmptyDatabase(t *testing.T) {
	t.Parallel()

	lib := filepath.Join(t.TempDir(), "empty.photoslibrary")
	if err := os.MkdirAll(filepath.Join(lib, "database"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "database", "Photos.sqlite"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	before := snapshotTree(t, lib)
	db, err := Open(t.Context(), lib, Options{TempDir: tmp})
	if err == nil || db != nil {
		t.Fatalf("Open of an empty database = %v, %v; want nil and an error", db, err)
	}
	assertNoCopies(t, tmp)
	assertTree(t, "empty database", before, snapshotTree(t, lib))
}

func TestOpenHostileValues(t *testing.T) {
	t.Parallel()

	const member = "(SELECT MIN(rowid) FROM Z_33ASSETS)"
	tests := []struct {
		name string
		edit string
		ok   bool
	}{
		{"text in an integer column", "UPDATE ZASSET SET ZWIDTH = 'SECRET-W' WHERE Z_PK = 3", false},
		{"text in a date column", "UPDATE ZASSET SET ZDATECREATED = 'SECRET-D' WHERE Z_PK = 4", false},
		{"real in an integer column", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZTIMEZONEOFFSET = 1.5 WHERE Z_PK = 5", false},
		{"text in a resource column", "UPDATE ZINTERNALRESOURCE SET ZDATALENGTH = 'SECRET-L' WHERE Z_PK = 6", false},
		{"blob UUID", "UPDATE ZASSET SET ZUUID = CAST(ZUUID AS BLOB) WHERE Z_PK = 7", true},
		{"NULL album of a membership", "UPDATE Z_33ASSETS SET Z_33ALBUMS = NULL WHERE rowid = " + member, true},
		{"NULL asset of a membership", "UPDATE Z_33ASSETS SET Z_3ASSETS = NULL WHERE rowid = " + member, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			change(t, lib, tt.edit)
			tmp := t.TempDir()
			db, err := Open(t.Context(), lib, Options{TempDir: tmp})
			assertNoCopies(t, tmp)
			if !tt.ok {
				if err == nil || !strings.Contains(err.Error(), "unexpected value type") || strings.Contains(err.Error(), "SECRET") {
					t.Fatalf("Open error %v, want an unexpected value type without the value", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			src := roDB(t, lib)
			if want := count(t, src, "SELECT COUNT(*) FROM ZASSET"); len(db.Photos()) != want {
				t.Errorf("%d photos, want %d", len(db.Photos()), want)
			}
			var uuid string
			if err := src.QueryRowContext(t.Context(), "SELECT CAST(ZUUID AS TEXT) FROM ZASSET WHERE Z_PK = 7").Scan(&uuid); err != nil {
				t.Fatal(err)
			}
			if _, ok := db.Photo(uuid); !ok {
				t.Errorf("Photo(%s) not found", uuid)
			}
			pairs := 0
			for _, p := range db.Photos() {
				pairs += len(p.AlbumUUIDs)
			}
			if want := count(t, src, "SELECT COUNT(*) FROM Z_33ASSETS m JOIN ZGENERICALBUM g ON g.Z_PK = m.Z_33ALBUMS "+
				"WHERE g.ZKIND IN (2, 1505) AND m.Z_3ASSETS IS NOT NULL") + collectionShares(t, src, shareMemberCount); pairs != want {
				t.Errorf("%d album memberships, want %d", pairs, want)
			}
		})
	}
}

func TestOpenDanglingReferences(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	change(t, lib,
		"UPDATE ZASSET SET ZMASTER = 999999 WHERE Z_PK = 2",
		"UPDATE ZADDITIONALASSETATTRIBUTES SET ZASSETDESCRIPTION = 999999 WHERE ZASSET = 2",
		"UPDATE ZASSET SET ZADDEDDATE = NULL WHERE Z_PK = 3",
		"INSERT INTO ZINTERNALRESOURCE (Z_PK, Z_ENT, Z_OPT, ZASSET, ZDATASTORESUBTYPE) SELECT MAX(Z_PK) + 1, MAX(Z_ENT), 1, NULL, 1 FROM ZINTERNALRESOURCE",
		"INSERT INTO ZINTERNALRESOURCE (Z_PK, Z_ENT, Z_OPT, ZASSET, ZDATASTORESUBTYPE) SELECT MAX(Z_PK) + 1, MAX(Z_ENT), 1, 999999, 17 FROM ZINTERNALRESOURCE",
		"INSERT INTO Z_33ASSETS (Z_33ALBUMS, Z_3ASSETS, Z_FOK_3ASSETS) SELECT MAX(Z_PK), 999999, 1 FROM ZGENERICALBUM WHERE ZKIND = 2",
	)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	src := roDB(t, lib)
	uuidOf := func(pk int) string {
		var u string
		if err := src.QueryRowContext(t.Context(), "SELECT ZUUID FROM ZASSET WHERE Z_PK = ?", pk).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	p2, _ := db.Photo(uuidOf(2))
	if p2 == nil || p2.InCloud != nil || p2.CloudMasterGUID != "" || p2.Description != "" {
		t.Errorf("asset 2: %+v; want no cloud master and no description", p2)
	}
	if p3, _ := db.Photo(uuidOf(3)); p3 == nil || p3.DateAdded != nil {
		t.Errorf("asset 3: %+v; want DateAdded nil", p3)
	}
	if n, want := len(db.Photos()), count(t, src, "SELECT COUNT(*) FROM ZASSET"); n != want {
		t.Errorf("%d photos, want %d", n, want)
	}
}

// Upstream lists shared albums (ZKIND 1505) among the albums of a photo
// (photoinfo.py:2023-2034), but not in album_info (photosdb/photosdb.py:612-637).
func TestOpenSharedAlbum(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	var album string
	err := roDB(t, lib).QueryRowContext(t.Context(), "SELECT g.ZUUID FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND g.ZTRASHEDSTATE = 0 AND "+
		"EXISTS (SELECT 1 FROM Z_33ASSETS m WHERE m.Z_33ALBUMS = g.Z_PK) ORDER BY g.Z_PK LIMIT 1").Scan(&album)
	if err != nil {
		t.Fatal(err)
	}
	inAlbum := func(db *DB) int {
		n := 0
		for _, p := range db.Photos() {
			if slices.Contains(p.AlbumUUIDs, album) {
				n++
			}
		}
		return n
	}
	has := func(albums []*albuminfo.AlbumInfo) bool {
		return slices.ContainsFunc(albums, func(a *albuminfo.AlbumInfo) bool { return a.UUID == album })
	}
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	user, ok := before.Album(album)
	photos := inAlbum(before)
	if photos == 0 || !ok || !has(before.AlbumInfo()) {
		t.Fatal("the user album has no photos or is not in AlbumInfo before the change")
	}
	change(t, lib, "UPDATE ZGENERICALBUM SET ZKIND = 1505 WHERE ZUUID = '"+album+"'")
	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if n := inAlbum(after); n != photos {
		t.Errorf("%d photos list the shared album, want %d", n, photos)
	}
	if shared, ok := after.Album(album); !ok || !slices.Equal(shared.PhotoUUIDs, user.PhotoUUIDs) {
		t.Error("Album does not find the shared album with the photos of the user album")
	}
	if !has(after.SharedAlbumInfo()) || has(after.AlbumInfo()) {
		t.Error("the shared album is not in SharedAlbumInfo or is in AlbumInfo")
	}
}

func TestOpenParallel(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	want := count(t, roDB(t, lib), "SELECT COUNT(*) FROM ZASSET")
	tmp := t.TempDir()
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Go(func() {
			db, err := Open(t.Context(), lib, Options{TempDir: tmp})
			if err == nil && len(db.Photos()) != want {
				err = errors.New("not every asset is a photo")
			}
			errs[i] = err
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	assertNoCopies(t, tmp)
}

// Counts and sums catch a column read into the wrong field.
func TestOpenAggregates(t *testing.T) {
	t.Parallel()

	type library struct{ name, path, id string }
	var libs []library
	for _, name := range testlib.Libraries(t) {
		libs = append(libs, library{name, testlib.OpenPristine(t, name), libraryModels[name].id})
	}
	for _, lib := range libs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			photos := db.Photos()
			n := schemaByID(t, lib.id)
			src := roDB(t, lib.path)
			const from = " FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK"
			counts := []struct {
				name  string
				where string
				goVal func(p *photoInfo) bool
			}{
				{"InTrash", "a.ZTRASHEDSTATE = 1", func(p *photoInfo) bool { return p.InTrash }},
				{"Hidden", "a.ZHIDDEN = 1", func(p *photoInfo) bool { return p.Hidden }},
				{"Favorite", "a.ZFAVORITE = 1", func(p *photoInfo) bool { return p.Favorite }},
				{"Live", "a.ZKINDSUBTYPE = 2", func(p *photoInfo) bool { return p.Live }},
				{"Screenshot", "a.ZKINDSUBTYPE = 10", func(p *photoInfo) bool { return p.Screenshot }},
				{"IsPhoto", "a.ZKIND = 0", func(p *photoInfo) bool { return p.IsPhoto }},
				{"IsMovie", "a.ZKIND = 1", func(p *photoInfo) bool { return p.IsMovie }},
				{"Burst", "a.ZAVALANCHEUUID IS NOT NULL", func(p *photoInfo) bool { return p.Burst }},
				{"HasAdjustments", "a." + n.AdjustmentsColumn + " != 0", func(p *photoInfo) bool { return p.HasAdjustments }},
				{"Shared", "a.ZCLOUDBATCHPUBLISHDATE IS NOT NULL", func(p *photoInfo) bool { return p.Shared }},
				{"IsReference", "a.ZSAVEDASSETTYPE = 10", func(p *photoInfo) bool { return p.IsReference }},
				{"IsCloudAsset", "a.ZCLOUDASSETGUID != ''", func(p *photoInfo) bool { return p.IsCloudAsset }},
				{"TZOffset nil", "x.ZTIMEZONEOFFSET IS NULL", func(p *photoInfo) bool { return p.TZOffset == nil }},
				// Outside the years 1-9999 Python raises and upstream takes its
				// default date; a test library has such a date.
				{"Date at the epoch", "a.ZDATECREATED IS NULL OR a.ZDATECREATED = -978307200 " +
					"OR a.ZDATECREATED > 252423993599 OR a.ZDATECREATED < -63113904000",
					func(p *photoInfo) bool { return p.Date.Equal(epoch) }},
				{"DateAdded nil", "a.ZADDEDDATE IS NULL", func(p *photoInfo) bool { return p.DateAdded == nil }},
				{"DateTrashed", "a.ZTRASHEDDATE IS NOT NULL", func(p *photoInfo) bool { return p.DateTrashed != nil }},
				{"Title", "x.ZTITLE != ''", func(p *photoInfo) bool { return p.Title != "" }},
			}
			for _, c := range counts {
				goCount := 0
				for _, p := range photos {
					if c.goVal(p) {
						goCount++
					}
				}
				if want := count(t, src, "SELECT COUNT(*)"+from+" WHERE "+c.where); goCount != want {
					t.Errorf("%s: %d photos, want %d", c.name, goCount, want)
				}
			}
			sums := []struct {
				name, column string
				goVal        func(p *photoInfo) int
			}{
				{"Width", "a.ZWIDTH", func(p *photoInfo) int { return p.Width }},
				{"Height", "a.ZHEIGHT", func(p *photoInfo) int { return p.Height }},
				{"Orientation", "a.ZORIENTATION", func(p *photoInfo) int { return p.Orientation }},
				{"OriginalWidth", "x.ZORIGINALWIDTH", func(p *photoInfo) int { return p.OriginalWidth }},
				{"OriginalHeight", "x.ZORIGINALHEIGHT", func(p *photoInfo) int { return p.OriginalHeight }},
				{"BurstPickType", "a.ZAVALANCHEPICKTYPE", func(p *photoInfo) int { return p.BurstPickType }},
				{"Keywords", "(SELECT COUNT(*) FROM Z_1KEYWORDS j JOIN ZKEYWORD k ON k.Z_PK = j." + n.KeywordColumn +
					" WHERE j.Z_1ASSETATTRIBUTES = x.Z_PK)", func(p *photoInfo) int { return len(p.Keywords) }},
				{"AlbumUUIDs", "(SELECT COUNT(DISTINCT g.ZUUID) FROM " + n.MemberTable + " m JOIN ZGENERICALBUM g ON g.Z_PK = m." +
					n.MemberAlbumColumn + " WHERE g.ZKIND IN (2, 1505) AND m." + n.MemberAssetColumn + " = a.Z_PK)",
					func(p *photoInfo) int { return len(p.AlbumUUIDs) }},
				{"Resources", "(SELECT COUNT(*) FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK)",
					func(p *photoInfo) int { return len(p.Resources) }},
			}
			for _, s := range sums {
				goSum := 0
				for _, p := range photos {
					goSum += s.goVal(p)
				}
				want := count(t, src, "SELECT COALESCE(SUM("+s.column+"), 0)"+from)
				if s.name == "AlbumUUIDs" { // and the CollectionShare of an asset, Photos 11 and later (photosdb.py:2832-2837)
					want += collectionShares(t, src, shareMemberCount)
				}
				if goSum != want {
					t.Errorf("sum of %s: %d, want %d", s.name, goSum, want)
				}
			}
		})
	}
}
