package photosdb

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/momentinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
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
	for _, name := range []string{"place.name", "photo.labels", "exif.camera_make", "searchinfo.season", "label", "label_normalized", "shortuuid", "nosuch"} {
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
	if got, err := tmplRender(t, db, p1, "{place.name}", o); !errors.Is(err, template.ErrUnsupportedField) {
		t.Errorf("RenderTemplate({place.name}) = %q, %v; want ErrUnsupportedField", got, err)
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

func tmplOpen(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(t.Context(), path, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
