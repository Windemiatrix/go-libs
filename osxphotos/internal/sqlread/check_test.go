package sqlread

import (
	"database/sql"
	"slices"
	"testing"
)

// TestQuickCheckVirtualTable pins the reason quick_check runs table by table:
// a full-text index whose tokenizer this SQLite does not have (as with the
// Photos app's own one) fails the check of the whole database, while its
// shadow tables, which hold the data, are still checked.
func TestQuickCheckVirtualTable(t *testing.T) {
	t.Parallel()
	path := makeDB(t,
		"CREATE TABLE t(a)",
		"INSERT INTO t VALUES (1)",
		"CREATE VIRTUAL TABLE v USING fts5(a, tokenize = 'ascii')",
		"INSERT INTO v(a) VALUES ('x')",
	)

	db := openWriter(t, path)
	got, err := ordinaryTables(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"t", "v_config", "v_content", "v_data", "v_docsize", "v_idx"}
	if !slices.Equal(got, want) {
		t.Errorf("ordinaryTables %q, want %q (the shadow tables, not the virtual table)", got, want)
	}
	// The tokenizer cannot be named in CREATE VIRTUAL TABLE: fts5 refuses an
	// unknown one, so the schema is edited instead.
	exec(t, db,
		"PRAGMA writable_schema = ON",
		"UPDATE sqlite_master SET sql = replace(sql, 'ascii', 'nosuchtok') WHERE name = 'v'",
		"PRAGMA writable_schema = OFF",
	)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// The fixture tells the two checks apart: the whole database fails.
	if res, err := wholeQuickCheck(t, path); err == nil && res == "ok" {
		t.Fatal("PRAGMA quick_check of the whole database passes: the fixture is vacuous")
	}

	tempDir := t.TempDir()
	d, err := Open(t.Context(), path, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err := d.CheckColumns(t.Context(), map[string][]string{"t": {"a"}, "v_content": {"id"}}); err != nil {
		t.Error(err)
	}
	if n := count(t, d.db, "SELECT COUNT(*) FROM v_content"); n != 1 {
		t.Errorf("v_content has %d rows, want 1", n)
	}
}

// wholeQuickCheck runs PRAGMA quick_check on the whole database at path.
func wholeQuickCheck(t *testing.T, path string) (string, error) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var res string
	err = db.QueryRowContext(t.Context(), "PRAGMA quick_check").Scan(&res)
	return res, err
}
