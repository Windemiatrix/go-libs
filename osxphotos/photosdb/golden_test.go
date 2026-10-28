package photosdb

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// The path is a pristine copy: the osxphotos output was made from the
// libraries without migrations.
type goldenLibrary struct{ name, path string }

func goldenLibraries(t *testing.T) []goldenLibrary {
	t.Helper()
	var libs []goldenLibrary
	for _, name := range testlib.Libraries(t) {
		libs = append(libs, goldenLibrary{name, testlib.OpenPristine(t, name)})
	}
	return libs
}

func TestGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := golden.Load(dir, lib.name)
			if err != nil {
				t.Fatal(err)
			}
			rep := compareGolden(t, db, ref, divs)
			if !rep.OK() {
				var b strings.Builder
				_ = rep.WriteDetails(&b)
				t.Errorf("%s\n%s", rep.Summary(), b.String())
			} else {
				t.Log(rep.Summary())
			}
		})
	}
}

// Shared by TestGolden and TestDiffLocal (difflocal_test.go, tag difflocal).
func compareGolden(t *testing.T, db *DB, ref *golden.Reference, divs []golden.Divergence) *golden.Report {
	t.Helper()
	photos := db.Photos()
	burstAlbums := map[string][]string{}
	for _, p := range photos {
		burstAlbums[p.UUID] = lookupUUIDs(db.BurstAlbumInfoOf(p))
	}
	rep, err := golden.Compare(ref, golden.Library{
		Photos: photos, Albums: db.albums, SharedAlbums: db.sharedAlbums, BurstAlbums: burstAlbums, Imports: db.imports, Projects: db.projects,
		Comments: db.comments, Likes: db.likes,
	},
		slices.Concat(golden.PhotoKeys(), golden.AlbumKeys()), divs)
	if err != nil {
		t.Fatal(err)
	}
	// Not vacuous: the objects compared are those that upstream
	// photos() returns, in the trash or not.
	selected := len(db.Query(query.Options{Deleted: true}))
	if rep.Objects == 0 || selected != rep.Objects {
		t.Errorf("%d reference objects, %d selected Go photos", rep.Objects, selected)
	}
	return rep
}

// The details hold the UUIDs and values of a library of the owner: the file
// is for the owner alone, also when it existed before with wider permissions.
func writeReport(path string, rep *golden.Report) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	return rep.WriteDetails(f)
}
func TestWriteReport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.txt")
	// Longer than the new report, and 0644 under any umask, so that a missing
	// O_TRUNC leaves a tail and a missing Chmod leaves the mode.
	if err := os.WriteFile(path, []byte(strings.Repeat("an older and longer report\n", 20)), 0o644); err != nil { //nolint:gosec // G306: the wider mode is what the test needs
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // G302: as above
		t.Fatal(err)
	}
	rep := &golden.Report{Diffs: []golden.Diff{{UUID: "00000000-0000-0000-0000-000000000001", Field: "title", Want: `"a"`, Got: `"b"`}}}
	if err := writeReport(path, rep); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	_ = rep.WriteDetails(&want)
	if !strings.Contains(string(got), "00000000-0000-0000-0000-000000000001") || string(got) != want.String() {
		t.Errorf("report %q, want %q", got, want.String())
	}
}

// The order of the osxphotos output: photoquery.py:888-898.
func TestGoldenOrder(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := golden.Load(dir, lib.name)
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []struct {
				file    string
				objects []golden.Object
			}{{"query.json", ref.Query}, {"deleted.json", ref.Deleted}} {
				want := make([]string, 0, len(part.objects))
				in := map[string]bool{}
				for _, o := range part.objects {
					var u string
					if err := json.Unmarshal(o["uuid"], &u); err != nil {
						t.Fatal(err)
					}
					want = append(want, u)
					in[u] = true
				}
				var got []string
				for _, p := range db.Photos() {
					if in[p.UUID] {
						got = append(got, p.UUID)
					}
				}
				if !slices.Equal(got, want) {
					t.Errorf("%s: the photos of Photos come in another order than in the file", part.file)
				}
			}
		})
	}
}
