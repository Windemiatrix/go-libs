package photosdb

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

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
