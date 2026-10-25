package photosdb

import (
	"database/sql"
	"math"
	"time"
)

// coreDataOffset is 2001-01-01T00:00:00Z in Unix seconds, TIME_DELTA of
// photos_datetime.py:13-15.
const coreDataOffset = 978307200

// The Unix seconds of the first and the last second of the years 1-9999,
// the range of Python datetime.
const (
	minUnixSecond = -62135596800
	maxUnixSecond = 253402300799
)

// maxOffset bounds a time zone offset: datetime.timezone accepts only
// -86400 < offset < 86400 seconds (photos_datetime.py:64-67).
const maxOffset = 86400

// epoch is DEFAULT_DATETIME of osxphotos (photos_datetime.py:18).
var epoch = time.Unix(0, 0).UTC()

// coreDataTime follows Python datetime.fromtimestamp (photos_datetime.py:52,
// 82): float64 arithmetic, the fraction rounded half to even to microseconds.
// ok is false where Python raises: NaN, ±Inf and a result outside the years
// 1-9999 (photos_datetime.py:66-67, 88-89).
func coreDataTime(ts float64) (t time.Time, ok bool) {
	if math.IsNaN(ts) || math.IsInf(ts, 0) {
		return time.Time{}, false
	}
	sec, frac := math.Modf(ts + coreDataOffset)
	us := math.RoundToEven(frac * 1e6)
	switch {
	case us >= 1e6:
		sec, us = sec+1, us-1e6
	case us < 0:
		sec, us = sec-1, us+1e6
	}
	if sec < minUnixSecond || sec > maxUnixSecond {
		return time.Time{}, false
	}
	return time.Unix(int64(sec), int64(us)*1000).UTC(), true
}

// assetDate follows photos_datetime with default=True (photos_datetime.py:47-67,
// photosdb.py:2159-2167). ZTIMEZONENAME is not used.
func assetDate(created sql.NullFloat64, tz sql.NullInt64) time.Time {
	if !created.Valid {
		return epoch
	}
	t, ok := coreDataTime(created.Float64)
	if !ok {
		return epoch
	}
	if !tz.Valid || tz.Int64 == 0 {
		return t
	}
	// Compared as int64 before any conversion: the column may hold any integer.
	if tz.Int64 <= -maxOffset || tz.Int64 >= maxOffset {
		return epoch
	}
	return t.In(time.FixedZone("", int(tz.Int64))) //nolint:gosec // G115: |tz| < 86400 is checked above
}

func optionalTime(v sql.NullFloat64) *time.Time {
	if !v.Valid {
		return nil
	}
	t, ok := coreDataTime(v.Float64)
	if !ok {
		return nil
	}
	return &t
}
