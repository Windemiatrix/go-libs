package photosdb

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
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
// the fixed zone of offset. For a name it resolves no other way, upstream
// searches every zone of the system (tzcanonical.py:426-446); package time
// cannot list them, so the fixed zone stands in.
// Upstream checks the offset of an override at the naive local time of the
// photo (tzcanonical.py:300-321); here it is checked at the moment.
func nameZone(t time.Time, offset int64, name string) *time.Location {
	token := strings.TrimSpace(name)
	if z := loadZone(token); z != nil {
		return z
	}
	if token == "" && name != "" { // photos_datetime.py:54 tests the name before it is stripped
		return loadZone(etcZone(offset))
	}
	if parsed, ok := parseOffset(token); ok && parsed == offset {
		return loadZone(etcZone(offset))
	}
	if z := loadZone(tzOverrides[strings.ToUpper(token)]); z != nil {
		if _, off := t.In(z).Zone(); int64(off) == offset {
			return z
		}
	}
	return nil
}

// etcZone is _etc_from_offset_seconds (tzcanonical.py:41-57), "" for None; the
// Etc/GMT zones invert the sign, as POSIX does.
func etcZone(offset int64) string {
	switch {
	case offset%3600 != 0:
		return ""
	case offset == 0:
		return "Etc/UTC"
	}
	return fmt.Sprintf("Etc/GMT%+d", -offset/3600)
}

// The three patterns of _parse_offset_str (tzcanonical.py:215, 221, 227).
var offsetString = regexp.MustCompile(`^(?:(?:GMT)?([+-])(\d{2}):?|(\d{1,2}):)(\d{2})$`)

// parseOffset is _parse_offset_str (tzcanonical.py:207-232) of a stripped
// name, in seconds. Go \d is ASCII only; Python \d takes any decimal digit.
func parseOffset(name string) (int64, bool) {
	name = strings.ToUpper(name)
	if name == "Z" || name == "UTC" || name == "GMT" {
		return 0, true
	}
	m := offsetString.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	// exactly one of the two hour groups is set
	h, _ := strconv.Atoi(m[2] + m[3])
	mi, _ := strconv.Atoi(m[4])
	offset := int64(h*3600 + mi*60)
	if m[1] == "-" {
		offset = -offset
	}
	return offset, true
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
