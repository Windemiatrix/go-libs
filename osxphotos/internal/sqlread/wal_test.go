package sqlread

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	walHeader      = 32 // bytes of the WAL header
	walFrameHeader = 24 // bytes of a frame header
)

// commitEnds returns the offsets right after each commit frame of a WAL with
// the given page size: a frame header holds the database size in pages at
// bytes 4–7, non-zero only in a commit frame.
func commitEnds(t *testing.T, wal []byte, pageSize int) []int {
	t.Helper()
	if len(wal) < walHeader {
		t.Fatalf("-wal is %d bytes, shorter than its header", len(wal))
	}
	frame := walFrameHeader + pageSize
	var ends []int
	off := walHeader
	for ; off+frame <= len(wal); off += frame {
		if binary.BigEndian.Uint32(wal[off+4:off+8]) != 0 {
			ends = append(ends, off+frame)
		}
	}
	if off != len(wal) {
		t.Fatalf("-wal is %d bytes, not a whole number of %d-byte frames after the header", len(wal), frame)
	}
	return ends
}

// TestTruncatedWAL opens a database with every prefix of its -wal: SQLite
// ignores an incomplete tail of the log, so each prefix gives the state after
// the last whole commit in it and never an error.
func TestTruncatedWAL(t *testing.T) {
	t.Parallel()
	const (
		pageSize = 512
		n        = 4 // transactions in -wal
	)
	w := filepath.Join(t.TempDir(), "Photos.sqlite")
	writer := openWriter(t, w)
	var mode string
	exec(t, writer, "PRAGMA page_size = 512")
	if err := writer.QueryRowContext(t.Context(), "PRAGMA journal_mode = WAL").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode %q, %v", mode, err)
	}
	exec(t, writer,
		"PRAGMA wal_autocheckpoint = 0",
		"CREATE TABLE t(i INTEGER)",
		"PRAGMA wal_checkpoint(TRUNCATE)",
	)
	for i := 1; i <= n; i++ {
		if _, err := writer.ExecContext(t.Context(), "INSERT INTO t VALUES (?)", i); err != nil {
			t.Fatal(err)
		}
	}
	db, err := os.ReadFile(w)
	if err != nil {
		t.Fatal(err)
	}
	wal, err := os.ReadFile(w + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ends := commitEnds(t, wal, pageSize)
	if len(ends) != n || ends[n-1] != len(wal) {
		t.Fatalf("commit frames end at %v in a %d-byte -wal, want %d commits ending at its end", ends, len(wal), n)
	}

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "Photos.sqlite")
	tempDir := t.TempDir()
	seen := map[int]bool{}
	start := time.Now()
	opens := 0
	// L = -1 is a source without -wal.
	for l := -1; l <= len(wal); l++ {
		if err := os.WriteFile(src, db, 0o600); err != nil { //nolint:gosec // G703: a file in t.TempDir()
			t.Fatal(err)
		}
		if l < 0 {
			if err := os.Remove(src + "-wal"); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(src+"-wal", wal[:l], 0o600); err != nil { //nolint:gosec // G703: a file in t.TempDir()
			t.Fatal(err)
		}
		want := 0
		for _, e := range ends {
			if e <= l {
				want++
			}
		}
		opens++
		d, err := Open(t.Context(), src, tempDir)
		if err != nil {
			t.Fatalf("L = %d: %v", l, err)
		}
		var rows, maxI int
		err = d.db.QueryRowContext(t.Context(), "SELECT COUNT(*), COALESCE(MAX(i), 0) FROM t").Scan(&rows, &maxI)
		if cerr := d.Close(); cerr != nil {
			t.Fatal(cerr)
		}
		if err != nil {
			t.Fatalf("L = %d: %v", l, err)
		}
		if rows != want || maxI != want {
			t.Fatalf("L = %d: %d rows, MAX(i) %d; want %d and %d (commits end at %v)", l, rows, maxI, want, want, ends)
		}
		seen[rows] = true
	}
	for k := 0; k <= n; k++ {
		if !seen[k] {
			t.Errorf("no prefix of -wal gave %d rows", k)
		}
	}
	assertEmpty(t, tempDir)
	t.Logf("%d opens of %d-byte -wal prefixes in %v, 0 errors", opens, len(wal), time.Since(start))
}
