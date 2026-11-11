package photosdb

import (
	"bytes"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const cloudPlistHeader = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0">`

func cloudXML(entries string) string {
	return cloudPlistHeader + "<dict>" + entries + "</dict></plist>"
}

// One entry of every type a property list holds.
const cloudMetaEntries = `<key>text</key><string>value</string><key>count</key><integer>7</integer><key>negative</key><integer>-3</integer>` +
	`<key>ratio</key><real>0.5</real><key>flag</key><true/><key>when</key><date>2020-01-02T03:04:05Z</date><key>blob</key><data>AQI=</data>` +
	`<key>nested</key><dict><key>inner</key><string>x</string></dict><key>list</key><array><string>a</string><integer>1</integer></array>`

// A property list decodes a non-negative integer as uint64, a negative one as
// int64.
func cloudMetaWant() map[string]any {
	return map[string]any{
		"text":     "value",
		"count":    uint64(7),
		"negative": int64(-3),
		"ratio":    0.5,
		"flag":     true,
		"when":     time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
		"blob":     []byte{1, 2},
		"nested":   map[string]any{"inner": "x"},
		"list":     []any{"a", uint64(1)},
	}
}

// The dates are compared as moments in UTC: reflect.DeepEqual would compare
// the pointers to the zones.
func sameCloudMetadata(got, want map[string]any) bool {
	g, w := maps.Clone(got), maps.Clone(want)
	gt, gotDate := g["when"].(time.Time)
	wt, wantDate := w["when"].(time.Time)
	delete(g, "when")
	delete(w, "when")
	return gotDate == wantDate && (!gotDate || (gt.Equal(wt) && gt.Location() == time.UTC)) && reflect.DeepEqual(g, w)
}

// The expectations are literals. The property list is read as plistlib.loads
// reads it, and any failure is nil where upstream gives {}
// (photoinfo.py:1755-1757).
func TestCloudMetadataOf(t *testing.T) {
	t.Parallel()

	ints := make([]any, maxBinaryObjects+1)
	for i := range ints {
		ints[i] = i
	}
	tests := []struct {
		name string
		data []byte
		want map[string]any
	}{
		{"every type", []byte(cloudXML(cloudMetaEntries)), cloudMetaWant()},
		{"a binary property list", marshal(t, map[string]any{"text": "value", "count": 7}, plist.BinaryFormat),
			map[string]any{"text": "value", "count": uint64(7)}},
		{"a binary property list under the object limit", marshal(t, map[string]any{"list": ints[:3]}, plist.BinaryFormat),
			map[string]any{"list": []any{uint64(0), uint64(1), uint64(2)}}},
		{"a binary property list over the object limit", marshal(t, map[string]any{"list": ints}, plist.BinaryFormat), nil},
		{"a root that is an array", []byte(cloudPlistHeader + "<array><string>a</string></array></plist>"), nil},
		{"a root that is a string", []byte(cloudPlistHeader + "<string>a</string></plist>"), nil},
		{"text that is not a property list", []byte("not a property list"), nil},
		{"empty data", []byte{}, nil},
		// The padding is inside the document, so that only the size can refuse it.
		{"data of 1 MiB", []byte(cloudXML(cloudMetaEntries + strings.Repeat(" ", maxPlistSize-len(cloudXML(cloudMetaEntries))))), cloudMetaWant()},
		{"data over 1 MiB", []byte(cloudXML(cloudMetaEntries + strings.Repeat(" ", maxPlistSize))), nil},
	}
	for _, tt := range tests {
		db := &DB{cloudMeta: map[string][]byte{"A-1": tt.data}}
		if got := db.CloudMetadataOf(&photoinfo.PhotoInfo{UUID: "A-1"}); !sameCloudMetadata(got, tt.want) {
			t.Errorf("%s: %d keys, want %d", tt.name, len(got), len(tt.want))
		}
	}

	db := &DB{cloudMeta: map[string][]byte{"A-1": []byte(cloudXML(cloudMetaEntries))}}
	if got := db.CloudMetadataOf(&photoinfo.PhotoInfo{UUID: "A-2"}); got != nil {
		t.Errorf("a photo without a row: %d keys, want nil", len(got))
	}
	if got := (&DB{}).CloudMetadataOf(&photoinfo.PhotoInfo{UUID: "A-1"}); got != nil {
		t.Errorf("a DB without rows: %d keys, want nil", len(got))
	}

	// The map is the caller's: a change must not reach the next call.
	p := &photoinfo.PhotoInfo{UUID: "A-1"}
	first := db.CloudMetadataOf(p)
	first["text"] = "changed"
	first["nested"].(map[string]any)["inner"] = "changed"
	first["list"].([]any)[0] = "changed"
	delete(first, "flag")
	if second := db.CloudMetadataOf(p); !sameCloudMetadata(second, cloudMetaWant()) {
		t.Error("a change of the first result shows in the second")
	}
}

// The rows are written into the copy of the synthetic library, so that the
// case does not depend on the rows it has; they are picked by integer keys, so
// that no identifier of a library is in the test.
func TestCloudMetadataDamaged(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	photos := before.Photos()
	if len(photos) < 7 {
		t.Fatalf("the synthetic library has %d photos, want 7 or more: the case is vacuous", len(photos))
	}
	pk := func(i int) string { return strconv.FormatInt(photos[i].PK, 10) }
	const insert = "INSERT INTO ZCLOUDMASTERMEDIAMETADATA (Z_PK, ZCLOUDMASTER, ZDATA) VALUES "
	xml := func(v string) string { return cloudXML("<key>k</key><string>" + v + "</string>") }
	blob := func(v string) string { return "CAST('" + xml(v) + "' AS BLOB)" }
	change(t, lib,
		"INSERT INTO ZCLOUDMASTER (Z_PK) VALUES (900001), (900002), (900003), (900004), (900005), (900006), (900009)",
		"UPDATE ZASSET SET ZMASTER = 900001 WHERE Z_PK = "+pk(0),
		"UPDATE ZASSET SET ZMASTER = 900002 WHERE Z_PK = "+pk(1),
		"UPDATE ZASSET SET ZMASTER = 900003 WHERE Z_PK = "+pk(2),
		"UPDATE ZASSET SET ZMASTER = 900004 WHERE Z_PK = "+pk(3),
		"UPDATE ZASSET SET ZMASTER = 900005 WHERE Z_PK = "+pk(4),
		"UPDATE ZASSET SET ZMASTER = 900006 WHERE Z_PK = "+pk(5),
		insert+"(900010, 900001, "+blob("first")+")",
		// The row with the higher Z_PK is written first.
		insert+"(900012, 900002, "+blob("new")+")",
		insert+"(900011, 900002, "+blob("old")+")",
		insert+"(900008, 900003, "+blob("older")+")",
		insert+"(900013, 900003, zeroblob(1048577))",
		insert+"(900007, 900004, "+blob("older")+")",
		insert+"(900014, 900004, NULL)",
		// plistlib.loads of an integer or a str raises inside suppress(Exception): upstream gives {} (photoinfo.py:1755-1757).
		insert+"(900015, 900005, 424242)",
		insert+"(900016, 900006, '"+xml("text")+"')",
		// A LEFT JOIN would hand Scan a NULL asset for the master that no asset has.
		insert+"(900019, 900009, "+blob("orphan")+")",
	)

	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	metadata := func(i int) map[string]any {
		p, ok := after.Photo(photos[i].UUID)
		if !ok {
			t.Fatalf("photo %d is gone", i)
		}
		return after.CloudMetadataOf(p)
	}
	for _, c := range []struct {
		name string
		got  map[string]any
		want map[string]any
	}{
		{"one row", metadata(0), map[string]any{"k": "first"}},
		{"two rows of one master: the highest Z_PK decides", metadata(1), map[string]any{"k": "new"}},
		{"a value over 1 MiB after an older one: the last row decides", metadata(2), nil},
		{"a NULL value after an older one: the older one decides", metadata(3), map[string]any{"k": "older"}},
		{"an integer value", metadata(4), nil},
		{"a text value that is a valid property list", metadata(5), nil},
	} {
		if !sameCloudMetadata(c.got, c.want) {
			t.Errorf("%s: %d keys, want %d", c.name, len(c.got), len(c.want))
		}
	}

	// Of the first six photos only those with a blob keep data.
	want, changed := 3, 0
	for _, p := range photos[6:] {
		old, had := before.cloudMeta[p.UUID]
		if had {
			want++
		}
		if !bytes.Equal(after.cloudMeta[p.UUID], old) {
			changed++
		}
	}
	if len(after.cloudMeta) != want || changed != 0 {
		t.Errorf("%d photos with data, want %d; %d of the %d other photos changed their data", len(after.cloudMeta), want, changed, len(photos)-6)
	}
}

// The oracle is the database: the rows load keeps, by SQL. They are the rows of
// the assets that have an attributes row, of the highest Z_PK of their master
// that has a value, with a blob of 1 byte to 1 MiB. The messages hold counts,
// not the metadata of a library.
func TestCloudMetadataLibraries(t *testing.T) {
	t.Parallel()

	const rows = " FROM ZCLOUDMASTERMEDIAMETADATA md JOIN ZCLOUDMASTER m ON m.Z_PK = md.ZCLOUDMASTER JOIN ZASSET a ON a.ZMASTER = m.Z_PK" +
		" WHERE md.ZDATA IS NOT NULL AND typeof(md.ZDATA) = 'blob' AND length(md.ZDATA) BETWEEN 1 AND 1048576 AND a.ZUUID IS NOT NULL" +
		" AND EXISTS (SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)" +
		" AND md.Z_PK = (SELECT MAX(md2.Z_PK) FROM ZCLOUDMASTERMEDIAMETADATA md2 WHERE md2.ZCLOUDMASTER = md.ZCLOUDMASTER AND md2.ZDATA IS NOT NULL)"

	var decoded, nulls, dates atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				wantRows := testlib.Count(t, ro, "SELECT COUNT(*)"+rows)
				wantBytes := testlib.Count(t, ro, "SELECT COALESCE(SUM(length(md.ZDATA)), 0)"+rows)

				size := 0
				for _, data := range db.cloudMeta {
					size += len(data)
				}
				nonEmpty, keys, withNulls, withDates := 0, 0, 0, 0
				for _, p := range db.Photos() {
					if m := db.CloudMetadataOf(p); len(m) > 0 {
						if nonEmpty == 0 {
							keys = len(m)
						}
						nonEmpty++
						hasNull, hasDate := cloudKinds(m)
						if hasNull {
							withNulls++
						}
						if hasDate {
							withDates++
						}
					}
				}
				if len(db.cloudMeta) != wantRows || size != wantBytes {
					t.Errorf("%d photos with data and %d bytes; the database has %d and %d", len(db.cloudMeta), size, wantRows, wantBytes)
				}
				// Measured: the only library with data.
				if lib.name == "Test-Cloud-13.1" && (len(db.cloudMeta) < 23 || size < 45046) {
					t.Errorf("%d photos with data and %d bytes, want 23 and 45046 or more", len(db.cloudMeta), size)
				}
				t.Logf("Photos %v: %d photos with data, %d bytes, %d non-empty maps, %d keys in the first, %d maps with bytes or a UID, %d with a date",
					db.info.PhotosVersion, len(db.cloudMeta), size, nonEmpty, keys, withNulls, withDates)
				decoded.Add(int64(nonEmpty))
				nulls.Add(int64(withNulls))
				dates.Add(int64(withDates))
			})
		}
	})
	if decoded.Load() == 0 {
		t.Error("no non-empty map in any library: the test is vacuous")
	}
	t.Logf("all libraries: %d maps with bytes or a UID (null in the reference), %d with a date", nulls.Load(), dates.Load())
}

// The values that json() writes as null, and the dates, are where the golden
// rule models upstream instead of copying it.
func cloudKinds(v any) (nulls, dates bool) {
	var children []any
	switch x := v.(type) {
	case []byte, plist.UID:
		return true, false
	case time.Time:
		return false, true
	case map[string]any:
		children = slices.Collect(maps.Values(x))
	case []any:
		children = x
	}
	for _, c := range children {
		n, d := cloudKinds(c)
		nulls, dates = nulls || n, dates || d
	}
	return nulls, dates
}
