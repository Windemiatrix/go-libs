package photosdb

import (
	"path"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // upstream depends on the tzdata package (pyproject.toml:114); libraries are read on Linux too, where the system may have no zoneinfo
)

// TZ_OVERRIDES of tzcanonical.py:60-87.
var tzOverrides = map[string]string{
	"IDT":  "Asia/Jerusalem",
	"BST":  "Europe/London",
	"CET":  "Europe/Berlin",
	"CEST": "Europe/Berlin",
	"PDT":  "America/Los_Angeles",
	"PST":  "America/Los_Angeles",
	"MDT":  "America/Denver",
	"MST":  "America/Denver",
	"CDT":  "America/Chicago",
	"CST":  "America/Chicago",
	"EDT":  "America/New_York",
	"EST":  "America/New_York",
	"AKDT": "America/Anchorage",
	"AKST": "America/Anchorage",
	"HST":  "Pacific/Honolulu",
	"JST":  "Asia/Tokyo",
	"KST":  "Asia/Seoul",
	"AEST": "Australia/Sydney",
	"AEDT": "Australia/Sydney",
	"ACST": "Australia/Adelaide",
	"ACDT": "Australia/Adelaide",
	"AWST": "Australia/Perth",
	"NZDT": "Pacific/Auckland",
	"NZST": "Pacific/Auckland",
	"UTC":  "Etc/UTC",
	"GMT":  "Etc/GMT",
}

// nameZone follows canonical_timezone (tzcanonical.py:398-424); nil stands for
// the fixed zone of offset. An offset string equal to offset gives a zone with
// that offset upstream (tzcanonical.py:412-415), and the zones upstream
// searches for any other name have offset (tzcanonical.py:426-446), so neither
// is ported.
// Upstream checks the offset of an override at the naive local time of the
// photo (tzcanonical.py:300-321); here it is checked at the moment.
func nameZone(t time.Time, offset int64, name string) *time.Location {
	name = strings.TrimSpace(name)
	if z := loadZone(name); z != nil {
		return z
	}
	if z := loadZone(tzOverrides[strings.ToUpper(name)]); z != nil {
		if _, off := t.In(z).Zone(); int64(off) == offset {
			return z
		}
	}
	return nil
}

// time.LoadLocation reads the zoneinfo on every call and returns a new
// Location; upstream caches the ZoneInfo or None of each name
// (tzcanonical.py:235-241).
var zones sync.Map

// zoneinfo.ZoneInfo has no "Local", which time.LoadLocation reads as the zone
// of the computer, and rejects a key that is not a normalized path, such as ""
// (CPython zoneinfo/_tzpath.py, _validate_tzfile_path).
func loadZone(name string) *time.Location {
	if name == "Local" || path.Clean(name) != name {
		return nil
	}
	if v, ok := zones.Load(name); ok {
		return v.(*time.Location)
	}
	z, err := time.LoadLocation(name)
	if err != nil {
		z = nil
	}
	v, _ := zones.LoadOrStore(name, z)
	return v.(*time.Location)
}
