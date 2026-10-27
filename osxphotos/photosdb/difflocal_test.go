//go:build difflocal

package photosdb

import (
	"flag"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

var (
	diffLocalLibrary = flag.String("library", "", "the library that osxphotos read")
	diffLocalQuery   = flag.String("json", "", "the output of osxphotos query --json")
	diffLocalDeleted = flag.String("deleted", "", "the output of osxphotos query --json --deleted-only")
	diffLocalReport  = flag.String("report", "", "the file for the details, with UUIDs and values; none when empty")
)

// TestDiffLocal is run by "make diff-local". The library is the owner's: the
// log holds numbers and key names only, the details go to -report.
func TestDiffLocal(t *testing.T) {
	// The test runs in the package directory: a relative -report would put the
	// owner's details into the working tree of the repository.
	ok := *diffLocalReport == "" || filepath.IsAbs(*diffLocalReport)
	for _, p := range []string{*diffLocalLibrary, *diffLocalQuery, *diffLocalDeleted} {
		ok = ok && filepath.IsAbs(p)
	}
	if !ok {
		t.Fatal("-library, -json and -deleted are required; every path, -report included, is absolute")
	}
	divs, err := golden.LoadDivergences(filepath.Join(testlib.RepoDir(t, "golden"), "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := golden.LoadFiles("local", *diffLocalQuery, *diffLocalDeleted, slices.Concat(golden.PhotoKeys(), golden.AlbumKeys()))
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(t.Context(), *diffLocalLibrary, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	rep := compareGolden(t, db, ref, divs)
	if *diffLocalReport != "" {
		if err := writeReport(*diffLocalReport, rep); err != nil {
			t.Fatal(err)
		}
	}
	if !rep.OK() {
		t.Error(rep.Summary())
	} else {
		t.Log(rep.Summary())
	}
}
