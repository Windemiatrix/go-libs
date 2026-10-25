package golden

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const (
	selectedFrame   = "00000000-0000-0000-0000-000000000001"
	unselectedFrame = "00000000-0000-0000-0000-000000000002"
	emptyPhoto      = "00000000-0000-0000-0000-000000000003"

	albumAInFolderX    = "00000000-0000-0000-0000-0000000000a1"
	albumTrashed       = "00000000-0000-0000-0000-0000000000a2"
	albumAInFolderY    = "00000000-0000-0000-0000-0000000000a3"
	albumBNoFolder     = "00000000-0000-0000-0000-0000000000a5"
	albumOfNoPhoto     = "00000000-0000-0000-0000-0000000000a9"
	albumSharedS       = "00000000-0000-0000-0000-0000000000c1"
	albumSharedTitledA = "00000000-0000-0000-0000-0000000000c2"
	folderX            = "00000000-0000-0000-0000-0000000000f1"
	folderY            = "00000000-0000-0000-0000-0000000000f2"
)

func ptr[T any](v T) *T { return &v }

func utc(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
	return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
}

// Every call returns new values, so that a test may spoil them.
func fixtureLib() Library {
	ist := time.FixedZone("IST", 19800)
	return Library{
		Photos: []*photoinfo.PhotoInfo{
			{
				UUID:            selectedFrame,
				CloudMasterGUID: "CLOUD-GUID-1", Fingerprint: "FP-1",
				Filename: "IMG_0001.HEIC", OriginalFilename: "orig_0001.heic",
				Date:     utc(2020, 6, 6, 14, 15, 24, 725564000).In(ist),
				TZOffset: ptr(19800), DateAdded: ptr(utc(2021, 1, 2, 3, 4, 5, 0).In(ist)),
				DateModified: ptr(utc(2021, 2, 3, 4, 5, 6, 500000000)), DateTrashed: ptr(utc(2022, 3, 4, 5, 6, 7, 0)),
				Width: 4032, Height: 3024, Orientation: 6,
				OriginalWidth: 4032, OriginalHeight: 3024, OriginalOrientation: 1, OriginalFilesize: 1195350,
				UTI: "public.heic", UTIOriginal: "public.heic", UTIRaw: "com.adobe.raw-image", UTIEdited: "public.heic",
				IsPhoto: true, Live: true, HasRAW: true, Screenshot: true,
				SlowMo: true, TimeLapse: true, ScreenRecording: true,
				HDR: true, Panorama: true, Portrait: true, Selfie: true, Visible: true, Spatial: 1,
				TZName: "Asia/Kolkata", CloudOwnerHashedID: "OWNER-1", SharedMoment: true, ExternalEdit: true,
				AdjustmentType: ptr(0), ImportedByName: "Camera", ImportedByBundleID: "com.example.camera",
				Burst: true, BurstSelected: true, BurstUUID: "BURST-A", BurstPickType: 12, BurstDefaultPick: true,
				Hidden: true, Favorite: true,
				Shared: true, Syndicated: true, SavedToLibrary: true, SharedLibrary: true,
				InCloud: ptr(true), IsMissing: true, IsCloudAsset: true, IsReference: true,
				Title: "Title A", Description: "Desc", Keywords: []string{"alpha", "beta"}, HasPersons: true,
				Latitude: ptr(48.8566), Longitude: ptr(2.3522),
				AlbumUUIDs:      []string{albumAInFolderY, albumTrashed, albumAInFolderX},
				BurstAlbumUUIDs: []string{albumBNoFolder, albumAInFolderX, albumAInFolderY},
			},
			{UUID: unselectedFrame, IsPhoto: true, Burst: true, BurstUUID: "BURST-A", BurstPickType: 2},
			{
				UUID: emptyPhoto, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG",
				Date: time.Unix(0, 0), UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true,
			},
		},
		Albums: []*albuminfo.AlbumInfo{
			{
				UUID: albumAInFolderX, Title: "Album A", ParentUUID: folderX, FolderNames: []string{"Folder X"},
				CreationDate: ptr(utc(2020, 1, 2, 3, 4, 5, 0)), StartDate: ptr(utc(2019, 5, 6, 7, 8, 9, 0)),
			},
			{UUID: albumTrashed, Title: "Album T", InTrash: true},
			{
				UUID: albumAInFolderY, Title: "Album A", ParentUUID: folderY, FolderNames: []string{"Folder Y"},
				EndDate: ptr(utc(2020, 12, 31, 23, 59, 59, 500000000)),
			},
			{UUID: albumBNoFolder, Title: "Album B", CreationDate: ptr(utc(2020, 2, 2, 2, 2, 2, 0))},
			{UUID: albumOfNoPhoto, Title: "Unused"},
		},
	}
}

// Arguments are JSON values: "null", `"Album A"`, `["Folder X"]`.
func albumJSON(uuid, title, parent, folderNames, creation, start, end, owner string) string {
	return fmt.Sprintf(`{"creation_date": %s, "end_date": %s, "folder_list": [], "folder_names": %s, "owner": %s, `+
		`"parent": %s, "sort_order": null, "start_date": %s, "title": %s, "uuid": %q}`,
		creation, end, folderNames, owner, parent, start, title, uuid)
}

var (
	elemAInFolderX = albumJSON(albumAInFolderX, `"Album A"`, `"`+folderX+`"`, `["Folder X"]`,
		`"2020-01-02T03:04:05+00:00"`, `"2019-05-06T07:08:09+00:00"`, "null", "null")
	elemAInFolderY = albumJSON(albumAInFolderY, `"Album A"`, `"`+folderY+`"`, `["Folder Y"]`,
		`"2001-01-01T00:00:00+00:00"`, "null", `"2020-12-31T23:59:59.500000+00:00"`, "null")
	elemBNoFolder = albumJSON(albumBNoFolder, `"Album B"`, "null", "[]",
		`"2020-02-02T02:02:02+00:00"`, "null", "null", "null")
	elemSharedS = albumJSON(albumSharedS, `"Shared S"`, "null", `["Shared Folder"]`,
		`"2020-03-03T03:03:03+00:00"`, "null", "null", `"Owner A"`)
	elemSharedTitledA = albumJSON(albumSharedTitledA, `"Album A"`, "null", `["Shared Folder"]`,
		`"2020-03-03T03:03:03+00:00"`, "null", "null", `"Owner A"`)
)

// The reference has shared albums that the Go side does not read.
func refSelectedFrame() map[string]string {
	return map[string]string{
		"album_info":           "[" + strings.Join([]string{elemAInFolderX, elemAInFolderY, elemSharedS, elemSharedTitledA}, ", ") + "]",
		"albums":               `["Album A", "Shared S"]`,
		"burst":                "true",
		"burst_album_info":     "[" + strings.Join([]string{elemBNoFolder, elemAInFolderX, elemAInFolderY, elemSharedS}, ", ") + "]",
		"burst_albums":         `["Album A", "Album B", "Shared S"]`,
		"burst_default_pick":   "true",
		"burst_key":            "false",
		"burst_photos":         `["` + unselectedFrame + `"]`,
		"burst_selected":       "true",
		"cloud_guid":           `"CLOUD-GUID-1"`,
		"date":                 `"2020-06-06T19:45:24.725564+05:30"`,
		"date_added":           `"2021-01-02T03:04:05+00:00"`,
		"date_modified":        `"2021-02-03T04:05:06.500000+00:00"`,
		"date_trashed":         `"2022-03-04T05:06:07+00:00"`,
		"description":          `"Desc"`,
		"favorite":             "true",
		"filename":             `"IMG_0001.HEIC"`,
		"fingerprint":          `"FP-1"`,
		"folders":              `{"Album A": ["Folder Y"], "Shared S": ["Shared Folder"]}`,
		"has_raw":              "true",
		"hasadjustments":       "false",
		"height":               "3024",
		"hidden":               "true",
		"incloud":              "true",
		"intrash":              "false",
		"iscloudasset":         "true",
		"ismissing":            "true",
		"ismovie":              "false",
		"isphoto":              "true",
		"israw":                "false",
		"isreference":          "true",
		"keywords":             `["alpha", "beta"]`,
		"latitude":             "48.8566",
		"live_photo":           "true",
		"location":             "[48.8566, 2.3522]",
		"longitude":            "2.3522",
		"orientation":          "6",
		"original_filename":    `"orig_0001.heic"`,
		"original_filesize":    "1195350",
		"original_height":      "3024",
		"original_orientation": "1",
		"original_width":       "4032",
		"persons":              `["Person A", "_UNKNOWN_"]`,
		"raw_original":         "false",
		"saved_to_library":     "true",
		"screen_recording":     "true",
		"screenshot":           "true",
		"shared":               "true",
		"shared_library":       "true",
		"slow_mo":              "true",
		"syndicated":           "true",
		"time_lapse":           "true",
		"title":                `"Title A"`,
		"tzoffset":             "19800",
		"uti":                  `"public.heic"`,
		"uti_edited":           `"public.heic"`,
		"uti_original":         `"public.heic"`,
		"uti_raw":              `"com.adobe.raw-image"`,
		"uuid":                 `"` + selectedFrame + `"`,
		"width":                "4032",

		"adjustment_type":       "0",
		"cloud_owner_hashed_id": `"OWNER-1"`,
		"external_edit":         "true",
		"hdr":                   "true",
		"imported_by":           `["Camera", "com.example.camera"]`,
		"panorama":              "true",
		"portrait":              "true",
		"selfie":                "true",
		"shared_moment":         "true",
		"spatial":               "1",
		"tzname":                `"Asia/Kolkata"`,
		"visible":               "true",
	}
}

func refEmptyPhoto() map[string]string {
	return map[string]string{
		"album_info": "[]", "albums": "[]", "burst": "false", "burst_album_info": "[]", "burst_albums": "[]",
		"burst_default_pick": "false", "burst_key": "false", "burst_photos": "[]", "burst_selected": "false",
		"cloud_guid": "null", "date": `"1970-01-01T00:00:00+00:00"`, "date_added": `"1970-01-01T00:00:00+00:00"`,
		"date_modified": "null", "date_trashed": "null", "description": "null", "favorite": "false",
		"filename": `"IMG_0003.JPG"`, "fingerprint": "null", "folders": "{}", "has_raw": "false", "hasadjustments": "false",
		"height": "null", "hidden": "false", "incloud": "null", "intrash": "false", "iscloudasset": "false",
		"ismissing": "false", "ismovie": "false", "isphoto": "true", "israw": "false", "isreference": "false",
		"keywords": "[]", "latitude": "null", "live_photo": "false", "location": "[null, null]", "longitude": "null",
		"orientation": "null", "original_filename": `"IMG_0003.JPG"`, "original_filesize": "null",
		"original_height": "null", "original_orientation": "null", "original_width": "null", "persons": "[]",
		"raw_original": "false", "saved_to_library": "false", "screenshot": "false", "shared": "false",
		"shared_library": "false", "syndicated": "false", "title": "null", "tzoffset": "null",
		"screen_recording": "false", "slow_mo": "false", "time_lapse": "false", "uti_edited": "null",
		"uti": `"public.jpeg"`, "uti_original": `"public.jpeg"`, "uti_raw": "null", "uuid": `"` + emptyPhoto + `"`, "width": "null",
		"adjustment_type": "null", "cloud_owner_hashed_id": "null", "external_edit": "false", "hdr": "false",
		"imported_by": "[null, null]", "panorama": "false", "portrait": "false", "selfie": "false",
		"shared_moment": "false", "spatial": "0", "tzname": "null", "visible": "false",
	}
}

func refObj(base map[string]string, overrides ...map[string]string) Object {
	merged := maps.Clone(base)
	for _, o := range overrides {
		maps.Copy(merged, o)
	}
	obj := make(Object, len(merged))
	for k, v := range merged {
		obj[k] = json.RawMessage(v)
	}
	return obj
}

func refOf(query ...Object) *Reference {
	return &Reference{Library: "lib", Query: query}
}

// unselectedFrame is not in the reference: osxphotos does not return it.
func fixtureRef() *Reference { return refOf(refObj(refSelectedFrame()), refObj(refEmptyPhoto())) }

func everyKey() []string { return slices.Concat(PhotoKeys(), AlbumKeys()) }

func compare(t *testing.T, ref *Reference, lib Library, divs ...Divergence) *Report {
	t.Helper()
	rep, err := Compare(ref, lib, everyKey(), divs)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	return rep
}

func diffKeys(rep *Report) []string {
	set := map[string]bool{}
	for _, d := range rep.Diffs {
		set[d.Field] = true
	}
	return slices.Sorted(maps.Keys(set))
}

func fieldsTxt(t testing.TB) []string {
	t.Helper()
	keys, err := ReadFields(filepath.Join(testlib.RepoDir(t, "golden"), "fields.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonStrings(list []string) string {
	b, _ := json.Marshal(list)
	return string(b)
}
