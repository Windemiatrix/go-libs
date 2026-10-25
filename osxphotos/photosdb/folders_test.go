package photosdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// folderRow is a folder row; an empty guid is a NULL ZCLOUDGUID.
func folderRow(pk int64, uuid string, parent, trashed sql.NullInt64, title, guid string) sqlread.Album {
	r := withTitle(albumRow(pk, uuid, sqlread.KindFolder, parent), title)
	r.Trashed = trashed
	if guid != "" {
		r.CloudGUID = nullString(guid)
	}
	return r
}

// folderAlbum is a user album row in the folder with this Z_PK.
func folderAlbum(pk int64, uuid string, parent int64, trashed sql.NullInt64) sqlread.Album {
	r := withTitle(albumRow(pk, uuid, sqlread.KindUserAlbum, nullInt(parent)), "Title of "+uuid)
	r.Trashed = trashed
	return r
}

// folderShape is a folder with its children as UUIDs, to compare as a
// literal; no children is nil.
type folderShape struct {
	uuid, cloudGUID, title, parent string
	subfolders, albums             []string
}

func folderRef(f *albuminfo.FolderInfo) folderShape {
	s := folderShape{uuid: f.UUID, cloudGUID: f.CloudGUID, title: f.Title, parent: f.ParentUUID, subfolders: folderUUIDs(f.Subfolders)}
	for _, a := range f.Albums {
		s.albums = append(s.albums, a.UUID)
	}
	return s
}

// folderUUIDs returns nil for no folders.
func folderUUIDs(fs []*albuminfo.FolderInfo) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.UUID)
	}
	return out
}

func folderTitles(fs []*albuminfo.FolderInfo) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Title)
	}
	return out
}

// hardenPlace is where an album lies: the UUIDs of its FolderList and its
// FolderNames.
type hardenPlace struct{ list, names []string }

func hardenPlaceOf(db *DB, a *albuminfo.AlbumInfo) hardenPlace {
	return hardenPlace{folderUUIDs(db.FolderList(a)), slices.Clone(a.FolderNames)}
}

func TestLoadFolders(t *testing.T) {
	t.Parallel()

	// The same title in the decomposed and in the composed form.
	nfd, nfcForm := "Cafe\U00000301", "Caf\U000000e9"
	if nfd == nfcForm {
		t.Fatal("the two forms of the title are one string")
	}
	none, root := sql.NullInt64{}, int64(1)
	f2 := folderRow(3, "F-2", nullInt(2), nullInt(0), "F2", "")
	f2.CloudGUID = nullString("")
	rows := []sqlread.Album{
		albumRow(root, "ROOT", sqlread.KindRootFolder, none),
		folderRow(2, "F-1", nullInt(root), nullInt(0), nfd, "G-1"),
		f2,
		folderRow(4, "F-S", nullInt(2), nullInt(1), "FS", ""),
		folderRow(5, "F-T", nullInt(root), nullInt(1), "FT", ""),
		folderRow(6, "F-N", nullInt(root), none, "FN", ""),
		folderRow(7, "F-P", none, nullInt(0), "FP", ""),
		folderRow(8, "F-E", nullInt(root), nullInt(0), "Same", ""),
		folderRow(9, "F-E2", nullInt(root), nullInt(0), "Same", ""),
		folderAlbum(10, "A-1", 2, nullInt(0)),
		folderAlbum(11, "A-T", 2, nullInt(1)),
		folderAlbum(12, "A-2", 3, nullInt(0)),
		folderAlbum(13, "A-R", root, nullInt(0)),
		// Albums in the folders in the trash: the folder state does not touch them.
		folderAlbum(14, "A-FT", 5, nullInt(0)),
		folderAlbum(15, "A-FS", 4, nullInt(0)),
	}
	albums, err := loadAlbums(rows, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	album := func(uuid string) *albuminfo.AlbumInfo {
		t.Helper()
		for _, a := range albums {
			if a.UUID == uuid {
				return a
			}
		}
		t.Fatalf("no album %s", uuid)
		return nil
	}

	all, top, err := loadFolders(rows, albums)
	if err != nil {
		t.Fatal(err)
	}

	wantAll := []folderShape{
		{uuid: "F-1", cloudGUID: "G-1", title: nfcForm, subfolders: []string{"F-2"}, albums: []string{"A-1"}},
		{uuid: "F-2", title: "F2", parent: "F-1", albums: []string{"A-2"}},
		{uuid: "F-E", title: "Same"},
		{uuid: "F-E2", title: "Same"},
		{uuid: "F-N", title: "FN"},
		{uuid: "F-P", title: "FP"},
		{uuid: "F-S", title: "FS", parent: "F-1", albums: []string{"A-FS"}},
		{uuid: "F-T", title: "FT", albums: []string{"A-FT"}},
	}
	var gotAll []folderShape
	for _, f := range all {
		gotAll = append(gotAll, folderRef(f))
	}
	// Fatal: the checks below index into all and top.
	if !reflect.DeepEqual(gotAll, wantAll) {
		t.Fatalf("folders\n got %+v\nwant %+v", gotAll, wantAll)
	}
	if want := []string{"F-1", "F-E", "F-E2"}; !slices.Equal(folderUUIDs(top), want) {
		t.Fatalf("top-level folders %q, want %q", folderUUIDs(top), want)
	}
	if all[0].Albums[0] != album("A-1") || all[0].Subfolders[0] != all[1] {
		t.Error("Albums and Subfolders of F-1 are copies, not the values of albums and folders")
	}

	db := &DB{albums: albums, folders: all, topFolders: top}
	if got := db.FolderInfo(); !slices.Equal(folderUUIDs(got), []string{"F-1", "F-E", "F-E2"}) || got[0] != all[0] {
		t.Errorf("FolderInfo = %q, want F-1, F-E, F-E2 as the values of folders", folderUUIDs(got))
	}
	db.FolderInfo()[0] = nil
	if db.topFolders[0] == nil {
		t.Error("FolderInfo returns the slice of the DB")
	}
	if got, want := db.FolderTitles(), []string{nfcForm, "Same", "Same"}; !slices.Equal(got, want) {
		t.Errorf("FolderTitles = %q, want %q", got, want)
	}
	if f, ok := db.Folder("F-T"); !ok || f != all[7] {
		t.Errorf("Folder(F-T) = %v, %v; want the folder in the trash", f, ok)
	}
	for _, u := range []string{"A-1", "ROOT", "", "F-"} {
		if f, ok := db.Folder(u); ok || f != nil {
			t.Errorf("Folder(%q) = %v, %v; want nothing", u, f, ok)
		}
	}

	lists := map[string][]string{"A-2": {"F-1", "F-2"}, "A-1": {"F-1"}, "A-R": nil, "A-T": {"F-1"}, "A-FT": {"F-T"}, "A-FS": {"F-1", "F-S"}}
	for u, want := range lists {
		got := db.FolderList(album(u))
		if !slices.Equal(folderUUIDs(got), want) || (want == nil) != (got == nil) {
			t.Errorf("FolderList(%s) = %q, want %q", u, folderUUIDs(got), want)
		}
	}
	for _, a := range albums {
		if got := folderTitles(db.FolderList(a)); !slices.Equal(got, a.FolderNames) {
			t.Errorf("FolderList(%s) titles %q, FolderNames %q", a.UUID, got, a.FolderNames)
		}
	}
}

func TestLoadFoldersErrors(t *testing.T) {
	t.Parallel()

	none := sql.NullInt64{}
	rootRow := albumRow(1, "SECRET-ROOT", sqlread.KindRootFolder, none)
	folder := func(pk int64, uuid string, parent sql.NullInt64) sqlread.Album {
		return folderRow(pk, uuid, parent, nullInt(0), "SECRET-TITLE", "SECRET-GUID")
	}
	const want = "folders whose parent folders do not lead to the root folder"
	tests := []struct {
		name string
		rows []sqlread.Album
		bad  int
	}{
		{"cycle of two folders", []sqlread.Album{rootRow, folder(2, "SECRET-F1", nullInt(3)), folder(3, "SECRET-F2", nullInt(2))}, 2},
		{"dangling parent", []sqlread.Album{rootRow, folder(2, "SECRET-F1", nullInt(99))}, 1},
		{"folder is its own parent", []sqlread.Album{rootRow, folder(2, "SECRET-F1", nullInt(2))}, 1},
		{"folders below a cycle", []sqlread.Album{
			rootRow, folder(2, "SECRET-F1", nullInt(3)), folder(3, "SECRET-F2", nullInt(2)),
			folder(4, "SECRET-F3", nullInt(2)), folder(5, "SECRET-F4", nullInt(1)),
		}, 3},
	}
	for _, tt := range tests {
		albums, err := loadAlbums(tt.rows, nil, nil, nil)
		if err != nil {
			t.Fatalf("%s: loadAlbums: %v; the case must pass it", tt.name, err)
		}
		all, top, err := loadFolders(tt.rows, albums)
		if wantText := fmt.Sprintf("photosdb: %d %s", tt.bad, want); err == nil || err.Error() != wantText || all != nil || top != nil {
			t.Errorf("%s: loadFolders = %d folders, %d at the top, %v; want nothing and %q", tt.name, len(all), len(top), err, wantText)
		}
		if err != nil && strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: error %q holds a value of the database", tt.name, err)
		}
	}
}

// TestFolders checks the folders against direct SQL on every test library.
func TestFolders(t *testing.T) {
	t.Parallel()

	const (
		topFrom = " FROM ZGENERICALBUM f JOIN ZGENERICALBUM r ON r.Z_PK = f.ZPARENTFOLDER " +
			"WHERE r.ZKIND = 3999 AND f.ZKIND = 4000 AND f.ZUUID IS NOT NULL AND f.ZTRASHEDSTATE = 0"
		childOf = "FROM ZGENERICALBUM c JOIN ZGENERICALBUM f ON f.Z_PK = c.ZPARENTFOLDER WHERE c.ZUUID IS NOT NULL AND c.ZTRASHEDSTATE = 0 AND "
	)
	folders, nonEmptyLists, nested := 0, 0, 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)

			top := db.FolderInfo()
			if want := count(t, src, "SELECT COUNT(*)"+topFrom); len(top) != want {
				t.Errorf("%d top-level folders, want %d", len(top), want)
			}
			if want := testlib.Rows(t, src, "SELECT f.ZUUID"+topFrom+" ORDER BY f.ZUUID"); !slices.Equal(folderUUIDs(top), want) {
				t.Errorf("FolderInfo has other folders, or another order, than the SQL")
			}
			if got := db.FolderTitles(); !slices.Equal(got, folderTitles(top)) {
				t.Errorf("FolderTitles differ from the titles of FolderInfo")
			}

			byAlbum := map[string]*albuminfo.AlbumInfo{}
			for _, a := range db.Albums() {
				byAlbum[a.UUID] = a
			}
			all := testlib.Rows(t, src, "SELECT ZUUID FROM ZGENERICALBUM WHERE ZKIND = 4000 AND ZUUID IS NOT NULL ORDER BY ZUUID")
			folders += len(all)
			inTrash := count(t, src, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 4000 AND ZUUID IS NOT NULL "+
				"AND (ZTRASHEDSTATE IS NULL OR ZTRASHEDSTATE != 0)")
			subfolders := 0

			for _, u := range all {
				f, ok := db.Folder(u)
				if !ok {
					t.Errorf("Folder(%s) found nothing", u)
					continue
				}
				if want := testlib.Scalar(t, src, "SELECT COALESCE(p.ZUUID, '') FROM ZGENERICALBUM f LEFT JOIN ZGENERICALBUM p "+
					"ON p.Z_PK = f.ZPARENTFOLDER AND p.ZKIND != 3999 WHERE f.ZUUID = '"+u+"'"); f.ParentUUID != want {
					t.Errorf("folder %s: ParentUUID %q, want %q", u, f.ParentUUID, want)
				}
				if want := testlib.Scalar(t, src, "SELECT COALESCE(ZCLOUDGUID, '') FROM ZGENERICALBUM WHERE ZUUID = '"+u+"'"); f.CloudGUID != want {
					t.Errorf("folder %s: CloudGUID differs from ZCLOUDGUID", u)
				}
				if want := count(t, src, "SELECT COUNT(*) "+childOf+"c.ZKIND = 4000 AND f.ZUUID = '"+u+"'"); len(f.Subfolders) != want {
					t.Errorf("folder %s: %d subfolders, want %d", u, len(f.Subfolders), want)
				}
				if want := count(t, src, "SELECT COUNT(*) "+childOf+"c.ZKIND = 2 AND f.ZUUID = '"+u+"'"); len(f.Albums) != want {
					t.Errorf("folder %s: %d albums, want %d", u, len(f.Albums), want)
				}
				subfolders += len(f.Subfolders)
				for i, s := range f.Subfolders {
					if same, _ := db.Folder(s.UUID); same != s || s.ParentUUID != u || (i > 0 && f.Subfolders[i-1].UUID >= s.UUID) {
						t.Errorf("folder %s: subfolder %d is not the folder %s, is not in it, or is out of UUID order", u, i, s.UUID)
					}
				}
				for i, a := range f.Albums {
					if a != byAlbum[a.UUID] || a.ParentUUID != u || a.InTrash || (i > 0 && f.Albums[i-1].UUID >= a.UUID) {
						t.Errorf("folder %s: album %d is not the value of Albums(), is not in it, is in the trash or is out of UUID order", u, i)
					}
				}
			}
			t.Logf("%d folders, %d at the top level outside the trash, %d in the trash, %d nested folders outside the trash, %d albums",
				len(all), len(top), inTrash, subfolders, len(byAlbum))
			notFolders := testlib.Rows(t, src, "SELECT ZUUID FROM ZGENERICALBUM WHERE ZKIND != 4000 AND ZUUID IS NOT NULL")
			for _, u := range notFolders {
				if f, ok := db.Folder(u); ok || f != nil {
					t.Errorf("Folder found a root folder, an album or another kind of row")
				}
			}

			for _, a := range db.Albums() {
				list := db.FolderList(a)
				if got := folderTitles(list); !slices.Equal(got, a.FolderNames) {
					t.Errorf("album %s: the titles of FolderList differ from FolderNames", a.UUID)
				}
				switch {
				case a.ParentUUID == "" && len(list) != 0:
					t.Errorf("album %s: FolderList of a top-level album has %d folders", a.UUID, len(list))
				case a.ParentUUID != "" && (len(list) == 0 || list[len(list)-1].UUID != a.ParentUUID):
					t.Errorf("album %s: the last folder of FolderList is not its parent", a.UUID)
				}
				if len(list) > 0 {
					nonEmptyLists++
				}
				if len(list) > 1 {
					nested++
				}
			}
		})
	}
	t.Logf("%d folders in all libraries, %d albums with a folder, %d of them in a folder in a folder", folders, nonEmptyLists, nested)
	if folders == 0 || nonEmptyLists == 0 {
		t.Errorf("%d folders and %d albums in a folder in all libraries; want some of each", folders, nonEmptyLists)
	}
}

// folderAlbumInfo is the part of an album_info element of the reference that
// holds the folders.
type folderAlbumInfo struct {
	UUID        string          `json:"uuid"`
	Owner       json.RawMessage `json:"owner"`
	FolderList  []string        `json:"folder_list"`
	FolderNames []*string       `json:"folder_names"`
}

// TestFoldersGolden compares FolderList and the parents of its folders with
// folder_list and folder_names of the osxphotos album_info. The reference
// does not cover folders without albums, folders in the trash, the order of
// the lists and the CloudGUID of a folder: TestLoadFolders and TestFolders
// hold them.
func TestFoldersGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	listed := 0
	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := golden.Load(dir, lib.name)
			if err != nil {
				t.Fatal(err)
			}
			byAlbum := map[string]*albuminfo.AlbumInfo{}
			for _, a := range db.Albums() {
				byAlbum[a.UUID] = a
			}
			for _, obj := range slices.Concat(ref.Query, ref.Deleted) {
				var elements []folderAlbumInfo
				if json.Unmarshal(obj["album_info"], &elements) != nil {
					t.Fatal("album_info of an object is not a list of objects")
				}
				for _, e := range elements {
					if len(e.Owner) > 0 && string(e.Owner) != "null" { // shared album, rule 8 of NORMALIZATION.md
						continue
					}
					a, ok := byAlbum[e.UUID]
					if !ok {
						t.Errorf("album %s of album_info is not in Albums()", e.UUID)
						continue
					}
					names := make([]string, len(e.FolderNames))
					for i, n := range e.FolderNames {
						if n != nil {
							names[i] = *n
						}
					}
					list := db.FolderList(a)
					if !slices.Equal(folderUUIDs(list), e.FolderList) {
						t.Errorf("album %s: FolderList %q, folder_list %q", e.UUID, folderUUIDs(list), e.FolderList)
					}
					if !slices.Equal(folderTitles(list), names) {
						t.Errorf("album %s: titles of FolderList differ from folder_names", e.UUID)
					}
					parent := ""
					for _, id := range e.FolderList {
						f, ok := db.Folder(id)
						if !ok || f.ParentUUID != parent {
							t.Errorf("album %s: folder %s is not found or its ParentUUID is not the folder before it", e.UUID, id)
							break
						}
						parent = id
					}
					if len(e.FolderList) > 0 {
						listed++
					}
				}
			}
		})
	}
	if listed == 0 {
		t.Error("no album_info element of any reference has a folder_list")
	}
}

// TestFoldersTrash puts a top-level folder, and a folder in it when there is
// one, into the trash of a copy.
func TestFoldersTrash(t *testing.T) {
	t.Parallel()

	const topOrder = "SELECT f.ZUUID FROM ZGENERICALBUM f JOIN ZGENERICALBUM r ON r.Z_PK = f.ZPARENTFOLDER " +
		"WHERE r.ZKIND = 3999 AND f.ZKIND = 4000 AND f.ZUUID IS NOT NULL AND f.ZTRASHEDSTATE = 0 " +
		"ORDER BY EXISTS (SELECT 1 FROM ZGENERICALBUM c WHERE c.ZPARENTFOLDER = f.Z_PK AND c.ZKIND = 4000 AND c.ZUUID IS NOT NULL " +
		"AND c.ZTRASHEDSTATE = 0) DESC, f.ZUUID LIMIT 1"

	var lib, top, sub string
	for _, name := range testlib.Libraries(t) {
		l := testlib.OpenPristine(t, name)
		src := roDB(t, l)
		tops := testlib.Rows(t, src, topOrder)
		if len(tops) == 0 {
			continue
		}
		subs := testlib.Rows(t, src, "SELECT c.ZUUID FROM ZGENERICALBUM c JOIN ZGENERICALBUM f ON f.Z_PK = c.ZPARENTFOLDER "+
			"WHERE f.ZUUID = '"+tops[0]+"' AND c.ZKIND = 4000 AND c.ZUUID IS NOT NULL AND c.ZTRASHEDSTATE = 0 ORDER BY c.ZUUID LIMIT 1")
		if lib == "" || len(subs) > 0 {
			lib, top, sub = l, tops[0], ""
			if len(subs) > 0 {
				sub = subs[0]
			}
		}
		if sub != "" {
			break
		}
	}
	if lib == "" {
		t.Fatal("no test library has a top-level folder outside the trash")
	}
	t.Logf("a folder in the folder to put into the trash: %v", sub != "")

	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	topBefore, ok := before.Folder(top)
	if !ok {
		t.Fatal("Folder does not find the top-level folder")
	}
	shapeBefore := folderRef(topBefore)
	var subShape folderShape
	placed := slices.Clone(topBefore.Albums)
	if sub != "" {
		f, ok := before.Folder(sub)
		if !ok {
			t.Fatal("Folder does not find the subfolder")
		}
		subShape = folderRef(f)
		placed = append(placed, f.Albums...)
	}
	wantPlace := map[string]hardenPlace{}
	for _, a := range placed {
		wantPlace[a.UUID] = hardenPlaceOf(before, a)
	}
	t.Logf("%d albums in the folders to put into the trash", len(placed))
	topsBefore := before.FolderInfo()
	at := slices.IndexFunc(topsBefore, func(f *albuminfo.FolderInfo) bool { return f.UUID == top })
	if at < 0 {
		t.Fatal("the top-level folder is not in FolderInfo before the change")
	}
	wantUUIDs := slices.Delete(folderUUIDs(topsBefore), at, at+1)
	wantTitles := slices.Delete(folderTitles(topsBefore), at, at+1)

	stmts := []string{"UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = 1 WHERE ZUUID = '" + top + "'"}
	wantSubs := shapeBefore.subfolders
	if sub != "" {
		stmts = append(stmts, "UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = 1 WHERE ZUUID = '"+sub+"'")
		wantSubs = slices.DeleteFunc(slices.Clone(wantSubs), func(u string) bool { return u == sub })
	}
	change(t, lib, stmts...)
	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	if got := folderUUIDs(after.FolderInfo()); !slices.Equal(got, wantUUIDs) {
		t.Errorf("FolderInfo after the change %q, want %q", got, wantUUIDs)
	}
	if got := after.FolderTitles(); !slices.Equal(got, wantTitles) {
		t.Errorf("FolderTitles after the change are not the titles before it without the folder put into the trash")
	}
	topAfter, ok := after.Folder(top)
	if !ok {
		t.Fatal("Folder does not find the folder in the trash")
	}
	shapeAfter := folderRef(topAfter)
	if !slices.Equal(shapeAfter.albums, shapeBefore.albums) || !slices.Equal(shapeAfter.subfolders, wantSubs) {
		t.Errorf("the folder in the trash: albums %q, subfolders %q; want %q, %q",
			shapeAfter.albums, shapeAfter.subfolders, shapeBefore.albums, wantSubs)
	}
	for _, a := range placed {
		got, found := after.Album(a.UUID)
		if !found {
			t.Errorf("album %s of a folder in the trash is not found after the change", a.UUID)
			continue
		}
		if place := hardenPlaceOf(after, got); !reflect.DeepEqual(place, wantPlace[a.UUID]) {
			t.Errorf("album %s: FolderList %q and FolderNames %q after the change, want %q and %q",
				a.UUID, place.list, place.names, wantPlace[a.UUID].list, wantPlace[a.UUID].names)
		}
	}
	if sub == "" {
		return
	}
	f, ok := after.Folder(sub)
	if !ok {
		t.Fatal("Folder does not find the subfolder in the trash")
	}
	if got := folderRef(f); !reflect.DeepEqual(got, subShape) {
		t.Errorf("the subfolder in the trash: %+v, want the shape before the change %+v", got, subShape)
	}
}

// TestOpenFolderCycle adds two folders that are the parents of each other to
// a copy: Open fails, names no UUID and removes the copy.
func TestOpenFolderCycle(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	uuids := albumUUIDs(t, lib)
	src := roDB(t, lib)
	next := count(t, src, "SELECT MAX(Z_PK) FROM ZGENERICALBUM") + 1
	entity := count(t, src, "SELECT Z_ENT FROM ZGENERICALBUM WHERE ZKIND = 4000 LIMIT 1")
	const insert = "INSERT INTO ZGENERICALBUM (Z_PK, Z_ENT, Z_OPT, ZKIND, ZUUID, ZPARENTFOLDER, ZTRASHEDSTATE) VALUES (%d, %d, 1, 4000, 'SECRET-CYCLE-%d', %d, 0)"
	change(t, lib, fmt.Sprintf(insert, next, entity, 1, next+1), fmt.Sprintf(insert, next+1, entity, 2, next))

	tmp := t.TempDir()
	db, err := Open(t.Context(), lib, Options{TempDir: tmp})
	assertNoCopies(t, tmp)
	const want = "2 folders whose parent folders do not lead to the root folder"
	if err == nil || db != nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Open = %v, %v; want nil and an error with %q", db, err, want)
	}
	for _, u := range append(uuids, "SECRET-CYCLE") {
		if strings.Contains(err.Error(), u) {
			t.Errorf("error %q holds a ZUUID of ZGENERICALBUM", err)
		}
	}
}
