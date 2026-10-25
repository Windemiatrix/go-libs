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
)

// TestFixtureKeys: the literals of the fixtures have exactly the keys of
// fields.txt, so that they do not rot when a key is added.
func TestFixtureKeys(t *testing.T) {
	t.Parallel()
	want := slices.Sorted(slices.Values(fieldsTxt(t)))
	for name, m := range map[string]map[string]string{"refP1": refP1(), "refP3": refP3()} {
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
	if rep.Library != "lib" || rep.Objects != 2 || len(rep.Keys) != 60 {
		t.Errorf("Library = %q, Objects = %d, Keys = %d; want lib, 2, 60", rep.Library, rep.Objects, len(rep.Keys))
	}
	if !slices.IsSorted(rep.Keys) {
		t.Error("Keys are not sorted")
	}
}

type mutation struct {
	name   string
	mutate func(l *Library)
	want   string // the keys of the differences, separated by spaces; "" is none
}

func photo(f func(p *photoinfo.PhotoInfo)) func(l *Library) {
	return func(l *Library) { f(l.Photos[0]) }
}

func albumOf(l *Library, uuid string) *albuminfo.AlbumInfo {
	for _, a := range l.Albums {
		if a.UUID == uuid {
			return a
		}
	}
	panic("no album " + uuid)
}

func album(uuid string, f func(a *albuminfo.AlbumInfo)) func(l *Library) {
	return func(l *Library) { f(albumOf(l, uuid)) }
}

// photoMutations spoil the selected frame p1 in one place each.
func photoMutations() []mutation {
	p := photo
	return []mutation{
		// Booleans.
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

		// Strings.
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

		// Integers.
		{"Width", p(func(x *photoinfo.PhotoInfo) { x.Width++ }), "width"},
		{"Height", p(func(x *photoinfo.PhotoInfo) { x.Height++ }), "height"},
		{"OriginalWidth", p(func(x *photoinfo.PhotoInfo) { x.OriginalWidth++ }), "original_width"},
		{"OriginalHeight", p(func(x *photoinfo.PhotoInfo) { x.OriginalHeight++ }), "original_height"},
		{"OriginalOrientation", p(func(x *photoinfo.PhotoInfo) { x.OriginalOrientation++ }), "original_orientation"},
		{"OriginalFilesize", p(func(x *photoinfo.PhotoInfo) { x.OriginalFilesize++ }), "original_filesize"},
		{"Orientation", p(func(x *photoinfo.PhotoInfo) { x.Orientation++ }), "orientation"},
		{"TZOffset", p(func(x *photoinfo.PhotoInfo) { x.TZOffset = ptr(0) }), "tzoffset"},
		{"TZOffset nil", p(func(x *photoinfo.PhotoInfo) { x.TZOffset = nil }), "tzoffset"},

		// Dates: a moment, to the nanosecond; the offset of the value is not compared.
		{"Date plus 1us", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(time.Microsecond) }), "date"},
		{"Date minus 1us", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(-time.Microsecond) }), "date"},
		{"Date plus 1ns", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(time.Nanosecond) }), "date"},
		{"Date plus 5h30m", p(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.Add(5*time.Hour + 30*time.Minute) }), "date"},
		{"DateAdded", p(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(x.DateAdded.Add(time.Second)) }), "date_added"},
		{"DateAdded nil", p(func(x *photoinfo.PhotoInfo) { x.DateAdded = nil }), "date_added"},
		{"DateModified nil", p(func(x *photoinfo.PhotoInfo) { x.DateModified = nil }), "date_modified"},
		{"DateModified", p(func(x *photoinfo.PhotoInfo) { x.DateModified = ptr(x.DateModified.Add(time.Microsecond)) }), "date_modified"},
		{"DateTrashed nil", p(func(x *photoinfo.PhotoInfo) { x.DateTrashed = nil }), "date_trashed"},

		// Optional values: null is nil, not false or 0.
		{"InCloud false", p(func(x *photoinfo.PhotoInfo) { x.InCloud = ptr(false) }), "incloud"},
		{"InCloud nil", p(func(x *photoinfo.PhotoInfo) { x.InCloud = nil }), "incloud"},
		{"Latitude", p(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(1.0) }), "latitude location"},
		{"Latitude by one ulp", p(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(math.Nextafter(*x.Latitude, 90)) }), "latitude location"},
		{"Latitude nil", p(func(x *photoinfo.PhotoInfo) { x.Latitude = nil }), "latitude location"},
		{"Longitude", p(func(x *photoinfo.PhotoInfo) { x.Longitude = ptr(1.0) }), "location longitude"},
		{"Longitude nil", p(func(x *photoinfo.PhotoInfo) { x.Longitude = nil }), "location longitude"},

		// Lists.
		{"Keywords swapped", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"beta", "alpha"} }), "keywords"},
		{"Keywords shorter", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"alpha"} }), "keywords"},
		{"Keywords with a repeat", p(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"alpha", "beta", "beta"} }), "keywords"},
		{"Keywords nil", p(func(x *photoinfo.PhotoInfo) { x.Keywords = nil }), "keywords"},
		{"HasPersons", p(func(x *photoinfo.PhotoInfo) { x.HasPersons = false }), "persons"},

		// Bursts.
		{"BurstUUID of the other frame", func(l *Library) { l.Photos[1].BurstUUID = "BURST-B" }, "burst_photos"},
		// The other frame is then a plain photo, which osxphotos would return.
		{"the other frame is not a burst frame", func(l *Library) { l.Photos[1].Burst = false }, "burst_photos uuid"},
		{"BurstUUID of the frame", p(func(x *photoinfo.PhotoInfo) { x.BurstUUID = "BURST-B" }), "burst_photos"},
		{"a third frame, not a photo and in the trash", func(l *Library) {
			l.Photos = append(l.Photos, &photoinfo.PhotoInfo{UUID: "00000000-0000-0000-0000-000000000004", Burst: true, BurstUUID: "BURST-A", InTrash: true})
		}, "burst_photos"},

		// Albums.
		{"AlbumUUIDs empty", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = nil }), "album_info albums folders"},
		{"AlbumUUIDs without Album A in Folder X", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = []string{albumA3, albumA2} }), "album_info"},
		{"AlbumUUIDs without Album A in Folder Y", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = []string{albumA1} }), "album_info folders"},
		{"BurstAlbumUUIDs empty", p(func(x *photoinfo.PhotoInfo) { x.BurstAlbumUUIDs = nil }), "burst_album_info burst_albums"},
		{"BurstAlbumUUIDs without Album B", p(func(x *photoinfo.PhotoInfo) { x.BurstAlbumUUIDs = []string{albumA1, albumA3} }), "burst_album_info burst_albums"},
		{"an album in the trash is listed", p(func(x *photoinfo.PhotoInfo) { x.BurstAlbumUUIDs = append(x.BurstAlbumUUIDs, albumA2) }), ""},
		{"an album of the library is listed", p(func(x *photoinfo.PhotoInfo) { x.AlbumUUIDs = append(x.AlbumUUIDs, albumA9) }), "album_info albums folders"},
		{"Album A in Folder X goes to the trash", album(albumA1, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), "album_info burst_album_info"},
		{"Album title", album(albumA3, func(a *albuminfo.AlbumInfo) { a.Title = "Album Z" }), "album_info albums burst_album_info burst_albums folders"},
		{"Album title is empty", album(albumA5, func(a *albuminfo.AlbumInfo) { a.Title = "" }), "burst_album_info burst_albums"},
		{"Album parent", album(albumA1, func(a *albuminfo.AlbumInfo) { a.ParentUUID = "" }), "album_info burst_album_info"},
		{"Album folder names", album(albumA1, func(a *albuminfo.AlbumInfo) { a.FolderNames = []string{"Folder Z"} }), "album_info burst_album_info"},
		{"Album folder names of the album that folders keeps", album(albumA3, func(a *albuminfo.AlbumInfo) { a.FolderNames = []string{"Folder Z"} }), "album_info burst_album_info folders"},
		{"Album creation date", album(albumA1, func(a *albuminfo.AlbumInfo) { a.CreationDate = nil }), "album_info burst_album_info"},
		{"Album creation date by 1us", album(albumA1, func(a *albuminfo.AlbumInfo) { a.CreationDate = ptr(a.CreationDate.Add(time.Microsecond)) }), "album_info burst_album_info"},
		{"Album start date", album(albumA3, func(a *albuminfo.AlbumInfo) { a.StartDate = ptr(utc(2000, 1, 1, 0, 0, 0, 0)) }), "album_info burst_album_info"},
		{"Album end date", album(albumA3, func(a *albuminfo.AlbumInfo) { a.EndDate = nil }), "album_info burst_album_info"},
	}
}

// TestCompareEachKey: the fixture has no differences, and each spoiled place
// gives the differences of exactly the keys that read it. The expectations
// are literals and the reference is a literal, not computed by the package.
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

// TestCompareNFD: the comparison does not normalize strings. The reference is
// in NFC; a Go string in NFD is another string, a difference to be fixed in the port.
func TestCompareNFD(t *testing.T) {
	t.Parallel()
	nfc := "Caf" + string(rune(0xe9))
	nfd := "Cafe" + string(rune(0x301))
	ref := refOf(refObj(refP3(), map[string]string{"title": jsonString(nfc)}))
	for name, tc := range map[string]struct {
		title string
		want  []string
	}{"NFC": {nfc, nil}, "NFD": {nfd, []string{"title"}}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib := fixtureLib()
			lib.Photos = lib.Photos[2:]
			lib.Photos[0].Title = tc.title
			rep := compare(t, ref, lib)
			if got := diffKeys(rep); !slices.Equal(got, tc.want) {
				t.Errorf("differences in %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCompareValues pins the canonical form that Diff carries.
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
		"date":               {`"2020-06-06T14:15:24.725564Z"`, `"2020-06-06T14:15:24.725565Z"`},
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
		if d.Want != w[0] || d.Got != w[1] || d.UUID != p1 {
			t.Errorf("%s: %+v, want %s -> %s of %s", k, d, w[0], w[1], p1)
		}
	}
}

// TestCompareEquivalent: different Go values that mean what the reference says.
func TestCompareEquivalent(t *testing.T) {
	t.Parallel()
	cases := []mutation{
		{"Date in UTC", photo(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.UTC() }), ""},
		{"Date in another zone", photo(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.In(time.FixedZone("X", -7*3600)) }), ""},
		{"Date in a zone with seconds", photo(func(x *photoinfo.PhotoInfo) { x.Date = x.Date.In(time.FixedZone("X", 5640+28)) }), ""},
		{"DateAdded in UTC", photo(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(x.DateAdded.UTC()) }), ""},
		{"albums in another order, a repeat", photo(func(x *photoinfo.PhotoInfo) {
			x.AlbumUUIDs = []string{albumA1, albumA1, albumA2, albumA3, albumA3}
			x.BurstAlbumUUIDs = []string{albumA3, albumA1, albumA5}
		}), ""},
		{"an explicit 2001 for the creation date that osxphotos defaults", album(albumA3, func(a *albuminfo.AlbumInfo) { a.CreationDate = ptr(utc(2001, 1, 1, 0, 0, 0, 0)) }), ""},
		{"the unselected frame with another pick", func(l *Library) { l.Photos[1].BurstPickType = 4 }, ""},
		{"an album of a trashed state not listed", album(albumA9, func(a *albuminfo.AlbumInfo) { a.InTrash = true }), ""},
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
		for _, date := range []string{`"2020-06-06T15:49:00.725564+01:33:36"`, `"2020-06-06T12:41:48.725564-01:33:36"`} {
			ref := refOf(refObj(refP1(), map[string]string{"date": date, "title": `"x\"y"`, "keywords": `["<&>", "x\"y"]`}), refObj(refP3()))
			if rep := compare(t, ref, lib); !rep.OK() {
				t.Errorf("date %s: differences %+v, want none", date, rep.Diffs)
			}
		}
	})

	t.Run("orientation of an adjusted photo is skipped, and compared when not adjusted", func(t *testing.T) {
		t.Parallel()
		adjusted := refObj(refP1(), map[string]string{"hasadjustments": "true", "orientation": "0"})
		lib := fixtureLib()
		lib.Photos[0].HasAdjustments, lib.Photos[0].Orientation = true, 8
		if rep := compare(t, refOf(adjusted, refObj(refP3())), lib); !rep.OK() {
			t.Errorf("differences %v, want none: the orientation of an adjusted photo is not compared", rep.Diffs)
		}
		// The reference decides: not adjusted there, so the orientation counts.
		plain := refObj(refP1(), map[string]string{"orientation": "0"})
		rep := compare(t, refOf(plain, refObj(refP3())), lib)
		if got := diffKeys(rep); !slices.Equal(got, []string{"hasadjustments", "orientation"}) {
			t.Errorf("differences in %v, want hasadjustments and orientation", got)
		}
	})
}

// TestCompareKeepsInput: Compare does not change what it is given.
func TestCompareKeepsInput(t *testing.T) {
	t.Parallel()
	spoil := func(l Library) Library {
		l.Photos[0].Keywords = []string{"beta", "alpha"} // unsorted lists that a careless sort would reorder
		l.Photos[0].AlbumUUIDs = []string{albumA3, albumA1, albumA3}
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

// TestCompareNulls spoils the photo with empty values (p3), whose nulls the
// rules turn into zero values, in one place each.
func TestCompareNulls(t *testing.T) {
	t.Parallel()
	p3m := func(f func(x *photoinfo.PhotoInfo)) func(l *Library) { return func(l *Library) { f(l.Photos[2]) } }
	cases := []mutation{
		{"nothing", p3m(func(*photoinfo.PhotoInfo) {}), ""},
		{"Title", p3m(func(x *photoinfo.PhotoInfo) { x.Title = "x" }), "title"},
		{"Description", p3m(func(x *photoinfo.PhotoInfo) { x.Description = "x" }), "description"},
		{"CloudMasterGUID", p3m(func(x *photoinfo.PhotoInfo) { x.CloudMasterGUID = "x" }), "cloud_guid"},
		{"Fingerprint", p3m(func(x *photoinfo.PhotoInfo) { x.Fingerprint = "x" }), "fingerprint"},
		{"UTIRaw", p3m(func(x *photoinfo.PhotoInfo) { x.UTIRaw = "x" }), "uti_raw"},
		{"UTIEdited", p3m(func(x *photoinfo.PhotoInfo) { x.UTIEdited = "x" }), "uti_edited"},
		{"Width", p3m(func(x *photoinfo.PhotoInfo) { x.Width = 1 }), "width"},
		{"Height", p3m(func(x *photoinfo.PhotoInfo) { x.Height = 1 }), "height"},
		{"OriginalWidth", p3m(func(x *photoinfo.PhotoInfo) { x.OriginalWidth = 1 }), "original_width"},
		{"OriginalHeight", p3m(func(x *photoinfo.PhotoInfo) { x.OriginalHeight = 1 }), "original_height"},
		{"OriginalOrientation", p3m(func(x *photoinfo.PhotoInfo) { x.OriginalOrientation = 1 }), "original_orientation"},
		{"OriginalFilesize", p3m(func(x *photoinfo.PhotoInfo) { x.OriginalFilesize = 1 }), "original_filesize"},
		{"Orientation", p3m(func(x *photoinfo.PhotoInfo) { x.Orientation = 1 }), "orientation"},
		{"InCloud false is not null", p3m(func(x *photoinfo.PhotoInfo) { x.InCloud = ptr(false) }), "incloud"},
		{"TZOffset 0 is not null", p3m(func(x *photoinfo.PhotoInfo) { x.TZOffset = ptr(0) }), "tzoffset"},
		{"Latitude 0 is not null", p3m(func(x *photoinfo.PhotoInfo) { x.Latitude = ptr(0.0) }), "latitude location"},
		{"Longitude 0 is not null", p3m(func(x *photoinfo.PhotoInfo) { x.Longitude = ptr(0.0) }), "location longitude"},
		{"DateModified at the epoch is not null", p3m(func(x *photoinfo.PhotoInfo) { x.DateModified = ptr(time.Unix(0, 0)) }), "date_modified"},
		{"DateTrashed at the epoch is not null", p3m(func(x *photoinfo.PhotoInfo) { x.DateTrashed = ptr(time.Unix(0, 0)) }), "date_trashed"},
		{"DateAdded at the epoch is the default", p3m(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(time.Unix(0, 0)) }), ""},
		{"DateAdded", p3m(func(x *photoinfo.PhotoInfo) { x.DateAdded = ptr(time.Unix(1, 0)) }), "date_added"},
		{"Keywords", p3m(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{"x"} }), "keywords"},
		{"Keywords empty, not nil", p3m(func(x *photoinfo.PhotoInfo) { x.Keywords = []string{} }), ""},
		{"HasPersons", p3m(func(x *photoinfo.PhotoInfo) { x.HasPersons = true }), "persons"},
		{"AlbumUUIDs of a trashed album", func(l *Library) { l.Photos[2].AlbumUUIDs = []string{albumA2} }, ""},
		{"AlbumUUIDs", func(l *Library) { l.Photos[2].AlbumUUIDs = []string{albumA5} }, "album_info albums folders"},
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
			UUID: uuid, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG", Date: time.Unix(0, 0),
			UTI: "public.jpeg", UTIOriginal: "public.jpeg", IsPhoto: true,
		}
	}
	withUUID := func(uuid string, o ...map[string]string) Object {
		return refObj(refP3(), append([]map[string]string{{"uuid": `"` + uuid + `"`}}, o...)...)
	}

	type setCase struct {
		name  string
		ref   func() *Reference
		lib   func() Library
		diffs []Diff // want
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
			[]Diff{{UUID: p1, Field: "uuid", Want: "true", Got: "false"}}},
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
			[]Diff{{UUID: p1, Field: "uuid", Want: "true", Got: "false"}}},
		{"a movie",
			func() *Reference {
				r := fixtureRef()
				r.Query[1] = withUUID(p3, map[string]string{"isphoto": "false", "ismovie": "true"})
				return r
			},
			func() Library { l := fixtureLib(); l.Photos[2].IsPhoto, l.Photos[2].IsMovie = false, true; return l },
			nil},
		{"the reference has an unselected frame",
			func() *Reference { r := fixtureRef(); r.Query = append(r.Query, withUUID(p2)); return r },
			fixtureLib,
			[]Diff{{UUID: p2, Field: "uuid", Want: "true", Got: "false"}}},
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
	const marker = "SECRET-VALUE"
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
			[]string{"albums"}, "album_info[0]: want an object"},
		{"an album without a title key", func() *Reference { r := fixtureRef(); r.Query[0]["album_info"] = []byte(`[{"uuid": "x"}]`); return r }, fixtureLib,
			[]string{"albums"}, "album_info[0].title: want a value"},
		{"folders that is a list", func() *Reference { r := fixtureRef(); r.Query[0]["folders"] = []byte(`[]`); return r }, fixtureLib,
			[]string{"folders"}, "folders: want an object"},
		{"a hasadjustments that is null while orientation is compared", func() *Reference { r := fixtureRef(); r.Query[0]["hasadjustments"] = []byte(`null`); return r }, fixtureLib,
			[]string{"orientation"}, "hasadjustments: want a boolean"},
		{"an object only in the reference is checked too", func() *Reference {
			r := fixtureRef()
			r.Query = append(r.Query, refObj(refP3(), map[string]string{"uuid": `"00000000-0000-0000-0000-0000000000e1"`, "width": `"` + marker + `"`}))
			return r
		}, fixtureLib, []string{"width"}, "query.json element 3: width: want an integer"},
		{"NaN from Go", fixtureRef, func() Library { l := fixtureLib(); l.Photos[0].Latitude = ptr(math.NaN()); return l },
			[]string{"latitude"}, "key latitude: the Go value cannot be written as JSON"},
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
			for _, leak := range []string{marker, p1, p2, p3, albumA1, "00000000-0000"} {
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

// TestCompareAlbumRules covers the rules of album_info, albums, folders.
func TestCompareAlbumRules(t *testing.T) {
	t.Parallel()
	albumKeys := AlbumKeys()
	// run compares the album keys of one object with the photo of the same
	// uuid, so that the other photos of the library are not differences.
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

	t.Run("shared albums are dropped from the reference", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		if rep := run(t, refObj(refP1()), lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	t.Run("a shared album with owner null is a user album the Go side lost", func(t *testing.T) {
		t.Parallel()
		lost := albumJSON(albumC1, `"Shared S"`, "null", `["Shared Folder"]`, `"2020-03-03T03:03:03+00:00"`, "null", "null", "null")
		ref := refObj(refP1(), map[string]string{
			"album_info":       "[" + strings.Join([]string{elemA1, elemA3, lost, elemC2}, ", ") + "]",
			"burst_album_info": "[" + strings.Join([]string{elemA5, elemA1, elemA3, lost}, ", ") + "]",
		})
		rep := run(t, ref, fixtureLib())
		want := []string{"album_info", "albums", "burst_album_info", "burst_albums", "folders"}
		if got := diffKeys(rep); !slices.Equal(got, want) {
			t.Errorf("differences in %v, want %v", got, want)
		}
	})
	t.Run("only shared albums", func(t *testing.T) {
		t.Parallel()
		ref := refObj(refP1(), map[string]string{
			"album_info":       "[" + elemC1 + ", " + elemC2 + "]",
			"albums":           `["Album A", "Shared S"]`,
			"burst_album_info": "[" + elemC1 + "]",
			"burst_albums":     `["Shared S"]`,
			"folders":          `{"Album A": ["Shared Folder"], "Shared S": ["Shared Folder"]}`,
		})
		lib := fixtureLib()
		lib.Photos[0].AlbumUUIDs, lib.Photos[0].BurstAlbumUUIDs = nil, nil
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
	})
	t.Run("a shared album with the title of a user album keeps the title", func(t *testing.T) {
		t.Parallel()
		// Album A of the user is missing on the Go side: albums must still say Album A.
		lib := fixtureLib()
		lib.Photos[0].AlbumUUIDs = []string{albumA2}
		rep := run(t, refObj(refP1()), lib)
		if got := diffKeys(rep); !slices.Equal(got, []string{"album_info", "albums", "folders"}) {
			t.Errorf("differences in %v, want album_info, albums, folders", got)
		}
	})
	t.Run("an album without a title", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Albums = append(lib.Albums, &albuminfo.AlbumInfo{UUID: "00000000-0000-0000-0000-0000000000a7", FolderNames: []string{"Folder X"}})
		lib.Photos[0].AlbumUUIDs = []string{"00000000-0000-0000-0000-0000000000a7"}
		lib.Photos[0].BurstAlbumUUIDs = []string{"00000000-0000-0000-0000-0000000000a7"}
		untitled := albumJSON("00000000-0000-0000-0000-0000000000a7", "null", "null", `["Folder X"]`, `"2001-01-01T00:00:00+00:00"`, "null", "null", "null")
		ref := refObj(refP1(), map[string]string{
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
		// Reference value of Album A is Folder X, the Go albums are X and Y.
		ref := refObj(refP1(), map[string]string{"folders": `{"Album A": ["Folder X"], "Shared S": ["Shared Folder"]}`})
		if rep := run(t, ref, fixtureLib()); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
		ref = refObj(refP1(), map[string]string{"folders": `{"Album A": ["Folder Q"], "Shared S": ["Shared Folder"]}`})
		rep := run(t, ref, fixtureLib())
		if len(rep.Diffs) != 1 || rep.Diffs[0].Field != "folders" {
			t.Fatalf("differences %+v, want folders only", rep.Diffs)
		}
		// With no value to match, the smaller folder list is shown.
		if want := `{"Album A":["Folder Q"]}`; rep.Diffs[0].Want != want {
			t.Errorf("Want = %s, want %s", rep.Diffs[0].Want, want)
		}
		if want := `{"Album A":["Folder X"]}`; rep.Diffs[0].Got != want {
			t.Errorf("Got = %s, want %s", rep.Diffs[0].Got, want)
		}
	})
	t.Run("a repeated album in the reference is one", func(t *testing.T) {
		t.Parallel()
		ref := refObj(refP1(), map[string]string{"album_info": "[" + strings.Join([]string{elemA1, elemA1, elemA3, elemC1, elemC2}, ", ") + "]"})
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
			Photos: []*photoinfo.PhotoInfo{{UUID: p3, IsPhoto: true, AlbumUUIDs: []string{b2, b1}}},
			Albums: []*albuminfo.AlbumInfo{{UUID: b1, Title: "\U0001F600"}, {UUID: b2, Title: "～"}},
		}
		ref := refObj(refP3(), map[string]string{
			"albums": `["～", "😀"]`,
			"album_info": "[" + albumJSON(b1, `"😀"`, "null", "[]", `"2001-01-01T00:00:00+00:00"`, "null", "null", "null") + ", " +
				albumJSON(b2, `"～"`, "null", "[]", `"2001-01-01T00:00:00+00:00"`, "null", "null", "null") + "]",
			"folders": `{"😀": [], "～": []}`,
		})
		if rep := run(t, ref, lib); !rep.OK() {
			t.Errorf("differences %+v, want none", rep.Diffs)
		}
		// The order of UTF-16 units is a difference.
		swapped := refObj(refP3(), map[string]string{"albums": `["😀", "～"]`})
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
	a.Burst, b.Burst = true, true // BurstUUID "" for both; c is not a burst frame
	ref := refOf(
		refObj(refP3(), map[string]string{"uuid": `"` + q1 + `"`, "burst": "true", "burst_photos": `["` + q2 + `"]`}),
		refObj(refP3(), map[string]string{"uuid": `"` + q2 + `"`, "burst": "true", "burst_photos": `["` + q1 + `"]`}),
		refObj(refP3(), map[string]string{"uuid": `"` + q3 + `"`}),
	)
	rep, err := Compare(ref, Library{Photos: []*photoinfo.PhotoInfo{c, b, a}}, []string{"burst", "burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("differences %+v, want none", rep.Diffs)
	}
	// A frame lists the others and not itself.
	own := refOf(
		refObj(refP3(), map[string]string{"uuid": `"` + q1 + `"`, "burst": "true", "burst_photos": `["` + q1 + `", "` + q2 + `"]`}),
	)
	rep, err = Compare(own, Library{Photos: []*photoinfo.PhotoInfo{a, b}}, []string{"burst_photos"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := diffKeys(rep); !slices.Equal(got, []string{"burst_photos", "uuid"}) { // uuid: q2 is not in the reference
		t.Errorf("differences in %v, want burst_photos: the frame must not list itself", got)
	}
	// Sorted by bytes.
	b.UUID = "Z"
	c.UUID, c.Burst = "a", true
	a.UUID = "B"
	c.BurstUUID, b.BurstUUID, a.BurstUUID = "x", "x", "x"
	burst := func(uuid, others string) Object {
		return refObj(refP3(), map[string]string{"uuid": `"` + uuid + `"`, "burst": "true", "burst_photos": others})
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
