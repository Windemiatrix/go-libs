package query_test

import (
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

func personUUIDs(photos []*photoinfo.PhotoInfo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.UUID)
	}
	return out
}

// photoquery.py:467-470, 996-1021: equality with an element of persons.
func TestPersonFilter(t *testing.T) {
	t.Parallel()

	const nfd, nfc = "Jose\U00000301", "Jos\U000000e9"
	if nfd == nfc {
		t.Fatal("the NFD and NFC forms are equal: the case is vacuous")
	}
	date := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	photos := []*photoinfo.PhotoInfo{
		{UUID: "P-1", IsPhoto: true, Date: date, Persons: []string{"Person A", "Person B"}},
		{UUID: "P-2", IsPhoto: true, Date: date, Persons: []string{nfc, "_UNKNOWN_"}},
		{UUID: "P-3", IsPhoto: true, Date: date},
		{UUID: "P-4", IsPhoto: true, Date: date, InTrash: true, Persons: []string{"Person A"}},
	}
	tests := []struct {
		name string
		o    query.Options
		want []string
	}{
		{"one name", query.Options{Person: []string{"Person A"}}, []string{"P-1"}},
		{"not a substring", query.Options{Person: []string{"Person"}}, nil},
		{"several names", query.Options{Person: []string{"Person B", "_UNKNOWN_"}}, []string{"P-1", "P-2"}},
		{"an NFD value", query.Options{Person: []string{nfd}}, []string{"P-2"}},
		{"case kept", query.Options{Person: []string{"person a"}}, nil},
		{"ignore case", query.Options{Person: []string{"PERSON A"}, IgnoreCase: true}, []string{"P-1"}},
		{"with the trash", query.Options{Person: []string{"Person A"}, Deleted: true}, []string{"P-1", "P-4"}},
		{"an empty list", query.Options{Person: []string{}}, []string{"P-1", "P-2", "P-3"}},
	}
	for _, tt := range tests {
		if got := personUUIDs(query.Apply(tt.o, photos, nil)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

type personRef struct {
	UUID    string   `json:"uuid"`
	Persons []string `json:"persons"`
}

// The expected photos are the objects of query.json with the name, in the
// order of the file: TestQueryGolden of photosdb holds that Query(Options{})
// keeps it. Names are personal data, so the messages hold numbers only.
func TestPersonGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	var mu sync.Mutex
	expected := 0
	t.Run("libraries", func(t *testing.T) {
		for _, name := range testlib.Libraries(t) {
			lib := testlib.OpenPristine(t, name)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				db, err := photosdb.Open(t.Context(), lib, photosdb.Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ref, err := golden.Load(dir, name)
				if err != nil {
					t.Fatal(err)
				}
				refs := make([]personRef, len(ref.Query))
				for i, obj := range ref.Query {
					data, err := json.Marshal(obj)
					if err == nil {
						err = json.Unmarshal(data, &refs[i])
					}
					if err != nil {
						t.Fatalf("query.json element %d: %v", i+1, err)
					}
				}
				set := map[string]bool{}
				for _, r := range refs {
					for _, p := range r.Persons {
						set[p] = true
					}
				}
				n := 0
				for i, person := range slices.Sorted(maps.Keys(set)) {
					var want []string
					for _, r := range refs {
						if slices.Contains(r.Persons, person) {
							want = append(want, r.UUID)
						}
					}
					if got := personUUIDs(db.Query(query.Options{Person: []string{person}})); !slices.Equal(got, want) {
						t.Errorf("person %d: %d photos, want %d", i, len(got), len(want))
					}
					n += len(want)
				}
				t.Logf("%d names, %d expected photos", len(set), n)
				mu.Lock()
				defer mu.Unlock()
				expected += n
			})
		}
	})
	if expected == 0 {
		t.Error("no library expects a photo of a person: the test is vacuous")
	}
}
