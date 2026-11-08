package photosdb

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/momentinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// A-1 is sorted by title, so the index of a photo there is not its place in
// the membership rows. P-3 is not among the photos of its album A-2, the case
// where photo_index raises (albuminfo.py:285-287). The title of A-4 is None
// upstream (photoinfo.py:640-643).
func tmplDB() *DB {
	day := time.Date(2020, 2, 4, 19, 7, 38, 0, time.UTC)
	photos := []*photoinfo.PhotoInfo{
		{UUID: "B-1", IsPhoto: true, Burst: true, BurstUUID: "BU", AlbumUUIDs: []string{"A-3"}},
		{UUID: "B-2", OriginalFilename: "IMG_0100.JPG", IsPhoto: true, Burst: true, BurstKey: true, BurstUUID: "BU",
			AlbumUUIDs: []string{"A-1"}},
		{UUID: "B-3", IsPhoto: true, Burst: true, BurstUUID: "BX"},
		{UUID: "P-1", PK: 7, Filename: "IMG_0001.HEIC", OriginalFilename: "IMG_0002.JPG", Title: "b", Description: "A day at the sea",
			IsPhoto: true, HDR: true, HasAdjustments: true, Favorite: true, Date: day,
			ImportedByName: "Camera", ImportedByBundleID: "com.apple.camera",
			Keywords: []string{"Beach", "Sea"}, Persons: []string{"Pat Example", photoinfo.UnknownPerson},
			PathDerivatives: []string{"/x/p.jpeg"}, AlbumUUIDs: []string{"A-1", "A-2"}, ProjectUUIDs: []string{"J-1", "J-2"}},
		{UUID: "P-2", Title: "c", IsPhoto: true, AlbumUUIDs: []string{"A-1"}},
		{UUID: "P-3", IsPhoto: true, AlbumUUIDs: []string{"A-2"}},
		{UUID: "P-4", IsPhoto: true, AlbumUUIDs: []string{"A-3", "A-4"}},
	}
	byUUID := map[string]*photoinfo.PhotoInfo{}
	for _, p := range photos {
		byUUID[p.UUID] = p
	}
	return &DB{
		photos: photos,
		byUUID: byUUID,
		albums: []*albuminfo.AlbumInfo{
			{UUID: "A-1", Title: "Holidays", FolderNames: []string{"Trips", "2020"}, SortOrder: albuminfo.AlbumSortTitle},
			{UUID: "A-2", Title: "Beach"},
			{UUID: "A-3", Title: "Zoo"},
			{UUID: "A-4"},
		},
		projects:    []*albuminfo.AlbumInfo{{UUID: "J-1", Title: "Card"}, {UUID: "J-2", Title: "Card"}},
		albumPhotos: map[string][]string{"A-1": {"P-1", "P-2", "B-2"}, "A-2": {"P-1"}, "A-3": {"B-1", "P-4"}, "A-4": {"P-4"}, "J-1": {"P-1"}, "J-2": {"P-1"}},
		bursts:      indexBursts(photos),
		comments:    map[string][]photoinfo.CommentInfo{"P-1": {{User: "Pat Example", Text: "Nice"}, {Text: "Cool"}}},
		momentData: momentData{
			moments:  map[int64]momentinfo.MomentInfo{5: {PK: 5, Title: "Seaside"}},
			momentOf: map[string]int64{"P-1": 5},
		},
	}
}

func tmplStrings(s ...string) template.Value { return template.Value{Strings: s} }

func TestFields(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p1, p2, p3, b1, b3 := db.byUUID["P-1"], db.byUUID["P-2"], db.byUUID["P-3"], db.byUUID["B-1"], db.byUUID["B-3"]
	modified := time.Date(2021, 11, 25, 6, 5, 4, 0, time.UTC)
	edited := &photoinfo.PhotoInfo{UUID: "E-1", DateModified: &modified}
	movie := &photoinfo.PhotoInfo{UUID: "M-1", IsMovie: true}
	pk := int64(7)
	none, str := template.Value{}, tmplStrings
	holidays := template.AlbumRef{Folders: []string{"Trips", "2020"}, Title: "Holidays"}
	beach, zoo, card := template.AlbumRef{Title: "Beach"}, template.AlbumRef{Title: "Zoo"}, template.AlbumRef{Title: "Card"}
	at := func(a template.AlbumRef, i int) template.AlbumRef {
		a.Index = i
		return a
	}
	albums := func(a ...template.AlbumRef) template.Value { return template.Value{Albums: a} }
	tests := []struct {
		p    *photoinfo.PhotoInfo
		name string
		want template.Value
	}{
		{p1, "name", str("IMG_0001.HEIC")},
		{p1, "original_name", str("IMG_0002.JPG")},
		{p1, "title", str("b")},
		{b1, "title", none},
		{p1, "descr", str("A day at the sea")},
		{p1, "caption", str("A day at the sea")},
		{b1, "descr", none},
		{p1, "photo_or_video", str("photo")},
		{movie, "photo_or_video", str("video")},
		{&photoinfo.PhotoInfo{}, "photo_or_video", str("video")},
		{p1, "hdr", str("hdr")},
		{b1, "hdr", none},
		{p1, "edited", str("edited")},
		{b1, "edited", none},
		{p1, "favorite", str("favorite")},
		{b1, "favorite", none},
		{p1, "created", template.Value{Time: &p1.Date}},
		{p1, "modified", none},
		{edited, "modified", template.Value{Time: &modified}},
		{p1, "uuid", str("P-1")},
		{p1, "id", template.Value{Int: &pk}},
		{p1, "imported_by.name", str("Camera")},
		{p1, "imported_by.id", str("com.apple.camera")},
		{b1, "imported_by.name", none},
		{p1, "moment", str("Seaside")},
		{b1, "moment", none},
		{p1, "burst", none},
		{b1, "burst", str("IMG_0100.JPG")},
		{b3, "burst", none},
		{p1, "album", str("Beach", "Holidays")},
		{b1, "album", str("Holidays", "Zoo")},
		{p1, "project", str("Card", "Card")},
		{p1, "album_project", str("Beach", "Card", "Card", "Holidays")},
		{b1, "album_project", str("Holidays", "Zoo")},
		{p1, "keyword", str("Beach", "Sea")},
		{p1, "person", str("Pat Example", "_UNKNOWN_")},
		{p1, "comment", str("None: Cool", "Pat Example: Nice")},
		{b1, "comment", none},
		{p1, "folder_album", albums(beach, holidays)},
		{b1, "folder_album", albums(holidays, zoo)},
		{p1, "folder_album_project", albums(beach, holidays, card, card)},
		{b1, "folder_album_project", albums(holidays, zoo)},
		{p1, "album_seq", albums(beach, at(holidays, 1))},
		{p2, "album_seq", albums(at(holidays, 2))},
		{p3, "album_seq", albums(at(beach, -1))},
		{b1, "album_seq", albums(zoo)},
		{b3, "album_seq", none},
	}
	for _, tt := range tests {
		if got, ok := db.Fields(tt.p).Field(tt.name); !ok || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Field(%q) of %s = %+v, %v; want %+v, true", tt.name, tt.p.UUID, got, ok, tt.want)
		}
	}
	for _, name := range []string{"photo.labels", "searchinfo.season", "label", "label_normalized", "shortuuid", "nosuch"} {
		if got, ok := db.Fields(p1).Field(name); ok {
			t.Errorf("Field(%q) = %+v, true; want false", name, got)
		}
	}
}

func TestFieldsKeepPhotoValues(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p1 := db.byUUID["P-1"]
	for _, name := range []string{"keyword", "person", "album", "folder_album", "album_seq",
		"photo.keywords", "photo.persons", "photo.path_derivatives", "photo.albums", "photo.burst_albums"} {
		v, _ := db.Fields(p1).Field(name)
		for i := range v.Strings {
			v.Strings[i] = "changed"
		}
		for _, a := range v.Albums {
			for i := range a.Folders {
				a.Folders[i] = "changed"
			}
		}
	}
	if !slices.Equal(p1.Keywords, []string{"Beach", "Sea"}) || p1.Persons[0] != "Pat Example" || p1.PathDerivatives[0] != "/x/p.jpeg" ||
		!slices.Equal(db.albums[0].FolderNames, []string{"Trips", "2020"}) {
		t.Errorf("after a change of the values of Fields: keywords %q, persons %q, derivatives %q, folders %q; want them untouched",
			p1.Keywords, p1.Persons, p1.PathDerivatives, db.albums[0].FolderNames)
	}
	created, _ := db.Fields(p1).Field("created")
	*created.Time = time.Time{}
	modified := time.Date(2021, 11, 25, 6, 5, 4, 0, time.UTC)
	edited, _ := db.Fields(&photoinfo.PhotoInfo{DateModified: &modified}).Field("modified")
	*edited.Time = time.Time{}
	if p1.Date.IsZero() || modified.IsZero() {
		t.Errorf("after a change of the times of Fields: created %v, modified %v; want them untouched", p1.Date, modified)
	}
}

// get_media_type tests the flags in a fixed order (phototemplate.py:1552-1577):
// each case sets one flag and the next one.
func TestFieldsMediaType(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	for _, tt := range []struct {
		p    photoinfo.PhotoInfo
		want string
	}{
		{photoinfo.PhotoInfo{Selfie: true, TimeLapse: true}, "selfie"},
		{photoinfo.PhotoInfo{TimeLapse: true, Panorama: true}, "time_lapse"},
		{photoinfo.PhotoInfo{Panorama: true, SlowMo: true}, "panorama"},
		{photoinfo.PhotoInfo{SlowMo: true, Screenshot: true}, "slow_mo"},
		{photoinfo.PhotoInfo{Screenshot: true, ScreenRecording: true}, "screenshot"},
		{photoinfo.PhotoInfo{ScreenRecording: true, Portrait: true}, "screen_recording"},
		{photoinfo.PhotoInfo{Portrait: true, Live: true}, "portrait"},
		{photoinfo.PhotoInfo{Live: true, Burst: true}, "live_photo"},
		{photoinfo.PhotoInfo{Burst: true, IsMovie: true}, "burst"},
		{photoinfo.PhotoInfo{IsMovie: true}, "video"},
		{photoinfo.PhotoInfo{IsPhoto: true}, "photo"},
		{photoinfo.PhotoInfo{}, "photo"},
	} {
		if got, ok := db.Fields(&tt.p).Field("media_type"); !ok || !reflect.DeepEqual(got, tmplStrings(tt.want)) {
			t.Errorf("media_type = %+v, %v; want %q", got, ok, tt.want)
		}
	}
}

func tmplRender(t *testing.T, db *DB, p *photoinfo.PhotoInfo, in string, o template.RenderOptions) ([]string, error) {
	t.Helper()
	tmpl, err := template.Parse(in)
	if err != nil {
		t.Fatalf("Parse(%q): %v", in, err)
	}
	got, unmatched, err := db.RenderTemplate(p, tmpl, o)
	if unmatched != nil {
		t.Errorf("RenderTemplate(%q) unmatched %q", in, unmatched)
	}
	return got, err
}

func TestRenderTemplate(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p1, b1 := db.byUUID["P-1"], db.byUUID["B-1"]
	o := template.RenderOptions{}
	for _, tt := range []struct {
		p    *photoinfo.PhotoInfo
		in   string
		o    template.RenderOptions
		want string
	}{
		{p1, "{name}", o, "IMG_0001"},
		{p1, "{original_name}", o, "IMG_0002"},
		{b1, "{title,no title}", o, "no title"},
		{&photoinfo.PhotoInfo{Selfie: true}, "{media_type,selfie=autoportrait}", o, "autoportrait"},
		{p1, "{album_seq}", template.RenderOptions{DestPath: "/out/Holidays"}, "1"},
		{p1, "{folder_album_seq(1)}", template.RenderOptions{DestPath: "/out/Trips/2020/Holidays"}, "2"},
		{p1, "{id:05d}", o, "00007"},
		{p1, "{created.year}", o, "2020"},
		{p1, "{modified.year}", o, "2020"},
		{p1, "{person}", o, "Pat Example"},
		{b1, "{burst}", o, "IMG_0100"},
		{p1, "{moment}", o, "Seaside"},
	} {
		if got, err := tmplRender(t, db, tt.p, tt.in, tt.o); err != nil || !slices.Equal(got, []string{tt.want}) {
			t.Errorf("RenderTemplate(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, want := range []string{"0", "1"} {
		if got, err := tmplRender(t, db, p1, "{counter}", o); err != nil || !slices.Equal(got, []string{want}) {
			t.Errorf("RenderTemplate({counter}) = %q, %v; want %q: one Renderer for the DB", got, err, want)
		}
	}
	if got, err := tmplRender(t, db, p1, "{searchinfo.season}", o); !errors.Is(err, template.ErrUnsupportedField) {
		t.Errorf("RenderTemplate({searchinfo.season}) = %q, %v; want ErrUnsupportedField", got, err)
	}
	if got, err := tmplRender(t, db, db.byUUID["P-3"], "{album_seq}", template.RenderOptions{DestPath: "/out/Beach"}); !errors.Is(err, template.ErrRender) {
		t.Errorf("RenderTemplate({album_seq}) of a photo that is not in Beach = %q, %v; want ErrRender", got, err)
	}
}

func TestRenderTemplatePhoto(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p1, b1, p4 := db.byUUID["P-1"], db.byUUID["B-1"], db.byUUID["P-4"]
	lat, lon, far, sum := 51.5, -3.0, 1e16, 0.1
	sum += 0.2 // at run time: the constant 0.1+0.2 is 0.3 exactly
	offset, adjustment, no, yes := 3600, 2, false, true
	added, trashed := time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC), time.Date(2022, 5, 6, 7, 8, 9, 0, time.UTC)
	p := &photoinfo.PhotoInfo{
		UUID: "X-1", OriginalFilename: "a/b.JPG", Path: "/x/IMG_0001.HEIC", PathDerivatives: []string{"/x/b.jpeg", "/x/a.jpeg"},
		PathEdited: "/x/e.jpeg", PathLive: "/x/l.mov", PathRaw: "/x/r.dng", PathEditedLive: "/x/el.mov",
		AdjustmentsPath: "/x/a.plist", OriginalAdjustmentsPath: "/x/a_5.aae", UTIRaw: "com.adobe.raw-image",
		Height: 1200, Width: 1600, Orientation: 6, OriginalFilesize: 12345, TZOffset: &offset, AdjustmentType: &adjustment,
		DateAdded: &added, DateTrashed: &trashed,
		Latitude: &lat, Longitude: &lon, Favorite: true, Live: true, InCloud: &no, ImportedByBundleID: "com.apple.camera",
		Date: time.Date(2020, 1, 2, 3, 4, 5, 6000, time.FixedZone("", 3600)), DateOriginal: time.Date(2020, 1, 2, 3, 4, 5, 0, time.FixedZone("", 3661)),
		Keywords: []string{"Beach", "Sea"}, Persons: []string{"Pat Example", photoinfo.UnknownPerson},
	}
	// LMT zones of the Americas have negative offsets with seconds, such as -4:56:02.
	q := &photoinfo.PhotoInfo{Latitude: &far, Longitude: &sum, InCloud: &yes, Date: time.Date(2020, 1, 2, 3, 4, 5, 500000000, time.FixedZone("", -3661))}
	bare := &photoinfo.PhotoInfo{}
	o, vals := template.RenderOptions{}, func(s ...string) []string { return s }
	for _, tt := range []struct {
		p    *photoinfo.PhotoInfo
		in   string
		o    template.RenderOptions
		want []string
	}{
		{p, "{photo.uuid}", o, vals("X-1")},
		{p, "{photo.original_filename}", o, vals("a/b.JPG")},
		{p, "{photo.original_filename}", template.RenderOptions{Filename: true}, vals("a:b.JPG")},
		{p, "{photo.path}", o, vals("/x/IMG_0001.HEIC")},
		{bare, "{photo.path}", o, vals("_")},
		{p, "{photo.path_edited}", o, vals("/x/e.jpeg")},
		{p, "{photo.path_live_photo}", o, vals("/x/l.mov")},
		{p, "{photo.path_raw}", o, vals("/x/r.dng")},
		{p, "{photo.path_edited_live_photo}", o, vals("/x/el.mov")},
		{p, "{photo.adjustments_path}", o, vals("/x/a.plist")},
		{p, "{photo.original_adjustments_path}", o, vals("/x/a_5.aae")},
		{p, "{photo.uti_raw}", o, vals("com.adobe.raw-image")},
		{p, "{photo.orientation}", o, vals("6")},
		{bare, "{photo.title}", o, vals("_")},
		{p, "{photo.height}", o, vals("1200")},
		{p, "{photo.height|float}", o, vals("1200.0")},
		{p, "{photo.width >= 1000?Y,N}", o, vals("Y")},
		{p, "{photo.original_filesize}", o, vals("12345")},
		{p, "{photo.tzoffset}", o, vals("3600")},
		{bare, "{photo.tzoffset}", o, vals("_")},
		{p, "{photo.adjustment_type}", o, vals("2")},
		{bare, "{photo.adjustment_type}", o, vals("_")},
		{bare, "{photo.rating}", o, vals("0")},
		{bare, "{photo.spatial}", o, vals("0")},
		{p, "{photo.latitude}", o, vals("51.5")},
		{q, "{photo.latitude}", o, vals("1e+16")},
		{q, "{photo.longitude}", o, vals("0.30000000000000004")},
		{bare, "{photo.latitude}", o, vals("_")},
		{p, "{photo.longitude}", o, vals("-3.0")},
		{p, "{photo.location}", o, vals("51.5", "-3.0")},
		{q, "{photo.location}", o, vals("1e+16", "0.30000000000000004")},
		{bare, "{photo.location}", o, vals("_")},
		{p, "{photo.date}", o, vals("2020-01-02T03:04:05.000006+01:00")},
		{p, "{photo.date_original}", o, vals("2020-01-02T03:04:05+01:01:01")},
		{q, "{photo.date}", o, vals("2020-01-02T03:04:05.500000-01:01:01")},
		{p, "{photo.date_trashed}", o, vals("2022-05-06T07:08:09+00:00")},
		{p, "{photo.date_added}", o, vals("2021-03-04T05:06:07+00:00")},
		{bare, "{photo.date_added}", o, vals("1970-01-01T00:00:00+00:00")},
		{bare, "{photo.date_modified}", o, vals("_")},
		{p, "{photo.favorite}", o, vals("favorite")},
		{bare, "{photo.favorite}", o, vals("_")},
		{p, "{photo.favorite?Y,N}", o, vals("Y")},
		{p, "{photo.live_photo}", o, vals("live_photo")},
		{p, "{photo.incloud}", o, vals("_")},
		{q, "{photo.incloud}", o, vals("incloud")},
		{bare, "{photo.incloud}", o, vals("_")},
		{p, "{photo.flagged}", o, vals("_")},
		{&photoinfo.PhotoInfo{InTrash: true}, "{photo.intrash}", o, vals("intrash")},
		{&photoinfo.PhotoInfo{Syndicated: true}, "{photo.syndicated}", o, vals("syndicated")},
		{&photoinfo.PhotoInfo{SavedToLibrary: true}, "{photo.saved_to_library}", o, vals("saved_to_library")},
		{p, "{photo.keywords}", o, vals("Beach", "Sea")},
		{p, "{photo.persons}", o, vals("Pat Example", "_UNKNOWN_")},
		{p, "{photo.path_derivatives}", o, vals("/x/b.jpeg", "/x/a.jpeg")},
		{p, "{photo.imported_by}", o, vals("com.apple.camera")},
		{bare, "{photo.imported_by}", o, vals("_")},
		{p1, "{photo.imported_by}", o, vals("Camera", "com.apple.camera")},
		{p1, "{photo.albums}", o, vals("Beach", "Holidays")},
		{b1, "{photo.albums}", o, vals("Zoo")},
		{b1, "{photo.burst_albums}", o, vals("Holidays", "Zoo")},
		{p4, "{photo.albums}", o, vals("Zoo")},
		{p4, "{photo.burst_albums}", o, vals("Zoo")},
	} {
		if got, err := tmplRender(t, db, tt.p, tt.in, tt.o); err != nil || !slices.Equal(got, tt.want) {
			t.Errorf("RenderTemplate(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"{photo.labels}", "{photo.nonsense}", "{photo.burst_uuid}", "{photo.date.year}", "{photo.score}", "{photo.place}"} {
		if got, err := tmplRender(t, db, p, in, o); !errors.Is(err, template.ErrUnsupportedField) {
			t.Errorf("RenderTemplate(%q) = %q, %v; want ErrUnsupportedField", in, got, err)
		}
	}
}

func TestRenderTemplatePhotoNoneChain(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	offset, adjustment, lat, lon, no := 3600, 2, 51.5, -3.0, false
	moment := time.Date(2022, 5, 6, 7, 8, 9, 0, time.UTC)
	properties := []struct {
		in  string
		set func(p *photoinfo.PhotoInfo)
	}{
		{"{photo.date_modified.year}", func(p *photoinfo.PhotoInfo) { p.DateModified = &moment }},
		{"{photo.date_trashed.isoformat}", func(p *photoinfo.PhotoInfo) { p.DateTrashed = &moment }},
		{"{photo.tzoffset.real}", func(p *photoinfo.PhotoInfo) { p.TZOffset = &offset }},
		{"{photo.adjustment_type.x}", func(p *photoinfo.PhotoInfo) { p.AdjustmentType = &adjustment }},
		{"{photo.latitude.real}", func(p *photoinfo.PhotoInfo) { p.Latitude = &lat }},
		{"{photo.longitude.real}", func(p *photoinfo.PhotoInfo) { p.Longitude = &lon }},
		{"{photo.incloud.x}", func(p *photoinfo.PhotoInfo) { p.InCloud = &no }},
		{"{photo.tzname.upper}", func(p *photoinfo.PhotoInfo) { p.TZName = "America/Chicago" }},
		{"{photo.path_edited.name}", func(p *photoinfo.PhotoInfo) { p.PathEdited = "/x/e.jpeg" }},
		{"{photo.title.upper}", func(p *photoinfo.PhotoInfo) { p.Title = "b" }},
	}
	o := template.RenderOptions{}
	for i, tt := range properties {
		only, others := &photoinfo.PhotoInfo{UUID: "X-1"}, &photoinfo.PhotoInfo{UUID: "X-2"}
		for j, other := range properties {
			if i == j {
				other.set(only)
			} else {
				other.set(others)
			}
		}
		if got, err := tmplRender(t, db, only, tt.in, o); !errors.Is(err, template.ErrUnsupportedField) {
			t.Errorf("RenderTemplate(%q) of a photo with the property = %q, %v; want ErrUnsupportedField", tt.in, got, err)
		}
		if got, err := tmplRender(t, db, others, tt.in, o); err != nil || !slices.Equal(got, []string{"_"}) {
			t.Errorf("RenderTemplate(%q) of a photo without the property = %q, %v; want _", tt.in, got, err)
		}
	}
	none := &photoinfo.PhotoInfo{UUID: "X-0"}
	if got, err := tmplRender(t, db, none, "{photo.date_added.year}", o); !errors.Is(err, template.ErrUnsupportedField) {
		t.Errorf("RenderTemplate({photo.date_added.year}) = %q, %v; want ErrUnsupportedField", got, err)
	}
	for _, in := range []string{"{photo.date_modified.year.x}", "{photo.place.name}", "{photo.date_modified}"} {
		if got, err := tmplRender(t, db, none, in, o); err != nil || !slices.Equal(got, []string{"_"}) {
			t.Errorf("RenderTemplate(%q) of a photo without the property = %q, %v; want _", in, got, err)
		}
	}
}

// Every value differs from the others, so that a name that reads another
// field shows. The city of the smaller area comes first (placeinfo.New).
func tmplNestedPhoto() *photoinfo.PhotoInfo {
	fired, iso, metering, rate, track, balance, offset := true, 100, 5, 44100, 7, 1, -18000
	aperture, bitRate, duration, bias, focal, fps, lat, lon, shutter := 2.2, 1.5e6, 3.0, -0.5, 4.15, 29.97, 51.5, -0.13, 0.001
	shot := time.Date(2020, 1, 2, 3, 4, 5, 0, time.FixedZone("", -18000))
	place := placeinfo.New([]placeinfo.Place{
		{Type: 1, Name: "Federation", Area: 200}, {Type: 1, Name: "Exampleland", Area: 100},
		{Type: 2, Name: "Wide Province", Area: 60}, {Type: 2, Name: "North Province", Area: 50},
		{Type: 4, Name: "Big City", Area: 10}, {Type: 4, Name: "Old Town", Area: 2},
		{Type: 8, Name: "Whole Park", Area: 5}, {Type: 8, Name: "Central Park", Area: 1},
	}, "EX", "1 Main St, Big City, NP 12345", true, placeinfo.Address{
		Street: "1 Main St", City: "Big City", StateProvince: "NP", PostalCode: "12345", Country: "Republic of Exampleland", ISOCountryCode: "XE",
	})
	return &photoinfo.PhotoInfo{
		UUID: "N-1",
		Score: scoreinfo.ScoreInfo{
			Overall: 0.51, Curation: 0.02, Promotion: 0.03, HighlightVisibility: 0.04, Behavioral: 0.05, Failure: -0.06,
			HarmoniousColor: 0.07, Immersiveness: 0.08, Interaction: 0.09, InterestingSubject: 0.1, IntrusiveObjectPresence: 0.11,
			LivelyColor: 0.12, LowLight: 0.13, Noise: 0.14, PleasantCameraTilt: 0.15, PleasantComposition: 0.16,
			PleasantLighting: 0.17, PleasantPattern: 0.18, PleasantPerspective: 0.19, PleasantPostProcessing: 0.2,
			PleasantReflection: 0.21, PleasantSymmetry: 0.22, SharplyFocusedSubject: 0.23, TastefullyBlurred: 0.24,
			WellChosenSubject: 0.25, WellFramedSubject: 0.26, WellTimedShot: 0.27,
		},
		ExifInfo: exifinfo.ExifInfo{
			FlashFired: &fired, ISO: &iso, MeteringMode: &metering, SampleRate: &rate, TrackFormat: &track, WhiteBalance: &balance,
			Aperture: &aperture, BitRate: &bitRate, Duration: &duration, ExposureBias: &bias, FocalLength: &focal, FPS: &fps,
			Latitude: &lat, Longitude: &lon, ShutterSpeed: &shutter,
			CameraMake: "Example Make", CameraModel: "Example Model 2", Codec: "avc1", LensModel: "Example Lens 4.15mm f/2.2",
			Date: &shot, TZOffset: &offset, TZName: "America/Chicago",
		},
		Place: &place,
	}
}

func TestRenderTemplateNested(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	no := false
	p, bare := tmplNestedPhoto(), &photoinfo.PhotoInfo{UUID: "N-0"}
	unfired := &photoinfo.PhotoInfo{UUID: "N-2", ExifInfo: exifinfo.ExifInfo{FlashFired: &no}, Place: &placeinfo.PlaceInfo{Name: "Somewhere"}}
	o := template.RenderOptions{}
	for _, tt := range []struct {
		p        *photoinfo.PhotoInfo
		in, want string
	}{
		{p, "{photo.score.overall}", "0.51"}, {p, "{photo.score.curation}", "0.02"}, {p, "{photo.score.promotion}", "0.03"},
		{p, "{photo.score.highlight_visibility}", "0.04"}, {p, "{photo.score.behavioral}", "0.05"}, {p, "{photo.score.failure}", "-0.06"},
		{p, "{photo.score.harmonious_color}", "0.07"}, {p, "{photo.score.immersiveness}", "0.08"}, {p, "{photo.score.interaction}", "0.09"},
		{p, "{photo.score.interesting_subject}", "0.1"}, {p, "{photo.score.intrusive_object_presence}", "0.11"},
		{p, "{photo.score.lively_color}", "0.12"}, {p, "{photo.score.low_light}", "0.13"}, {p, "{photo.score.noise}", "0.14"},
		{p, "{photo.score.pleasant_camera_tilt}", "0.15"}, {p, "{photo.score.pleasant_composition}", "0.16"},
		{p, "{photo.score.pleasant_lighting}", "0.17"}, {p, "{photo.score.pleasant_pattern}", "0.18"},
		{p, "{photo.score.pleasant_perspective}", "0.19"}, {p, "{photo.score.pleasant_post_processing}", "0.2"},
		{p, "{photo.score.pleasant_reflection}", "0.21"}, {p, "{photo.score.pleasant_symmetry}", "0.22"},
		{p, "{photo.score.sharply_focused_subject}", "0.23"}, {p, "{photo.score.tastefully_blurred}", "0.24"},
		{p, "{photo.score.well_chosen_subject}", "0.25"}, {p, "{photo.score.well_framed_subject}", "0.26"},
		{p, "{photo.score.well_timed_shot}", "0.27"},
		{bare, "{photo.score.overall}", "0.0"},
		{p, "{photo.score.overall > 0.5?Y,N}", "Y"},
		{p, "{photo.exif_info.flash_fired}", "flash_fired"}, {unfired, "{photo.exif_info.flash_fired}", "_"},
		{p, "{photo.exif_info.iso}", "100"}, {p, "{photo.exif_info.metering_mode}", "5"}, {p, "{photo.exif_info.sample_rate}", "44100"},
		{p, "{photo.exif_info.track_format}", "7"}, {p, "{photo.exif_info.white_balance}", "1"},
		{p, "{photo.exif_info.aperture}", "2.2"}, {p, "{photo.exif_info.bit_rate}", "1500000.0"}, {p, "{photo.exif_info.duration}", "3.0"},
		{p, "{photo.exif_info.exposure_bias}", "-0.5"}, {p, "{photo.exif_info.focal_length}", "4.15"}, {p, "{photo.exif_info.fps}", "29.97"},
		{p, "{photo.exif_info.latitude}", "51.5"}, {p, "{photo.exif_info.longitude}", "-0.13"}, {p, "{photo.exif_info.shutter_speed}", "0.001"},
		{p, "{photo.exif_info.camera_make}", "Example Make"}, {p, "{photo.exif_info.camera_model}", "Example Model 2"},
		{p, "{photo.exif_info.codec}", "avc1"}, {p, "{photo.exif_info.lens_model}", "Example Lens 4.15mm f/2.2"},
		{p, "{photo.exif_info.date}", "2020-01-02T03:04:05-05:00"}, {p, "{photo.exif_info.tzoffset}", "-18000"},
		{p, "{photo.exif_info.tzname}", "America/Chicago"},
		{p, "{photo.place.name}", "Central Park, Old Town, North Province, Exampleland"}, {p, "{photo.place.country_code}", "EX"},
		{p, "{photo.place.address_str}", "1 Main St, Big City, NP 12345"}, {p, "{photo.place.ishome}", "ishome"},
		{unfired, "{photo.place.name}", "Somewhere"}, {unfired, "{photo.place.ishome}", "_"},
	} {
		if got, err := tmplRender(t, db, tt.p, tt.in, o); err != nil || !slices.Equal(got, []string{tt.want}) {
			t.Errorf("RenderTemplate(%q) of %s = %q, %v; want %q", tt.in, tt.p.UUID, got, err, tt.want)
		}
	}
	// Without an EXIF row every value is None (exifinfo.py:42-45); without a place the chain stops at None (phototemplate.py:1358-1359).
	for _, in := range []string{"{photo.exif_info.flash_fired}", "{photo.exif_info.iso}", "{photo.exif_info.aperture}", "{photo.exif_info.camera_make}",
		"{photo.exif_info.date}", "{photo.exif_info.tzoffset}", "{photo.exif_info.tzname}",
		"{photo.place.name}", "{photo.place.country_code}", "{photo.place.address_str}", "{photo.place.ishome}"} {
		if got, err := tmplRender(t, db, bare, in, o); err != nil || !slices.Equal(got, []string{"_"}) {
			t.Errorf("RenderTemplate(%q) of a photo without EXIF data and place = %q, %v; want _", in, got, err)
		}
	}
	for _, in := range []string{"{photo.place.names}", "{photo.place.address}", "{photo.place.name.city}", "{photo.place.names.city}",
		"{photo.place.address.street}", "{photo.place.nonsense}"} {
		if got, err := tmplRender(t, db, p, in, o); !errors.Is(err, template.ErrUnsupportedField) {
			t.Errorf("RenderTemplate(%q) = %q, %v; want ErrUnsupportedField", in, got, err)
		}
		if got, err := tmplRender(t, db, bare, in, o); err != nil || !slices.Equal(got, []string{"_"}) {
			t.Errorf("RenderTemplate(%q) of a photo without place = %q, %v; want _", in, got, err)
		}
	}
	for _, in := range []string{"{photo.score.nonsense}", "{photo.exif_info.nonsense}", "{photo.nonsense.overall}"} {
		for _, q := range []*photoinfo.PhotoInfo{p, bare} {
			if got, err := tmplRender(t, db, q, in, o); !errors.Is(err, template.ErrUnsupportedField) {
				t.Errorf("RenderTemplate(%q) of %s = %q, %v; want ErrUnsupportedField", in, q.UUID, got, err)
			}
		}
	}
}

func TestRenderTemplatePlaceExif(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p, bare := tmplNestedPhoto(), &photoinfo.PhotoInfo{UUID: "N-0"}
	nameless := &photoinfo.PhotoInfo{UUID: "N-2", Place: &placeinfo.PlaceInfo{Name: "Somewhere"}}
	o := template.RenderOptions{}
	for _, tt := range []struct{ in, want, bare string }{
		{"{place.name}", "Central Park, Old Town, North Province, Exampleland", "_"},
		{"{place.country_code}", "EX", "_"},
		{"{place.name.country}", "Exampleland", "_"},
		{"{place.name.state_province}", "North Province", "_"},
		{"{place.name.city}", "Old Town", "_"},
		{"{place.name.area_of_interest}", "Central Park", "_"},
		{"{place.address}", "1 Main St, Big City, NP 12345", "_"},
		{"{place.address.street}", "1 Main St", "_"},
		{"{place.address.city}", "Big City", "_"},
		{"{place.address.state_province}", "NP", "_"},
		{"{place.address.postal_code}", "12345", "_"},
		{"{place.address.country}", "Republic of Exampleland", "_"},
		{"{place.address.country_code}", "XE", "_"},
		{"{place.name?Y,N}", "Y", "N"},
		{"{exif.camera_make}", "Example Make", "_"},
		{"{exif.camera_model}", "Example Model 2", "_"},
		{"{exif.lens_model}", "Example Lens 4.15mm f/2.2", "_"},
	} {
		if got, err := tmplRender(t, db, p, tt.in, o); err != nil || !slices.Equal(got, []string{tt.want}) {
			t.Errorf("RenderTemplate(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
		if got, err := tmplRender(t, db, bare, tt.in, o); err != nil || !slices.Equal(got, []string{tt.bare}) {
			t.Errorf("RenderTemplate(%q) of a photo without place and EXIF data = %q, %v; want %q", tt.in, got, err, tt.bare)
		}
	}
	for _, in := range []string{"{place.name.country}", "{place.name.state_province}", "{place.name.city}", "{place.name.area_of_interest}"} {
		if got, err := tmplRender(t, db, nameless, in, o); err != nil || !slices.Equal(got, []string{"_"}) {
			t.Errorf("RenderTemplate(%q) of a place without names = %q, %v; want _", in, got, err)
		}
	}
	// Render never asks for these: upstream has no such field (phototemplate.py:849-883).
	for _, name := range []string{"place.nonsense", "place.name.nonsense", "place.address.nonsense", "exif.iso"} {
		tmpl, err := template.Parse("{" + name + "}")
		if err != nil {
			t.Fatalf("Parse(%q): %v", name, err)
		}
		for _, q := range []*photoinfo.PhotoInfo{p, bare} {
			if got, ok := db.Fields(q).Field(name); ok {
				t.Errorf("Field(%q) of %s = %+v, true; want false", name, q.UUID, got)
			}
			if values, unmatched, err := db.RenderTemplate(q, tmpl, o); err != nil || values != nil || !slices.Equal(unmatched, []string{name}) {
				t.Errorf("RenderTemplate({%s}) of %s = %q, unmatched %q, %v; want unmatched", name, q.UUID, values, unmatched, err)
			}
		}
	}
}

func tmplOpen(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(t.Context(), path, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
