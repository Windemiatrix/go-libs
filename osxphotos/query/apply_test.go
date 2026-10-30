package query

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

type applyCase struct {
	name   string
	o      Options
	photos []*photoinfo.PhotoInfo
	want   []string
}

func applyRun(t *testing.T, tests []applyCase, albums []*albuminfo.AlbumInfo) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := uuids(Apply(tt.o, tt.photos, albums)); !slices.Equal(got, tt.want) {
				t.Errorf("Apply = %q, want %q", got, tt.want)
			}
		})
	}
}

// The loader keeps titles and keywords in NFC, and photo_query normalizes the
// values it searches for (photoquery.py:447-455).
const (
	applyNFD = "Cafe\U00000301"
	applyNFC = "Caf\U000000e9"
)

func applyCheckNFD(t *testing.T) {
	t.Helper()
	if applyNFD == applyNFC || norm.NFC.String(applyNFD) != applyNFC {
		t.Fatal("applyNFD must differ from applyNFC and compose to it")
	}
}

func TestApplyZero(t *testing.T) {
	t.Parallel()

	day := func(d int) time.Time { return at("2020-01-01T00:00:00Z").AddDate(0, 0, d) }
	burst := func(uuid string, pick, d int) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, Burst: true, BurstPickType: pick, Date: day(d)}
	}
	photos := []*photoinfo.PhotoInfo{
		{UUID: "movie", IsMovie: true, Date: day(2)},
		burst("pick24", 24, 9),
		{UUID: "trash", IsPhoto: true, InTrash: true, Date: day(0)},
		burst("pick0", 0, 6),
		burst("pick2", 2, 3),
		{UUID: "photo", IsPhoto: true, Date: day(1)},
		burst("pick8", 8, 7),
		burst("pick4", 4, 4),
		{UUID: "no kind", Date: day(5)},
		burst("pick16", 16, 8),
		// photosdb.py:3347 looks at the pick type of burst frames only.
		{UUID: "not burst", IsPhoto: true, BurstPickType: 2, Date: day(10)},
	}
	want := []string{"photo", "movie", "pick0", "pick8", "pick16", "pick24", "not burst"}
	if got := uuids(Apply(Options{}, photos, nil)); !slices.Equal(got, want) {
		t.Errorf("Apply(Options{}) = %q, want %q", got, want)
	}
	if got := Apply(Options{}, nil, nil); len(got) != 0 {
		t.Errorf("Apply of no photos returned %d", len(got))
	}
}

func TestApplySelection(t *testing.T) {
	t.Parallel()

	// A is 2019-12-31 19:00 on its wall clock: the moment is compared.
	minus5 := time.FixedZone("-05", -5*3600)
	photos := []*photoinfo.PhotoInfo{
		{UUID: "B", IsMovie: true, Date: at("2021-01-01T00:00:00Z")},
		{UUID: "A", IsPhoto: true, Date: at("2020-01-01T00:00:00Z").In(minus5)},
		{UUID: "D", IsMovie: true, InTrash: true, Date: at("2022-01-01T00:00:00Z")},
		{UUID: "C", IsPhoto: true, InTrash: true, Date: at("2019-01-01T00:00:00Z")},
		// In the trash, a burst frame that photos() leaves out (photosdb.py:3347-3351):
		// the deleted selections do not bring it back.
		{UUID: "E", IsPhoto: true, InTrash: true, Burst: true, BurstPickType: 2, Date: at("2018-01-01T00:00:00Z")},
		{UUID: "G", IsPhoto: true, InTrash: true, Burst: true, BurstPickType: photoinfo.BurstPickSelected, Date: at("2018-06-01T00:00:00Z")},
		{UUID: "H", IsPhoto: true, InTrash: true, Burst: true, BurstPickType: photoinfo.BurstPickKey, Date: at("2018-09-01T00:00:00Z")},
	}
	applyRun(t, []applyCase{
		{"zero", Options{}, photos, []string{"A", "B"}},
		{"deleted", Options{Deleted: true}, photos, []string{"G", "H", "C", "A", "B", "D"}},
		{"deleted only", Options{DeletedOnly: true}, photos, []string{"G", "H", "C", "D"}},
		{"deleted and deleted only", Options{Deleted: true, DeletedOnly: true}, photos, []string{"G", "H", "C", "D"}},
		{"only photos", Options{OnlyPhotos: true}, photos, []string{"A"}},
		{"only movies", Options{OnlyMovies: true}, photos, []string{"B"}},
		{"only photos and only movies", Options{OnlyPhotos: true, OnlyMovies: true}, photos, nil},
		{"only photos with deleted", Options{OnlyPhotos: true, Deleted: true}, photos, []string{"G", "H", "C", "A"}},
		{"uuid with an unknown one", Options{UUID: []string{"B", "X"}}, photos, []string{"B"}},
		{"uuid in the trash", Options{UUID: []string{"C"}}, photos, nil},
		{"uuid in the trash with deleted", Options{UUID: []string{"C"}, Deleted: true}, photos, []string{"C"}},
		{"uuid keeps case", Options{UUID: []string{"a"}}, photos, nil},
		{"empty uuid list", Options{UUID: []string{}}, photos, []string{"A", "B"}},
		{"from date equal", Options{FromDate: at("2020-01-01T00:00:00Z")}, photos, []string{"A", "B"}},
		{"from date after", Options{FromDate: at("2020-01-01T00:00:00Z").Add(time.Nanosecond)}, photos, []string{"B"}},
		{"to date equal", Options{ToDate: at("2021-01-01T00:00:00Z")}, photos, []string{"A"}},
		{"to date after", Options{ToDate: at("2021-01-01T00:00:00Z").Add(time.Nanosecond)}, photos, []string{"A", "B"}},
		{"from date in another zone", Options{FromDate: at("2020-01-01T03:00:00+03:00")}, photos, []string{"A", "B"}},
		{"to date in another zone", Options{ToDate: at("2020-01-01T01:00:00+01:00")}, photos, nil},
		{"from and to date with deleted", Options{Deleted: true, FromDate: at("2019-06-01T00:00:00Z"), ToDate: at("2020-06-01T00:00:00Z")},
			photos, []string{"A"}},
	}, nil)
}

func TestBurstUnselected(t *testing.T) {
	t.Parallel()

	const unrelatedBit = 32
	tests := []struct {
		name  string
		burst bool
		pick  int
		want  bool
	}{
		{"not a burst frame", false, 2, false},
		{"no pick type", true, 0, false},
		{"bit 1 only", true, 1, true},
		{"bit 2 only", true, 2, true},
		{"bits 1 and 2", true, 3, true},
		{"default pick only", true, photoinfo.BurstPickDefault, true},
		{"default pick and bit 2", true, photoinfo.BurstPickDefault | 2, true},
		{"selected", true, photoinfo.BurstPickSelected, false},
		{"selected and bit 2", true, photoinfo.BurstPickSelected | 2, false},
		{"key", true, photoinfo.BurstPickKey, false},
		{"key and default pick", true, photoinfo.BurstPickKey | photoinfo.BurstPickDefault, false},
		{"key and selected", true, photoinfo.BurstPickKey | photoinfo.BurstPickSelected, false},
		{"unrelated bit only", true, unrelatedBit, true},
		{"unrelated bit and selected", true, unrelatedBit | photoinfo.BurstPickSelected, false},
		{"unrelated bit and key", true, unrelatedBit | photoinfo.BurstPickKey, false},
	}
	for _, tt := range tests {
		p := &photoinfo.PhotoInfo{Burst: tt.burst, BurstPickType: tt.pick}
		if got := burstUnselected(p); got != tt.want {
			t.Errorf("%s: burstUnselected(PhotoInfo{Burst: %v, BurstPickType: %d}) = %v, want %v", tt.name, tt.burst, tt.pick, got, tt.want)
		}
	}

	if burstUnselected(&photoinfo.PhotoInfo{}) {
		t.Error("burstUnselected(PhotoInfo{}) = true, want false")
	}
}

type applyPair struct {
	name    string
	x, notX func(*Options)
	yes, no photoinfo.PhotoInfo
}

func TestApplyFilters(t *testing.T) {
	t.Parallel()
	applyCheckNFD(t)

	pairs := []applyPair{
		{"edited", func(o *Options) { o.Edited = true }, func(o *Options) { o.NotEdited = true },
			photoinfo.PhotoInfo{HasAdjustments: true}, photoinfo.PhotoInfo{}},
		{"favorite", func(o *Options) { o.Favorite = true }, func(o *Options) { o.NotFavorite = true },
			photoinfo.PhotoInfo{Favorite: true}, photoinfo.PhotoInfo{}},
		{"hidden", func(o *Options) { o.Hidden = true }, func(o *Options) { o.NotHidden = true },
			photoinfo.PhotoInfo{Hidden: true}, photoinfo.PhotoInfo{}},
		// By Path, not by IsMissing (photoquery.py:570-573).
		{"missing", func(o *Options) { o.Missing = true }, func(o *Options) { o.NotMissing = true },
			photoinfo.PhotoInfo{}, photoinfo.PhotoInfo{Path: "originals/IMG_0001.HEIC", IsMissing: true}},
		{"shared", func(o *Options) { o.Shared = true }, func(o *Options) { o.NotShared = true },
			photoinfo.PhotoInfo{Shared: true}, photoinfo.PhotoInfo{}},
		{"burst", func(o *Options) { o.Burst = true }, func(o *Options) { o.NotBurst = true },
			photoinfo.PhotoInfo{Burst: true}, photoinfo.PhotoInfo{}},
		{"live", func(o *Options) { o.Live = true }, func(o *Options) { o.NotLive = true },
			photoinfo.PhotoInfo{Live: true}, photoinfo.PhotoInfo{}},
		{"screenshot", func(o *Options) { o.Screenshot = true }, func(o *Options) { o.NotScreenshot = true },
			photoinfo.PhotoInfo{Screenshot: true}, photoinfo.PhotoInfo{}},
		{"cloud asset", func(o *Options) { o.CloudAsset = true }, func(o *Options) { o.NotCloudAsset = true },
			photoinfo.PhotoInfo{IsCloudAsset: true}, photoinfo.PhotoInfo{}},
		{"in cloud", func(o *Options) { o.InCloud = true }, func(o *Options) { o.NotInCloud = true },
			photoinfo.PhotoInfo{InCloud: ptr(true)}, photoinfo.PhotoInfo{InCloud: ptr(false)}},
		{"reference", func(o *Options) { o.IsReference = true }, func(o *Options) { o.NotReference = true },
			photoinfo.PhotoInfo{IsReference: true}, photoinfo.PhotoInfo{}},
		{"location", func(o *Options) { o.Location = true }, func(o *Options) { o.NoLocation = true },
			photoinfo.PhotoInfo{Latitude: ptr(1.5)}, photoinfo.PhotoInfo{}},
		{"syndicated", func(o *Options) { o.Syndicated = true }, func(o *Options) { o.NotSyndicated = true },
			photoinfo.PhotoInfo{Syndicated: true}, photoinfo.PhotoInfo{}},
		{"saved to library", func(o *Options) { o.SavedToLibrary = true }, func(o *Options) { o.NotSavedToLibrary = true },
			photoinfo.PhotoInfo{Syndicated: true, SavedToLibrary: true}, photoinfo.PhotoInfo{Syndicated: true}},
		{"shared library", func(o *Options) { o.SharedLibrary = true }, func(o *Options) { o.NotSharedLibrary = true },
			photoinfo.PhotoInfo{SharedLibrary: true}, photoinfo.PhotoInfo{}},
	}
	var tests []applyCase
	for _, pr := range pairs {
		yes, no := pr.yes, pr.no
		yes.UUID, yes.IsPhoto = "Y", true
		no.UUID, no.IsPhoto = "N", true
		photos := []*photoinfo.PhotoInfo{&yes, &no}
		var x, notX, both Options
		pr.x(&x)
		pr.notX(&notX)
		pr.x(&both)
		pr.notX(&both)
		tests = append(tests,
			applyCase{pr.name, x, photos, []string{"Y"}},
			applyCase{"not " + pr.name, notX, photos, []string{"N"}},
			applyCase{pr.name + " and not " + pr.name, both, photos, []string{"Y"}},
		)
	}

	mk := func(uuid string, p photoinfo.PhotoInfo) *photoinfo.PhotoInfo {
		p.UUID, p.IsPhoto = uuid, true
		return &p
	}
	cloud := []*photoinfo.PhotoInfo{
		mk("F", photoinfo.PhotoInfo{InCloud: ptr(false)}),
		mk("N", photoinfo.PhotoInfo{}),
		mk("S", photoinfo.PhotoInfo{InCloud: ptr(true), Shared: true}),
		mk("SF", photoinfo.PhotoInfo{InCloud: ptr(false), Shared: true}),
		mk("T", photoinfo.PhotoInfo{InCloud: ptr(true)}),
	}
	syndication := []*photoinfo.PhotoInfo{
		mk("A", photoinfo.PhotoInfo{SavedToLibrary: true}),
		mk("B", photoinfo.PhotoInfo{}),
		mk("S", photoinfo.PhotoInfo{Syndicated: true}),
		mk("SS", photoinfo.PhotoInfo{Syndicated: true, SavedToLibrary: true}),
	}
	raw := []*photoinfo.PhotoInfo{mk("N", photoinfo.PhotoInfo{}), mk("Y", photoinfo.PhotoInfo{HasRAW: true})}
	longitude := []*photoinfo.PhotoInfo{mk("L", photoinfo.PhotoInfo{Longitude: ptr(-0.5)})}
	keywords := []*photoinfo.PhotoInfo{
		mk("K1", photoinfo.PhotoInfo{Keywords: []string{"Beach", "Sun"}}),
		mk("K2", photoinfo.PhotoInfo{Keywords: []string{"Beaches"}}),
		mk("K3", photoinfo.PhotoInfo{}),
		mk("K4", photoinfo.PhotoInfo{Keywords: []string{applyNFC}}),
	}
	names := []*photoinfo.PhotoInfo{
		mk("F1", photoinfo.PhotoInfo{Filename: "ABCD.heic", OriginalFilename: "IMG_0001.HEIC"}),
		mk("F2", photoinfo.PhotoInfo{Filename: "IMG_0002.JPG", OriginalFilename: "Holiday.jpg"}),
		mk("F3", photoinfo.PhotoInfo{Filename: "EFGH.jpg", OriginalFilename: applyNFC + ".jpg"}),
	}
	utis := []*photoinfo.PhotoInfo{
		mk("U1", photoinfo.PhotoInfo{UTI: "public.jpeg", UTIOriginal: "public.heic"}),
		mk("U2", photoinfo.PhotoInfo{UTI: "public.heic", UTIOriginal: "public.jpeg"}),
	}
	tests = append(tests, []applyCase{
		{"has raw", Options{HasRAW: true}, raw, []string{"Y"}},
		{"in cloud leaves out shared", Options{InCloud: true}, cloud, []string{"T"}},
		{"not in cloud: nil is not in cloud, shared left out", Options{NotInCloud: true}, cloud, []string{"F", "N"}},
		{"saved to library of syndicated only", Options{SavedToLibrary: true}, syndication, []string{"SS"}},
		{"not saved to library of syndicated only", Options{NotSavedToLibrary: true}, syndication, []string{"S"}},
		{"location by longitude", Options{Location: true}, longitude, []string{"L"}},
		{"no location by longitude", Options{NoLocation: true}, longitude, nil},

		{"keyword is equal, not a substring", Options{Keyword: []string{"Beach"}}, keywords, []string{"K1"}},
		{"keywords or", Options{Keyword: []string{"Sun", "Beaches"}}, keywords, []string{"K1", "K2"}},
		{"keyword keeps case", Options{Keyword: []string{"beach"}}, keywords, nil},
		{"keyword ignore case", Options{Keyword: []string{"bEACH"}, IgnoreCase: true}, keywords, []string{"K1"}},
		{"keyword in NFD", Options{Keyword: []string{applyNFD}}, keywords, []string{"K4"}},
		{"no keyword", Options{NoKeyword: true}, keywords, []string{"K3"}},
		{"no keyword with keyword", Options{NoKeyword: true, Keyword: []string{"Sun"}}, keywords, []string{"K1"}},

		{"name of original filename", Options{Name: []string{"IMG_000"}}, names, []string{"F1"}},
		{"name not of filename", Options{Name: []string{"ABCD"}}, names, nil},
		{"name keeps case", Options{Name: []string{"img_0001"}}, names, nil},
		{"name ignore case", Options{Name: []string{"img_0001.heic"}, IgnoreCase: true}, names, []string{"F1"}},
		{"names once", Options{Name: []string{"IMG", "0001"}}, names, []string{"F1"}},
		{"name in NFD", Options{Name: []string{applyNFD}}, names, []string{"F3"}},

		{"uti of original", Options{UTI: "heic"}, utis, []string{"U1"}},
		{"uti not of uti", Options{UTI: "jpeg"}, utis, []string{"U2"}},
		{"uti keeps case with ignore case", Options{UTI: "HEIC", IgnoreCase: true}, utis, nil},
	}...)

	textPhotos := func(set func(*photoinfo.PhotoInfo, string)) []*photoinfo.PhotoInfo {
		var out []*photoinfo.PhotoInfo
		for i, s := range []string{"Summer Beach", "", "beach day", applyNFC + " terrace"} {
			p := mk(fmt.Sprintf("T%d", i+1), photoinfo.PhotoInfo{})
			set(p, s)
			out = append(out, p)
		}
		return out
	}
	for _, f := range []struct {
		name   string
		photos []*photoinfo.PhotoInfo
		set    func(o *Options, values []string, none bool)
	}{
		{"title", textPhotos(func(p *photoinfo.PhotoInfo, s string) { p.Title = s }),
			func(o *Options, v []string, none bool) { o.Title, o.NoTitle = v, none }},
		{"description", textPhotos(func(p *photoinfo.PhotoInfo, s string) { p.Description = s }),
			func(o *Options, v []string, none bool) { o.Description, o.NoDescription = v, none }},
	} {
		opt := func(values []string, none, ignoreCase bool) Options {
			o := Options{IgnoreCase: ignoreCase}
			f.set(&o, values, none)
			return o
		}
		tests = append(tests, []applyCase{
			{f.name + " substring", opt([]string{"Beach"}, false, false), f.photos, []string{"T1"}},
			{f.name + " ignore case", opt([]string{"BEACH"}, false, true), f.photos, []string{"T1", "T3"}},
			{f.name + " empty value finds the photos with one", opt([]string{""}, false, false), f.photos, []string{"T1", "T3", "T4"}},
			{f.name + " two matching values, the photo once", opt([]string{"Summer", "Beach"}, false, false), f.photos, []string{"T1"}},
			{f.name + " in NFD", opt([]string{applyNFD}, false, false), f.photos, []string{"T4"}},
			{"no " + f.name, opt(nil, true, false), f.photos, []string{"T2"}},
			{"no " + f.name + " with " + f.name, opt([]string{"day"}, true, false), f.photos, []string{"T3"}},
		}...)
	}
	applyRun(t, tests, nil)
}

// photoquery.py:518-550.
func TestApplyPlace(t *testing.T) {
	t.Parallel()
	applyCheckNFD(t)

	placed := func(uuid string, names placeinfo.Names) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, Place: &placeinfo.PlaceInfo{Name: "Nameton", Names: names}}
	}
	// Upstream searches p.place.names only (photoquery.py:531, 545).
	unnamed := &placeinfo.PlaceInfo{Name: "Townsville", CountryCode: "Townsville", AddressStr: "Townsville"}
	address := reflect.ValueOf(&unnamed.Address).Elem()
	for i := range address.NumField() {
		address.Field(i).SetString("Townsville")
	}
	photos := []*photoinfo.PhotoInfo{
		placed("P1", placeinfo.Names{City: []string{"Townsville"}, Country: []string{"Exampleland"}}),
		placed("P2", placeinfo.Names{Field13: []string{"Example Park"}}),
		placed("P3", placeinfo.Names{Region: []string{"", applyNFC + " Bay"}}),
		placed("P4", placeinfo.Names{}),
		placed("P5", placeinfo.Names{City: []string{""}}),
		{UUID: "PA", IsPhoto: true, Place: unnamed},
		{UUID: "Z", IsPhoto: true, Place: &placeinfo.PlaceInfo{}},
		{UUID: "N", IsPhoto: true},
	}
	applyRun(t, []applyCase{
		{"place substring", Options{Place: []string{"Towns"}}, photos, []string{"P1"}},
		{"place keeps case", Options{Place: []string{"towns"}}, photos, nil},
		{"place ignore case", Options{Place: []string{"tOWNS"}, IgnoreCase: true}, photos, []string{"P1"}},
		{"place ignore case of a lower value", Options{Place: []string{"towns"}, IgnoreCase: true}, photos, []string{"P1"}},
		{"place every value, in names of two kinds", Options{Place: []string{"Towns", "land"}}, photos, []string{"P1"}},
		{"place every value, one not found", Options{Place: []string{"Towns", "nope"}}, photos, nil},
		{"place not of Name", Options{Place: []string{"Nameton"}}, photos, nil},
		{"place in NFD", Options{Place: []string{applyNFD}}, photos, []string{"P3"}},
		{"place in NFD ignore case", Options{Place: []string{applyNFD}, IgnoreCase: true}, photos, []string{"P3"}},
		{"place empty value finds a name that is not empty", Options{Place: []string{""}}, photos, []string{"P1", "P2", "P3"}},
		{"no place", Options{NoPlace: true}, photos, []string{"N"}},
		{"no place with place", Options{NoPlace: true, Place: []string{"Park"}}, photos, []string{"P2"}},
		{"no place with an empty place list", Options{NoPlace: true, Place: []string{}}, photos, []string{"N"}},
		{"empty place list", Options{Place: []string{}}, photos, []string{"N", "P1", "P2", "P3", "P4", "P5", "PA", "Z"}},
	}, nil)

	names := reflect.TypeFor[placeinfo.Names]()
	if names.NumField() != 19 {
		t.Fatalf("Names has %d fields, PlaceNames 19", names.NumField())
	}
	for i := range names.NumField() {
		t.Run("place in "+names.Field(i).Name, func(t *testing.T) {
			t.Parallel()
			var n placeinfo.Names
			reflect.ValueOf(&n).Elem().Field(i).Set(reflect.ValueOf([]string{"Nameton"}))
			p := &photoinfo.PhotoInfo{UUID: "P", IsPhoto: true, Place: &placeinfo.PlaceInfo{Names: n}}
			if got := uuids(Apply(Options{Place: []string{"ameto"}}, []*photoinfo.PhotoInfo{p}, nil)); !slices.Equal(got, []string{"P"}) {
				t.Errorf("Apply = %q, want [P]", got)
			}
		})
	}
}

// photoquery.py:656-664: has_comment and has_likes win over no_comment and
// no_likes.
func TestApplyCatalog(t *testing.T) {
	t.Parallel()

	day := func(uuid string, d int) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, Date: at("2020-01-01T00:00:00Z").AddDate(0, 0, d), AlbumUUIDs: []string{"AL1"}}
	}
	photos := []*photoinfo.PhotoInfo{day("N", 2), day("C", 0), day("L", 1)}
	albums := []*albuminfo.AlbumInfo{{UUID: "AL1", Title: "A"}}
	comments, likes := map[string]bool{"C": true, "X": true}, map[string]bool{"L": true, "X": true}
	c := Catalog{Photos: photos, Albums: albums, Comments: comments, Likes: likes}
	before, beforeComments, beforeLikes := slices.Clone(photos), maps.Clone(comments), maps.Clone(likes)
	var photoValues []photoinfo.PhotoInfo
	for _, p := range photos {
		photoValues = append(photoValues, *p)
	}

	all := []string{"C", "L", "N"}
	for _, tt := range []struct {
		name string
		o    Options
		want []string
	}{
		{"has comment", Options{HasComment: true}, []string{"C"}},
		{"no comment", Options{NoComment: true}, []string{"L", "N"}},
		{"has comment and no comment", Options{HasComment: true, NoComment: true}, []string{"C"}},
		{"has likes", Options{HasLikes: true}, []string{"L"}},
		{"no likes", Options{NoLikes: true}, []string{"C", "N"}},
		{"has likes and no likes", Options{HasLikes: true, NoLikes: true}, []string{"L"}},
		{"has comment and has likes", Options{HasComment: true, HasLikes: true}, nil},
		{"no comment and no likes", Options{NoComment: true, NoLikes: true}, []string{"N"}},
		{"with an album option", Options{HasComment: true, Album: []string{"A"}}, []string{"C"}},
		{"none", Options{}, all},
	} {
		if got := uuids(ApplyCatalog(tt.o, c)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: ApplyCatalog = %q, want %q", tt.name, got, tt.want)
		}
	}

	// The filters run before the duplicate step (photoquery.py:656-664,
	// 754-771), so the step brings back the twin they leave out.
	twins := Catalog{Photos: []*photoinfo.PhotoInfo{flagsTwin("D1"), flagsTwin("D2")},
		Comments: map[string]bool{"D1": true}, Likes: map[string]bool{"D1": true}}
	for _, tt := range []struct {
		name string
		o    Options
	}{
		{"has comment", Options{HasComment: true, Duplicate: true}},
		{"no comment", Options{NoComment: true, Duplicate: true}},
		{"has likes", Options{HasLikes: true, Duplicate: true}},
		{"no likes", Options{NoLikes: true, Duplicate: true}},
	} {
		if got := uuids(ApplyCatalog(tt.o, twins)); !slices.Equal(got, []string{"D1", "D2"}) {
			t.Errorf("%s with duplicate: ApplyCatalog = %q, want [D1 D2]", tt.name, got)
		}
	}

	for _, tt := range []struct {
		name string
		o    Options
		want []string
	}{
		{"has comment", Options{HasComment: true}, nil},
		{"no comment", Options{NoComment: true}, all},
		{"has likes", Options{HasLikes: true}, nil},
		{"no likes", Options{NoLikes: true}, all},
		{"album", Options{Album: []string{"A"}}, all},
	} {
		got := uuids(Apply(tt.o, photos, albums))
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: Apply = %q, want %q", tt.name, got, tt.want)
		}
		if nilMaps := uuids(ApplyCatalog(tt.o, Catalog{Photos: photos, Albums: albums})); !slices.Equal(nilMaps, got) {
			t.Errorf("%s: ApplyCatalog with nil maps = %q, Apply = %q", tt.name, nilMaps, got)
		}
	}

	if !slices.Equal(photos, before) || !maps.Equal(comments, beforeComments) || !maps.Equal(likes, beforeLikes) {
		t.Error("ApplyCatalog changed its input slice or maps")
	}
	for i, p := range photos {
		if !reflect.DeepEqual(*p, photoValues[i]) {
			t.Errorf("ApplyCatalog changed photo %s", p.UUID)
		}
	}
}

func TestApplyAlbums(t *testing.T) {
	t.Parallel()
	applyCheckNFD(t)

	albums := []*albuminfo.AlbumInfo{
		{UUID: "AL1", Title: "A"},
		{UUID: "AL2", Title: "A", FolderNames: []string{"F1", "F2"}},
		{UUID: "AL3", Title: "A/B"},
		{UUID: "AL4", Title: "Trip", FolderNames: []string{"F1"}},
		{UUID: "AL5", Title: "Gone", FolderNames: []string{"F1"}, InTrash: true},
		{UUID: "AL6", Title: "F1"},
		{UUID: "AL7", Title: applyNFC, FolderNames: []string{applyNFC}},
	}
	in := func(uuid string, albums ...string) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, AlbumUUIDs: albums}
	}
	photos := []*photoinfo.PhotoInfo{
		in("P01", "AL1"), in("P02", "AL2"), in("P03", "AL3"), in("P04", "AL4"), in("P05", "AL5"),
		in("P06", "AL6"), in("P07"), in("P08", "AL-NOT-IN-ALBUMS"), in("P09", "AL7"), in("P10", "AL1", "AL4"),
	}
	inAlbum := []string{"P01", "P02", "P03", "P04", "P06", "P09", "P10"}
	applyRun(t, []applyCase{
		{"album at the top level", Options{Album: []string{"A"}}, photos, []string{"P01", "P10"}},
		{"album in folders", Options{Album: []string{"F1/F2/A"}}, photos, []string{"P02"}},
		{"album with an escaped slash", Options{Album: []string{"A//B"}}, photos, []string{"P03"}},
		{"album with a slash not escaped", Options{Album: []string{"A/B"}}, photos, nil},
		{"albums or, each photo once", Options{Album: []string{"F1/F2/A", "F1/Trip", "A"}}, photos, []string{"P01", "P02", "P04", "P10"}},
		{"album keeps case", Options{Album: []string{"f1/f2/a"}}, photos, nil},
		{"album ignore case", Options{Album: []string{"f1/F2/a"}, IgnoreCase: true}, photos, []string{"P02"}},
		{"album in the trash", Options{Album: []string{"F1/Gone"}}, photos, nil},
		{"album in NFD", Options{Album: []string{applyNFD + "/" + applyNFD}}, photos, []string{"P09"}},
		{"album named as a folder", Options{Album: []string{"F1"}}, photos, []string{"P06"}},
		{"folder with subfolders", Options{Folder: []string{"F1"}}, photos, []string{"P02", "P04", "P10"}},
		{"nested folder", Options{Folder: []string{"F1/F2"}}, photos, []string{"P02"}},
		{"folder keeps case", Options{Folder: []string{"f1"}}, photos, nil},
		{"folder ignore case", Options{Folder: []string{"f1/F2"}, IgnoreCase: true}, photos, []string{"P02"}},
		{"folder is not an album title", Options{Folder: []string{"A"}}, photos, nil},
		{"folder in NFD", Options{Folder: []string{applyNFD}}, photos, []string{"P09"}},
		{"album and folder", Options{Album: []string{"A"}, Folder: []string{"F1"}}, photos, []string{"P10"}},
		{"in album", Options{InAlbum: true}, photos, inAlbum},
		{"not in album", Options{NotInAlbum: true}, photos, []string{"P05", "P07", "P08"}},
		{"in album and not in album", Options{InAlbum: true, NotInAlbum: true}, photos, inAlbum},
	}, albums)

	twins := []*albuminfo.AlbumInfo{
		{UUID: "T1", Title: "Twin", FolderNames: []string{"F1"}, InTrash: true},
		{UUID: "T2", Title: "Twin", FolderNames: []string{"F1"}},
	}
	applyRun(t, []applyCase{
		{"album twin of a trashed album", Options{Album: []string{"F1/Twin"}}, []*photoinfo.PhotoInfo{in("Q1", "T1", "T2"), in("Q2", "T1")}, []string{"Q1"}},
	}, twins)
}

func TestApplyOrder(t *testing.T) {
	t.Parallel()

	photos := []*photoinfo.PhotoInfo{
		{UUID: "C", IsPhoto: true, Date: at("2022-01-01T00:00:00Z"), AlbumUUIDs: []string{"AL1"}},
		{UUID: "A", IsPhoto: true, Date: at("2020-01-01T00:00:00Z"), AlbumUUIDs: []string{"AL1"}},
		{UUID: "B", IsMovie: true, Date: at("2021-01-01T00:00:00Z"), AlbumUUIDs: []string{"AL1"}},
	}
	albums := []*albuminfo.AlbumInfo{{UUID: "AL1", Title: "A", FolderNames: []string{"F"}, PhotoUUIDs: []string{"A", "B", "C"}}}
	before, beforeAlbums := slices.Clone(photos), slices.Clone(albums)
	values := func() (ps []photoinfo.PhotoInfo, as []albuminfo.AlbumInfo) {
		for _, p := range photos {
			ps = append(ps, *p)
		}
		for _, a := range albums {
			as = append(as, *a)
		}
		return ps, as
	}
	photoValues, albumValues := values()

	if got, want := uuids(Apply(Options{Folder: []string{"F"}}, photos, albums)), []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
	newest := Apply(Options{Folder: []string{"F"}, NewestFirst: true}, photos, albums)
	want := uuids(oracle(photos))
	slices.Reverse(want)
	if got := uuids(newest); !slices.Equal(got, want) {
		t.Errorf("Apply with NewestFirst = %q, want %q", got, want)
	}
	for _, p := range newest {
		if !slices.Contains(photos, p) {
			t.Errorf("the result holds %p (%s), not an element of photos", p, p.UUID)
		}
	}
	newest[0] = &photoinfo.PhotoInfo{UUID: "new"}

	if !slices.Equal(photos, before) || !slices.Equal(albums, beforeAlbums) {
		t.Error("Apply changed its input slices")
	}
	if ps, as := values(); !reflect.DeepEqual(ps, photoValues) || !reflect.DeepEqual(as, albumValues) {
		t.Error("Apply changed a photo or an album")
	}
}
