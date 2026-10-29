package golden

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
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

const zeroExif = `{"aperture":null,"bit_rate":null,"camera_make":"","camera_model":"","codec":"","date":null,"duration":null,` +
	`"exposure_bias":null,"flash_fired":null,"focal_length":null,"fps":null,"iso":null,"latitude":null,"lens_model":"","longitude":null,` +
	`"metering_mode":null,"sample_rate":null,"shutter_speed":null,"track_format":null,"tzname":"","tzoffset":null,"white_balance":null}`

// The rule reads the fields of exifinfo.ExifInfo by position.
func TestExifKeysFollowFields(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[exifinfo.ExifInfo]()
	if typ.NumField() != len(exifKeys) {
		t.Fatalf("ExifInfo has %d fields, exifKeys %d keys", typ.NumField(), len(exifKeys))
	}
	for i, k := range exifKeys {
		if got, want := strings.ToLower(typ.Field(i).Name), strings.ReplaceAll(k, "_", ""); got != want {
			t.Errorf("field %d is %s, its key %s", i, typ.Field(i).Name, k)
		}
	}
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
		{name: "date", key: "date", raw: `"2020-06-06T14:15:24.725564+00:00"`, want: `["2020-06-06T14:15:24.725564Z",0]`},
		{name: "date in another offset", key: "date", raw: `"2020-06-06T19:45:24.725564+05:30"`, want: `["2020-06-06T14:15:24.725564Z",19800]`},
		{name: "date in -07:00", key: "date", raw: `"2020-06-06T07:15:24.725564-07:00"`, want: `["2020-06-06T14:15:24.725564Z",-25200]`},
		{name: "date with an offset in seconds", key: "date", raw: `"2020-06-06T15:49:00+01:33:36"`, want: `["2020-06-06T14:15:24Z",5616]`},
		{name: "date_added in another offset", key: "date_added", raw: `"2020-06-06T19:45:24.725564+05:30"`, want: `"2020-06-06T14:15:24.725564Z"`},
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
		{name: "persons unknown", key: "persons", raw: `["_UNKNOWN_"]`, want: `["_UNKNOWN_"]`},
		{name: "persons unknown and a name", key: "persons", raw: `["_UNKNOWN_", "A"]`, want: `["_UNKNOWN_","A"]`},
		{name: "persons empty", key: "persons", raw: `[]`, want: `[]`},
		{name: "persons null item", key: "persons", raw: `[null]`, want: `[""]`},
		{name: "persons empty name", key: "persons", raw: `[""]`, want: `[""]`},
		{name: "persons name", key: "persons", raw: `["A"]`, want: `["A"]`},
		{name: "persons null", key: "persons", raw: `null`, wantErr: "persons: want a list of strings"},

		{name: "hdr null", key: "hdr", raw: `null`, wantErr: "hdr: want a boolean"},
		{name: "visible", key: "visible", raw: `false`, want: `false`},
		{name: "tzname null", key: "tzname", raw: `null`, want: `""`},
		{name: "cloud_owner_hashed_id null", key: "cloud_owner_hashed_id", raw: `null`, want: `""`},
		{name: "spatial", key: "spatial", raw: `2`, want: `2`},
		{name: "spatial null", key: "spatial", raw: `null`, want: `0`},
		{name: "spatial as a float", key: "spatial", raw: `2.0`, wantErr: "spatial: want an integer"},
		{name: "adjustment_type null", key: "adjustment_type", raw: `null`, want: `null`},
		{name: "adjustment_type 0 is not null", key: "adjustment_type", raw: `0`, want: `0`},
		{name: "adjustment_type as a float", key: "adjustment_type", raw: `2.0`, wantErr: "adjustment_type: want an integer or null"},
		{name: "imported_by", key: "imported_by", raw: `["Camera", null]`, want: `["Camera",""]`},
		{name: "imported_by nulls", key: "imported_by", raw: `[null, null]`, want: `["",""]`},
		{name: "imported_by of one", key: "imported_by", raw: `["a"]`, wantErr: "imported_by: want a list of two strings or nulls"},
		{name: "imported_by null", key: "imported_by", raw: `null`, wantErr: "imported_by: want a list of two strings or nulls"},
		{name: "imported_by of three", key: "imported_by", raw: `["a", "b", "c"]`, wantErr: "imported_by: want a list of two strings or nulls"},
		{name: "imported_by with a number", key: "imported_by", raw: `["a", 1]`, wantErr: "imported_by: want a list of two strings or nulls"},

		{name: "import_info of a photo without a session", key: "import_info", raw: `{}`, want: `{}`},
		{name: "import_info", key: "import_info",
			raw:  `{"creation_date": "2020-06-07T06:32:03+05:30", "end_date": null, "start_date": "2020-06-07T01:00:00.500000+00:00", "title": "I", "uuid": "U"}`,
			want: `{"creation_date":"2020-06-07T01:02:03Z","end_date":null,"start_date":"2020-06-07T01:00:00.5Z","title":"I","uuid":"U"}`},
		{name: "import_info with a null title", key: "import_info",
			raw:  `{"creation_date": "2001-01-01T00:00:00+00:00", "end_date": null, "start_date": null, "title": null, "uuid": "U"}`,
			want: `{"creation_date":"2001-01-01T00:00:00Z","end_date":null,"start_date":null,"title":"","uuid":"U"}`},
		{name: "import_info null", key: "import_info", raw: `null`, wantErr: "import_info: want an object"},
		{name: "import_info list", key: "import_info", raw: `[]`, wantErr: "import_info: want an object"},
		{name: "import_info without uuid", key: "import_info", raw: `{"title": null}`, wantErr: "import_info.uuid: want a value (the key is missing)"},
		{name: "import_info without end_date", key: "import_info",
			raw: `{"creation_date": null, "start_date": null, "title": null, "uuid": "U"}`, wantErr: "import_info.end_date: want a value (the key is missing)"},
		{name: "import_info uuid null", key: "import_info",
			raw: `{"creation_date": null, "end_date": null, "start_date": null, "title": null, "uuid": null}`, wantErr: "import_info.uuid: want a string"},
		{name: "import_info title number", key: "import_info",
			raw: `{"creation_date": null, "end_date": null, "start_date": null, "title": 5, "uuid": "U"}`, wantErr: "import_info.title: want a string or null"},
		{name: "import_info date number", key: "import_info",
			raw: `{"creation_date": 5, "end_date": null, "start_date": null, "title": null, "uuid": "U"}`, wantErr: "import_info.creation_date: want an ISO 8601 string"},

		{name: "comments", key: "comments",
			raw:  `[{"datetime": "2020-06-06T19:45:24.725564+05:30", "ismine": true, "text": "t", "user": null}, {"datetime": "1970-01-01T00:00:00+00:00", "ismine": false, "text": "a", "user": "u"}]`,
			want: `[{"datetime":"2020-06-06T14:15:24.725564Z","ismine":true,"text":"t","user":""},{"datetime":"1970-01-01T00:00:00Z","ismine":false,"text":"a","user":"u"}]`},
		{name: "comments empty", key: "comments", raw: `[]`, want: `[]`},
		{name: "comments null", key: "comments", raw: `null`, wantErr: "comments: want a list of comments"},
		{name: "a comment without ismine", key: "comments", raw: `[{"datetime": "2020-06-06T00:00:00+00:00", "text": "t", "user": null}]`, wantErr: "comments: want a list of comments"},
		{name: "a comment without text", key: "comments", raw: `[{"datetime": "2020-06-06T00:00:00+00:00", "ismine": true, "user": null}]`, wantErr: "comments: want a list of comments"},
		{name: "a comment with a null date", key: "comments", raw: `[{"datetime": null, "ismine": true, "text": "t", "user": null}]`, wantErr: "comments: want a list of comments"},
		{name: "a comment with a text that is a number", key: "comments", raw: `[{"datetime": "2020-06-06T00:00:00+00:00", "ismine": true, "text": 5, "user": null}]`, wantErr: "comments: want a list of comments"},
		{name: "a null comment", key: "comments", raw: `[null]`, wantErr: "comments: want a list of comments"},
		{name: "likes", key: "likes", raw: `[{"datetime": "2020-06-06T00:00:00+00:00", "ismine": false, "user": "u"}]`, want: `[{"datetime":"2020-06-06T00:00:00Z","ismine":false,"user":"u"}]`},
		{name: "a like with a date that is not a date", key: "likes", raw: `[{"datetime": "SECRET-DATE", "ismine": false, "user": null}]`, wantErr: "likes: want a list of likes"},
		{name: "a like with a string ismine", key: "likes", raw: `[{"datetime": "2020-06-06T00:00:00+00:00", "ismine": "SECRET-M", "user": null}]`, wantErr: "likes: want a list of likes"},

		{name: "date_original", key: "date_original", raw: `"2020-06-06T19:45:24.725564+05:30"`, want: `["2020-06-06T14:15:24.725564Z",19800]`},
		{name: "date_original null", key: "date_original", raw: `null`, wantErr: "date_original: want an ISO 8601 string"},
		{name: "owner", key: "owner", raw: `"A"`, want: `"A"`},
		{name: "owner null", key: "owner", raw: `null`, want: `""`},
		{name: "owner number", key: "owner", raw: `5`, wantErr: "owner: want a string or null"},

		{name: "exif_info", key: "exif_info", raw: refExif, want: `{"aperture":1.8,"bit_rate":1000.5,"camera_make":"Acme","camera_model":"Acme One","codec":"hvc1",` +
			`"date":["2020-06-06T10:00:00Z",19800],"duration":12.25,"exposure_bias":-0.5,"flash_fired":true,"focal_length":4.25,"fps":29.97,"iso":100,` +
			`"latitude":48.8566,"lens_model":"Acme One 4mm","longitude":2.3522,"metering_mode":5,"sample_rate":48000,"shutter_speed":0.008,` +
			`"track_format":1,"tzname":"Asia/Kolkata","tzoffset":19800,"white_balance":0}`},
		{name: "exif_info of a photo without a row", key: "exif_info", raw: refExifNull, want: zeroExif},
		{name: "exif_info null", key: "exif_info", raw: `null`, wantErr: "exif_info: want an object"},
		{name: "exif_info list", key: "exif_info", raw: `[]`, wantErr: "exif_info: want an object"},
		{name: "exif_info empty", key: "exif_info", raw: `{}`, wantErr: "exif_info.flash_fired: want a value (the key is missing)"},
		{name: "exif_info without tzname", key: "exif_info", raw: strings.Replace(refExifNull, `, "tzname": null`, ``, 1), wantErr: "exif_info.tzname: want a value (the key is missing)"},
		{name: "exif_info iso as a float", key: "exif_info", raw: strings.Replace(refExif, `"iso": 100`, `"iso": 100.0`, 1), wantErr: "exif_info.iso: want a value of the type"},
		{name: "exif_info iso as a string", key: "exif_info", raw: strings.Replace(refExif, `"iso": 100`, `"iso": "SECRET-ISO"`, 1), wantErr: "exif_info.iso: want a value of the type"},
		{name: "exif_info flash_fired as a number", key: "exif_info", raw: strings.Replace(refExif, `"flash_fired": true`, `"flash_fired": 1`, 1), wantErr: "exif_info.flash_fired: want a value of the type"},
		{name: "exif_info camera_make as a number", key: "exif_info", raw: strings.Replace(refExif, `"camera_make": "Acme"`, `"camera_make": 5`, 1), wantErr: "exif_info.camera_make: want a value of the type"},
		{name: "exif_info date that is not a date", key: "exif_info", raw: strings.Replace(refExif, `"2020-06-06T15:30:00+05:30"`, `"SECRET-DATE"`, 1), wantErr: "exif_info.date: want an ISO 8601 string"},
		{name: "exif_info date as a number", key: "exif_info", raw: strings.Replace(refExif, `"2020-06-06T15:30:00+05:30"`, `5`, 1), wantErr: "exif_info.date: want an ISO 8601 string or null"},

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
	zones := []*time.Location{time.UTC, ist, time.FixedZone("W", -7*3600), time.FixedZone("S", 5640+28)}
	t.Run("date is the moment and the offset of its zone", func(t *testing.T) {
		t.Parallel()
		for i, want := range []string{
			`["2020-06-06T14:15:24.725564Z",0]`, `["2020-06-06T14:15:24.725564Z",19800]`,
			`["2020-06-06T14:15:24.725564Z",-25200]`, `["2020-06-06T14:15:24.725564Z",5668]`,
		} {
			p := &photoinfo.PhotoInfo{Date: when.In(zones[i])}
			if got := goCanon(t, "date", p); got != want {
				t.Errorf("zone %v: %s, want %s", zones[i], got, want)
			}
		}
	})
	t.Run("date_original and the date of exif_info are the moment and the offset of their zone", func(t *testing.T) {
		t.Parallel()
		p := &photoinfo.PhotoInfo{DateOriginal: when}
		p.ExifInfo.Date = ptr(when.In(zones[2]))
		if got := goCanon(t, "date_original", p); got != `["2020-06-06T14:15:24.725564Z",19800]` {
			t.Errorf("date_original %s", got)
		}
		if got := goCanon(t, "exif_info", p); !strings.Contains(got, `"date":["2020-06-06T14:15:24.725564Z",-25200]`) {
			t.Errorf("exif_info %s", got)
		}
	})
	t.Run("date_added is a moment in any zone", func(t *testing.T) {
		t.Parallel()
		for _, loc := range zones {
			p := &photoinfo.PhotoInfo{DateAdded: ptr(when.In(loc))}
			if got := goCanon(t, "date_added", p); got != `"2020-06-06T14:15:24.725564Z"` {
				t.Errorf("zone %v: %s", loc, got)
			}
		}
	})
	t.Run("nanoseconds are not rounded", func(t *testing.T) {
		t.Parallel()
		p := &photoinfo.PhotoInfo{Date: when.Add(time.Nanosecond)}
		if got := goCanon(t, "date", p); got != `["2020-06-06T14:15:24.725564001Z",19800]` {
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
			"persons": `[]`, "burst_default_pick": `false`,
			"adjustment_type": `null`, "imported_by": `["",""]`, "spatial": `0`, "tzname": `""`, "visible": `false`,
			"import_info": `{}`, "project_info": `[]`, "comments": `[]`, "likes": `[]`,
			"owner": `""`, "exif_info": zeroExif,
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
