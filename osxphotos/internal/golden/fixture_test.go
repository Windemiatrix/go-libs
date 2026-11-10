package golden

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
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

	importSession = "00000000-0000-0000-0000-0000000000e5"
	importBare    = "00000000-0000-0000-0000-0000000000e6"

	projectP       = "00000000-0000-0000-0000-0000000000a6"
	projectTrashed = "00000000-0000-0000-0000-0000000000a8"

	personA       = "00000000-0000-0000-0000-0000000000d5"
	personUnknown = "00000000-0000-0000-0000-0000000000d6"
	faceA         = "00000000-0000-0000-0000-0000000000b9"
)

func ptr[T any](v T) *T { return &v }

// A photo without a row: every score 0.0, not None (photoinfo.py:1507-1537).
func noScoreRow() scoreinfo.ScoreInfo {
	var s scoreinfo.ScoreInfo
	fields := reflect.ValueOf(&s).Elem()
	for i := range fields.NumField() {
		fields.Field(i).Set(reflect.ValueOf(new(float64)))
	}
	return s
}

func utc(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
	return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
}

// goLib is the Go side of the fixture as typed values, so that a test spoils
// a field and not a key; lib builds Library.Dicts from it as
// photosdb.DB.AsDict(p, false) does, for the keys of fields.txt.
type goLib struct {
	Photos        []*photoinfo.PhotoInfo
	Albums        []*albuminfo.AlbumInfo // user albums, trashed ones included
	SharedAlbums  []*albuminfo.AlbumInfo // shared albums, trashed ones included
	BurstAlbums   map[string][]string    // by photo UUID: the album UUIDs of burst_album_info
	Imports       []*albuminfo.ImportInfo
	Projects      []*albuminfo.AlbumInfo // trashed ones included
	Comments      map[string][]photoinfo.CommentInfo
	Likes         map[string][]photoinfo.LikeInfo
	Persons       map[string][]personinfo.PersonInfo
	Faces         map[string][]personinfo.FaceInfo
	CloudMetadata map[string]map[string]any
	SearchInfo    map[string]searchinfo.SearchInfo
	Override      map[string]map[string]any // by photo UUID: values set over the dictionary, for the keys no field gives
}

func (l goLib) lib() Library {
	dicts := make(map[string]map[string]any, len(l.Photos))
	for _, p := range l.Photos {
		dicts[p.UUID] = l.dict(p)
	}
	return Library{Photos: l.Photos, Dicts: dicts}
}

// As photosdb.DB.AlbumInfoOf: albums outside the trash, by title, then UUID.
func (l goLib) albumsOf(uuids []string, from ...[]*albuminfo.AlbumInfo) []*albuminfo.AlbumInfo {
	var out []*albuminfo.AlbumInfo
	for _, id := range uuids {
		for _, a := range slices.Concat(from...) {
			if a.UUID == id && !a.InTrash {
				out = append(out, a)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b *albuminfo.AlbumInfo) int {
		return cmp.Or(strings.Compare(a.Title, b.Title), strings.Compare(a.UUID, b.UUID))
	})
	return out
}

func titlesOf(albums []*albuminfo.AlbumInfo) []any {
	var titles []string
	for _, a := range albums {
		titles = append(titles, a.Title)
	}
	slices.Sort(titles)
	out := []any{}
	for _, s := range slices.Compact(titles) {
		out = append(out, none(s))
	}
	return out
}

func dictsOf[T any](items []T, asDict func(T) map[string]any) []any {
	out := []any{}
	for _, v := range items {
		out = append(out, asDict(v))
	}
	return out
}

func none(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orNone[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func (l goLib) dict(p *photoinfo.PhotoInfo) map[string]any {
	albums := l.albumsOf(p.AlbumUUIDs, l.Albums, l.SharedAlbums)
	burstAlbums := l.albumsOf(l.BurstAlbums[p.UUID], l.Albums, l.SharedAlbums)
	folders := map[string]any{}
	for _, a := range albums {
		folders[a.Title] = append([]string{}, a.FolderNames...)
	}
	place := map[string]any{}
	if p.Place != nil {
		place = p.Place.AsDict()
	}
	importInfo := map[string]any{}
	for _, s := range l.Imports {
		if p.ImportUUID != "" && s.UUID == p.ImportUUID {
			importInfo = s.AsDict()
		}
	}
	burst := []string{}
	for _, f := range l.Photos {
		if p.Burst && f.Burst && f.BurstUUID == p.BurstUUID && f.UUID != p.UUID {
			burst = append(burst, f.UUID)
		}
	}
	cloud := l.CloudMetadata[p.UUID]
	if cloud == nil {
		cloud = map[string]any{}
	}
	search := l.SearchInfo[p.UUID]
	d := map[string]any{
		"album_info": dictsOf(albums, (*albuminfo.AlbumInfo).AsDict), "albums": titlesOf(albums),
		"burst_album_info": dictsOf(burstAlbums, (*albuminfo.AlbumInfo).AsDict), "burst_albums": titlesOf(burstAlbums),
		"folders": folders, "import_info": importInfo,
		"project_info": dictsOf(l.albumsOf(p.ProjectUUIDs, l.Projects), (*albuminfo.AlbumInfo).AsDict),
		"comments":     dictsOf(l.Comments[p.UUID], photoinfo.CommentInfo.AsDict),
		"likes":        dictsOf(l.Likes[p.UUID], photoinfo.LikeInfo.AsDict),
		"person_info":  dictsOf(l.Persons[p.UUID], personinfo.PersonInfo.AsDict),
		"face_info": dictsOf(l.Faces[p.UUID], func(f personinfo.FaceInfo) map[string]any {
			return f.AsDict(p.Width, p.Height, p.Orientation)
		}),
		"cloud_metadata": cloud, "labels": search.Labels(), "labels_normalized": search.Normalized().Labels(),
		"search_info": search.AsDict(), "search_info_normalized": search.Normalized().AsDict(),
		"burst_photos": burst, "persons": p.Persons, "keywords": p.Keywords, "path_derivatives": p.PathDerivatives,
		"location": []any{orNone(p.Latitude), orNone(p.Longitude)}, "orientation": p.Orientation,
		"incloud": orNone(p.InCloud), "tzoffset": orNone(p.TZOffset), "latitude": orNone(p.Latitude),
		"longitude": orNone(p.Longitude), "adjustment_type": orNone(p.AdjustmentType),
		"imported_by": []any{none(p.ImportedByName), none(p.ImportedByBundleID)},
		"adjustments": map[string]any{}, "original_adjustments": map[string]any{},
		"date": p.Date, "date_original": p.DateOriginal, "exif_info": p.ExifInfo.AsDict(), "score": p.Score.AsDict(), "place": place,
		"date_added": *cmp.Or(p.DateAdded, &defaultDateTimeLocal), "date_modified": orNone(p.DateModified),
		"date_trashed": orNone(p.DateTrashed),

		"burst": p.Burst, "burst_default_pick": p.BurstDefaultPick, "burst_key": p.BurstKey, "burst_selected": p.BurstSelected,
		"favorite": p.Favorite, "hidden": p.Hidden, "intrash": p.InTrash, "isphoto": p.IsPhoto, "ismovie": p.IsMovie,
		"live_photo": p.Live, "screenshot": p.Screenshot, "screen_recording": p.ScreenRecording, "slow_mo": p.SlowMo,
		"time_lapse": p.TimeLapse, "isreference": p.IsReference, "hasadjustments": p.HasAdjustments, "has_raw": p.HasRAW,
		"raw_original": p.RawOriginal, "ismissing": p.IsMissing, "iscloudasset": p.IsCloudAsset, "israw": p.IsRAW,
		"shared": p.Shared, "shared_library": p.SharedLibrary, "syndicated": p.Syndicated, "saved_to_library": p.SavedToLibrary,
		"hdr": p.HDR, "panorama": p.Panorama, "portrait": p.Portrait, "selfie": p.Selfie, "visible": p.Visible,
		"shared_moment": p.SharedMoment, "external_edit": p.ExternalEdit,

		"uuid": p.UUID, "filename": p.Filename, "original_filename": p.OriginalFilename, "uti": p.UTI, "uti_original": p.UTIOriginal,
		"uti_edited": none(p.UTIEdited), "uti_raw": none(p.UTIRaw), "cloud_guid": none(p.CloudMasterGUID),
		"description": none(p.Description), "fingerprint": none(p.Fingerprint), "title": none(p.Title), "tzname": none(p.TZName),
		"cloud_owner_hashed_id": none(p.CloudOwnerHashedID), "owner": none(p.Owner),

		"width": p.Width, "height": p.Height, "original_width": p.OriginalWidth, "original_height": p.OriginalHeight,
		"original_orientation": p.OriginalOrientation, "original_filesize": p.OriginalFilesize, "spatial": p.Spatial, "rating": 0,
	}
	maps.Copy(d, l.Override[p.UUID])
	return d
}

// Every call returns new values, so that a test may spoil them.
func fixtureLib() goLib {
	ist := time.FixedZone("IST", 19800)
	return goLib{
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
				Title: "Title A", Description: "Desc", Keywords: []string{"alpha", "beta"}, Persons: []string{"Person A", "_UNKNOWN_"},
				Latitude: ptr(48.8566), Longitude: ptr(2.3522),
				AlbumUUIDs:   []string{albumAInFolderY, albumTrashed, albumAInFolderX, albumSharedS, albumSharedTitledA},
				ProjectUUIDs: []string{projectP, projectTrashed},
				ImportUUID:   importSession,
				Owner:        "Photo Owner",
				DateOriginal: utc(2020, 6, 6, 10, 0, 0, 0).In(ist),
				ExifInfo: exifinfo.ExifInfo{
					FlashFired: ptr(true), ISO: ptr(100), MeteringMode: ptr(5), SampleRate: ptr(48000), TrackFormat: ptr(1), WhiteBalance: ptr(0),
					Aperture: ptr(1.8), BitRate: ptr(1000.5), Duration: ptr(12.25), ExposureBias: ptr(-0.5), FocalLength: ptr(4.25), FPS: ptr(29.97),
					Latitude: ptr(48.8566), Longitude: ptr(2.3522), ShutterSpeed: ptr(0.008),
					CameraMake: "Acme", CameraModel: "Acme One", Codec: "hvc1", LensModel: "Acme One 4mm",
					Date: ptr(utc(2020, 6, 6, 10, 0, 0, 0).In(ist)), TZOffset: ptr(19800), TZName: "Asia/Kolkata",
				},
				// Behavioral and Interaction are null in refScore, as from Photos 11 on.
				Score: scoreinfo.ScoreInfo{
					Overall: ptr(0.01), Curation: ptr(0.02), Promotion: ptr(0.03), HighlightVisibility: ptr(0.04), Failure: ptr(0.06),
					HarmoniousColor: ptr(0.07), Immersiveness: ptr(0.08), InterestingSubject: ptr(0.1), IntrusiveObjectPresence: ptr(0.11),
					LivelyColor: ptr(0.12), LowLight: ptr(0.13), Noise: ptr(0.14), PleasantCameraTilt: ptr(0.15), PleasantComposition: ptr(0.16),
					PleasantLighting: ptr(0.17), PleasantPattern: ptr(0.18), PleasantPerspective: ptr(0.19), PleasantPostProcessing: ptr(0.2),
					PleasantReflection: ptr(0.21), PleasantSymmetry: ptr(0.22), SharplyFocusedSubject: ptr(0.23), TastefullyBlurred: ptr(0.24),
					WellChosenSubject: ptr(0.25), WellFramedSubject: ptr(0.26), WellTimedShot: ptr(0.27),
				},
				Place: &placeinfo.PlaceInfo{
					Name: "Example Park, Townsville, Exampleland",
					Names: placeinfo.Names{
						Country: []string{"Exampleland"}, City: []string{"Townsville", "Greater Townsville"},
						AreaOfInterest: []string{"Example Park"}, Field13: []string{"Field Thirteen"},
					},
					CountryCode: "EX", IsHome: true, AddressStr: "1 Example Street, Townsville",
					Address: placeinfo.Address{Street: "1 Example Street", City: "Townsville", Country: "Exampleland", ISOCountryCode: "EX"},
				},
			},
			{UUID: unselectedFrame, IsPhoto: true, Burst: true, BurstUUID: "BURST-A", BurstPickType: 2},
			{
				UUID: emptyPhoto, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG",
				Date: time.Unix(0, 0).UTC(), DateOriginal: time.Unix(0, 0).UTC(), UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true,
				Score: noScoreRow(),
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
		SharedAlbums: []*albuminfo.AlbumInfo{
			{UUID: albumSharedS, Title: "Shared S", Owner: "Owner A", CreationDate: ptr(utc(2020, 3, 3, 3, 3, 3, 0))},
			{UUID: albumSharedTitledA, Title: "Album A", Owner: "Owner A", CreationDate: ptr(utc(2020, 3, 3, 3, 3, 3, 0))},
		},
		BurstAlbums: map[string][]string{selectedFrame: {albumBNoFolder, albumAInFolderX, albumAInFolderY, albumSharedS}},
		Imports: []*albuminfo.ImportInfo{
			{
				UUID: importSession, Title: "Import 1", CreationDate: ptr(utc(2020, 6, 7, 1, 2, 3, 0)),
				StartDate: ptr(utc(2020, 6, 7, 1, 0, 0, 0)), EndDate: ptr(utc(2020, 6, 7, 1, 1, 0, 500000000)),
			},
			{UUID: importBare},
		},
		Projects: []*albuminfo.AlbumInfo{
			{UUID: projectP, Title: "Project P", ParentUUID: folderX, CreationDate: ptr(utc(2020, 4, 4, 4, 4, 4, 0))},
			{UUID: projectTrashed, Title: "Project T", InTrash: true},
		},
		Comments: map[string][]photoinfo.CommentInfo{selectedFrame: {
			{Datetime: ptr(utc(2020, 7, 1, 0, 0, 0, 0)), User: "Person A", IsMine: true, Text: "Comment 1"},
			{Datetime: ptr(utc(2020, 7, 2, 0, 0, 0, 500000000).In(ist)), Text: "Comment 2"},
		}},
		Likes: map[string][]photoinfo.LikeInfo{selectedFrame: {{Datetime: ptr(utc(2020, 7, 3, 0, 0, 0, 0)), User: "Person A"}}},
		// By name, as PersonInfoOf gives them: the rule compares in order.
		Persons: map[string][]personinfo.PersonInfo{selectedFrame: {
			{UUID: personA, Name: "Person A", DisplayName: "Pat", KeyFace: 11, FaceCount: 3, KeyPhotoUUID: selectedFrame, Favorite: true, SortOrder: 2, FeatureLess: true},
			{UUID: personUnknown, Name: "_UNKNOWN_"},
		}},
		// Size 0 and the center keep the values of FaceInfo.AsDict the same for
		// the other widths, heights and orientations 6 to 8 that tests set.
		Faces: map[string][]personinfo.FaceInfo{selectedFrame: {{
			PK: 21, UUID: faceA, Name: "Person A", AssetUUID: selectedFrame, PersonPK: 31, PersonUUID: personA,
			CenterX: 0.5, CenterY: 0.5, Quality: 0.5, Roll: 0.25, SourceWidth: 4032, SourceHeight: 3024, HasSmile: true, AgeType: 2,
			DetectionType: 1, BodyCenterX: 0.25, BodyCenterY: 0.75, BodyWidth: 0.5, BodyHeight: 0.125,
		}}},
		// Types as howett.net/plist decodes them: a non-negative integer is a uint64, a negative one an int64.
		CloudMetadata: map[string]map[string]any{selectedFrame: {
			"mediaType": "image",
			"count":     uint64(3),
			"big":       uint64(9007199254740993),
			"ratio":     0.5,
			"flag":      true,
			"when":      utc(2020, 1, 2, 3, 4, 5, 6000),
			"nested":    map[string]any{"list": []any{int64(-1), "x"}},
		}},
		SearchInfo: map[string]searchinfo.SearchInfo{selectedFrame: searchinfo.New(searchRecords())},
	}
}

// The labels are out of order, so that only a sorted side matches the
// reference. Index 0 is the label Dog, 1 the city, 3 the holiday.
func searchRecords() []searchinfo.Record {
	return []searchinfo.Record{
		{Category: searchinfo.CategoryLabel, Content: "Dog", Normalized: "dog"},
		{Category: searchinfo.CategoryCity, Content: "Townsville", Normalized: "townsville"},
		{Category: searchinfo.CategoryLabel, Content: "Beach", Normalized: "beach"},
		{Category: searchinfo.CategoryHoliday, Content: "New Year", Normalized: "new year"},
	}
}

// The asdict of a SearchInfo without terms (searchinfo.py:347-394), as json.dumps
// writes it with sorted keys.
const refSearchInfoEmpty = `{"activities": [], "age_groups": [], "areas_of_interest": [], "bodies_of_water": [], "camera": "", ` +
	`"captured_by_me": "", "city": "", "continent": "", "country": "", "country_code": "", "day_of_week": "", "detected_text": [], ` +
	`"document_types": [], "event_performers": [], "event_types": [], "events": [], "file_type": "", "holidays": [], "home": "", ` +
	`"human_actions": [], "id_document_names": [], "id_document_types": [], "labels": [], "landmarks": [], "locality_names": [], ` +
	`"media_types": [], "month": "", "neighborhoods": [], "pets": [], "photographic_style": "", "place_names": [], "region": "", ` +
	`"season": "", "sounds": [], "source": "", "state": "", "state_abbreviation": "", "streets": [], "times_of_day": [], "trip": "", ` +
	`"venue_types": [], "venues": [], "week_part": "", "year": ""}`

func refSearchInfo(normalized bool) string {
	labels, city, holiday := `"Beach", "Dog"`, "Townsville", "New Year"
	if normalized {
		labels, city, holiday = `"beach", "dog"`, "townsville", "new year"
	}
	return strings.NewReplacer(`"labels": []`, `"labels": [`+labels+`]`, `"city": ""`, `"city": "`+city+`"`,
		`"holidays": []`, `"holidays": ["`+holiday+`"]`).Replace(refSearchInfoEmpty)
}

// The integer above 2^53 and the date with microseconds are what a float64 and a rounded date would change.
const refCloudMetadata = `{"big": 9007199254740993, "count": 3, "flag": true, "mediaType": "image", "nested": {"list": [-1, "x"]}, ` +
	`"ratio": 0.5, "when": "2020-01-02T03:04:05.000006"}`

const refPersonInfo = `[{"displayname": "Pat", "facecount": 3, "favorite": true, "feature_less": true, "keyface": 11, "keyphoto": "` + selectedFrame +
	`", "name": "Person A", "sort_order": 2, "uuid": "` + personA + `"}, ` +
	`{"displayname": null, "facecount": null, "favorite": false, "feature_less": false, "keyface": null, "keyphoto": null, ` +
	`"name": "_UNKNOWN_", "sort_order": null, "uuid": "` + personUnknown + `"}]`

// The face of the fixture as personinfo.py:455-496 computes it for a photo of
// 4032 x 3024 in orientation 6.
const refFaceInfo = `[{"_person_pk": 31, "_pk": 21, "age_type": 2, "asset_uuid": "` + selectedFrame + `", ` +
	`"body_center_x": 0.25, "body_center_y": 0.75, "body_height": 0.125, "body_width": 0.5, "center": [1512, 2016], ` +
	`"center_x": 0.5, "center_y": 0.5, "detection_type": 1, "eye_makeup_type": 0, "eye_state": 0, "face_rect": [[1512.0, 2016.0], [1512.0, 2016.0]], ` +
	`"face_type": null, "facial_hair_type": 0, "gender_type": 0, "glasses_type": 0, "hair_color_type": 0, "has_smile": 1, ` +
	`"intrash": 0, "is_pet": false, "lip_makeup_type": 0, "manual": 0, "mpri_reg_rect": {"h": 0.0, "w": 0.0, "x": 0.5, "y": 0.5}, ` +
	`"mwg_rs_area": {"h": 0.0, "w": 0.0, "x": 0.5, "y": 0.5}, "name": "Person A", "pitch": 0, "quality": 0.5, "roll": 0.25, ` +
	`"size": 0.0, "smile_type": 0, "source_height": 3024, "source_width": 4032, "uuid": "` + faceA + `", "yaw": 0}]`

const (
	refComments = `[{"datetime": "2020-07-01T00:00:00+00:00", "ismine": true, "text": "Comment 1", "user": "Person A"}, ` +
		`{"datetime": "2020-07-02T05:30:00.500000+05:30", "ismine": false, "text": "Comment 2", "user": null}]`
	refLikes = `[{"datetime": "2020-07-03T00:00:00+00:00", "ismine": false, "user": "Person A"}]`
)

const (
	refExif = `{"flash_fired": true, "iso": 100, "metering_mode": 5, "sample_rate": 48000, "track_format": 1, "white_balance": 0, ` +
		`"aperture": 1.8, "bit_rate": 1000.5, "duration": 12.25, "exposure_bias": -0.5, "focal_length": 4.25, "fps": 29.97, ` +
		`"latitude": 48.8566, "longitude": 2.3522, "shutter_speed": 0.008, ` +
		`"camera_make": "Acme", "camera_model": "Acme One", "codec": "hvc1", "lens_model": "Acme One 4mm", ` +
		`"date": "2020-06-06T15:30:00+05:30", "tzoffset": 19800, "tzname": "Asia/Kolkata"}`

	// A photo without a row (photoinfo.py:1702-1704).
	refExifNull = `{"flash_fired": null, "iso": null, "metering_mode": null, "sample_rate": null, "track_format": null, "white_balance": null, ` +
		`"aperture": null, "bit_rate": null, "duration": null, "exposure_bias": null, "focal_length": null, "fps": null, ` +
		`"latitude": null, "longitude": null, "shutter_speed": null, ` +
		`"camera_make": null, "camera_model": null, "codec": null, "lens_model": null, ` +
		`"date": null, "tzoffset": null, "tzname": null}`
)

const (
	// The value of every key is its position in ScoreInfo divided by 100, except behavioral and interaction (Photos 11).
	refScore = `{"overall": 0.01, "curation": 0.02, "promotion": 0.03, "highlight_visibility": 0.04, "behavioral": null, "failure": 0.06, ` +
		`"harmonious_color": 0.07, "immersiveness": 0.08, "interaction": null, "interesting_subject": 0.1, "intrusive_object_presence": 0.11, ` +
		`"lively_color": 0.12, "low_light": 0.13, "noise": 0.14, "pleasant_camera_tilt": 0.15, "pleasant_composition": 0.16, ` +
		`"pleasant_lighting": 0.17, "pleasant_pattern": 0.18, "pleasant_perspective": 0.19, "pleasant_post_processing": 0.2, ` +
		`"pleasant_reflection": 0.21, "pleasant_symmetry": 0.22, "sharply_focused_subject": 0.23, "tastefully_blurred": 0.24, ` +
		`"well_chosen_subject": 0.25, "well_framed_subject": 0.26, "well_timed_shot": 0.27}`

	// A photo without a row: upstream writes 0.0 for every score (photoinfo.py:1507-1537).
	refScoreZero = `{"overall": 0.0, "curation": 0.0, "promotion": 0.0, "highlight_visibility": 0.0, "behavioral": 0.0, "failure": 0.0, ` +
		`"harmonious_color": 0.0, "immersiveness": 0.0, "interaction": 0.0, "interesting_subject": 0.0, "intrusive_object_presence": 0.0, ` +
		`"lively_color": 0.0, "low_light": 0.0, "noise": 0.0, "pleasant_camera_tilt": 0.0, "pleasant_composition": 0.0, ` +
		`"pleasant_lighting": 0.0, "pleasant_pattern": 0.0, "pleasant_perspective": 0.0, "pleasant_post_processing": 0.0, ` +
		`"pleasant_reflection": 0.0, "pleasant_symmetry": 0.0, "sharply_focused_subject": 0.0, "tastefully_blurred": 0.0, ` +
		`"well_chosen_subject": 0.0, "well_framed_subject": 0.0, "well_timed_shot": 0.0}`
)

// The lists of names are in the order of area, not sorted (placeinfo.py:643-653).
const (
	refPlaceAddress = `{"city": "Townsville", "country": "Exampleland", "iso_country_code": "EX", "postal_code": null, "state_province": null, ` +
		`"street": "1 Example Street", "sub_administrative_area": null, "sub_locality": null}`
	refPlace = `{"address": ` + refPlaceAddress + `, "address_str": "1 Example Street, Townsville", "country_code": "EX", "ishome": true, ` +
		`"name": "Example Park, Townsville, Exampleland", "names": {"additional_city_info": [], "area_of_interest": ["Example Park"], ` +
		`"body_of_water": [], "city": ["Townsville", "Greater Townsville"], "country": ["Exampleland"], "field0": [], "field10": [], ` +
		`"field13": ["Field Thirteen"], "field15": [], "field16": [], "field5": [], "inland_water": [], "ocean": [], "postal_code": [], ` +
		`"region": [], "state_province": [], "street_address": [], "sub_administrative_area": [], "sub_throughfare": []}}`
)

const elemImport = `{"creation_date": "2020-06-07T01:02:03+00:00", "end_date": "2020-06-07T01:01:00.500000+00:00", ` +
	`"start_date": "2020-06-07T01:00:00+00:00", "title": "Import 1", "uuid": "` + importSession + `"}`

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
	elemSharedS = albumJSON(albumSharedS, `"Shared S"`, "null", "[]",
		`"2020-03-03T03:03:03+00:00"`, "null", "null", `"Owner A"`)
	elemSharedTitledA = albumJSON(albumSharedTitledA, `"Album A"`, "null", "[]",
		`"2020-03-03T03:03:03+00:00"`, "null", "null", `"Owner A"`)
	elemProjectP = albumJSON(projectP, `"Project P"`, `"`+folderX+`"`, "[]",
		`"2020-04-04T04:04:04+00:00"`, "null", "null", "null")
)

// A user album and a shared album share the title Album A.
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
		"comments":             refComments,
		"date":                 `"2020-06-06T19:45:24.725564+05:30"`,
		"date_added":           `"2021-01-02T03:04:05+00:00"`,
		"date_modified":        `"2021-02-03T04:05:06.500000+00:00"`,
		"date_original":        `"2020-06-06T15:30:00+05:30"`,
		"date_trashed":         `"2022-03-04T05:06:07+00:00"`,
		"description":          `"Desc"`,
		"exif_info":            refExif,
		"favorite":             "true",
		"filename":             `"IMG_0001.HEIC"`,
		"fingerprint":          `"FP-1"`,
		"folders":              `{"Album A": ["Folder Y"], "Shared S": []}`,
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
		"labels":               `["Beach", "Dog"]`,
		"labels_normalized":    `["beach", "dog"]`,
		"latitude":             "48.8566",
		"likes":                refLikes,
		"live_photo":           "true",
		"location":             "[48.8566, 2.3522]",
		"longitude":            "2.3522",
		"orientation":          "6",
		"original_filename":    `"orig_0001.heic"`,
		"original_filesize":    "1195350",
		"original_height":      "3024",
		"original_orientation": "1",
		"original_width":       "4032",
		"owner":                `"Photo Owner"`,
		"person_info":          refPersonInfo,
		"persons":              `["Person A", "_UNKNOWN_"]`,
		"place":                refPlace,
		"raw_original":         "false",
		"saved_to_library":     "true",
		"score":                refScore,
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
		"cloud_metadata":        refCloudMetadata,
		"cloud_owner_hashed_id": `"OWNER-1"`,
		"external_edit":         "true",
		"hdr":                   "true",
		"import_info":           elemImport,
		"imported_by":           `["Camera", "com.example.camera"]`,
		"panorama":              "true",
		"portrait":              "true",
		"project_info":          "[" + elemProjectP + "]",
		"selfie":                "true",
		"shared_moment":         "true",
		"spatial":               "1",
		"tzname":                `"Asia/Kolkata"`,
		"visible":               "true",

		"search_info":            refSearchInfo(false),
		"search_info_normalized": refSearchInfo(true),

		"adjustments":          "{}",
		"face_info":            refFaceInfo,
		"original_adjustments": "{}",
		"path_derivatives":     "[]",
		"rating":               "0",
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
		"original_height": "null", "original_orientation": "null", "original_width": "null", "person_info": "[]", "persons": "[]",
		"raw_original": "false", "saved_to_library": "false", "screenshot": "false", "shared": "false",
		"shared_library": "false", "syndicated": "false", "title": "null", "tzoffset": "null",
		"screen_recording": "false", "slow_mo": "false", "time_lapse": "false", "uti_edited": "null",
		"uti": `"public.jpeg"`, "uti_original": `"public.jpeg"`, "uti_raw": "null", "uuid": `"` + emptyPhoto + `"`, "width": "null",
		"adjustment_type": "null", "cloud_owner_hashed_id": "null", "external_edit": "false", "hdr": "false",
		"import_info": "{}", "imported_by": "[null, null]", "panorama": "false", "portrait": "false", "project_info": "[]", "selfie": "false",
		"shared_moment": "false", "spatial": "0", "tzname": "null", "visible": "false",
		"comments": "[]", "likes": "[]", "cloud_metadata": "{}",
		"date_original": `"1970-01-01T00:00:00+00:00"`, "exif_info": refExifNull, "owner": "null", "place": "{}", "score": refScoreZero,
		"labels": "[]", "labels_normalized": "[]", "search_info": refSearchInfoEmpty, "search_info_normalized": refSearchInfoEmpty,
		"adjustments": "{}", "face_info": "[]", "original_adjustments": "{}", "path_derivatives": "[]", "rating": "0",
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

func compare(t *testing.T, ref *Reference, lib goLib, divs ...Divergence) *Report {
	t.Helper()
	rep, err := Compare(ref, lib.lib(), Keys(), divs)
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
