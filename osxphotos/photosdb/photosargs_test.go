package photosdb

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

func argsDay(day int) time.Time { return time.Date(2023, 1, day, 0, 0, 0, 0, time.UTC) }

func argsIDs(photos []*photoinfo.PhotoInfo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.UUID)
	}
	return out
}

// The slice is not in the order of Date, to catch a result left in the order of the library.
func argsDB() *DB {
	return &DB{
		photos: []*photoinfo.PhotoInfo{
			{UUID: "P-6", IsPhoto: true, Date: argsDay(6), Persons: []string{"Zo\U000000eb"}, AlbumUUIDs: []string{"A3"}},
			{UUID: "P-2", IsMovie: true, Date: argsDay(2), Keywords: []string{"beach"}, Persons: []string{"Bob"}, AlbumUUIDs: []string{"A2"}},
			{UUID: "P-1", IsPhoto: true, Date: argsDay(1), Keywords: []string{"beach", "sun"}, Persons: []string{"Alice"}, AlbumUUIDs: []string{"A1"}},
			{UUID: "P-3", IsPhoto: true, Date: argsDay(3), InTrash: true, Keywords: []string{"caf\U000000e9"}, AlbumUUIDs: []string{"A3"}},
			{UUID: "P-4", IsPhoto: true, Date: argsDay(4), Burst: true, BurstPickType: photoinfo.BurstPickDefault},
			{UUID: "P-5", IsPhoto: true, Date: argsDay(5), Burst: true, BurstPickType: photoinfo.BurstPickSelected, AlbumUUIDs: []string{"A5"}},
			{UUID: "P-7", Date: argsDay(7)},
		},
		albums: []*albuminfo.AlbumInfo{
			{UUID: "A1", Title: "Holidays"},
			{UUID: "A2", Title: "Holidays"},
			{UUID: "A3", Title: "Old", InTrash: true},
			{UUID: "A4", Title: "Empty"},
			{UUID: "A5", Title: "Zo\U000000eb"},
		},
	}
}

func TestPhotosWith(t *testing.T) {
	t.Parallel()

	db := argsDB()
	tests := []struct {
		name string
		o    PhotosOptions
		want []string
	}{
		{"no arguments: outside the trash, without unselected burst frames and assets of no kind", PhotosOptions{}, []string{"P-1", "P-2", "P-5", "P-6"}},
		{"in trash", PhotosOptions{InTrash: true}, []string{"P-3"}},
		{"no images", PhotosOptions{NoImages: true}, []string{"P-2"}},
		{"no movies", PhotosOptions{NoMovies: true}, []string{"P-1", "P-5", "P-6"}},
		{"no images and no movies", PhotosOptions{NoImages: true, NoMovies: true}, nil},
		{"uuid, one of them unknown", PhotosOptions{UUID: []string{"P-6", "P-1", "P-0"}}, []string{"P-1", "P-6"}},
		{"uuid of an unselected burst frame", PhotosOptions{UUID: []string{"P-4"}}, nil},
		{"uuid of a photo in the trash", PhotosOptions{UUID: []string{"P-3"}}, nil},
		{"keyword", PhotosOptions{Keywords: []string{"beach"}}, []string{"P-1", "P-2"}},
		{"keywords are alternatives", PhotosOptions{Keywords: []string{"sun", "unknown"}}, []string{"P-1"}},
		{"keyword is not a substring", PhotosOptions{Keywords: []string{"bea"}}, nil},
		{"keyword is case sensitive", PhotosOptions{Keywords: []string{"Beach"}}, nil},
		{"keyword in NFD", PhotosOptions{Keywords: []string{"cafe\U00000301"}, InTrash: true}, []string{"P-3"}},
		{"person", PhotosOptions{Persons: []string{"Alice"}}, []string{"P-1"}},
		{"persons are alternatives", PhotosOptions{Persons: []string{"Bob", "Alice"}}, []string{"P-1", "P-2"}},
		{"person in NFD", PhotosOptions{Persons: []string{"Zoe\U00000308"}}, []string{"P-6"}},
		{"album titles are alternatives", PhotosOptions{Albums: []string{"Holidays", "Empty"}}, []string{"P-1", "P-2"}},
		{"album without photos", PhotosOptions{Albums: []string{"Empty"}}, nil},
		{"album is case sensitive", PhotosOptions{Albums: []string{"holidays"}}, nil},
		{"album title in NFC", PhotosOptions{Albums: []string{"Zo\U000000eb"}}, []string{"P-5"}},
		{"album title in NFD finds nothing", PhotosOptions{Albums: []string{"Zoe\U00000308"}}, nil},
		{"album title is not a substring", PhotosOptions{Albums: []string{"Zo"}}, nil},
		{"empty albums are not set", PhotosOptions{Albums: []string{}}, []string{"P-1", "P-2", "P-5", "P-6"}},
		{"album in the trash, photo outside it", PhotosOptions{Albums: []string{"Old"}}, []string{"P-6"}},
		{"album in the trash, photo in it", PhotosOptions{Albums: []string{"Old"}, InTrash: true}, []string{"P-3"}},
		{"from date is included", PhotosOptions{FromDate: argsDay(5)}, []string{"P-5", "P-6"}},
		{"to date is excluded", PhotosOptions{ToDate: argsDay(2)}, []string{"P-1"}},
		{"from and to date", PhotosOptions{FromDate: argsDay(2), ToDate: argsDay(6)}, []string{"P-2", "P-5"}},
		{"in trash with a date", PhotosOptions{InTrash: true, FromDate: argsDay(4)}, nil},
		{"keyword and person", PhotosOptions{Keywords: []string{"beach"}, Persons: []string{"Bob", "Zoe"}}, []string{"P-2"}},
		{"keyword and album and kind", PhotosOptions{Keywords: []string{"beach"}, Albums: []string{"Holidays"}, NoMovies: true}, []string{"P-1"}},
		{"uuid and date", PhotosOptions{UUID: []string{"P-1", "P-2"}, FromDate: argsDay(2)}, []string{"P-2"}},
	}
	for _, tt := range tests {
		if got := argsIDs(db.PhotosWith(tt.o)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: PhotosWith = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestPhotosWithNewSlice(t *testing.T) {
	t.Parallel()

	db := argsDB()
	for _, o := range []PhotosOptions{{}, {Albums: []string{"Holidays"}}} {
		got := db.PhotosWith(o)
		got[0] = nil
		if again := db.PhotosWith(o); again[0] == nil {
			t.Errorf("%+v: a change of the PhotosWith result is visible in the next call", o)
		}
	}
}

// photos() globs together the albums of one title (photosdb/photosdb.py:3286-3297).
func TestPhotosWithLibraries(t *testing.T) {
	t.Parallel()

	var checked, trashed atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				inTrash := argsIDs(db.PhotosWith(PhotosOptions{InTrash: true}))
				if want := argsIDs(db.Query(query.Options{DeletedOnly: true})); !slices.Equal(inTrash, want) {
					t.Errorf("PhotosWith(InTrash) has %d photos, Query(DeletedOnly) %d", len(inTrash), len(want))
				}
				trashed.Add(int64(len(inTrash)))
				selectable := map[string]bool{}
				for _, p := range db.Query(query.Options{Deleted: true}) {
					selectable[p.UUID] = true
				}
				for _, a := range db.albums {
					var want []string
					for _, b := range db.albums {
						if b.Title == a.Title {
							want = append(want, slices.DeleteFunc(slices.Clone(b.PhotoUUIDs), func(u string) bool { return !selectable[u] })...)
						}
					}
					slices.Sort(want)
					want = slices.Compact(want)
					got := slices.Sorted(slices.Values(slices.Concat(
						argsIDs(db.PhotosWith(PhotosOptions{Albums: []string{a.Title}})),
						argsIDs(db.PhotosWith(PhotosOptions{Albums: []string{a.Title}, InTrash: true})))))
					if !slices.Equal(got, want) {
						t.Errorf("albums titled as %s: %d photos, want %d", a.UUID, len(got), len(want))
					}
					checked.Add(int64(len(want)))
				}
			})
		}
	})
	if checked.Load() == 0 {
		t.Error("no album of any library has a selectable photo: the test is vacuous")
	}
	if trashed.Load() == 0 {
		t.Error("no library has a photo in the trash: the InTrash check is vacuous")
	}
}
