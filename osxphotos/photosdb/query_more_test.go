package photosdb

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

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
	DateAdded        *string  `json:"date_added"`
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

// queryMoreClock is date.time() of Python in seconds: the wall clock of the
// ISO string of the date, characters 11-19. A threshold of whole seconds
// splits the dates with a fraction of a second as it splits their seconds.
func queryMoreClock(t *testing.T, r queryMoreRef) time.Duration {
	t.Helper()
	if len(r.Date) < 19 {
		t.Fatalf("reference object %s has no date", r.UUID)
	}
	clock, err := time.Parse(time.TimeOnly, r.Date[11:19])
	if err != nil {
		t.Fatalf("reference object %s: the date has no time of day", r.UUID)
	}
	h, m, s := clock.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

// queryMoreAdded is date_added of the reference as Sort reads it: null is
// 1970-01-01T00:00:00Z, the documented deviation from upstream.
func queryMoreAdded(t *testing.T, r queryMoreRef) time.Time {
	t.Helper()
	if r.DateAdded == nil {
		return time.Unix(0, 0).UTC()
	}
	moment, err := time.Parse(time.RFC3339Nano, *r.DateAdded)
	if err != nil {
		t.Fatalf("reference object %s: date_added is not an ISO 8601 string", r.UUID)
	}
	return moment
}

// queryMoreSample keeps about 20 of the sorted values, evenly: a case per
// value would be thousands for a large library.
func queryMoreSample[T any](sorted []T) []T {
	var out []T
	for i := 0; i < len(sorted); i += max(1, len(sorted)/20) {
		out = append(out, sorted[i])
	}
	return out
}

type queryMoreCase struct {
	name, family string
	o            query.Options
	want         func(queryMoreRef) bool
}

func queryMoreCases(t *testing.T, refs []queryMoreRef) []queryMoreCase {
	t.Helper()
	year, clock, added := map[string]int{}, map[string]time.Duration{}, map[string]time.Time{}
	years, sizes, clocks := map[int]bool{}, map[int64]bool{}, map[time.Duration]bool{}
	var moments []time.Time
	for _, r := range refs {
		year[r.UUID], clock[r.UUID], added[r.UUID] = queryMoreYear(t, r), queryMoreClock(t, r), queryMoreAdded(t, r)
		years[year[r.UUID]] = true
		clocks[clock[r.UUID]] = true
		moments = append(moments, added[r.UUID])
		if r.OriginalFilesize > 0 { // 0 leaves the option unset
			sizes[r.OriginalFilesize] = true
		}
	}
	slices.SortFunc(moments, time.Time.Compare)
	moments = slices.CompactFunc(moments, time.Time.Equal)

	var cases []queryMoreCase
	every := slices.Sorted(maps.Keys(years))
	for i, y := range every {
		cases = append(cases, queryMoreCase{fmt.Sprintf("year %d", i), "years", query.Options{Year: []int{y}},
			func(r queryMoreRef) bool { return year[r.UUID] == y }})
	}
	cases = append(cases, queryMoreCase{"every year", "years", query.Options{Year: every}, func(queryMoreRef) bool { return true }})

	for i, v := range queryMoreSample(slices.Sorted(maps.Keys(sizes))) {
		cases = append(cases,
			queryMoreCase{fmt.Sprintf("min size %d", i), "sizes", query.Options{MinSize: v},
				func(r queryMoreRef) bool { return r.OriginalFilesize >= v }},
			queryMoreCase{fmt.Sprintf("max size %d", i), "sizes", query.Options{MaxSize: v},
				func(r queryMoreRef) bool { return r.OriginalFilesize <= v }})
	}
	for i, m := range queryMoreSample(moments) {
		cases = append(cases,
			queryMoreCase{fmt.Sprintf("added after %d", i), "added", query.Options{AddedAfter: m},
				func(r queryMoreRef) bool { return added[r.UUID].After(m) }},
			queryMoreCase{fmt.Sprintf("added before %d", i), "added", query.Options{AddedBefore: m},
				func(r queryMoreRef) bool { return added[r.UUID].Before(m) }})
	}
	for i, c := range queryMoreSample(slices.Sorted(maps.Keys(clocks))) {
		cases = append(cases,
			queryMoreCase{fmt.Sprintf("from time %d", i), "clocks", query.Options{FromTime: &c},
				func(r queryMoreRef) bool { return clock[r.UUID] >= c }},
			queryMoreCase{fmt.Sprintf("to time %d", i), "clocks", query.Options{ToTime: &c},
				func(r queryMoreRef) bool { return clock[r.UUID] < c }})
	}
	return cases
}

// TestQueryMoreGolden checks Year, FromTime, ToTime, AddedAfter, AddedBefore,
// MinSize and MaxSize against the values of the reference, as TestQueryGolden
// checks the other options.
func TestQueryMoreGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
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

				skip := querySkip(divs, lib.name, []string{"date", "original_filesize", "date_added"})
				counts := map[string]int{}
				for _, c := range queryMoreCases(t, inQuery) {
					var want []string
					for _, r := range inQuery {
						if !skip(r.UUID) && c.want(r) {
							want = append(want, r.UUID)
						}
					}
					queryCompare(t, c.name, queryIDs(db.Query(c.o), skip), want)
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
	for _, f := range []string{"years", "sizes", "added", "clocks"} {
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
// from the library under test, so the path that adds a frame because it is not
// missing is held by TestMoreBurstPhotos, not by this test.
func TestQueryMoreBurstPhotos(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	libs := goldenLibraries(t)
	var mu sync.Mutex
	bursts, addedByMissing := 0, 0
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

				skip := querySkip(divs, lib.name, []string{"burst", "burst_photos", "ismissing"})
				var n int
				wantCount := map[bool]int{}
				for _, withMissing := range []bool{false, true} {
					want := map[string]bool{}
					n = 0
					for _, r := range inQuery {
						if !r.Burst || skip(r.UUID) {
							continue
						}
						n++
						want[r.UUID] = true
						for _, u := range r.BurstPhotos {
							if !skip(u) && (withMissing || !isMissing(u)) {
								want[u] = true
							}
						}
					}
					o := query.Options{Burst: true, BurstPhotos: true, MissingBursts: withMissing}
					got := slices.Sorted(slices.Values(queryIDs(db.Query(o), skip)))
					queryCompare(t, fmt.Sprintf("missing bursts %t", withMissing), got, slices.Sorted(maps.Keys(want)))
					t.Logf("missing bursts %t: %d burst objects in query.json, %d photos expected", withMissing, n, len(want))
					wantCount[withMissing] = len(want)
				}
				mu.Lock()
				defer mu.Unlock()
				bursts += n
				addedByMissing += wantCount[true] - wantCount[false]
			})
		}
	})
	if bursts == 0 {
		t.Error("no library has a burst photo in query.json; the test is vacuous")
	}
	if addedByMissing == 0 {
		t.Error("MissingBursts adds no photo in any library; its switch is untested")
	}
}
