package photosdb

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/gen"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// goldenLibrary is a library of testdata/golden and the path of a pristine
// copy of it: the osxphotos output was made from the libraries without
// migrations.
type goldenLibrary struct{ name, path string }

func goldenLibraries(t *testing.T) []goldenLibrary {
	t.Helper()
	var libs []goldenLibrary
	for _, name := range testlib.Libraries(t) {
		libs = append(libs, goldenLibrary{name, testlib.OpenPristine(t, name)})
	}
	return append(libs, goldenLibrary{"synthetic", genLibrary(t, gen.DefaultConfig())})
}

// TestGolden compares every photo key of the osxphotos output with what Open
// reads, for the test libraries and the synthetic library.
func TestGolden(t *testing.T) {
	t.Parallel()

	dir := golden.Dir(t)
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
			photos := db.Photos()
			rep, err := golden.Compare(ref, golden.Library{Photos: photos}, golden.PhotoKeys(), divs)
			if err != nil {
				t.Fatal(err)
			}
			// Not vacuous: the objects compared are those that upstream
			// photos() returns.
			selected := 0
			for _, p := range photos {
				if !p.BurstUnselected() && (p.IsPhoto || p.IsMovie) {
					selected++
				}
			}
			if rep.Objects == 0 || selected != rep.Objects {
				t.Errorf("%d reference objects, %d selected Go photos", rep.Objects, selected)
			}
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

// rawPairs is the number of RAW+JPEG pairs of gen.DefaultConfig(): 4% of
// 1000 assets, fixed by TestPinnedDefault of internal/gen.
const rawPairs = 40

// TestGoldenHasRAW is the independent condition of the has_raw record of the
// synthetic library in divergences.json: without records, the has_raw
// differences are exactly the assets that have a resource of subtype 3 and
// resource type 4 and none of subtype 17, which Go reads as RAW and upstream
// does not (photosdb/photosdb.py:2537-2551). With the records, all of them
// are explained.
func TestGoldenHasRAW(t *testing.T) {
	t.Parallel()

	dir := golden.Dir(t)
	lib := genLibrary(t, gen.DefaultConfig())
	candidates := map[string]bool{}
	rows, err := roDB(t, lib).QueryContext(t.Context(), "SELECT a.ZUUID FROM ZASSET a WHERE "+
		"EXISTS (SELECT 1 FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 3 AND r.ZRESOURCETYPE = 4) "+
		"AND NOT EXISTS (SELECT 1 FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 17)")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		candidates[u] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if len(candidates) != rawPairs {
		t.Fatalf("%d assets with a RAW resource of subtype 3 only, want %d", len(candidates), rawPairs)
	}

	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := golden.Load(dir, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	glib := golden.Library{Photos: db.Photos()}
	rep, err := golden.Compare(ref, glib, golden.PhotoKeys(), nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range rep.Diffs {
		if d.Field != "has_raw" {
			continue
		}
		n++
		if !candidates[d.UUID] || d.Want != "false" || d.Got != "true" {
			t.Errorf("has_raw difference %s (want %s, got %s) is not a subtype 3 RAW pair", d.UUID, d.Want, d.Got)
		}
	}
	if n != len(candidates) {
		t.Errorf("%d has_raw differences, want %d", n, len(candidates))
	}

	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(divs, func(d golden.Divergence) bool {
		return d.Library == "synthetic" && d.UUID == "*" && d.Field == "has_raw"
	}) {
		t.Fatal("divergences.json has no has_raw '*' record for the synthetic library")
	}
	rep, err = golden.Compare(ref, glib, golden.PhotoKeys(), divs)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Explained["has_raw"] != rawPairs {
		t.Errorf("%d has_raw differences explained, want %d", rep.Explained["has_raw"], rawPairs)
	}
}

// TestGoldenOrder checks the order of Photos against the order of the
// osxphotos output (photoquery.py:888-898): the UUIDs of Photos that are in
// query.json, and those in deleted.json, come in the order of the file.
func TestGoldenOrder(t *testing.T) {
	t.Parallel()

	dir := golden.Dir(t)
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
