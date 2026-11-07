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
// where photo_index raises (albuminfo.py:285-287).
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
			AlbumUUIDs: []string{"A-1", "A-2"}, ProjectUUIDs: []string{"J-1", "J-2"}},
		{UUID: "P-2", Title: "c", IsPhoto: true, AlbumUUIDs: []string{"A-1"}},
		{UUID: "P-3", IsPhoto: true, AlbumUUIDs: []string{"A-2"}},
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
		},
		projects:    []*albuminfo.AlbumInfo{{UUID: "J-1", Title: "Card"}, {UUID: "J-2", Title: "Card"}},
		albumPhotos: map[string][]string{"A-1": {"P-1", "P-2", "B-2"}, "A-2": {"P-1"}, "A-3": {"B-1"}, "J-1": {"P-1"}, "J-2": {"P-1"}},
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
	for _, name := range []string{"place.name", "photo.title", "exif.camera_make", "searchinfo.season", "label", "label_normalized", "shortuuid", "nosuch"} {
		if got, ok := db.Fields(p1).Field(name); ok {
			t.Errorf("Field(%q) = %+v, true; want false", name, got)
		}
	}
}

func TestFieldsKeepPhotoValues(t *testing.T) {
	t.Parallel()
	db := tmplDB()
	p1 := db.byUUID["P-1"]
	for _, name := range []string{"keyword", "person", "album", "folder_album", "album_seq"} {
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
	if !slices.Equal(p1.Keywords, []string{"Beach", "Sea"}) || p1.Persons[0] != "Pat Example" || !slices.Equal(db.albums[0].FolderNames, []string{"Trips", "2020"}) {
		t.Errorf("after a change of the values of Fields: keywords %q, persons %q, folders %q; want them untouched",
			p1.Keywords, p1.Persons, db.albums[0].FolderNames)
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

func tmplOpen(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(t.Context(), path, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
