package photosdb

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// baseAsset is a plain photo with an attributes row and no NULL that
// matters.
func baseAsset(pk int64, uuid string) sqlread.Asset {
	var a sqlread.Asset
	a.PK, a.UUID, a.Filename, a.Directory = pk, nullString(uuid), nullString(uuid+".jpeg"), nullString(uuid[:1])
	a.UTI, a.Kind, a.KindSubtype = nullString("public.jpeg"), nullInt(0), nullInt(0)
	a.Created, a.Modified, a.Added = nullFloat(694224000), nullFloat(694224000), nullFloat(694224000)
	a.TrashedState, a.Hidden, a.Favorite = nullInt(0), nullInt(0), nullInt(0)
	a.Latitude, a.Longitude = nullFloat(noCoordinate), nullFloat(noCoordinate)
	a.Adjustments, a.Width, a.Height, a.Orientation = nullInt(0), nullInt(4032), nullInt(3024), nullInt(1)
	a.SavedAssetType, a.ScopeParticipation, a.ScopeShareState = nullInt(3), nullInt(0), nullInt(0)
	a.HasAttributes, a.OriginalFilename, a.TZOffset = true, nullString("IMG_0001.JPG"), nullInt(0)
	a.OriginalWidth, a.OriginalHeight, a.OriginalOrientation = nullInt(4032), nullInt(3024), nullInt(1)
	a.OriginalFilesize, a.OriginalResourceChoice, a.SyndicationHistory = nullInt(1000), nullInt(0), nullInt(0)
	return a
}

// loadOne loads one asset with its resources and returns its photo.
func loadOne(t *testing.T, a sqlread.Asset, res ...sqlread.Resource) *photoinfo.PhotoInfo {
	t.Helper()
	for i := range res {
		res[i].AssetPK = a.PK
		if res[i].PK == 0 {
			res[i].PK = int64(i + 1)
		}
	}
	c, err := load(tables{assets: []sqlread.Asset{a}, resources: res}, "ZORIGINALSTABLEHASH")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(c.photos) != 1 {
		t.Fatalf("load: %d photos, want 1", len(c.photos))
	}
	return c.photos[0]
}

func resource(subtype, typ int64, local sql.NullInt64) sqlread.Resource {
	return sqlread.Resource{Subtype: nullInt(subtype), Type: nullInt(typ), Local: local, Remote: nullInt(1), DataLength: nullInt(10)}
}

func TestLoadFields(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Café", "Café"
	tests := []struct {
		name  string
		edit  func(a *sqlread.Asset)
		res   []sqlread.Resource
		check func(p *photoInfo) bool
	}{
		{"UUID", nil, nil, func(p *photoInfo) bool { return p.UUID == "A-1" }},
		{"cloud GUID", func(a *sqlread.Asset) { a.CloudAssetGUID = nullString("G") }, nil,
			func(p *photoInfo) bool { return p.CloudAssetGUID == "G" && p.IsCloudAsset }},
		{"cloud GUID NULL", nil, nil, func(p *photoInfo) bool { return p.CloudAssetGUID == "" && !p.IsCloudAsset }},
		{"cloud GUID empty", func(a *sqlread.Asset) { a.CloudAssetGUID = nullString("") }, nil,
			func(p *photoInfo) bool { return p.CloudAssetGUID == "" && !p.IsCloudAsset }},
		{"no cloud master", nil, nil, func(p *photoInfo) bool { return p.InCloud == nil && p.CloudMasterGUID == "" }},
		{"cloud master local", func(a *sqlread.Asset) {
			a.HasMaster, a.CloudLocalState, a.CloudMasterGUID = true, nullInt(1), nullString("M")
		}, nil, func(p *photoInfo) bool { return p.InCloud != nil && !*p.InCloud && p.CloudMasterGUID == "M" }},
		{"cloud master in iCloud", func(a *sqlread.Asset) { a.HasMaster, a.CloudLocalState = true, nullInt(3) }, nil,
			func(p *photoInfo) bool { return p.InCloud != nil && *p.InCloud && p.CloudMasterGUID == "" }},
		{"fingerprint", func(a *sqlread.Asset) { a.Fingerprint = nullString("FP") }, nil,
			func(p *photoInfo) bool { return p.Fingerprint == "FP" && p.FingerprintSchema == "ZORIGINALSTABLEHASH" }},
		{"fingerprint NULL", nil, nil, func(p *photoInfo) bool { return p.Fingerprint == "" && p.FingerprintSchema == "" }},
		{"fingerprint empty", func(a *sqlread.Asset) { a.Fingerprint = nullString("") }, nil,
			func(p *photoInfo) bool { return p.Fingerprint == "" && p.FingerprintSchema == "" }},
		{"file names NFC", func(a *sqlread.Asset) {
			a.Filename, a.OriginalFilename = nullString(nfd+".jpeg"), nullString(nfd+".JPG")
		}, nil, func(p *photoInfo) bool {
			return p.Filename == nfcForm+".jpeg" && p.OriginalFilename == nfcForm+".JPG"
		}},
		{"file name NULL", func(a *sqlread.Asset) { a.Filename, a.OriginalFilename = sql.NullString{}, sql.NullString{} }, nil,
			func(p *photoInfo) bool {
				return p.Filename == "" && p.OriginalFilename == "" && p.UTIOriginal == "public.jpeg"
			}},
		{"original file name NULL", func(a *sqlread.Asset) { a.OriginalFilename = sql.NullString{} }, nil,
			func(p *photoInfo) bool { return p.OriginalFilename == "A-1.jpeg" }},
		{"original file name empty", func(a *sqlread.Asset) { a.OriginalFilename = nullString("") }, nil,
			func(p *photoInfo) bool { return p.OriginalFilename == "A-1.jpeg" }},
		{"dates", func(a *sqlread.Asset) {
			a.Added, a.Adjusted, a.Trashed, a.Modified = nullFloat(0), nullFloat(86400), nullFloat(172800), nullFloat(-86400)
		}, nil, func(p *photoInfo) bool {
			return timeIs(p.DateAdded, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)) &&
				timeIs(p.DateModified, time.Date(2001, 1, 2, 0, 0, 0, 0, time.UTC)) &&
				timeIs(p.DateTrashed, time.Date(2001, 1, 3, 0, 0, 0, 0, time.UTC)) &&
				timeIs(p.DateModifiedRaw, time.Date(2000, 12, 31, 0, 0, 0, 0, time.UTC))
		}},
		{"dates NULL", func(a *sqlread.Asset) {
			a.Added, a.Adjusted, a.Trashed, a.Modified = sql.NullFloat64{}, sql.NullFloat64{}, sql.NullFloat64{}, sql.NullFloat64{}
		}, nil, func(p *photoInfo) bool {
			return p.DateAdded == nil && p.DateModified == nil && p.DateTrashed == nil && p.DateModifiedRaw == nil
		}},
		{"date in the zone", func(a *sqlread.Asset) { a.TZOffset = nullInt(19800) }, nil, func(p *photoInfo) bool {
			_, off := p.Date.Zone()
			return p.Date.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)) && off == 19800 && *p.TZOffset == 19800
		}},
		{"offset NULL", func(a *sqlread.Asset) { a.TZOffset = sql.NullInt64{} }, nil, func(p *photoInfo) bool {
			return p.TZOffset == nil && p.Date.Location() == time.UTC
		}},
		{"offset 86399", func(a *sqlread.Asset) { a.TZOffset = nullInt(86399) }, nil, func(p *photoInfo) bool {
			_, off := p.Date.Zone()
			return off == 86399 && *p.TZOffset == 86399
		}},
		{"offset 86400", func(a *sqlread.Asset) { a.TZOffset = nullInt(86400) }, nil, func(p *photoInfo) bool {
			return p.Date.Equal(epoch) && p.Date.Location() == time.UTC && *p.TZOffset == 86400
		}},
		{"offset -86400", func(a *sqlread.Asset) { a.TZOffset = nullInt(-86400) }, nil, func(p *photoInfo) bool {
			return p.Date.Equal(epoch) && *p.TZOffset == -86400
		}},
		{"offset max int64", func(a *sqlread.Asset) { a.TZOffset = nullInt(math.MaxInt64) }, nil, func(p *photoInfo) bool {
			return p.Date.Equal(epoch) && *p.TZOffset == math.MaxInt64
		}},
		{"offset min int64", func(a *sqlread.Asset) { a.TZOffset = nullInt(math.MinInt64) }, nil, func(p *photoInfo) bool {
			return p.Date.Equal(epoch) && *p.TZOffset == math.MinInt64
		}},
		{"creation date NULL", func(a *sqlread.Asset) { a.Created = sql.NullFloat64{} }, nil, func(p *photoInfo) bool {
			return p.Date.Equal(epoch) && p.Date.Location() == time.UTC
		}},
		{"sizes", func(a *sqlread.Asset) {
			a.Width, a.Height, a.Orientation = nullInt(11), nullInt(22), nullInt(6)
			a.OriginalWidth, a.OriginalHeight, a.OriginalOrientation, a.OriginalFilesize = nullInt(33), nullInt(44), nullInt(8), nullInt(5_000_000_000)
		}, nil, func(p *photoInfo) bool {
			return p.Width == 11 && p.Height == 22 && p.Orientation == 6 && p.OriginalWidth == 33 &&
				p.OriginalHeight == 44 && p.OriginalOrientation == 8 && p.OriginalFilesize == 5_000_000_000
		}},
		{"sizes NULL", func(a *sqlread.Asset) {
			a.Width, a.Height, a.Orientation = sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}
			a.OriginalWidth, a.OriginalHeight, a.OriginalOrientation, a.OriginalFilesize = sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}
		}, nil, func(p *photoInfo) bool {
			return p.Width == 0 && p.Height == 0 && p.Orientation == 0 && p.OriginalWidth == 0 &&
				p.OriginalHeight == 0 && p.OriginalOrientation == 0 && p.OriginalFilesize == 0
		}},
		{"UTI NULL", func(a *sqlread.Asset) { a.UTI = sql.NullString{} }, nil, func(p *photoInfo) bool {
			return p.UTI == "" && p.UTIOriginal == "public.jpeg"
		}},
		{"UTI original of a RAW", func(a *sqlread.Asset) { a.OriginalFilename = nullString("IMG_1.DNG") }, nil,
			func(p *photoInfo) bool { return p.UTIOriginal == "com.adobe.raw-image" && p.IsRAW }},
		{"UTI original unknown extension", func(a *sqlread.Asset) { a.OriginalFilename = nullString("IMG_1.xyz9") }, nil,
			func(p *photoInfo) bool { return p.UTIOriginal == "public.jpeg" && !p.IsRAW }},
		{"UTI original without extension", func(a *sqlread.Asset) { a.OriginalFilename = nullString("a.") }, nil,
			func(p *photoInfo) bool { return p.UTIOriginal == "public.jpeg" }},
		{"UTI original of a shared photo", func(a *sqlread.Asset) {
			a.OriginalFilename, a.CloudBatchPublishDate = nullString("IMG_1.DNG"), nullFloat(1)
		}, nil, func(p *photoInfo) bool { return p.Shared && p.UTIOriginal == "public.jpeg" && !p.IsRAW }},
		{"movie", func(a *sqlread.Asset) { a.Kind = nullInt(1) }, nil, func(p *photoInfo) bool { return p.IsMovie && !p.IsPhoto }},
		{"photo", nil, nil, func(p *photoInfo) bool { return p.IsPhoto && !p.IsMovie }},
		{"other kind", func(a *sqlread.Asset) { a.Kind = nullInt(7) }, nil, func(p *photoInfo) bool { return !p.IsPhoto && !p.IsMovie }},
		{"kind NULL", func(a *sqlread.Asset) { a.Kind = sql.NullInt64{} }, nil, func(p *photoInfo) bool { return !p.IsPhoto && !p.IsMovie }},
		{"live", func(a *sqlread.Asset) { a.KindSubtype = nullInt(2) }, nil, func(p *photoInfo) bool { return p.Live && !p.Screenshot }},
		{"screenshot", func(a *sqlread.Asset) { a.KindSubtype = nullInt(10) }, nil, func(p *photoInfo) bool { return p.Screenshot && !p.Live }},
		{"RAW subtype 17 of any type", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(17, 0, nullInt(1))},
			func(p *photoInfo) bool { return p.HasRAW }},
		{"RAW subtype 3 type 4", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(3, 4, nullInt(1))},
			func(p *photoInfo) bool { return p.HasRAW }},
		{"RAW subtype 3 type 0", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(3, 0, nullInt(1))},
			func(p *photoInfo) bool { return !p.HasRAW }},
		{"RAW subtype 3 type NULL", nil, []sqlread.Resource{{Subtype: nullInt(3), Local: nullInt(1)}},
			func(p *photoInfo) bool { return !p.HasRAW }},
		{"Live video subtype 18 type 3", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(18, 3, nullInt(1))},
			func(p *photoInfo) bool { return !p.HasRAW }},
		{"original subtype 1 type 4", nil, []sqlread.Resource{resource(1, 4, nullInt(1))},
			func(p *photoInfo) bool { return !p.HasRAW }},
		{"RAW 17 and 3", nil, []sqlread.Resource{resource(17, 4, nullInt(1)), resource(3, 4, nullInt(1))},
			func(p *photoInfo) bool { return p.HasRAW }},
		{"RAW is the original", func(a *sqlread.Asset) { a.OriginalResourceChoice = nullInt(1) }, nil,
			func(p *photoInfo) bool { return p.RawOriginal }},
		{"RAW choice NULL", func(a *sqlread.Asset) { a.OriginalResourceChoice = sql.NullInt64{} }, nil,
			func(p *photoInfo) bool { return !p.RawOriginal }},
		{"missing: no resources", nil, nil, func(p *photoInfo) bool { return !p.IsMissing }},
		{"missing: last original not local", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(3, 0, nullInt(0))},
			func(p *photoInfo) bool { return p.IsMissing }},
		{"missing: last original local", nil, []sqlread.Resource{resource(1, 0, nullInt(0)), resource(1, 0, nullInt(1))},
			func(p *photoInfo) bool { return !p.IsMissing }},
		{"missing: local NULL", nil, []sqlread.Resource{{Subtype: nullInt(1)}},
			func(p *photoInfo) bool { return p.IsMissing }},
		{"missing: other subtypes only", nil, []sqlread.Resource{resource(18, 3, nullInt(0)), resource(17, 4, nullInt(0))},
			func(p *photoInfo) bool { return !p.IsMissing }},
		{"missing: the fingerprint wins", func(a *sqlread.Asset) { a.Fingerprint = nullString("FP") }, []sqlread.Resource{
			{PK: 1, Subtype: nullInt(1), Local: nullInt(0), Fingerprint: nullString("FP")},
			{PK: 2, Subtype: nullInt(1), Local: nullInt(1), Fingerprint: nullString("FP")},
			{PK: 3, Subtype: nullInt(3), Local: nullInt(0), Fingerprint: nullString("OTHER")},
		}, func(p *photoInfo) bool { return !p.IsMissing }},
		{"missing: fingerprint without a match", func(a *sqlread.Asset) { a.Fingerprint = nullString("FP") }, []sqlread.Resource{
			{PK: 1, Subtype: nullInt(1), Local: nullInt(0), Fingerprint: nullString("OTHER")},
		}, func(p *photoInfo) bool { return p.IsMissing }},
		{"resources", nil, []sqlread.Resource{
			{PK: 2},
			{PK: 5, Type: nullInt(4), Subtype: nullInt(3), DataLength: nullInt(7_000_000_000), Local: nullInt(1), Remote: nullInt(2)},
		}, func(p *photoInfo) bool {
			return reflect.DeepEqual(p.Resources, []photoinfo.Resource{
				{},
				{Type: 4, Subtype: 3, Size: 7_000_000_000, LocalAvailability: 1, RemoteAvailability: 2},
			})
		}},
		{"burst", func(a *sqlread.Asset) { a.BurstUUID, a.BurstPickType = nullString("B"), nullInt(24) }, nil,
			func(p *photoInfo) bool {
				return p.Burst && p.BurstUUID == "B" && p.BurstPickType == 24 && p.BurstKey && p.BurstSelected
			}},
		{"burst with an empty UUID", func(a *sqlread.Asset) { a.BurstUUID = nullString("") }, nil,
			func(p *photoInfo) bool { return p.Burst && p.BurstUUID == "" && p.BurstPickType == 0 }},
		{"not a burst", nil, nil, func(p *photoInfo) bool { return !p.Burst && !p.BurstKey && !p.BurstSelected }},
		{"burst pick NULL", func(a *sqlread.Asset) { a.BurstUUID = nullString("B") }, nil,
			func(p *photoInfo) bool { return p.BurstPickType == 0 && !p.BurstKey && !p.BurstSelected }},
		{"hidden", func(a *sqlread.Asset) { a.Hidden = nullInt(1) }, nil, func(p *photoInfo) bool { return p.Hidden && !p.Favorite }},
		{"favorite", func(a *sqlread.Asset) { a.Favorite = nullInt(1) }, nil, func(p *photoInfo) bool { return p.Favorite && !p.Hidden }},
		{"trash", func(a *sqlread.Asset) { a.TrashedState = nullInt(1) }, nil, func(p *photoInfo) bool { return p.InTrash && p.DateTrashed == nil }},
		{"trash date without the state", func(a *sqlread.Asset) { a.Trashed = nullFloat(1) }, nil, func(p *photoInfo) bool { return !p.InTrash }},
		{"adjustments 2", func(a *sqlread.Asset) { a.Adjustments = nullInt(2) }, nil, func(p *photoInfo) bool { return p.HasAdjustments }},
		{"adjustments NULL", func(a *sqlread.Asset) { a.Adjustments = sql.NullInt64{} }, nil, func(p *photoInfo) bool { return !p.HasAdjustments }},
		{"shared", func(a *sqlread.Asset) { a.CloudBatchPublishDate = nullFloat(0) }, nil, func(p *photoInfo) bool { return p.Shared }},
		{"syndicated, not saved", func(a *sqlread.Asset) { a.SyndicationIdentifier = nullString("S") }, nil,
			func(p *photoInfo) bool { return p.Syndicated && !p.SavedToLibrary }},
		{"syndicated, history NULL", func(a *sqlread.Asset) {
			a.SyndicationIdentifier, a.SyndicationHistory = nullString("S"), sql.NullInt64{}
		}, nil, func(p *photoInfo) bool { return p.Syndicated && p.SavedToLibrary }},
		{"not syndicated, history 1", func(a *sqlread.Asset) { a.SyndicationHistory = nullInt(1) }, nil,
			func(p *photoInfo) bool { return !p.Syndicated && p.SavedToLibrary }},
		{"shared library", func(a *sqlread.Asset) {
			a.ScopeParticipation, a.ScopeShareState, a.Scope = nullInt(1), nullInt(2), nullInt(7)
		}, nil, func(p *photoInfo) bool {
			return p.SharedLibrary && p.LibraryScopeShareState == 2 && p.LibraryScope == 7
		}},
		{"shared library NULL", func(a *sqlread.Asset) {
			a.ScopeParticipation, a.ScopeShareState, a.Scope = sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}
		}, nil, func(p *photoInfo) bool {
			return !p.SharedLibrary && p.LibraryScopeShareState == 0 && p.LibraryScope == 0
		}},
		{"reference", func(a *sqlread.Asset) { a.SavedAssetType = nullInt(10) }, nil, func(p *photoInfo) bool { return p.IsReference }},
		{"title and description NFC", func(a *sqlread.Asset) { a.Title, a.Description = nullString(nfd), nullString(nfd+"!") }, nil,
			func(p *photoInfo) bool { return p.Title == nfcForm && p.Description == nfcForm+"!" }},
		{"title and description NULL", nil, nil, func(p *photoInfo) bool { return p.Title == "" && p.Description == "" }},
		{"no coordinates", nil, nil, func(p *photoInfo) bool { return p.Latitude == nil && p.Longitude == nil }},
		{"coordinates", func(a *sqlread.Asset) { a.Latitude, a.Longitude = nullFloat(55.75), nullFloat(-180) }, nil,
			func(p *photoInfo) bool {
				return p.Latitude != nil && *p.Latitude == 55.75 && p.Longitude != nil && *p.Longitude == -180
			}},
		{"coordinates NULL", func(a *sqlread.Asset) { a.Latitude, a.Longitude = sql.NullFloat64{}, nullFloat(37.5) }, nil,
			func(p *photoInfo) bool { return p.Latitude == nil && p.Longitude != nil && *p.Longitude == 37.5 }},
	}
	for _, tt := range tests {
		a := baseAsset(1, "A-1")
		if tt.edit != nil {
			tt.edit(&a)
		}
		p := loadOne(t, a, slices.Clone(tt.res)...)
		if !tt.check(p) {
			t.Errorf("%s: got %+v", tt.name, *p)
		}
	}
}

func TestLoadKindFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subtype sql.NullInt64
		want    [3]bool // SlowMo, TimeLapse, ScreenRecording
	}{
		{"no subtype", nullInt(0), [3]bool{}},
		{"panorama", nullInt(1), [3]bool{}},
		{"live", nullInt(2), [3]bool{}},
		{"screenshot", nullInt(10), [3]bool{}},
		{"slow motion", nullInt(101), [3]bool{true, false, false}},
		{"time lapse", nullInt(102), [3]bool{false, true, false}},
		{"screen recording", nullInt(103), [3]bool{false, false, true}},
		{"100", nullInt(100), [3]bool{}},
		{"104", nullInt(104), [3]bool{}},
		{"NULL", sql.NullInt64{}, [3]bool{}},
	}
	for _, tt := range tests {
		a := baseAsset(1, "A-1")
		a.KindSubtype = tt.subtype
		p := loadOne(t, a)
		if got := [3]bool{p.SlowMo, p.TimeLapse, p.ScreenRecording}; got != tt.want {
			t.Errorf("%s: SlowMo, TimeLapse, ScreenRecording = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoadBurstDefaultPick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		pick  sql.NullInt64
		burst bool
		want  bool
	}{
		// burst_default_pick does not look at the burst (photoinfo.py:1021-1029).
		{"default, not in a burst", nullInt(4), false, true},
		{"NULL", sql.NullInt64{}, true, false},
		{"0", nullInt(0), true, false},
		{"selected", nullInt(8), true, false},
		{"key", nullInt(16), true, false},
		{"default", nullInt(4), true, true},
		{"default and key", nullInt(20), true, true},
		{"default and an unknown bit", nullInt(36), true, true},
	}
	for _, tt := range tests {
		a := baseAsset(1, "A-1")
		a.BurstPickType = tt.pick
		if tt.burst {
			a.BurstUUID = nullString("B")
		}
		if got := loadOne(t, a).BurstDefaultPick; got != tt.want {
			t.Errorf("%s: BurstDefaultPick = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoadUTIEdited(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		adjustments sql.NullInt64
		uti         sql.NullString
		want        string
	}{
		{"adjusted", nullInt(2), nullString("public.heic"), "public.heic"},
		{"not adjusted", nullInt(0), nullString("public.heic"), ""},
		{"adjustments NULL", sql.NullInt64{}, nullString("public.heic"), ""},
		{"adjusted, UTI NULL", nullInt(2), sql.NullString{}, ""},
	}
	for _, tt := range tests {
		a := baseAsset(1, "A-1")
		a.Adjustments, a.UTI = tt.adjustments, tt.uti
		if got := loadOne(t, a).UTIEdited; got != tt.want {
			t.Errorf("%s: UTIEdited = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func timeIs(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want) && got.Location() == time.UTC
}

func TestLoadKeywordsAlbumsPersons(t *testing.T) {
	t.Parallel()

	const nfd = "Café"
	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2")},
		keywords: []sqlread.Keyword{
			{AssetPK: 1, Title: nullString("b")}, {AssetPK: 1, Title: nullString(nfd)}, {AssetPK: 1, Title: sql.NullString{}},
			{AssetPK: 1, Title: nullString("b")}, {AssetPK: 1, Title: nullString("B")}, {AssetPK: 99, Title: nullString("orphan")},
		},
		members: []sqlread.Member{
			{AssetPK: 1, AlbumUUID: "AL-2"}, {AssetPK: 1, AlbumUUID: "AL-1"}, {AssetPK: 1, AlbumUUID: "AL-2"}, {AssetPK: 99, AlbumUUID: "AL-9"},
		},
		persons:   []int64{2, 99},
		resources: []sqlread.Resource{{PK: 1, AssetPK: 99, Subtype: nullInt(17)}},
	}
	c, err := load(in, "ZMASTERFINGERPRINT")
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := c.byUUID["A-1"], c.byUUID["A-2"]
	// Upstream keeps a repeated keyword (photosdb.py:2005-2010) and sorts the
	// list (photoinfo.py:705-708); NULL is "".
	if want := []string{"", "B", "Café", "b", "b"}; !slices.Equal(p1.Keywords, want) {
		t.Errorf("Keywords = %q, want %q", p1.Keywords, want)
	}
	if want := []string{"AL-1", "AL-2"}; !slices.Equal(p1.AlbumUUIDs, want) || !slices.Equal(p1.BurstAlbumUUIDs, want) {
		t.Errorf("AlbumUUIDs = %q, BurstAlbumUUIDs = %q, want %q", p1.AlbumUUIDs, p1.BurstAlbumUUIDs, want)
	}
	if p1.HasPersons || !p2.HasPersons {
		t.Errorf("HasPersons = %v, %v; want false, true", p1.HasPersons, p2.HasPersons)
	}
	if len(p2.Keywords) != 0 || len(p2.AlbumUUIDs) != 0 || p2.HasRAW {
		t.Errorf("A-2 = %+v, want no keywords, albums or RAW", *p2)
	}
}

// TestLoadOtherAssetFingerprint: a resource of another asset with the
// fingerprint of the asset does not decide IsMissing (upstream it does,
// photosdb.py:2498-2516); the last of two originals of the asset does.
func TestLoadOtherAssetFingerprint(t *testing.T) {
	t.Parallel()

	a, b := baseAsset(1, "A-1"), baseAsset(2, "A-2")
	a.Fingerprint = nullString("FP")
	in := tables{
		assets: []sqlread.Asset{a, b},
		resources: []sqlread.Resource{
			{PK: 1, AssetPK: 1, Subtype: nullInt(1), Local: nullInt(0)},
			{PK: 2, AssetPK: 1, Subtype: nullInt(1), Local: nullInt(1)},
			{PK: 3, AssetPK: 2, Subtype: nullInt(1), Local: nullInt(0), Fingerprint: nullString("FP")},
		},
	}
	c, err := load(in, "Z")
	if err != nil {
		t.Fatal(err)
	}
	if c.byUUID["A-1"].IsMissing || !c.byUUID["A-2"].IsMissing {
		t.Errorf("IsMissing = %v, %v; want false, true", c.byUUID["A-1"].IsMissing, c.byUUID["A-2"].IsMissing)
	}
}

func TestLoadBurstAlbums(t *testing.T) {
	t.Parallel()

	burst := func(pk int64, uuid, burstUUID string, pick int64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		a.BurstUUID, a.BurstPickType = nullString(burstUUID), nullInt(pick)
		return a
	}
	in := tables{
		assets: []sqlread.Asset{
			burst(1, "K", "B1", 16), burst(2, "S", "B1", 8), burst(3, "U", "B1", 2),
			burst(4, "E1", "", 16), burst(5, "E2", "", 0),
			baseAsset(6, "N"),
			burst(7, "O", "B2", 2), // a burst without a key frame
		},
		members: []sqlread.Member{
			{AssetPK: 1, AlbumUUID: "AL-K"}, {AssetPK: 2, AlbumUUID: "AL-S"}, {AssetPK: 4, AlbumUUID: "AL-E"},
			{AssetPK: 6, AlbumUUID: "AL-N"}, {AssetPK: 7, AlbumUUID: "AL-O"},
		},
	}
	c, err := load(in, "ZORIGINALSTABLEHASH")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"K":  {"AL-K"},
		"S":  {"AL-K", "AL-S"},
		"U":  {"AL-K"},
		"E1": {"AL-E"},
		"E2": {"AL-E"},
		"N":  {"AL-N"},
		"O":  {"AL-O"},
	}
	for uuid, w := range want {
		if got := c.byUUID[uuid].BurstAlbumUUIDs; !slices.Equal(got, w) {
			t.Errorf("%s: BurstAlbumUUIDs = %q, want %q", uuid, got, w)
		}
	}
}

func TestLoadSkippedAndUUID(t *testing.T) {
	t.Parallel()

	skipped := baseAsset(3, "A-3")
	skipped.HasAttributes = false
	c, err := load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1"), skipped, baseAsset(2, "A-2")}}, "Z")
	if err != nil {
		t.Fatal(err)
	}
	if c.skipped != 1 || len(c.photos) != 2 || c.byUUID["A-3"] != nil {
		t.Errorf("load: skipped %d, %d photos, A-3 %v; want 1, 2, nil", c.skipped, len(c.photos), c.byUUID["A-3"])
	}

	noUUID := baseAsset(4, "A-4")
	noUUID.UUID = sql.NullString{}
	emptyUUID := baseAsset(5, "A-5")
	emptyUUID.UUID = nullString("")
	tests := []struct {
		name   string
		assets []sqlread.Asset
		want   string
	}{
		{"two assets with one UUID", []sqlread.Asset{baseAsset(1, "SECRET-UUID"), baseAsset(2, "SECRET-UUID")}, "1 assets with the ZASSET.ZUUID of another"},
		{"three assets with one UUID", []sqlread.Asset{baseAsset(1, "SECRET-UUID"), baseAsset(2, "SECRET-UUID"), baseAsset(3, "SECRET-UUID")}, "2 assets"},
		{"no UUID", []sqlread.Asset{baseAsset(1, "SECRET-UUID"), noUUID}, "1 assets without ZASSET.ZUUID"},
		{"empty UUID", []sqlread.Asset{noUUID, emptyUUID}, "2 assets without ZASSET.ZUUID"},
	}
	for _, tt := range tests {
		_, err := load(tables{assets: tt.assets}, "Z")
		if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: load error %v, want one with %q and without values", tt.name, err, tt.want)
		}
	}

	// An asset without attributes never reaches the catalog (upstream drops
	// it, photosdb.py:2086-2090), so its ZUUID is not checked.
	skippedNoUUID := baseAsset(6, "A-6")
	skippedNoUUID.UUID, skippedNoUUID.HasAttributes = sql.NullString{}, false
	skippedDup := baseAsset(7, "A-1")
	skippedDup.HasAttributes = false
	c, err = load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1"), skippedNoUUID, skippedDup}}, "Z")
	if err != nil {
		t.Fatalf("assets without attributes, NULL and repeated ZUUID: %v", err)
	}
	if c.skipped != 2 || len(c.photos) != 1 || c.byUUID["A-1"] == nil || c.byUUID["A-1"].UUID != "A-1" {
		t.Errorf("load: skipped %d, %d photos; want 2, 1 (A-1)", c.skipped, len(c.photos))
	}
}

// TestLoadHostileBurst is one burst that holds every asset, all of them key
// frames in one album: load stays fast, and every BurstAlbumUUIDs holds the
// album once.
func TestLoadHostileBurst(t *testing.T) {
	t.Parallel()

	const n = 20000
	in := tables{assets: make([]sqlread.Asset, 0, n), members: make([]sqlread.Member, 0, n)}
	for i := range n {
		a := baseAsset(int64(i+1), fmt.Sprintf("U-%05d", i))
		a.BurstUUID, a.BurstPickType = nullString(""), nullInt(16)
		in.assets = append(in.assets, a)
		in.members = append(in.members, sqlread.Member{AssetPK: a.PK, AlbumUUID: "AL"})
	}
	c, err := load(in, "Z")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.photos {
		if got := p.BurstAlbumUUIDs; len(got) != 1 || got[0] != "AL" {
			t.Fatalf("%s: BurstAlbumUUIDs %q, want [AL]", p.UUID, got)
		}
	}
}

// TestLoadProperties shuffles the keyword, album and person rows, whose
// readers give no order load relies on: the result does not depend on their
// order, and the invariants hold.
func TestLoadProperties(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // G404: deterministic test data
	var in tables
	for i := range 60 {
		a := baseAsset(int64(i+1), fmt.Sprintf("U-%03d", r.IntN(1000)*100+i))
		a.HasAttributes = i%13 != 0
		if i%4 == 0 {
			a.BurstUUID, a.BurstPickType = nullString(fmt.Sprintf("B%d", i%3)), nullInt(int64([]int{0, 2, 8, 16}[r.IntN(4)]))
		}
		in.assets = append(in.assets, a)
		for k := range r.IntN(4) {
			var res sqlread.Resource
			res.PK, res.AssetPK = int64(1000*i+k), a.PK
			res.Subtype = nullInt(int64([]int{1, 3, 17, 18}[r.IntN(4)]))
			res.Type, res.Local = nullInt(int64(r.IntN(5))), nullInt(int64(r.IntN(2)))
			in.resources = append(in.resources, res)
		}
		for range r.IntN(3) {
			in.keywords = append(in.keywords, sqlread.Keyword{AssetPK: a.PK, Title: nullString(fmt.Sprintf("k%d", r.IntN(5)))})
			in.members = append(in.members, sqlread.Member{AssetPK: a.PK, AlbumUUID: fmt.Sprintf("AL-%d", r.IntN(6))})
		}
		if r.IntN(3) == 0 {
			in.persons = append(in.persons, a.PK)
		}
	}
	want, err := load(in, "Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(want.photos)+want.skipped != len(in.assets) {
		t.Errorf("%d photos + %d skipped, want %d assets", len(want.photos), want.skipped, len(in.assets))
	}
	for _, p := range want.photos {
		if !slices.IsSorted(p.Keywords) || !slices.IsSorted(p.AlbumUUIDs) || len(slices.Compact(slices.Clone(p.AlbumUUIDs))) != len(p.AlbumUUIDs) {
			t.Errorf("%s: Keywords %q or AlbumUUIDs %q not sorted or repeated", p.UUID, p.Keywords, p.AlbumUUIDs)
		}
		for _, u := range p.AlbumUUIDs {
			if !slices.Contains(p.BurstAlbumUUIDs, u) {
				t.Errorf("%s: BurstAlbumUUIDs %q lacks %s of AlbumUUIDs", p.UUID, p.BurstAlbumUUIDs, u)
			}
		}
		if !p.Burst && !slices.Equal(p.BurstAlbumUUIDs, p.AlbumUUIDs) {
			t.Errorf("%s: not a burst, BurstAlbumUUIDs %q != AlbumUUIDs %q", p.UUID, p.BurstAlbumUUIDs, p.AlbumUUIDs)
		}
	}
	for range 50 {
		s := tables{
			assets: in.assets, resources: in.resources, keywords: slices.Clone(in.keywords),
			persons: slices.Clone(in.persons), members: slices.Clone(in.members),
		}
		r.Shuffle(len(s.keywords), func(i, j int) { s.keywords[i], s.keywords[j] = s.keywords[j], s.keywords[i] })
		r.Shuffle(len(s.persons), func(i, j int) { s.persons[i], s.persons[j] = s.persons[j], s.persons[i] })
		r.Shuffle(len(s.members), func(i, j int) { s.members[i], s.members[j] = s.members[j], s.members[i] })
		got, err := load(s, "Z")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("load of shuffled rows differs from load of the rows in order")
		}
	}
}
