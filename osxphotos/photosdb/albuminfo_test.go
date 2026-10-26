package photosdb

import (
	"database/sql"
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

func lookupUUIDs(albums []*albuminfo.AlbumInfo) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.UUID
	}
	return out
}

func lookupTexts(t *testing.T, src *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := src.QueryContext(t.Context(), q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, norm.NFC.String(s.String))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// COUNT DISTINCT with the join to the attributes equals upstream
// len(_dbalbums_album[album]) (photosdb/photosdb.py:1873-1901) because the
// composite primary key (album, asset) of the membership table rules out a
// repeated member. Members without an attributes row are left out here and are
// the AlbumCounts line of UPSTREAM.md.
func lookupCounts(t *testing.T, src *sql.DB, table, albumCol, assetCol string) map[string]int {
	t.Helper()
	q := "SELECT g.ZTITLE, (SELECT COUNT(DISTINCT m." + assetCol + ") FROM " + table + " m " + //nolint:gosec // G202: the membership table and columns of internal/schema
		"JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = m." + assetCol + " WHERE m." + albumCol + " = g.Z_PK) " +
		"FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND g.ZUUID IS NOT NULL AND g.ZTRASHEDSTATE = 0"
	rows, err := src.QueryContext(t.Context(), q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var title sql.NullString
		var n int
		if err := rows.Scan(&title, &n); err != nil {
			t.Fatal(err)
		}
		out[norm.NFC.String(title.String)] += n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAlbumLookupUnit(t *testing.T) {
	t.Parallel()

	album := func(uuid, title string, inTrash bool, photos ...string) *albuminfo.AlbumInfo {
		return &albuminfo.AlbumInfo{UUID: uuid, Title: title, InTrash: inTrash, PhotoUUIDs: photos}
	}
	a1, a2, a3 := album("A-1", "B", false), album("A-2", "B", false), album("A-3", "", false)
	a4 := album("A-4", "A", true)
	a5, a6 := album("A-5", "C", false, "P-1", "P-2"), album("A-6", "C", false, "P-3")
	a7 := album("A-7", "D", false)
	all := []*albuminfo.AlbumInfo{a1, a2, a3, a4, a5, a6, a7}

	p := &photoinfo.PhotoInfo{UUID: "P-1", Burst: true, BurstUUID: "B", AlbumUUIDs: []string{"A-1", "A-2", "A-3", "A-4", "A-UNKNOWN"}}
	key := &photoinfo.PhotoInfo{UUID: "P-2", Burst: true, BurstUUID: "B", BurstKey: true, AlbumUUIDs: []string{"A-5", "A-1", "A-4"}}
	reversed := &photoinfo.PhotoInfo{AlbumUUIDs: []string{"A-2", "A-1"}}
	hidden := &photoinfo.PhotoInfo{AlbumUUIDs: []string{"A-4", "A-UNKNOWN"}}
	db := &DB{albums: slices.Clone(all), bursts: indexBursts([]*photoinfo.PhotoInfo{p, key})}

	albumsCases := []struct {
		name string
		got  []*albuminfo.AlbumInfo
		want []*albuminfo.AlbumInfo
	}{
		{"AlbumInfoOf", db.AlbumInfoOf(p), []*albuminfo.AlbumInfo{a3, a1, a2}},
		{"AlbumInfoOf equal titles by UUID", db.AlbumInfoOf(reversed), []*albuminfo.AlbumInfo{a1, a2}},
		{"BurstAlbumInfoOf", db.BurstAlbumInfoOf(p), []*albuminfo.AlbumInfo{a3, a1, a2, a5}},
		{"BurstAlbumInfoOf of the key photo", db.BurstAlbumInfoOf(key), []*albuminfo.AlbumInfo{a1, a5}},
		{"BurstAlbumInfoOf outside a burst", db.BurstAlbumInfoOf(reversed), []*albuminfo.AlbumInfo{a1, a2}},
		{"AlbumInfoOf trashed and unknown only", db.AlbumInfoOf(hidden), nil},
		{"BurstAlbumInfoOf trashed and unknown only", db.BurstAlbumInfoOf(hidden), nil},
		{"AlbumInfo", db.AlbumInfo(), []*albuminfo.AlbumInfo{a1, a2, a3, a5, a6, a7}},
	}
	for _, tt := range albumsCases {
		if !slices.Equal(tt.got, tt.want) || (tt.got == nil) != (tt.want == nil) {
			t.Errorf("%s = %q, want %q", tt.name, lookupUUIDs(tt.got), lookupUUIDs(tt.want))
		}
	}

	titlesCases := []struct {
		name      string
		got, want []string
	}{
		{"AlbumTitlesOf", db.AlbumTitlesOf(p), []string{"", "B"}},
		{"BurstAlbumTitlesOf", db.BurstAlbumTitlesOf(p), []string{"", "B", "C"}},
		{"AlbumTitlesOf trashed and unknown only", db.AlbumTitlesOf(hidden), nil},
		{"BurstAlbumTitlesOf trashed and unknown only", db.BurstAlbumTitlesOf(hidden), nil},
		{"AlbumTitles", db.AlbumTitles(), []string{"", "B", "C", "D"}},
	}
	for _, tt := range titlesCases {
		if !sameStringsNilAware(tt.got, tt.want) {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}

	if want := map[string]int{"": 0, "B": 0, "C": 3, "D": 0}; !maps.Equal(db.AlbumCounts(), want) {
		t.Errorf("AlbumCounts = %v, want %v", db.AlbumCounts(), want)
	}

	for _, a := range all {
		if got, ok := db.Album(a.UUID); !ok || got != a {
			t.Errorf("Album(%s) = %p, %v; want the album itself", a.UUID, got, ok)
		}
	}
	if a, ok := db.Album("A-4"); !ok || !a.InTrash {
		t.Errorf("Album(A-4) = %v, %v; want the album in the trash", a, ok)
	}
	// Before the first, between two, after the last, empty.
	for _, u := range []string{"", "A-0", "A-10", "A-8", "a-1", "B-1"} {
		if a, ok := db.Album(u); ok || a != nil {
			t.Errorf("Album(%q) = %v, %v; want nil, false", u, a, ok)
		}
	}

	empty := &DB{}
	if a, ok := empty.Album("A-1"); ok || a != nil || empty.AlbumInfo() != nil || empty.AlbumTitles() != nil ||
		empty.AlbumInfoOf(p) != nil || empty.BurstAlbumInfoOf(p) != nil {
		t.Errorf("a library without albums gave an album or a non-nil empty list")
	}
	if counts := empty.AlbumCounts(); counts == nil || len(counts) != 0 {
		t.Errorf("AlbumCounts of a library without albums = %#v, want an empty map", counts)
	}

	db.AlbumInfo()[0] = nil
	db.AlbumInfoOf(p)[0] = nil
	db.BurstAlbumInfoOf(p)[0] = nil
	db.AlbumTitles()[0] = "changed"
	db.AlbumTitlesOf(p)[0] = "changed"
	db.BurstAlbumTitlesOf(p)[0] = "changed"
	counts := db.AlbumCounts()
	counts["C"] = 99
	delete(counts, "D")
	if !slices.Equal(db.albums, all) {
		t.Errorf("a write into a result changed the albums of the database: %q", lookupUUIDs(db.albums))
	}
	if got := db.AlbumInfo(); len(got) != 6 || got[0] != a1 {
		t.Errorf("AlbumInfo after a write into a result = %q", lookupUUIDs(got))
	}
	if got := db.AlbumInfoOf(p); !slices.Equal(got, []*albuminfo.AlbumInfo{a3, a1, a2}) {
		t.Errorf("AlbumInfoOf after a write into a result = %q", lookupUUIDs(got))
	}
	if got := db.BurstAlbumInfoOf(p); !slices.Equal(got, []*albuminfo.AlbumInfo{a3, a1, a2, a5}) {
		t.Errorf("BurstAlbumInfoOf after a write into a result = %q", lookupUUIDs(got))
	}
	if want := []string{"A-5", "A-1", "A-4"}; !slices.Equal(key.AlbumUUIDs, want) {
		t.Errorf("AlbumUUIDs of the key photo after BurstAlbumInfoOf = %q, want %q", key.AlbumUUIDs, want)
	}
	if got := db.AlbumTitles(); !slices.Equal(got, []string{"", "B", "C", "D"}) {
		t.Errorf("AlbumTitles after a write into a result = %q", got)
	}
	if got := db.AlbumTitlesOf(p); !slices.Equal(got, []string{"", "B"}) {
		t.Errorf("AlbumTitlesOf after a write into a result = %q", got)
	}
	if got := db.BurstAlbumTitlesOf(p); !slices.Equal(got, []string{"", "B", "C"}) {
		t.Errorf("BurstAlbumTitlesOf after a write into a result = %q", got)
	}
	if want := map[string]int{"": 0, "B": 0, "C": 3, "D": 0}; !maps.Equal(db.AlbumCounts(), want) {
		t.Errorf("AlbumCounts after a write into a result = %v, want %v", db.AlbumCounts(), want)
	}
}

func TestBurstAlbumInfoOf(t *testing.T) {
	t.Parallel()

	burst := func(pk int64, uuid, burstUUID string, pick int64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		a.BurstUUID, a.BurstPickType = nullString(burstUUID), nullInt(pick)
		return a
	}
	const withKey, withoutKey = "WITH-KEY", "WITHOUT-KEY"
	in := tables{
		assets: []sqlread.Asset{
			burst(1, "K", withKey, photoinfo.BurstPickKey), burst(2, "S", withKey, photoinfo.BurstPickSelected),
			burst(3, "U", withKey, pickUnselected),
			burst(4, "E1", "", photoinfo.BurstPickKey), burst(5, "E2", "", 0),
			baseAsset(6, "N"),
			burst(7, "O", withoutKey, pickUnselected),
		},
		members: []sqlread.Member{
			{AssetPK: 1, AlbumUUID: "AL-K"}, {AssetPK: 1, AlbumUUID: "AL-T"}, {AssetPK: 2, AlbumUUID: "AL-S"},
			{AssetPK: 2, AlbumUUID: "AL-K"}, {AssetPK: 4, AlbumUUID: "AL-E"}, {AssetPK: 6, AlbumUUID: "AL-N"},
			{AssetPK: 7, AlbumUUID: "AL-O"},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	var albums []*albuminfo.AlbumInfo
	for _, u := range []string{"AL-E", "AL-K", "AL-N", "AL-O", "AL-S", "AL-T"} {
		albums = append(albums, &albuminfo.AlbumInfo{UUID: u, Title: u, InTrash: u == "AL-T"})
	}
	db := &DB{photos: c.photos, byUUID: c.byUUID, albums: albums, bursts: indexBursts(c.photos)}
	want := map[string][]string{
		"K":  {"AL-K"},
		"S":  {"AL-K", "AL-S"},
		"U":  {"AL-K"},
		"E1": {"AL-E"},
		"E2": {"AL-E"},
		"N":  {"AL-N"},
		"O":  {"AL-O"},
	}
	for uuid, w := range want {
		if got := lookupUUIDs(db.BurstAlbumInfoOf(c.byUUID[uuid])); !slices.Equal(got, w) {
			t.Errorf("%s: BurstAlbumInfoOf = %q, want %q", uuid, got, w)
		}
	}
}

func TestAlbumLookup(t *testing.T) {
	t.Parallel()

	outside, withPhotos, others := 0, 0, 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)
			n := schemaByID(t, libraryModels[name].id)
			const user = " FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZUUID IS NOT NULL AND ZTRASHEDSTATE = 0"

			info := db.AlbumInfo()
			outside += len(info)
			if want := count(t, src, "SELECT COUNT(*)"+user); len(info) != want {
				t.Errorf("%d albums outside the trash, want %d", len(info), want)
			}
			for _, a := range info {
				if a.InTrash {
					t.Errorf("album %s in the trash is in AlbumInfo", a.UUID)
				}
			}

			for _, a := range db.albums {
				if got, ok := db.Album(a.UUID); !ok || got != a {
					t.Errorf("Album(%s) = %p, %v; want the album read", a.UUID, got, ok)
				}
			}
			// The root folder (ZKIND 3999) and the folders (4000) are not albums.
			notAlbums := lookupTexts(t, src, "SELECT ZUUID FROM ZGENERICALBUM WHERE ZKIND IN (3999, 4000) AND ZUUID IS NOT NULL")
			if len(notAlbums) == 0 {
				t.Error("no root folder or folder to look up")
			}
			others += len(notAlbums)
			for _, u := range notAlbums {
				if a, ok := db.Album(u); ok || a != nil {
					t.Errorf("Album(%s) found a folder", u)
				}
			}

			titles := lookupTexts(t, src, "SELECT ZTITLE"+user)
			slices.Sort(titles)
			if want := slices.Compact(titles); !slices.Equal(db.AlbumTitles(), want) {
				t.Errorf("AlbumTitles has %d titles, want %d", len(db.AlbumTitles()), len(want))
			}

			counts := db.AlbumCounts()
			want := lookupCounts(t, src, n.MemberTable, n.MemberAlbumColumn, n.MemberAssetColumn)
			if !maps.Equal(counts, want) {
				t.Errorf("AlbumCounts has %d titles, %d photos in all; want %d titles, %d photos", len(counts), lookupSum(counts), len(want), lookupSum(want))
			}
			withPhotos += lookupSum(counts)
		})
	}
	if outside == 0 || withPhotos == 0 || others == 0 {
		t.Errorf("%d albums outside the trash, %d photos in them, %d folders in all libraries; want some of each", outside, withPhotos, others)
	}
}

func lookupSum(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}

type lookupKey struct {
	info, titles string
	infoOf       func(*DB, *photoinfo.PhotoInfo) []*albuminfo.AlbumInfo
	titlesOf     func(*DB, *photoinfo.PhotoInfo) []string
}

var lookupKeys = []lookupKey{
	{"album_info", "albums", (*DB).AlbumInfoOf, (*DB).AlbumTitlesOf},
	{"burst_album_info", "burst_albums", (*DB).BurstAlbumInfoOf, (*DB).BurstAlbumTitlesOf},
}

type lookupElem struct {
	UUID string `json:"uuid"`
}

func lookupDiverges(divs []golden.Divergence, library, uuid, key string) bool {
	return slices.ContainsFunc(divs, func(d golden.Divergence) bool {
		return d.Library == library && d.Field == key && (d.UUID == "*" || d.UUID == uuid)
	})
}

// The lists of the reference hold the user and the shared albums
// (photoinfo.py:2022-2034). Messages carry UUIDs and keys, never values of
// the reference.
func lookupCompare(t *testing.T, db *DB, o golden.Object, p *photoinfo.PhotoInfo, k lookupKey, library string, divs []golden.Divergence) int {
	t.Helper()
	var elems []lookupElem
	if json.Unmarshal(o[k.info], &elems) != nil || elems == nil {
		t.Fatalf("photo %s: key %s is not a list of albums", p.UUID, k.info)
	}
	var wantUUIDs []string
	for _, e := range elems {
		wantUUIDs = append(wantUUIDs, e.UUID)
	}
	slices.Sort(wantUUIDs)
	wantUUIDs = slices.Compact(wantUUIDs)

	got := k.infoOf(db, p)
	for i := 1; i < len(got); i++ {
		if got[i-1].Title > got[i].Title {
			t.Errorf("photo %s: %s: albums %d and %d are not in the order of titles", p.UUID, k.info, i, i+1)
		}
	}
	if !lookupDiverges(divs, library, p.UUID, k.info) {
		gotUUIDs := lookupUUIDs(got)
		slices.Sort(gotUUIDs)
		if !slices.Equal(gotUUIDs, wantUUIDs) {
			t.Errorf("photo %s: %s has %d albums, the reference %d; or they differ", p.UUID, k.info, len(gotUUIDs), len(wantUUIDs))
		}
	}

	var refTitles []*string
	if json.Unmarshal(o[k.titles], &refTitles) != nil || refTitles == nil {
		t.Fatalf("photo %s: key %s is not a list of titles", p.UUID, k.titles)
	}
	var wantTitles []string
	for _, s := range refTitles {
		title := ""
		if s != nil {
			title = *s
		}
		wantTitles = append(wantTitles, title)
	}
	slices.Sort(wantTitles)
	wantTitles = slices.Compact(wantTitles)

	gotTitles := k.titlesOf(db, p)
	if !slices.IsSorted(gotTitles) {
		t.Errorf("photo %s: %s are not sorted", p.UUID, k.titles)
	}
	if !lookupDiverges(divs, library, p.UUID, k.titles) && !slices.Equal(gotTitles, wantTitles) {
		t.Errorf("photo %s: %s has %d titles, the reference %d; or they differ", p.UUID, k.titles, len(gotTitles), len(wantTitles))
	}
	return len(got)
}

func TestAlbumLookupGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
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
			objects, albums := 0, 0
			for _, o := range slices.Concat(ref.Query, ref.Deleted) {
				var uuid string
				if json.Unmarshal(o["uuid"], &uuid) != nil || uuid == "" {
					t.Fatalf("an object of %s has no uuid", lib.name)
				}
				p, ok := db.Photo(uuid)
				if !ok {
					t.Errorf("photo %s of the reference is not in the database", uuid)
					continue
				}
				objects++
				for _, k := range lookupKeys {
					albums += lookupCompare(t, db, o, p, k, lib.name, divs)
				}
			}
			t.Logf("%d objects, %d albums of photos in album_info and burst_album_info", objects, albums)
			total += albums
		})
	}
	if total == 0 {
		t.Error("no album of any photo in any library: nothing was compared")
	}
}

func TestAlbumLookupTrash(t *testing.T) {
	t.Parallel()

	const name = "Test-15.4.1"
	lib := testlib.OpenPristine(t, name)
	n := schemaByID(t, libraryModels[name].id)
	src := roDB(t, lib)
	var picked string
	err := src.QueryRowContext(t.Context(), "SELECT g.ZUUID FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND g.ZUUID IS NOT NULL AND g.ZTRASHEDSTATE = 0 AND "+
		"EXISTS (SELECT 1 FROM "+n.MemberTable+" m WHERE m."+n.MemberAlbumColumn+" = g.Z_PK) ORDER BY g.ZUUID LIMIT 1").Scan(&picked)
	if err != nil {
		t.Fatal(err)
	}
	// Decided on the pristine rows: does another album outside the trash have the title?
	otherTitles := lookupTexts(t, src, "SELECT ZTITLE FROM ZGENERICALBUM WHERE ZKIND = 2 AND ZUUID IS NOT NULL AND ZTRASHEDSTATE = 0 AND ZUUID != ?", picked)

	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := before.Album(picked)
	if !ok || a.InTrash || len(a.PhotoUUIDs) == 0 {
		t.Fatalf("album %s before the change: found %v, want an album outside the trash with photos", picked, ok)
	}
	title, members := a.Title, slices.Clone(a.PhotoUUIDs)
	for _, u := range members {
		p, found := before.Photo(u)
		if !found || !slices.Contains(lookupUUIDs(before.AlbumInfoOf(p)), picked) {
			t.Fatalf("photo %s: not found or without album %s in AlbumInfoOf before the change", u, picked)
		}
	}
	wantCounts, wantTitles := before.AlbumCounts(), before.AlbumTitles()

	// Query through the album: what stays is the photos of another album with
	// the same path; the photos with no other album leave InAlbum.
	path, ok := queryPath(append(slices.Clone(a.FolderNames), title))
	if !ok {
		t.Fatalf("album %s: its path is ambiguous in Query", picked)
	}
	album := query.Options{Album: []string{path}}
	albumBefore, inBefore := before.Query(album), before.Query(query.Options{InAlbum: true})
	if len(albumBefore) == 0 {
		t.Fatalf("album %s: Query by its path selects no photo before the change", picked)
	}
	var wantAlbum, moved []string
	for _, p := range albumBefore {
		if slices.ContainsFunc(before.AlbumInfoOf(p), func(o *albuminfo.AlbumInfo) bool {
			return o.UUID != picked && o.Title == title && slices.Equal(o.FolderNames, a.FolderNames)
		}) {
			wantAlbum = append(wantAlbum, p.UUID)
		}
	}
	for _, p := range inBefore {
		if !slices.ContainsFunc(before.AlbumInfoOf(p), func(o *albuminfo.AlbumInfo) bool { return o.UUID != picked }) {
			moved = append(moved, p.UUID)
		}
	}
	if len(moved) == 0 {
		t.Fatalf("album %s: no photo is in it alone, so NotInAlbum is not checked", picked)
	}
	wantIn := slices.DeleteFunc(queryIDs(inBefore, nil), func(u string) bool { return slices.Contains(moved, u) })
	t.Logf("%d photos leave InAlbum for NotInAlbum, %d stay in an album of the same path", len(moved), len(wantAlbum))

	change(t, lib, "UPDATE ZGENERICALBUM SET ZTRASHEDSTATE = 1 WHERE ZUUID = '"+picked+"'")
	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got, found := after.Album(picked); !found || !got.InTrash {
		t.Errorf("Album(%s) after the change = %v, %v; want the album in the trash", picked, got, found)
	}
	if slices.Contains(lookupUUIDs(after.AlbumInfo()), picked) {
		t.Errorf("album %s in the trash is in AlbumInfo", picked)
	}
	for _, p := range after.Photos() {
		if slices.Contains(lookupUUIDs(after.AlbumInfoOf(p)), picked) || slices.Contains(lookupUUIDs(after.BurstAlbumInfoOf(p)), picked) {
			t.Errorf("photo %s lists the album %s in the trash", p.UUID, picked)
		}
	}

	if slices.Contains(otherTitles, title) {
		wantCounts[title] -= len(members)
	} else {
		delete(wantCounts, title)
		wantTitles = slices.DeleteFunc(wantTitles, func(s string) bool { return s == title })
	}
	if got := after.AlbumCounts(); !maps.Equal(got, wantCounts) {
		t.Errorf("AlbumCounts after the change has %d titles and %d photos, want %d and %d", len(got), lookupSum(got), len(wantCounts), lookupSum(wantCounts))
	}
	if got := after.AlbumTitles(); !slices.Equal(got, wantTitles) {
		t.Errorf("AlbumTitles after the change has %d titles, want %d", len(got), len(wantTitles))
	}

	if got := queryIDs(after.Query(album), nil); !slices.Equal(got, wantAlbum) {
		t.Errorf("Query by the path of the album in the trash selects %d photos, want %d", len(got), len(wantAlbum))
	}
	gotIn := queryIDs(after.Query(query.Options{InAlbum: true}), nil)
	slices.Sort(gotIn)
	slices.Sort(wantIn)
	if !slices.Equal(gotIn, wantIn) {
		t.Errorf("InAlbum selects %d photos after the change, want %d; or they differ", len(gotIn), len(wantIn))
	}
	notIn := queryIDs(after.Query(query.Options{NotInAlbum: true}), nil)
	for _, u := range moved {
		if !slices.Contains(notIn, u) {
			t.Errorf("photo %s was only in the album %s, and NotInAlbum after the change lacks it", u, picked)
		}
	}
}
