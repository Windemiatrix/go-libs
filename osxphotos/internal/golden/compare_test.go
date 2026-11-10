package golden

import (
	"errors"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

func TestFixtureKeys(t *testing.T) {
	t.Parallel()
	want := slices.Sorted(slices.Values(fieldsTxt(t)))
	for name, m := range map[string]map[string]string{"refSelectedFrame": refSelectedFrame(), "refEmptyPhoto": refEmptyPhoto()} {
		if got := slices.Sorted(maps.Keys(m)); !slices.Equal(got, want) {
			t.Errorf("%s has keys %v, fields.txt has %v", name, got, want)
		}
	}
}

func TestCompareNoDifferences(t *testing.T) {
	t.Parallel()
	rep := compare(t, fixtureRef(), fixtureLib())
	if !rep.OK() || len(rep.Diffs) != 0 || len(rep.Explained) != 0 || len(rep.Stale) != 0 {
		t.Fatalf("report = %+v, want no differences", rep)
	}
	if rep.Library != "lib" || rep.Objects != 2 || len(rep.Keys) != 92 {
		t.Errorf("Library = %q, Objects = %d, Keys = %d; want lib, 2, 92", rep.Library, rep.Objects, len(rep.Keys))
	}
	if !slices.IsSorted(rep.Keys) {
		t.Error("Keys are not sorted")
	}
}

type mutation struct {
	name   string
	mutate func(l *goLib)
	want   string
}

func photo(f func(p *photoinfo.PhotoInfo)) func(l *goLib) {
	return func(l *goLib) { f(l.Photos[0]) }
}

func albumOf(l *goLib, uuid string) *albuminfo.AlbumInfo {
	for _, a := range slices.Concat(l.Albums, l.SharedAlbums) {
		if a.UUID == uuid {
			return a
		}
	}
	panic("no album " + uuid)
}

func album(uuid string, f func(a *albuminfo.AlbumInfo)) func(l *goLib) {
	return func(l *goLib) { f(albumOf(l, uuid)) }
}

func photoMutations() []mutation {
	p := photo
	return []mutation{
		{"Favorite", p(func(x *photoinfo.PhotoInfo) { x.Favorite = false }), "favorite"},
		{"Hidden", p(func(x *photoinfo.PhotoInfo) { x.Hidden = false }), "hidden"},
		{"InTrash", p(func(x *photoinfo.PhotoInfo) { x.InTrash = true }), "intrash"},
		{"IsPhoto and IsMovie swapped", p(func(x *photoinfo.PhotoInfo) { x.IsPhoto, x.IsMovie = false, true }), "ismovie isphoto"},
		{"IsMovie also", p(func(x *photoinfo.PhotoInfo) { x.IsMovie = true }), "ismovie"},
		{"Live", p(func(x *photoinfo.PhotoInfo) { x.Live = false }), "live_photo"},
		{"Screenshot", p(func(x *photoinfo.PhotoInfo) { x.Screenshot = false }), "screenshot"},
		{"IsReference", p(func(x *photoinfo.PhotoInfo) { x.IsReference = false }), "isreference"},
		{"HasAdjustments", p(func(x *photoinfo.PhotoInfo) { x.HasAdjustments = true }), "hasadjustments"},
		{"HasRAW", p(func(x *photoinfo.PhotoInfo) { x.HasRAW = false }), "has_raw"},
		{"RawOriginal", p(func(x *photoinfo.PhotoInfo) { x.RawOriginal = true }), "raw_original"},
		{"IsMissing", p(func(x *photoinfo.PhotoInfo) { x.IsMissing = false }), "ismissing"},
		{"IsCloudAsset", p(func(x *photoinfo.PhotoInfo) { x.IsCloudAsset = false }), "iscloudasset"},
		{"IsRAW", p(func(x *photoinfo.PhotoInfo) { x.IsRAW = true }), "israw"},
		{"Shared", p(func(x *photoinfo.PhotoInfo) { x.Shared = false }), "shared"},
		{"SharedLibrary", p(func(x *photoinfo.PhotoInfo) { x.SharedLibrary = false }), "shared_library"},
		{"Syndicated", p(func(x *photoinfo.PhotoInfo) { x.Syndicated = false }), "syndicated"},
		{"SavedToLibrary", p(func(x *photoinfo.PhotoInfo) { x.SavedToLibrary = false }), "saved_to_library"},
		{"BurstKey", p(func(x *photoinfo.PhotoInfo) { x.BurstKey = true }), "burst_key"},
		{"BurstSelected", p(func(x *photoinfo.PhotoInfo) { x.BurstSelected = false }), "burst_selected"},
		{"BurstDefaultPick", p(func(x *photoinfo.PhotoInfo) { x.BurstDefaultPick = false }), "burst_default_pick"},
		{"SlowMo", p(func(x *photoinfo.PhotoInfo) { x.SlowMo = false }), "slow_mo"},
		{"TimeLapse", p(func(x *photoinfo.PhotoInfo) { x.TimeLapse = false }), "time_lapse"},
		{"ScreenRecording", p(func(x *photoinfo.PhotoInfo) { x.ScreenRecording = false }), "screen_recording"},
		{"Burst", p(func(x *photoinfo.PhotoInfo) { x.Burst = false }), "burst burst_photos"},
		{"HDR", p(func(x *photoinfo.PhotoInfo) { x.HDR = false }), "hdr"},
		{"Panorama", p(func(x *photoinfo.PhotoInfo) { x.Panorama = false }), "panorama"},
		{"Portrait", p(func(x *photoinfo.PhotoInfo) { x.Portrait = false }), "portrait"},
		{"Selfie", p(func(x *photoinfo.PhotoInfo) { x.Selfie = false }), "selfie"},
		{"Visible", p(func(x *photoinfo.PhotoInfo) { x.Visible = false }), "visible"},
		{"SharedMoment", p(func(x *photoinfo.PhotoInfo) { x.SharedMoment = false }), "shared_moment"},
		{"ExternalEdit", p(func(x *photoinfo.PhotoInfo) { x.ExternalEdit = false }), "external_edit"},
		{"Spatial", p(func(x *photoinfo.PhotoInfo) { x.Spatial = 2 }), "spatial"},
		{"TZName", p(func(x *photoinfo.PhotoInfo) { x.TZName = "" }), "tzname"},
		{"CloudOwnerHashedID", p(func(x *photoinfo.PhotoInfo) { x.CloudOwnerHashedID = "" }), "cloud_owner_hashed_id"},
		{"AdjustmentType", p(func(x *photoinfo.PhotoInfo) { x.AdjustmentType = ptr(2) }), "adjustment_type"},
		{"AdjustmentType nil", p(func(x *photoinfo.PhotoInfo) { x.AdjustmentType = nil }), "adjustment_type"},
		{"ImportedByName", p(func(x *photoinfo.PhotoInfo) { x.ImportedByName = "" }), "imported_by"},
		{"ImportedByBundleID", p(func(x *photoinfo.PhotoInfo) { x.ImportedByBundleID = "" }), "imported_by"},

		{"CloudMasterGUID", p(func(x *photoinfo.PhotoInfo) { x.CloudMasterGUID = "" }), "cloud_guid"},
		{"Description", p(func(x *photoinfo.PhotoInfo) { x.Description = "" }), "description"},
		{"Filename", p(func(x *photoinfo.PhotoInfo) { x.Filename += "x" }), "filename"},
		{"Fingerprint", p(func(x *photoinfo.PhotoInfo) { x.Fingerprint = "" }), "fingerprint"},
		{"OriginalFilename", p(func(x *photoinfo.PhotoInfo) { x.OriginalFilename = "" }), "original_filename"},
		{"Title", p(func(x *photoinfo.PhotoInfo) { x.Title = "" }), "title"},
		{"UTI", p(func(x *photoinfo.PhotoInfo) { x.UTI = "public.jpeg" }), "uti"},
		{"UTIOriginal", p(func(x *photoinfo.PhotoInfo) { x.UTIOriginal = "public.jpeg" }), "uti_original"},
		{"UTIRaw", p(func(x *photoinfo.PhotoInfo) { x.UTIRaw = "" }), "uti_raw"},
		{"UTIEdited", p(func(x *photoinfo.PhotoInfo) { x.UTIEdited = "" }), "uti_edited"},

		{"Width", p(func(x *photoinfo.PhotoInfo) { x.Width++ }), "width"},
		{"Height", p(func(x *photoinfo.PhotoInfo) { x.Height++ }), "height"},
		{"OriginalWidth", p(func(x *photoinfo.PhotoInfo) { x.OriginalWidth++ }), "original_width"},
		{"OriginalHeight", p(func(x *photoinfo.PhotoInfo) { x.OriginalHeight++ }), "original_height"},
		{"OriginalOrientation", p(func(x *photoinfo.PhotoInfo) { x.OriginalOrientation++ }), "original_orientation"},
		{"OriginalFilesize", p(func(x *photoinfo.PhotoInfo) { x.OriginalFilesize++ }), "original_filesize"},
		{"Orientation", p(func(x *photoinfo.PhotoInfo) { x.Orientation++ }), "orientation"},
		{"TZOffset", p(func(x *photoinfo.PhotoInfo) { x.TZOffset = ptr(0) }), "tzoffset"},
		{"TZOffset nil", p(func(x *photoinfo.PhotoInfo) { x.TZOffset = nil }), "tzoffset"},

		{"Date plus 1us", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(time.Microsecond) }), "date"},
		{"Date minus 1us", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(-time.Microsecond) }), "date"},
		{"Date plus 1ns", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(time.Nanosecond) }), "date"},
		{"Date plus 5h30m", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(5*time.Hour + 30*time.Minute) }), "date"},
		{"Date in another zone, the same moment", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.In(time.FixedZone("X", -7*3600)) }), "date"},
		{"Date in UTC, the same moment", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.UTC() }), "date"},
		{"DateAdded", p(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(x.DateAdded.Add(time.Second)) }), "date_added"},
		{"DateAdded nil", p(func(x *photoinfo.PhotoInfo) { x.DateAdded = nil }), "date_added"},
		{"DateModified nil", p(func(x *photoinfo.PhotoInfo) { x.DateModified = nil }), "date_modified"},
		{"DateModified", p(func(x *photoinfo.PhotoInfo) { x.DateModified = ptr(x.DateModified.Add(time.Microsecond)) }), "date_modified"},
		{"DateTrashed nil", p(func(x *photoinfo.PhotoInfo) { x.DateTrashed = nil }), "date_trashed"},

		{"DateOriginal plus 1us", p(func(x *photoinfo.PhotoInfo) { x.DateOriginal = x.DateOriginal.Add(time.Microsecond) }), "date_original"},
		{"DateOriginal is Date", p(func(x *photoinfo.PhotoInfo) { x.DateOriginal = x.Date }), "date_original"},
		{"DateOriginal in another zone, the same moment", p(func(x *photoinfo.PhotoInfo) {
			x.DateOriginal = x.DateOriginal.In(time.FixedZone("X", -7*3600))
		}), "date_original"},
		{"Owner", p(func(x *photoinfo.PhotoInfo) { x.Owner = "Someone Else" }), "owner"},
		{"Owner empty", p(func(x *photoinfo.PhotoInfo) { x.Owner = "" }), "owner"},

		{"ExifInfo FlashFired false", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.FlashFired = ptr(false) }), "exif_info"},
		{"ExifInfo FlashFired nil", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.FlashFired = nil }), "exif_info"},
		{"ExifInfo ISO", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.ISO = ptr(101) }), "exif_info"},
		{"ExifInfo ISO nil", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.ISO = nil }), "exif_info"},
		{"ExifInfo WhiteBalance 0 is not null", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.WhiteBalance = nil }), "exif_info"},
		{"ExifInfo TrackFormat and SampleRate swapped", p(func(x *photoinfo.PhotoInfo) {
			x.ExifInfo.TrackFormat, x.ExifInfo.SampleRate = x.ExifInfo.SampleRate, x.ExifInfo.TrackFormat
		}), "exif_info"},
		{"ExifInfo Aperture by one ulp", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Aperture = ptr(math.Nextafter(*x.ExifInfo.Aperture, 2)) }), "exif_info"},
		{"ExifInfo Duration rounded", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Duration = ptr(12.0) }), "exif_info"},
		{"ExifInfo FPS and BitRate swapped", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.FPS, x.ExifInfo.BitRate = x.ExifInfo.BitRate, x.ExifInfo.FPS }), "exif_info"},
		{"ExifInfo Latitude", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Latitude = ptr(1.0) }), "exif_info"},
		{"ExifInfo CameraMake", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.CameraMake = "Other" }), "exif_info"},
		{"ExifInfo CameraMake empty", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.CameraMake = "" }), "exif_info"},
		{"ExifInfo CameraMake in NFD", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.CameraMake = "Acme" + string(rune(0x301)) }), "exif_info"},
		{"ExifInfo LensModel", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.LensModel = "" }), "exif_info"},
		{"ExifInfo Date nil", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Date = nil }), "exif_info"},
		{"ExifInfo Date plus 1us", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Date = ptr(x.ExifInfo.Date.Add(time.Microsecond)) }), "exif_info"},
		{"ExifInfo Date in another zone, the same moment", p(func(x *photoinfo.PhotoInfo) {
			x.ExifInfo.Date = ptr(x.ExifInfo.Date.In(time.FixedZone("X", -7*3600)))
		}), "exif_info"},
		{"ExifInfo TZOffset nil", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.TZOffset = nil }), "exif_info"},
		{"ExifInfo TZName", p(func(x *photoinfo.PhotoInfo) { x.ExifInfo.TZName = "" }), "exif_info"},

		// Only a few fields here: TestCompareScoreEveryField changes each field.
		{"Score Overall", p(func(x *photoinfo.PhotoInfo) { x.Score.Overall = ptr(0.011) }), "score"},
		{"Score Overall and Curation swapped", p(func(x *photoinfo.PhotoInfo) { x.Score.Overall, x.Score.Curation = x.Score.Curation, x.Score.Overall }), "score"},
		{"Score WellTimedShot", p(func(x *photoinfo.PhotoInfo) { x.Score.WellTimedShot = ptr(0.28) }), "score"},
		{"Score Noise by one ulp", p(func(x *photoinfo.PhotoInfo) { x.Score.Noise = ptr(math.Nextafter(*x.Score.Noise, 1)) }), "score"},
		{"Score Behavioral against a null", p(func(x *photoinfo.PhotoInfo) { x.Score.Behavioral = ptr(0.5) }), "score"},
		{"Score Behavioral 0.0 against a null", p(func(x *photoinfo.PhotoInfo) { x.Score.Behavioral = ptr(0.0) }), "score"},
		{"Score Overall None against a value", p(func(x *photoinfo.PhotoInfo) { x.Score.Overall = nil }), "score"},
		{"Score every one None", p(func(x *photoinfo.PhotoInfo) { x.Score = scoreinfo.ScoreInfo{} }), "score"},
		{"Score of a photo without a row", p(func(x *photoinfo.PhotoInfo) { x.Score = noScoreRow() }), "score"},

		// Only nil, order and position here: TestComparePlaceEveryLeaf changes each leaf.
		{"Place nil", p(func(x *photoinfo.PhotoInfo) { x.Place = nil }), "place"},
		{"Place City swapped", p(func(x *photoinfo.PhotoInfo) { x.Place.Names.City = []string{"Greater Townsville", "Townsville"} }), "place"},
		{"Place Field13 of Field15", p(func(x *photoinfo.PhotoInfo) {
			x.Place.Names.Field13, x.Place.Names.Field15 = nil, x.Place.Names.Field13
		}), "place"},

		{"InCloud false", p(func(x *photoinfo.PhotoInfo) { x.InCloud = ptr(false) }), "incloud"},
		{"InCloud nil", p(func(x *photoinfo.PhotoInfo) { x.InCloud = nil }), "incloud"},
		{"Latitude", p(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(1.0) }), "latitude location"},
		{"Latitude by one ulp", p(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(math.Nextafter(*x.Latitude, 90)) }), "latitude location"},
		{"Latitude nil", p(func(x *photoinfo.PhotoInfo) { x.Latitude = nil }), "latitude location"},
		{"Longitude", p(func(x *photoinfo.PhotoInfo) { x.Longitude = ptr(1.0) }), "location longitude"},
		{"Longitude nil", p(func(x *photoinfo.PhotoInfo) { x.Longitude = nil }), "location longitude"},

		{"Keywords swapped", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"beta", "alpha"} }), "keywords"},
		{"Keywords shorter", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"alpha"} }), "keywords"},
		{"Keywords with a repeat", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"alpha", "beta", "beta"} }), "keywords"},
		{"Keywords nil", p(func(x *photoinfo.PhotoInfo) { x.Keywords = nil }), "keywords"},
		{"Persons swapped", p(func(x *photoinfo.PhotoInfo) { x.Persons = []string{"_UNKNOWN_", "Person A"} }), "persons"},
		{"Persons shorter", p(func(x *photoinfo.PhotoInfo) { x.Persons = []string{"Person A"} }), "persons"},
		{"Persons with a repeat", p(func(x *photoinfo.PhotoInfo) { x.Persons = []string{"Person A", "Person A", "_UNKNOWN_"} }), "persons"},
		{"Persons nil", p(func(x *photoinfo.PhotoInfo) { x.Persons = nil }), "persons"},

		{"BurstUUID of the other frame", func(l *goLib) { l.Photos[1].BurstUUID = "BURST-B" }, "burst_photos"},
		// The other frame is then a plain photo, which osxphotos would return.
		{"the other frame is not a burst frame", func(l *goLib) { l.Photos[1].Burst = false }, "burst_photos uuid"},
		{"BurstUUID of the frame", p(func(x *photoinfo.PhotoInfo) { x.BurstUUID = "BURST-B" }), "burst_photos"},
		{"a third frame, not a photo and in the trash", func(l *goLib) {
			l.Photos = append(l.Photos, &photoinfo.PhotoInfo{UUID: "00000000-0000-0000-0000-000000000004", Burst: true, BurstUUID: "BURST-A", InTrash: true})
		}, "burst_photos"},

		{"AlbumUUIDs empty", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = nil }), "album_info albums folders"},
		{"AlbumUUIDs without Album A in Folder X", p(func(x *photoinfo.PhotoInfo) {
			x.AlbumUUIDs = []string{albumAInFolderY, albumTrashed, albumSharedS, albumSharedTitledA}
		}), "album_info"},
		{"AlbumUUIDs without Album A in Folder Y", p(func(x *photoinfo.PhotoInfo) {
			x.AlbumUUIDs = []string{albumAInFolderX, albumSharedS, albumSharedTitledA}
		}), "album_info folders"},
		{"AlbumUUIDs without the shared albums", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = []string{albumAInFolderX, albumAInFolderY} }), "album_info albums folders"},
		{"BurstAlbums empty", func(l *goLib) { l.BurstAlbums[selectedFrame] = nil }, "burst_album_info burst_albums"},
		{"BurstAlbums without Album B", func(l *goLib) {
			l.BurstAlbums[selectedFrame] = []string{albumAInFolderX, albumAInFolderY, albumSharedS}
		}, "burst_album_info burst_albums"},
		{"an album in the trash is listed", func(l *goLib) { l.BurstAlbums[selectedFrame] = append(l.BurstAlbums[selectedFrame], albumTrashed) }, ""},
		{"an album of the library is listed", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = append(x.AlbumUUIDs, albumOfNoPhoto) }), "album_info albums folders"},
		{"Album A in Folder X goes to the trash", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), "album_info burst_album_info"},
		{"Album title", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.Title = "Album Z" }), "album_info albums burst_album_info burst_albums folders"},
		{"Album title is empty", album(albumBNoFolder, func(a *albuminfo.AlbumInfo) { a.Title = "" }), "burst_album_info burst_albums"},
		{"Album parent", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.ParentUUID = "" }), "album_info burst_album_info"},
		{"Album folder names", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.FolderNames = []string{"Folder Z"} }), "album_info burst_album_info"},
		{"Album folder names of the album that folders keeps", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.FolderNames = []string{"Folder Z"} }), "album_info burst_album_info folders"},
		{"Album creation date", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.CreationDate = nil }), "album_info burst_album_info"},
		{"Album creation date by 1us", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.CreationDate = ptr(a.CreationDate.Add(time.Microsecond)) }), "album_info burst_album_info"},
		{"Album start date", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.StartDate = ptr(utc(2000, 1, 1, 0, 0, 0, 0)) }), "album_info burst_album_info"},
		{"Album end date", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.EndDate = nil }), "album_info burst_album_info"},
		{"Album owner", album(albumAInFolderX, func(a *albuminfo.AlbumInfo) { a.Owner = "Owner A" }), "album_info burst_album_info"},
		{"Shared album owner", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.Owner = "Owner B" }), "album_info burst_album_info"},
		{"Shared album owner empty", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.Owner = "" }), "album_info burst_album_info"},
		{"Shared album title", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.Title = "Shared Z" }), "album_info albums burst_album_info burst_albums folders"},
		{"Shared album folder names", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.FolderNames = []string{"Folder X"} }), "album_info burst_album_info folders"},
		{"Shared album creation date", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.CreationDate = nil }), "album_info burst_album_info"},
		{"Shared album with the title of a user album goes to the trash", album(albumSharedTitledA, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), "album_info"},
		{"Shared album parent", album(albumSharedS, func(a *albuminfo.AlbumInfo) { a.ParentUUID = folderX }), "album_info burst_album_info"},
		{"Shared album with a parent among the user albums", func(l *goLib) {
			l.SharedAlbums[0].ParentUUID = folderX
			l.Albums, l.SharedAlbums = append(l.Albums, l.SharedAlbums[0]), l.SharedAlbums[1:]
		}, "album_info burst_album_info"},

		{"ImportUUID empty", p(func(x *photoinfo.PhotoInfo) { x.ImportUUID = "" }), "import_info"},
		{"ImportUUID of another session", p(func(x *photoinfo.PhotoInfo) { x.ImportUUID = importBare }), "import_info"},
		{"Import title", func(l *goLib) { l.Imports[0].Title = "Import 2" }, "import_info"},
		{"Import title empty", func(l *goLib) { l.Imports[0].Title = "" }, "import_info"},
		{"Import creation date", func(l *goLib) { l.Imports[0].CreationDate = nil }, "import_info"},
		{"Import start date", func(l *goLib) { l.Imports[0].StartDate = nil }, "import_info"},
		{"Import end date by 1us", func(l *goLib) { l.Imports[0].EndDate = ptr(l.Imports[0].EndDate.Add(time.Microsecond)) }, "import_info"},
		{"Import start and end dates swapped", func(l *goLib) {
			l.Imports[0].StartDate, l.Imports[0].EndDate = l.Imports[0].EndDate, l.Imports[0].StartDate
		}, "import_info"},
		{"Import UUID", func(l *goLib) { l.Imports[0].UUID, l.Imports[1].UUID = importBare, importSession }, "import_info"},

		{"ProjectUUIDs empty", p(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = nil }), "project_info"},
		{"ProjectUUIDs without the project outside the trash", p(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed} }), "project_info"},
		{"Project goes to the trash", func(l *goLib) { l.Projects[0].InTrash = true }, "project_info"},
		{"Project in the trash comes out of it", func(l *goLib) { l.Projects[1].InTrash = false }, "project_info"},
		{"Project title", func(l *goLib) { l.Projects[0].Title = "Project Q" }, "project_info"},
		{"Project folder names", func(l *goLib) { l.Projects[0].FolderNames = []string{"Folder X"} }, "project_info"},
		{"Project creation date", func(l *goLib) { l.Projects[0].CreationDate = nil }, "project_info"},
		{"Project end date", func(l *goLib) { l.Projects[0].EndDate = ptr(utc(2020, 4, 5, 0, 0, 0, 0)) }, "project_info"},
		{"a project among the albums of the photo", func(l *goLib) {
			l.Projects = l.Projects[1:]
			l.Albums = append(l.Albums, &albuminfo.AlbumInfo{UUID: projectP, Title: "Project P", ParentUUID: folderX, CreationDate: ptr(utc(2020, 4, 4, 4, 4, 4, 0))})
			l.Photos[0].AlbumUUIDs = append(l.Photos[0].AlbumUUIDs, projectP)
			l.Photos[0].ProjectUUIDs = []string{projectTrashed}
		}, "album_info albums folders project_info"},

		{"Comments nil", func(l *goLib) { l.Comments = nil }, "comments"},
		{"Comments swapped", func(l *goLib) { c := l.Comments[selectedFrame]; c[0], c[1] = c[1], c[0] }, "comments"},
		{"Comment date by 1us", func(l *goLib) {
			c := &l.Comments[selectedFrame][1]
			c.Datetime = ptr(c.Datetime.Add(time.Microsecond))
		}, "comments"},
		{"Comment date nil", func(l *goLib) { l.Comments[selectedFrame][0].Datetime = nil }, "comments"},
		{"Comment user", func(l *goLib) { l.Comments[selectedFrame][0].User = "" }, "comments"},
		{"Comment user of null", func(l *goLib) { l.Comments[selectedFrame][1].User = "Person B" }, "comments"},
		{"Comment IsMine", func(l *goLib) { l.Comments[selectedFrame][0].IsMine = false }, "comments"},
		{"Comment text", func(l *goLib) { l.Comments[selectedFrame][1].Text = "Comment 3" }, "comments"},
		{"Comment of another photo", func(l *goLib) {
			l.Comments[emptyPhoto], l.Comments[selectedFrame] = l.Comments[selectedFrame], nil
		}, "comments"},
		{"Likes nil", func(l *goLib) { l.Likes = nil }, "likes"},
		{"Like date", func(l *goLib) { c := &l.Likes[selectedFrame][0]; c.Datetime = ptr(c.Datetime.Add(time.Second)) }, "likes"},
		{"Like user", func(l *goLib) { l.Likes[selectedFrame][0].User = "" }, "likes"},
		{"Like IsMine", func(l *goLib) { l.Likes[selectedFrame][0].IsMine = true }, "likes"},
		{"a second like", func(l *goLib) {
			l.Likes[selectedFrame] = append(l.Likes[selectedFrame], photoinfo.LikeInfo{Datetime: ptr(utc(2020, 7, 4, 0, 0, 0, 0))})
		}, "likes"},

		{"Persons nil", func(l *goLib) { l.Persons = nil }, "person_info"},
		{"Persons of another photo", func(l *goLib) { l.Persons[emptyPhoto], l.Persons[selectedFrame] = l.Persons[selectedFrame], nil }, "person_info"},
		// A person per face (photoinfo.py:605-611): a repeat is not merged.
		{"Persons with a repeat", func(l *goLib) {
			l.Persons[selectedFrame] = append(l.Persons[selectedFrame], l.Persons[selectedFrame][1])
		}, "person_info"},
		// Upstream gives the persons of a photo by name (photoinfo.py:605-611); the rule does not sort them.
		{"Persons in another order", func(l *goLib) { p := l.Persons[selectedFrame]; p[0], p[1] = p[1], p[0] }, "person_info"},
		{"PersonInfo UUID", person(0, func(x *personinfo.PersonInfo) { x.UUID = personUnknown }), "person_info"},
		{"PersonInfo Name", person(0, func(x *personinfo.PersonInfo) { x.Name = "Person B" }), "person_info"},
		{"PersonInfo Name of the unknown person empty", person(1, func(x *personinfo.PersonInfo) { x.Name = "" }), "person_info"},
		{"PersonInfo DisplayName", person(0, func(x *personinfo.PersonInfo) { x.DisplayName = "" }), "person_info"},
		{"PersonInfo KeyFace", person(0, func(x *personinfo.PersonInfo) { x.KeyFace++ }), "person_info"},
		{"PersonInfo FaceCount", person(0, func(x *personinfo.PersonInfo) { x.FaceCount++ }), "person_info"},
		{"PersonInfo KeyPhotoUUID", person(0, func(x *personinfo.PersonInfo) { x.KeyPhotoUUID = "" }), "person_info"},
		{"PersonInfo Favorite", person(0, func(x *personinfo.PersonInfo) { x.Favorite = false }), "person_info"},
		{"PersonInfo SortOrder", person(0, func(x *personinfo.PersonInfo) { x.SortOrder++ }), "person_info"},
		{"PersonInfo FeatureLess", person(0, func(x *personinfo.PersonInfo) { x.FeatureLess = false }), "person_info"},
		{"PersonInfo FeatureLess of the unknown person", person(1, func(x *personinfo.PersonInfo) { x.FeatureLess = true }), "person_info"},

		{"Faces nil", func(l *goLib) { l.Faces = nil }, "face_info"},
		{"Faces of another photo", func(l *goLib) { l.Faces[emptyPhoto], l.Faces[selectedFrame] = l.Faces[selectedFrame], nil }, "face_info"},
		{"Faces with a repeat", func(l *goLib) { l.Faces[selectedFrame] = append(l.Faces[selectedFrame], l.Faces[selectedFrame][0]) }, "face_info"},
		{"FaceInfo PK", face(func(x *personinfo.FaceInfo) { x.PK++ }), "face_info"},
		{"FaceInfo Name", face(func(x *personinfo.FaceInfo) { x.Name = "Person B" }), "face_info"},
		{"FaceInfo Name empty", face(func(x *personinfo.FaceInfo) { x.Name = "" }), "face_info"},
		{"FaceInfo AssetUUID", face(func(x *personinfo.FaceInfo) { x.AssetUUID = emptyPhoto }), "face_info"},
		{"FaceInfo CenterX", face(func(x *personinfo.FaceInfo) { x.CenterX = 0.25 }), "face_info"},
		{"FaceInfo Size", face(func(x *personinfo.FaceInfo) { x.Size = 0.5 }), "face_info"},
		{"FaceInfo HasSmile", face(func(x *personinfo.FaceInfo) { x.HasSmile = false }), "face_info"},
		{"FaceInfo Roll", face(func(x *personinfo.FaceInfo) { x.Roll = 0 }), "face_info"},
		{"FaceInfo DetectionType", face(func(x *personinfo.FaceInfo) { x.DetectionType = 0 }), "face_info"},
		{"FaceInfo BodyWidth and BodyHeight swapped", face(func(x *personinfo.FaceInfo) { x.BodyWidth, x.BodyHeight = x.BodyHeight, x.BodyWidth }), "face_info"},
		// A cat with a body region and no face region: is_pet and the regions differ (personinfo.py:200-212).
		{"FaceInfo of a cat", face(func(x *personinfo.FaceInfo) { x.DetectionType = 4 }), "face_info"},

		{"PathDerivatives", p(func(x *photoinfo.PhotoInfo) {
			x.PathDerivatives = []string{"resources/derivatives/0/IMG_0001_1_105_c.jpeg"}
		}), "path_derivatives"},
		{"rating", override("rating", 1), "rating"},
		{"adjustments", override("adjustments", map[string]any{"editor": "com.example.editor"}), "adjustments"},
		{"original_adjustments", override("original_adjustments", map[string]any{"editor": "com.example.editor"}), "original_adjustments"},

		{"CloudMetadata nil", func(l *goLib) { l.CloudMetadata = nil }, "cloud_metadata"},
		{"CloudMetadata of another photo", func(l *goLib) {
			l.CloudMetadata[emptyPhoto], l.CloudMetadata[selectedFrame] = l.CloudMetadata[selectedFrame], nil
		}, "cloud_metadata"},
		{"CloudMetadata string", cloudMeta(func(m map[string]any) { m["mediaType"] = "video" }), "cloud_metadata"},
		{"CloudMetadata extra key", cloudMeta(func(m map[string]any) { m["extra"] = true }), "cloud_metadata"},
		{"CloudMetadata missing key", cloudMeta(func(m map[string]any) { delete(m, "flag") }), "cloud_metadata"},
		{"CloudMetadata integer above 2^53 by one", cloudMeta(func(m map[string]any) { m["big"] = uint64(9007199254740992) }), "cloud_metadata"},
		{"CloudMetadata real by one ulp", cloudMeta(func(m map[string]any) { m["ratio"] = math.Nextafter(0.5, 1) }), "cloud_metadata"},
		{"CloudMetadata date by 1us", cloudMeta(func(m map[string]any) { m["when"] = utc(2020, 1, 2, 3, 4, 5, 7000) }), "cloud_metadata"},
		{"CloudMetadata date without microseconds", cloudMeta(func(m map[string]any) { m["when"] = utc(2020, 1, 2, 3, 4, 5, 0) }), "cloud_metadata"},
		{"CloudMetadata list swapped", cloudMeta(func(m map[string]any) { m["nested"] = map[string]any{"list": []any{"x", int64(-1)}} }), "cloud_metadata"},

		{"SearchInfo nil", func(l *goLib) { l.SearchInfo = nil }, "labels labels_normalized search_info search_info_normalized"},
		{"SearchInfo of another photo", func(l *goLib) {
			l.SearchInfo[emptyPhoto], l.SearchInfo[selectedFrame] = l.SearchInfo[selectedFrame], searchinfo.SearchInfo{}
		}, "labels labels_normalized search_info search_info_normalized"},
		{"a label", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { r[0].Content = "Cat"; return r }), "labels search_info"},
		{"a normalized label", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { r[0].Normalized = "cat"; return r }),
			"labels_normalized search_info_normalized"},
		// SearchInfo keeps a term once per record, as upstream does (searchinfo.py:401-413).
		{"a repeated label", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { return append(r, r[0]) }),
			"labels labels_normalized search_info search_info_normalized"},
		{"the city", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { r[1].Content = "Elsewhere"; return r }), "search_info"},
		{"the city as a state", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { r[1].Category = searchinfo.CategoryState; return r }),
			"search_info search_info_normalized"},
		{"a normalized holiday", searchTerms(func(r []searchinfo.Record) []searchinfo.Record { r[3].Normalized = "x"; return r }),
			"search_info_normalized"},
	}
}

func cloudMeta(f func(m map[string]any)) func(l *goLib) {
	return func(l *goLib) { f(l.CloudMetadata[selectedFrame]) }
}

func searchTerms(f func(r []searchinfo.Record) []searchinfo.Record) func(l *goLib) {
	return func(l *goLib) { l.SearchInfo[selectedFrame] = searchinfo.New(f(searchRecords())) }
}

// i indexes goLib.Persons of selectedFrame: 0 is Person A, 1 the unknown person.
func person(i int, f func(x *personinfo.PersonInfo)) func(l *goLib) {
	return func(l *goLib) { f(&l.Persons[selectedFrame][i]) }
}

func face(f func(x *personinfo.FaceInfo)) func(l *goLib) {
	return func(l *goLib) { f(&l.Faces[selectedFrame][0]) }
}

// For the keys that no field of the fixture gives.
func override(key string, v any) func(l *goLib) {
	return func(l *goLib) { l.Override = map[string]map[string]any{selectedFrame: {key: v}} }
}

// The expectations and the reference are literals, not computed by the package.
func TestCompareEachKey(t *testing.T) {
	t.Parallel()
	cases := photoMutations()

	covered := map[string]bool{}
	for _, c := range cases {
		for _, k := range strings.Fields(c.want) {
			covered[k] = true
		}
	}
	// uuid is the set of objects, spoiled in TestCompareSets.
	for _, k := range fieldsTxt(t) {
		if k != "uuid" && !covered[k] {
			t.Errorf("no case spoils the key %s", k)
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			c.mutate(&lib)
			rep := compare(t, fixtureRef(), lib)
			want := strings.Fields(c.want)
			slices.Sort(want)
			if got := diffKeys(rep); !slices.Equal(got, want) {
				t.Errorf("differences in %v, want %v\n%+v", got, want, rep.Diffs)
			}
			for _, d := range rep.Diffs {
				if d.Want == d.Got {
					t.Errorf("a difference of %s with equal values %s", d.Field, d.Want)
				}
			}
		})
	}
}

// A key that AsDict or the rule left out would be missing on both sides: only
// a change of one field of the Go side shows it.
func TestCompareScoreEveryField(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[scoreinfo.ScoreInfo]()
	for i := range typ.NumField() {
		t.Run(typ.Field(i).Name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			f := reflect.ValueOf(&lib.Photos[0].Score).Elem().Field(i)
			v := 0.5
			if !f.IsNil() {
				v += f.Elem().Float()
			}
			f.Set(reflect.ValueOf(&v))
			if got := diffKeys(compare(t, fixtureRef(), lib)); !slices.Equal(got, []string{"score"}) {
				t.Errorf("differences in %v, want score", got)
			}
		})
	}
}

// Both sides go through placeValue, so a leaf it dropped would be dropped on
// both: only a change on one side shows it.
func TestComparePlaceEveryLeaf(t *testing.T) {
	t.Parallel()
	type leaf struct {
		name  string
		index []int
	}
	var leaves []leaf
	top := reflect.TypeFor[placeinfo.PlaceInfo]()
	for i := range top.NumField() {
		f := top.Field(i)
		if f.Type.Kind() != reflect.Struct {
			leaves = append(leaves, leaf{f.Name, []int{i}})
			continue
		}
		for j := range f.Type.NumField() {
			leaves = append(leaves, leaf{f.Name + "." + f.Type.Field(j).Name, []int{i, j}})
		}
	}
	if len(leaves) != 4+19+8 {
		t.Fatalf("PlaceInfo has %d leaves, want 31", len(leaves))
	}
	for _, l := range leaves {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			v := reflect.ValueOf(lib.Photos[0].Place).Elem().FieldByIndex(l.index)
			switch v.Kind() {
			case reflect.String:
				v.SetString(v.String() + "x")
			case reflect.Bool:
				v.SetBool(!v.Bool())
			default:
				v.Set(reflect.Append(v, reflect.ValueOf("x")))
			}
			if got := diffKeys(compare(t, fixtureRef(), lib)); !slices.Equal(got, []string{"place"}) {
				t.Errorf("differences in %v, want place", got)
			}
		})
	}
}

// The comparison must not normalize strings: upstream gives the title in NFC
// (unicode.py:76-92), the strings of exif_info as SQLite returns them
// (exifinfo.py:63-66) and the country codes of place as the archive holds them
// (placeinfo.py:96, 250), so a Go string in another form than the reference is
// a difference to be fixed in the port.
func TestCompareNFD(t *testing.T) {
	t.Parallel()
	nfc := "Caf" + string(rune(0xe9))
	nfd := "Cafe" + string(rune(0x301))
	exif := func(cameraMake string) string {
		return strings.Replace(refExifNull, `"camera_make": null`, `"camera_make": `+jsonString(cameraMake), 1)
	}
	titleRef := map[string]string{"title": jsonString(nfc)}
	exifRef := map[string]string{"exif_info": exif(nfd)}
	placeRef := map[string]string{"place": strings.NewReplacer(
		`"name": "Example Park, Townsville, Exampleland"`, `"name": `+jsonString(nfc),
		`"country_code": "EX"`, `"country_code": `+jsonString(nfd)).Replace(refPlace)}
	place := func(name, countryCode string) func(p *photoinfo.PhotoInfo) {
		return func(p *photoinfo.PhotoInfo) {
			p.Place = fixtureLib().Photos[0].Place
			p.Place.Name, p.Place.CountryCode = name, countryCode
		}
	}
	for name, tc := range map[string]struct {
		ref  map[string]string
		set  func(p *photoinfo.PhotoInfo)
		want []string
	}{
		"title NFC":              {titleRef, func(p *photoinfo.PhotoInfo) { p.Title = nfc }, nil},
		"title NFD":              {titleRef, func(p *photoinfo.PhotoInfo) { p.Title = nfd }, []string{"title"}},
		"camera_make NFD kept":   {exifRef, func(p *photoinfo.PhotoInfo) { p.ExifInfo.CameraMake = nfd }, nil},
		"camera_make NFD to NFC": {exifRef, func(p *photoinfo.PhotoInfo) { p.ExifInfo.CameraMake = nfc }, []string{"exif_info"}},
		"place name NFC":         {placeRef, place(nfc, nfd), nil},
		"place name NFD":         {placeRef, place(nfd, nfd), []string{"place"}},
		"place country_code NFC": {placeRef, place(nfc, nfc), []string{"place"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			lib.Photos = lib.Photos[2:]
			tc.set(lib.Photos[0])
			rep := compare(t, refOf(refObj(refEmptyPhoto(), tc.ref)), lib)
			if got := diffKeys(rep); !slices.Equal(got, tc.want) {
				t.Errorf("differences in %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Photos[0].Date = lib.Photos[0].Date.Add(time.Microsecond)
	lib.Photos[0].Keywords = []string{"beta", "alpha"}
	lib.Photos[0].InCloud = nil
	lib.Photos[0].Latitude = ptr(1.5)
	lib.Photos[0].Width = 4033 // the center of the face stays where it is
	lib.Photos[0].Title = "A \"quoted\" <title> & more"
	lib.Photos[0].BurstDefaultPick = false
	rep := compare(t, fixtureRef(), lib)
	got := map[string]Diff{}
	for _, d := range rep.Diffs {
		got[d.Field] = d
	}
	want := map[string][2]string{
		"date":               {`["2020-06-06T14:15:24.725564Z",19800]`, `["2020-06-06T14:15:24.725565Z",19800]`},
		"keywords":           {`["alpha","beta"]`, `["beta","alpha"]`},
		"incloud":            {`true`, `null`},
		"latitude":           {`48.8566`, `1.5`},
		"location":           {`[48.8566,2.3522]`, `[1.5,2.3522]`},
		"width":              {`4032`, `4033`},
		"title":              {`"Title A"`, `"A \"quoted\" <title> & more"`},
		"burst_default_pick": {`true`, `false`},
	}
	if len(got) != len(want) {
		t.Errorf("differences in %v, want %d keys", diffKeys(rep), len(want))
	}
	for k, w := range want {
		d := got[k]
		if d.Want != w[0] || d.Got != w[1] || d.UUID != selectedFrame {
			t.Errorf("%s: %+v, want %s -> %s of %s", k, d, w[0], w[1], selectedFrame)
		}
	}
}

func TestWriteDetailsPersons(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Photos[0].Persons = []string{"Person A"}
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "persons", Want: []byte(`["Person A"]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale)
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"persons " + selectedFrame + " want=<2 names> got=<1 names>\n",
		"stale persons " + emptyPhoto + " want=<1 names> got=*\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails has no line %q", line)
		}
	}
	if strings.Contains(b.String(), "Person A") {
		t.Error("WriteDetails writes a name of a person")
	}
}

func TestWriteDetailsPersonInfo(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Persons[selectedFrame][0].Name, lib.Persons[selectedFrame][0].DisplayName = "SECRET-NAME", "SECRET-DISPLAY"
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "person_info", Want: []byte(`[{"name":"SECRET-STALE","displayname":""}]`), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: selectedFrame, Field: "person_info", Want: []byte(`{"name":"SECRET-OBJECT"}`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"person_info"}) {
		t.Fatalf("differences in %v, want person_info", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "Person A", "Pat", "_UNKNOWN_"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	// want 2 + got 2 names, the stale record 1; want 1 + got 1 display names, "" kept.
	if n := strings.Count(b.String(), `"name":"<name>"`); n != 5 {
		t.Errorf("%d names written as <name>, want 5:\n%s", n, b.String())
	}
	if n := strings.Count(b.String(), `"displayname":"<name>"`); n != 2 {
		t.Errorf("%d display names written as <name>, want 2:\n%s", n, b.String())
	}
	if !strings.Contains(b.String(), `"displayname":""`) || !strings.Contains(b.String(), personA) {
		t.Errorf("WriteDetails lost the empty display names or the UUIDs:\n%s", b.String())
	}
	if line := "stale person_info " + selectedFrame + " want=<unparsed> got=*\n"; !strings.Contains(b.String(), line) {
		t.Errorf("WriteDetails has no line %q:\n%s", line, b.String())
	}
}

func TestWriteDetailsOwners(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	albumOf(&lib, albumSharedS).Owner = "Owner B"
	lib.Projects[0].Owner = "Owner C"
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "burst_album_info", Want: []byte("[" + elemSharedS + "]"), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: emptyPhoto, Field: "project_info", Want: []byte(`{"owner":"Owner D"}`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"album_info", "burst_album_info", "project_info"}) {
		t.Fatalf("differences in %v, want album_info, burst_album_info and project_info", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Owner A", "Owner B", "Owner C", "Owner D"} {
		if strings.Contains(b.String(), name) {
			t.Errorf("WriteDetails writes the owner %s", name)
		}
	}
	// album_info 2 + 2, burst_album_info 1 + 1, project_info 0 + 1, the stale record 1.
	if n := strings.Count(b.String(), `"owner":"<name>"`); n != 8 {
		t.Errorf("%d owners written as <name>, want 8:\n%s", n, b.String())
	}
	if !strings.Contains(b.String(), `"owner":""`) || !strings.Contains(b.String(), albumSharedS) {
		t.Errorf("WriteDetails lost the empty owners or the UUIDs:\n%s", b.String())
	}
	if line := "stale project_info " + emptyPhoto + " want=<unparsed> got=*\n"; !strings.Contains(b.String(), line) {
		t.Errorf("WriteDetails has no line %q:\n%s", line, b.String())
	}
}

func TestWriteDetailsExifAndOwner(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Photos[0].Owner = "SECRET-NAME"
	lib.Photos[0].ExifInfo.CameraMake = "SECRET-MAKE"
	lib.Photos[0].ExifInfo.Latitude = ptr(12.5)
	lib.Photos[0].ExifInfo.Codec = ""
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "exif_info", Want: []byte(`{"camera_make":"SECRET-STALE","codec":""}`), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: selectedFrame, Field: "exif_info", Want: []byte(`[1]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"exif_info", "owner"}) {
		t.Fatalf("differences in %v, want exif_info and owner", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "Acme", "12.5", "Photo Owner", "Asia/Kolkata", "48.8566"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	for _, line := range []string{
		"owner " + selectedFrame + " want=<name> got=<name>\n",
		`"camera_make":"<value>"`,
		`"codec":"","date":["<value>","<value>"]`,
		`"flash_fired":"<value>"`,
		`"white_balance":"<value>"`,
		`stale exif_info ` + emptyPhoto + ` want={"camera_make":"<value>","codec":""} got=*` + "\n",
		"stale exif_info " + selectedFrame + " want=<unparsed> got=*\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails lacks %q:\n%s", line, b.String())
		}
	}
	if got := redact("owner", `""`); got != `""` {
		t.Errorf("redact of an empty owner = %s, want it kept", got)
	}
}

// Scores are not personal data, and a difference of one key cannot be found
// among 27 placeholders.
func TestWriteDetailsScore(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Photos[0].Score.Overall = ptr(0.5)
	rep := compare(t, fixtureRef(), lib)
	if got := diffKeys(rep); !slices.Equal(got, []string{"score"}) {
		t.Fatalf("differences in %v, want score", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, need := range []string{`"overall":0.01`, `"overall":0.5`, `"behavioral":null`} {
		if !strings.Contains(b.String(), need) {
			t.Errorf("WriteDetails lacks %q:\n%s", need, b.String())
		}
	}
}

func TestWriteDetailsPlace(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	pl := lib.Photos[0].Place
	pl.Name, pl.CountryCode, pl.AddressStr, pl.IsHome = "SECRET-NAME", "SECRET-CODE", "SECRET-STR", false
	address := reflect.ValueOf(&pl.Address).Elem()
	for i := range address.NumField() {
		address.Field(i).SetString("SECRET-ADDRESS")
	}
	pl.Names.City, pl.Names.Region, pl.Names.Field16 = []string{"SECRET-CITY"}, []string{"SECRET-REGION", ""}, []string{"SECRET-FIELD"}
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "place", Want: []byte(`{"name":"SECRET-STALE","ishome":false,"names":{"city":["SECRET-S",""]}}`), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: selectedFrame, Field: "place", Want: []byte(`[1]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"place"}) {
		t.Fatalf("differences in %v, want place", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "Townsville", "Exampleland", "Example", `"EX"`, "Thirteen"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	names := func(city, region, field16 string) string {
		return `"names":{"additional_city_info":[],"area_of_interest":["<value>"],"body_of_water":[],"city":` + city +
			`,"country":["<value>"],"field0":[],"field10":[],"field13":["<value>"],"field15":[],"field16":` + field16 +
			`,"field5":[],"inland_water":[],"ocean":[],"postal_code":[],"region":` + region +
			`,"state_province":[],"street_address":[],"sub_administrative_area":[],"sub_throughfare":[]}}`
	}
	want := `{"address":{"city":"<value>","country":"<value>","iso_country_code":"<value>","postal_code":"","state_province":"",` +
		`"street":"<value>","sub_administrative_area":"","sub_locality":""},"address_str":"<value>","country_code":"<value>",` +
		`"ishome":"<value>","name":"<value>",` + names(`["<value>","<value>"]`, `[]`, `[]`)
	got := `{"address":{"city":"<value>","country":"<value>","iso_country_code":"<value>","postal_code":"<value>","state_province":"<value>",` +
		`"street":"<value>","sub_administrative_area":"<value>","sub_locality":"<value>"},"address_str":"<value>","country_code":"<value>",` +
		`"ishome":"<value>","name":"<value>",` + names(`["<value>"]`, `["<value>",""]`, `["<value>"]`)
	for _, line := range []string{
		"place " + selectedFrame + " want=" + want + " got=" + got + "\n",
		`stale place ` + emptyPhoto + ` want={"ishome":"<value>","name":"<value>","names":{"city":["<value>",""]}} got=*` + "\n",
		"stale place " + selectedFrame + " want=<unparsed> got=*\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails lacks %q:\n%s", line, b.String())
		}
	}
}

func TestWriteDetailsComments(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Comments[selectedFrame][0].Text = "SECRET-TEXT"
	lib.Likes[selectedFrame][0].User = "SECRET-NAME"
	unparsed := Divergence{Library: "lib", UUID: emptyPhoto, Field: "likes", Want: []byte(`{"user":"SECRET-STALE"}`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"comments", "likes"}) {
		t.Fatalf("differences in %v, want comments and likes", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "Person A", "Comment 1", "Comment 2"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	// comments 1 + 1, likes 1 + 1; the comment without a user keeps "".
	if n := strings.Count(b.String(), `"user":"<name>"`); n != 4 {
		t.Errorf("%d users written as <name>, want 4:\n%s", n, b.String())
	}
	if n := strings.Count(b.String(), `"text":"<text>"`); n != 4 {
		t.Errorf("%d texts written as <text>, want 4:\n%s", n, b.String())
	}
	if !strings.Contains(b.String(), `"user":""`) || !strings.Contains(b.String(), `"datetime":"2020-07-01T00:00:00Z"`) {
		t.Errorf("WriteDetails lost the empty user or the dates:\n%s", b.String())
	}
	if line := "stale likes " + emptyPhoto + " want=<unparsed> got=*\n"; !strings.Contains(b.String(), line) {
		t.Errorf("WriteDetails has no line %q:\n%s", line, b.String())
	}
}

func TestWriteDetailsFaceInfo(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Faces[selectedFrame][0].Name = "SECRET-NAME"
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "face_info", Want: []byte(`[{"name":"SECRET-STALE","uuid":null}]`), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: selectedFrame, Field: "face_info", Want: []byte(`{"name":"SECRET-OBJECT"}`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"face_info"}) {
		t.Fatalf("differences in %v, want face_info", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "Person A"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	if n := strings.Count(b.String(), `"name":"<name>"`); n != 3 {
		t.Errorf("%d names written as <name>, want 3:\n%s", n, b.String())
	}
	if !strings.Contains(b.String(), faceA) || !strings.Contains(b.String(), `"uuid":null`) {
		t.Errorf("WriteDetails lost the UUIDs or the nulls:\n%s", b.String())
	}
	if line := "stale face_info " + selectedFrame + " want=<unparsed> got=*\n"; !strings.Contains(b.String(), line) {
		t.Errorf("WriteDetails has no line %q:\n%s", line, b.String())
	}
}

// Upstream joins the library path to each preview (photoinfo.py:1200-1205),
// and the adjustments hold the edits of the owner (adjustmentsinfo.py:210-228).
func TestWriteDetailsPathsAndAdjustments(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Override = map[string]map[string]any{selectedFrame: {
		"path_derivatives": []string{"/Users/SECRET/Pictures/x.photoslibrary/resources/derivatives/0/IMG_0001_1_105_c.jpeg"},
		"adjustments":      map[string]any{"editor": "SECRET-EDITOR", "adjustments": []any{"SECRET-OP"}},
	}}
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "path_derivatives", Want: []byte(`["/SECRET-STALE"]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale)
	if got := diffKeys(rep); !slices.Equal(got, []string{"adjustments", "path_derivatives"}) {
		t.Fatalf("differences in %v, want adjustments and path_derivatives", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "SECRET") {
		t.Errorf("WriteDetails writes a path or an edit:\n%s", b.String())
	}
	for _, line := range []string{
		"path_derivatives " + selectedFrame + ` want=[] got=["<value>"]` + "\n",
		"stale path_derivatives " + emptyPhoto + ` want=["<value>"] got=*` + "\n",
		"adjustments " + selectedFrame + ` want={} got={"adjustments":["<value>"],"editor":"<value>"}` + "\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails lacks %q:\n%s", line, b.String())
		}
	}
}

func TestCompareEquivalent(t *testing.T) {
	t.Parallel()
	cases := []mutation{
		{"Date in the zone of the same offset", photo(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.In(time.FixedZone("X", 19800)) }), ""},
		{"DateAdded in UTC", photo(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(x.DateAdded.UTC()) }), ""},
		{"albums in another order, a repeat", func(l *goLib) {
			l.Photos[0].AlbumUUIDs = []string{albumSharedS, albumAInFolderX, albumAInFolderX, albumTrashed, albumAInFolderY, albumSharedTitledA, albumAInFolderY}
			l.BurstAlbums[selectedFrame] = []string{albumAInFolderY, albumSharedS, albumAInFolderX, albumBNoFolder}
		}, ""},
		{"an explicit 2001 for the creation date that osxphotos defaults", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.CreationDate = ptr(utc(2001, 1, 1, 0, 0, 0, 0)) }), ""},
		{"the unselected frame with another pick", func(l *goLib) { l.Photos[1].BurstPickType = 4 }, ""},
		{"an album of a trashed state not listed", album(albumOfNoPhoto, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), ""},
		// json() sorts the pair (photoinfo.py:2279-2285): a Cyrillic name follows its bundle ID.
		{"an imported_by name after its bundle ID in byte order", photo(func(x *photoinfo.PhotoInfo) {
			x.ImportedByName, x.ImportedByBundleID = x.ImportedByBundleID, x.ImportedByName
		}), ""},
		{"projects in another order, a repeat", photo(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed, projectP, projectP} }), ""},
		// osxphotos writes the parent of a project as null when the parent row holds no folder or user album.
		{"the parent of a project is not compared", func(l *goLib) { l.Projects[0].ParentUUID = "" }, ""},
		// osxphotos writes these dates in the zone of the computer (photos_datetime.py:70-89).
		{"a comment date in another zone, the same moment", func(l *goLib) {
			c := &l.Comments[selectedFrame][0]
			c.Datetime = ptr(c.Datetime.In(time.FixedZone("X", -7*3600)))
		}, ""},
		// The reference holds a naive UTC datetime and a double, whatever the Go types are.
		{"a cloud metadata date in another zone, the same moment", cloudMeta(func(m map[string]any) {
			m["when"] = utc(2020, 1, 2, 3, 4, 5, 6000).In(time.FixedZone("X", 19800))
		}), ""},
		{"a cloud metadata real of four bytes and an integer of another type", cloudMeta(func(m map[string]any) {
			m["ratio"], m["count"] = float32(0.5), int64(3)
		}), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			c.mutate(&lib)
			rep := compare(t, fixtureRef(), lib)
			if want := strings.Fields(c.want); !slices.Equal(diffKeys(rep), want) {
				t.Errorf("differences in %v, want %v", diffKeys(rep), want)
			}
		})
	}

	t.Run("quotes, HTML characters and offsets with seconds in the reference", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].Title, lib.Photos[0].Keywords = `x"y`, []string{"<&>", `x"y`}
		for date, offset := range map[string]int{`"2020-06-06T15:49:00.725564+01:33:36"`: 5616, `"2020-06-06T12:41:48.725564-01:33:36"`: -5616} {
			lib.Photos[0].Date = lib.Photos[0].Date.In(time.FixedZone("", offset))
			ref := refOf(refObj(refSelectedFrame(), map[string]string{"date": date, "title": `"x\"y"`, "keywords": `["<&>", "x\"y"]`}), refObj(refEmptyPhoto()))
			if rep := compare(t, ref, lib); !rep.OK() {
				t.Errorf("date %s: differences %+v, want none", date, rep.Diffs)
			}
		}
	})

	// Upstream sorts by name only, equal names in the order of SQLite (photoinfo.py:605-611).
	t.Run("persons of one name in the order of SQLite", func(t *testing.T) {
		t.Parallel()
		const other = "00000000-0000-0000-0000-0000000000d7"
		unknown := func(uuid string) string {
			return `{"displayname": null, "facecount": null, "favorite": false, "feature_less": false, "keyface": null, "keyphoto": null, ` +
				`"name": "_UNKNOWN_", "sort_order": null, "uuid": "` + uuid + `"}`
		}
		ref := refOf(refObj(refSelectedFrame()), refObj(refEmptyPhoto(), map[string]string{"person_info": "[" + unknown(other) + ", " + unknown(personUnknown) + "]"}))
		lib := fixtureLib()
		lib.Persons[emptyPhoto] = []personinfo.PersonInfo{{UUID: personUnknown, Name: "_UNKNOWN_"}, {UUID: other, Name: "_UNKNOWN_"}}
		if rep := compare(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none: equal names are compared by uuid", rep.Diffs)
		}
	})

	// Upstream sorts by name only, equal names in the order of SQLite (photoinfo.py:613-623).
	t.Run("faces of one name in the order of SQLite", func(t *testing.T) {
		t.Parallel()
		face21 := strings.TrimSuffix(strings.TrimPrefix(refFaceInfo, "["), "]")
		face22 := strings.Replace(face21, `"_pk": 21`, `"_pk": 22`, 1)
		withFaces := func(faces ...string) *Reference {
			return refOf(refObj(refSelectedFrame(), map[string]string{"face_info": "[" + strings.Join(faces, ", ") + "]"}), refObj(refEmptyPhoto()))
		}
		lib := fixtureLib()
		second := lib.Faces[selectedFrame][0]
		second.PK = 22
		lib.Faces[selectedFrame] = append(lib.Faces[selectedFrame], second)
		if rep := compare(t, withFaces(face22, face21), lib); !rep.OK() {
			t.Errorf("differences %+v, want none: equal names are compared by _pk", rep.Diffs)
		}
		lib.Faces[selectedFrame][1].Name = "Person B"
		named := strings.Replace(face22, `"name": "Person A"`, `"name": "Person B"`, 1)
		if got := diffKeys(compare(t, withFaces(named, face21), lib)); !slices.Equal(got, []string{"face_info"}) {
			t.Errorf("differences in %v, want face_info: faces of other names keep their order", got)
		}
	})

	// json() sorts the list (photoinfo.py:2279-2285); upstream gives it by size,
	// largest first (photoinfo.py:1174).
	t.Run("path_derivatives by size against the sorted reference", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].PathDerivatives = []string{"b.jpeg", "a.jpeg"}
		ref := refOf(refObj(refSelectedFrame(), map[string]string{"path_derivatives": `["a.jpeg", "b.jpeg"]`}), refObj(refEmptyPhoto()))
		if rep := compare(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})

	t.Run("an import session without a title and a creation date", func(t *testing.T) {
		t.Parallel()
		bare := `{"creation_date": "2001-01-01T00:00:00+00:00", "end_date": null, "start_date": null, "title": null, "uuid": "` + importBare + `"}`
		ref := refOf(refObj(refSelectedFrame()), refObj(refEmptyPhoto(), map[string]string{"import_info": bare}))
		lib := fixtureLib()
		lib.Photos[2].ImportUUID = importBare
		if rep := compare(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none: null is the empty title, 2001-01-01 the missing creation date", rep.Diffs)
		}
		lib.Imports[1].CreationDate = ptr(utc(2001, 1, 1, 0, 0, 0, 0))
		if rep := compare(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none: an explicit 2001-01-01 is the default", rep.Diffs)
		}
	})

	t.Run("orientation of an adjusted photo is skipped, and compared when not adjusted", func(t *testing.T) {
		t.Parallel()
		adjusted := refObj(refSelectedFrame(), map[string]string{"hasadjustments": "true", "orientation": "0"})
		lib := fixtureLib()
		lib.Photos[0].HasAdjustments, lib.Photos[0].Orientation = true, 8
		if rep := compare(t, refOf(adjusted, refObj(refEmptyPhoto())), lib); !rep.OK() {
			t.Errorf("differences %v, want none: the orientation of an adjusted photo is not compared", rep.Diffs)
		}
		// The reference decides: not adjusted there, so the orientation counts.
		plain := refObj(refSelectedFrame(), map[string]string{"orientation": "0"})
		rep := compare(t, refOf(plain, refObj(refEmptyPhoto())), lib)
		if got := diffKeys(rep); !slices.Equal(got, []string{"hasadjustments", "orientation"}) {
			t.Errorf("differences in %v, want hasadjustments and orientation", got)
		}
	})
}

func TestCompareKeepsInput(t *testing.T) {
	t.Parallel()
	spoil := func(l goLib) Library {
		l.Photos[0].Keywords = []string{"beta", "alpha"} // unsorted lists that a careless sort would reorder
		l.Photos[0].AlbumUUIDs = []string{albumAInFolderY, albumAInFolderX, albumAInFolderY}
		l.Photos[0].ImportedByName, l.Photos[0].ImportedByBundleID = "z", "a"
		l.Photos[0].Owner = "" // nil, which a careless rule would set to ""
		return l.lib()
	}
	lib, ref := spoil(fixtureLib()), fixtureRef()
	divs := []Divergence{{Library: "lib", UUID: "*", Field: "title", Want: []byte(`"x"`), Got: []byte(`"y"`)}}
	if _, err := Compare(ref, lib, Keys(), divs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lib, spoil(fixtureLib())) {
		t.Error("Compare changed the Library")
	}
	if !reflect.DeepEqual(ref, fixtureRef()) {
		t.Error("Compare changed the Reference")
	}
}

func TestCompareNulls(t *testing.T) {
	t.Parallel()
	onEmptyPhoto := func(f func(x *photoinfo.PhotoInfo)) func(l *goLib) { return func(l *goLib) { f(l.Photos[2]) } }
	cases := []mutation{
		{"nothing", onEmptyPhoto(func(*photoinfo.PhotoInfo) {}), ""},
		{"Title", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Title = "x" }), "title"},
		{"Description", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Description = "x" }), "description"},
		{"CloudMasterGUID", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.CloudMasterGUID = "x" }), "cloud_guid"},
		{"Fingerprint", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Fingerprint = "x" }), "fingerprint"},
		{"UTIRaw", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.UTIRaw = "x" }), "uti_raw"},
		{"UTIEdited", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.UTIEdited = "x" }), "uti_edited"},
		{"Width", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Width = 1 }), "width"},
		{"Height", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Height = 1 }), "height"},
		{"OriginalWidth", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.OriginalWidth = 1 }), "original_width"},
		{"OriginalHeight", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.OriginalHeight = 1 }), "original_height"},
		{"OriginalOrientation", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.OriginalOrientation = 1 }), "original_orientation"},
		{"OriginalFilesize", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.OriginalFilesize = 1 }), "original_filesize"},
		{"Orientation", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Orientation = 1 }), "orientation"},
		{"Spatial", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Spatial = 1 }), "spatial"},
		{"TZName", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.TZName = "x" }), "tzname"},
		{"CloudOwnerHashedID", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.CloudOwnerHashedID = "x" }), "cloud_owner_hashed_id"},
		{"AdjustmentType 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.AdjustmentType = ptr(0) }), "adjustment_type"},
		{"ImportedByName", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ImportedByName = "x" }), "imported_by"},
		{"ImportedByBundleID", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ImportedByBundleID = "x" }), "imported_by"},
		{"InCloud false is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.InCloud = ptr(false) }), "incloud"},
		{"TZOffset 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.TZOffset = ptr(0) }), "tzoffset"},
		{"Latitude 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(0.0) }), "latitude location"},
		{"Longitude 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Longitude = ptr(0.0) }), "location longitude"},
		{"DateModified at the epoch is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateModified = ptr(time.Unix(0, 0)) }), "date_modified"},
		{"DateTrashed at the epoch is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateTrashed = ptr(time.Unix(0, 0)) }), "date_trashed"},
		{"DateAdded at the epoch is the default", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(time.Unix(0, 0)) }), ""},
		{"DateAdded", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(time.Unix(1, 0)) }), "date_added"},
		{"Keywords", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"x"} }), "keywords"},
		{"Keywords empty, not nil", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{} }), ""},
		{"Persons", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Persons = []string{"_UNKNOWN_"} }), "persons"},
		{"Persons empty, not nil", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Persons = []string{} }), ""},
		{"AlbumUUIDs of a trashed album", func(l *goLib) { l.Photos[2].AlbumUUIDs = []string{albumTrashed} }, ""},
		{"AlbumUUIDs", func(l *goLib) { l.Photos[2].AlbumUUIDs = []string{albumBNoFolder} }, "album_info albums folders"},
		{"ImportUUID", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ImportUUID = importBare }), "import_info"},
		{"ProjectUUIDs of a trashed project", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed} }), ""},
		{"ProjectUUIDs", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectP} }), "project_info"},
		{"Comments empty, not nil", func(l *goLib) { l.Comments[emptyPhoto] = []photoinfo.CommentInfo{} }, ""},
		{"Likes", func(l *goLib) { l.Likes[emptyPhoto] = []photoinfo.LikeInfo{{}} }, "likes"},
		{"PersonInfo", func(l *goLib) { l.Persons[emptyPhoto] = []personinfo.PersonInfo{{Name: "_UNKNOWN_"}} }, "person_info"},
		{"PersonInfo empty, not nil", func(l *goLib) { l.Persons[emptyPhoto] = []personinfo.PersonInfo{} }, ""},
		{"Owner", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Owner = "x" }), "owner"},
		{"DateOriginal", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateOriginal = time.Unix(1, 0).UTC() }), "date_original"},
		{"ExifInfo FlashFired false is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.FlashFired = ptr(false) }), "exif_info"},
		{"ExifInfo ISO 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.ISO = ptr(0) }), "exif_info"},
		{"ExifInfo Latitude 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Latitude = ptr(0.0) }), "exif_info"},
		{"ExifInfo CameraMake", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.CameraMake = "x" }), "exif_info"},
		{"ExifInfo Date at the epoch is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Date = ptr(time.Unix(0, 0).UTC()) }), "exif_info"},
		{"ExifInfo TZOffset 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.TZOffset = ptr(0) }), "exif_info"},
		{"Score Overall", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Score.Overall = ptr(0.5) }), "score"},
		{"Score Behavioral", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Score.Behavioral = ptr(0.5) }), "score"},
		{"Score Overall None is not 0.0", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Score.Overall = nil }), "score"},
		{"Place zero is not {}", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Place = &placeinfo.PlaceInfo{} }), "place"},
		{"SearchInfo without records", func(l *goLib) { l.SearchInfo[emptyPhoto] = searchinfo.New([]searchinfo.Record{}) }, ""},
		{"SearchInfo of one term", func(l *goLib) {
			l.SearchInfo[emptyPhoto] = searchinfo.New([]searchinfo.Record{{Category: searchinfo.CategoryCity, Content: "x", Normalized: "x"}})
		}, "search_info search_info_normalized"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			c.mutate(&lib)
			rep := compare(t, fixtureRef(), lib)
			want := strings.Fields(c.want)
			slices.Sort(want)
			if got := diffKeys(rep); !slices.Equal(got, want) {
				t.Errorf("differences in %v, want %v", got, want)
			}
		})
	}
}

func TestCompareSets(t *testing.T) {
	t.Parallel()
	const only = "00000000-0000-0000-0000-0000000000e1"
	emptyGo := func(uuid string) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{
			UUID: uuid, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG", Date: time.Unix(0, 0).UTC(), DateOriginal: time.Unix(0, 0).UTC(),
			UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true, Score: noScoreRow(),
		}
	}
	withUUID := func(uuid string, o ...map[string]string) Object {
		return refObj(refEmptyPhoto(), append([]map[string]string{{"uuid": `"` + uuid + `"`}}, o...)...)
	}

	type setCase struct {
		name  string
		ref   func() *Reference
		lib   func() goLib
		diffs []Diff
	}
	cases := []setCase{
		{"an object only in the reference",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(only)); return r },
			fixtureLib,
			[]Diff{{UUID: only, Field: "uuid", Want: "true", Got: "false"}}},
		{"an object only in Go",
			fixtureRef,
			func() goLib { l := fixtureLib(); l.Photos = append(l.Photos, emptyGo(only)); return l },
			[]Diff{{UUID: only, Field: "uuid", Want: "false", Got: "true"}}},
		{"an object only in deleted.json of the reference",
			func() *Reference {
				r := fixtureRef()
				r.Deleted = append(r.Deleted, withUUID(only, map[string]string{"intrash": "true"}))
				return r
			},
			func() goLib {
				l := fixtureLib()
				x := emptyGo(only)
				x.InTrash = true
				l.Photos = append(l.Photos, x)
				return l
			},
			nil},
		{"the same object in deleted.json when Go has no trash flag",
			func() *Reference {
				r := fixtureRef()
				r.Deleted = append(r.Deleted, withUUID(only, map[string]string{"intrash": "true"}))
				return r
			},
			func() goLib { l := fixtureLib(); l.Photos = append(l.Photos, emptyGo(only)); return l },
			[]Diff{{UUID: only, Field: "intrash", Want: "true", Got: "false"}}},
		{"an unselected burst frame only in Go",
			fixtureRef,
			func() goLib {
				l := fixtureLib()
				l.Photos = append(l.Photos, &photoinfo.PhotoInfo{UUID: only, IsPhoto: true, Burst: true, BurstUUID: "BURST-B", BurstPickType: 4})
				return l
			},
			nil},
		{"an object that is neither a photo nor a movie, only in Go",
			fixtureRef,
			func() goLib {
				l := fixtureLib()
				x := emptyGo(only)
				x.IsPhoto = false
				l.Photos = append(l.Photos, x)
				return l
			},
			nil},
		{"an object that is neither a photo nor a movie, in both",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(only)); return r },
			func() goLib {
				l := fixtureLib()
				x := emptyGo(only)
				x.IsPhoto = false
				l.Photos = append(l.Photos, x)
				return l
			},
			[]Diff{{UUID: only, Field: "uuid", Want: "true", Got: "false"}}},
		{"a selected frame turned unselected",
			fixtureRef,
			func() goLib { l := fixtureLib(); l.Photos[0].BurstPickType = 4; return l },
			[]Diff{{UUID: selectedFrame, Field: "uuid", Want: "true", Got: "false"}}},
		{"a frame without a pick is in the set",
			fixtureRef,
			func() goLib { l := fixtureLib(); l.Photos[0].BurstPickType = 0; return l },
			nil},
		{"the key frame is in the set",
			fixtureRef,
			func() goLib { l := fixtureLib(); l.Photos[0].BurstPickType = 16; return l },
			nil},
		{"a photo that is not a photo",
			fixtureRef,
			func() goLib { l := fixtureLib(); l.Photos[0].IsPhoto = false; return l },
			[]Diff{{UUID: selectedFrame, Field: "uuid", Want: "true", Got: "false"}}},
		{"a movie",
			func() *Reference {
				r := fixtureRef()
				r.Query[1] = withUUID(emptyPhoto, map[string]string{"isphoto": "false", "ismovie": "true"})
				return r
			},
			func() goLib { l := fixtureLib(); l.Photos[2].IsPhoto, l.Photos[2].IsMovie = false, true; return l },
			nil},
		{"the reference has an unselected frame",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(unselectedFrame)); return r },
			fixtureLib,
			[]Diff{{UUID: unselectedFrame, Field: "uuid", Want: "true", Got: "false"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rep := compare(t, c.ref(), c.lib())
			if !slices.Equal(rep.Diffs, c.diffs) {
				t.Errorf("Diffs = %+v, want %+v", rep.Diffs, c.diffs)
			}
		})
	}
}

func TestCompareErrors(t *testing.T) {
	t.Parallel()
	const (
		marker       = "SECRET-VALUE"
		wantComments = "query.json element 1: comments: want a list of comments as osxphotos writes them"
	)
	withValue := func(key, value string) func() *Reference {
		return func() *Reference { r := fixtureRef(); r.Query[0][key] = []byte(value); return r }
	}
	cases := []struct {
		name string
		ref  func() *Reference
		lib  func() goLib
		keys []string
		want string
	}{
		{"unknown key", fixtureRef, fixtureLib, []string{"title", "nonsense"}, `unknown key "nonsense"`},
		{"a UUID repeated in Library.Photos", fixtureRef, func() goLib { l := fixtureLib(); l.Photos = append(l.Photos, l.Photos[0]); return l },
			[]string{"title"}, "UUID is repeated in Library.Photos"},
		{"a UUID repeated in query.json", func() *Reference { r := fixtureRef(); r.Query = append(r.Query, r.Query[0]); return r }, fixtureLib,
			[]string{"title"}, "query.json element 3: the uuid repeats that of query.json element 1"},
		{"a UUID in both files", func() *Reference { r := fixtureRef(); r.Deleted = []Object{r.Query[1]}; return r }, fixtureLib,
			[]string{"title"}, "deleted.json element 1: the uuid repeats that of query.json element 2"},
		{"no uuid in the reference", func() *Reference { r := fixtureRef(); delete(r.Query[0], "uuid"); return r }, fixtureLib,
			[]string{"title"}, "query.json element 1: uuid: want a value"},
		{"uuid is not a string", func() *Reference { r := fixtureRef(); r.Query[0]["uuid"] = []byte(`7`); return r }, fixtureLib,
			[]string{"title"}, "query.json element 1: uuid: want a non-empty string"},
		{"uuid is an object with a marker", func() *Reference { r := fixtureRef(); r.Query[0]["uuid"] = []byte(`{"k": "` + marker + `"}`); return r }, fixtureLib,
			[]string{"title"}, "query.json element 1: uuid: want a non-empty string"},
		{"a boolean that is a string with a marker", func() *Reference { r := fixtureRef(); r.Query[1]["favorite"] = []byte(`"` + marker + `"`); return r }, fixtureLib,
			[]string{"favorite"}, "query.json element 2: favorite: want a boolean"},
		{"a boolean that is null", func() *Reference { r := fixtureRef(); r.Query[1]["favorite"] = []byte(`null`); return r }, fixtureLib,
			[]string{"favorite"}, "query.json element 2: favorite: want a boolean"},
		{"a malformed value of a key that is not compared", func() *Reference { r := fixtureRef(); r.Query[0]["width"] = []byte(`"x"`); return r }, fixtureLib, []string{"title"}, ""},
		{"a date that is not a date", func() *Reference { r := fixtureRef(); r.Query[0]["date"] = []byte(`"` + marker + `"`); return r }, fixtureLib,
			[]string{"date"}, "date: want an ISO 8601 string"},
		{"an integer that is a string", func() *Reference { r := fixtureRef(); r.Query[0]["width"] = []byte(`"` + marker + `"`); return r }, fixtureLib,
			[]string{"width"}, "width: want an integer"},
		{"an integer that is a float", func() *Reference { r := fixtureRef(); r.Query[0]["width"] = []byte(`3024.0`); return r }, fixtureLib,
			[]string{"width"}, "width: want an integer"},
		{"an integer out of range", func() *Reference { r := fixtureRef(); r.Query[0]["width"] = []byte(`99999999999999999999`); return r }, fixtureLib,
			[]string{"width"}, "width: want an integer"},
		{"a string that is a number", func() *Reference { r := fixtureRef(); r.Query[0]["filename"] = []byte(`5`); return r }, fixtureLib,
			[]string{"filename"}, "filename: want a string"},
		{"a filename that is null", func() *Reference { r := fixtureRef(); r.Query[0]["filename"] = []byte(`null`); return r }, fixtureLib,
			[]string{"filename"}, "filename: want a string"},
		{"a list of strings with a number", func() *Reference { r := fixtureRef(); r.Query[0]["keywords"] = []byte(`["a", 5]`); return r }, fixtureLib,
			[]string{"keywords"}, "keywords: want a list of strings"},
		{"a location with three numbers", func() *Reference { r := fixtureRef(); r.Query[0]["location"] = []byte(`[1, 2, 3]`); return r }, fixtureLib,
			[]string{"location"}, "location: want a list of two"},
		{"a latitude that is a string", func() *Reference { r := fixtureRef(); r.Query[0]["latitude"] = []byte(`"` + marker + `"`); return r }, fixtureLib,
			[]string{"latitude"}, "latitude: want a number or null"},
		{"an album that is not an object", func() *Reference { r := fixtureRef(); r.Query[0]["album_info"] = []byte(`[5]`); return r }, fixtureLib,
			[]string{"album_info"}, "album_info[0]: want an object"},
		{"an album without a title key", func() *Reference { r := fixtureRef(); r.Query[0]["album_info"] = []byte(`[{"uuid": "x"}]`); return r }, fixtureLib,
			[]string{"album_info"}, "album_info[0].title: want a value"},
		{"an owner that is not a string", func() *Reference {
			r := fixtureRef()
			r.Query[0]["album_info"] = []byte("[" + albumJSON(albumSharedS, `"Shared S"`, "null", "[]", "null", "null", "null", `{"k": "`+marker+`"}`) + "]")
			return r
		}, fixtureLib, []string{"album_info"}, "album_info[0]: want an album with values of the types osxphotos writes"},
		{"a malformed album_info is not read for albums", func() *Reference { r := fixtureRef(); r.Query[0]["album_info"] = []byte(`[5]`); return r }, fixtureLib,
			[]string{"albums", "folders"}, ""},
		{"an album without folder_list", func() *Reference {
			r := fixtureRef()
			r.Query[0]["album_info"] = []byte("[" + strings.Replace(elemSharedS, `"folder_list": [], `, ``, 1) + "]")
			return r
		}, fixtureLib, []string{"album_info"}, "album_info[0].folder_list: want a value (the key is missing)"},
		{"a folder_list that is null", func() *Reference {
			r := fixtureRef()
			r.Query[0]["album_info"] = []byte("[" + strings.Replace(elemSharedS, `"folder_list": []`, `"folder_list": null`, 1) + "]")
			return r
		}, fixtureLib, []string{"album_info"}, "album_info[0].folder_list: want a list of strings"},
		{"face_info that is an object", withValue("face_info", `{"name": "`+marker+`"}`), fixtureLib, []string{"face_info"},
			"query.json element 1: face_info: want a list of objects"},
		{"a face that is a string", withValue("face_info", `["`+marker+`"]`), fixtureLib, []string{"face_info"},
			"query.json element 1: face_info: want a list of objects"},
		{"adjustments that is null", withValue("adjustments", `null`), fixtureLib, []string{"adjustments"},
			"query.json element 1: adjustments: want an object"},
		{"a rating that is a float", withValue("rating", `0.0`), fixtureLib, []string{"rating"}, "query.json element 1: rating: want an integer"},
		{"path_derivatives with a number", withValue("path_derivatives", `[5]`), fixtureLib, []string{"path_derivatives"},
			"query.json element 1: path_derivatives: want a list of strings"},
		{"folders that is a list", func() *Reference { r := fixtureRef(); r.Query[0]["folders"] = []byte(`[]`); return r }, fixtureLib,
			[]string{"folders"}, "folders: want an object"},
		{"a hasadjustments that is null while orientation is compared", func() *Reference { r := fixtureRef(); r.Query[0]["hasadjustments"] = []byte(`null`); return r }, fixtureLib,
			[]string{"orientation"}, "hasadjustments: want a boolean"},
		{"an object only in the reference is checked too", func() *Reference {
			r := fixtureRef()
			r.Query = append(r.Query, refObj(refEmptyPhoto(), map[string]string{"uuid": `"00000000-0000-0000-0000-0000000000e1"`, "width": `"` + marker + `"`}))
			return r
		}, fixtureLib, []string{"width"}, "query.json element 3: width: want an integer"},
		{"NaN from Go", fixtureRef, func() goLib { l := fixtureLib(); l.Photos[0].Latitude = ptr(math.NaN()); return l },
			[]string{"latitude"}, "key latitude: the Go value cannot be written as JSON"},
		{"an import_info that is a list", func() *Reference { r := fixtureRef(); r.Query[0]["import_info"] = []byte(`[]`); return r }, fixtureLib,
			[]string{"import_info"}, "query.json element 1: import_info: want an object"},
		{"an import_info without a title key", func() *Reference { r := fixtureRef(); r.Query[0]["import_info"] = []byte(`{"uuid": "x"}`); return r }, fixtureLib,
			[]string{"import_info"}, "import_info.title: want a value"},
		{"an import date that is not a date", func() *Reference {
			r := fixtureRef()
			r.Query[0]["import_info"] = []byte(`{"uuid": "x", "title": null, "creation_date": "` + marker + `", "start_date": null, "end_date": null}`)
			return r
		}, fixtureLib, []string{"import_info"}, "import_info.creation_date: want an ISO 8601 string"},
		{"a comment with a null datetime", withValue("comments", `[{"datetime": null, "user": "`+marker+`", "ismine": false, "text": "x"}]`),
			fixtureLib, []string{"comments"}, wantComments},
		{"a comment without a text", withValue("comments", `[{"datetime": "2020-07-01T00:00:00+00:00", "user": "`+marker+`", "ismine": false}]`),
			fixtureLib, []string{"comments"}, wantComments},
		{"a comment with a null ismine", withValue("comments", `[{"datetime": "2020-07-01T00:00:00+00:00", "user": "`+marker+`", "ismine": null, "text": "x"}]`),
			fixtureLib, []string{"comments"}, wantComments},
		{"comments that is not a list", withValue("comments", `{"text": "`+marker+`"}`), fixtureLib, []string{"comments"}, wantComments},
		{"a like with a null ismine", withValue("likes", `[{"datetime": "2020-07-03T00:00:00+00:00", "user": "`+marker+`", "ismine": null}]`),
			fixtureLib, []string{"likes"}, "query.json element 1: likes: want a list of likes as osxphotos writes them"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Compare(c.ref(), c.lib().lib(), c.keys, nil)
			if c.want == "" {
				if err != nil {
					t.Fatalf("Compare: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Compare error = %v, want it to contain %q", err, c.want)
			}
			for _, leak := range []string{marker, selectedFrame, unselectedFrame, emptyPhoto, albumAInFolderX, "00000000-0000"} {
				if strings.Contains(err.Error(), leak) {
					t.Errorf("the error %q repeats %q", err, leak)
				}
			}
		})
	}

	t.Run("an error is a shape error", func(t *testing.T) {
		t.Parallel()
		r := fixtureRef()
		r.Query[0]["favorite"] = []byte(`null`)
		_, err := Compare(r, fixtureLib().lib(), []string{"favorite"}, nil)
		var se *shapeError
		if !errors.As(err, &se) || se.path != "favorite" {
			t.Errorf("error = %v, want a *shapeError for favorite", err)
		}
	})
	t.Run("no keys compares the sets only", func(t *testing.T) {
		t.Parallel()
		r := fixtureRef()
		r.Query[0]["favorite"] = []byte(`null`) // not looked at
		rep, err := Compare(r, fixtureLib().lib(), nil, nil)
		if err != nil || !rep.OK() || len(rep.Keys) != 0 || rep.Objects != 2 {
			t.Errorf("Compare = %+v, %v; want no differences and no keys", rep, err)
		}
	})
	t.Run("a repeated key is compared once", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].Title = ""
		rep, err := Compare(fixtureRef(), lib.lib(), []string{"title", "uuid", "title"}, nil)
		if err != nil || len(rep.Diffs) != 1 || !slices.Equal(rep.Keys, []string{"title", "uuid"}) {
			t.Errorf("Compare = %+v, %v; want one difference and the keys title and uuid", rep, err)
		}
	})
	t.Run("a key compared that the dictionary of a photo lacks", func(t *testing.T) {
		t.Parallel()
		for name, spoil := range map[string]func(l Library){
			"the key":   func(l Library) { delete(l.Dicts[selectedFrame], "title") },
			"the photo": func(l Library) { delete(l.Dicts, selectedFrame) },
		} {
			lib := fixtureLib().lib()
			spoil(lib)
			_, err := Compare(fixtureRef(), lib, []string{"title"}, nil)
			want := "query.json element 1: key title: the key is missing from the dictionary of the photo in Library.Dicts"
			if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), selectedFrame) {
				t.Errorf("%s: error = %v, want %q without the UUID", name, err, want)
			}
		}
	})
}

// A rule that read the photo instead of its dictionary would not see the
// value planted there. folders also reads album_info: the reference keeps
// one album of a title, which may be any album of album_info of that title.
func TestCompareReadsDicts(t *testing.T) {
	t.Parallel()
	for _, key := range fieldsTxt(t) {
		if key == "uuid" {
			continue
		}
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib().lib()
			lib.Dicts[selectedFrame][key] = "SPOILED"
			want := []string{key}
			if key == "album_info" {
				want = []string{"album_info", "folders"}
			}
			rep, err := Compare(fixtureRef(), lib, Keys(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := diffKeys(rep); !slices.Equal(got, want) {
				t.Errorf("differences in %v, want %v", got, want)
			}
		})
	}
}

// The reference reads null as "" for these keys; a key that the object of the
// dictionary lacks is not that null.
func TestCompareMissingInnerKey(t *testing.T) {
	t.Parallel()
	item := func(key string, i int) func(d map[string]any) map[string]any {
		return func(d map[string]any) map[string]any { return d[key].([]any)[i].(map[string]any) }
	}
	for _, c := range []struct {
		key, uuid, inner string
		object           func(d map[string]any) map[string]any
	}{
		{"exif_info", emptyPhoto, "codec", func(d map[string]any) map[string]any { return d["exif_info"].(map[string]any) }},
		{"album_info", selectedFrame, "owner", item("album_info", 0)},
		{"person_info", selectedFrame, "displayname", item("person_info", 1)},
		{"comments", selectedFrame, "user", item("comments", 1)},
	} {
		t.Run(c.key, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib().lib()
			delete(c.object(lib.Dicts[c.uuid]), c.inner)
			rep, err := Compare(fixtureRef(), lib, Keys(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := diffKeys(rep); !slices.Equal(got, []string{c.key}) {
				t.Errorf("differences in %v, want %s: the dictionary lacks %s", got, c.key, c.inner)
			}
		})
	}
}

func TestCompareAlbumRules(t *testing.T) {
	t.Parallel()
	albumKeys := []string{"album_info", "albums", "burst_album_info", "burst_albums", "cloud_metadata", "comments", "folders",
		"import_info", "labels", "labels_normalized", "likes", "person_info", "project_info", "search_info", "search_info_normalized"}
	// The other photos of the library would be differences.
	run := func(t *testing.T, ref Object, lib goLib) *Report {
		t.Helper()
		id, _ := decode[string](ref, "uuid", false, "")
		lib.Photos = slices.DeleteFunc(slices.Clone(lib.Photos), func(p *photoinfo.PhotoInfo) bool { return p.UUID != id })
		rep, err := Compare(refOf(ref), lib.lib(), albumKeys, nil)
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}

	t.Run("shared albums are compared", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		if rep := run(t, refObj(refSelectedFrame()), lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	// DEFAULT_DATETIME_LOCAL is in the zone of the computer (photos_datetime.py:19-21).
	t.Run("a like without a date is the default of upstream", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Likes[selectedFrame][0].Datetime = nil
		local := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local).Format(time.RFC3339)
		ref := refObj(refSelectedFrame(), map[string]string{"likes": `[{"datetime": "` + local + `", "ismine": false, "user": "Person A"}]`})
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	t.Run("a shared album that the Go side lost", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].AlbumUUIDs = []string{albumAInFolderY, albumAInFolderX, albumSharedTitledA}
		lib.BurstAlbums[selectedFrame] = []string{albumBNoFolder, albumAInFolderX, albumAInFolderY}
		rep := run(t, refObj(refSelectedFrame()), lib)
		want := []string{"album_info", "albums", "burst_album_info", "burst_albums", "folders"}
		if got := diffKeys(rep); !slices.Equal(got, want) {
			t.Errorf("differences in %v, want %v", got, want)
		}
	})
	t.Run("a shared album without an owner", func(t *testing.T) {
		t.Parallel()
		unowned := albumJSON(albumSharedS, `"Shared S"`, "null", "[]", `"2020-03-03T03:03:03+00:00"`, "null", "null", "null")
		ref := refObj(refSelectedFrame(), map[string]string{
			"album_info":       "[" + strings.Join([]string{elemAInFolderX, elemAInFolderY, unowned, elemSharedTitledA}, ", ") + "]",
			"burst_album_info": "[" + strings.Join([]string{elemBNoFolder, elemAInFolderX, elemAInFolderY, unowned}, ", ") + "]",
		})
		lib := fixtureLib()
		if got := diffKeys(run(t, ref, lib)); !slices.Equal(got, []string{"album_info", "burst_album_info"}) {
			t.Errorf("differences in %v, want album_info and burst_album_info", got)
		}
		albumOf(&lib, albumSharedS).Owner = ""
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none: null is the empty owner", rep.Diffs)
		}
	})
	t.Run("only shared albums", func(t *testing.T) {
		t.Parallel()
		ref := refObj(refSelectedFrame(), map[string]string{
			"album_info":       "[" + elemSharedS + ", " + elemSharedTitledA + "]",
			"albums":           `["Album A", "Shared S"]`,
			"burst_album_info": "[" + elemSharedS + "]",
			"burst_albums":     `["Shared S"]`,
			"folders":          `{"Album A": [], "Shared S": []}`,
		})
		lib := fixtureLib()
		lib.Photos[0].AlbumUUIDs = []string{albumSharedTitledA, albumSharedS}
		lib.BurstAlbums[selectedFrame] = []string{albumSharedS}
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	t.Run("an album without a title", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Albums = append(lib.Albums, &albuminfo.AlbumInfo{UUID: "00000000-0000-0000-0000-0000000000a7", FolderNames: []string{"Folder X"}})
		lib.Photos[0].AlbumUUIDs = []string{"00000000-0000-0000-0000-0000000000a7"}
		lib.BurstAlbums[selectedFrame] = []string{"00000000-0000-0000-0000-0000000000a7"}
		untitled := albumJSON("00000000-0000-0000-0000-0000000000a7", "null", "null", `["Folder X"]`, `"2001-01-01T00:00:00+00:00"`, "null", "null", "null")
		ref := refObj(refSelectedFrame(), map[string]string{
			"album_info": "[" + untitled + "]", "albums": "[null]", "burst_album_info": "[" + untitled + "]",
			"burst_albums": "[null]", "folders": `{"null": ["Folder X"]}`,
		})
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none: the key null is the album without a title", rep.Diffs)
		}
		lib.Albums[len(lib.Albums)-1].FolderNames = nil
		if got := diffKeys(run(t, ref, lib)); !slices.Equal(got, []string{"album_info", "burst_album_info", "folders"}) {
			t.Errorf("differences in %v, want album_info, burst_album_info, folders", got)
		}
	})
	t.Run("two albums with one title: the reference keeps the last value", func(t *testing.T) {
		t.Parallel()
		for _, folders := range []string{`{"Album A": ["Folder X"], "Shared S": []}`, `{"Album A": [], "Shared S": []}`} {
			ref := refObj(refSelectedFrame(), map[string]string{"folders": folders})
			if rep := run(t, ref, fixtureLib()); !rep.OK() {
				t.Errorf("folders %s: differences %+v, want none", folders, rep.Diffs)
			}
		}
		ref := refObj(refSelectedFrame(), map[string]string{"folders": `{"Album A": ["Folder Q"], "Shared S": []}`})
		rep := run(t, ref, fixtureLib())
		if len(rep.Diffs) != 1 || rep.Diffs[0].Field != "folders" {
			t.Fatalf("differences %+v, want folders only", rep.Diffs)
		}
		if want := `{"Album A":["Folder Q"],"Shared S":[]}`; rep.Diffs[0].Want != want {
			t.Errorf("Want = %s, want %s", rep.Diffs[0].Want, want)
		}
		// With no value to match, folders of the dictionary is shown: the last album of the title, the shared one.
		if want := `{"Album A":[],"Shared S":[]}`; rep.Diffs[0].Got != want {
			t.Errorf("Got = %s, want %s", rep.Diffs[0].Got, want)
		}
	})
	t.Run("a repeated album in the reference is one", func(t *testing.T) {
		t.Parallel()
		ref := refObj(refSelectedFrame(), map[string]string{"album_info": "[" + strings.Join([]string{elemAInFolderX, elemAInFolderX, elemAInFolderY, elemSharedS, elemSharedTitledA}, ", ") + "]"})
		if rep := run(t, ref, fixtureLib()); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	t.Run("albums are sorted by code points", func(t *testing.T) {
		t.Parallel()
		// In code point order (and in UTF-8 order) U+FF5E comes before U+1F600; in
		// UTF-16 order it comes after, because the emoji is a pair of surrogates.
		const b1, b2 = "00000000-0000-0000-0000-0000000000b1", "00000000-0000-0000-0000-0000000000b2"
		lib := goLib{
			Photos: []*photoinfo.PhotoInfo{{UUID: emptyPhoto, IsPhoto: true, AlbumUUIDs: []string{b2, b1}}},
			Albums: []*albuminfo.AlbumInfo{{UUID: b1, Title: "\U0001F600"}, {UUID: b2, Title: "～"}},
		}
		ref := refObj(refEmptyPhoto(), map[string]string{
			"albums": `["～", "😀"]`,
			"album_info": "[" + albumJSON(b1, `"😀"`, "null", "[]", `"2001-01-01T00:00:00+00:00"`, "null", "null", "null") + ", " +
				albumJSON(b2, `"～"`, "null", "[]", `"2001-01-01T00:00:00+00:00"`, "null", "null", "null") + "]",
			"folders": `{"😀": [], "～": []}`,
		})
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
		swapped := refObj(refEmptyPhoto(), map[string]string{"albums": `["😀", "～"]`})
		lib.Albums = lib.Albums[:2]
		rep, err := Compare(refOf(swapped), lib.lib(), []string{"albums"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := diffKeys(rep); !slices.Equal(got, []string{"albums"}) {
			t.Errorf("differences in %v, want albums", got)
		}
	})
}

// TestCompareBurstPhotos: osxphotos groups frames by "is not None", so the
// empty BurstUUID is a burst of its own.
func TestCompareBurstPhotos(t *testing.T) {
	t.Parallel()
	const (
		q1 = "00000000-0000-0000-0000-0000000000d1"
		q2 = "00000000-0000-0000-0000-0000000000d2"
		q3 = "00000000-0000-0000-0000-0000000000d3"
	)
	frame := func(uuid string) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{
			UUID: uuid, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG", Date: time.Unix(0, 0),
			UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true,
		}
	}
	a, b, c := frame(q1), frame(q2), frame(q3)
	a.Burst, b.Burst = true, true // BurstUUID "" for both: "" is a burst too
	ref := refOf(
		refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + q1 + `"`, "burst": "true", "burst_photos": `["` + q2 + `"]`}),
		refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + q2 + `"`, "burst": "true", "burst_photos": `["` + q1 + `"]`}),
		refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + q3 + `"`}),
	)
	rep, err := Compare(ref, goLib{Photos: []*photoinfo.PhotoInfo{c, b, a}}.lib(), []string{"burst", "burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("differences %+v, want none", rep.Diffs)
	}
	own := refOf(
		refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + q1 + `"`, "burst": "true", "burst_photos": `["` + q1 + `", "` + q2 + `"]`}),
	)
	rep, err = Compare(own, goLib{Photos: []*photoinfo.PhotoInfo{a, b}}.lib(), []string{"burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := diffKeys(rep); !slices.Equal(got, []string{"burst_photos", "uuid"}) { // uuid: q2 is not in the reference
		t.Errorf("differences in %v, want burst_photos: the frame must not list itself", got)
	}
	b.UUID = "Z"
	c.UUID, c.Burst = "a", true
	a.UUID = "B"
	c.BurstUUID, b.BurstUUID, a.BurstUUID = "x", "x", "x"
	burst := func(uuid, others string) Object {
		return refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + uuid + `"`, "burst": "true", "burst_photos": others})
	}
	sorted := refOf(burst("B", `["Z", "a"]`), burst("Z", `["B", "a"]`), burst("a", `["B", "Z"]`))
	rep, err = Compare(sorted, goLib{Photos: []*photoinfo.PhotoInfo{a, c, b}}.lib(), []string{"burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("differences %+v, want none: UUIDs sort as bytes", rep.Diffs)
	}
}
