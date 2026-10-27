// Package testlib prepares copies of the test Photos libraries of
// testdata/libraries, which is in the repository but outside the module.
//
// The libraries are never opened for writing: Open copies a database into a
// temporary directory and applies the SQL files of that library to the copy:
// reference/ (seen by osxphotos too) and then migrations/ (Go tests only); see
// testdata/libraries/SOURCES.md for the rules of these files.
package testlib

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"modernc.org/sqlite"
)

const rowsAny = "-- rows: any"

// The triggers of Photos.sqlite call coreDataFunctions, which exist only inside
// the Photos app (Core Data); without stand-ins SQLite refuses to run a
// statement that fires such a trigger, for example an UPDATE of
// ZASSET.ZTRASHEDSTATE.
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
	dir := RepoDir(t, "libraries")
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

// Open copies the named library as OpenPristine does, applies every
// migrations/*.sql of the library in file-name order inside one transaction and
// returns the library path (the directory that photosdb.Open takes). Each
// statement must change at least one row unless the line right before it is
// "-- rows: any". Any failure is t.Fatal.
func Open(t testing.TB, name string) string {
	t.Helper()
	lib := OpenPristine(t, name)
	applyDir(t, lib, name, "migrations")
	return lib
}

// OpenPristine copies database/Photos.sqlite and, when present,
// Photos.sqlite-wal of the named library into
// <t.TempDir()>/<name>.photoslibrary/database/ and returns the library path
// (the directory that photosdb.Open takes), without migrations, for tests that
// compare the original state with the migrated one.
// It does apply reference/*.sql of the library: those statements are part of
// the library, and scripts/golden-build.sh applies them too before it runs osxphotos.
// Any failure is t.Fatal.
func OpenPristine(t testing.TB, name string) string {
	t.Helper()
	lib := copyLibrary(t, name)
	applyDir(t, lib, name, "reference")
	return lib
}

func applyDir(t testing.TB, lib, name, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(RepoDir(t, "libraries"), name, dir, "*.sql"))
	if err != nil {
		t.Fatalf("testlib: %s: %v", name, err)
	}
	if len(files) == 0 {
		return
	}
	slices.Sort(files)
	if err := migrate(t.Context(), filepath.Join(lib, "database", "Photos.sqlite"), files); err != nil {
		t.Fatalf("testlib: %s: %v", name, err)
	}
}

func copyLibrary(t testing.TB, name string) string {
	t.Helper()
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		t.Fatalf("testlib: invalid library name %q", name)
	}
	src := filepath.Join(RepoDir(t, "libraries"), name, "database")
	lib := filepath.Join(t.TempDir(), name+".photoslibrary")
	dst := filepath.Join(lib, "database")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatalf("testlib: %v", err)
	}
	for _, file := range []string{"Photos.sqlite", "Photos.sqlite-wal"} {
		data, err := os.ReadFile(filepath.Join(src, file)) //nolint:gosec // G304: a database file of a test library in testdata
		if file != "Photos.sqlite" && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err == nil {
			err = os.WriteFile(filepath.Join(dst, file), data, 0o600) //nolint:gosec // G703: dst is the test's temporary directory, file one of two fixed names
		}
		if err != nil {
			t.Fatalf("testlib: %s: %v", name, err)
		}
	}
	return lib
}

// Synthetic returns a copy of the synthetic library, made as OpenPristine
// makes it, that holds copies times its assets: every further copy of an
// asset has its own Z_PK and ZUUID and its own rows in
// ZADDITIONALASSETATTRIBUTES, ZCLOUDMASTER and ZINTERNALRESOURCE. Any failure
// is t.Fatal.
func Synthetic(t testing.TB, copies int) string {
	t.Helper()
	lib := OpenPristine(t, "synthetic")
	if err := grow(t.Context(), filepath.Join(lib, "database", "Photos.sqlite"), copies); err != nil {
		t.Fatalf("testlib: synthetic: %v", err)
	}
	return lib
}

func grow(ctx context.Context, dbPath string, copies int) error {
	return inTx(ctx, dbPath, func(tx *sql.Tx) error {
		var asset, attrs, master, resource int64
		if err := tx.QueryRowContext(ctx, "SELECT (SELECT MAX(Z_PK) FROM ZASSET), (SELECT MAX(Z_PK) FROM ZADDITIONALASSETATTRIBUTES), "+
			"(SELECT MAX(Z_PK) FROM ZCLOUDMASTER), (SELECT MAX(Z_PK) FROM ZINTERNALRESOURCE)").Scan(&asset, &attrs, &master, &resource); err != nil {
			return err
		}
		stmts := []string{
			"CREATE TEMP TABLE a AS SELECT * FROM ZASSET",
			"CREATE TEMP TABLE x AS SELECT * FROM ZADDITIONALASSETATTRIBUTES",
			"CREATE TEMP TABLE m AS SELECT * FROM ZCLOUDMASTER",
			"CREATE TEMP TABLE r AS SELECT * FROM ZINTERNALRESOURCE",
		}
		for i := 1; i < copies; i++ {
			stmts = append(stmts,
				fmt.Sprintf("UPDATE temp.a SET Z_PK = Z_PK + %d, ZUUID = printf('%%08X-0000-0000-0000-%%012X', %d, Z_PK), "+
					"ZADDITIONALATTRIBUTES = ZADDITIONALATTRIBUTES + %d, ZMASTER = ZMASTER + %d", asset, i, attrs, master),
				fmt.Sprintf("UPDATE temp.x SET Z_PK = Z_PK + %d, ZASSET = ZASSET + %d", attrs, asset),
				fmt.Sprintf("UPDATE temp.m SET Z_PK = Z_PK + %d", master),
				fmt.Sprintf("UPDATE temp.r SET Z_PK = Z_PK + %d, ZASSET = ZASSET + %d", resource, asset),
				"INSERT INTO ZASSET SELECT * FROM temp.a",
				"INSERT INTO ZADDITIONALASSETATTRIBUTES SELECT * FROM temp.x",
				"INSERT INTO ZCLOUDMASTER SELECT * FROM temp.m",
				"INSERT INTO ZINTERNALRESOURCE SELECT * FROM temp.r",
			)
		}
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	})
}

// RepoDir returns testdata/<name> of the repository, found relative to this
// source file. It skips the test when the directory is not available, for
// example in the module zip.
func RepoDir(t testing.TB, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("testlib: cannot locate the package source")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", name)
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("testlib: testdata/%s is not available (%v); it is in the repository, not in the module zip", name, err)
	}
	return dir
}

// OpenDB opens the database file dbPath for reading and writing and closes it
// at the end of the test.
func OpenDB(t testing.TB, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
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

// Scalar returns the single value of query; NULL is "NULL", no row is "<no row>".
func Scalar(t testing.TB, db *sql.DB, query string) string {
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

// Count returns the single integer value of query.
func Count(t testing.TB, db *sql.DB, query string) int {
	t.Helper()
	s := Scalar(t, db, query)
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("%s: %q is not a number", query, s)
	}
	return n
}

// Rows returns the first column of every row of query.
func Rows(t testing.TB, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query, args...)
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

func migrate(ctx context.Context, dbPath string, files []string) error {
	return inTx(ctx, dbPath, func(tx *sql.Tx) error {
		for _, file := range files {
			if err := applyFile(ctx, tx, file); err != nil {
				return err
			}
		}
		return nil
	})
}

func inTx(ctx context.Context, dbPath string, fn func(*sql.Tx) error) (err error) {
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
	if err := fn(tx); err != nil {
		return err
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

// checkDB runs quick_check per ordinary table, not on the whole database: that
// fails on a virtual table whose module exists but cannot be loaded (fts5 with
// a tokenizer of its own); see sqlread.TestQuickCheckVirtualTable.
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

type statement struct {
	sql     string
	line    int // from 1
	anyRows bool
}

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
