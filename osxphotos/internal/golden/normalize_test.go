package golden

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// The escape is built, not written, so that the source has no such escape that
// an editor or a tool might turn into a character.
func uesc(units ...string) string {
	var s string
	for _, u := range units {
		s += string(rune(92)) + "u" + u
	}
	return s
}

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
		"2020-06-06T00:00:00.+00:00", "2020-06-06T00:00:00+0500",
		"2020-06-06T00:00:00+05", "2020-06-06T00:00:00+05:00x", "2020-06-06T00:00:00+00:00 ", " 2020-06-06T00:00:00+00:00",
		"2020-06-06T00:00:00", "20200606T000000+00:00", "2020-06-06t00:00:00+00:00", "2020-06-06T0a:00:00+00:00",
		"+2020-06-06T00:00:00+00:00", "2020-6-06T00:00:00+00:00", "2020-06-06T00:00:00+00:00:00:00",
	} {
		if got, ok := parseMoment(in); ok {
			t.Errorf("parseMoment(%q) = %v, want a failure", in, momentString(got))
		}
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, raw string
		more           []string
		want           string
		wantErr        string
	}{
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
		{name: "string with a quote", key: "title", raw: `"x\"y"`, want: `"x\"y"`},
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

		{name: "favorite", key: "favorite", raw: `true`, want: `true`},
		{name: "favorite null", key: "favorite", raw: `null`, wantErr: "favorite: want a boolean"},
		{name: "favorite string", key: "favorite", raw: `"SECRET-F"`, wantErr: "favorite: want a boolean"},
		{name: "favorite number", key: "favorite", raw: `1`, wantErr: "favorite: want a boolean"},
		{name: "isphoto null", key: "isphoto", raw: `null`, wantErr: "isphoto: want a boolean"},
		{name: "burst null", key: "burst", raw: `null`, wantErr: "burst: want a boolean"},
		{name: "shared_library null", key: "shared_library", raw: `null`, wantErr: "want a boolean"},

		{name: "width", key: "width", raw: `3024`, want: `3024`},
		{name: "width as a float", key: "width", raw: `3024.0`, wantErr: "width: want an integer"},
		{name: "width in exponent form", key: "width", raw: `3e3`, wantErr: "width: want an integer"},
		{name: "width as a string", key: "width", raw: `"SECRET-W"`, wantErr: "width: want an integer"},
		{name: "width above 2^53", key: "original_filesize", raw: `9007199254740993`, want: `9007199254740993`},
		{name: "width above int64", key: "original_filesize", raw: `9223372036854775808`, wantErr: "want an integer"},
		{name: "width below int64", key: "original_filesize", raw: `-9223372036854775809`, wantErr: "want an integer"},
		{name: "width negative", key: "width", raw: `-1`, want: `-1`},
		{name: "width true", key: "width", raw: `true`, wantErr: "want an integer"},

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
		{name: "location with a string", key: "location", raw: `[1, "x"]`, wantErr: "location: want a list of two numbers or nulls"},

		{name: "keywords", key: "keywords", raw: `["b", "a", "a"]`, want: `["b","a","a"]`},
		{name: "keywords with null", key: "keywords", raw: `[null, "a"]`, want: `["","a"]`},
		{name: "keywords null", key: "keywords", raw: `null`, wantErr: "keywords: want a list of strings"},
		{name: "keywords with a number", key: "keywords", raw: `["a", 1]`, wantErr: "keywords: want a list of strings"},
		{name: "keywords with HTML characters", key: "keywords", raw: `["<&>", "x\"y"]`, want: `["<&>","x\"y"]`},
		{name: "burst_photos", key: "burst_photos", raw: `["b", "a"]`, want: `["b","a"]`},
		{name: "persons unknown", key: "persons", raw: `["_UNKNOWN_"]`, want: `false`},
		{name: "persons unknown and a name", key: "persons", raw: `["_UNKNOWN_", "A"]`, want: `true`},
		{name: "persons empty", key: "persons", raw: `[]`, want: `false`},
		{name: "persons null item", key: "persons", raw: `[null]`, want: `false`},
		{name: "persons empty name", key: "persons", raw: `[""]`, want: `false`},
		{name: "persons name", key: "persons", raw: `["A"]`, want: `true`},
		{name: "persons null", key: "persons", raw: `null`, wantErr: "persons: want a list of strings"},

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

func TestCanonicalRefusesNaN(t *testing.T) {
	t.Parallel()
	if _, err := canonical(math.NaN()); !errors.Is(err, errUnsupported) {
		t.Errorf("error = %v, want errUnsupported", err)
	}
	if _, err := canonical(math.Inf(1)); err == nil {
		t.Error("no error for infinity")
	}
}
