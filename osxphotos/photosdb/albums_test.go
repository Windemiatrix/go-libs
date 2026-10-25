package photosdb

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
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

	got, err := loadAlbums(rows, members, assets, byUUID)
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
		got, err := loadAlbums(tt.rows, nil, nil, nil)
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

			got := db.Albums()
			albums += len(got)
			if want := count(t, src, "SELECT COUNT(*)"+user); len(got) != want {
				t.Errorf("%d albums, want %d", len(got), want)
			}
			byUUID := map[string]*albuminfo.AlbumInfo{}
			trashed, guids, members := 0, 0, 0
			for i, a := range got {
				if i > 0 && got[i-1].UUID >= a.UUID {
					t.Errorf("album %d out of UUID order or repeated", i+1)
				}
				byUUID[a.UUID] = a
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
				for _, u := range slices.Concat(p.AlbumUUIDs, p.BurstAlbumUUIDs) {
					if byUUID[u] == nil {
						t.Errorf("an album of photo %s is not in Albums()", p.UUID)
					}
				}
				for _, u := range p.AlbumUUIDs {
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
			for _, a := range db.Albums() {
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
	for _, a := range before.Albums() {
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
	got := after.Albums()
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
