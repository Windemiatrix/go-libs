package photosdb

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func orderUUIDs(photos []*photoinfo.PhotoInfo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.UUID)
	}
	return out
}

func orderDay(day int) time.Time { return time.Date(2023, 1, day, 0, 0, 0, 0, time.UTC) }

func orderDB() *DB {
	photos := []*photoinfo.PhotoInfo{
		{UUID: "P-1", Date: orderDay(2), Title: "b"},
		{UUID: "P-2", Date: orderDay(1)},
		{UUID: "P-3", Date: orderDay(2), Title: "a", InTrash: true},
		{UUID: "P-4", Date: orderDay(3), Title: "b"},
		{UUID: "P-5", Date: orderDay(1), Title: "c"},
	}
	byUUID := map[string]*photoinfo.PhotoInfo{}
	var assets []sqlread.Asset
	for i, p := range photos {
		byUUID[p.UUID] = p
		assets = append(assets, sqlread.Asset{PK: int64(i + 1), UUID: nullString(p.UUID), HasAttributes: true})
	}
	assets = append(assets, sqlread.Asset{PK: 9, UUID: nullString("P-1")})
	none := sql.NullInt64{}
	members := []sqlread.Member{
		{AssetPK: 1, AlbumUUID: "A", Order: nullInt(20)},
		{AssetPK: 4, AlbumUUID: "A", Order: nullInt(40)},
		{AssetPK: 9, AlbumUUID: "A", Order: nullInt(5)},
		{AssetPK: 3, AlbumUUID: "A", Order: nullInt(30)},
		{AssetPK: 99, AlbumUUID: "A", Order: nullInt(1)},
		{AssetPK: 4, AlbumUUID: "A", Order: nullInt(10)},
		{AssetPK: 2, AlbumUUID: "A", Order: none},
		{AssetPK: 5, AlbumUUID: "B", Order: nullInt(7)},
		{AssetPK: 1, AlbumUUID: "B", Order: nullInt(7)},
		{AssetPK: 3, AlbumUUID: "B", Order: nullInt(-1)},
		{AssetPK: 4, AlbumUUID: "B", Order: none},
		{AssetPK: 2, AlbumUUID: "B", Order: none},
	}
	return &DB{byUUID: byUUID, albumPhotos: membershipOrder(members, assets, byUUID)}
}

func TestAlbumPhotos(t *testing.T) {
	t.Parallel()

	db := orderDB()
	tests := []struct {
		name  string
		album string
		order albuminfo.AlbumSortOrder
		want  []string
	}{
		{"manual", "A", albuminfo.AlbumSortManual, []string{"P-2", "P-4", "P-1", "P-3", "P-4"}},
		{"unknown", "A", albuminfo.AlbumSortUnknown, []string{"P-2", "P-4", "P-1", "P-3", "P-4"}},
		{"oldest first", "A", albuminfo.AlbumSortOldestFirst, []string{"P-2", "P-1", "P-3", "P-4", "P-4"}},
		{"newest first", "A", albuminfo.AlbumSortNewestFirst, []string{"P-4", "P-4", "P-1", "P-3", "P-2"}},
		{"title", "A", albuminfo.AlbumSortTitle, []string{"P-2", "P-3", "P-4", "P-1", "P-4"}},
		{"NULL before a negative order, ties by UUID", "B", albuminfo.AlbumSortManual, []string{"P-2", "P-4", "P-3", "P-1", "P-5"}},
		{"no members", "C", albuminfo.AlbumSortManual, nil},
	}
	for _, tt := range tests {
		a := &albuminfo.AlbumInfo{UUID: tt.album, SortOrder: tt.order}
		got := db.AlbumPhotos(a)
		if uuids := orderUUIDs(got); !slices.Equal(uuids, tt.want) {
			t.Errorf("%s: AlbumPhotos %q, want %q", tt.name, uuids, tt.want)
		}
		if got != nil {
			got[0] = nil
			if again := db.AlbumPhotos(a); again[0] == nil {
				t.Errorf("%s: a change of the AlbumPhotos result is visible in the next call", tt.name)
			}
		}
	}
}

func TestPhotoIndex(t *testing.T) {
	t.Parallel()

	db := orderDB()
	tests := []struct {
		name   string
		order  albuminfo.AlbumSortOrder
		photo  string
		want   int
		wantOK bool
	}{
		{"first of a repeated photo", albuminfo.AlbumSortManual, "P-4", 1, true},
		{"photo in the trash", albuminfo.AlbumSortManual, "P-3", 3, true},
		{"by title", albuminfo.AlbumSortTitle, "P-4", 2, true},
		{"not in the album", albuminfo.AlbumSortManual, "P-5", -1, false},
	}
	for _, tt := range tests {
		a := &albuminfo.AlbumInfo{UUID: "A", SortOrder: tt.order}
		if got, ok := db.PhotoIndex(a, db.byUUID[tt.photo]); got != tt.want || ok != tt.wantOK {
			t.Errorf("%s: PhotoIndex = %d, %v; want %d, %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

var orderKeys = map[albuminfo.AlbumSortOrder]func(p, q *photoinfo.PhotoInfo) int{
	albuminfo.AlbumSortNewestFirst: func(p, q *photoinfo.PhotoInfo) int { return q.Date.Compare(p.Date) },
	albuminfo.AlbumSortOldestFirst: func(p, q *photoinfo.PhotoInfo) int { return p.Date.Compare(q.Date) },
	albuminfo.AlbumSortTitle:       func(p, q *photoinfo.PhotoInfo) int { return strings.Compare(p.Title, q.Title) },
}

func orderCheckLibrary(t *testing.T, name, lib string) (manual, reordered, sorted int) {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	src := roDB(t, lib)
	n := schemaByID(t, libraryModels[name].id)
	membership := "SELECT a.ZUUID FROM " + n.MemberTable + " m JOIN ZGENERICALBUM g ON g.Z_PK = m." + n.MemberAlbumColumn +
		" JOIN ZASSET a ON a.Z_PK = m." + n.MemberAssetColumn +
		" WHERE EXISTS (SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK) AND g.ZUUID = '"
	for _, a := range db.albums {
		photos := db.AlbumPhotos(a)
		got := orderUUIDs(photos)
		set := slices.Clone(got)
		slices.Sort(set)
		if !slices.Equal(set, a.PhotoUUIDs) {
			t.Errorf("album %s: AlbumPhotos %q, want the photos of PhotoUUIDs %q once each", a.UUID, got, a.PhotoUUIDs)
		}
		for i, p := range photos {
			if j, ok := db.PhotoIndex(a, p); j != i || !ok {
				t.Errorf("album %s: PhotoIndex(%s) = %d, %v; want %d, true", a.UUID, p.UUID, j, ok, i)
			}
		}
		if key, ok := orderKeys[a.SortOrder]; ok {
			sorted++
			if !slices.IsSortedFunc(photos, key) {
				t.Errorf("album %s: AlbumPhotos %q not in the sort order %d", a.UUID, got, a.SortOrder)
			}
			continue
		}
		if a.SortOrder != albuminfo.AlbumSortManual {
			continue
		}
		manual++
		if want := testlib.Rows(t, src, membership+a.UUID+"' ORDER BY m."+n.MemberOrderColumn+", a.ZUUID"); !slices.Equal(got, want) {
			t.Errorf("album %s: AlbumPhotos %q, want %q", a.UUID, got, want)
		}
		if !slices.Equal(got, a.PhotoUUIDs) {
			reordered++
		}
	}
	return manual, reordered, sorted
}

// The reference cannot check the order: osxphotos prints no photos of an album.
func TestAlbumPhotosLibraries(t *testing.T) {
	t.Parallel()

	var manual, reordered, sorted int
	add := func(m, r, s int) { manual, reordered, sorted = manual+m, reordered+r, sorted+s }
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) { add(orderCheckLibrary(t, name, testlib.OpenPristine(t, name))) })
	}
	t.Run("synthetic with every sort order and NULL positions", func(t *testing.T) {
		lib := testlib.OpenPristine(t, "synthetic")
		n := schemaByID(t, libraryModels["synthetic"].id)
		change(t, lib,
			"UPDATE ZGENERICALBUM SET ZCUSTOMSORTKEY = CASE Z_PK % 4 WHEN 0 THEN 1 WHEN 1 THEN 1 WHEN 2 THEN 5 ELSE 0 END, "+
				"ZCUSTOMSORTASCENDING = Z_PK % 4 WHERE ZKIND = 2",
			"UPDATE "+n.MemberTable+" SET "+n.MemberOrderColumn+" = CASE WHEN rowid % 5 = 0 THEN NULL ELSE (rowid * 7919) % 1009 END",
		)
		add(orderCheckLibrary(t, "synthetic", lib))
	})
	t.Logf("%d manual albums, %d of them not in UUID order; %d albums sorted by date or title", manual, reordered, sorted)
	if reordered == 0 || sorted == 0 {
		t.Error("no manual album out of UUID order or no album sorted by date or title: the SQL oracle checks little")
	}
}
