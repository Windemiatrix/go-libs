package photosdb

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/momentinfo"
)

type momentText struct{ Title, Subtitle, Start, End, Date, Modified, Lat, Lon string }

// RFC 3339 shows the offset of a date, which a comparison of instants would not.
func textOf(m momentinfo.MomentInfo) momentText {
	coordinate := func(p *float64) string {
		if p == nil {
			return "nil"
		}
		return strconv.FormatFloat(*p, 'g', -1, 64)
	}
	date := func(t time.Time) string { return t.Format(time.RFC3339Nano) }
	return momentText{
		Title: m.Title, Subtitle: m.Subtitle,
		Start: date(m.StartDate), End: date(m.EndDate), Date: date(m.Date), Modified: date(m.ModificationDate),
		Lat: coordinate(m.Latitude), Lon: coordinate(m.Longitude),
	}
}

func momentDB(t *testing.T, in tables) *DB {
	t.Helper()
	c, err := load(in)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return &DB{photos: c.photos, byUUID: c.byUUID, momentData: loadMoments(in, c)}
}

func momentUUIDs(photos []*photoInfo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.UUID)
	}
	return out
}

// The expected values are literals: the moments of photosdb.py:2734-2785 from
// built rows. The fixture titles are invented.
func TestLoadMoments(t *testing.T) {
	t.Parallel()

	const epochText = "1970-01-01T00:00:00Z"
	// Core Data second 0 is 2001-01-01T00:00:00Z.
	at := func(offset int64, seconds float64) sqlread.Moment {
		return sqlread.Moment{
			PK: 1, TZOffset: nullInt(offset),
			Start: nullFloat(seconds), End: nullFloat(seconds), Representative: nullFloat(seconds), Modified: nullFloat(seconds),
		}
	}
	allDates := func(s string) func(*momentText) {
		return func(w *momentText) { w.Start, w.End, w.Date, w.Modified = s, s, s, s }
	}
	tests := []struct {
		name string
		row  sqlread.Moment
		edit func(*momentText)
	}{
		{"every column NULL", sqlread.Moment{PK: 1}, nil},
		{"title and subtitle as they are", sqlread.Moment{PK: 1, Title: nullString("Beach Day"), Subtitle: nullString("")},
			func(w *momentText) { w.Title = "Beach Day" }},
		// U+00A0 is what Photos writes for a space: NFC keeps it, NFKC does not.
		{"no-break space in the subtitle", sqlread.Moment{PK: 1, Title: nullString("Beach Day"), Subtitle: nullString("Old\U000000a0Town")},
			func(w *momentText) { w.Title, w.Subtitle = "Beach Day", "Old Town" }},
		// U+FB01 is a ligature: NFC keeps it, NFKC decomposes it.
		{"ligature in the title", sqlread.Moment{PK: 1, Title: nullString("\U0000fb01sh")},
			func(w *momentText) { w.Title = "fish" }},

		{"both coordinates -180", sqlread.Moment{PK: 1, Latitude: nullFloat(-180), Longitude: nullFloat(-180)}, nil},
		{"one coordinate -180", sqlread.Moment{PK: 1, Latitude: nullFloat(-180), Longitude: nullFloat(10.5)},
			func(w *momentText) { w.Lat, w.Lon = "-180", "10.5" }},
		{"latitude NULL", sqlread.Moment{PK: 1, Longitude: nullFloat(5)}, func(w *momentText) { w.Lon = "5" }},
		{"longitude NULL", sqlread.Moment{PK: 1, Latitude: nullFloat(7.5)}, func(w *momentText) { w.Lat = "7.5" }},
		{"latitude -180, longitude NULL", sqlread.Moment{PK: 1, Latitude: nullFloat(-180)}, func(w *momentText) { w.Lat = "-180" }},

		{"each date in the zone of the offset",
			sqlread.Moment{PK: 1, TZOffset: nullInt(3600), Start: nullFloat(0), End: nullFloat(3600), Representative: nullFloat(7200), Modified: nullFloat(60)},
			func(w *momentText) {
				w.Start, w.End, w.Date, w.Modified = "2001-01-01T01:00:00+01:00", "2001-01-01T02:00:00+01:00", "2001-01-01T03:00:00+01:00", "2001-01-01T01:01:00+01:00"
			}},
		{"NULL offset is UTC", sqlread.Moment{PK: 1, Start: nullFloat(0)}, func(w *momentText) { w.Start = "2001-01-01T00:00:00Z" }},
		{"fraction of a second", sqlread.Moment{PK: 1, Start: nullFloat(0.5)}, func(w *momentText) { w.Start = "2001-01-01T00:00:00.5Z" }},
		{"NULL date is the epoch, in UTC", sqlread.Moment{PK: 1, TZOffset: nullInt(3600), End: nullFloat(0)},
			func(w *momentText) { w.End = "2001-01-01T01:00:00+01:00" }},
		{"date beyond the year 9999", sqlread.Moment{PK: 1, TZOffset: nullInt(3600), Start: nullFloat(1e13), End: nullFloat(0)},
			func(w *momentText) { w.End = "2001-01-01T01:00:00+01:00" }},

		// datetime.timezone takes only -86400 < offset < 86400 seconds (photos_datetime.py:64-67).
		{"offset 86400", at(86400, 0), allDates(epochText)},
		{"offset -86400", at(-86400, 0), allDates(epochText)},
		{"offset beyond 32 bits", at(1<<40, 0), allDates(epochText)},
		{"offset 86340", at(86340, 0), allDates("2001-01-01T23:59:00+23:59")},
		{"offset -86340", at(-86340, 0), allDates("2000-12-31T00:01:00-23:59")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := momentText{Start: epochText, End: epochText, Date: epochText, Modified: epochText, Lat: "nil", Lon: "nil"}
			if tc.edit != nil {
				tc.edit(&want)
			}
			got := loadMoments(tables{moments: []sqlread.Moment{tc.row}}, &catalog{}).moments[1]
			if got.PK != 1 || textOf(got) != want {
				t.Errorf("moment %+v, want PK 1 and %+v", textOf(got), want)
			}
		})
	}
}

// The moment 5 has no photo and A-4 names a moment without a row: such a photo
// has no moment, as upstream catches the ValueError of MomentInfo
// (photoinfo.py:625-631). The same holds for A-3 with a NULL ZMOMENT although
// a row has the Z_PK 0. A-2 in the trash and A-5, an unselected burst frame,
// stay in MomentPhotos: photos_by_uuid does not filter (photosdb.py:3378-3394).
func TestMomentInfoOf(t *testing.T) {
	t.Parallel()

	asset := func(pk int64, uuid string, moment int64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		if moment != 0 {
			a.MomentPK = nullInt(moment)
		}
		return a
	}
	// The catalog is in ZUUID order, the Z_PK of A-1 and A-2 runs the other way. The asset without
	// attributes shares the ZUUID of A-1 and has the moment 5.
	skipped := asset(5, "A-1", 5)
	skipped.HasAttributes = false
	trashed := asset(3, "A-2", 9)
	trashed.TrashedState = nullInt(trashedState)
	frame := asset(6, "A-5", 9)
	frame.BurstUUID, frame.BurstPickType = nullString("B"), nullInt(0)
	db := momentDB(t, tables{
		assets:  []sqlread.Asset{asset(4, "A-1", 9), skipped, trashed, asset(2, "A-3", 0), asset(1, "A-4", 77), frame},
		moments: []sqlread.Moment{{PK: 0, Title: nullString("Zero Day")}, {PK: 5, Title: nullString("Empty Day")}, {PK: 9, Title: nullString("Beach Day")}},
	})

	for _, uuid := range []string{"A-1", "A-2", "A-5"} {
		if m, ok := db.MomentInfoOf(db.byUUID[uuid]); !ok || m.PK != 9 || m.Title != "Beach Day" {
			t.Errorf("MomentInfoOf(%s) = PK %d, %v; want the moment 9, true", uuid, m.PK, ok)
		}
	}
	for _, uuid := range []string{"A-3", "A-4", "A-0"} {
		if _, ok := db.MomentInfoOf(&photoInfo{UUID: uuid}); ok {
			t.Errorf("MomentInfoOf(%s) is true; want false: no moment, a moment without a row, a photo outside the catalog", uuid)
		}
	}

	m9, _ := db.MomentInfoOf(db.byUUID["A-1"])
	photos := db.MomentPhotos(m9)
	if got := momentUUIDs(photos); !slices.Equal(got, []string{"A-1", "A-2", "A-5"}) {
		t.Errorf("MomentPhotos = %v, want [A-1 A-2 A-5]: the ZUUID order, not that of Z_PK, the trashed and the burst frame included", got)
	}
	// 5 is named only by the asset without attributes, 77 by A-4 without a row, 0 by nothing.
	for _, pk := range []int64{5, 77, 0} {
		if got := db.MomentPhotos(momentinfo.MomentInfo{PK: pk}); got != nil {
			t.Errorf("MomentPhotos of the Z_PK %d has %d photos, want nil", pk, len(got))
		}
	}
	photos[0] = nil
	if db.MomentPhotos(m9)[0] == nil {
		t.Error("MomentPhotos shares its slice")
	}
}

// The moments are written into the copy of the synthetic library, so that the
// case does not depend on its rows: one with a title and a location, one
// without, one with every column and a photo whose moment is gone. The rows are
// chosen by integer keys.
func TestLoadMomentsDamaged(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	photos := before.Photos()
	if len(photos) < 4 {
		t.Fatalf("the synthetic library has %d photos, want 4 or more: the case is vacuous", len(photos))
	}
	pk := func(i int) string { return strconv.FormatInt(photos[i].PK, 10) }
	change(t, lib,
		"INSERT INTO ZMOMENT (Z_PK, ZTITLE, ZAPPROXIMATELATITUDE, ZAPPROXIMATELONGITUDE) VALUES (900001, 'Beach Day', 10.5, 20.5)",
		"INSERT INTO ZMOMENT (Z_PK, ZTITLE, ZAPPROXIMATELATITUDE, ZAPPROXIMATELONGITUDE) VALUES (900002, NULL, -180, -180)",
		"INSERT INTO ZMOMENT (Z_PK, ZTITLE, ZSUBTITLE, ZTIMEZONEOFFSET, ZSTARTDATE, ZENDDATE, ZREPRESENTATIVEDATE, ZMODIFICATIONDATE) "+
			"VALUES (900004, 'Harbor Day', 'Old Town', 3600, 0, 3600, 7200, 60)",
		"UPDATE ZASSET SET ZMOMENT = 900001 WHERE Z_PK = "+pk(0),
		"UPDATE ZASSET SET ZMOMENT = 900002 WHERE Z_PK = "+pk(1),
		"UPDATE ZASSET SET ZMOMENT = 900003 WHERE Z_PK = "+pk(2),
		"UPDATE ZASSET SET ZMOMENT = 900004 WHERE Z_PK = "+pk(3),
	)

	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	photo := func(i int) *photoInfo {
		p, ok := after.Photo(photos[i].UUID)
		if !ok {
			t.Fatalf("photo %d is gone", i)
		}
		return p
	}
	m, ok := after.MomentInfoOf(photo(0))
	if text := textOf(m); !ok || m.PK != 900001 || text.Title != "Beach Day" || text.Lat != "10.5" || text.Lon != "20.5" {
		t.Errorf("photo 0: PK %d, %v, location %s %s; want the moment 900001 with a title at 10.5 20.5", m.PK, ok, text.Lat, text.Lon)
	}
	m, ok = after.MomentInfoOf(photo(1))
	if text := textOf(m); !ok || m.PK != 900002 || text.Title != "" || text.Lat != "nil" || text.Lon != "nil" {
		t.Errorf("photo 1: PK %d, %v, location %s %s; want the moment 900002 with an empty title at nil nil", m.PK, ok, text.Lat, text.Lon)
	}
	if _, has := after.MomentInfoOf(photo(2)); has {
		t.Error("photo 2 names a moment without a row and has a moment")
	}
	if got := momentUUIDs(after.MomentPhotos(m)); !slices.Equal(got, []string{photos[1].UUID}) {
		t.Errorf("MomentPhotos of the moment 900002 has %d photos, want the photo 1", len(got))
	}
	full, ok := after.MomentInfoOf(photo(3))
	wantFull := momentText{
		Title: "Harbor Day", Subtitle: "Old Town",
		Start: "2001-01-01T01:00:00+01:00", End: "2001-01-01T02:00:00+01:00", Date: "2001-01-01T03:00:00+01:00", Modified: "2001-01-01T01:01:00+01:00",
		Lat: "nil", Lon: "nil",
	}
	if got := textOf(full); !ok || full.PK != 900004 || got != wantFull {
		t.Errorf("photo 3: PK %d, %v, %+v; want the moment 900004 and %+v", full.PK, ok, got, wantFull)
	}

	var changed int
	for i, p := range photos[4:] {
		b, bok := before.MomentInfoOf(p)
		a, aok := after.MomentInfoOf(photo(i + 4))
		if bok != aok || textOf(a) != textOf(b) {
			changed++
		}
	}
	if changed != 0 {
		t.Errorf("%d of the %d other photos changed their moment", changed, len(photos)-4)
	}
}

// Upstream gives the epoch for a text date (photos_datetime.py:66-67); the
// difference is recorded in UPSTREAM.md.
func TestOpenMomentWrongType(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	change(t, lib, "INSERT INTO ZMOMENT (Z_PK, ZSTARTDATE) VALUES (900001, 'x')")
	if _, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()}); err == nil {
		t.Error("Open succeeded with a text in ZMOMENT.ZSTARTDATE, want an error")
	}
}

// The oracle is the database: the counts of the same rows by SQL. The messages
// hold counts, not the titles of a library.
func TestLoadMomentsLibraries(t *testing.T) {
	t.Parallel()

	// Not a JOIN with the attributes: an asset with two attribute rows would count twice.
	const (
		withAttributes = " WHERE EXISTS (SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"
		ofMoment       = "SELECT COUNT(*) FROM ZASSET a JOIN ZMOMENT m ON m.Z_PK = a.ZMOMENT" + withAttributes
	)
	var mu sync.Mutex
	var moments, onMoment int
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				var with, titled, located int
				for _, p := range db.Photos() {
					m, ok := db.MomentInfoOf(p)
					if !ok {
						continue
					}
					with++
					if m.Title != "" {
						titled++
					}
					if m.Latitude != nil && m.Longitude != nil {
						located++
					}
				}
				var listed int
				for _, m := range db.moments {
					photos := db.MomentPhotos(m)
					listed += len(photos)
					if !slices.IsSortedFunc(photos, func(a, b *photoInfo) int { return strings.Compare(a.UUID, b.UUID) }) {
						t.Error("the photos of a moment are not in ZUUID order")
					}
				}
				for _, c := range []struct {
					name  string
					got   int
					query string
				}{
					{"moments", len(db.moments), "SELECT COUNT(*) FROM ZMOMENT"},
					{"photos with ZMOMENT", len(db.momentOf), "SELECT COUNT(*) FROM ZASSET a" + withAttributes + " AND a.ZMOMENT IS NOT NULL"},
					{"photos with a moment", with, ofMoment},
					{"photos in MomentPhotos", listed, ofMoment},
					{"photos with a title", titled, ofMoment + " AND m.ZTITLE IS NOT NULL AND m.ZTITLE != ''"},
					{"photos with a location", located, ofMoment + " AND m.ZAPPROXIMATELATITUDE IS NOT NULL AND m.ZAPPROXIMATELONGITUDE IS NOT NULL" +
						" AND NOT (m.ZAPPROXIMATELATITUDE = -180 AND m.ZAPPROXIMATELONGITUDE = -180)"},
				} {
					if want := count(t, ro, c.query); c.got != want {
						t.Errorf("%s: %d, want %d", c.name, c.got, want)
					}
				}
				t.Logf("%d moments; %d photos with a moment, %d with a title, %d with a location", len(db.moments), with, titled, located)
				mu.Lock()
				defer mu.Unlock()
				moments += len(db.moments)
				onMoment += with
			})
		}
	})
	if moments == 0 || onMoment == 0 {
		t.Errorf("%d moments and %d photos with a moment in all libraries; want some of both: the test is vacuous", moments, onMoment)
	}
}

// {moment} and {moment?Y,N} are the titles of the moments in the osxphotos
// rendering (phototemplate.py:971-972, 1012): a missing moment and an empty
// title both give "_". The messages hold ids and counts, not the titles.
func TestMomentOracle(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	lines, err := golden.TemplateLines(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lines["t501"] != "{moment}" || lines["t502"] != "{moment?Y,N}" {
		t.Fatal("t501 or t502 of templates.txt is not the moment template: the ids moved")
	}
	var mu sync.Mutex
	var titled int
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				want, err := golden.Templates(dir, lib.name)
				if err != nil {
					t.Fatal(err)
				}
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				var missing, shape, badTitle, badFlag, withTitle int
				for uuid, values := range want {
					p, ok := db.Photo(uuid)
					if !ok {
						missing++
						continue
					}
					if len(values["t501"]) != 1 || len(values["t502"]) != 1 {
						shape++
						continue
					}
					title, flag := "_", "N"
					if m, has := db.MomentInfoOf(p); has && m.Title != "" {
						title, flag = m.Title, "Y"
						withTitle++
					}
					if values["t501"][0] != title {
						badTitle++
					}
					if values["t502"][0] != flag {
						badFlag++
					}
				}
				if missing+shape+badTitle+badFlag != 0 {
					t.Errorf("of %d photos of the reference: %d are not in the library, %d have no single value of t501 or t502, t501 differs in %d, t502 in %d",
						len(want), missing, shape, badTitle, badFlag)
				}
				t.Logf("%d photos in the reference, %d with a moment title", len(want), withTitle)
				mu.Lock()
				defer mu.Unlock()
				titled += withTitle
			})
		}
	})
	if titled == 0 {
		t.Error("no photo has a moment title in any library: the test is vacuous")
	}
}
