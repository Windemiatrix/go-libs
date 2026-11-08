package photosdb

import (
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// An id added to templates.txt is supported until it is placed here.

// excludedIDs are never compared: Render must give the error named here.
// path.suffixes and path.parts render the repr of a Python list and tuple
// upstream, which Render does not mimic (template.TestRenderGoldenPythonRepr).
var excludedIDs = map[string]error{"t287": template.ErrUnsupportedConstruct, "t289": template.ErrUnsupportedConstruct}

// pendingIDs render ErrUnsupportedField for every photo until their field is
// ported; a pending id that renders is an error. Empty: every id of
// templates.txt renders.
var pendingIDs = map[string][]string{}

// vacuousIDs are supported ids whose value is "_" for every photo of the test
// libraries. They are compared all the same; the list keeps a new vacuous id
// from passing unnoticed. The test libraries hold no files, so uti_raw, read
// from the RAW file, is None (t414, photoinfo.py:940-946); the photos compared
// are outside the trash, so they have no date_trashed (t443) and intrash is
// false (t454); no photo was shared through Messages, so syndicated and
// saved_to_library are false (t473, t474, photoinfo.py:1569-1599); flagged is
// false on Photos (t478, photoinfo.py:817-820); the search databases of the
// test libraries hold no venue terms (t506, t507).
var vacuousIDs = []string{"t014", "t042", "t056", "t096", "t097", "t099", "t144", "t209", "t240", "t340", "t343", "t414", "t443", "t454", "t473", "t474", "t478", "t506", "t507"}

type templateIDs struct {
	parsed                       map[string]*template.Template
	supported, pending, excluded []string
}

func templateSets(t *testing.T) templateIDs {
	t.Helper()
	lines, err := golden.TemplateLines(testlib.RepoDir(t, "golden"))
	if err != nil {
		t.Fatal(err)
	}
	sets := templateIDs{parsed: make(map[string]*template.Template, len(lines)), excluded: slices.Sorted(maps.Keys(excludedIDs))}
	for id, line := range lines {
		if sets.parsed[id], err = template.Parse(line); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	for _, ids := range pendingIDs {
		sets.pending = append(sets.pending, ids...)
	}
	slices.Sort(sets.pending)
	taken := map[string]bool{}
	for _, id := range slices.Concat(sets.excluded, sets.pending) {
		if _, ok := lines[id]; !ok {
			t.Fatalf("%s is not an id of the %d templates of templates.txt", id, len(lines))
		}
		if taken[id] {
			t.Fatalf("%s is in more than one set", id)
		}
		taken[id] = true
	}
	for _, id := range vacuousIDs {
		if _, ok := lines[id]; !ok || taken[id] {
			t.Fatalf("%s of vacuousIDs is not a supported id of the %d templates of templates.txt", id, len(lines))
		}
	}
	if !slices.IsSorted(vacuousIDs) {
		t.Fatal("vacuousIDs is not sorted")
	}
	for _, id := range slices.Sorted(maps.Keys(lines)) {
		if !taken[id] {
			sets.supported = append(sets.supported, id)
		}
	}
	return sets
}

// The messages carry ids and counts, never a value, a UUID or the text of a
// template, so that the test can run on a library of the owner.
func TestTemplateGolden(t *testing.T) {
	t.Parallel()
	sets := templateSets(t)
	dir := testlib.RepoDir(t, "golden")
	var mu sync.Mutex
	live := map[string]bool{}
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				here := compareTemplates(t, lib, dir, sets)
				mu.Lock()
				maps.Copy(live, here)
				mu.Unlock()
			})
		}
	})
	var never []string
	for _, id := range sets.supported {
		if !live[id] {
			never = append(never, id)
		}
	}
	if !slices.Equal(never, vacuousIDs) {
		t.Errorf("ids whose value is _ for every photo of every library: %v, want %v", never, vacuousIDs)
	}
}

func compareTemplates(t *testing.T, lib goldenLibrary, dir string, sets templateIDs) map[string]bool {
	t.Helper()
	db := tmplOpen(t, lib.path)
	oracle, err := golden.Templates(dir, lib.name)
	if err != nil {
		t.Fatal(err)
	}
	// templates.json is "osxphotos query" without --deleted (scripts/golden-build.sh),
	// the default selection of PhotosDB.query (cli/query.py:120,132).
	photos := db.Query(query.Options{})
	missing := 0
	for _, p := range photos {
		if _, ok := oracle[p.UUID]; !ok {
			missing++
		}
	}
	if len(photos) == 0 || missing > 0 || len(photos) != len(oracle) {
		t.Fatalf("%s: %d photos outside the trash, %d of them not in the reference of %d photos", lib.name, len(photos), missing, len(oracle))
	}

	live := map[string]bool{}
	for _, id := range sets.supported {
		diffs := 0
		for _, p := range photos {
			values, unmatched, err := db.RenderTemplate(p, sets.parsed[id], template.RenderOptions{})
			want, ok := oracle[p.UUID][id]
			if err != nil || unmatched != nil || !ok || !slices.Equal(values, want) {
				diffs++
			} else if !slices.Equal(values, []string{"_"}) {
				live[id] = true
			}
		}
		if diffs > 0 {
			t.Errorf("%s: %s differs for %d of %d photos", lib.name, id, diffs, len(photos))
		}
	}
	for _, id := range sets.pending {
		if bad := countNot(db, photos, sets.parsed[id], template.ErrUnsupportedField); bad > 0 {
			t.Errorf("%s: %s renders for %d of %d photos: move it to the supported set", lib.name, id, bad, len(photos))
		}
	}
	for _, id := range sets.excluded {
		if bad := countNot(db, photos, sets.parsed[id], excludedIDs[id]); bad > 0 {
			t.Errorf("%s: %s gives no %v for %d of %d photos", lib.name, id, excludedIDs[id], bad, len(photos))
		}
	}
	t.Logf("%s: %d photos, %d supported ids compared, %d of them live", lib.name, len(photos), len(sets.supported), len(live))
	return live
}

func countNot(db *DB, photos []*photoinfo.PhotoInfo, tmpl *template.Template, want error) int {
	bad := 0
	for _, p := range photos {
		if _, _, err := db.RenderTemplate(p, tmpl, template.RenderOptions{}); !errors.Is(err, want) {
			bad++
		}
	}
	return bad
}
