// Package testlib prepares copies of the test Photos libraries for the tests
// of this module.
//
// The libraries live in testdata/libraries of the repository, outside the
// module, and are copies of the osxphotos test libraries. They are never
// opened for writing: Open copies a database into a temporary directory and
// applies the SQL migrations of that library to the copy (see
// testdata/libraries/SOURCES.md for the rules of a migration file).
package testlib

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"modernc.org/sqlite"
)

// rowsAny on the line right before a statement allows it to change no rows.
const rowsAny = "-- rows: any"

// coreDataFunctions are called by the triggers of Photos.sqlite. They exist
// only inside the Photos app (Core Data); without stand-ins SQLite refuses to
// run a statement that fires such a trigger, for example an UPDATE of
// ZASSET.ZTRASHEDSTATE. The stand-ins do nothing and return NULL.
var coreDataFunctions = []string{
	"NSCoreDataTriggerUpdateAffectedObjectValue",
	"NSCoreDataDATriggerUpdatedAffectedObjectValue",
	"NSCoreDataDATriggerInsertUpdatedAffectedObjectValue",
}

func init() {
	for _, name := range coreDataFunctions {
		sqlite.MustRegisterScalarFunction(name, -1, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			return nil, nil
		})
	}
}

// Libraries returns the names of the test libraries (the directories of
// testdata/libraries that contain database/Photos.sqlite), sorted. It skips
// the test when the directory is not available, for example in the module
// cache.
func Libraries(t testing.TB) []string {
	t.Helper()
	dir := librariesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("testlib: %v", err)
	}
	var names []string
	for _, e := range entries { // os.ReadDir sorts by name
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "database", "Photos.sqlite")); err == nil {
			names = append(names, e.Name())
		}
	}
	return names
}

// Open copies database/Photos.sqlite and, when present, Photos.sqlite-wal of
// the named library into <t.TempDir()>/<name>.photoslibrary/database/, applies
// every migrations/*.sql of the library in file-name order inside one
// transaction, checkpoints the WAL and returns the library path (the
// directory that photosdb.Open takes). Each statement must change at least
// one row unless the line right before it is "-- rows: any". Any failure is
// t.Fatal.
func Open(t testing.TB, name string) string {
	t.Helper()
	lib := OpenPristine(t, name)
	files, err := filepath.Glob(filepath.Join(librariesDir(t), name, "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("testlib: %s: %v", name, err)
	}
	if len(files) == 0 {
		return lib
	}
	slices.Sort(files)
	if err := migrate(t.Context(), filepath.Join(lib, "database", "Photos.sqlite"), files); err != nil {
		t.Fatalf("testlib: %s: %v", name, err)
	}
	return lib
}

// OpenPristine makes the same copy as Open without applying migrations, for
// tests that compare the original state with the migrated one.
func OpenPristine(t testing.TB, name string) string {
	t.Helper()
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		t.Fatalf("testlib: invalid library name %q", name)
	}
	src := filepath.Join(librariesDir(t), name, "database")
	lib := filepath.Join(t.TempDir(), name+".photoslibrary")
	dst := filepath.Join(lib, "database")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatalf("testlib: %v", err)
	}
	for _, file := range []string{"Photos.sqlite", "Photos.sqlite-wal"} {
		err := copyFile(filepath.Join(src, file), filepath.Join(dst, file))
		if file != "Photos.sqlite" && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("testlib: %s: %v", name, err)
		}
	}
	return lib
}

// librariesDir is testdata/libraries of the repository, found relative to
// this source file.
func librariesDir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("testlib: cannot locate the package source")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "libraries")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("testlib: test libraries are not available (%v); they are in the repository, not in the module zip", err)
	}
	return dir
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src) //nolint:gosec // G304: a database file of a test library in testdata
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: a file in t.TempDir()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}

// migrate applies the migration files to the database in one transaction,
// then checkpoints the WAL and checks the database.
func migrate(ctx context.Context, dbPath string, files []string) (err error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	db.SetMaxOpenConns(1)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, file := range files {
		if err := applyFile(ctx, tx, file); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return checkDB(ctx, db)
}

func applyFile(ctx context.Context, tx *sql.Tx, file string) error {
	name := filepath.Base(file)
	src, err := os.ReadFile(file) //nolint:gosec // G304: a migration file of a test library in testdata
	if err != nil {
		return err
	}
	stmts, err := parse(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, st := range stmts {
		res, err := tx.ExecContext(ctx, st.sql)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", name, st.line, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("%s:%d: %w", name, st.line, err)
		}
		if n == 0 && !st.anyRows {
			return fmt.Errorf("%s:%d: the statement changed no rows; the library lacks what the migration expects "+
				"(put %q on the line before a statement that may legitimately match nothing)", name, st.line, rowsAny)
		}
	}
	return nil
}

// checkDB checkpoints the WAL into the main file and runs quick_check on every
// ordinary table. The whole-database quick_check is not used: it fails on a
// virtual table whose module exists but cannot be loaded (fts5 with a
// tokenizer of its own); see sqlread.TestQuickCheckVirtualTable.
func checkDB(ctx context.Context, db *sql.DB) error {
	var busy, walFrames, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &walFrames, &checkpointed); err != nil {
		return fmt.Errorf("wal_checkpoint: %w", err)
	}
	if busy != 0 {
		return errors.New("wal_checkpoint: the database is busy")
	}
	tables, err := ordinaryTables(ctx, db)
	if err != nil {
		return err
	}
	for _, table := range tables {
		var res string
		if err := db.QueryRowContext(ctx, "SELECT quick_check FROM pragma_quick_check(?)", table).Scan(&res); err != nil {
			return fmt.Errorf("quick_check %s: %w", table, err)
		}
		if res != "ok" {
			return fmt.Errorf("quick_check %s: %s", table, res)
		}
	}
	return nil
}

func ordinaryTables(ctx context.Context, db *sql.DB) (names []string, err error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	return names, nil
}

// statement is one SQL statement of a migration file.
type statement struct {
	sql     string
	line    int  // first line of the statement, from 1
	anyRows bool // the line right before it is rowsAny
}

// parse splits a migration file into statements. Lines starting with "--"
// are comments and blank lines are ignored; a statement is the text up to a
// line that ends with ";".
func parse(src string) ([]statement, error) {
	var (
		out     []statement
		cur     []string
		first   int
		anyRows bool
		prev    string
	)
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line != "" && !strings.HasPrefix(line, "--") {
			if len(cur) == 0 {
				first, anyRows = i+1, prev == rowsAny
			}
			cur = append(cur, line)
			if strings.HasSuffix(line, ";") {
				out = append(out, statement{sql: strings.Join(cur, "\n"), line: first, anyRows: anyRows})
				cur = nil
			}
		}
		prev = line
	}
	if len(cur) > 0 {
		return nil, fmt.Errorf("line %d: the statement does not end with a line ending in ';'", first)
	}
	return out, nil
}
