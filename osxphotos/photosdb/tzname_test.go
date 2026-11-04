package photosdb

import (
	"database/sql"
	"testing"
	"time"
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
		// EST is an IANA name and an override; the name wins (tzcanonical.py:402-424).
		{"IANA name before override", summerTS, nullInt(-14400), "EST", summer, -18000, "EST"},
		{"IANA name before override, winter", winterTS, nullInt(-18000), "EST", winter, -18000, "EST"},
		{"GMT+0300, equal offset", winterTS, nullInt(10800), "GMT+0300", winter, 10800, "Etc/GMT-3"},
		{"GMT-0500, equal offset", winterTS, nullInt(-18000), "GMT-0500", winter, -18000, "Etc/GMT+5"},
		{"GMT+0000, equal offset", winterTS, nullInt(0), "GMT+0000", winter, 0, "Etc/UTC"},
		{"GMT-0330, equal offset", winterTS, nullInt(-12600), "GMT-0330", winter, -12600, ""},
		{"offset string with spaces", winterTS, nullInt(10800), " +03:00 ", winter, 10800, "Etc/GMT-3"},
		{"GMT+0300, another offset", winterTS, nullInt(7200), "GMT+0300", winter, 7200, ""},
		{"GMT+0300, offset NULL", winterTS, sql.NullInt64{}, "GMT+0300", winter, 0, "UTC"},
		{"GMT+0530", winterTS, nullInt(19800), "GMT+0530", winter, 19800, ""},
		{"GMT+0545", winterTS, nullInt(20700), "GMT+0545", winter, 20700, ""},
		// There is no Etc/GMT-15 (tzcanonical.py:57).
		{"offset string beyond Etc zones", winterTS, nullInt(54000), "+1500", winter, 54000, ""},
		{"UTC", winterTS, nullInt(3600), "UTC", winter, 0, "UTC"},
		{"GMT", winterTS, nullInt(3600), "GMT", winter, 0, "GMT"},
		// The IANA name wins over the offset string (tzcanonical.py:402-403).
		{"GMT, offset 0", winterTS, nullInt(0), "GMT", winter, 0, "GMT"},
		{"UTC, offset 0", winterTS, nullInt(0), "UTC", winter, 0, "UTC"},
		{"Z, offset 0", winterTS, nullInt(0), "Z", winter, 0, "Etc/UTC"},
		{"Z, another offset", winterTS, nullInt(3600), "Z", winter, 3600, ""},
		{"override", summerTS, nullInt(-25200), "PDT", summer, -25200, "America/Los_Angeles"},
		{"override, lower case", summerTS, nullInt(-25200), "pdt", summer, -25200, "America/Los_Angeles"},
		{"override, another offset", winterTS, nullInt(-25200), "PDT", winter, -25200, ""},
		{"override, offset NULL", winterTS, sql.NullInt64{}, "BST", winter, 0, "Europe/London"},
		{"override, not a zone name", summerTS, nullInt(7200), "CEST", summer, 7200, "Europe/Berlin"},
		{"unknown name", winterTS, nullInt(3600), "Nowhere/Town", winter, 3600, ""},
		{"unknown name, offset out of range", winterTS, nullInt(86400), "Nowhere/Town", epoch, 0, "UTC"},
		{"empty name", winterTS, nullInt(3600), "", winter, 3600, ""},
		// photos_datetime.py:54 tests the name before canonical_timezone strips it.
		{"spaces only", winterTS, nullInt(3600), "  ", winter, 3600, "Etc/GMT-1"},
		{"spaces only, not a whole hour", winterTS, nullInt(19800), "  ", winter, 19800, ""},
		{"spaces only, negative half hour", winterTS, nullInt(-12600), "  ", winter, -12600, ""},
		{"Local", winterTS, nullInt(3600), "Local", winter, 3600, ""},
		{"path out of the database", winterTS, nullInt(3600), "../Europe/Paris", winter, 3600, ""},
		{"path with a dot", winterTS, nullInt(3600), "./Europe/Paris", winter, 3600, ""},
		{"path with a double slash", winterTS, nullInt(3600), "Europe//Paris", winter, 3600, ""},
		{"absolute path", winterTS, nullInt(3600), "/usr/share/zoneinfo/Europe/Paris", winter, 3600, ""},
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

func TestAssetDateOffsetStringAbbreviation(t *testing.T) {
	t.Parallel()

	// tzdata 2017a and later name Etc/GMT-3 "+03", not "GMT-3".
	tests := []struct {
		tzName string
		offset int64
		want   string
	}{
		{"GMT+0300", 10800, "+03"},
		{"GMT-0500", -18000, "-05"},
		{"Z", 0, "UTC"},
	}
	for _, tt := range tests {
		got := assetDate(nullFloat(694224000), nullInt(tt.offset), nullString(tt.tzName))
		if s := got.Format("MST"); s != tt.want {
			t.Errorf("%s, %d: zone %q of %v, want %q", tt.tzName, tt.offset, s, got.Location(), tt.want)
		}
	}
}

func TestParseOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		want   int64
		wantOK bool
	}{
		{"GMT+0300", 10800, true},
		{"GMT+03:00", 10800, true},
		{"gmt-0545", -20700, true},
		{"+0300", 10800, true},
		{"-05:45", -20700, true},
		{"-00:00", 0, true},
		{"3:45", 13500, true},
		{"03:00", 10800, true},
		{"Z", 0, true},
		{"z", 0, true},
		{"utc", 0, true},
		{"gmt", 0, true},
		{"", 0, false},
		{"GMT+3", 0, false},
		{"GMT+030", 0, false},
		{"GMT03:00", 0, false},
		{"+03", 0, false},
		{"0300", 0, false},
		{"-3:00", 0, false},
		{"UTC+0300", 0, false},
		{"GMT+0300x", 0, false},
		{"x+0300", 0, false},
		{"103:45", 0, false},
		// Python \d also matches these digits: upstream reads 10800.
		{"+\U00000660\U00000663\U00000660\U00000660", 0, false},
	}
	for _, tt := range tests {
		if got, ok := parseOffset(tt.name); got != tt.want || ok != tt.wantOK {
			t.Errorf("parseOffset(%q) = %d, %v, want %d, %v", tt.name, got, ok, tt.want, tt.wantOK)
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

func TestLoadZoneCache(t *testing.T) {
	t.Parallel()

	if a, b := loadZone("Europe/Paris"), loadZone("Europe/Paris"); a == nil || a != b {
		t.Errorf("loadZone gives %p and %p, want one Location", a, b)
	}
	const missing = "Nowhere/Cached"
	if z := loadZone(missing); z != nil {
		t.Fatalf("loadZone(%q) = %v, want nil", missing, z)
	}
	if v, ok := zones.Load(missing); !ok || v.(*time.Location) != nil {
		t.Errorf("zones has %v, %v for %q; want a cached nil", v, ok, missing)
	}
}
