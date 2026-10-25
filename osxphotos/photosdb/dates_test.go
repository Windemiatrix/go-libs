package photosdb

import (
	"database/sql"
	"encoding/json"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestCoreDataTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ts   float64
		want time.Time
	}{
		{"Core Data epoch", 0, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"Unix epoch", -978307200, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"whole seconds", 694224000, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"milliseconds", 694224000.25, time.Date(2023, 1, 1, 0, 0, 0, 250_000_000, time.UTC)},
		// 1/128 s is 7812.5 µs: half to even gives 7812, math.Round 7813.
		{"tie to even, down", -978307199.9921875, time.Date(1970, 1, 1, 0, 0, 0, 7_812_000, time.UTC)},
		// 3/128 s is 23437.5 µs: half to even gives 23438, truncation 23437.
		{"tie to even, up", -978307199.9765625, time.Date(1970, 1, 1, 0, 0, 0, 23_438_000, time.UTC)},
		// A fraction of -1/128 s: -7812 µs, carried into the second before.
		{"negative fraction", -978307200.0078125, time.Date(1969, 12, 31, 23, 59, 59, 992_188_000, time.UTC)},
		{"first second of year 1", -63113904000, time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"last second of year 9999", 252423993599, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)},
	}
	for _, tt := range tests {
		got, ok := coreDataTime(tt.ts)
		if !ok || !got.Equal(tt.want) || got.Location() != time.UTC {
			t.Errorf("%s: coreDataTime(%v) = %v, %v; want %v, true (UTC)", tt.name, tt.ts, got, ok, tt.want)
		}
	}

	for _, ts := range []float64{252423993600, -63113904001, math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64, -math.MaxFloat64} {
		if got, ok := coreDataTime(ts); ok {
			t.Errorf("coreDataTime(%v) = %v, true; want ok false", ts, got)
		}
	}
}

// oracleMicros is the exact Unix time in microseconds of f (the float64 sum
// ts + 978307200) rounded half to even, computed with math/big.
func oracleMicros(f float64) int64 {
	r := new(big.Rat).SetFloat64(f)
	r.Mul(r, big.NewRat(1_000_000, 1))
	q, m := new(big.Int).DivMod(r.Num(), r.Denom(), new(big.Int)) // floor, 0 <= m < denom
	twice := new(big.Int).Mul(m, big.NewInt(2))
	switch c := twice.Cmp(r.Denom()); {
	case c > 0, c == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

// randomTimestamps returns timestamps of about ±34 years around 2001, with
// binary and with decimal fractions. They are made from integers by one
// division, never by x*y + z.
func randomTimestamps(n int) []float64 {
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: deterministic test data
	out := make([]float64, 0, n)
	for i := range n {
		if i%2 == 0 {
			out = append(out, float64(r.Int64N(1<<41)-1<<40)/1024)
		} else {
			out = append(out, float64(r.Int64N(2_000_000_000_000)-1_000_000_000_000)/1000)
		}
	}
	return out
}

func TestCoreDataTimeOracle(t *testing.T) {
	t.Parallel()

	ts := randomTimestamps(20000)
	for _, v := range ts {
		got, ok := coreDataTime(v)
		if !ok {
			t.Fatalf("coreDataTime(%v): ok false", v)
		}
		f := float64(v + coreDataOffset)
		if want := oracleMicros(f); got.UnixMicro() != want || got.Nanosecond()%1000 != 0 {
			t.Fatalf("coreDataTime(%v) = %v (%d µs), want %d µs", v, got, got.UnixMicro(), want)
		}
		// The result is within half a microsecond of f.
		exact := new(big.Rat).SetFloat64(f)
		diff := new(big.Rat).Sub(new(big.Rat).SetFrac64(got.UnixNano(), 1_000_000_000), exact)
		if diff.Abs(diff).Cmp(big.NewRat(1, 2_000_000)) > 0 {
			t.Fatalf("coreDataTime(%v) = %v is more than 500 ns from %v", v, got, f)
		}
	}

	slices.Sort(ts)
	prev, _ := coreDataTime(ts[0])
	for _, v := range ts[1:] {
		cur, _ := coreDataTime(v)
		if cur.Before(prev) {
			t.Fatalf("coreDataTime is not monotonic at %v: %v before %v", v, cur, prev)
		}
		prev = cur
	}
}

func FuzzCoreDataTime(f *testing.F) {
	for _, v := range []float64{0, -978307200, -978307199.9921875, -978307200.0078125, 694224000.5,
		252423993599, 252423993600, -63113904000, math.NaN(), math.Inf(1), 1e300, -1e300, 5e-324} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, ts float64) {
		got, ok := coreDataTime(ts)
		if !ok {
			return
		}
		if got.Location() != time.UTC || got.Nanosecond()%1000 != 0 {
			t.Errorf("coreDataTime(%v) = %v: not UTC or not whole microseconds", ts, got)
		}
		if y := got.Year(); y < 1 || y > 9999 {
			t.Errorf("coreDataTime(%v) = %v: year %d", ts, got, y)
		}
	})
}

func nullFloat(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }
func nullInt(v int64) sql.NullInt64       { return sql.NullInt64{Int64: v, Valid: true} }

func TestAssetDate(t *testing.T) {
	t.Parallel()

	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	moment := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		created sql.NullFloat64
		tz      sql.NullInt64
		want    time.Time
		offset  int
	}{
		{"no offset", nullFloat(694224000), sql.NullInt64{}, moment, 0},
		{"offset 0", nullFloat(694224000), nullInt(0), moment, 0},
		{"offset east", nullFloat(694224000), nullInt(10800), moment, 10800},
		{"offset west", nullFloat(694224000), nullInt(-18000), moment, -18000},
		{"largest offset", nullFloat(694224000), nullInt(86399), moment, 86399},
		{"smallest offset", nullFloat(694224000), nullInt(-86399), moment, -86399},
		{"offset of a day", nullFloat(694224000), nullInt(86400), epoch, 0},
		{"offset of minus a day", nullFloat(694224000), nullInt(-86400), epoch, 0},
		{"max int64 offset", nullFloat(694224000), nullInt(math.MaxInt64), epoch, 0},
		{"min int64 offset", nullFloat(694224000), nullInt(math.MinInt64), epoch, 0},
		{"no date", sql.NullFloat64{}, nullInt(3600), epoch, 0},
		{"date out of range", nullFloat(1e300), sql.NullInt64{}, epoch, 0},
		{"NaN", nullFloat(math.NaN()), nullInt(3600), epoch, 0},
	}
	for _, tt := range tests {
		got := assetDate(tt.created, tt.tz)
		_, off := got.Zone()
		if !got.Equal(tt.want) || off != tt.offset {
			t.Errorf("%s: assetDate = %v (offset %d), want %v (offset %d)", tt.name, got, off, tt.want, tt.offset)
		}
		if tt.offset == 0 && got.Location() != time.UTC {
			t.Errorf("%s: assetDate location %v, want UTC", tt.name, got.Location())
		}
	}
}

func TestOptionalTime(t *testing.T) {
	t.Parallel()

	if got := optionalTime(sql.NullFloat64{}); got != nil {
		t.Errorf("optionalTime(NULL) = %v, want nil", got)
	}
	if got := optionalTime(nullFloat(math.Inf(1))); got != nil {
		t.Errorf("optionalTime(+Inf) = %v, want nil", got)
	}
	// A negative ZMODIFICATIONDATE is a moment, not nil (photosdb.py:2153-2155).
	for _, tt := range []struct {
		ts   float64
		want time.Time
	}{
		{694224000, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{-31622400, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}, // 2000 has 366 days
	} {
		got := optionalTime(nullFloat(tt.ts))
		if got == nil || !got.Equal(tt.want) || got.Location() != time.UTC {
			t.Errorf("optionalTime(%v) = %v, want %v UTC", tt.ts, got, tt.want)
		}
	}
}

// goldenDir is testdata/golden of the repository. A missing fixture fails
// the test: a skip would let TestCoreDataTimeGolden pass without checking.
func goldenDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the package source")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "golden")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata/golden is not available: %v", err)
	}
	return dir
}

// TestCoreDataTimeGolden converts ZADDEDDATE of three assets of Test-13.0.0
// and compares the moments with date_added of the osxphotos output.
func TestCoreDataTimeGolden(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(goldenDir(t), "Test-13.0.0", "query.json"))
	if err != nil {
		t.Fatal(err)
	}
	var objects []struct {
		UUID      string `json:"uuid"`
		DateAdded string `json:"date_added"`
	}
	if err := json.Unmarshal(data, &objects); err != nil {
		t.Fatal(err)
	}
	db := roDB(t, testlib.OpenPristine(t, "Test-13.0.0"))
	checked := 0
	for _, o := range objects {
		var added sql.NullFloat64
		if err := db.QueryRowContext(t.Context(), "SELECT ZADDEDDATE FROM ZASSET WHERE ZUUID = ?", o.UUID).Scan(&added); err != nil {
			t.Fatal(err)
		}
		// A fraction is needed to tell the rounding rules apart.
		if !added.Valid || added.Float64 == math.Trunc(added.Float64) {
			continue
		}
		want, err := time.Parse(time.RFC3339Nano, o.DateAdded)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := coreDataTime(added.Float64)
		if !ok || !got.Equal(want) {
			t.Errorf("coreDataTime(ZADDEDDATE %v) = %v, %v; osxphotos date_added %v", added.Float64, got, ok, want)
		}
		if checked++; checked == 3 {
			break
		}
	}
	if checked < 3 {
		t.Fatalf("found %d assets with a fractional ZADDEDDATE, want 3", checked)
	}
}

// roDB opens database/Photos.sqlite of the library read-only.
func roDB(t *testing.T, lib string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(lib, "database", "Photos.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
