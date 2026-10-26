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

func importRow(pk int64, uuid string, trashed sql.NullInt64) sqlread.Album {
	r := albumRow(pk, uuid, sqlread.KindImportSession, sql.NullInt64{})
	r.Trashed = trashed
	return r
}

func importUUIDs(sessions []*albuminfo.ImportInfo) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.UUID
	}
	return out
}

func importPairs(t *testing.T, src *sql.DB, q string) map[string]string {
	t.Helper()
	rows, err := src.QueryContext(t.Context(), q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLoadImports(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9"
	full := withTitle(importRow(13, "S-A", nullInt(0)), nfd)
	full.Created, full.Start, full.End = nullFloat(694224000), nullFloat(0), nullFloat(86400)
	rows := []sqlread.Album{
		albumRow(1, "ROOT", sqlread.KindRootFolder, sql.NullInt64{}),
		albumRow(2, "AL-1", sqlread.KindUserAlbum, nullInt(1)),
		importRow(10, "S-EMPTY", nullInt(0)),
		importRow(11, "S-TRASH", nullInt(1)),
		importRow(12, "S-NULL", sql.NullInt64{}),
		full,
	}
	photos := []*photoinfo.PhotoInfo{
		{UUID: "P-1", ImportUUID: "S-A"},
		{UUID: "P-2"},
		{UUID: "P-3", ImportUUID: "S-TRASH", InTrash: true},
		{UUID: "P-4", ImportUUID: "S-A", InTrash: true},
		{UUID: "P-5", ImportUUID: "AL-1"},
	}

	all, info := loadImports(rows, photos)
	created := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	start, end := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2001, 1, 2, 0, 0, 0, 0, time.UTC)
	want := []*albuminfo.ImportInfo{
		{UUID: "S-A", Title: nfcForm, CreationDate: &created, StartDate: &start, EndDate: &end, PhotoUUIDs: []string{"P-1", "P-4"}},
		{UUID: "S-EMPTY"},
		{UUID: "S-NULL"},
		{UUID: "S-TRASH", PhotoUUIDs: []string{"P-3"}},
	}
	if len(all) != len(want) {
		t.Fatalf("%d import sessions %q, want %d", len(all), importUUIDs(all), len(want))
	}
	for i, w := range want {
		g := all[i]
		if g.UUID != w.UUID || g.Title != w.Title || !sameStringsNilAware(g.PhotoUUIDs, w.PhotoUUIDs) ||
			!sameTime(g.CreationDate, w.CreationDate) || !sameTime(g.StartDate, w.StartDate) || !sameTime(g.EndDate, w.EndDate) {
			t.Errorf("session %d = %+v, want %+v", i, *g, *w)
		}
	}
	// ZTRASHEDSTATE NULL is in the trash, as for albums (photosdb.py:1936).
	if !slices.Equal(info, []*albuminfo.ImportInfo{all[0], all[1]}) {
		t.Errorf("sessions outside the trash %q, want S-A and S-EMPTY", importUUIDs(info))
	}

	db := &DB{imports: all, importInfo: info}
	for _, s := range all {
		if got, ok := db.Import(s.UUID); !ok || got != s {
			t.Errorf("Import(%s) = %p, %v; want the session itself", s.UUID, got, ok)
		}
	}
	for _, u := range []string{"", "AL-1", "ROOT", "S-0", "S-B", "S-Z", "s-a"} {
		if s, ok := db.Import(u); ok || s != nil {
			t.Errorf("Import(%q) = %v, %v; want nil, false", u, s, ok)
		}
	}
	db.ImportInfo()[0] = nil
	if got := importUUIDs(db.ImportInfo()); !slices.Equal(got, []string{"S-A", "S-EMPTY"}) {
		t.Errorf("ImportInfo after a write into a result = %q", got)
	}

	empty := &DB{}
	if s, ok := empty.Import("S-A"); ok || s != nil || empty.ImportInfo() != nil {
		t.Error("a library without import sessions gave a session or a non-nil empty list")
	}
}

func TestLoadImportUUID(t *testing.T) {
	t.Parallel()

	a := baseAsset(1, "A-1")
	if p := loadOne(t, a); p.ImportUUID != "" {
		t.Errorf("no session: ImportUUID %q, want \"\"", p.ImportUUID)
	}
	a.ImportUUID = nullString("S-1")
	if p := loadOne(t, a); p.ImportUUID != "S-1" {
		t.Errorf("ImportUUID %q, want S-1", p.ImportUUID)
	}
}

// The ZUUID checks of loadAlbums cover the sessions: upstream keeps every
// ZGENERICALBUM row in one dictionary by ZUUID (photosdb.py:1923-1948).
func TestLoadAlbumsImportRows(t *testing.T) {
	t.Parallel()

	root := albumRow(1, "SECRET-ROOT", sqlread.KindRootFolder, sql.NullInt64{})
	tests := []struct {
		name string
		rows []sqlread.Album
		want string
	}{
		{"a session with the ZUUID of an album", []sqlread.Album{
			root, albumRow(2, "SECRET-U", sqlread.KindUserAlbum, nullInt(1)), importRow(3, "SECRET-U", nullInt(0)),
		}, "1 albums and folders with an empty or repeated ZGENERICALBUM.ZUUID"},
		{"a session without ZUUID", []sqlread.Album{root, importRow(2, "", nullInt(0))}, "1 albums and folders with an empty or repeated"},
		{"sessions without a root", []sqlread.Album{importRow(2, "SECRET-S", nullInt(0))}, "0 root folders"},
	}
	for _, tt := range tests {
		got, err := loadAlbums(tt.rows, nil)
		if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "SECRET") || got != nil {
			t.Errorf("%s: loadAlbums = %d albums, %v; want an error with %q and without values", tt.name, len(got), err, tt.want)
		}
	}
	got, err := loadAlbums([]sqlread.Album{root, importRow(2, "SECRET-S", nullInt(0))}, nil)
	if err != nil || got != nil {
		t.Errorf("loadAlbums of a root and a session = %d albums, %v; want none, nil", len(got), err)
	}
}

func TestImports(t *testing.T) {
	t.Parallel()

	sessions, outside, imported := 0, 0, 0
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, lib)
			const rows = " FROM ZGENERICALBUM WHERE ZKIND = 1506 AND ZUUID IS NOT NULL"

			sessions += len(db.imports)
			if want := count(t, src, "SELECT COUNT(*)"+rows); len(db.imports) != want {
				t.Errorf("%d import sessions, want %d", len(db.imports), want)
			}
			info := db.ImportInfo()
			outside += len(info)
			if want := count(t, src, "SELECT COUNT(*)"+rows+" AND ZTRASHEDSTATE = 0"); len(info) != want {
				t.Errorf("%d import sessions outside the trash, want %d", len(info), want)
			}
			// The premise of the import_info line of UPSTREAM.md: upstream also
			// leaves out a session with an owner (photosdb/photosdb.py:3206).
			if owned := count(t, src, "SELECT COUNT(*)"+rows+" AND ZCLOUDOWNERHASHEDPERSONID IS NOT NULL"); owned != 0 {
				t.Errorf("%d import sessions with ZCLOUDOWNERHASHEDPERSONID", owned)
			}
			// The premise of Import: upstream takes the row of ZIMPORTSESSION of
			// any ZKIND (photosdb/photosdb.py:2383-2393), Import only sessions.
			if other := count(t, src, "SELECT COUNT(*) FROM ZASSET a JOIN ZGENERICALBUM g ON g.Z_PK = a.ZIMPORTSESSION WHERE g.ZKIND != 1506"); other != 0 {
				t.Errorf("%d assets whose ZIMPORTSESSION is a row of another ZKIND", other)
			}

			want := importPairs(t, src, "SELECT a.ZUUID, g.ZUUID FROM ZASSET a JOIN ZGENERICALBUM g ON g.Z_PK = a.ZIMPORTSESSION "+
				"WHERE a.ZUUID IS NOT NULL AND g.ZUUID IS NOT NULL")
			photosOf := map[string][]string{}
			for _, p := range db.Photos() {
				if p.ImportUUID != want[p.UUID] {
					t.Errorf("photo %s: ImportUUID %q, want %q", p.UUID, p.ImportUUID, want[p.UUID])
				}
				if p.ImportUUID != "" {
					imported++
					photosOf[p.ImportUUID] = append(photosOf[p.ImportUUID], p.UUID)
				}
			}
			for i, s := range db.imports {
				if i > 0 && db.imports[i-1].UUID >= s.UUID {
					t.Errorf("session %d out of UUID order or repeated", i+1)
				}
				w := photosOf[s.UUID]
				slices.Sort(w)
				if !slices.Equal(s.PhotoUUIDs, w) {
					t.Errorf("session %s: PhotoUUIDs %q, want the photos that name it %q", s.UUID, s.PhotoUUIDs, w)
				}
				if got, ok := db.Import(s.UUID); !ok || got != s {
					t.Errorf("Import(%s) = %p, %v; want the session read", s.UUID, got, ok)
				}
			}
			for _, s := range info {
				if got, _ := db.Import(s.UUID); got != s {
					t.Errorf("session %s of ImportInfo is not the one of Import", s.UUID)
				}
			}
		})
	}
	if sessions == 0 || outside == 0 || imported == 0 {
		t.Errorf("%d import sessions, %d outside the trash, %d photos with a session in all libraries; want some of each", sessions, outside, imported)
	}
}
