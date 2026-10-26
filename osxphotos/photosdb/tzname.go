package photosdb

import (
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // upstream reads the zoneinfo of the system or of the tzdata package
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
// the fixed zone of offset. An offset string equal to offset gives that fixed
// zone upstream (tzcanonical.py:412-415), and the zones upstream searches for
// any other name have offset (tzcanonical.py:426-446), so neither is ported.
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

// time.LoadLocation returns a new Location on every call, and every Date keeps
// its own; upstream keeps one ZoneInfo per name. Only names that load are kept,
// so a name that is not a zone costs no memory.
var zones sync.Map

// zoneinfo.ZoneInfo has no "" and no "Local", which time.LoadLocation reads
// as UTC and the zone of the computer.
func loadZone(name string) *time.Location {
	if name == "" || name == "Local" {
		return nil
	}
	if v, ok := zones.Load(name); ok {
		return v.(*time.Location)
	}
	z, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	zones.Store(name, z)
	return z
}
