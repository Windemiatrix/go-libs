package photosdb

import (
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

func albumRow(pk int64, uuid string, kind int64, parent sql.NullInt64) sqlread.Album {
	return sqlread.Album{PK: pk, UUID: uuid, Kind: kind, Parent: parent}
}

func withTitle(a sqlread.Album, title string) sqlread.Album {
	a.Title = nullString(title)
	return a
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sameStringsNilAware(a, b []string) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}

func TestLoadAlbums(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9" // escapes: an editor may normalize the literal itself
	none := sql.NullInt64{}
	root, f1, f2, fn, ft := int64(1), int64(2), int64(3), int64(4), int64(5)

	deep := withTitle(albumRow(10, "AL-E", sqlread.KindUserAlbum, nullInt(f2)), nfd)
	deep.Trashed, deep.CloudGUID = nullInt(0), nullString("GUID-1")
	deep.Created, deep.Start, deep.End = nullFloat(694224000), nullFloat(0), nullFloat(1e300)
	top := withTitle(albumRow(11, "AL-A", sqlread.KindUserAlbum, nullInt(root)), "Top")
	noParent := withTitle(albumRow(12, "AL-D", sqlread.KindUserAlbum, none), "No parent")
	noParent.Trashed, noParent.CloudGUID = nullInt(1), nullString("")
	underFN := withTitle(albumRow(13, "AL-B", sqlread.KindUserAlbum, nullInt(fn)), "Under FN")
	underFN.Trashed = nullInt(2)
	noTitle := albumRow(14, "AL-C", sqlread.KindUserAlbum, nullInt(ft))
	noTitle.Trashed = nullInt(0)
	same1 := withTitle(albumRow(15, "AL-G", sqlread.KindUserAlbum, nullInt(f2)), "Same")
	same1.Trashed = nullInt(0)
	same2 := withTitle(albumRow(16, "AL-F", sqlread.KindUserAlbum, nullInt(f2)), "Same")
	same2.Trashed = nullInt(0)

	rows := []sqlread.Album{
		albumRow(root, "ROOT", sqlread.KindRootFolder, none),
		withTitle(albumRow(f1, "F-1", sqlread.KindFolder, nullInt(root)), nfd),
		withTitle(albumRow(f2, "F-2", sqlread.KindFolder, nullInt(f1)), "F2"),
		withTitle(albumRow(fn, "F-N", sqlread.KindFolder, none), "FN"),
		albumRow(ft, "F-T", sqlread.KindFolder, nullInt(root)),
		deep, top, noParent, underFN, noTitle, same1, same2,
	}
	skippedDup := sqlread.Asset{PK: 103, UUID: nullString("P-1")}
	assets := []sqlread.Asset{
		{PK: 100, UUID: nullString("P-1"), HasAttributes: true},
		{PK: 101, UUID: nullString("P-2"), HasAttributes: true},
		{PK: 102, UUID: nullString("P-3")},
		skippedDup,
	}
	byUUID := map[string]*photoinfo.PhotoInfo{"P-1": {UUID: "P-1"}, "P-2": {UUID: "P-2"}}
	members := []sqlread.Member{
		{AssetPK: 101, AlbumUUID: "AL-E"}, {AssetPK: 100, AlbumUUID: "AL-E"}, {AssetPK: 101, AlbumUUID: "AL-E"},
		{AssetPK: 102, AlbumUUID: "AL-E"}, {AssetPK: 999, AlbumUUID: "AL-E"},
		{AssetPK: 103, AlbumUUID: "AL-A"},
		{AssetPK: 100, AlbumUUID: "AL-B"},
		{AssetPK: 100, AlbumUUID: "AL-G"}, {AssetPK: 101, AlbumUUID: "AL-F"},
	}

	created, start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	want := []*albuminfo.AlbumInfo{
		{UUID: "AL-A", Title: "Top", InTrash: true},
		{UUID: "AL-B", Title: "Under FN", ParentUUID: "F-N", FolderNames: []string{"FN"}, PhotoUUIDs: []string{"P-1"}, InTrash: true},
		{UUID: "AL-C", ParentUUID: "F-T", FolderNames: []string{""}},
		{UUID: "AL-D", Title: "No parent", InTrash: true},
		{
			UUID: "AL-E", CloudGUID: "GUID-1", Title: nfcForm, ParentUUID: "F-2", FolderNames: []string{nfcForm, "F2"},
			PhotoUUIDs: []string{"P-1", "P-2"}, CreationDate: &created, StartDate: &start,
		},
		{UUID: "AL-F", Title: "Same", ParentUUID: "F-2", FolderNames: []string{nfcForm, "F2"}, PhotoUUIDs: []string{"P-2"}},
		{UUID: "AL-G", Title: "Same", ParentUUID: "F-2", FolderNames: []string{nfcForm, "F2"}, PhotoUUIDs: []string{"P-1"}},
	}

	got, err := loadAlbums(rows, membershipOrder(members, assets, byUUID))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%d albums, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.UUID != w.UUID || g.CloudGUID != w.CloudGUID || g.Title != w.Title || g.ParentUUID != w.ParentUUID ||
			!sameStringsNilAware(g.FolderNames, w.FolderNames) || !sameStringsNilAware(g.PhotoUUIDs, w.PhotoUUIDs) || g.InTrash != w.InTrash ||
			!sameTime(g.CreationDate, w.CreationDate) || !sameTime(g.StartDate, w.StartDate) || !sameTime(g.EndDate, w.EndDate) {
			t.Errorf("album %d = %+v, want %+v", i, *g, *w)
		}
	}
}

func TestLoadAlbumsSortOrder(t *testing.T) {
	t.Parallel()

	none := sql.NullInt64{}
	tests := []struct {
		name     string
		key, asc sql.NullInt64
		want     albuminfo.AlbumSortOrder
	}{
		{"newest first", nullInt(1), nullInt(0), albuminfo.AlbumSortNewestFirst},
		{"oldest first", nullInt(1), nullInt(1), albuminfo.AlbumSortOldestFirst},
		{"another direction", nullInt(1), nullInt(2), albuminfo.AlbumSortUnknown},
		{"no direction", nullInt(1), none, albuminfo.AlbumSortUnknown},
		{"title", nullInt(5), nullInt(1), albuminfo.AlbumSortTitle},
		{"title without direction", nullInt(5), none, albuminfo.AlbumSortTitle},
		{"manual", nullInt(0), none, albuminfo.AlbumSortManual},
		{"manual with a direction", nullInt(0), nullInt(1), albuminfo.AlbumSortManual},
		{"another key", nullInt(2), nullInt(0), albuminfo.AlbumSortUnknown},
		{"no key", none, nullInt(0), albuminfo.AlbumSortUnknown},
		{"no key and no direction", none, none, albuminfo.AlbumSortUnknown},
	}
	rows := []sqlread.Album{albumRow(1, "ROOT", sqlread.KindRootFolder, none)}
	for i, tt := range tests {
		r := albumRow(int64(i+2), fmt.Sprintf("A-%02d", i), sqlread.KindUserAlbum, nullInt(1))
		r.SortKey, r.SortAscending = tt.key, tt.asc
		rows = append(rows, r)
	}
	got, err := loadAlbums(rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(tests) {
		t.Fatalf("%d albums, want %d", len(got), len(tests))
	}
	for i, tt := range tests {
		if got[i].SortOrder != tt.want {
			t.Errorf("%s: SortOrder %d, want %d", tt.name, got[i].SortOrder, tt.want)
		}
	}
}

func sortRow(pk int64, uuid string, kind int64, parent, order sql.NullInt64) sqlread.Album {
	r := albumRow(pk, uuid, kind, parent)
	r.ListOrder = order
	return r
}

func sortListRows() []sqlread.Album {
	none := sql.NullInt64{}
	const folder, album = sqlread.KindFolder, sqlread.KindUserAlbum
	return []sqlread.Album{
		sortRow(1, "ROOT", sqlread.KindRootFolder, none, none),
		sortRow(2, "F-1", folder, nullInt(1), nullInt(3)),
		sortRow(3, "F-2", folder, nullInt(2), nullInt(7)),
		sortRow(4, "F-N", folder, nullInt(1), none),
		sortRow(5, "F-NN", folder, nullInt(4), nullInt(4)),
		sortRow(6, "F-P", folder, none, nullInt(2)),
		sortRow(7, "F-X", folder, nullInt(2), none),
		sortRow(8, "A-TOP", album, nullInt(1), nullInt(5)),
		sortRow(9, "A-DEEP", album, nullInt(3), nullInt(2)),
		sortRow(10, "A-DEEP2", album, nullInt(3), nullInt(9)),
		sortRow(11, "A-ZERO", album, nullInt(2), nullInt(0)),
		sortRow(12, "A-UNDERNULL", album, nullInt(4), nullInt(6)),
		sortRow(13, "A-UNDERNN", album, nullInt(5), nullInt(8)),
		sortRow(14, "A-NULL", album, nullInt(2), none),
		sortRow(15, "A-NOPARENT", album, none, nullInt(1)),
		sortRow(16, "A-UNDERX", album, nullInt(7), nullInt(1)),
	}
}

func TestLoadAlbumsListOrder(t *testing.T) {
	t.Parallel()

	want := map[string][]int{
		"A-TOP":       {5},
		"A-DEEP":      {3, 7, 2},
		"A-DEEP2":     {3, 7, 9},
		"A-ZERO":      {3, 0},
		"A-UNDERNULL": {9999999, 6},
		"A-UNDERNN":   {9999999, 4, 8},
		"A-NULL":      {9999999},
		"A-NOPARENT":  {1},
		"A-UNDERX":    {9999999, 1},
	}
	got, err := loadAlbums(sortListRows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%d albums, want %d", len(got), len(want))
	}
	for _, a := range got {
		if !slices.Equal(a.LibraryListOrder, want[a.UUID]) {
			t.Errorf("album %s: LibraryListOrder %v, want %v", a.UUID, a.LibraryListOrder, want[a.UUID])
		}
	}
}

const sortCase = "SELECT CASE WHEN ZCUSTOMSORTKEY = 1 AND ZCUSTOMSORTASCENDING = 0 THEN 2 " +
	"WHEN ZCUSTOMSORTKEY = 1 AND ZCUSTOMSORTASCENDING = 1 THEN 3 WHEN ZCUSTOMSORTKEY = 5 THEN 5 " +
	"WHEN ZCUSTOMSORTKEY = 0 THEN 1 ELSE 0 END FROM ZGENERICALBUM WHERE ZUUID = '"

// A row without an order ends the walk up, as the exception of int(None) does in upstream.
func sortChain(uuid string) string {
	return "WITH RECURSIVE up(depth, parent, ord) AS (" +
		"SELECT 0, ZPARENTFOLDER, Z_FOK_PARENTFOLDER FROM ZGENERICALBUM WHERE ZUUID = '" + uuid + "' " +
		"UNION ALL SELECT up.depth + 1, g.ZPARENTFOLDER, g.Z_FOK_PARENTFOLDER FROM up JOIN ZGENERICALBUM g ON g.Z_PK = up.parent " +
		"WHERE g.ZKIND != 3999 AND up.ord IS NOT NULL) SELECT COALESCE(ord, 9999999) FROM up ORDER BY depth DESC"
}

func sortStrings(order []int) []string {
	out := make([]string, len(order))
	for i, v := range order {
		out[i] = strconv.Itoa(v)
	}
	return out
}

func sortCheckLibrary(t *testing.T, lib string, seen map[albuminfo.AlbumSortOrder]int) (chains int) {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	src := roDB(t, lib)
	for _, a := range db.albums {
		seen[a.SortOrder]++
		if want := testlib.Count(t, src, sortCase+a.UUID+"'"); int(a.SortOrder) != want {
			t.Errorf("album %s: SortOrder %d, want %d", a.UUID, a.SortOrder, want)
		}
		if want := testlib.Rows(t, src, sortChain(a.UUID)); !slices.Equal(sortStrings(a.LibraryListOrder), want) {
			t.Errorf("album %s: LibraryListOrder %v, want %q", a.UUID, a.LibraryListOrder, want)
		}
		if n := len(a.LibraryListOrder); n > 2 || (n > 1 && slices.Contains(a.LibraryListOrder, noListOrder)) {
			chains++
		}
	}
	for _, u := range testlib.Rows(t, src, "SELECT ZUUID FROM ZGENERICALBUM WHERE ZKIND = 4000 AND ZUUID IS NOT NULL") {
		f, ok := db.Folder(u)
		if !ok {
			t.Errorf("Folder(%s) found nothing", u)
			continue
		}
		if want := testlib.Rows(t, src, sortChain(u)); !slices.Equal(sortStrings(f.LibraryListOrder), want) {
			t.Errorf("folder %s: LibraryListOrder %v, want %q", u, f.LibraryListOrder, want)
		}
	}
	return chains
}

// The reference cannot check these: osxphotos prints sort_order as null and has
// no library_list_order.
func TestAlbumsOrder(t *testing.T) {
	t.Parallel()

	seen := map[albuminfo.AlbumSortOrder]int{}
	chains := 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) { chains += sortCheckLibrary(t, testlib.OpenPristine(t, name), seen) })
	}
	t.Run("synthetic with every sort order and some orders removed", func(t *testing.T) {
		lib := testlib.OpenPristine(t, "synthetic")
		change(t, lib,
			"UPDATE ZGENERICALBUM SET "+
				"ZCUSTOMSORTKEY = CASE Z_PK % 6 WHEN 0 THEN 1 WHEN 1 THEN 1 WHEN 2 THEN 5 WHEN 3 THEN 0 WHEN 4 THEN 1 END, "+
				"ZCUSTOMSORTASCENDING = CASE Z_PK % 6 WHEN 0 THEN 0 WHEN 1 THEN 1 WHEN 4 THEN 2 END WHERE ZKIND = 2",
			"UPDATE ZGENERICALBUM SET Z_FOK_PARENTFOLDER = NULL WHERE ZKIND IN (2, 4000) AND Z_PK % 5 = 3",
		)
		chains += sortCheckLibrary(t, lib, seen)
	})
	t.Logf("sort orders of all albums: %v; %d albums with a deep chain or a missing order above", seen, chains)
	// Without them the SQL oracle says nothing about the drop of the folders above a row without an order.
	if chains == 0 {
		t.Error("no album has a chain deeper than one folder or an enclosing folder without an order")
	}
	for _, o := range []albuminfo.AlbumSortOrder{
		albuminfo.AlbumSortUnknown, albuminfo.AlbumSortManual, albuminfo.AlbumSortNewestFirst,
		albuminfo.AlbumSortOldestFirst, albuminfo.AlbumSortTitle,
	} {
		if seen[o] == 0 {
			t.Errorf("no album has the sort order %d", o)
		}
	}
}

func TestLoadAlbumsErrors(t *testing.T) {
	t.Parallel()

	none := sql.NullInt64{}
	rootRow := albumRow(1, "SECRET-ROOT", sqlread.KindRootFolder, none)
	album := func(pk int64, uuid string, parent sql.NullInt64) sqlread.Album {
		return withTitle(albumRow(pk, uuid, sqlread.KindUserAlbum, parent), "SECRET-TITLE")
	}
	folder := func(pk int64, uuid string, parent sql.NullInt64) sqlread.Album {
		return withTitle(albumRow(pk, uuid, sqlread.KindFolder, parent), "SECRET-FOLDER")
	}
	tests := []struct {
		name string
		rows []sqlread.Album
		want string
	}{
		{"no root", []sqlread.Album{album(2, "SECRET-A", none)}, "0 root folders in ZGENERICALBUM, want 1"},
		{"two roots", []sqlread.Album{rootRow, albumRow(2, "SECRET-ROOT-2", sqlread.KindRootFolder, none)}, "2 root folders"},
		{"empty UUID", []sqlread.Album{rootRow, album(2, "", nullInt(1))}, "1 albums and folders with an empty or repeated ZGENERICALBUM.ZUUID"},
		{"repeated UUID", []sqlread.Album{rootRow, folder(2, "SECRET-U", nullInt(1)), album(3, "SECRET-U", nullInt(2))},
			"1 albums and folders with an empty or repeated"},
		{"dangling parent", []sqlread.Album{rootRow, album(2, "SECRET-A", nullInt(99))},
			"1 albums whose parent folders do not lead to the root folder"},
		{"cycle of two folders", []sqlread.Album{
			rootRow, folder(2, "SECRET-F1", nullInt(3)), folder(3, "SECRET-F2", nullInt(2)), album(4, "SECRET-A", nullInt(2)),
		}, "1 albums whose parent folders"},
		{"album is its own parent", []sqlread.Album{rootRow, album(2, "SECRET-A", nullInt(2))}, "1 albums whose parent folders"},
	}
	for _, tt := range tests {
		got, err := loadAlbums(tt.rows, nil)
		if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "SECRET") || got != nil {
			t.Errorf("%s: loadAlbums = %d albums, %v; want an error with %q and without values", tt.name, len(got), err, tt.want)
		}
	}
}

func TestAlbums(t *testing.T) {
	t.Parallel()

	albums, nested := 0, 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)
			n := schemaByID(t, libraryModels[name].id)
			const user = " FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZUUID IS NOT NULL"

			got := db.albums
			albums += len(got)
			if want := count(t, src, "SELECT COUNT(*)"+user); len(got) != want {
				t.Errorf("%d albums, want %d", len(got), want)
			}
			trashed, guids, members := 0, 0, 0
			for i, a := range got {
				if i > 0 && got[i-1].UUID >= a.UUID {
					t.Errorf("album %d out of UUID order or repeated", i+1)
				}
				if a.InTrash {
					trashed++
				}
				if a.CloudGUID != "" {
					guids++
				}
				if len(a.FolderNames) > 0 {
					nested++
				}
				members += len(a.PhotoUUIDs)
			}

			photosOf := map[string][]string{}
			for _, p := range db.Photos() {
				for _, u := range p.AlbumUUIDs {
					if _, ok := db.Album(u); !ok {
						t.Errorf("an album of photo %s is not among the albums read", p.UUID)
					}
					photosOf[u] = append(photosOf[u], p.UUID)
				}
			}
			for _, a := range got {
				want := photosOf[a.UUID]
				slices.Sort(want)
				if !slices.Equal(a.PhotoUUIDs, want) {
					t.Errorf("album %s: PhotoUUIDs %q, want the photos that list it %q", a.UUID, a.PhotoUUIDs, want)
				}
			}

			if want := count(t, src, "SELECT COUNT(*) FROM (SELECT DISTINCT m."+n.MemberAlbumColumn+", m."+n.MemberAssetColumn+
				" FROM "+n.MemberTable+" m JOIN ZGENERICALBUM g ON g.Z_PK = m."+n.MemberAlbumColumn+
				" JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = m."+n.MemberAssetColumn+
				" WHERE g.ZKIND = 2 AND g.ZUUID IS NOT NULL)"); members != want {
				t.Errorf("%d album members, want %d", members, want)
			}
			if want := count(t, src, "SELECT COUNT(*)"+user+" AND (ZTRASHEDSTATE IS NULL OR ZTRASHEDSTATE != 0)"); trashed != want {
				t.Errorf("%d albums in the trash, want %d", trashed, want)
			}
			if want := count(t, src, "SELECT COUNT(*)"+user+" AND ZCLOUDGUID != ''"); guids != want {
				t.Errorf("%d albums with a CloudGUID, want %d", guids, want)
			}
			// The premise of the owner line of UPSTREAM.md: no user album has an owner.
			owned := count(t, src, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZCLOUDOWNERHASHEDPERSONID IS NOT NULL")
			t.Logf("%d user albums with an owner", owned)
			if owned != 0 {
				t.Errorf("%d user albums with ZCLOUDOWNERHASHEDPERSONID", owned)
			}
		})
	}
	if albums == 0 || nested == 0 {
		t.Errorf("%d albums in all libraries, %d of them in a folder; want some of each", albums, nested)
	}
}

func TestAlbumsCloudGUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		library, marker, guid string
		onlyGUID              bool // the library has no iCloud: no other album has a GUID
	}{
		{"Test-13.0.0", "mig:album-cloud-guid", "TEST-ALBUM-GUID-0001", true},
		{"Test-26.1", "mig:album-cloud-guid", "TEST-ALBUM-GUID-0001", true},
		{"Test-Cloud-13.1", "mig:album-cloud-guid-null", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.library, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), testlib.Open(t, tt.library), Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, a := range db.albums {
				switch {
				case a.Title == tt.marker:
					found = true
					if a.CloudGUID != tt.guid {
						t.Errorf("%s: CloudGUID %q, want %q", tt.marker, a.CloudGUID, tt.guid)
					}
				case tt.onlyGUID && a.CloudGUID != "":
					t.Errorf("album %s: CloudGUID %q, want \"\"", a.UUID, a.CloudGUID)
				}
			}
			if !found {
				t.Fatalf("no album %s", tt.marker)
			}
		})
	}
}

// ZTRASHEDSTATE NULL is read as trashed by upstream too (photosdb.py:1936).
func TestAlbumsTrash(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "Test-15.4.1")
	n := schemaByID(t, libraryModels["Test-15.4.1"].id)
	rows, err := roDB(t, lib).QueryContext(t.Context(), "SELECT g.ZUUID FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND g.ZTRASHEDSTATE = 0 AND "+
		"EXISTS (SELECT 1 FROM "+n.MemberTable+" m WHERE m."+n.MemberAlbumColumn+" = g.Z_PK) ORDER BY g.ZUUID LIMIT 2")
	if err != nil {
		t.Fatal(err)
	}
	var picked []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		picked = append(picked, u)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if len(picked) != 2 {
		t.Fatalf("%d albums with members outside the trash, want 2", len(picked))
	}

	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	inTrash := map[string]bool{}
	for _, a := range before.albums {
		inTrash[a.UUID] = a.InTrash
	}
	photosOf := func(db *DB, album string) []string {
		var out []string
		for _, p := range db.Photos() {
			if slices.Contains(p.AlbumUUIDs, album) {
				out = append(out, p.UUID)
			}
		}
		return out
	}
	members := photosOf(before, picked[0])
	if len(members) == 0 {
		t.Fatal("the first album has no photos before the change")
	}

	change(t, lib,
		"UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = 1 WHERE ZUUID = '"+picked[0]+"'",
		"UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = NULL WHERE ZUUID = '"+picked[1]+"'",
	)
	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	inTrash[picked[0]], inTrash[picked[1]] = true, true
	got := after.albums
	if len(got) != len(inTrash) {
		t.Errorf("%d albums after the change, want %d", len(got), len(inTrash))
	}
	for _, a := range got {
		if want, ok := inTrash[a.UUID]; !ok || a.InTrash != want {
			t.Errorf("album %s: InTrash %v, want %v (known %v)", a.UUID, a.InTrash, want, ok)
		}
	}
	if m := photosOf(after, picked[0]); !slices.Equal(m, members) {
		t.Errorf("photos of the trashed album: %q, want %q", m, members)
	}
}

func TestOpenAlbumParents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(t *testing.T, lib string) string
		want string
	}{
		{"cycle", func(t *testing.T, lib string) string {
			var album, folder int64
			err := roDB(t, lib).QueryRowContext(t.Context(), "SELECT a.Z_PK, f.Z_PK FROM ZGENERICALBUM a JOIN ZGENERICALBUM f ON f.Z_PK = a.ZPARENTFOLDER "+
				"WHERE a.ZKIND = 2 AND f.ZKIND = 4000 ORDER BY a.Z_PK LIMIT 1").Scan(&album, &folder)
			if err != nil {
				t.Fatalf("an album in a folder of the synthetic library: %v", err)
			}
			return fmt.Sprintf("UPDATE ZGENERICALBUM SET ZPARENTFOLDER = %d WHERE Z_PK = %d", album, folder)
		}, "albums whose parent folders do not lead to the root folder"},
		{"no root", func(*testing.T, string) string { return "DELETE FROM ZGENERICALBUM WHERE ZKIND = 3999" },
			"0 root folders in ZGENERICALBUM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			uuids := albumUUIDs(t, lib)
			change(t, lib, tt.edit(t, lib))
			tmp := t.TempDir()
			db, err := Open(t.Context(), lib, Options{TempDir: tmp})
			assertNoCopies(t, tmp)
			if err == nil || db != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Open = %v, %v; want nil and an error with %q", db, err, tt.want)
			}
			for _, u := range uuids {
				if strings.Contains(err.Error(), u) {
					t.Errorf("error %q holds a ZUUID of ZGENERICALBUM", err)
				}
			}
		})
	}
}

func albumUUIDs(t *testing.T, lib string) []string {
	t.Helper()
	rows, err := roDB(t, lib).QueryContext(t.Context(), "SELECT ZUUID FROM ZGENERICALBUM WHERE ZUUID IS NOT NULL")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if len(out) == 0 {
		t.Fatal("no ZUUID in ZGENERICALBUM")
	}
	return out
}

func TestLoadSharedAlbums(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9"
	none := sql.NullInt64{}
	user := withTitle(albumRow(3, "AL-1", sqlread.KindUserAlbum, nullInt(2)), "Album")
	user.Trashed = nullInt(0)
	shared := withTitle(albumRow(4, "SH-A", sqlread.KindSharedAlbum, nullInt(1)), "Shared")
	shared.Trashed, shared.ParentUUID, shared.Owner, shared.ListOrder = nullInt(0), nullString("ROOT"), nullString(nfd), nullInt(3)
	// Photos never puts a shared album in a folder; osxphotos would not list it there either.
	inFolder := withTitle(albumRow(5, "SH-B", sqlread.KindSharedAlbum, nullInt(2)), "Shared")
	inFolder.Trashed, inFolder.ParentUUID = nullInt(0), nullString("F-1")
	trashed := withTitle(albumRow(6, "SH-C", sqlread.KindSharedAlbum, none), "Trashed")
	trashed.Trashed = nullInt(1)
	rows := []sqlread.Album{
		albumRow(1, "ROOT", sqlread.KindRootFolder, none),
		withTitle(albumRow(2, "F-1", sqlread.KindFolder, nullInt(1)), "F1"),
		user, shared, inFolder, trashed,
	}
	in := tables{
		assets: []sqlread.Asset{baseAsset(100, "P-1"), baseAsset(101, "P-2"), baseAsset(102, "P-3")},
		members: []sqlread.Member{
			{AssetPK: 100, AlbumUUID: "AL-1", Kind: sqlread.KindUserAlbum},
			{AssetPK: 100, AlbumUUID: "SH-A", Kind: sqlread.KindSharedAlbum},
			{AssetPK: 101, AlbumUUID: "SH-A", Kind: sqlread.KindSharedAlbum},
			{AssetPK: 102, AlbumUUID: "SH-C", Kind: sqlread.KindSharedAlbum},
		},
		albums: rows,
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	// Upstream keeps one list for both kinds (photoinfo.py:2022-2034).
	if got := c.byUUID["P-1"].AlbumUUIDs; !slices.Equal(got, []string{"AL-1", "SH-A"}) {
		t.Errorf("AlbumUUIDs of P-1 = %q, want the user album and the shared album", got)
	}
	albumPhotos := membershipOrder(in.members, in.assets, c.byUUID)
	albums, err := loadAlbums(rows, albumPhotos)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || albums[0].UUID != "AL-1" || albums[0].Owner != "" {
		t.Fatalf("albums %q, want AL-1 alone, without an owner", lookupUUIDs(albums))
	}
	got, err := loadFolderless(rows, albumPhotos, sqlread.KindSharedAlbum, "shared albums")
	if err != nil {
		t.Fatal(err)
	}
	want := []*albuminfo.AlbumInfo{
		{UUID: "SH-A", Title: "Shared", Owner: nfcForm, PhotoUUIDs: []string{"P-1", "P-2"}, LibraryListOrder: []int{3}},
		{UUID: "SH-B", Title: "Shared", ParentUUID: "F-1", LibraryListOrder: []int{noListOrder}},
		{UUID: "SH-C", Title: "Trashed", PhotoUUIDs: []string{"P-3"}, InTrash: true, LibraryListOrder: []int{noListOrder}},
	}
	if len(got) != len(want) {
		t.Fatalf("%d shared albums, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.UUID != w.UUID || g.Title != w.Title || g.Owner != w.Owner || g.ParentUUID != w.ParentUUID || g.FolderNames != nil ||
			!sameStringsNilAware(g.PhotoUUIDs, w.PhotoUUIDs) || g.InTrash != w.InTrash || !slices.Equal(g.LibraryListOrder, w.LibraryListOrder) {
			t.Errorf("shared album %d = %+v, want %+v", i, *g, *w)
		}
	}

	folders, _, err := loadFolders(rows, albums)
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{photos: c.photos, byUUID: c.byUUID, albums: albums, sharedAlbums: got, folders: folders}
	p1, p3 := c.byUUID["P-1"], c.byUUID["P-3"]
	for _, tt := range []struct {
		name      string
		got, want []string
	}{
		{"AlbumInfo", lookupUUIDs(db.AlbumInfo()), []string{"AL-1"}},
		{"SharedAlbumInfo", lookupUUIDs(db.SharedAlbumInfo()), []string{"SH-A", "SH-B"}},
		{"SharedAlbumTitles", db.SharedAlbumTitles(), []string{"Shared"}},
		{"AlbumTitles", db.AlbumTitles(), []string{"Album"}},
		{"AlbumInfoOf", lookupUUIDs(db.AlbumInfoOf(p1)), []string{"AL-1", "SH-A"}},
		{"AlbumInfoOf of a photo in a trashed shared album only", lookupUUIDs(db.AlbumInfoOf(p3)), []string{}},
		{"FolderList of a shared album with a folder as its parent", folderUUIDs(db.FolderList(got[1])), []string{}},
		{"Query InAlbum", queryIDs(db.Query(query.Options{InAlbum: true}), nil), []string{"P-1", "P-2"}},
		{"Query NotInAlbum", queryIDs(db.Query(query.Options{NotInAlbum: true}), nil), []string{"P-3"}},
		{"Query Album", queryIDs(db.Query(query.Options{Album: []string{"Shared"}}), nil), []string{"P-1", "P-2"}},
		{"Query Folder", queryIDs(db.Query(query.Options{Folder: []string{"F1"}}), nil), []string{"P-1"}},
	} {
		if !slices.Equal(tt.got, tt.want) {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
	if got := db.SharedAlbumCounts(); !maps.Equal(got, map[string]int{"Shared": 2}) {
		t.Errorf("SharedAlbumCounts = %v, want the two shared albums outside the trash added up under Shared", got)
	}
	if got := db.AlbumCounts(); !maps.Equal(got, map[string]int{"Album": 1}) {
		t.Errorf("AlbumCounts = %v, want the user album alone", got)
	}
	if a, ok := db.Album("SH-C"); !ok || !a.InTrash {
		t.Errorf("Album(SH-C) = %v, %v; want the shared album in the trash", a, ok)
	}
	if f, ok := db.Folder("F-1"); !ok || !slices.Equal(lookupUUIDs(f.Albums), []string{"AL-1"}) {
		t.Errorf("Folder(F-1) = %v, %v; want the user album alone in it", f, ok)
	}

	dangling := withTitle(albumRow(7, "SECRET-S", sqlread.KindSharedAlbum, nullInt(99)), "SECRET-TITLE")
	if bad, err := loadFolderless([]sqlread.Album{rows[0], dangling}, nil, sqlread.KindSharedAlbum, "shared albums"); err == nil || bad != nil ||
		!strings.Contains(err.Error(), "1 shared albums whose ZPARENTFOLDER is not a row") || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("a parent that is not a row: %d shared albums, %v; want an error without values", len(bad), err)
	}
}

// The CollectionShare rows of ZSHARE, Photos 11 and later (photosdb.py:2787-2916);
// 0 for a library without the entity, whose ZASSET may have no ZCOLLECTIONSHARE.
func collectionShares(t *testing.T, src *sql.DB, q string) int {
	t.Helper()
	if count(t, src, "SELECT COUNT(*) FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare'") == 0 {
		return 0
	}
	return count(t, src, strings.ReplaceAll(q, "CS_ENT", "(SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare')"))
}

const shareRows = " FROM ZSHARE WHERE Z_ENT = CS_ENT AND ZUUID IS NOT NULL"

const shareMemberCount = "SELECT COUNT(*) FROM ZASSET a JOIN ZSHARE s ON s.Z_PK = a.ZCOLLECTIONSHARE " +
	"WHERE s.Z_ENT = CS_ENT AND s.ZUUID IS NOT NULL AND a.Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)"

// The shared albums of Test-Cloud-13.1 are also in its reference, which
// TestGolden compares, and so is the CollectionShare of the synthetic library
// (reference/006_shared-album.sql); this checks every library against its
// database.
func TestSharedAlbums(t *testing.T) {
	t.Parallel()

	shared, owned, ofShare := 0, 0, 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)
			n := schemaByID(t, libraryModels[name].id)
			const rows = " FROM ZGENERICALBUM WHERE ZKIND = 1505 AND ZUUID IS NOT NULL"
			shares := collectionShares(t, src, "SELECT COUNT(*)"+shareRows)
			ofShare += shares
			if want := count(t, src, "SELECT COUNT(*)"+rows) + shares; len(db.sharedAlbums) != want {
				t.Errorf("%d shared albums, want %d", len(db.sharedAlbums), want)
			}
			// Upstream reads the state of a CollectionShare with bool() (photosdb.py:2906).
			if want := count(t, src, "SELECT COUNT(*)"+rows+" AND ZTRASHEDSTATE = 0") +
				collectionShares(t, src, "SELECT COUNT(*)"+shareRows+" AND COALESCE(ZTRASHEDSTATE, 0) = 0"); len(db.SharedAlbumInfo()) != want {
				t.Errorf("%d shared albums outside the trash, want %d", len(db.SharedAlbumInfo()), want)
			}
			members := 0
			for i, a := range db.sharedAlbums {
				if i > 0 && db.sharedAlbums[i-1].UUID >= a.UUID {
					t.Errorf("shared album %d out of UUID order or repeated", i+1)
				}
				members += len(a.PhotoUUIDs)
				if a.Owner != "" {
					owned++
				}
				if got, ok := db.Album(a.UUID); !ok || got != a || a.FolderNames != nil || db.FolderList(a) != nil {
					t.Errorf("shared album %s: not found by Album, or in folders", a.UUID)
				}
				parent := testlib.Rows(t, src, "SELECT p.ZUUID FROM ZGENERICALBUM g JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER "+
					"WHERE g.ZUUID = ? AND p.ZKIND != 3999", a.UUID)
				if want := strings.Join(parent, ""); a.ParentUUID != want {
					t.Errorf("shared album %s: ParentUUID %q, want %q", a.UUID, a.ParentUUID, want)
				}
			}
			if want := count(t, src, projectMembers(n.MemberTable, n.MemberAlbumColumn, n.MemberAssetColumn, "1505")) +
				collectionShares(t, src, shareMemberCount); members != want {
				t.Errorf("%d shared album members, want %d", members, want)
			}
			shared += len(db.sharedAlbums)
		})
	}
	t.Logf("%d shared albums in all libraries, %d with an owner, %d of ZSHARE", shared, owned, ofShare)
	if shared == 0 || owned == 0 || ofShare == 0 {
		t.Error("no shared album with an owner, or none of ZSHARE, in any library: the test is vacuous")
	}
}

// A shared album of ZSHARE keeps the order of the publish date
// (photosdb.py:2815-2842) and takes its owner from ZSHAREPARTICIPANT; the
// video of a shared album is missing when its own resource is
// (photosdb.py:2918-2942).
func TestOpenCollectionShare(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	src := roDB(t, lib)
	share := testlib.Rows(t, src, "SELECT ZUUID FROM ZSHARE WHERE Z_ENT = "+
		"(SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare') AND ZUUID IS NOT NULL")
	if len(share) != 1 {
		t.Fatalf("%d CollectionShare rows in the synthetic library, want the one of reference/006_shared-album.sql", len(share))
	}
	video := testlib.Rows(t, src, "SELECT ZUUID FROM ZASSET WHERE ZKIND = 1 AND ZTRASHEDSTATE = 0 AND ZCOLLECTIONSHARE IS NULL "+
		"AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES) ORDER BY ZUUID LIMIT 1")
	if len(video) != 1 {
		t.Fatal("no video outside the trash in the synthetic library")
	}
	const videoPK = "(SELECT Z_PK FROM ZASSET WHERE ZUUID = '"
	change(t, lib,
		"UPDATE ZINTERNALRESOURCE SET ZLOCALAVAILABILITY = 1 WHERE ZASSET = "+videoPK+video[0]+"')",
		"INSERT INTO ZINTERNALRESOURCE (Z_PK, ZASSET, ZDATASTORESUBTYPE, ZLOCALAVAILABILITY) "+
			"VALUES ((SELECT MAX(Z_PK) + 1 FROM ZINTERNALRESOURCE), "+videoPK+video[0]+"'), 8, 0)",
		"UPDATE ZASSET SET ZCOLLECTIONSHARE = (SELECT Z_PK FROM ZSHARE WHERE ZUUID = '"+share[0]+"') WHERE ZUUID = '"+video[0]+"'",
		fmt.Sprintf("INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZSHARE, ZROLE, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES "+
			"((SELECT COALESCE(MAX(Z_PK), 0) + 1 FROM ZSHAREPARTICIPANT), (SELECT Z_PK FROM ZSHARE WHERE ZUUID = '%s'), 1, 'TEST-P-OWNER', X'%x')",
			share[0], nameArchive(t, plist.UID(3), plist.UID(4))),
	)
	order := testlib.Rows(t, src, "SELECT a.ZUUID FROM ZASSET a WHERE a.ZCOLLECTIONSHARE = (SELECT Z_PK FROM ZSHARE WHERE ZUUID = ?)"+
		" AND a.Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES) ORDER BY a.ZCLOUDBATCHPUBLISHDATE, a.Z_PK", share[0])
	if len(order) < 3 || slices.IsSorted(order) {
		t.Fatalf("%d photos in the shared album, in UUID order or fewer than 3: the order is not checked", len(order))
	}

	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := db.Album(share[0])
	if !ok || a.InTrash || a.Owner != "Jos\U000000e9 Example" || !slices.Contains(lookupUUIDs(db.SharedAlbumInfo()), share[0]) {
		t.Fatalf("Album(%s) = %v: want a shared album outside the trash, with the owner written in NFC, in SharedAlbumInfo", share[0], ok)
	}
	var got []string
	for _, p := range db.AlbumPhotos(a) {
		got = append(got, p.UUID)
		if !slices.Contains(p.AlbumUUIDs, share[0]) {
			t.Errorf("photo %s: AlbumUUIDs without the shared album", p.UUID)
		}
	}
	if !slices.Equal(got, order) {
		t.Errorf("AlbumPhotos %q, want %q", got, order)
	}
	if p := db.byUUID[video[0]]; p == nil || !p.IsMissing {
		t.Error("the video of the shared album without its own resource: not found or IsMissing false")
	}
}

// Upstream skips an archive that does not decode (_photosdb_process_comments.py:175-179);
// a role that is not 1 is not the owner (_constants.py:372).
func TestOpenHostileOwner(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	share := testlib.Rows(t, roDB(t, lib), "SELECT ZUUID FROM ZSHARE WHERE Z_ENT = "+
		"(SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare') AND ZUUID IS NOT NULL")
	if len(share) != 1 {
		t.Fatalf("%d CollectionShare rows in the synthetic library, want the one of reference/006_shared-album.sql", len(share))
	}
	pk := "(SELECT Z_PK FROM ZSHARE WHERE ZUUID = '" + share[0] + "')"
	change(t, lib, "INSERT INTO ZSHAREPARTICIPANT (Z_PK, ZSHARE, ZROLE, ZHASHEDPERSONID, ZNAMECOMPONENTS) VALUES "+
		"(900001, "+pk+", 1, 'TEST-P-HOSTILE', 7), (900002, "+pk+", 1, 'TEST-P-HOSTILE', 'SECRET-N'), "+
		fmt.Sprintf("(900003, %s, 1, 'TEST-P-HOSTILE', X'%x'), ", pk, nameArchiveOf(t, plist.UID(3), plist.UID(4), strings.Repeat("a", maxPlistSize)))+
		fmt.Sprintf("(900004, %s, 'SECRET-R', 'TEST-P-ROLE', X'%x')", pk, nameArchive(t, plist.UID(3), plist.UID(4))))
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: an error, with a value %v", strings.Contains(err.Error(), "SECRET"))
	}
	if a, ok := db.Album(share[0]); !ok || a.Owner != "" {
		t.Errorf("Album(%s) = %v: want the shared album without an owner", share[0], ok)
	}
}

func TestLoadCollectionShares(t *testing.T) {
	t.Parallel()

	none := sql.NullInt64{}
	rows := []sqlread.Album{albumRow(1, "ROOT", sqlread.KindRootFolder, none), albumRow(2, "SH-1", sqlread.KindSharedAlbum, none)}
	// A ZSHARE.Z_PK may equal a ZGENERICALBUM.Z_PK (photosdb.py:2914-2916): a guard against a map by Z_PK.
	share := albumRow(1, "SH-2", sqlread.KindSharedAlbum, none)
	share.Trashed = nullInt(0)
	got, err := loadFolderless(append(slices.Clone(rows), share), map[string][]string{"SH-2": {"P-1"}}, sqlread.KindSharedAlbum, "shared albums")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].UUID != "SH-2" || got[1].InTrash || !slices.Equal(got[1].PhotoUUIDs, []string{"P-1"}) ||
		got[1].ParentUUID != "" || !slices.Equal(got[1].LibraryListOrder, []int{noListOrder}) {
		t.Errorf("shared albums %q, want SH-1 and SH-2 with its photo, outside the trash, without a parent", lookupUUIDs(got))
	}
	for _, tt := range []struct {
		name  string
		share sqlread.Album
	}{
		{"the ZUUID of ZGENERICALBUM", albumRow(3, "SECRET-1", sqlread.KindSharedAlbum, none)},
		{"an empty ZUUID", albumRow(3, "", sqlread.KindSharedAlbum, none)},
	} {
		withShare := append(slices.Clone(rows), albumRow(4, "SECRET-1", sqlread.KindUserAlbum, nullInt(1)), tt.share)
		bad, err := loadFolderless(withShare, nil, sqlread.KindSharedAlbum, "shared albums")
		if err == nil || bad != nil || !strings.Contains(err.Error(), "1 shared albums with an empty ZUUID or the ZUUID of another album") ||
			strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: %d shared albums, %v; want an error without values", tt.name, len(bad), err)
		}
	}
}

// The archive has the keys that upstream decodes (shareparticipant.py:90-117).
func nameArchive(t *testing.T, given, family plist.UID) []byte {
	t.Helper()
	return nameArchiveOf(t, given, family, "Jose\U00000301")
}

func nameArchiveOf(t *testing.T, given, family plist.UID, firstName string) []byte {
	t.Helper()
	class := func(name string) map[string]any {
		return map[string]any{"$classname": name, "$classes": []any{name, "NSObject"}}
	}
	b, err := plist.Marshal(map[string]any{
		"$archiver": "NSKeyedArchiver", "$version": 100000,
		"$top": map[string]any{"root": plist.UID(1)},
		"$objects": []any{
			"$null",
			map[string]any{"$class": plist.UID(5), "NSnameComponentsPrivate": plist.UID(2)},
			map[string]any{"$class": plist.UID(6), "NSgivenName": given, "NSfamilyName": family, "NSmiddleName": plist.UID(3)},
			firstName, "Example",
			class("NSPersonNameComponents"), class("_NSPersonNameComponentsData"),
		},
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Upstream joins the given and the family name and skips an archive without
// a name or one that does not decode (_photosdb_process_comments.py:172-187).
func TestOwnerName(t *testing.T) {
	t.Parallel()

	const given, family, null = plist.UID(3), plist.UID(4), plist.UID(0)
	full := nameArchive(t, given, family)
	oversized := nameArchiveOf(t, given, family, strings.Repeat("a", maxPlistSize))
	xml, err := plist.Marshal(map[string]any{"$objects": []any{"$null"}}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		album sqlread.Album
		want  string
	}{
		{"the name of the UNION", sqlread.Album{Owner: nullString("Jose\U00000301 Example")}, "Jos\U000000e9 Example"},
		{"no owner", sqlread.Album{}, ""},
		{"given and family name", sqlread.Album{OwnerComponents: [][]byte{full}}, "Jos\U000000e9 Example"},
		{"given name", sqlread.Album{OwnerComponents: [][]byte{nameArchive(t, given, null)}}, "Jos\U000000e9"},
		{"family name", sqlread.Album{OwnerComponents: [][]byte{nameArchive(t, null, family)}}, "Example"},
		{"no name, then a name", sqlread.Album{OwnerComponents: [][]byte{nameArchive(t, null, null), xml, nameArchive(t, null, family)}}, "Example"},
		{"not a property list, then a name", sqlread.Album{OwnerComponents: [][]byte{[]byte("not a list"), nil, full}}, "Jos\U000000e9 Example"},
		{"a reference out of range", sqlread.Album{OwnerComponents: [][]byte{nameArchive(t, plist.UID(99), null)}}, ""},
		{"an archive over the limit", sqlread.Album{OwnerComponents: [][]byte{oversized}}, ""},
	} {
		if got := ownerName(tt.album); got != tt.want {
			t.Errorf("%s: another name", tt.name)
		}
	}
}

// The video of a shared album is a resource apart from the poster frame
// (photosdb.py:2918-2942); of several, the one with the highest Z_PK decides,
// as in isMissing.
func TestLoadSharedVideos(t *testing.T) {
	t.Parallel()

	asset := func(pk int64, uuid string, kind int64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		a.Kind = nullInt(kind)
		return a
	}
	res := func(pk, asset, subtype, local int64) sqlread.Resource {
		return sqlread.Resource{PK: pk, AssetPK: asset, Subtype: nullInt(subtype), Local: nullInt(local)}
	}
	in := tables{
		assets: []sqlread.Asset{
			asset(1, "V-FOUND", kindMovie), asset(2, "V-MISSING", kindMovie), asset(3, "PHOTO", kindPhoto),
			asset(4, "V-NOT-SHARED", kindMovie), asset(5, "V-NO-VIDEO", kindMovie),
		},
		resources: []sqlread.Resource{
			res(10, 1, 1, 0), res(11, 1, 8, 0), res(12, 1, 8, 1),
			res(13, 2, 1, 1), res(14, 2, 8, 1), res(15, 2, 8, 0),
			res(16, 3, 1, 1), res(17, 3, 8, 0),
			res(18, 4, 1, 1), res(19, 4, 8, 0),
			res(20, 5, 1, 0), res(21, 5, 2, 1),
		},
		members: []sqlread.Member{{AssetPK: 4, AlbumUUID: "ALBUM", Kind: sqlread.KindUserAlbum}},
		shareMembers: []sqlread.Member{
			{AssetPK: 1, AlbumUUID: "SH"}, {AssetPK: 2, AlbumUUID: "SH"}, {AssetPK: 3, AlbumUUID: "SH"}, {AssetPK: 5, AlbumUUID: "SH"},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"V-FOUND": false, "V-MISSING": true, "PHOTO": false, "V-NOT-SHARED": false, "V-NO-VIDEO": true}
	for u, w := range want {
		if p := c.byUUID[u]; p == nil || p.IsMissing != w {
			t.Errorf("%s: not loaded, or IsMissing not %v", u, w)
		}
	}
}
