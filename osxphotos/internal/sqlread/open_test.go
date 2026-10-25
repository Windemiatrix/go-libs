package sqlread

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestOpenCopy(t *testing.T) {
	t.Parallel()
	src := syntheticDB(t)
	srcNames := names(t, filepath.Dir(src))
	tempDir := t.TempDir()

	d, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	dir := copyDir(t, tempDir)
	assertFiles(t, dir, "Photos.sqlite")
	if n := count(t, d.db, "SELECT COUNT(*) FROM ZASSET"); n != 1000 {
		t.Errorf("ZASSET has %d rows, want 1000", n)
	}
	for _, stmt := range []string{"CREATE TABLE x(a)", "UPDATE ZASSET SET ZFAVORITE = 1"} {
		if _, err := d.db.ExecContext(t.Context(), stmt); err == nil || !strings.Contains(err.Error(), "readonly") {
			t.Errorf("%s: error %v, want readonly", stmt, err)
		}
	}
	assertFiles(t, dir, "Photos.sqlite") // a failed write leaves no journal
	if n := count(t, d.db, "PRAGMA trusted_schema"); n != 0 {
		t.Errorf("trusted_schema %d, want 0", n)
	}

	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	assertEmpty(t, tempDir)
	if err := d.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if got := names(t, filepath.Dir(src)); !slices.Equal(got, srcNames) {
		t.Errorf("source directory holds %q after Open and Close, want %q", got, srcNames)
	}
}

func TestOpenWAL(t *testing.T) {
	t.Parallel()
	const k = 5
	src := filepath.Join(t.TempDir(), "Photos.sqlite")
	// The writer stays open for the whole test: its close would checkpoint
	// the WAL into the database and remove -wal.
	w := openWriter(t, src)
	var mode string
	if err := w.QueryRowContext(t.Context(), "PRAGMA journal_mode = WAL").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode %q, %v", mode, err)
	}
	exec(t, w,
		"PRAGMA wal_autocheckpoint = 0",
		"PRAGMA cache_size = 10",
		"CREATE TABLE t(i INTEGER PRIMARY KEY, pad BLOB)",
		"PRAGMA wal_checkpoint(TRUNCATE)",
	)
	for range k {
		exec(t, w, "INSERT INTO t(pad) VALUES (randomblob(100))")
	}
	committed := fileSize(t, src+"-wal")

	// An open transaction large enough to spill its pages into -wal past the
	// cache of 10 pages: these frames have no commit mark.
	tx, err := w.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(),
		"WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 500) "+
			"INSERT INTO t(pad) SELECT randomblob(1000) FROM n"); err != nil {
		t.Fatal(err)
	}
	if spilled := fileSize(t, src+"-wal"); spilled <= committed {
		t.Fatalf("-wal is %d bytes after the open transaction, %d after the commits: the uncommitted pages did not spill, the test is vacuous",
			spilled, committed)
	}

	// The writer holds POSIX locks on the database in this process; Open's
	// os.Open of the same file and its close drop them. So this test proves
	// that the copy needs no locks of the source, not how Open behaves next
	// to a writer in another process.
	tempDir := t.TempDir()
	d, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if n := count(t, d.db, "SELECT COUNT(*) FROM t"); n != k {
		t.Errorf("the copy has %d rows, want %d (the committed rows of -wal, not the open transaction)", n, k)
	}
	// SQLite creates -shm of the copy; it gets the mode of the database.
	assertFiles(t, copyDir(t, tempDir), "Photos.sqlite", "Photos.sqlite-shm", "Photos.sqlite-wal")
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// walLibraries are the test libraries with a -wal file (testdata/libraries/SOURCES.md).
var walLibraries = []string{"Test-14.6.0", "Test-15.4.1", "Test-16.0.0-beta", "Test-27.0_DevBeta", "Test-Cloud-13.1"}

// commonColumns exist in every schema of the test libraries.
var commonColumns = map[string][]string{
	"Z_METADATA": {"Z_PLIST", "Z_UUID", "Z_VERSION"},
	"ZASSET":     {"Z_PK", "ZUUID"},
}

func TestOpenLibraries(t *testing.T) {
	t.Parallel()
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(testlib.OpenPristine(t, name), "database", "Photos.sqlite")
			_, err := os.Stat(src + "-wal")
			hasWAL := err == nil
			if want := slices.Contains(walLibraries, name); hasWAL != want {
				t.Fatalf("-wal present: %v, want %v (SOURCES.md)", hasWAL, want)
			}

			tempDir := t.TempDir()
			d, err := Open(t.Context(), src, tempDir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = d.Close() }()
			assertModes(t, copyDir(t, tempDir)) // including -shm created by SQLite
			plist, _, err := d.Metadata(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(plist) == 0 {
				t.Error("Metadata: empty plist")
			}
			if err := d.CheckColumns(t.Context(), commonColumns); err != nil {
				t.Error(err)
			}
			if !hasWAL {
				return
			}
			// The rows of -wal are applied: the counts equal those of another
			// copy whose WAL is checkpointed by a writer.
			got := tableCounts(t, d.db)
			w := openWriter(t, filepath.Join(testlib.OpenPristine(t, name), "database", "Photos.sqlite"))
			var busy, frames, done int
			// PASSIVE reports the frames of the log; TRUNCATE would report 0.
			if err := w.QueryRowContext(t.Context(), "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &frames, &done); err != nil || busy != 0 {
				t.Fatalf("wal_checkpoint: busy %d, %v", busy, err)
			}
			if frames <= 0 || done != frames {
				t.Fatalf("wal_checkpoint: %d frames, %d checkpointed; want all of a non-empty log", frames, done)
			}
			if err := w.QueryRowContext(t.Context(), "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &done); err != nil || busy != 0 {
				t.Fatalf("wal_checkpoint: busy %d, %v", busy, err)
			}
			if want := tableCounts(t, w); !maps.Equal(got, want) {
				t.Errorf("row counts of the copy differ from the checkpointed database:\n got %v\nwant %v", got, want)
			}
		})
	}
}

func TestOpenCorrupt(t *testing.T) {
	t.Parallel()
	src := pagedDB(t)
	corruptMiddlePage(t, src)
	before := sha(t, src)
	tempDir := t.TempDir()

	d, err := Open(t.Context(), src, tempDir)
	if err == nil {
		_ = d.Close()
		t.Fatal("Open of a corrupt database succeeded")
	}
	if !strings.Contains(err.Error(), "quick_check") || !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("error %q, want quick_check and 3 attempts", err)
	}
	assertEmpty(t, tempDir)
	if sha(t, src) != before {
		t.Error("the source changed")
	}
}

// TestOpenRetry damages chosen attempts through afterCopy; it sets a package
// variable, so it is not parallel.
func TestOpenRetry(t *testing.T) {
	t.Cleanup(func() { afterCopy = nil })
	for _, tc := range []struct {
		name    string
		damage  []int
		attempt int // attempts made
		ok      bool
	}{
		{"first", []int{1}, 2, true},
		{"first two", []int{1, 2}, 3, true},
		{"all", []int{1, 2, 3}, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := pagedDB(t)
			tempDir := t.TempDir()
			var paths []string
			afterCopy = func(attempt int, copyPath string) {
				if attempt != len(paths)+1 {
					t.Errorf("attempt %d, want %d", attempt, len(paths)+1)
				}
				// The copy of a failed attempt is gone before the next one.
				if n := names(t, tempDir); len(n) != 1 || n[0] != filepath.Base(filepath.Dir(copyPath)) {
					t.Errorf("attempt %d: %s holds %q, want only the current copy", attempt, tempDir, n)
				}
				paths = append(paths, copyPath)
				if slices.Contains(tc.damage, attempt) {
					corruptMiddlePage(t, copyPath)
				}
			}
			d, err := Open(t.Context(), src, tempDir)
			afterCopy = nil
			if len(paths) != tc.attempt {
				t.Errorf("%d attempts, want %d", len(paths), tc.attempt)
			}
			if !tc.ok {
				if err == nil || !strings.Contains(err.Error(), "3 attempts") {
					t.Errorf("error %v, want 3 attempts", err)
				}
				assertEmpty(t, tempDir)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			copyDir(t, tempDir) // the damaged copies are gone
			if n := count(t, d.db, "SELECT COUNT(*) FROM t"); n != 2000 {
				t.Errorf("%d rows, want 2000", n)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			assertEmpty(t, tempDir)
		})
	}
}

// TestOpenCanceledDuringCheck cancels the context after the first copy: Open
// returns the context error at once instead of retrying. It sets afterCopy,
// so it is not parallel.
func TestOpenCanceledDuringCheck(t *testing.T) {
	t.Cleanup(func() { afterCopy = nil })
	src := pagedDB(t)
	tempDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	afterCopy = func(int, string) {
		calls++
		cancel()
	}
	d, err := Open(ctx, src, tempDir)
	afterCopy = nil
	if err == nil {
		_ = d.Close()
		t.Fatal("Open succeeded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("%d attempts, want 1", calls)
	}
	assertEmpty(t, tempDir)
}

func TestSourceUnchanged(t *testing.T) {
	t.Parallel()
	lib := testlib.OpenPristine(t, "Test-Cloud-13.1")
	dir := filepath.Join(lib, "database")
	src := filepath.Join(dir, "Photos.sqlite")
	if err := os.WriteFile(src+"-shm", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	for _, n := range names(t, dir) {
		if err := os.Chmod(filepath.Join(dir, n), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // G302: a read-only directory in t.TempDir() is the point of the test
		t.Fatal(err)
	}
	// Registered after OpenPristine and t.TempDir, so it runs before their
	// RemoveAll, which cannot remove a directory without write permission.
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700) //nolint:gosec // G302: restores the test directory for its removal
	})
	before := snapshot(t, dir)
	if len(before) != 3 {
		t.Fatalf("source directory holds %d files, want Photos.sqlite, -shm and -wal", len(before))
	}

	d, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Metadata(t.Context()); err != nil {
		t.Error(err)
	}
	if err := d.CheckColumns(t.Context(), commonColumns); err != nil {
		t.Error(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, dir); !maps.Equal(after, before) {
		t.Errorf("source directory changed:\n got %v\nwant %v", after, before)
	}
}

func TestOpenSweeps(t *testing.T) {
	t.Parallel()
	src := syntheticDB(t)
	tempDir := t.TempDir()
	orphan := filepath.Join(tempDir, "osxphotos-sqlread-1073741824-x")
	if err := os.Mkdir(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "Photos.sqlite"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	d1, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d1.Close() }()
	first := copyDir(t, tempDir) // the orphan is gone, the own copy is there

	// Copies of this process are never swept.
	d2, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d2.Close() }()
	d3, err := Open(t.Context(), src, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d3.Close() }()
	if n := names(t, tempDir); len(n) != 3 {
		t.Fatalf("%q, want three copies", n)
	}
	if err := d1.Close(); err != nil {
		t.Fatal(err)
	}
	if n := names(t, tempDir); len(n) != 2 || slices.Contains(n, filepath.Base(first)) {
		t.Fatalf("%q after closing %s, want the two other copies", n, filepath.Base(first))
	}
	for _, d := range []*DB{d2, d3} {
		if n := count(t, d.db, "SELECT COUNT(*) FROM ZASSET"); n != 1000 {
			t.Errorf("%d assets, want 1000", n)
		}
	}
}

func TestOpenPathChars(t *testing.T) {
	t.Parallel()
	src := syntheticDB(t)
	for _, name := range []string{"a b#c?d%e", "x?mode=rwc&y"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tempDir := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(tempDir, 0o700); err != nil {
				t.Fatal(err)
			}
			d, err := Open(t.Context(), src, tempDir)
			if err != nil {
				t.Fatal(err)
			}
			if n := count(t, d.db, "SELECT COUNT(*) FROM ZASSET"); n != 1000 {
				t.Errorf("%d assets, want 1000", n)
			}
			if _, err := d.db.ExecContext(t.Context(), "CREATE TABLE x(a)"); err == nil || !strings.Contains(err.Error(), "readonly") {
				t.Errorf("CREATE TABLE: error %v, want readonly", err)
			}
			assertFiles(t, copyDir(t, tempDir), "Photos.sqlite")
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			assertEmpty(t, tempDir)
		})
	}
}

func TestOpenErrors(t *testing.T) {
	t.Parallel()
	src := syntheticDB(t)
	dbDir := filepath.Dir(src)
	srcSum := sha(t, src)
	srcNames := names(t, dbDir)

	text := filepath.Join(t.TempDir(), "Photos.sqlite")
	if err := os.WriteFile(text, []byte(strings.Repeat("not a database\n", 70)), 0o600); err != nil {
		t.Fatal(err)
	}
	walDir := makeDB(t, "CREATE TABLE t(a)")
	if err := os.Mkdir(walDir+"-wal", 0o700); err != nil {
		t.Fatal(err)
	}
	walUnreadable := makeDB(t, "CREATE TABLE t(a)")
	if err := os.WriteFile(walUnreadable+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(walUnreadable+"-wal", 0); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		src     string
		tempDir string // "" is a new t.TempDir()
		want    string
		is      error
	}{
		{name: "missing source", src: filepath.Join(t.TempDir(), "Photos.sqlite"), is: os.ErrNotExist},
		{name: "directory source", src: t.TempDir(), want: "is a directory"},
		{name: "not a database", src: text, want: "quick_check failed after 3 attempts"},
		{name: "wal is a directory", src: walDir, want: "is a directory"},
		{name: "wal unreadable", src: walUnreadable, is: os.ErrPermission},
		{name: "missing temporary directory", src: src, tempDir: filepath.Join(t.TempDir(), "none"), is: os.ErrNotExist},
		{name: "temporary directory is a file", src: src, tempDir: text, want: "not a directory"},
		{name: "canceled", ctx: canceled, src: src, is: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.name == "wal unreadable" && os.Geteuid() == 0 {
				t.Skip("root reads a file without permissions")
			}
			ctx := tc.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			tempDir := tc.tempDir
			if tempDir == "" {
				tempDir = t.TempDir()
			}
			d, err := Open(ctx, tc.src, tempDir)
			if err == nil {
				_ = d.Close()
				t.Fatal("Open succeeded")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q, want %q", err, tc.want)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("error %v, want errors.Is %v", err, tc.is)
			}
			if tc.tempDir == "" {
				assertEmpty(t, tempDir)
			}
		})
	}

	t.Cleanup(func() {
		if got := names(t, dbDir); !slices.Equal(got, srcNames) {
			t.Errorf("the database directory holds %q, want %q", got, srcNames)
		}
		if sha(t, src) != srcSum {
			t.Error("the source changed")
		}
	})
}

// TestOpenDefaultTempDir covers tempDir "": the copy goes to os.TempDir().
func TestOpenDefaultTempDir(t *testing.T) {
	t.Parallel()
	d, err := Open(t.Context(), syntheticDB(t), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := d.dir
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != tmp {
		t.Errorf("copy directory %s, want one in %s", dir, tmp)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s after Close: %v, want not exist", dir, err)
	}
}
