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
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
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
	if rep.Library != "lib" || rep.Objects != 2 || len(rep.Keys) != 80 {
		t.Errorf("Library = %q, Objects = %d, Keys = %d; want lib, 2, 80", rep.Library, rep.Objects, len(rep.Keys))
	}
	if !slices.IsSorted(rep.Keys) {
		t.Error("Keys are not sorted")
	}
}

type mutation struct {
	name   string
	mutate func(l *Library)
	want   string
}

func photo(f func(p *photoinfo.PhotoInfo)) func(l *Library) {
	return func(l *Library) { f(l.Photos[0]) }
}

func albumOf(l *Library, uuid string) *albuminfo.AlbumInfo {
	for _, a := range slices.Concat(l.Albums, l.SharedAlbums) {
		if a.UUID == uuid {
			return a
		}
	}
	panic("no album " + uuid)
}

func album(uuid string, f func(a *albuminfo.AlbumInfo)) func(l *Library) {
	return func(l *Library) { f(albumOf(l, uuid)) }
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

		{"Place nil", p(func(x *photoinfo.PhotoInfo) { x.Place = nil }), "place"},
		{"Place Name", p(func(x *photoinfo.PhotoInfo) { x.Place.Name = "Townsville" }), "place"},
		{"Place City swapped", p(func(x *photoinfo.PhotoInfo) { x.Place.Names.City = []string{"Greater Townsville", "Townsville"} }), "place"},
		{"Place Field13 of Field15", p(func(x *photoinfo.PhotoInfo) {
			x.Place.Names.Field13, x.Place.Names.Field15 = nil, x.Place.Names.Field13
		}), "place"},
		{"Place BodyOfWater", p(func(x *photoinfo.PhotoInfo) { x.Place.Names.BodyOfWater = []string{"Example Bay"} }), "place"},
		{"Place CountryCode", p(func(x *photoinfo.PhotoInfo) { x.Place.CountryCode = "" }), "place"},
		{"Place IsHome", p(func(x *photoinfo.PhotoInfo) { x.Place.IsHome = false }), "place"},
		{"Place AddressStr", p(func(x *photoinfo.PhotoInfo) { x.Place.AddressStr = "" }), "place"},
		{"Place Address ISOCountryCode", p(func(x *photoinfo.PhotoInfo) { x.Place.Address.ISOCountryCode = "" }), "place"},
		{"Place Address PostalCode", p(func(x *photoinfo.PhotoInfo) { x.Place.Address.PostalCode = "00000" }), "place"},

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

		{"BurstUUID of the other frame", func(l *Library) { l.Photos[1].BurstUUID = "BURST-B" }, "burst_photos"},
		// The other frame is then a plain photo, which osxphotos would return.
		{"the other frame is not a burst frame", func(l *Library) { l.Photos[1].Burst = false }, "burst_photos uuid"},
		{"BurstUUID of the frame", p(func(x *photoinfo.PhotoInfo) { x.BurstUUID = "BURST-B" }), "burst_photos"},
		{"a third frame, not a photo and in the trash", func(l *Library) {
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
		{"BurstAlbums empty", func(l *Library) { l.BurstAlbums[selectedFrame] = nil }, "burst_album_info burst_albums"},
		{"BurstAlbums without Album B", func(l *Library) {
			l.BurstAlbums[selectedFrame] = []string{albumAInFolderX, albumAInFolderY, albumSharedS}
		}, "burst_album_info burst_albums"},
		{"an album in the trash is listed", func(l *Library) { l.BurstAlbums[selectedFrame] = append(l.BurstAlbums[selectedFrame], albumTrashed) }, ""},
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
		{"Shared album with a parent among the user albums", func(l *Library) {
			l.SharedAlbums[0].ParentUUID = folderX
			l.Albums, l.SharedAlbums = append(l.Albums, l.SharedAlbums[0]), l.SharedAlbums[1:]
		}, "album_info burst_album_info"},

		{"ImportUUID empty", p(func(x *photoinfo.PhotoInfo) { x.ImportUUID = "" }), "import_info"},
		{"ImportUUID of another session", p(func(x *photoinfo.PhotoInfo) { x.ImportUUID = importBare }), "import_info"},
		{"Import title", func(l *Library) { l.Imports[0].Title = "Import 2" }, "import_info"},
		{"Import title empty", func(l *Library) { l.Imports[0].Title = "" }, "import_info"},
		{"Import creation date", func(l *Library) { l.Imports[0].CreationDate = nil }, "import_info"},
		{"Import start date", func(l *Library) { l.Imports[0].StartDate = nil }, "import_info"},
		{"Import end date by 1us", func(l *Library) { l.Imports[0].EndDate = ptr(l.Imports[0].EndDate.Add(time.Microsecond)) }, "import_info"},
		{"Import start and end dates swapped", func(l *Library) {
			l.Imports[0].StartDate, l.Imports[0].EndDate = l.Imports[0].EndDate, l.Imports[0].StartDate
		}, "import_info"},
		{"Import UUID", func(l *Library) { l.Imports[0].UUID, l.Imports[1].UUID = importBare, importSession }, "import_info"},

		{"ProjectUUIDs empty", p(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = nil }), "project_info"},
		{"ProjectUUIDs without the project outside the trash", p(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed} }), "project_info"},
		{"Project goes to the trash", func(l *Library) { l.Projects[0].InTrash = true }, "project_info"},
		{"Project in the trash comes out of it", func(l *Library) { l.Projects[1].InTrash = false }, "project_info"},
		{"Project title", func(l *Library) { l.Projects[0].Title = "Project Q" }, "project_info"},
		{"Project folder names", func(l *Library) { l.Projects[0].FolderNames = []string{"Folder X"} }, "project_info"},
		{"Project creation date", func(l *Library) { l.Projects[0].CreationDate = nil }, "project_info"},
		{"Project end date", func(l *Library) { l.Projects[0].EndDate = ptr(utc(2020, 4, 5, 0, 0, 0, 0)) }, "project_info"},
		{"a project among the albums of the photo", func(l *Library) {
			l.Projects = l.Projects[1:]
			l.Albums = append(l.Albums, &albuminfo.AlbumInfo{UUID: projectP, Title: "Project P", ParentUUID: folderX, CreationDate: ptr(utc(2020, 4, 4, 4, 4, 4, 0))})
			l.Photos[0].AlbumUUIDs = append(l.Photos[0].AlbumUUIDs, projectP)
			l.Photos[0].ProjectUUIDs = []string{projectTrashed}
		}, "album_info albums folders project_info"},

		{"Comments nil", func(l *Library) { l.Comments = nil }, "comments"},
		{"Comments swapped", func(l *Library) { c := l.Comments[selectedFrame]; c[0], c[1] = c[1], c[0] }, "comments"},
		{"Comment date by 1us", func(l *Library) {
			c := &l.Comments[selectedFrame][1]
			c.Datetime = ptr(c.Datetime.Add(time.Microsecond))
		}, "comments"},
		{"Comment date nil", func(l *Library) { l.Comments[selectedFrame][0].Datetime = nil }, "comments"},
		{"Comment user", func(l *Library) { l.Comments[selectedFrame][0].User = "" }, "comments"},
		{"Comment user of null", func(l *Library) { l.Comments[selectedFrame][1].User = "Person B" }, "comments"},
		{"Comment IsMine", func(l *Library) { l.Comments[selectedFrame][0].IsMine = false }, "comments"},
		{"Comment text", func(l *Library) { l.Comments[selectedFrame][1].Text = "Comment 3" }, "comments"},
		{"Comment of another photo", func(l *Library) {
			l.Comments[emptyPhoto], l.Comments[selectedFrame] = l.Comments[selectedFrame], nil
		}, "comments"},
		{"Likes nil", func(l *Library) { l.Likes = nil }, "likes"},
		{"Like date", func(l *Library) { c := &l.Likes[selectedFrame][0]; c.Datetime = ptr(c.Datetime.Add(time.Second)) }, "likes"},
		{"Like user", func(l *Library) { l.Likes[selectedFrame][0].User = "" }, "likes"},
		{"Like IsMine", func(l *Library) { l.Likes[selectedFrame][0].IsMine = true }, "likes"},
		{"a second like", func(l *Library) {
			l.Likes[selectedFrame] = append(l.Likes[selectedFrame], photoinfo.LikeInfo{Datetime: ptr(utc(2020, 7, 4, 0, 0, 0, 0))})
		}, "likes"},
	}
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

// The comparison must not normalize strings: upstream gives the title in NFC
// (unicode.py:76-92) and the strings of exif_info as SQLite returns them
// (exifinfo.py:63-66), so a Go string in another form than the reference is a
// difference to be fixed in the port.
func TestCompareNFD(t *testing.T) {
	t.Parallel()
	nfc := "Caf" + string(rune(0xe9))
	nfd := "Cafe" + string(rune(0x301))
	exif := func(cameraMake string) string {
		return strings.Replace(refExifNull, `"camera_make": null`, `"camera_make": `+jsonString(cameraMake), 1)
	}
	titleRef := map[string]string{"title": jsonString(nfc)}
	exifRef := map[string]string{"exif_info": exif(nfd)}
	for name, tc := range map[string]struct {
		ref  map[string]string
		set  func(p *photoinfo.PhotoInfo)
		want []string
	}{
		"title NFC":              {titleRef, func(p *photoinfo.PhotoInfo) { p.Title = nfc }, nil},
		"title NFD":              {titleRef, func(p *photoinfo.PhotoInfo) { p.Title = nfd }, []string{"title"}},
		"camera_make NFD kept":   {exifRef, func(p *photoinfo.PhotoInfo) { p.ExifInfo.CameraMake = nfd }, nil},
		"camera_make NFD to NFC": {exifRef, func(p *photoinfo.PhotoInfo) { p.ExifInfo.CameraMake = nfc }, []string{"exif_info"}},
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
	lib.Photos[0].Width = 7
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
		"width":              {`4032`, `7`},
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
		`"codec":"","date":"<value>"`,
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

func TestWriteDetailsPlace(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.Photos[0].Place.Names.City = []string{"SECRET-CITY"}
	lib.Photos[0].Place.Address.Street = ""
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "place", Want: []byte(`{"name":"SECRET-STALE","ishome":false}`), Reason: "r", Upstream: "a.py:1"}
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
	for _, part := range []string{
		`"city":["<value>","<value>"]`,
		`"city":["<value>"]`,
		`"street":""`,
		`"field0":[]`,
		`"ishome":"<value>"`,
		`stale place ` + emptyPhoto + ` want={"ishome":"<value>","name":"<value>"} got=*` + "\n",
		"stale place " + selectedFrame + " want=<unparsed> got=*\n",
	} {
		if !strings.Contains(b.String(), part) {
			t.Errorf("WriteDetails lacks %q:\n%s", part, b.String())
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

func TestCompareEquivalent(t *testing.T) {
	t.Parallel()
	cases := []mutation{
		{"Date in the zone of the same offset", photo(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.In(time.FixedZone("X", 19800)) }), ""},
		{"DateAdded in UTC", photo(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(x.DateAdded.UTC()) }), ""},
		{"albums in another order, a repeat", func(l *Library) {
			l.Photos[0].AlbumUUIDs = []string{albumSharedS, albumAInFolderX, albumAInFolderX, albumTrashed, albumAInFolderY, albumSharedTitledA, albumAInFolderY}
			l.BurstAlbums[selectedFrame] = []string{albumAInFolderY, albumSharedS, albumAInFolderX, albumBNoFolder}
		}, ""},
		{"an explicit 2001 for the creation date that osxphotos defaults", album(albumAInFolderY, func(a *albuminfo.AlbumInfo) { a.CreationDate = ptr(utc(2001, 1, 1, 0, 0, 0, 0)) }), ""},
		{"the unselected frame with another pick", func(l *Library) { l.Photos[1].BurstPickType = 4 }, ""},
		{"an album of a trashed state not listed", album(albumOfNoPhoto, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), ""},
		// json() sorts the pair (photoinfo.py:2279-2285): a Cyrillic name follows its bundle ID.
		{"an imported_by name after its bundle ID in byte order", photo(func(x *photoinfo.PhotoInfo) {
			x.ImportedByName, x.ImportedByBundleID = x.ImportedByBundleID, x.ImportedByName
		}), ""},
		{"projects in another order, a repeat", photo(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed, projectP, projectP} }), ""},
		// osxphotos writes the parent of a project as null when the parent row holds no folder or user album.
		{"the parent of a project is not compared", func(l *Library) { l.Projects[0].ParentUUID = "" }, ""},
		// osxphotos writes these dates in the zone of the computer (photos_datetime.py:70-89).
		{"a comment date in another zone, the same moment", func(l *Library) {
			c := &l.Comments[selectedFrame][0]
			c.Datetime = ptr(c.Datetime.In(time.FixedZone("X", -7*3600)))
		}, ""},
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
	spoil := func(l Library) Library {
		l.Photos[0].Keywords = []string{"beta", "alpha"} // unsorted lists that a careless sort would reorder
		l.Photos[0].AlbumUUIDs = []string{albumAInFolderY, albumAInFolderX, albumAInFolderY}
		return l
	}
	lib, ref := spoil(fixtureLib()), fixtureRef()
	divs := []Divergence{{Library: "lib", UUID: "*", Field: "title", Want: []byte(`"x"`), Got: []byte(`"y"`)}}
	if _, err := Compare(ref, lib, everyKey(), divs); err != nil {
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
	onEmptyPhoto := func(f func(x *photoinfo.PhotoInfo)) func(l *Library) { return func(l *Library) { f(l.Photos[2]) } }
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
		{"AlbumUUIDs of a trashed album", func(l *Library) { l.Photos[2].AlbumUUIDs = []string{albumTrashed} }, ""},
		{"AlbumUUIDs", func(l *Library) { l.Photos[2].AlbumUUIDs = []string{albumBNoFolder} }, "album_info albums folders"},
		{"ImportUUID", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ImportUUID = importBare }), "import_info"},
		{"ProjectUUIDs of a trashed project", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectTrashed} }), ""},
		{"ProjectUUIDs", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ProjectUUIDs = []string{projectP} }), "project_info"},
		{"Comments empty, not nil", func(l *Library) { l.Comments[emptyPhoto] = []photoinfo.CommentInfo{} }, ""},
		{"Likes", func(l *Library) { l.Likes[emptyPhoto] = []photoinfo.LikeInfo{{}} }, "likes"},
		{"Owner", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Owner = "x" }), "owner"},
		{"DateOriginal", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.DateOriginal = time.Unix(1, 0).UTC() }), "date_original"},
		{"ExifInfo FlashFired false is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.FlashFired = ptr(false) }), "exif_info"},
		{"ExifInfo ISO 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.ISO = ptr(0) }), "exif_info"},
		{"ExifInfo Latitude 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Latitude = ptr(0.0) }), "exif_info"},
		{"ExifInfo CameraMake", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.CameraMake = "x" }), "exif_info"},
		{"ExifInfo Date at the epoch is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.Date = ptr(time.Unix(0, 0).UTC()) }), "exif_info"},
		{"ExifInfo TZOffset 0 is not null", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.ExifInfo.TZOffset = ptr(0) }), "exif_info"},
		{"Place zero is not {}", onEmptyPhoto(func(x *photoinfo.PhotoInfo) { x.Place = &placeinfo.PlaceInfo{} }), "place"},
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
			UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true,
		}
	}
	withUUID := func(uuid string, o ...map[string]string) Object {
		return refObj(refEmptyPhoto(), append([]map[string]string{{"uuid": `"` + uuid + `"`}}, o...)...)
	}

	type setCase struct {
		name  string
		ref   func() *Reference
		lib   func() Library
		diffs []Diff
	}
	cases := []setCase{
		{"an object only in the reference",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(only)); return r },
			fixtureLib,
			[]Diff{{UUID: only, Field: "uuid", Want: "true", Got: "false"}}},
		{"an object only in Go",
			fixtureRef,
			func() Library { l := fixtureLib(); l.Photos = append(l.Photos, emptyGo(only)); return l },
			[]Diff{{UUID: only, Field: "uuid", Want: "false", Got: "true"}}},
		{"an object only in deleted.json of the reference",
			func() *Reference {
				r := fixtureRef()
				r.Deleted = append(r.Deleted, withUUID(only, map[string]string{"intrash": "true"}))
				return r
			},
			func() Library {
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
			func() Library { l := fixtureLib(); l.Photos = append(l.Photos, emptyGo(only)); return l },
			[]Diff{{UUID: only, Field: "intrash", Want: "true", Got: "false"}}},
		{"an unselected burst frame only in Go",
			fixtureRef,
			func() Library {
				l := fixtureLib()
				l.Photos = append(l.Photos, &photoinfo.PhotoInfo{UUID: only, IsPhoto: true, Burst: true, BurstUUID: "BURST-B", BurstPickType: 4})
				return l
			},
			nil},
		{"an object that is neither a photo nor a movie, only in Go",
			fixtureRef,
			func() Library {
				l := fixtureLib()
				x := emptyGo(only)
				x.IsPhoto = false
				l.Photos = append(l.Photos, x)
				return l
			},
			nil},
		{"an object that is neither a photo nor a movie, in both",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(only)); return r },
			func() Library {
				l := fixtureLib()
				x := emptyGo(only)
				x.IsPhoto = false
				l.Photos = append(l.Photos, x)
				return l
			},
			[]Diff{{UUID: only, Field: "uuid", Want: "true", Got: "false"}}},
		{"a selected frame turned unselected",
			fixtureRef,
			func() Library { l := fixtureLib(); l.Photos[0].BurstPickType = 4; return l },
			[]Diff{{UUID: selectedFrame, Field: "uuid", Want: "true", Got: "false"}}},
		{"a frame without a pick is in the set",
			fixtureRef,
			func() Library { l := fixtureLib(); l.Photos[0].BurstPickType = 0; return l },
			nil},
		{"the key frame is in the set",
			fixtureRef,
			func() Library { l := fixtureLib(); l.Photos[0].BurstPickType = 16; return l },
			nil},
		{"a photo that is not a photo",
			fixtureRef,
			func() Library { l := fixtureLib(); l.Photos[0].IsPhoto = false; return l },
			[]Diff{{UUID: selectedFrame, Field: "uuid", Want: "true", Got: "false"}}},
		{"a movie",
			func() *Reference {
				r := fixtureRef()
				r.Query[1] = withUUID(emptyPhoto, map[string]string{"isphoto": "false", "ismovie": "true"})
				return r
			},
			func() Library { l := fixtureLib(); l.Photos[2].IsPhoto, l.Photos[2].IsMovie = false, true; return l },
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
		lib  func() Library
		keys []string
		want string
	}{
		{"unknown key", fixtureRef, fixtureLib, []string{"title", "nonsense"}, `unknown key "nonsense"`},
		{"album key without albums", fixtureRef, func() Library { l := fixtureLib(); l.Albums = nil; return l }, []string{"albums"}, "an album of a photo is missing from Library.Albums"},
		{"photo keys without albums", fixtureRef, func() Library { l := fixtureLib(); l.Albums = nil; return l }, []string{"title"}, ""},
		{"an album of a photo that Library does not have", fixtureRef, func() Library { l := fixtureLib(); l.Albums = l.Albums[1:]; return l },
			[]string{"album_info"}, "query.json element 1: key album_info: an album of a photo is missing"},
		{"an album UUID repeated", fixtureRef, func() Library { l := fixtureLib(); l.Albums = append(l.Albums, l.Albums[0]); return l },
			[]string{"albums"}, "an album UUID is repeated"},
		{"a UUID repeated in Library.Photos", fixtureRef, func() Library { l := fixtureLib(); l.Photos = append(l.Photos, l.Photos[0]); return l },
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
		{"a shared album with the UUID of an album", fixtureRef, func() Library {
			l := fixtureLib()
			l.SharedAlbums = append(l.SharedAlbums, &albuminfo.AlbumInfo{UUID: albumOfNoPhoto})
			return l
		}, []string{"album_info"}, "an album UUID is repeated in Library.Albums, Library.SharedAlbums or Library.Projects"},
		{"an album of a photo found only among the shared albums is found", fixtureRef, func() Library {
			l := fixtureLib()
			l.SharedAlbums = append(l.SharedAlbums, l.Albums[0])
			l.Albums = l.Albums[1:]
			return l
		}, []string{"albums"}, ""},
		{"folders that is a list", func() *Reference { r := fixtureRef(); r.Query[0]["folders"] = []byte(`[]`); return r }, fixtureLib,
			[]string{"folders"}, "folders: want an object"},
		{"a hasadjustments that is null while orientation is compared", func() *Reference { r := fixtureRef(); r.Query[0]["hasadjustments"] = []byte(`null`); return r }, fixtureLib,
			[]string{"orientation"}, "hasadjustments: want a boolean"},
		{"an object only in the reference is checked too", func() *Reference {
			r := fixtureRef()
			r.Query = append(r.Query, refObj(refEmptyPhoto(), map[string]string{"uuid": `"00000000-0000-0000-0000-0000000000e1"`, "width": `"` + marker + `"`}))
			return r
		}, fixtureLib, []string{"width"}, "query.json element 3: width: want an integer"},
		{"NaN from Go", fixtureRef, func() Library { l := fixtureLib(); l.Photos[0].Latitude = ptr(math.NaN()); return l },
			[]string{"latitude"}, "key latitude: the Go value cannot be written as JSON"},
		{"an import session of a photo that Library does not have", fixtureRef, func() Library { l := fixtureLib(); l.Imports = l.Imports[1:]; return l },
			[]string{"import_info"}, "query.json element 1: key import_info: an import session of a photo is missing from Library.Imports"},
		{"an import session UUID repeated", fixtureRef, func() Library { l := fixtureLib(); l.Imports = append(l.Imports, l.Imports[0]); return l },
			[]string{"import_info"}, "an import session UUID is repeated"},
		{"a project of a photo that Library does not have", fixtureRef, func() Library { l := fixtureLib(); l.Projects = l.Projects[1:]; return l },
			[]string{"project_info"}, "query.json element 1: key project_info: a project of a photo is missing from Library.Projects"},
		{"an album of a photo found only among the projects", fixtureRef, func() Library {
			l := fixtureLib()
			l.Projects = append(l.Projects, l.Albums[0])
			l.Albums = l.Albums[1:]
			return l
		}, []string{"album_info"}, "an album of a photo is missing from Library.Albums"},
		{"a project UUID repeated", fixtureRef, func() Library { l := fixtureLib(); l.Projects = append(l.Projects, l.Projects[0]); return l },
			[]string{"project_info"}, "an album UUID is repeated in Library.Albums, Library.SharedAlbums or Library.Projects"},
		{"a project with the UUID of an album", fixtureRef, func() Library {
			l := fixtureLib()
			l.Projects = append(l.Projects, &albuminfo.AlbumInfo{UUID: albumOfNoPhoto})
			return l
		}, []string{"project_info"}, "an album UUID is repeated in Library.Albums, Library.SharedAlbums or Library.Projects"},
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
			_, err := Compare(c.ref(), c.lib(), c.keys, nil)
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
		_, err := Compare(r, fixtureLib(), []string{"favorite"}, nil)
		var se *shapeError
		if !errors.As(err, &se) || se.path != "favorite" {
			t.Errorf("error = %v, want a *shapeError for favorite", err)
		}
	})
	t.Run("no keys compares the sets only", func(t *testing.T) {
		t.Parallel()
		r := fixtureRef()
		r.Query[0]["favorite"] = []byte(`null`) // not looked at
		rep, err := Compare(r, fixtureLib(), nil, nil)
		if err != nil || !rep.OK() || len(rep.Keys) != 0 || rep.Objects != 2 {
			t.Errorf("Compare = %+v, %v; want no differences and no keys", rep, err)
		}
	})
	t.Run("a repeated key is compared once", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].Title = ""
		rep, err := Compare(fixtureRef(), lib, []string{"title", "uuid", "title"}, nil)
		if err != nil || len(rep.Diffs) != 1 || !slices.Equal(rep.Keys, []string{"title", "uuid"}) {
			t.Errorf("Compare = %+v, %v; want one difference and the keys title and uuid", rep, err)
		}
	})
}

func TestCompareAlbumRules(t *testing.T) {
	t.Parallel()
	albumKeys := AlbumKeys()
	// The other photos of the library would be differences.
	run := func(t *testing.T, ref Object, lib Library) *Report {
		t.Helper()
		id, _ := decode[string](ref, "uuid", false, "")
		lib.Photos = slices.DeleteFunc(slices.Clone(lib.Photos), func(p *photoinfo.PhotoInfo) bool { return p.UUID != id })
		rep, err := Compare(refOf(ref), lib, albumKeys, nil)
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
	t.Run("a like without a date is the default of upstream", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Likes[selectedFrame][0].Datetime = nil
		ref := refObj(refSelectedFrame(), map[string]string{"likes": `[{"datetime": "1970-01-01T00:00:00+00:00", "ismine": false, "user": "Person A"}]`})
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
		// With no value to match, the smaller folder list is shown: that of the shared album.
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
		lib := Library{
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
		rep, err := Compare(refOf(swapped), lib, []string{"albums"}, nil)
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
	rep, err := Compare(ref, Library{Photos: []*photoinfo.PhotoInfo{c, b, a}}, []string{"burst", "burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("differences %+v, want none", rep.Diffs)
	}
	own := refOf(
		refObj(refEmptyPhoto(), map[string]string{"uuid": `"` + q1 + `"`, "burst": "true", "burst_photos": `["` + q1 + `", "` + q2 + `"]`}),
	)
	rep, err = Compare(own, Library{Photos: []*photoinfo.PhotoInfo{a, b}}, []string{"burst_photos"}, nil)
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
	rep, err = Compare(sorted, Library{Photos: []*photoinfo.PhotoInfo{a, c, b}}, []string{"burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("differences %+v, want none: UUIDs sort as bytes", rep.Diffs)
	}
}
