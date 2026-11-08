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

func TestAsDict(t *testing.T) {
	t.Parallel()
	lat, lon := 48.8566, 2.3522
	p := &photoinfo.PhotoInfo{UUID: "P", Filename: "IMG_0001.HEIC", Latitude: &lat, Longitude: &lon, Keywords: []string{"alpha"}}
	comment := photoinfo.CommentInfo{User: "Person A", Text: "Comment 1"}
	db := &DB{comments: map[string][]photoinfo.CommentInfo{"P": {comment}}}
	d := db.AsDict(p, false)
	for key, want := range map[string]any{
		"filename": "IMG_0001.HEIC", "title": nil, "owner": nil, "tzoffset": nil, "rating": 0,
		"media_analysis": nil, "ai_caption": nil, "location": []any{48.8566, 2.3522},
		"albums": []any{}, "burst_albums": []any{}, "keywords": []string{"alpha"}, "path_derivatives": []string{},
		"burst_photos": []string{}, "face_info": []any{}, "folders": map[string]any{}, "imported_by": []any{nil, nil},
		"place": map[string]any{}, "import_info": map[string]any{}, "cloud_metadata": map[string]any{},
		"adjustments": map[string]any{}, "original_adjustments": map[string]any{},
		"comments": []any{comment.AsDict()},
	} {
		if !reflect.DeepEqual(d[key], want) {
			t.Errorf("%s = %#v, want %#v", key, d[key], want)
		}
	}
	// DEFAULT_DATETIME_LOCAL for a photo without the date (photosdb/photosdb.py:2317).
	if added, ok := d["date_added"].(time.Time); !ok || !added.Equal(time.Unix(0, 0)) {
		t.Errorf("date_added = %v, want the epoch", d["date_added"])
	}
	shallow := db.AsDict(p, true)
	if _, ok := shallow["album_info"]; ok || shallow["comments"] == nil {
		t.Error("shallow has album_info or lacks comments")
	}
	if keywords, _ := d["keywords"].([]string); len(keywords) > 0 {
		keywords[0] = "beta"
	}
	if p.Keywords[0] != "alpha" {
		t.Error("a write into the dictionary changed the photo")
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
		albums := db.AlbumInfoOf(p)
		elems, _ := d["album_info"].([]any)
		folders, _ := d["folders"].(map[string]any)
		if len(elems) != len(albums) {
			t.Fatalf("album_info has %d albums, AlbumInfoOf %d", len(elems), len(albums))
		}
		for i, a := range albums {
			el, _ := elems[i].(map[string]any)
			if !reflect.DeepEqual(el["folder_list"], append([]string{}, a.FolderUUIDs...)) ||
				!reflect.DeepEqual(el["folder_names"], append([]string{}, a.FolderNames...)) {
				t.Errorf("album %d of a photo: folder_list or folder_names is not FolderUUIDs or FolderNames", i)
			}
			if _, ok := folders[a.Title]; !ok {
				t.Errorf("album %d of a photo: its title is not a key of folders", i)
			}
			if len(a.FolderNames) > 0 {
				inFolders++
			}
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
