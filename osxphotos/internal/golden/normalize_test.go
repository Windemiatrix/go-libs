package golden

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
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

// v is the value of key in the dictionary of a photo.
func goCanon(t *testing.T, key string, v any) string {
	t.Helper()
	got, err := rules[key].got(map[string]any{key: v}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := canonical(got)
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

// The rule reads the fields of scoreinfo.ScoreInfo by position.
func TestScoreKeysFollowFields(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[scoreinfo.ScoreInfo]()
	if typ.NumField() != len(scoreKeys) {
		t.Fatalf("ScoreInfo has %d fields, scoreKeys %d keys", typ.NumField(), len(scoreKeys))
	}
	for i, k := range scoreKeys {
		if got, want := strings.ToLower(typ.Field(i).Name), strings.ReplaceAll(k, "_", ""); got != want {
			t.Errorf("field %d is %s, its key %s", i, typ.Field(i).Name, k)
		}
	}
}

// The rule reads the fields of placeinfo.Names and placeinfo.Address by
// position.
func TestPlaceKeysFollowFields(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		typ  reflect.Type
		keys []string
	}{
		{reflect.TypeFor[placeinfo.Names](), placeNameKeys},
		{reflect.TypeFor[placeinfo.Address](), addressKeys},
	} {
		if c.typ.NumField() != len(c.keys) {
			t.Fatalf("%s has %d fields, %d keys", c.typ.Name(), c.typ.NumField(), len(c.keys))
		}
		for i, k := range c.keys {
			if got, want := strings.ToLower(c.typ.Field(i).Name), strings.ReplaceAll(k, "_", ""); got != want {
				t.Errorf("%s field %d is %s, its key %s", c.typ.Name(), i, c.typ.Field(i).Name, k)
			}
		}
	}
}

// The keys are sorted, as the encoder of canonical writes them. wantScore is
// refScore, nullScore every score null.
const (
	zeroScore = `{"behavioral":0,"curation":0,"failure":0,"harmonious_color":0,"highlight_visibility":0,"immersiveness":0,"interaction":0,` +
		`"interesting_subject":0,"intrusive_object_presence":0,"lively_color":0,"low_light":0,"noise":0,"overall":0,"pleasant_camera_tilt":0,` +
		`"pleasant_composition":0,"pleasant_lighting":0,"pleasant_pattern":0,"pleasant_perspective":0,"pleasant_post_processing":0,` +
		`"pleasant_reflection":0,"pleasant_symmetry":0,"promotion":0,"sharply_focused_subject":0,"tastefully_blurred":0,` +
		`"well_chosen_subject":0,"well_framed_subject":0,"well_timed_shot":0}`
	wantScore = `{"behavioral":null,"curation":0.02,"failure":0.06,"harmonious_color":0.07,"highlight_visibility":0.04,"immersiveness":0.08,"interaction":null,` +
		`"interesting_subject":0.1,"intrusive_object_presence":0.11,"lively_color":0.12,"low_light":0.13,"noise":0.14,"overall":0.01,` +
		`"pleasant_camera_tilt":0.15,"pleasant_composition":0.16,"pleasant_lighting":0.17,"pleasant_pattern":0.18,"pleasant_perspective":0.19,` +
		`"pleasant_post_processing":0.2,"pleasant_reflection":0.21,"pleasant_symmetry":0.22,"promotion":0.03,"sharply_focused_subject":0.23,` +
		`"tastefully_blurred":0.24,"well_chosen_subject":0.25,"well_framed_subject":0.26,"well_timed_shot":0.27}`
)

var nullScore = strings.NewReplacer(`:0,`, `:null,`, `:0}`, `:null}`).Replace(zeroScore)

func scoreWith(old, value string) string { return strings.Replace(refScore, old, value, 1) }

const (
	wantPlaceAddress = `{"city":"Townsville","country":"Exampleland","iso_country_code":"EX","postal_code":"","state_province":"",` +
		`"street":"1 Example Street","sub_administrative_area":"","sub_locality":""}`
	wantPlace = `{"address":` + wantPlaceAddress + `,"address_str":"1 Example Street, Townsville","country_code":"EX","ishome":true,` +
		`"name":"Example Park, Townsville, Exampleland","names":{"additional_city_info":[],"area_of_interest":["Example Park"],` +
		`"body_of_water":[],"city":["Townsville","Greater Townsville"],"country":["Exampleland"],"field0":[],"field10":[],` +
		`"field13":["Field Thirteen"],"field15":[],"field16":[],"field5":[],"inland_water":[],"ocean":[],"postal_code":[],` +
		`"region":[],"state_province":[],"street_address":[],"sub_administrative_area":[],"sub_throughfare":[]}}`
)

func placeWith(old, value string) string { return strings.Replace(refPlace, old, value, 1) }

// The values of refSearchInfoEmpty are only [] and "", so the compact form is
// the literal without the spaces after its separators.
var zeroSearchInfo = strings.NewReplacer(`": `, `":`, `, "`, `,"`).Replace(refSearchInfoEmpty)

func searchInfoWith(old, value string) string {
	return strings.Replace(refSearchInfoEmpty, old, value, 1)
}

// The keys of a person but uuid and name, as osxphotos writes them (personinfo.py:110-123).
const (
	personNulls  = `"displayname": null, "facecount": null, "favorite": false, "feature_less": false, "keyface": null, "keyphoto": null, "sort_order": null`
	personValues = `"displayname": "D", "facecount": 2, "favorite": true, "feature_less": true, "keyface": 7, "keyphoto": "K", "sort_order": 1`
)

// uuid and name are JSON values.
func personJSON(uuid, name, rest string) string {
	return `{"uuid": ` + uuid + `, "name": ` + name + `, ` + rest + `}`
}

func wantPerson(uuid, name string) string {
	return `{"displayname":"","facecount":0,"favorite":false,"feature_less":false,"keyface":0,"keyphoto":"","name":"` + name +
		`","sort_order":0,"uuid":"` + uuid + `"}`
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

		{name: "person_info of equal names by uuid, nulls as zeros", key: "person_info",
			raw: `[` + personJSON(`"U-3"`, `"A"`, personNulls) + `, ` + personJSON(`"U-2"`, `"_UNKNOWN_"`, personNulls) + `, ` +
				personJSON(`"U-1"`, `"_UNKNOWN_"`, personValues) + `, ` + personJSON(`"U-1"`, `"_UNKNOWN_"`, personValues) + `]`,
			want: `[` + wantPerson("U-3", "A") + `,{"displayname":"D","facecount":2,"favorite":true,"feature_less":true,"keyface":7,"keyphoto":"K",` +
				`"name":"_UNKNOWN_","sort_order":1,"uuid":"U-1"},{"displayname":"D","facecount":2,"favorite":true,"feature_less":true,"keyface":7,` +
				`"keyphoto":"K","name":"_UNKNOWN_","sort_order":1,"uuid":"U-1"},` + wantPerson("U-2", "_UNKNOWN_") + `]`},
		{name: "person_info of other names in the order of the reference", key: "person_info",
			raw:  `[` + personJSON(`"U-2"`, `"_UNKNOWN_"`, personNulls) + `, ` + personJSON(`"U-1"`, `"A"`, personNulls) + `]`,
			want: `[` + wantPerson("U-2", "_UNKNOWN_") + `,` + wantPerson("U-1", "A") + `]`},
		{name: "person_info empty", key: "person_info", raw: `[]`, want: `[]`},
		{name: "person_info null", key: "person_info", raw: `null`, wantErr: "person_info: want a list of objects"},
		{name: "person_info of strings", key: "person_info", raw: `["SECRET-P"]`, wantErr: "person_info[0]: want an object"},
		{name: "person_info null element", key: "person_info", raw: `[null]`, wantErr: "person_info[0].uuid: want a value (the key is missing)"},
		{name: "person_info name null", key: "person_info", raw: `[` + personJSON(`"U"`, `null`, personNulls) + `]`, wantErr: "person_info[0].name: want a string"},
		{name: "person_info name number", key: "person_info", raw: `[` + personJSON(`"U"`, `424242`, personNulls) + `]`, wantErr: "person_info[0].name: want a string"},
		{name: "person_info uuid null", key: "person_info", raw: `[` + personJSON(`null`, `"A"`, personNulls) + `]`, wantErr: "person_info[0].uuid: want a string"},
		{name: "person_info favorite null", key: "person_info", raw: `[` + personJSON(`"U"`, `"A"`, strings.Replace(personNulls, `"favorite": false`, `"favorite": null`, 1)) + `]`,
			wantErr: "person_info[0].favorite: want a boolean"},
		{name: "person_info feature_less null", key: "person_info",
			raw:     `[` + personJSON(`"U"`, `"A"`, strings.Replace(personNulls, `"feature_less": false`, `"feature_less": null`, 1)) + `]`,
			wantErr: "person_info[0].feature_less: want a boolean"},
		{name: "person_info keyface as a float", key: "person_info", raw: `[` + personJSON(`"U"`, `"A"`, strings.Replace(personNulls, `"keyface": null`, `"keyface": 7.0`, 1)) + `]`,
			wantErr: "person_info[0].keyface: want an integer or null"},
		{name: "person_info displayname number", key: "person_info",
			raw:     `[` + personJSON(`"U"`, `"A"`, strings.Replace(personNulls, `"displayname": null`, `"displayname": 424242`, 1)) + `]`,
			wantErr: "person_info[0].displayname: want a string or null"},
		{name: "person_info without keyphoto", key: "person_info", raw: `[` + personJSON(`"U"`, `"A"`, strings.Replace(personNulls, `"keyphoto": null, `, ``, 1)) + `]`,
			wantErr: "person_info[0].keyphoto: want a value (the key is missing)"},

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

		{name: "score, null behavioral and interaction stay null", key: "score", raw: refScore, want: wantScore},
		{name: "score of a photo without a row", key: "score", raw: refScoreZero, want: zeroScore},
		{name: "score of nulls", key: "score", raw: strings.NewReplacer(`0.0`, `null`).Replace(refScoreZero), want: nullScore},
		{name: "score with an integer", key: "score", raw: scoreWith(`"overall": 0.01`, `"overall": 1`), want: strings.Replace(wantScore, `"overall":0.01`, `"overall":1`, 1)},
		{name: "score with an extra key", key: "score", raw: scoreWith(`"overall": 0.01`, `"overall": 0.01, "extra": "SECRET-X"`), want: wantScore},
		{name: "score null", key: "score", raw: `null`, wantErr: "score: want an object"},
		{name: "score list", key: "score", raw: `[]`, wantErr: "score: want an object"},
		{name: "score empty", key: "score", raw: `{}`, wantErr: "score.overall: want a value (the key is missing)"},
		{name: "score without well_timed_shot", key: "score", raw: scoreWith(`, "well_timed_shot": 0.27`, ``), wantErr: "score.well_timed_shot: want a value (the key is missing)"},
		{name: "score overall as a string", key: "score", raw: scoreWith(`"overall": 0.01`, `"overall": "SECRET-S"`), wantErr: "score.overall: want a number or null"},
		{name: "score failure as a boolean", key: "score", raw: scoreWith(`"failure": 0.06`, `"failure": true`), wantErr: "score.failure: want a number or null"},
		{name: "score noise out of range", key: "score", raw: scoreWith(`"noise": 0.14`, `"noise": 1e999`), wantErr: "score.noise: want a number or null"},

		{name: "place of a photo without one", key: "place", raw: `{}`, want: `{}`},
		{name: "place", key: "place", raw: refPlace, want: wantPlace},
		{name: "place with nulls", key: "place",
			raw: strings.NewReplacer(refPlaceAddress, `{"city": null, "country": null, "iso_country_code": null, "postal_code": null, `+
				`"state_province": null, "street": null, "sub_administrative_area": null, "sub_locality": null}`,
				`"ishome": true`, `"ishome": null`, `"name": "Example Park, Townsville, Exampleland"`, `"name": null`,
				`"city": ["Townsville"`, `"city": [null`).Replace(refPlace),
			want: strings.NewReplacer(wantPlaceAddress, `{"city":"","country":"","iso_country_code":"","postal_code":"","state_province":"",`+
				`"street":"","sub_administrative_area":"","sub_locality":""}`,
				`"ishome":true`, `"ishome":false`, `"name":"Example Park, Townsville, Exampleland"`, `"name":""`,
				`"city":["Townsville"`, `"city":[""`).Replace(wantPlace)},
		{name: "place null", key: "place", raw: `null`, wantErr: "place: want an object"},
		{name: "place list", key: "place", raw: `[]`, wantErr: "place: want an object"},
		{name: "place without names", key: "place", raw: `{"name": null}`, wantErr: "place.names: want a value (the key is missing)"},
		{name: "place without name", key: "place", raw: placeWith(`"name": "Example Park, Townsville, Exampleland", `, ``),
			wantErr: "place.name: want a value (the key is missing)"},
		{name: "place name number", key: "place", raw: placeWith(`"name": "Example Park, Townsville, Exampleland"`, `"name": 424242`),
			wantErr: "place.name: want a string or null"},
		{name: "place ishome string", key: "place", raw: placeWith(`"ishome": true`, `"ishome": "SECRET-H"`), wantErr: "place.ishome: want a boolean or null"},
		{name: "place address null", key: "place", raw: placeWith(refPlaceAddress, `null`), wantErr: "place.address: want an object"},
		{name: "place address city number", key: "place", raw: placeWith(`"city": "Townsville"`, `"city": 424242`), wantErr: "place.address.city: want a string or null"},
		{name: "place names without city", key: "place", raw: placeWith(`"city": ["Townsville", "Greater Townsville"], `, ``),
			wantErr: "place.names.city: want a value (the key is missing)"},
		{name: "place names city null", key: "place", raw: placeWith(`["Townsville", "Greater Townsville"]`, `null`), wantErr: "place.names.city: want a list of strings"},
		{name: "place names city with a number", key: "place", raw: placeWith(`["Townsville", "Greater Townsville"]`, `["SECRET-C", 424242]`),
			wantErr: "place.names.city: want a list of strings"},

		{name: "labels in the order of the reference", key: "labels", raw: `["b", "a"]`, want: `["b","a"]`},
		{name: "labels null", key: "labels", raw: `null`, wantErr: "labels: want a list of strings"},
		{name: "labels_normalized with a number", key: "labels_normalized", raw: `["a", 424242]`, wantErr: "labels_normalized: want a list of strings"},
		{name: "search_info of a photo without terms", key: "search_info", raw: refSearchInfoEmpty, want: zeroSearchInfo},
		{name: "search_info", key: "search_info", raw: refSearchInfo(false), want: strings.NewReplacer(`"city":""`, `"city":"Townsville"`,
			`"holidays":[]`, `"holidays":["New Year"]`, `"labels":[]`, `"labels":["Beach","Dog"]`).Replace(zeroSearchInfo)},
		{name: "search_info with an extra key", key: "search_info", raw: searchInfoWith(`"year": ""}`, `"year": "", "text_found": ["SECRET-X"]}`),
			want: zeroSearchInfo},
		{name: "search_info null", key: "search_info", raw: `null`, wantErr: "search_info: want an object"},
		{name: "search_info list", key: "search_info", raw: `[]`, wantErr: "search_info: want an object"},
		{name: "search_info empty", key: "search_info", raw: `{}`, wantErr: "search_info.activities: want a value (the key is missing)"},
		{name: "search_info without year", key: "search_info", raw: searchInfoWith(`, "year": ""`, ``),
			wantErr: "search_info.year: want a value (the key is missing)"},
		{name: "search_info labels null", key: "search_info", raw: searchInfoWith(`"labels": []`, `"labels": null`), wantErr: "search_info.labels: want a list of strings"},
		{name: "search_info labels with a number", key: "search_info", raw: searchInfoWith(`"labels": []`, `"labels": ["SECRET-L", 424242]`),
			wantErr: "search_info.labels: want a list of strings"},
		{name: "search_info city null", key: "search_info", raw: searchInfoWith(`"city": ""`, `"city": null`), wantErr: "search_info.city: want a string"},
		{name: "search_info city as a list", key: "search_info", raw: searchInfoWith(`"city": ""`, `"city": ["SECRET-C"]`), wantErr: "search_info.city: want a string"},
		{name: "search_info_normalized empty", key: "search_info_normalized", raw: `{}`,
			wantErr: "search_info_normalized.activities: want a value (the key is missing)"},

		{name: "face_info of equal names by _pk, null as \"\"", key: "face_info",
			raw:  `[{"_pk": 5, "name": null, "uuid": "F", "asset_uuid": null, "center": [1, 2]}, {"_pk": 3, "name": "", "uuid": null, "asset_uuid": "P", "center": [1.0, 2.5]}]`,
			want: `[{"_pk":3,"asset_uuid":"P","center":[1,2.5],"name":"","uuid":""},{"_pk":5,"asset_uuid":"","center":[1,2],"name":"","uuid":"F"}]`},
		{name: "face_info empty", key: "face_info", raw: `[]`, want: `[]`},
		{name: "face_info null", key: "face_info", raw: `null`, wantErr: "face_info: want a list of objects"},
		{name: "face_info of numbers", key: "face_info", raw: `[424242]`, wantErr: "face_info: want a list of objects"},
		{name: "rating", key: "rating", raw: `0`, want: `0`},
		{name: "rating as a float", key: "rating", raw: `0.0`, wantErr: "rating: want an integer"},
		{name: "adjustments of a photo without the file", key: "adjustments", raw: `{}`, want: `{}`},
		{name: "adjustments timestamp is a moment", key: "adjustments", raw: `{"editor": "E", "timestamp": "2020-06-06T19:45:24.725564+05:30"}`,
			want: `{"editor":"E","timestamp":"2020-06-06T14:15:24.725564Z"}`},
		{name: "adjustments timestamp that is not a date", key: "original_adjustments", raw: `{"timestamp": "SECRET-DATE"}`,
			wantErr: "original_adjustments.timestamp: want an ISO 8601 string"},
		{name: "original_adjustments null", key: "original_adjustments", raw: `null`, wantErr: "original_adjustments: want an object"},
		{name: "original_adjustments list", key: "original_adjustments", raw: `[]`, wantErr: "original_adjustments: want an object"},
		{name: "path_derivatives in the order of the reference", key: "path_derivatives", raw: `["b", "a"]`, want: `["b","a"]`},
		{name: "path_derivatives null", key: "path_derivatives", raw: `null`, wantErr: "path_derivatives: want a list of strings"},

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
				if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "424242") || strings.Contains(err.Error(), c.raw) && len(c.raw) > 6 {
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
			if got := goCanon(t, "date", when.In(zones[i])); got != want {
				t.Errorf("zone %v: %s, want %s", zones[i], got, want)
			}
		}
	})
	t.Run("date_original and the date of exif_info are the moment and the offset of their zone", func(t *testing.T) {
		t.Parallel()
		if got := goCanon(t, "date_original", when); got != `["2020-06-06T14:15:24.725564Z",19800]` {
			t.Errorf("date_original %s", got)
		}
		if got := goCanon(t, "exif_info", exifinfo.ExifInfo{Date: ptr(when.In(zones[2]))}.AsDict()); !strings.Contains(got, `"date":["2020-06-06T14:15:24.725564Z",-25200]`) {
			t.Errorf("exif_info %s", got)
		}
	})
	t.Run("date_added is a moment in any zone", func(t *testing.T) {
		t.Parallel()
		for _, loc := range zones {
			if got := goCanon(t, "date_added", when.In(loc)); got != `"2020-06-06T14:15:24.725564Z"` {
				t.Errorf("zone %v: %s", loc, got)
			}
		}
	})
	t.Run("nanoseconds are not rounded", func(t *testing.T) {
		t.Parallel()
		if got := goCanon(t, "date", when.Add(time.Nanosecond)); got != `["2020-06-06T14:15:24.725564001Z",19800]` {
			t.Errorf("got %s", got)
		}
	})
	t.Run("None where the reference reads null", func(t *testing.T) {
		t.Parallel()
		for _, c := range []struct {
			key  string
			v    any
			want string
		}{
			{"title", nil, `""`},
			{"owner", nil, `""`},
			{"tzoffset", nil, `null`},
			{"date_modified", nil, `null`},
			{"location", []any{nil, nil}, `[null,null]`},
			{"albums", []any{nil, "A"}, `["","A"]`},
			{"imported_by", []any{"z", nil}, `["","z"]`},
			{"burst_photos", []string{"b", "a"}, `["a","b"]`},
			{"exif_info", exifinfo.ExifInfo{}.AsDict(), zeroExif},
			{"score", scoreinfo.ScoreInfo{}.AsDict(), nullScore},
			{"place", map[string]any{}, `{}`},
			{"place", placeinfo.PlaceInfo{}.AsDict(), strings.NewReplacer(wantPlaceAddress, `{"city":"","country":"","iso_country_code":"",`+
				`"postal_code":"","state_province":"","street":"","sub_administrative_area":"","sub_locality":""}`,
				`"address_str":"1 Example Street, Townsville"`, `"address_str":""`, `"country_code":"EX"`, `"country_code":""`,
				`"ishome":true`, `"ishome":false`, `"name":"Example Park, Townsville, Exampleland"`, `"name":""`,
				`["Example Park"]`, `[]`, `["Townsville","Greater Townsville"]`, `[]`, `["Exampleland"]`, `[]`, `["Field Thirteen"]`, `[]`).Replace(wantPlace)},
			{"import_info", map[string]any{}, `{}`},
			{"import_info", (&albuminfo.ImportInfo{UUID: "U"}).AsDict(),
				`{"creation_date":"2001-01-01T00:00:00Z","end_date":null,"start_date":null,"title":"","uuid":"U"}`},
			{"album_info", []any{
				(&albuminfo.AlbumInfo{UUID: "B", SortOrder: albuminfo.AlbumSortTitle}).AsDict(),
				(&albuminfo.AlbumInfo{UUID: "A", Title: "T", Owner: "O", ParentUUID: "P", FolderNames: []string{"F"}, FolderUUIDs: []string{"G"}}).AsDict(),
				(&albuminfo.AlbumInfo{UUID: "B"}).AsDict(),
			}, `[{"creation_date":"2001-01-01T00:00:00Z","end_date":null,"folder_list":["G"],"folder_names":["F"],"owner":"O","parent":"P",` +
				`"sort_order":null,"start_date":null,"title":"T","uuid":"A"},{"creation_date":"2001-01-01T00:00:00Z","end_date":null,` +
				`"folder_list":[],"folder_names":[],"owner":"","parent":"","sort_order":null,"start_date":null,"title":"","uuid":"B"}]`},
			{"project_info", []any{(&albuminfo.AlbumInfo{UUID: "A", ParentUUID: "P"}).AsDict()},
				`[{"creation_date":"2001-01-01T00:00:00Z","end_date":null,"folder_list":[],"folder_names":[],"owner":"",` +
					`"sort_order":null,"start_date":null,"title":"","uuid":"A"}]`},
			{"comments", []any{photoinfo.CommentInfo{Datetime: ptr(when), IsMine: true, Text: "t"}.AsDict()},
				`[{"datetime":"2020-06-06T14:15:24.725564Z","ismine":true,"text":"t","user":""}]`},
			{"person_info", []any{
				personinfo.PersonInfo{UUID: "U-2", Name: "A"}.AsDict(),
				personinfo.PersonInfo{UUID: "U-1", Name: "A", DisplayName: "D", KeyPhotoUUID: "K"}.AsDict(),
			}, `[{"displayname":"D","facecount":0,"favorite":false,"feature_less":false,"keyface":0,"keyphoto":"K","name":"A","sort_order":0,"uuid":"U-1"},` +
				wantPerson("U-2", "A") + `]`},
			{"face_info", []any{
				map[string]any{"_pk": int64(9), "name": nil, "uuid": nil, "asset_uuid": "P"},
				map[string]any{"_pk": int64(8), "name": nil, "uuid": "F", "asset_uuid": nil},
			}, `[{"_pk":8,"asset_uuid":"","name":"","uuid":"F"},{"_pk":9,"asset_uuid":"P","name":"","uuid":""}]`},
			{"adjustments", map[string]any{"data": []byte("x"), "editor": nil, "timestamp": when}, `{"data":null,"editor":null,"timestamp":"2020-06-06T14:15:24.725564Z"}`},
		} {
			if got := goCanon(t, c.key, c.v); got != c.want {
				t.Errorf("%s of %v: %s, want %s", c.key, c.v, got, c.want)
			}
		}
	})
	t.Run("the timestamp of adjustments is one moment on both sides", func(t *testing.T) {
		t.Parallel()
		ref, err := refCanon("adjustments", `{"adjustment_format_version": null, "adjustments": [], "base_version": null, "data": null, `+
			`"editor": null, "format_id": null, "format_version": null, "metadata": null, "orientation": null, `+
			`"timestamp": "2020-06-06T19:45:24.725564+05:30", "version_info": null}`)
		if err != nil {
			t.Fatal(err)
		}
		if got := goCanon(t, "adjustments", adjustmentsinfo.AdjustmentsInfo{Timestamp: &when}.AsDict()); got != ref {
			t.Errorf("Go %s, reference %s", got, ref)
		}
	})
	t.Run("a key missing from an object is not null", func(t *testing.T) {
		t.Parallel()
		missing := goCanon(t, "face_info", []any{map[string]any{"_pk": 1}})
		if null := goCanon(t, "face_info", []any{map[string]any{"_pk": 1, "name": nil}}); missing == null {
			t.Errorf("a face without name and one with a null name are both %s", missing)
		}
	})
	t.Run("the list of the Go side is not sorted", func(t *testing.T) {
		t.Parallel()
		keywords := []string{"b", "a"}
		if got := goCanon(t, "keywords", keywords); got != `["b","a"]` {
			t.Errorf("got %s", got)
		}
		if keywords[0] != "b" {
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

func TestPyValueNumber(t *testing.T) {
	t.Parallel()
	got := pyValue(map[string]any{"a": []any{json.Number("1.0"), json.Number("2")}})
	want := map[string]any{"a": []any{1.0, 2.0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pyValue = %#v, want %#v", got, want)
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
