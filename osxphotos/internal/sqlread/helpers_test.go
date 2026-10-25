package sqlread

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// syntheticDB returns the path of the database of a copy of the synthetic
// library, in the rollback journal mode, without a -wal file.
func syntheticDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(testlib.OpenPristine(t, "synthetic"), "database", "Photos.sqlite")
}

// openWriter opens path for writing with one connection: PRAGMA page_size,
// journal_mode, wal_autocheckpoint and cache_size apply to one connection,
// and in a pool a later statement could run on another one.
func openWriter(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.ExecContext(t.Context(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// makeDB creates a database in the rollback journal mode from the statements
// and returns its path.
func makeDB(t *testing.T, stmts ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Photos.sqlite")
	db := openWriter(t, path)
	exec(t, db, stmts...)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// pagedDB is a database in the rollback journal mode with one table t of
// 2000 rows of 100 bytes, about 50 pages of 4096 bytes.
func pagedDB(t *testing.T) string {
	t.Helper()
	return makeDB(t,
		"PRAGMA page_size = 4096",
		"CREATE TABLE t(i INTEGER PRIMARY KEY, pad BLOB)",
		"WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 2000) "+
			"INSERT INTO t SELECT i, randomblob(100) FROM n",
	)
}

// corruptMiddlePage writes 64 bytes 0xFF at the start of the middle page of
// a database of pagedDB. Every page but the first belongs to table t.
func corruptMiddlePage(t *testing.T, path string) {
	t.Helper()
	const pageSize = 4096
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pages := info.Size() / pageSize
	if pages < 3 {
		t.Fatalf("%s: %d pages, want at least 3", path, pages)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	ff := make([]byte, 64)
	for i := range ff {
		ff[i] = 0xFF
	}
	if _, err := f.WriteAt(ff, pages/2*pageSize); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func sha(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// names returns the names of the entries of dir, sorted.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	if n := names(t, dir); len(n) != 0 {
		t.Errorf("%s holds %q, want nothing", dir, n)
	}
}

// copyDir returns the only entry of tempDir, which must be a copy directory
// of this process with mode 0700.
func copyDir(t *testing.T, tempDir string) string {
	t.Helper()
	n := names(t, tempDir)
	if len(n) != 1 {
		t.Fatalf("%s holds %q, want one copy directory", tempDir, n)
	}
	if prefix := fmt.Sprintf("osxphotos-sqlread-%d-", os.Getpid()); !strings.HasPrefix(n[0], prefix) {
		t.Fatalf("copy directory %q, want prefix %q", n[0], prefix)
	}
	dir := filepath.Join(tempDir, n[0])
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("%s: mode %v, want a directory with 0700", dir, info.Mode())
	}
	return dir
}

// assertFiles checks that dir holds exactly the regular files want (sorted),
// each with mode 0600.
func assertFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	if got := names(t, dir); !slices.Equal(got, want) {
		t.Errorf("%s holds %q, want %q", dir, got, want)
	}
	assertModes(t, dir)
}

// assertModes checks that every entry of dir is a regular file with 0600.
func assertModes(t *testing.T, dir string) {
	t.Helper()
	for _, n := range names(t, dir) {
		info, err := os.Lstat(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want a regular file with 0600", n, info.Mode())
		}
	}
}

// fileState is what TestSourceUnchanged compares for a file.
type fileState struct {
	size  int64
	mode  fs.FileMode
	mtime time.Time
	sum   [sha256.Size]byte
}

func snapshot(t *testing.T, dir string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	for _, n := range names(t, dir) {
		p := filepath.Join(dir, n)
		info, err := os.Lstat(p)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = fileState{size: info.Size(), mode: info.Mode(), mtime: info.ModTime(), sum: sha(t, p)}
	}
	return out
}

func count(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// tableCounts returns COUNT(*) of every ordinary table of db.
func tableCounts(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		"SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, n := range tables {
		out[n] = count(t, db, `SELECT COUNT(*) FROM "`+strings.ReplaceAll(n, `"`, `""`)+`"`)
	}
	return out
}
