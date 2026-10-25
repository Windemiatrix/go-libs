package query

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func uuids(ps []*photoinfo.PhotoInfo) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.UUID
	}
	return out
}

func TestSortOrder(t *testing.T) {
	t.Parallel()

	plus3 := time.FixedZone("+03", 3*3600)
	minus5 := time.FixedZone("-05", -5*3600)
	same := at("2020-06-01T12:00:00Z")

	tests := []struct {
		name string
		in   []*photoinfo.PhotoInfo
		want []string
	}{
		{
			name: "by date",
			in: []*photoinfo.PhotoInfo{
				{UUID: "A", Date: at("2021-01-01T00:00:00Z")},
				{UUID: "B", Date: at("2019-01-01T00:00:00Z")},
				{UUID: "C", Date: at("2020-01-01T00:00:00Z")},
			},
			want: []string{"B", "C", "A"},
		},
		{
			// One moment in two zones is a tie on Date; DateAdded decides,
			// not the wall clock (15:00 in +03 against 07:00 in -05).
			name: "same moment in different zones",
			in: []*photoinfo.PhotoInfo{
				{UUID: "A", Date: same.In(plus3), DateAdded: ptr(at("2022-01-02T00:00:00Z"))},
				{UUID: "B", Date: same.In(minus5), DateAdded: ptr(at("2022-01-01T00:00:00Z"))},
			},
			want: []string{"B", "A"},
		},
		{
			// A nil DateAdded is 1970-01-01T00:00:00Z: after 1969, before 2000.
			name: "nil DateAdded is the epoch",
			in: []*photoinfo.PhotoInfo{
				{UUID: "A", Date: same, DateAdded: ptr(at("2000-01-01T00:00:00Z"))},
				{UUID: "B", Date: same},
				{UUID: "C", Date: same, DateAdded: ptr(at("1969-12-31T23:59:59Z"))},
			},
			want: []string{"C", "B", "A"},
		},
		{
			name: "nil DateAdded equals the epoch, UUID decides",
			in: []*photoinfo.PhotoInfo{
				{UUID: "B", Date: same},
				{UUID: "A", Date: same, DateAdded: ptr(time.Unix(0, 0).In(plus3))},
			},
			want: []string{"A", "B"},
		},
		{
			// UUIDs compare byte by byte: upper case before lower case.
			name: "equal dates by UUID",
			in: []*photoinfo.PhotoInfo{
				{UUID: "b", Date: same, DateAdded: ptr(same)},
				{UUID: "B", Date: same, DateAdded: ptr(same)},
				{UUID: "A", Date: same, DateAdded: ptr(same)},
			},
			want: []string{"A", "B", "b"},
		},
		{
			// Sort does not guess a date for the zero time: year 1 sorts
			// first. NULL dates are mapped to the epoch by the loader.
			name: "zero Date sorts as year 1",
			in: []*photoinfo.PhotoInfo{
				{UUID: "A", Date: time.Unix(0, 0).UTC()},
				{UUID: "B"},
			},
			want: []string{"B", "A"},
		},
		{name: "empty", in: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := uuids(Sort(tt.in)); !slices.Equal(got, tt.want) {
				t.Errorf("Sort order = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortKeepsInput(t *testing.T) {
	t.Parallel()

	in := []*photoinfo.PhotoInfo{
		{UUID: "C", Date: at("2022-01-01T00:00:00Z")},
		{UUID: "A", Date: at("2020-01-01T00:00:00Z")},
		{UUID: "B", Date: at("2021-01-01T00:00:00Z")},
	}
	before := slices.Clone(in)
	out := Sort(in)

	if !slices.Equal(in, before) {
		t.Errorf("Sort reordered its input: %q, was %q", uuids(in), uuids(before))
	}
	if len(out) != len(in) {
		t.Fatalf("len(Sort) = %d, want %d", len(out), len(in))
	}
	for _, p := range out {
		if !slices.Contains(in, p) {
			t.Errorf("Sort result holds %p (%s), not an element of the input", p, p.UUID)
		}
	}
	out[0] = &photoinfo.PhotoInfo{UUID: "new"}
	if slices.ContainsFunc(in, func(p *photoinfo.PhotoInfo) bool { return p.UUID == "new" }) {
		t.Error("a write to the Sort result is visible in the input: the result shares its backing array")
	}

	if got := Sort([]*photoinfo.PhotoInfo{}); len(got) != 0 {
		t.Errorf("Sort of an empty slice has length %d", len(got))
	}
}

// TestSortStable checks that photos with fully equal keys keep their input
// order. The input is large and mixes several groups of equal keys: a sort
// that is not stable keeps the order of a handful of elements (insertion
// sort) or of a single group (no comparison says "less"), so neither would
// be red.
func TestSortStable(t *testing.T) {
	t.Parallel()

	keys := []*photoinfo.PhotoInfo{
		{UUID: "K1", Date: at("2021-01-01T00:00:00Z")},
		{UUID: "K2", Date: at("2020-01-01T00:00:00Z")},
		{UUID: "K3", Date: at("2020-01-01T00:00:00Z"), DateAdded: ptr(at("2020-02-01T00:00:00Z"))},
	}
	const perKey = 40
	interleaved := make([]*photoinfo.PhotoInfo, 0, perKey*len(keys))
	for range perKey {
		for _, k := range keys {
			p := *k
			interleaved = append(interleaved, &p)
		}
	}
	seed1, seed2 := uint64(time.Now().UnixNano()), uint64(0x2545f4914f6cdd1d)
	shuffled := slices.Clone(interleaved)
	rand.New(rand.NewPCG(seed1, seed2)).Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })

	for _, tt := range []struct {
		name string
		in   []*photoinfo.PhotoInfo
	}{{"interleaved", interleaved}, {"shuffled", shuffled}} {
		position := make(map[*photoinfo.PhotoInfo]int, len(tt.in))
		for i, p := range tt.in {
			position[p] = i
		}
		out := Sort(tt.in)
		last := map[string]int{} // UUID -> input position of the last photo of the group seen
		for _, p := range out {
			if prev, ok := last[p.UUID]; ok && position[p] < prev {
				t.Fatalf("%s (PCG seed %d, %d): Sort put the photo at input position %d after the one at %d; equal keys must keep their input order",
					tt.name, seed1, seed2, position[p], prev)
			}
			last[p.UUID] = position[p]
		}
	}
}

// oracle sorts by insertion on (Date moment, DateAdded moment, UUID), written
// independently of Sort.
func oracle(in []*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
	epoch := time.Unix(0, 0)
	added := func(p *photoinfo.PhotoInfo) time.Time {
		if p.DateAdded == nil {
			return epoch
		}
		return *p.DateAdded
	}
	less := func(a, b *photoinfo.PhotoInfo) bool {
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if aa, ba := added(a), added(b); !aa.Equal(ba) {
			return aa.Before(ba)
		}
		return a.UUID < b.UUID
	}
	out := make([]*photoinfo.PhotoInfo, 0, len(in))
	for _, p := range in {
		i := len(out)
		for i > 0 && less(p, out[i-1]) {
			i--
		}
		out = slices.Insert(out, i, p)
	}
	return out
}

func TestSortPermutations(t *testing.T) {
	t.Parallel()

	zones := []*time.Location{time.UTC, time.FixedZone("+05:30", 19800), time.FixedZone("-08", -8*3600)}
	dates := []time.Time{at("2019-03-01T10:00:00Z"), at("2020-01-01T00:00:00Z"), at("1969-07-20T20:17:00Z")}
	added := []*time.Time{nil, ptr(time.Unix(0, 0)), ptr(at("2021-05-05T05:05:05Z")), ptr(at("1960-01-01T00:00:00Z"))}

	// Every (date, added) pair twice under different UUIDs and zones: ties on
	// Date and on (Date, DateAdded) are frequent; UUIDs are unique, so the
	// order is fully determined.
	var in []*photoinfo.PhotoInfo
	n := 0
	for _, d := range dates {
		for _, a := range added {
			for range 2 {
				p := &photoinfo.PhotoInfo{UUID: fmt.Sprintf("U%02d", (n*7)%24), Date: d.In(zones[n%len(zones)])}
				if a != nil {
					p.DateAdded = ptr(a.In(zones[(n+1)%len(zones)]))
				}
				in = append(in, p)
				n++
			}
		}
	}
	want := oracle(in)
	wantIDs := uuids(want)

	seed1, seed2 := uint64(time.Now().UnixNano()), uint64(0x9e3779b97f4a7c15)
	rng := rand.New(rand.NewPCG(seed1, seed2))
	for i := range 200 {
		perm := slices.Clone(in)
		rng.Shuffle(len(perm), func(a, b int) { perm[a], perm[b] = perm[b], perm[a] })
		if got := uuids(Sort(perm)); !slices.Equal(got, wantIDs) {
			t.Fatalf("permutation %d (PCG seed %d, %d): Sort order\n%s\nwant\n%s",
				i, seed1, seed2, strings.Join(got, " "), strings.Join(wantIDs, " "))
		}
		if got := uuids(oracle(perm)); !slices.Equal(got, wantIDs) {
			t.Fatalf("permutation %d (PCG seed %d, %d): the oracle depends on input order", i, seed1, seed2)
		}
	}
}
