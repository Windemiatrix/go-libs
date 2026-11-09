package photosdb

import (
	"bytes"
	"encoding/json"
	"maps"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// The keys of PhotoInfo.asdict(shallow=True), photoinfo.py:2134-2205.
var shallowKeys = []string{
	"albums", "burst", "cloud_guid", "cloud_owner_hashed_id", "comments", "date_added", "date_modified", "date_trashed",
	"date", "description", "exif_info", "external_edit", "face_info", "favorite", "filename", "fingerprint", "folders",
	"has_raw", "hasadjustments", "hdr", "height", "hidden", "incloud", "intrash", "iscloudasset", "ismissing", "ismovie",
	"isphoto", "israw", "isreference", "keywords", "labels", "latitude", "library", "likes", "live_photo", "location",
	"longitude", "orientation", "original_filename", "original_filesize", "original_height", "original_orientation",
	"original_width", "owner", "panorama", "path_edited_live_photo", "path_edited", "path_live_photo", "path_raw", "path",
	"persons", "place", "portrait", "raw_original", "score", "screenshot", "selfie", "shared", "slow_mo", "time_lapse",
	"title", "tzoffset", "uti_edited", "uti_original", "uti_raw", "uti", "uuid", "visible", "width",
}

// The keys of AsDict that fields.txt leaves out for good: paths of the
// machine, and values of a database of another program.
var notInFields = []string{"ai_caption", "library", "media_analysis", "path", "path_edited", "path_edited_live_photo", "path_live_photo", "path_raw"}

func TestAsDictKeys(t *testing.T) {
	t.Parallel()
	fields, err := golden.ReadFields(filepath.Join(testlib.RepoDir(t, "golden"), "fields.txt"))
	if err != nil {
		t.Fatal(err)
	}
	db, p := &DB{}, &photoinfo.PhotoInfo{}
	if got, want := slices.Sorted(maps.Keys(db.AsDict(p, false))), slices.Sorted(slices.Values(slices.Concat(fields, notInFields))); !slices.Equal(got, want) {
		t.Errorf("AsDict has the keys\n%v\nwant those of fields.txt and %v:\n%v", got, notInFields, want)
	}
	if got, want := slices.Sorted(maps.Keys(db.AsDict(p, true))), slices.Sorted(slices.Values(shallowKeys)); !slices.Equal(got, want) {
		t.Errorf("AsDict shallow has the keys\n%v\nwant\n%v", got, want)
	}
}

// The values are synthetic, so a failure may print them.
func TestAsDict(t *testing.T) {
	t.Parallel()
	lat, lon := 48.8566, 2.3522
	p := &photoinfo.PhotoInfo{
		UUID: "P", Filename: "IMG_0001.HEIC", Latitude: &lat, Longitude: &lon, Keywords: []string{"alpha"},
		Persons: []string{"Person A"}, PathDerivatives: []string{"/lib/d"},
		CloudMasterGUID: "guid", CloudOwnerHashedID: "owner-id", Description: "description", Fingerprint: "fingerprint",
		Owner: "Owner A", Title: "Title A", TZName: "Asia/Kolkata", UTIEdited: "public.heic", UTIRaw: "com.adobe.raw-image",
		Path: "/lib/p", PathEdited: "/lib/e", PathLive: "/lib/l", PathRaw: "/lib/r", PathEditedLive: "/lib/el",
		ImportedByName: "Camera", ImportedByBundleID: "com.example.camera",
	}
	comment := photoinfo.CommentInfo{User: "Person A", Text: "Comment 1"}
	db := &DB{info: Info{LibraryPath: "/lib"}, comments: map[string][]photoinfo.CommentInfo{"P": {comment}}}
	d := db.AsDict(p, false)
	for key, want := range map[string]any{
		"filename": "IMG_0001.HEIC", "rating": 0, "location": []any{48.8566, 2.3522},
		"keywords": []string{"alpha"}, "persons": []string{"Person A"}, "path_derivatives": []string{"/lib/d"},
		"cloud_guid": "guid", "cloud_owner_hashed_id": "owner-id", "description": "description", "fingerprint": "fingerprint",
		"owner": "Owner A", "title": "Title A", "tzname": "Asia/Kolkata", "uti_edited": "public.heic", "uti_raw": "com.adobe.raw-image",
		"library": "/lib", "path": "/lib/p", "path_edited": "/lib/e", "path_live_photo": "/lib/l", "path_raw": "/lib/r",
		"path_edited_live_photo": "/lib/el", "imported_by": []any{"Camera", "com.example.camera"},
		"comments": []any{comment.AsDict()},
	} {
		if !reflect.DeepEqual(d[key], want) {
			t.Errorf("%s = %#v, want %#v", key, d[key], want)
		}
	}

	empty := db.AsDict(&photoinfo.PhotoInfo{}, false)
	for _, key := range []string{
		"cloud_guid", "cloud_owner_hashed_id", "description", "fingerprint", "owner", "title", "tzname", "uti_edited", "uti_raw",
		"path", "path_edited", "path_live_photo", "path_raw", "path_edited_live_photo", "tzoffset", "media_analysis", "ai_caption",
	} {
		if empty[key] != nil {
			t.Errorf("%s of a photo without it = %#v, want nil", key, empty[key])
		}
	}
	for key, want := range map[string]any{
		"albums": []any{}, "burst_albums": []any{}, "keywords": []string{}, "persons": []string{}, "path_derivatives": []string{},
		"burst_photos": []string{}, "labels": []string{}, "labels_normalized": []string{},
		"face_info": []any{}, "comments": []any{}, "likes": []any{}, "person_info": []any{}, "project_info": []any{},
		"album_info": []any{}, "burst_album_info": []any{}, "folders": map[string]any{}, "place": map[string]any{},
		"import_info": map[string]any{}, "cloud_metadata": map[string]any{}, "adjustments": map[string]any{},
		"original_adjustments": map[string]any{}, "imported_by": []any{nil, nil}, "location": []any{nil, nil},
	} {
		if !reflect.DeepEqual(empty[key], want) {
			t.Errorf("%s of a photo without it = %#v, want %#v", key, empty[key], want)
		}
	}
	for _, key := range []string{"search_info", "search_info_normalized"} {
		if search, _ := empty[key].(map[string]any); !reflect.DeepEqual(search["labels"], []string{}) {
			t.Errorf("labels of %s = %#v, want []string{}", key, search["labels"])
		}
	}
	// 1970-01-01T00:00:00Z, where upstream takes midnight in the zone of the
	// computer (photosdb/photosdb.py:2317; photos_datetime.py:19-21).
	if added, ok := empty["date_added"].(time.Time); !ok || !added.Equal(time.Unix(0, 0)) {
		t.Errorf("date_added = %v, want the epoch", empty["date_added"])
	}

	shallow := db.AsDict(p, true)
	if _, ok := shallow["album_info"]; ok || shallow["comments"] == nil {
		t.Error("shallow has album_info or lacks comments")
	}
	for _, key := range []string{"keywords", "persons", "path_derivatives"} {
		if l, _ := d[key].([]string); len(l) > 0 {
			l[0] = "changed"
		}
	}
	if p.Keywords[0] != "alpha" || p.Persons[0] != "Person A" || p.PathDerivatives[0] != "/lib/d" {
		t.Error("a write into the dictionary changed the photo")
	}
}

// Adjustments and OriginalAdjustments are two files (photoinfo.py:2212-2214, 2248-2250).
func TestAsDictAdjustments(t *testing.T) {
	t.Parallel()
	lib := t.TempDir()
	p := &photoinfo.PhotoInfo{AdjustmentsPath: filepath.Join(lib, "edited.plist"), OriginalAdjustmentsPath: filepath.Join(lib, "original.aae")}
	adjWrite(t, p.AdjustmentsPath, adjEditor(t, "com.example.edited"))
	adjWrite(t, p.OriginalAdjustmentsPath, adjEditor(t, "com.example.original"))
	d := (&DB{info: Info{LibraryPath: lib}}).AsDict(p, false)
	for key, want := range map[string]string{"adjustments": "com.example.edited", "original_adjustments": "com.example.original"} {
		if m, _ := d[key].(map[string]any); m["editor"] != want {
			t.Errorf("%s = %v, want the editor %s", key, m, want)
		}
	}
}

// An album in a folder gives folders, folder_names and folder_list
// (photoinfo.py:2125; albuminfo.py:293-294).
func TestAsDictFolders(t *testing.T) {
	t.Parallel()
	db, err := Open(t.Context(), testlib.OpenPristine(t, "synthetic"), Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	inFolders := 0
	for _, p := range db.Photos() {
		d := db.AsDict(p, false)
		// Not printed: paths of the machine stay out of the log.
		if d["library"] != db.info.LibraryPath {
			t.Fatal("library is not Info().LibraryPath")
		}
		albums := db.AlbumInfoOf(p)
		elems, _ := d["album_info"].([]any)
		folders, _ := d["folders"].(map[string]any)
		if len(elems) != len(albums) {
			t.Fatalf("album_info has %d albums, AlbumInfoOf %d", len(elems), len(albums))
		}
		last := map[string]any{}
		for i, a := range albums {
			el, _ := elems[i].(map[string]any)
			if !reflect.DeepEqual(el["folder_list"], append([]string{}, a.FolderUUIDs...)) ||
				!reflect.DeepEqual(el["folder_names"], append([]string{}, a.FolderNames...)) {
				t.Errorf("album %d of a photo: folder_list or folder_names is not FolderUUIDs or FolderNames", i)
			}
			if len(a.FolderNames) > 0 {
				inFolders++
				if len(a.FolderUUIDs) != len(a.FolderNames) {
					t.Errorf("album %d of a photo: %d folder UUIDs for %d folder names", i, len(a.FolderUUIDs), len(a.FolderNames))
				}
			}
			last[a.Title] = append([]string{}, a.FolderNames...)
		}
		// A dictionary keeps the value of the last album of a title (photoinfo.py:2125).
		if !reflect.DeepEqual(folders, last) {
			t.Error("folders of a photo is not the folder names of the last album of each title")
		}
	}
	if inFolders == 0 {
		t.Error("no album of a photo of synthetic is in a folder")
	}
}

// The test libraries have no files, so adjustments and path_derivatives are
// expected empty in all of them.
func TestAsDictLibraries(t *testing.T) {
	t.Parallel()
	withFaces := 0
	for _, lib := range goldenLibraries(t) {
		db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		var faces, adjustments, derivatives int
		for _, p := range db.Photos() {
			d := db.AsDict(p, false)
			if l, _ := d["face_info"].([]any); len(l) > 0 {
				faces++
			}
			if m, _ := d["adjustments"].(map[string]any); len(m) > 0 {
				adjustments++
			}
			if l, _ := d["path_derivatives"].([]string); len(l) > 0 {
				derivatives++
			}
		}
		t.Logf("%s: %d photos with face_info, %d with adjustments, %d with path_derivatives", lib.name, faces, adjustments, derivatives)
		withFaces += faces
	}
	if withFaces == 0 {
		t.Error("no photo of any library has face_info")
	}
}

// The bytes are those json.dumps(v, sort_keys=True, indent=indent) of Python
// writes for the same values; osxphotos cannot run here, so they are literals.
func TestJSONEncoder(t *testing.T) {
	t.Parallel()
	nested := map[string]any{"a": []any{map[string]any{"k": []any{1}}}}
	// json.loads gives an int for 1, 2**64 and -0, a float for 1.0 and 1e2
	// (adjustmentsinfo.py:94).
	adjustments := decodeAdjustments(adjFile(t, []byte(`{"adjustments":[{"big":18446744073709551616,"e":1e2,"f":1.0,"i":1,"z":-0}],`+
		`"metadata":{"orientation":1}}`), plist.BinaryFormat)).AsDict()
	for _, tt := range []struct {
		name   string
		v      any
		indent int
		want   string
	}{
		{"separators", map[string]any{
			"a": 1, "b": []any{1, 2}, "c": "Ж", "d": nil, "e": 1.0, "f": time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			"g": time.Date(2020, 1, 2, 3, 4, 5, 123456000, time.FixedZone("", 19800)),
		}, 0, `{"a": 1, "b": [1, 2], "c": "\` + `u0416", "d": null, "e": 1.0, "f": "2020-01-02T03:04:05+00:00", ` +
			`"g": "2020-01-02T03:04:05.123456+05:30"}`},
		{"indent", map[string]any{"b": []string{"s", "t"}, "a": map[string]any{"y": []any{}, "x": map[string]any{}}}, 2,
			"{\n  \"a\": {\n    \"x\": {},\n    \"y\": []\n  },\n  \"b\": [\n    \"s\",\n    \"t\"\n  ]\n}"},
		{"nested, indent 2", nested, 2, "{\n  \"a\": [\n    {\n      \"k\": [\n        1\n      ]\n    }\n  ]\n}"},
		{"nested, indent 4", nested, 4, "{\n    \"a\": [\n        {\n            \"k\": [\n                1\n            ]\n        }\n    ]\n}"},
		{"nested, indent -1", nested, -1, `{"a": [{"k": [1]}]}`},
		{"empty", map[string]any{"b": []string{}, "a": map[string]any{}}, 0, `{"a": {}, "b": []}`},
		{"escapes", "q\"b\\ \n\r\t\b\f\x01\x7f~ \U0001F600 /<>& \U000000e9\x00\U00002028", 0,
			`"q\"b\\ \n\r\t\b\f\` + `u0001\` + `u007f~ \` + `ud83d\` + `ude00 /<>& \` + `u00e9\` + `u0000\` + `u2028"`},
		{"types", map[string]any{
			"big": 1e16, "bytes": []byte("x"), "e15": 1e15, "e22": 1e22, "f32": float32(0.1), "i64": int64(-5), "inf": math.Inf(1),
			"min": 5e-324, "nan": math.NaN(), "negzero": math.Copysign(0, -1), "ninf": math.Inf(-1), "no": false,
			"order": albuminfo.AlbumSortTitle, "tiny": 1e-05, "u64": uint64(math.MaxUint64), "uid": plist.UID(7), "yes": true,
		}, 0, `{"big": 1e+16, "bytes": null, "e15": 1000000000000000.0, "e22": 1e+22, "f32": 0.10000000149011612, "i64": -5, ` +
			`"inf": Infinity, "min": 5e-324, "nan": NaN, "negzero": -0.0, "ninf": -Infinity, "no": false, "order": null, "tiny": 1e-05, ` +
			`"u64": 18446744073709551615, "uid": null, "yes": true}`},
		{"adjustments of JSON data", adjustments, 0, `{"adjustment_format_version": null, ` +
			`"adjustments": [{"big": 18446744073709551616, "e": 100.0, "f": 1.0, "i": 1, "z": 0}], "base_version": 0, "data": null, ` +
			`"editor": "com.example.editor", "format_id": "com.example.format", "format_version": "1.1", "metadata": {"orientation": 1}, ` +
			`"orientation": 1, "timestamp": "2024-05-01T07:30:00+00:00", "version_info": null}`},
	} {
		if got := string(pyJSON(tt.v, tt.indent)); got != tt.want {
			t.Errorf("%s: pyJSON = %s\nwant %s", tt.name, got, tt.want)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("pyJSON of an int32 did not panic")
		}
	}()
	pyJSON(map[string]any{"x": int32(1)}, 0)
}

// photoinfo.py:2279-2285 sorts the lists at the top level only, None as "",
// and not location or a list whose first element is a dict; plistlib gives
// the dates of cloud_metadata naive, the other dates are aware.
func TestJSONDumps(t *testing.T) {
	t.Parallel()
	d := map[string]any{
		"keywords": []string{"b", "a"}, "albums": []any{"b", nil, "a", ""}, "location": []any{"b", "a"},
		"comments": []any{map[string]any{"k": 2}, "b", "a"}, "likes": []any{},
		"folders": map[string]any{"t": []string{"z", "y"}}, "date": time.Date(2020, 1, 2, 3, 4, 5, 0, time.FixedZone("", 19800)),
		"cloud_metadata": map[string]any{
			"d": time.Date(2020, 1, 2, 3, 4, 5, 600, time.FixedZone("", 3600)),
			"l": []any{time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}, "f": float32(0.5),
		},
	}
	want := `{"albums": [null, "", "a", "b"], "cloud_metadata": {"d": "2020-01-02T02:04:05.000001", "f": 0.5, "l": ["2020-01-02T03:04:05"]}, ` +
		`"comments": [{"k": 2}, "b", "a"], "date": "2020-01-02T03:04:05+05:30", "folders": {"t": ["z", "y"]}, "keywords": ["a", "b"], ` +
		`"likes": [], "location": ["b", "a"]}`
	if got := string(dumps(d, 0)); got != want {
		t.Errorf("dumps = %s\nwant %s", got, want)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	lat, lon := 48.8566, 2.3522
	p := &photoinfo.PhotoInfo{UUID: "P", Keywords: []string{"b", "a"}, Latitude: &lat, Longitude: &lon}
	db := &DB{info: Info{LibraryPath: "/lib"}}
	for _, shallow := range []bool{true, false} {
		compact, indented := db.JSON(p, shallow, 0), db.JSON(p, shallow, 2)
		if s := string(compact); !strings.Contains(s, `"keywords": ["a", "b"], `) || !strings.Contains(s, `"location": [48.8566, 2.3522], `) {
			t.Errorf("JSON(shallow %v) = %s, want keywords sorted and location as it is", shallow, s)
		}
		var d, di map[string]any
		if err := json.Unmarshal(compact, &d); err != nil {
			t.Fatalf("JSON(shallow %v) does not decode: %v", shallow, err)
		}
		if err := json.Unmarshal(indented, &di); err != nil || !reflect.DeepEqual(d, di) || !strings.Contains(string(indented), "\n  \"uuid\": \"P\",\n") {
			t.Errorf("JSON(shallow %v, indent 2) = %s, %v; want the same object, indented", shallow, indented, err)
		}
		if len(d) != len(db.AsDict(p, shallow)) {
			t.Errorf("JSON(shallow %v) has %d keys, AsDict %d", shallow, len(d), len(db.AsDict(p, shallow)))
		}
	}
	if p.Keywords[0] != "b" {
		t.Error("JSON sorted the keywords of the photo")
	}
}

// _json_hexdigest drops face_info, whose order is not stable, and visible,
// which follows the Photos UI (photoinfo.py:2299-2311). The expected digest is
// built from the bytes of JSON, not from AsDict as Hexdigest builds it.
func TestHexdigest(t *testing.T) {
	t.Parallel()
	db := &DB{info: Info{LibraryPath: "/lib"}}
	photo := func(title string, visible bool) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: "P", Title: title, Visible: visible, Keywords: []string{"b", "a"}}
	}
	p := photo("Title A", true)
	got := db.Hexdigest(p)
	if len(got) != 40 || strings.Trim(got, "0123456789abcdef") != "" {
		t.Fatalf("Hexdigest = %q, want 40 hex digits", got)
	}
	if db.Hexdigest(p) != got || db.Hexdigest(photo("Title A", false)) != got {
		t.Error("Hexdigest changed with another call or with visible")
	}
	if db.Hexdigest(photo("Title B", true)) == got {
		t.Error("Hexdigest did not change with the title")
	}
	s := strings.Replace(string(db.JSON(p, true, 0)), `"face_info": [], `, "", 1)
	if want := hexdigest(strings.Replace(s, `"visible": true, `, "", 1)); got != want {
		t.Errorf("Hexdigest = %s, want %s: the digest of JSON(p, true, 0) without face_info and visible", got, want)
	}
}

// Every type that AsDict and CloudMetadataOf give passes through the encoder:
// a type without a branch panics. json.Indent lays out the compact bytes as
// the indent of Python does: an item on each line, ": " and {} and [] when
// empty. Only numbers are printed: the bytes hold paths of the machine.
func TestJSONLibraries(t *testing.T) {
	t.Parallel()
	withCloud := 0
	for _, lib := range goldenLibraries(t) {
		db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		var bad, cloud, faces int
		for _, p := range db.Photos() {
			var want bytes.Buffer
			if json.Indent(&want, db.JSON(p, false, 0), "", "  ") != nil || !bytes.Equal(want.Bytes(), db.JSON(p, false, 2)) {
				bad++
			}
			if len(db.CloudMetadataOf(p)) > 0 {
				cloud++
			}
			if len(db.FaceInfoOf(p)) > 0 {
				faces++
			}
		}
		n := len(db.Photos())
		t.Logf("%s: %d photos, %d with cloud_metadata, %d with face_info", lib.name, n, cloud, faces)
		if bad > 0 {
			t.Errorf("%s: JSON of %d of %d photos is not valid or not laid out as json.Indent lays it out", lib.name, bad, n)
		}
		withCloud += cloud
	}
	if withCloud == 0 {
		t.Error("no photo of any library has cloud_metadata: the integers, bytes and UIDs of a property list reach no encoder")
	}
}
