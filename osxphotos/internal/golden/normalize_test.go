package golden

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// uesc writes JSON escapes of UTF-16 units: uesc("d83d", "de00") is the text
// backslash-u-d83d backslash-u-de00. It is built, not written, so that the
// source has no such escape that an editor or a tool might turn into a character.
func uesc(units ...string) string {
	var s string
	for _, u := range units {
		s += string(rune(92)) + "u" + u
	}
	return s
}

// refCanon is the canonical form of the reference value raw of the rule key.
func refCanon(key string, raw string, more ...string) (string, error) {
	o := Object{key: []byte(raw)}
	for i := 0; i+1 < len(more); i += 2 {
		o[more[i]] = []byte(more[i+1])
	}
	v, err := rules[key].ref(o)
	if err != nil {
		return "", err
	}
	return canonical(v)
}

// goCanon is the canonical form of the Go value of the rule key.
func goCanon(t *testing.T, key string, p *photoinfo.PhotoInfo) string {
	t.Helper()
	v, err := rules[key].got(&env{}, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := canonical(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseMoment(t *testing.T) {
	t.Parallel()
	valid := []struct{ in, want string }{
		{"2020-06-06T14:15:24.725564+00:00", "2020-06-06T14:15:24.725564Z"},
		{"2020-06-06T14:15:24+00:00", "2020-06-06T14:15:24Z"},
		{"2020-06-06T19:45:24.725564+05:30", "2020-06-06T14:15:24.725564Z"},
		{"1970-01-01T05:45:00+05:45", "1970-01-01T00:00:00Z"},
		{"2020-06-06T07:15:24.000001-07:00", "2020-06-06T14:15:24.000001Z"},
		{"2020-06-06T14:15:24.725564-00:00", "2020-06-06T14:15:24.725564Z"},
		{"2000-01-01T01:53:28+01:53:28", "2000-01-01T00:00:00Z"}, // an offset with seconds, which time.RFC3339 does not read
		{"2000-01-01T00:00:00-01:53:28", "2000-01-01T01:53:28Z"},
		{"2020-06-06T14:15:24.5+00:00", "2020-06-06T14:15:24.5Z"},
		{"2020-06-06T14:15:24.120000+00:00", "2020-06-06T14:15:24.12Z"},
		{"2020-06-06T14:15:24.725564123+00:00", "2020-06-06T14:15:24.725564123Z"},
		{"2020-06-06T14:15:24Z", "2020-06-06T14:15:24Z"},
		{"0001-01-01T00:00:00+00:00", "0001-01-01T00:00:00Z"},
		{"9999-12-31T23:59:59.999999+00:00", "9999-12-31T23:59:59.999999Z"},
		{"2020-02-29T00:00:00+00:00", "2020-02-29T00:00:00Z"},
		{"2001-01-01T00:00:00+00:00", "2001-01-01T00:00:00Z"},
	}
	for _, c := range valid {
		got, ok := parseMoment(c.in)
		if !ok || momentString(got) != c.want {
			t.Errorf("parseMoment(%q) = %v, %v; want %s", c.in, momentString(got), ok, c.want)
		}
	}
	for _, in := range []string{
		"", "2020", "2020-06-06", "2020-06-06T14:15:24", "2020-06-06 14:15:24+00:00", "2020-06-06T14:15+00:00",
		"2020-13-01T00:00:00+00:00", "2020-00-01T00:00:00+00:00", "2020-06-00T00:00:00+00:00", "2020-02-30T00:00:00+00:00",
		"2021-02-29T00:00:00+00:00", "2020-06-06T24:00:00+00:00", "2020-06-06T00:60:00+00:00", "2020-06-06T00:00:60+00:00",
		"0000-01-01T00:00:00+00:00", "2020-06-06T00:00:00.+00:00", "2020-06-06T00:00:00.1234567891+00:00",
		"2020-06-06T00:00:00+24:00", "2020-06-06T00:00:00+05:60", "2020-06-06T00:00:00+05:00:60", "2020-06-06T00:00:00+0500",
		"2020-06-06T00:00:00+05", "2020-06-06T00:00:00+05:00x", "2020-06-06T00:00:00+00:00 ", " 2020-06-06T00:00:00+00:00",
		"2020-06-06T00:00:00", "20200606T000000+00:00", "2020-06-06t00:00:00+00:00", "2020-06-06T0a:00:00+00:00",
		"+2020-06-06T00:00:00+00:00", "2020-6-06T00:00:00+00:00", "2020-06-06T00:00:00+00:00:00:00",
	} {
		if got, ok := parseMoment(in); ok {
			t.Errorf("parseMoment(%q) = %v, want a failure", in, momentString(got))
		}
	}
}

// TestNormalize pins the forms of values that the rules accept and refuse.
func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, raw string
		more           []string
		want           string // canonical form
		wantErr        string // part of the error
	}{
		// Dates are moments; the offset of the string is not compared.
		{name: "date", key: "date", raw: `"2020-06-06T14:15:24.725564+00:00"`, want: `"2020-06-06T14:15:24.725564Z"`},
		{name: "date in another offset", key: "date", raw: `"2020-06-06T19:45:24.725564+05:30"`, want: `"2020-06-06T14:15:24.725564Z"`},
		{name: "date in -07:00", key: "date", raw: `"2020-06-06T07:15:24.725564-07:00"`, want: `"2020-06-06T14:15:24.725564Z"`},
		{name: "date with an offset in seconds", key: "date", raw: `"2020-06-06T15:49:00+01:33:36"`, want: `"2020-06-06T14:15:24Z"`},
		{name: "date null", key: "date", raw: `null`, wantErr: "date: want an ISO 8601 string"},
		{name: "date number", key: "date", raw: `5`, wantErr: "date: want an ISO 8601 string"},
		{name: "date nonsense", key: "date", raw: `"SECRET-DATE"`, wantErr: "date: want an ISO 8601 string"},
		{name: "date_added at the epoch", key: "date_added", raw: `"1970-01-01T00:00:00+00:00"`, want: `"1970-01-01T00:00:00Z"`},
		{name: "date_added null", key: "date_added", raw: `null`, want: `null`},
		{name: "date_modified null", key: "date_modified", raw: `null`, want: `null`},
		{name: "date_trashed", key: "date_trashed", raw: `"2120-06-10T16:24:47.685857+00:00"`, want: `"2120-06-10T16:24:47.685857Z"`},

		// null ↔ the zero value, where the table says so.
		{name: "title null", key: "title", raw: `null`, want: `""`},
		{name: "title", key: "title", raw: `"A"`, want: `"A"`},
		{name: "title number", key: "title", raw: `5`, wantErr: "title: want a string or null"},
		{name: "description null", key: "description", raw: `null`, want: `""`},
		{name: "cloud_guid null", key: "cloud_guid", raw: `null`, want: `""`},
		{name: "fingerprint null", key: "fingerprint", raw: `null`, want: `""`},
		{name: "uti_raw null", key: "uti_raw", raw: `null`, want: `""`},
		{name: "filename null", key: "filename", raw: `null`, wantErr: "filename: want a string"},
		{name: "original_filename null", key: "original_filename", raw: `null`, wantErr: "want a string"},
		{name: "uti null", key: "uti", raw: `null`, wantErr: "want a string"},
		{name: "uti_original null", key: "uti_original", raw: `null`, wantErr: "want a string"},
		{name: "string in NFD is kept", key: "title", raw: `"é"`, want: "\"é\""},
		{name: "string outside the BMP", key: "title", raw: `"😀"`, want: "\"\U0001F600\""},
		{name: "NFD from an escape is kept", key: "title", raw: `"e` + uesc("0301") + `"`, want: "\"e" + string(rune(0x301)) + "\""},
		{name: "NFC from an escape", key: "title", raw: `"` + uesc("00e9") + `"`, want: "\"" + string(rune(0xe9)) + "\""},
		{name: "a pair of surrogates", key: "title", raw: `"` + uesc("d83d", "de00") + `"`, want: "\"" + string(rune(0x1F600)) + "\""},
		{name: "U+2028 is written as an escape", key: "title", raw: `"a` + uesc("2028") + `"`, want: `"a` + uesc("2028") + `"`},
		{name: "string with HTML characters", key: "title", raw: `"<a & b>"`, want: `"<a & b>"`},
		{name: "width null", key: "width", raw: `null`, want: `0`},
		{name: "height null", key: "height", raw: `null`, want: `0`},
		{name: "original_width null", key: "original_width", raw: `null`, want: `0`},
		{name: "original_height null", key: "original_height", raw: `null`, want: `0`},
		{name: "original_orientation null", key: "original_orientation", raw: `null`, want: `0`},
		{name: "original_filesize null", key: "original_filesize", raw: `null`, want: `0`},
		{name: "orientation null", key: "orientation", raw: `null`, want: `0`},
		{name: "tzoffset null", key: "tzoffset", raw: `null`, want: `null`},
		{name: "tzoffset", key: "tzoffset", raw: `-25200`, want: `-25200`},
		{name: "tzoffset 0 is not null", key: "tzoffset", raw: `0`, want: `0`},
		{name: "incloud null", key: "incloud", raw: `null`, want: `null`},
		{name: "incloud false", key: "incloud", raw: `false`, want: `false`},
		{name: "incloud string", key: "incloud", raw: `"true"`, wantErr: "incloud: want a boolean or null"},
		{name: "latitude null", key: "latitude", raw: `null`, want: `null`},
		{name: "longitude null", key: "longitude", raw: `null`, want: `null`},

		// Booleans: null is a mistake of the form.
		{name: "favorite", key: "favorite", raw: `true`, want: `true`},
		{name: "favorite null", key: "favorite", raw: `null`, wantErr: "favorite: want a boolean"},
		{name: "favorite string", key: "favorite", raw: `"SECRET-F"`, wantErr: "favorite: want a boolean"},
		{name: "favorite number", key: "favorite", raw: `1`, wantErr: "favorite: want a boolean"},
		{name: "isphoto null", key: "isphoto", raw: `null`, wantErr: "isphoto: want a boolean"},
		{name: "burst null", key: "burst", raw: `null`, wantErr: "burst: want a boolean"},
		{name: "shared_library null", key: "shared_library", raw: `null`, wantErr: "want a boolean"},

		// Integers are exact.
		{name: "width", key: "width", raw: `3024`, want: `3024`},
		{name: "width as a float", key: "width", raw: `3024.0`, wantErr: "width: want an integer"},
		{name: "width in exponent form", key: "width", raw: `3e3`, wantErr: "width: want an integer"},
		{name: "width as a string", key: "width", raw: `"SECRET-W"`, wantErr: "width: want an integer"},
		{name: "width above 2^53", key: "original_filesize", raw: `9007199254740993`, want: `9007199254740993`},
		{name: "width above int64", key: "original_filesize", raw: `9223372036854775808`, wantErr: "want an integer"},
		{name: "width below int64", key: "original_filesize", raw: `-9223372036854775809`, wantErr: "want an integer"},
		{name: "width negative", key: "width", raw: `-1`, want: `-1`},
		{name: "width true", key: "width", raw: `true`, wantErr: "want an integer"},

		// Floats are exact doubles.
		{name: "latitude", key: "latitude", raw: `48.8566`, want: `48.8566`},
		{name: "latitude 0", key: "latitude", raw: `0`, want: `0`},
		{name: "latitude 0.0", key: "latitude", raw: `0.0`, want: `0`},
		{name: "latitude -0.0", key: "latitude", raw: `-0.0`, want: `-0`},
		{name: "latitude 1e-05", key: "latitude", raw: `1e-05`, want: `0.00001`},
		{name: "latitude 5e-324", key: "latitude", raw: `5e-324`, want: `5e-324`},
		{name: "latitude 17 digits", key: "latitude", raw: `0.12345678901234568`, want: `0.12345678901234568`},
		{name: "latitude 17 digits that round", key: "latitude", raw: `0.1234567890123456789`, want: `0.12345678901234568`},
		{name: "latitude 1e21", key: "latitude", raw: `1e21`, want: `1e+21`},
		{name: "latitude out of range", key: "latitude", raw: `1e999`, wantErr: "latitude: want a number or null"},
		{name: "latitude string", key: "latitude", raw: `"SECRET-L"`, wantErr: "latitude: want a number or null"},
		{name: "location", key: "location", raw: `[48.8566, null]`, want: `[48.8566,null]`},
		{name: "location nulls", key: "location", raw: `[null, null]`, want: `[null,null]`},
		{name: "location of one", key: "location", raw: `[1]`, wantErr: "location: want a list of two"},
		{name: "location null", key: "location", raw: `null`, wantErr: "location: want a list of two"},
		{name: "location with a string", key: "location", raw: `[1, "x"]`, wantErr: "location[1]: want a number or null"},

		// Lists.
		{name: "keywords", key: "keywords", raw: `["b", "a", "a"]`, want: `["b","a","a"]`}, // a list, in the order of the reference, repeats kept
		{name: "keywords with null", key: "keywords", raw: `[null, "a"]`, want: `["","a"]`},
		{name: "keywords null", key: "keywords", raw: `null`, wantErr: "keywords: want a list of strings"},
		{name: "keywords with a number", key: "keywords", raw: `["a", 1]`, wantErr: "keywords[1]: want a string or null"},
		{name: "burst_photos", key: "burst_photos", raw: `["b", "a"]`, want: `["b","a"]`},
		{name: "persons unknown", key: "persons", raw: `["_UNKNOWN_"]`, want: `false`},
		{name: "persons unknown and a name", key: "persons", raw: `["_UNKNOWN_", "A"]`, want: `true`},
		{name: "persons empty", key: "persons", raw: `[]`, want: `false`},
		{name: "persons null item", key: "persons", raw: `[null]`, want: `false`},
		{name: "persons empty name", key: "persons", raw: `[""]`, want: `false`},
		{name: "persons name", key: "persons", raw: `["A"]`, want: `true`},
		{name: "persons null", key: "persons", raw: `null`, wantErr: "persons: want a list of strings"},

		// A key that is missing is a mistake of the input, not null.
		{name: "no key", key: "title", raw: ``, wantErr: "title: want a value (the key is missing)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var got string
			var err error
			if c.raw == "" {
				v, e := rules[c.key].ref(Object{})
				err = e
				if e == nil {
					got, err = canonical(v)
				}
			} else {
				got, err = refCanon(c.key, c.raw, c.more...)
			}
			if c.wantErr != "" {
				var se *shapeError
				if err == nil || !errors.As(err, &se) || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error = %v, want a shape error with %q", err, c.wantErr)
				}
				if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), c.raw) && len(c.raw) > 6 {
					t.Errorf("the error repeats the value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("canonical = %s, want %s", got, c.want)
			}
		})
	}
}

// TestNormalizeGoSide: how the Go side is written.
func TestNormalizeGoSide(t *testing.T) {
	t.Parallel()
	ist := time.FixedZone("IST", 19800)
	when := time.Date(2020, 6, 6, 19, 45, 24, 725564000, ist)
	t.Run("a moment in any zone", func(t *testing.T) {
		t.Parallel()
		for _, loc := range []*time.Location{time.UTC, ist, time.FixedZone("W", -7*3600), time.FixedZone("S", 5640+28)} {
			p := &photoinfo.PhotoInfo{Date: when.In(loc)}
			if got := goCanon(t, "date", p); got != `"2020-06-06T14:15:24.725564Z"` {
				t.Errorf("zone %v: %s", loc, got)
			}
		}
	})
	t.Run("nanoseconds are not rounded", func(t *testing.T) {
		t.Parallel()
		p := &photoinfo.PhotoInfo{Date: when.Add(time.Nanosecond)}
		if got := goCanon(t, "date", p); got != `"2020-06-06T14:15:24.725564001Z"` {
			t.Errorf("got %s", got)
		}
	})
	t.Run("defaults of nil", func(t *testing.T) {
		t.Parallel()
		p := &photoinfo.PhotoInfo{}
		want := map[string]string{
			"date_added": `"1970-01-01T00:00:00Z"`, "date_modified": `null`, "date_trashed": `null`,
			"tzoffset": `null`, "incloud": `null`, "latitude": `null`, "longitude": `null`,
			"location": `[null,null]`, "keywords": `[]`, "burst_photos": `[]`, "title": `""`, "width": `0`,
			"persons": `false`, "burst_default_pick": `false`,
		}
		for key, w := range want {
			if got := goCanon(t, key, p); got != w {
				t.Errorf("%s: %s, want %s", key, got, w)
			}
		}
	})
	t.Run("the pick type bit of the default pick", func(t *testing.T) {
		t.Parallel()
		for pick, want := range map[int]string{0: "false", 2: "false", 4: "true", 12: "true", 16: "false", 20: "true"} {
			if got := goCanon(t, "burst_default_pick", &photoinfo.PhotoInfo{BurstPickType: pick}); got != want {
				t.Errorf("pick %d: %s, want %s", pick, got, want)
			}
		}
	})
	t.Run("the list of the Go side is not sorted", func(t *testing.T) {
		t.Parallel()
		p := &photoinfo.PhotoInfo{Keywords: []string{"b", "a"}}
		if got := goCanon(t, "keywords", p); got != `["b","a"]` {
			t.Errorf("got %s", got)
		}
		if p.Keywords[0] != "b" {
			t.Error("the list of the photo was sorted in place")
		}
	})
}

// TestOrientationSkip: the rule that skips the key for an adjusted photo.
func TestOrientationSkip(t *testing.T) {
	t.Parallel()
	skip := rules["orientation"].skip
	for raw, want := range map[string]bool{"true": true, "false": false} {
		got, err := skip(Object{"hasadjustments": []byte(raw)})
		if err != nil || got != want {
			t.Errorf("hasadjustments %s: skip = %v, %v; want %v", raw, got, err, want)
		}
	}
	if _, err := skip(Object{"hasadjustments": []byte("null")}); err == nil {
		t.Error("no error for hasadjustments null")
	}
	if _, err := skip(Object{}); err == nil {
		t.Error("no error without hasadjustments")
	}
}

// pyIsoformat writes a moment as Python datetime.isoformat does.
func pyIsoformat(local time.Time, offset int) string {
	s := local.Format("2006-01-02T15:04:05")
	if us := local.Nanosecond() / 1000; us != 0 {
		s += fmt.Sprintf(".%06d", us)
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	s += fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
	if offset%60 != 0 {
		s += fmt.Sprintf(":%02d", offset%60)
	}
	return s
}

// TestMomentRoundTrip: parse(format(t)) is t for random moments, any year of
// Python and any offset it allows, and the canonical form reads back as itself.
func TestMomentRoundTrip(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(11, 13))
	for i := 0; i < 10000; i++ {
		year := 1 + r.IntN(9999)
		naive := time.Date(year, time.Month(1+r.IntN(12)), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1_000_000)*1000, time.UTC)
		if r.IntN(5) == 0 {
			naive = naive.Truncate(time.Second) // isoformat leaves out a zero fraction
		}
		offset := r.IntN(2*86400-1) - (86400 - 1) // -23:59:59 .. +23:59:59
		text := pyIsoformat(naive, offset)
		got, ok := parseMoment(text)
		want := naive.Add(-time.Duration(offset) * time.Second)
		if !ok || !got.Equal(want) {
			t.Fatalf("parseMoment(%q) = %v, %v; want %v", text, got, ok, want)
		}
		canon := momentString(got)
		if y := got.Year(); y < 1 || y > 9999 {
			continue // the canonical form of a year out of Python's range is not read back
		}
		again, ok := parseMoment(canon)
		if !ok || !again.Equal(got) || momentString(again) != canon {
			t.Fatalf("the canonical form %q of %q does not read back", canon, text)
		}
	}
}

// FuzzParseMoment: no panic, and a date that is read is read the same way
// from its canonical form.
func FuzzParseMoment(f *testing.F) {
	for _, s := range []string{"2020-06-06T14:15:24.725564+00:00", "2000-01-01T01:53:28+01:53:28", "", "Z", "9999-12-31T23:59:59.999999-23:59:59",
		"0001-01-01T00:00:00+23:59", "2020-02-30T00:00:00+00:00", "2020-06-06T14:15:24.1234567890Z"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, ok := parseMoment(in)
		if !ok {
			return
		}
		if y := got.UTC().Year(); y < 1 || y > 9999 {
			return
		}
		again, ok := parseMoment(momentString(got))
		if !ok || !again.Equal(got) {
			t.Errorf("%q: the canonical form does not read back", in)
		}
	})
}

// FuzzRefValues: no value of the reference makes a rule panic, and an error
// does not repeat the value.
func FuzzRefValues(f *testing.F) {
	for _, s := range []string{`"SECRET-VALUE"`, `null`, `5`, `3024.0`, `[1, 2]`, `{"a": "SECRET-VALUE"}`, `true`, `["SECRET-VALUE", null]`, `1e999`, `"\ud800"`} {
		f.Add(s)
	}
	keys := allKeys()
	f.Fuzz(func(t *testing.T, raw string) {
		for _, key := range keys {
			o := Object{}
			for _, k := range keys {
				o[k] = []byte(raw)
			}
			_, err := rules[key].ref(o)
			if err == nil || len(raw) < 8 || !utf8.ValidString(raw) {
				continue
			}
			if strings.Contains(err.Error(), raw) {
				t.Errorf("%s: the error repeats the value", key)
			}
		}
	})
}

func TestCanonicalRefusesNaN(t *testing.T) {
	t.Parallel()
	if _, err := canonical(math.NaN()); !errors.Is(err, errUnsupported) {
		t.Errorf("error = %v, want errUnsupported", err)
	}
	if _, err := canonical(math.Inf(1)); err == nil {
		t.Error("no error for infinity")
	}
}
