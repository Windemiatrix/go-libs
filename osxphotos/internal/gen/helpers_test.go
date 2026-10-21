package gen

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	// The init of testlib registers stand-ins of the Core Data functions that
	// the triggers of Photos.sqlite call; a few corruption cases fire them.
	_ "github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// generate writes a library for cfg into a new temporary directory and
// returns the library directory and the open database.
func generate(t testing.TB, cfg Config) (string, *sql.DB) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Synthetic.photoslibrary")
	if err := Generate(t.Context(), dir, cfg); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return dir, openDB(t, dbPath(dir))
}

func dbPath(dir string) string {
	return filepath.Join(dir, "database", "Photos.sqlite")
}

func openDB(t testing.TB, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// scalar returns the single value of query; NULL is "NULL", no row is "<no row>".
func scalar(t testing.TB, db *sql.DB, query string) string {
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

func count(t testing.TB, db *sql.DB, query string) int64 {
	t.Helper()
	s := scalar(t, db, query)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("%s: %q is not a number", query, s)
	}
	return n
}

func exec(t testing.TB, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

func rowStrings(t testing.TB, db *sql.DB, query string) []string {
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

// check is a query that returns one value; NULL is rendered as "NULL".
type check struct {
	name  string
	query string
	want  string
}

func runChecks(t *testing.T, db *sql.DB, checks []check) {
	t.Helper()
	for _, c := range checks {
		if got := scalar(t, db, c.query); got != c.want {
			t.Errorf("%s: %s\n got %q\nwant %q", c.name, c.query, got, c.want)
		}
	}
}

// zeroConfig is DefaultConfig with every share and count set to zero.
func zeroConfig(assets int) Config {
	return Config{
		Schema:          DefaultSchema,
		Assets:          assets,
		Seed:            1,
		AssetsPerAlbum:  40,
		AlbumsPerFolder: 25,
	}
}
