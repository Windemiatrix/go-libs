package photosdb

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// queryMoreRef is what the date, size and burst options read from an object
// of the reference.
type queryMoreRef struct {
	UUID             string   `json:"uuid"`
	Date             string   `json:"date"`
	OriginalFilesize int64    `json:"original_filesize"`
	Burst            bool     `json:"burst"`
	IsMissing        bool     `json:"ismissing"`
	BurstPhotos      []string `json:"burst_photos"`
}

func queryMoreRefs(t *testing.T, objs []golden.Object) []queryMoreRef {
	t.Helper()
	out := make([]queryMoreRef, len(objs))
	for i, obj := range objs {
		data, err := json.Marshal(obj)
		if err == nil {
			err = json.Unmarshal(data, &out[i])
		}
		if err != nil {
			t.Fatalf("reference element %d: %v", i+1, err)
		}
	}
	return out
}

// queryMoreYear is date.year of Python: the first four digits of the ISO
// string, which is the wall clock of the zone of the date.
func queryMoreYear(t *testing.T, r queryMoreRef) int {
	t.Helper()
	if len(r.Date) < 4 {
		t.Fatalf("reference object %s has no date", r.UUID)
	}
	year, err := strconv.Atoi(r.Date[:4])
	if err != nil {
		t.Fatalf("reference object %s: the date does not start with a year", r.UUID)
	}
	return year
}

type queryMoreCase struct {
	name, family string
	o            query.Options
	want         func(queryMoreRef) bool
}

func queryMoreCases(t *testing.T, refs []queryMoreRef) []queryMoreCase {
	t.Helper()
	year := map[string]int{}
	years, sizes := map[int]bool{}, map[int64]bool{}
	for _, r := range refs {
		year[r.UUID] = queryMoreYear(t, r)
		years[year[r.UUID]] = true
		if r.OriginalFilesize > 0 { // 0 leaves the option unset
			sizes[r.OriginalFilesize] = true
		}
	}

	var cases []queryMoreCase
	every := slices.Sorted(maps.Keys(years))
	for i, y := range every {
		cases = append(cases, queryMoreCase{fmt.Sprintf("year %d", i), "years", query.Options{Year: []int{y}},
			func(r queryMoreRef) bool { return year[r.UUID] == y }})
	}
	cases = append(cases, queryMoreCase{"every year", "years", query.Options{Year: every}, func(queryMoreRef) bool { return true }})

	values := slices.Sorted(maps.Keys(sizes))
	step := max(1, len(values)/20)
	for i := 0; i < len(values); i += step {
		v := values[i]
		cases = append(cases,
			queryMoreCase{fmt.Sprintf("min size %d", i), "sizes", query.Options{MinSize: v},
				func(r queryMoreRef) bool { return r.OriginalFilesize >= v }},
			queryMoreCase{fmt.Sprintf("max size %d", i), "sizes", query.Options{MaxSize: v},
				func(r queryMoreRef) bool { return r.OriginalFilesize <= v }})
	}
	return cases
}

// TestQueryMoreGolden checks Year, MinSize and MaxSize against the values of
// the reference, as TestQueryGolden checks the other options.
func TestQueryMoreGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	libs := goldenLibraries(t)
	var mu sync.Mutex
	expected := map[string]int{}
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libs {
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
				inQuery := queryMoreRefs(t, ref.Query)

				counts := map[string]int{}
				for _, c := range queryMoreCases(t, inQuery) {
					var want []string
					for _, r := range inQuery {
						if c.want(r) {
							want = append(want, r.UUID)
						}
					}
					queryCompare(t, c.name, queryIDs(db.Query(c.o), nil), want)
					counts[c.family] += len(want)
				}
				mu.Lock()
				defer mu.Unlock()
				for f, n := range counts {
					expected[f] += n
				}
			})
		}
	})
	for _, f := range []string{"years", "sizes"} {
		if expected[f] == 0 {
			t.Errorf("family %s: no case of any library expects a photo; the test is vacuous", f)
		}
	}
	t.Logf("expected photos per family, all libraries: %v", expected)
}

// TestQueryMoreBurstPhotos checks that the result of Burst with BurstPhotos
// is the burst photos of query.json and the burst_photos of their objects,
// without the missing ones unless MissingBursts is set. The reference holds
// the selected photos only: for a frame it does not hold, IsMissing is read
// from the library.
func TestQueryMoreBurstPhotos(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	libs := goldenLibraries(t)
	var mu sync.Mutex
	bursts := 0
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libs {
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
				inQuery := queryMoreRefs(t, ref.Query)
				missing := map[string]bool{}
				for _, r := range slices.Concat(inQuery, queryMoreRefs(t, ref.Deleted)) {
					missing[r.UUID] = r.IsMissing
				}
				isMissing := func(uuid string) bool {
					if m, ok := missing[uuid]; ok {
						return m
					}
					p, ok := db.Photo(uuid)
					if !ok {
						t.Fatalf("burst photo %s is not a photo of the library", uuid)
					}
					return p.IsMissing
				}

				var n int
				for _, withMissing := range []bool{false, true} {
					want := map[string]bool{}
					n = 0
					for _, r := range inQuery {
						if !r.Burst {
							continue
						}
						n++
						want[r.UUID] = true
						for _, u := range r.BurstPhotos {
							if withMissing || !isMissing(u) {
								want[u] = true
							}
						}
					}
					o := query.Options{Burst: true, BurstPhotos: true, MissingBursts: withMissing}
					got := slices.Sorted(slices.Values(queryIDs(db.Query(o), nil)))
					queryCompare(t, fmt.Sprintf("missing bursts %t", withMissing), got, slices.Sorted(maps.Keys(want)))
					t.Logf("missing bursts %t: %d burst photos, %d expected", withMissing, n, len(want))
				}
				mu.Lock()
				defer mu.Unlock()
				bursts += n
			})
		}
	})
	if bursts == 0 {
		t.Error("no library has a burst photo in query.json; the test is vacuous")
	}
}
