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

// parentUUID is what the LEFT JOIN of sqlread.Albums gives; "" is NULL.
func projectRow(pk int64, uuid string, parent sql.NullInt64, parentUUID string) sqlread.Album {
	r := albumRow(pk, uuid, sqlread.KindProject, parent)
	r.Trashed = nullInt(0)
	if parentUUID != "" {
		r.ParentUUID = nullString(parentUUID)
	}
	return r
}

func TestLoadProjects(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9"
	none := sql.NullInt64{}
	inTrash := withTitle(projectRow(5, "PR-B", nullInt(1), "ROOT"), "Card")
	inTrash.Trashed = nullInt(1)
	untitled := projectRow(6, "PR-A", nullInt(2), "F-1")
	// The parent is of a ZKIND that sqlread.Albums does not read.
	noPhotos := withTitle(projectRow(7, "PR-C", nullInt(9), "PROJECTS"), nfd)
	noPhotos.CloudGUID, noPhotos.Created, noPhotos.ListOrder = nullString("GUID-P"), nullFloat(694224000), nullInt(4)
	noPhotos.SortKey, noPhotos.SortAscending = nullInt(1), nullInt(0)
	rows := []sqlread.Album{
		albumRow(1, "ROOT", sqlread.KindRootFolder, none),
		withTitle(albumRow(2, "F-1", sqlread.KindFolder, nullInt(1)), "F1"),
		withTitle(albumRow(3, "AL-1", sqlread.KindUserAlbum, nullInt(2)), "Album"),
		inTrash, untitled, noPhotos,
	}
	in := tables{
		assets: []sqlread.Asset{baseAsset(100, "P-1"), baseAsset(101, "P-2")},
		members: []sqlread.Member{
			{AssetPK: 100, AlbumUUID: "AL-1", Kind: sqlread.KindUserAlbum},
			{AssetPK: 100, AlbumUUID: "PR-A", Kind: sqlread.KindProject},
			{AssetPK: 101, AlbumUUID: "PR-A", Kind: sqlread.KindProject},
			{AssetPK: 101, AlbumUUID: "PR-B", Kind: sqlread.KindProject},
		},
		albums: rows,
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := c.byUUID["P-1"], c.byUUID["P-2"]
	if !slices.Equal(p1.AlbumUUIDs, []string{"AL-1"}) || p2.AlbumUUIDs != nil ||
		!slices.Equal(p1.ProjectUUIDs, []string{"PR-A"}) || !slices.Equal(p2.ProjectUUIDs, []string{"PR-A", "PR-B"}) {
		t.Errorf("AlbumUUIDs %q, %q and ProjectUUIDs %q, %q; want [AL-1], none and [PR-A], [PR-A PR-B]",
			p1.AlbumUUIDs, p2.AlbumUUIDs, p1.ProjectUUIDs, p2.ProjectUUIDs)
	}

	albumPhotos := membershipOrder(in.members, in.assets, c.byUUID)
	albums, err := loadAlbums(rows, albumPhotos)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || albums[0].UUID != "AL-1" || !slices.Equal(albums[0].PhotoUUIDs, []string{"P-1"}) {
		t.Errorf("albums %q, want AL-1 with the photo P-1 alone", lookupUUIDs(albums))
	}
	got, err := loadProjects(rows, albumPhotos)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	want := []*albuminfo.AlbumInfo{
		{UUID: "PR-A", ParentUUID: "F-1", PhotoUUIDs: []string{"P-1", "P-2"}, LibraryListOrder: []int{noListOrder}},
		{UUID: "PR-B", Title: "Card", PhotoUUIDs: []string{"P-2"}, InTrash: true, LibraryListOrder: []int{noListOrder}},
		{
			UUID: "PR-C", CloudGUID: "GUID-P", Title: nfcForm, ParentUUID: "PROJECTS", CreationDate: &created,
			SortOrder: albuminfo.AlbumSortNewestFirst, LibraryListOrder: []int{4},
		},
	}
	if len(got) != len(want) {
		t.Fatalf("%d projects, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.UUID != w.UUID || g.CloudGUID != w.CloudGUID || g.Title != w.Title || g.ParentUUID != w.ParentUUID || g.FolderNames != nil ||
			!sameStringsNilAware(g.PhotoUUIDs, w.PhotoUUIDs) || g.InTrash != w.InTrash || g.SortOrder != w.SortOrder ||
			!slices.Equal(g.LibraryListOrder, w.LibraryListOrder) ||
			!sameTime(g.CreationDate, w.CreationDate) || g.StartDate != nil || g.EndDate != nil {
			t.Errorf("project %d = %+v, want %+v", i, *g, *w)
		}
	}
}

func TestLoadProjectsParent(t *testing.T) {
	t.Parallel()

	none := sql.NullInt64{}
	root := albumRow(1, "SECRET-ROOT", sqlread.KindRootFolder, none)
	project := func(parent sql.NullInt64, parentUUID string) sqlread.Album {
		return withTitle(projectRow(2, "SECRET-P", parent, parentUUID), "SECRET-TITLE")
	}
	for _, tt := range []struct {
		name string
		row  sqlread.Album
		ok   bool
	}{
		{"no parent", project(none, ""), true},
		{"the root folder", project(nullInt(1), "SECRET-ROOT"), true},
		{"a parent that is not a row", project(nullInt(99), ""), false},
	} {
		got, err := loadProjects([]sqlread.Album{root, tt.row}, nil)
		switch {
		case tt.ok && (err != nil || len(got) != 1 || got[0].ParentUUID != ""):
			t.Errorf("%s: loadProjects = %d projects, %v; want one at the top level", tt.name, len(got), err)
		case !tt.ok && (err == nil || got != nil || !strings.Contains(err.Error(), "1 projects whose ZPARENTFOLDER is not a row") ||
			strings.Contains(err.Error(), "SECRET")):
			t.Errorf("%s: loadProjects = %d projects, %v; want an error without values", tt.name, len(got), err)
		}
	}
}

func TestOpenProjectParent(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	change(t, lib, "UPDATE ZGENERICALBUM SET ZKIND = 1508, ZPARENTFOLDER = (SELECT MAX(Z_PK) + 1 FROM ZGENERICALBUM) "+
		"WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)")
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err == nil || db != nil || !strings.Contains(err.Error(), "1 projects whose ZPARENTFOLDER is not a row") {
		t.Errorf("Open = %v, %v; want nil and an error about the parent of a project", db, err)
	}
}

func TestProjectLookup(t *testing.T) {
	t.Parallel()

	project := func(uuid, title string, inTrash bool) *albuminfo.AlbumInfo {
		return &albuminfo.AlbumInfo{UUID: uuid, Title: title, InTrash: inTrash}
	}
	p1, p2, p3, p4 := project("PR-1", "B", false), project("PR-2", "B", false), project("PR-3", "", false), project("PR-4", "A", true)
	all := []*albuminfo.AlbumInfo{p1, p2, p3, p4}
	album := &albuminfo.AlbumInfo{UUID: "AL-1", Title: "A"}
	db := &DB{projects: slices.Clone(all), albums: []*albuminfo.AlbumInfo{album}}
	photo := &photoinfo.PhotoInfo{ProjectUUIDs: []string{"PR-1", "PR-2", "PR-3", "PR-4"}, AlbumUUIDs: []string{"AL-1"}}

	for _, tt := range []struct {
		name      string
		got, want []*albuminfo.AlbumInfo
	}{
		// Upstream fails on the project without a title (photoinfo.py:700-703); "" sorts first here.
		{"ProjectInfoOf", db.ProjectInfoOf(photo), []*albuminfo.AlbumInfo{p3, p1, p2}},
		{"ProjectInfoOf equal titles by UUID", db.ProjectInfoOf(&photoinfo.PhotoInfo{ProjectUUIDs: []string{"PR-2", "PR-1"}}), []*albuminfo.AlbumInfo{p1, p2}},
		{"ProjectInfoOf of a project in the trash", db.ProjectInfoOf(&photoinfo.PhotoInfo{ProjectUUIDs: []string{"PR-4"}}), nil},
		{"ProjectInfo", db.ProjectInfo(), []*albuminfo.AlbumInfo{p1, p2, p3}},
		{"AlbumInfoOf", db.AlbumInfoOf(photo), []*albuminfo.AlbumInfo{album}},
	} {
		if !slices.Equal(tt.got, tt.want) || (tt.got == nil) != (tt.want == nil) {
			t.Errorf("%s = %q, want %q", tt.name, lookupUUIDs(tt.got), lookupUUIDs(tt.want))
		}
	}
	for _, a := range all {
		if got, ok := db.Project(a.UUID); !ok || got != a {
			t.Errorf("Project(%s) = %p, %v; want the project itself, in the trash too", a.UUID, got, ok)
		}
		if _, ok := db.Album(a.UUID); ok {
			t.Errorf("Album(%s) found a project", a.UUID)
		}
	}
	for _, u := range []string{"", "AL-1", "PR-0", "PR-5"} {
		if a, ok := db.Project(u); ok || a != nil {
			t.Errorf("Project(%q) = %v, %v; want nil, false", u, a, ok)
		}
	}

	db.ProjectInfo()[0] = nil
	db.ProjectInfoOf(photo)[0] = nil
	if !slices.Equal(db.projects, all) {
		t.Errorf("a write into a result changed the projects of the database: %q", lookupUUIDs(db.projects))
	}

	empty := &DB{}
	if a, ok := empty.Project("PR-1"); ok || a != nil || empty.ProjectInfo() != nil || empty.ProjectInfoOf(photo) != nil {
		t.Error("a library without projects gave a project or a non-nil empty list")
	}
}

// The count of the distinct pairs is upstream len(_dbalbums_album[uuid])
// (photosdb/photosdb.py:1873-1901), as in lookupCounts.
func projectMembers(table, albumCol, assetCol, kind string) string {
	return "SELECT COUNT(*) FROM (SELECT DISTINCT m." + albumCol + ", m." + assetCol + " FROM " + table + " m " +
		"JOIN ZGENERICALBUM g ON g.Z_PK = m." + albumCol + " JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = m." + assetCol +
		" WHERE g.ZKIND = " + kind + " AND g.ZUUID IS NOT NULL)"
}

func projectCheck(t *testing.T, name, lib string) (projects, members int) {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	src := roDB(t, lib)
	n := schemaByID(t, libraryModels[name].id)
	const rows = " FROM ZGENERICALBUM WHERE ZKIND = 1508 AND ZUUID IS NOT NULL"
	if want := count(t, src, "SELECT COUNT(*)"+rows); len(db.projects) != want {
		t.Errorf("%d projects, want %d", len(db.projects), want)
	}
	if want := count(t, src, "SELECT COUNT(*)"+rows+" AND ZTRASHEDSTATE = 0"); len(db.ProjectInfo()) != want {
		t.Errorf("%d projects outside the trash, want %d", len(db.ProjectInfo()), want)
	}
	// The premise of ProjectInfo, which does not read the owner that upstream
	// also requires to be NULL (photosdb/photosdb.py:3206).
	if owned := count(t, src, "SELECT COUNT(*)"+rows+" AND ZCLOUDOWNERHASHEDPERSONID IS NOT NULL"); owned != 0 {
		t.Errorf("%d projects with ZCLOUDOWNERHASHEDPERSONID", owned)
	}

	photosOf := map[string][]string{}
	for _, p := range db.Photos() {
		for _, u := range p.ProjectUUIDs {
			if _, ok := db.Project(u); !ok {
				t.Errorf("a project of photo %s is not among the projects read", p.UUID)
			}
			photosOf[u] = append(photosOf[u], p.UUID)
		}
		for _, u := range p.AlbumUUIDs {
			if _, ok := db.Project(u); ok {
				t.Errorf("photo %s lists the project %s among its albums", p.UUID, u)
			}
		}
		info := db.ProjectInfoOf(p)
		if slices.ContainsFunc(info, func(a *albuminfo.AlbumInfo) bool { return a.InTrash }) ||
			!slices.IsSortedFunc(info, func(a, b *albuminfo.AlbumInfo) int { return strings.Compare(a.Title, b.Title) }) {
			t.Errorf("photo %s: ProjectInfoOf has a project in the trash or is not in the order of titles", p.UUID)
		}
	}
	for _, a := range db.projects {
		want := photosOf[a.UUID]
		slices.Sort(want)
		if !slices.Equal(a.PhotoUUIDs, want) || len(db.AlbumPhotos(a)) != len(a.PhotoUUIDs) {
			t.Errorf("project %s: PhotoUUIDs %q and %d AlbumPhotos, want the photos that list it %q", a.UUID, a.PhotoUUIDs, len(db.AlbumPhotos(a)), want)
		}
		members += len(a.PhotoUUIDs)
		if _, ok := db.Album(a.UUID); ok || a.FolderNames != nil {
			t.Errorf("project %s is among the albums or has folders %q", a.UUID, a.FolderNames)
		}
		parent := testlib.Rows(t, src, "SELECT p.ZUUID FROM ZGENERICALBUM g JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER "+
			"WHERE g.ZUUID = ? AND p.ZKIND != 3999", a.UUID)
		if want := strings.Join(parent, ""); a.ParentUUID != want {
			t.Errorf("project %s: ParentUUID %q, want %q", a.UUID, a.ParentUUID, want)
		}
	}
	t.Logf("ZKIND of the parents of projects: %q", testlib.Rows(t, src, "SELECT DISTINCT p.ZKIND FROM ZGENERICALBUM g "+
		"JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER WHERE g.ZKIND = 1508 ORDER BY 1"))
	if want := count(t, src, projectMembers(n.MemberTable, n.MemberAlbumColumn, n.MemberAssetColumn, "1508")); members != want {
		t.Errorf("%d project members, want %d", members, want)
	}
	albumMembers := 0
	for _, a := range db.albums {
		albumMembers += len(a.PhotoUUIDs)
	}
	if want := count(t, src, projectMembers(n.MemberTable, n.MemberAlbumColumn, n.MemberAssetColumn, "2")); albumMembers != want {
		t.Errorf("%d album members, want %d: the members of projects reach the albums", albumMembers, want)
	}
	return len(db.projects), members
}

func TestProjects(t *testing.T) {
	t.Parallel()

	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			projects, members := projectCheck(t, name, testlib.OpenPristine(t, name))
			t.Logf("%d projects, %d members", projects, members)
		})
	}
	// The libraries of upstream may hold no project with photos, so some
	// albums of the synthetic one become projects, one in two of them trashed.
	t.Run("synthetic with albums turned into projects", func(t *testing.T) {
		lib := testlib.OpenPristine(t, "synthetic")
		change(t, lib,
			"UPDATE ZGENERICALBUM SET ZKIND = 1508 WHERE ZKIND = 2 AND Z_PK % 2 = 0",
			"UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = 1 WHERE ZKIND = 1508 AND Z_PK % 4 = 0",
		)
		if projects, members := projectCheck(t, "synthetic", lib); projects < 2 || members == 0 {
			t.Errorf("%d projects with %d members, want some of each", projects, members)
		}
	})
}
