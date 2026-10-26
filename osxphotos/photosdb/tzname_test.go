package photosdb

import (
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestAssetDateZoneName(t *testing.T) {
	t.Parallel()

	winterTS, summerTS := nullFloat(694224000), nullFloat(709862400)
	winter := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	summer := time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		created sql.NullFloat64
		tz      sql.NullInt64
		tzName  string
		want    time.Time
		offset  int
		zone    string // Location().String(): "" is the fixed zone of the offset
	}{
		{"IANA, offset of the zone", winterTS, nullInt(3600), "Europe/Paris", winter, 3600, "Europe/Paris"},
		{"IANA, another offset", winterTS, nullInt(7200), "Europe/Paris", winter, 3600, "Europe/Paris"},
		{"IANA, summer", summerTS, nullInt(3600), "Europe/Paris", summer, 7200, "Europe/Paris"},
		{"IANA, offset NULL", winterTS, sql.NullInt64{}, "America/New_York", winter, -18000, "America/New_York"},
		// The range error is raised only by the fixed zone (photos_datetime.py:64-67).
		{"IANA, offset out of range", winterTS, nullInt(86400), "Europe/Paris", winter, 3600, "Europe/Paris"},
		{"IANA with spaces", winterTS, nullInt(7200), " Europe/Paris\t", winter, 3600, "Europe/Paris"},
		{"GMT+0300, equal offset", winterTS, nullInt(10800), "GMT+0300", winter, 10800, ""},
		{"GMT+0300, another offset", winterTS, nullInt(7200), "GMT+0300", winter, 7200, ""},
		{"GMT+0530", winterTS, nullInt(19800), "GMT+0530", winter, 19800, ""},
		{"UTC", winterTS, nullInt(3600), "UTC", winter, 0, "UTC"},
		{"GMT", winterTS, nullInt(3600), "GMT", winter, 0, "GMT"},
		{"Z, offset 0", winterTS, nullInt(0), "Z", winter, 0, "UTC"},
		{"Z, another offset", winterTS, nullInt(3600), "Z", winter, 3600, ""},
		{"override", summerTS, nullInt(-25200), "PDT", summer, -25200, "America/Los_Angeles"},
		{"override, lower case", summerTS, nullInt(-25200), "pdt", summer, -25200, "America/Los_Angeles"},
		{"override, another offset", winterTS, nullInt(-25200), "PDT", winter, -25200, ""},
		{"override, offset NULL", winterTS, sql.NullInt64{}, "BST", winter, 0, "Europe/London"},
		{"override, not a zone name", summerTS, nullInt(7200), "CEST", summer, 7200, "Europe/Berlin"},
		{"unknown name", winterTS, nullInt(3600), "Nowhere/Town", winter, 3600, ""},
		{"unknown name, offset out of range", winterTS, nullInt(86400), "Nowhere/Town", epoch, 0, "UTC"},
		{"empty name", winterTS, nullInt(3600), "", winter, 3600, ""},
		{"spaces only", winterTS, nullInt(3600), "  ", winter, 3600, ""},
		{"Local", winterTS, nullInt(3600), "Local", winter, 3600, ""},
		{"path out of the database", winterTS, nullInt(3600), "../Europe/Paris", winter, 3600, ""},
		{"no date", sql.NullFloat64{}, nullInt(3600), "Europe/Paris", epoch, 0, "UTC"},
	}
	for _, tt := range tests {
		got := assetDate(tt.created, tt.tz, nullString(tt.tzName))
		_, off := got.Zone()
		if !got.Equal(tt.want) || off != tt.offset || got.Location().String() != tt.zone {
			t.Errorf("%s: assetDate = %v (offset %d, zone %q), want %v (offset %d, zone %q)",
				tt.name, got, off, got.Location(), tt.want, tt.offset, tt.zone)
		}
	}
}

func TestTZOverrides(t *testing.T) {
	t.Parallel()

	if len(tzOverrides) != 26 { // tzcanonical.py:60-87
		t.Errorf("%d overrides, want 26", len(tzOverrides))
	}
	for abbr, name := range tzOverrides {
		if loadZone(name) == nil {
			t.Errorf("override %s: zone %s does not load", abbr, name)
		}
	}
}

// TestGolden compares date as a moment; the zone shows only in the offset.
func TestDateOffsetGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := golden.Load(dir, lib.name)
			if err != nil {
				t.Fatal(err)
			}
			dates := map[string]time.Time{}
			for _, p := range db.Photos() {
				dates[p.UUID] = p.Date
			}
			compared, differ := 0, 0
			for _, o := range slices.Concat(ref.Query, ref.Deleted) {
				var uuid, date string
				if json.Unmarshal(o["uuid"], &uuid) != nil || json.Unmarshal(o["date"], &date) != nil {
					t.Fatal("a reference object without the strings uuid and date")
				}
				want, ok := parseIsoformat(date)
				if !ok {
					t.Fatal("a reference date is not ISO 8601")
				}
				got, ok := dates[uuid]
				if !ok {
					continue // TestGolden reports the missing photo
				}
				compared++
				_, wantOff := want.Zone()
				if _, gotOff := got.Zone(); gotOff != wantOff {
					if differ == 0 {
						t.Errorf("first difference: offset %d, osxphotos %d", gotOff, wantOff)
					}
					differ++
				}
			}
			if compared == 0 || differ > 0 {
				t.Errorf("%d of %d dates have an offset other than that of osxphotos", differ, compared)
			}
		})
	}
}

// Python isoformat writes the seconds of an offset that has them, which
// time.RFC3339Nano does not read.
func parseIsoformat(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-07:00:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
