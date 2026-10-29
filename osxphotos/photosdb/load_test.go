package photosdb

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// PK is the value of {id} (phototemplate.py:977-978).
func TestLoadPK(t *testing.T) {
	t.Parallel()

	if got := loadOne(t, baseAsset(7, "A-1")).PK; got != 7 {
		t.Errorf("PK = %d, want 7", got)
	}
	var mu sync.Mutex
	apart := 0
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				photos := db.Photos()
				if len(photos) == 0 {
					t.Fatal("no photos: the test is vacuous")
				}
				ro := roDB(t, lib.path)
				want := map[string]string{}
				for _, row := range testlib.Rows(t, ro, "SELECT a.ZUUID || ':' || a.Z_PK FROM ZASSET a "+
					"JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK WHERE a.ZUUID IS NOT NULL") {
					uuid, pk, _ := strings.Cut(row, ":")
					want[uuid] = pk
				}
				for _, p := range photos {
					if want[p.UUID] != strconv.FormatInt(p.PK, 10) {
						t.Errorf("photo %s: PK %d, want its ZASSET.Z_PK %s", p.UUID, p.PK, want[p.UUID])
					}
				}
				n := testlib.Count(t, ro, "SELECT COUNT(*) FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK WHERE x.Z_PK != a.Z_PK")
				t.Logf("%d photos, %d with a ZADDITIONALASSETATTRIBUTES.Z_PK apart from ZASSET.Z_PK", len(photos), n)
				mu.Lock()
				defer mu.Unlock()
				apart += n
			})
		}
	})
	if apart == 0 {
		t.Error("ZADDITIONALASSETATTRIBUTES.Z_PK equals ZASSET.Z_PK in every library: the test cannot tell the two keys apart")
	}
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

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
	a.HDRType, a.DepthType, a.VisibilityState, a.CameraCaptureDevice = nullInt(0), nullInt(0), nullInt(0), nullInt(0)
	return a
}

func loadOne(t *testing.T, a sqlread.Asset, res ...sqlread.Resource) *photoinfo.PhotoInfo {
	t.Helper()
	for i := range res {
		res[i].AssetPK = a.PK
		if res[i].PK == 0 {
			res[i].PK = int64(i + 1)
		}
	}
	c, err := load(tables{assets: []sqlread.Asset{a}, resources: res})
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

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9"
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
			func(p *photoInfo) bool { return p.Fingerprint == "FP" }},
		{"fingerprint NULL", nil, nil, func(p *photoInfo) bool { return p.Fingerprint == "" }},
		{"fingerprint empty", func(a *sqlread.Asset) { a.Fingerprint = nullString("") }, nil,
			func(p *photoInfo) bool { return p.Fingerprint == "" }},
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
		{"subtype 3 type 4 is not RAW", nil, []sqlread.Resource{resource(1, 0, nullInt(1)), resource(3, 4, nullInt(1))},
			func(p *photoInfo) bool { return !p.HasRAW }},
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
			a.ScopeParticipation, a.ScopeShareState = nullInt(1), nullInt(2)
		}, nil, func(p *photoInfo) bool {
			return p.SharedLibrary && p.LibraryScopeShareState == 2
		}},
		{"shared library NULL", func(a *sqlread.Asset) {
			a.ScopeParticipation, a.ScopeShareState = sql.NullInt64{}, sql.NullInt64{}
		}, nil, func(p *photoInfo) bool {
			return !p.SharedLibrary && p.LibraryScopeShareState == 0
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
		want    [3]bool
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

func TestLoadColumns(t *testing.T) {
	t.Parallel()

	const externalEdit = "com.apple.Photos.externalEdit"
	tests := []struct {
		name  string
		edit  func(a *sqlread.Asset)
		check func(p *photoInfo) bool
	}{
		{"none", nil, func(p *photoInfo) bool {
			return !p.HDR && !p.Panorama && !p.Portrait && !p.Selfie && p.Visible && p.Spatial == 0 && p.TZName == "" &&
				p.CloudOwnerHashedID == "" && !p.SharedMoment && !p.ExternalEdit && p.ImportedByName == "" && p.ImportedByBundleID == ""
		}},
		{"HDR", func(a *sqlread.Asset) { a.HDRType = nullInt(3) }, func(p *photoInfo) bool { return p.HDR && !p.Panorama }},
		{"HDR type 4", func(a *sqlread.Asset) { a.HDRType = nullInt(4) }, func(p *photoInfo) bool { return !p.HDR && !p.Panorama }},
		{"HDR type NULL", func(a *sqlread.Asset) { a.HDRType = sql.NullInt64{} }, func(p *photoInfo) bool { return !p.HDR && !p.Panorama }},
		{"panorama subtype", func(a *sqlread.Asset) { a.KindSubtype = nullInt(1) }, func(p *photoInfo) bool { return p.Panorama && !p.HDR }},
		{"panorama HDR type 6 without the subtype", func(a *sqlread.Asset) { a.HDRType = nullInt(6) },
			func(p *photoInfo) bool { return p.Panorama && !p.HDR }},
		{"portrait", func(a *sqlread.Asset) { a.DepthType = nullInt(1) }, func(p *photoInfo) bool { return p.Portrait }},
		// Python None != 0 (photosdb.py:2251).
		{"portrait depth type NULL", func(a *sqlread.Asset) { a.DepthType = sql.NullInt64{} }, func(p *photoInfo) bool { return p.Portrait }},
		{"selfie", func(a *sqlread.Asset) { a.CameraCaptureDevice = nullInt(1) }, func(p *photoInfo) bool { return p.Selfie }},
		{"camera 2", func(a *sqlread.Asset) { a.CameraCaptureDevice = nullInt(2) }, func(p *photoInfo) bool { return !p.Selfie }},
		{"camera NULL", func(a *sqlread.Asset) { a.CameraCaptureDevice = sql.NullInt64{} }, func(p *photoInfo) bool { return !p.Selfie }},
		{"not visible", func(a *sqlread.Asset) { a.VisibilityState = nullInt(2) }, func(p *photoInfo) bool { return !p.Visible }},
		{"visibility 1", func(a *sqlread.Asset) { a.VisibilityState = nullInt(1) }, func(p *photoInfo) bool { return !p.Visible }},
		{"visibility NULL", func(a *sqlread.Asset) { a.VisibilityState = sql.NullInt64{} }, func(p *photoInfo) bool { return !p.Visible }},
		{"spatial", func(a *sqlread.Asset) { a.SpatialType = nullInt(2) }, func(p *photoInfo) bool { return p.Spatial == 2 }},
		{"time zone name", func(a *sqlread.Asset) { a.TZName = nullString("Europe/Paris") }, func(p *photoInfo) bool { return p.TZName == "Europe/Paris" }},
		{"date in the zone of the name", func(a *sqlread.Asset) { a.TZName, a.TZOffset = nullString("Europe/Paris"), nullInt(7200) },
			func(p *photoInfo) bool {
				_, off := p.Date.Zone()
				return p.Date.Location().String() == "Europe/Paris" && off == 3600 && *p.TZOffset == 7200
			}},
		{"cloud owner", func(a *sqlread.Asset) { a.CloudOwnerHashedID = nullString("TEST-OWNER") },
			func(p *photoInfo) bool { return p.CloudOwnerHashedID == "TEST-OWNER" }},
		{"shared moment", func(a *sqlread.Asset) { a.MomentShare = nullInt(5) }, func(p *photoInfo) bool { return p.SharedMoment }},
		{"shared moment 0", func(a *sqlread.Asset) { a.MomentShare = nullInt(0) }, func(p *photoInfo) bool { return !p.SharedMoment }},
		{"ExternalEdit", func(a *sqlread.Asset) { a.AdjustmentFormat = nullString(externalEdit) }, func(p *photoInfo) bool { return p.ExternalEdit }},
		{"ExternalEdit of another adjustment format", func(a *sqlread.Asset) { a.AdjustmentFormat = nullString("com.apple.photo") },
			func(p *photoInfo) bool { return !p.ExternalEdit }},
		{"adjustment type", func(a *sqlread.Asset) { a.Adjustments = nullInt(2) },
			func(p *photoInfo) bool { return p.AdjustmentType != nil && *p.AdjustmentType == 2 }},
		{"adjustment type 0", nil, func(p *photoInfo) bool { return p.AdjustmentType != nil && *p.AdjustmentType == 0 }},
		{"adjustment type NULL", func(a *sqlread.Asset) { a.Adjustments = sql.NullInt64{} }, func(p *photoInfo) bool { return p.AdjustmentType == nil }},
		{"imported by", func(a *sqlread.Asset) {
			a.ImportedByName, a.ImportedByBundleID = nullString("Camera"), nullString("com.example.camera")
		}, func(p *photoInfo) bool {
			return p.ImportedByName == "Camera" && p.ImportedByBundleID == "com.example.camera"
		}},
	}
	for _, tt := range tests {
		a := baseAsset(1, "A-1")
		if tt.edit != nil {
			tt.edit(&a)
		}
		if p := loadOne(t, a); !tt.check(p) {
			t.Errorf("%s: got %+v", tt.name, *p)
		}
	}
}

func timeIs(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want) && got.Location() == time.UTC
}

func TestLoadKeywordsAlbumsPersons(t *testing.T) {
	t.Parallel()

	const nfd = "Cafe\U00000301"
	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2")},
		keywords: []sqlread.Keyword{
			{AssetPK: 1, Title: nullString("b")}, {AssetPK: 1, Title: nullString(nfd)}, {AssetPK: 1, Title: sql.NullString{}},
			{AssetPK: 1, Title: nullString("b")}, {AssetPK: 1, Title: nullString("B")}, {AssetPK: 99, Title: nullString("orphan")},
		},
		members: []sqlread.Member{
			{AssetPK: 1, AlbumUUID: "AL-2"}, {AssetPK: 1, AlbumUUID: "AL-1"}, {AssetPK: 1, AlbumUUID: "AL-2"}, {AssetPK: 99, AlbumUUID: "AL-9"},
			{AssetPK: 1, AlbumUUID: "PR-2", Kind: sqlread.KindProject}, {AssetPK: 1, AlbumUUID: "PR-1", Kind: sqlread.KindProject},
			{AssetPK: 1, AlbumUUID: "AL-3", Kind: sqlread.KindUserAlbum},
		},
		faces:     []sqlread.Face{{AssetPK: 2, FullName: nullString("Person A")}, {AssetPK: 99, FullName: nullString("Person B")}},
		resources: []sqlread.Resource{{PK: 1, AssetPK: 99, Subtype: nullInt(17)}},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := c.byUUID["A-1"], c.byUUID["A-2"]
	// Upstream keeps a repeated keyword (photosdb.py:2005-2010) and sorts the
	// list (photoinfo.py:705-708); NULL is "".
	if want := []string{"", "B", "Caf\U000000e9", "b", "b"}; !slices.Equal(p1.Keywords, want) {
		t.Errorf("Keywords = %q, want %q", p1.Keywords, want)
	}
	if want := []string{"AL-1", "AL-2", "AL-3"}; !slices.Equal(p1.AlbumUUIDs, want) {
		t.Errorf("AlbumUUIDs = %q, want %q", p1.AlbumUUIDs, want)
	}
	if want := []string{"PR-1", "PR-2"}; !slices.Equal(p1.ProjectUUIDs, want) || p2.ProjectUUIDs != nil {
		t.Errorf("ProjectUUIDs = %q and %q, want %q and none", p1.ProjectUUIDs, p2.ProjectUUIDs, want)
	}
	if p1.Persons != nil || !slices.Equal(p2.Persons, []string{"Person A"}) {
		t.Errorf("Persons of %d and %d names; want 0 and 1", len(p1.Persons), len(p2.Persons))
	}
	if len(p2.Keywords) != 0 || len(p2.AlbumUUIDs) != 0 || p2.HasRAW {
		t.Errorf("A-2 = %+v, want no keywords, albums or RAW", *p2)
	}
}

// Upstream names a person without a name "_UNKNOWN_" (photosdb.py:1792-1796)
// and lists a name once per face (photosdb.py:1858-1864, photoinfo.py:597-603).
func TestLoadPersons(t *testing.T) {
	t.Parallel()

	const nfd, nfcForm = "Cafe\U00000301", "Caf\U000000e9"
	face := func(pk int64, name sql.NullString) sqlread.Face { return sqlread.Face{AssetPK: pk, FullName: name} }
	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), baseAsset(3, "A-3"), baseAsset(4, "A-4"), baseAsset(5, "A-5")},
		faces: []sqlread.Face{
			face(1, nullString("Person B")), face(1, sql.NullString{}), face(1, nullString("Person A")),
			face(1, nullString("Person B")), face(1, nullString("")), face(1, nullString(nfd)),
			face(2, sql.NullString{}), face(3, nullString("")), face(99, nullString("Person C")),
			face(5, nullString("_UNKNOWN_")),
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		uuid    string
		persons []string
	}{
		{"A-1", []string{nfcForm, "Person A", "Person B", "Person B", "_UNKNOWN_", "_UNKNOWN_"}},
		{"A-2", []string{"_UNKNOWN_"}},
		{"A-3", []string{"_UNKNOWN_"}},
		{"A-4", nil},
		{"A-5", []string{"_UNKNOWN_"}},
	}
	for _, tt := range tests {
		p := c.byUUID[tt.uuid]
		if !slices.Equal(p.Persons, tt.persons) || (p.Persons == nil) != (tt.persons == nil) {
			t.Errorf("%s: %d persons; want %d", tt.uuid, len(p.Persons), len(tt.persons))
		}
	}
}

func exifPtr[T any](v T) *T { return &v }

// The expected values are literals: ExifInfo of a photo from one row of
// ZEXTENDEDATTRIBUTES (exifinfo.py:42-77, photoinfo.py:147-163, 1687-1704).
// CameraMake stays decomposed: upstream does not normalize the strings of the
// row (exifinfo.py:63-66).
func TestLoadExifRows(t *testing.T) {
	t.Parallel()

	const nfd = "Cafe\U00000301"
	// A-1 has the date 2023-01-01T00:00:00Z.
	loadA1 := func(t *testing.T, rows ...sqlread.Exif) *photoinfo.PhotoInfo {
		t.Helper()
		c, err := load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2")}, exif: rows})
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return c.byUUID["A-1"]
	}

	t.Run("no row", func(t *testing.T) {
		t.Parallel()
		p := loadA1(t, sqlread.Exif{PK: 1, AssetPK: 99, ISO: nullInt(50)}, sqlread.Exif{PK: 2, AssetPK: 2, ISO: nullInt(50)})
		if !reflect.DeepEqual(p.ExifInfo, exifinfo.ExifInfo{}) {
			t.Errorf("ExifInfo = %+v, want the zero value", p.ExifInfo)
		}
		if !p.DateOriginal.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)) || !p.DateOriginal.Equal(p.Date) {
			t.Errorf("DateOriginal = %v, want Date %v", p.DateOriginal, p.Date)
		}
	})

	t.Run("a row of NULLs", func(t *testing.T) {
		t.Parallel()
		p := loadA1(t, sqlread.Exif{PK: 1, AssetPK: 1})
		if want := (exifinfo.ExifInfo{FlashFired: exifPtr(false)}); !reflect.DeepEqual(p.ExifInfo, want) {
			t.Errorf("ExifInfo = %+v, want only FlashFired false", p.ExifInfo)
		}
		if !p.DateOriginal.Equal(p.Date) {
			t.Errorf("DateOriginal = %v, want Date %v", p.DateOriginal, p.Date)
		}
	})

	t.Run("a row of values", func(t *testing.T) {
		t.Parallel()
		p := loadA1(t, sqlread.Exif{
			PK: 1, AssetPK: 1,
			FlashFired: nullInt(1), ISO: nullInt(100), MeteringMode: nullInt(5), SampleRate: nullInt(48000), TrackFormat: nullInt(1), WhiteBalance: nullInt(0),
			Aperture: nullFloat(1.8), BitRate: nullFloat(1000.5), Duration: nullFloat(12.345678901234), ExposureBias: nullFloat(-0.5),
			FocalLength: nullFloat(4.25), FPS: nullFloat(29.97), Latitude: nullFloat(48.8566), Longitude: nullFloat(2.3522), ShutterSpeed: nullFloat(0.008),
			CameraMake: nullString(nfd), CameraModel: nullString("Acme One"), Codec: nullString("hvc1"), LensModel: nullString(""),
			DateCreated: nullFloat(694310400), TZOffset: nullInt(19800), TZName: nullString("Asia/Kolkata"),
		})
		want := exifinfo.ExifInfo{
			FlashFired: exifPtr(true), ISO: exifPtr(100), MeteringMode: exifPtr(5), SampleRate: exifPtr(48000), TrackFormat: exifPtr(1), WhiteBalance: exifPtr(0),
			Aperture: exifPtr(1.8), BitRate: exifPtr(1000.5), Duration: exifPtr(12.345678901234), ExposureBias: exifPtr(-0.5),
			FocalLength: exifPtr(4.25), FPS: exifPtr(29.97), Latitude: exifPtr(48.8566), Longitude: exifPtr(2.3522), ShutterSpeed: exifPtr(0.008),
			CameraMake: nfd, CameraModel: "Acme One", Codec: "hvc1", LensModel: "",
			TZOffset: exifPtr(19800), TZName: "Asia/Kolkata",
		}
		got := p.ExifInfo
		d := got.Date
		if d == nil || !d.Equal(time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("Date = %v, want 2023-01-02T00:00:00Z", d)
		}
		if _, off := d.Zone(); off != 19800 {
			t.Errorf("Date has the offset %d, want 19800", off)
		}
		got.Date = nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ExifInfo = %+v, want %+v", got, want)
		}
		if !p.DateOriginal.Equal(*d) || p.DateOriginal.Equal(p.Date) {
			t.Errorf("DateOriginal = %v, want the date of ExifInfo %v and not Date %v", p.DateOriginal, *d, p.Date)
		}
	})

	t.Run("the date", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			created sql.NullFloat64
			offset  sql.NullInt64
			unix    int64
			zone    int
			isNil   bool
		}{
			{name: "NULL", offset: nullInt(3600), isNil: true},
			{name: "after the year 9999", created: nullFloat(1e18), isNil: true},
			{name: "before the year 1", created: nullFloat(-1e18), isNil: true},
			{name: "NaN", created: nullFloat(math.NaN()), isNil: true},
			{name: "offset 86400", created: nullFloat(694224000), offset: nullInt(86400), isNil: true},
			{name: "offset -86400", created: nullFloat(694224000), offset: nullInt(-86400), isNil: true},
			{name: "the epoch is a date", created: nullFloat(-978307200)},
			{name: "offset NULL", created: nullFloat(694224000), unix: 1672531200},
			{name: "offset 3600", created: nullFloat(694224000), offset: nullInt(3600), unix: 1672531200, zone: 3600},
		}
		for _, tt := range tests {
			p := loadA1(t, sqlread.Exif{PK: 1, AssetPK: 1, DateCreated: tt.created, TZOffset: tt.offset})
			d := p.ExifInfo.Date
			if tt.isNil {
				if d != nil || !p.DateOriginal.Equal(p.Date) {
					t.Errorf("%s: Date %v and DateOriginal %v, want nil and Date %v", tt.name, d, p.DateOriginal, p.Date)
				}
				continue
			}
			if d == nil {
				t.Errorf("%s: Date is nil", tt.name)
				continue
			}
			if _, off := d.Zone(); d.Unix() != tt.unix || off != tt.zone || !p.DateOriginal.Equal(*d) {
				t.Errorf("%s: Date %v (offset %d), DateOriginal %v; want unix %d, offset %d", tt.name, d, off, p.DateOriginal, tt.unix, tt.zone)
			}
		}
	})

	t.Run("the highest Z_PK decides", func(t *testing.T) {
		t.Parallel()
		first := sqlread.Exif{PK: 1, AssetPK: 1, CameraMake: nullString("First"), ISO: nullInt(100)}
		second := sqlread.Exif{PK: 2, AssetPK: 1, CameraMake: nullString("Second")}
		for _, rows := range [][]sqlread.Exif{{first, second}, {second, first}} {
			if e := loadA1(t, rows...).ExifInfo; e.CameraMake != "Second" || e.ISO != nil {
				t.Errorf("CameraMake %q and ISO %v, want the row Second whole", e.CameraMake, e.ISO)
			}
		}
	})
}

// The oracle is the database. The messages hold field names, photo indexes and
// counts, not the values of a library.
func TestLoadExif(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var makes, dates int
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				hasRow := map[string]bool{}
				for _, pk := range testlib.Rows(t, ro, "SELECT DISTINCT ZASSET FROM ZEXTENDEDATTRIBUTES WHERE ZASSET IS NOT NULL") {
					hasRow[pk] = true
				}
				const oracle = "SELECT ZCAMERAMAKE, ZISO, ZFOCALLENGTH, ZDATECREATED, ZTIMEZONEOFFSET FROM ZEXTENDEDATTRIBUTES WHERE ZASSET = ? ORDER BY Z_PK DESC LIMIT 1"
				var rows, withMake, withDate, apart, checked int
				for i, p := range db.Photos() {
					e := p.ExifInfo
					want := p.Date
					if e.Date != nil {
						want = *e.Date
					}
					if !p.DateOriginal.Equal(want) {
						t.Errorf("photo %d: DateOriginal is not the date of ExifInfo, or Date without one", i)
					}
					if !p.DateOriginal.Equal(p.Date) {
						apart++
					}
					if e.CameraMake != "" {
						withMake++
					}
					if e.Date != nil {
						withDate++
					}
					if has := hasRow[strconv.FormatInt(p.PK, 10)]; has != (e.FlashFired != nil) {
						t.Errorf("photo %d: a row in the database %v, a row in ExifInfo %v", i, has, e.FlashFired != nil)
					}
					if e.FlashFired == nil {
						if !reflect.DeepEqual(e, exifinfo.ExifInfo{}) {
							t.Errorf("photo %d: no row, but ExifInfo is not the zero value", i)
						}
						continue
					}
					rows++
					if checked >= 5 {
						continue
					}
					checked++
					var (
						maker          sql.NullString
						iso, offset    sql.NullInt64
						focal, created sql.NullFloat64
					)
					if err := ro.QueryRowContext(t.Context(), oracle, p.PK).Scan(&maker, &iso, &focal, &created, &offset); err != nil {
						t.Fatalf("photo %d: the query of ZEXTENDEDATTRIBUTES failed", i)
					}
					if e.CameraMake != maker.String {
						t.Errorf("photo %d: CameraMake differs from the database", i)
					}
					if (e.ISO != nil) != iso.Valid || (iso.Valid && int64(*e.ISO) != iso.Int64) {
						t.Errorf("photo %d: ISO differs from the database", i)
					}
					if (e.FocalLength != nil) != focal.Valid || (focal.Valid && *e.FocalLength != focal.Float64) {
						t.Errorf("photo %d: FocalLength differs from the database", i)
					}
					if (e.TZOffset != nil) != offset.Valid || (offset.Valid && int64(*e.TZOffset) != offset.Int64) {
						t.Errorf("photo %d: TZOffset differs from the database", i)
					}
					if (e.Date != nil) != created.Valid {
						t.Errorf("photo %d: Date is set %v, ZDATECREATED is set %v", i, e.Date != nil, created.Valid)
					} else if created.Valid && math.Abs(float64(e.Date.Unix())-math.Floor(created.Float64+978307200)) > 1 {
						t.Errorf("photo %d: Date differs from ZDATECREATED", i)
					}
				}
				t.Logf("%d photos: %d with a row, %d with a camera make, %d with a date, %d with DateOriginal apart from Date", len(db.Photos()), rows, withMake, withDate, apart)
				mu.Lock()
				defer mu.Unlock()
				makes += withMake
				dates += withDate
			})
		}
	})
	if makes == 0 || dates == 0 {
		t.Errorf("%d camera makes and %d dates of ExifInfo in all libraries, want some of both: the test is vacuous", makes, dates)
	}
}

// Test-13.0.0 has the rows of the cases: a photo without a row, a row of
// NULLs and a second row of one photo.
func TestLoadExifRowStates(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "Test-13.0.0")
	ids := testlib.Rows(t, roDB(t, lib), `SELECT a.ZUUID FROM ZASSET a
JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK
JOIN ZEXTENDEDATTRIBUTES e ON e.ZASSET = a.Z_PK
WHERE a.ZUUID IS NOT NULL GROUP BY a.Z_PK HAVING COUNT(*) = 1 ORDER BY a.ZUUID LIMIT 3`)
	if len(ids) != 3 {
		t.Fatalf("Test-13.0.0 has %d photos with one ZEXTENDEDATTRIBUTES row, want 3", len(ids))
	}
	w := testlib.OpenDB(t, filepath.Join(lib, "database", "Photos.sqlite"))
	w.SetMaxOpenConns(1)
	const asset = "ZASSET = (SELECT Z_PK FROM ZASSET WHERE ZUUID = ?)"
	for _, st := range []struct {
		query string
		args  []any
	}{
		{"DELETE FROM ZEXTENDEDATTRIBUTES WHERE " + asset, []any{ids[0]}},
		{"UPDATE ZEXTENDEDATTRIBUTES SET ZDATECREATED = NULL, ZCAMERAMAKE = NULL, ZISO = NULL, ZFLASHFIRED = 0 WHERE " + asset, []any{ids[1]}},
		{"CREATE TEMP TABLE second AS SELECT * FROM ZEXTENDEDATTRIBUTES WHERE 0", nil},
		{"INSERT INTO temp.second SELECT * FROM ZEXTENDEDATTRIBUTES WHERE " + asset, []any{ids[2]}},
		{"UPDATE temp.second SET Z_PK = (SELECT MAX(Z_PK) + 1 FROM main.ZEXTENDEDATTRIBUTES), ZCAMERAMAKE = 'Second Make'", nil},
		{"INSERT INTO main.ZEXTENDEDATTRIBUTES SELECT * FROM temp.second", nil},
	} {
		if _, err := w.ExecContext(t.Context(), st.query, st.args...); err != nil {
			t.Fatalf("%s: %v", st.query, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	none, nulls, second := db.byUUID[ids[0]], db.byUUID[ids[1]], db.byUUID[ids[2]]
	if none == nil || nulls == nil || second == nil {
		t.Fatal("a photo of the case is missing")
	}
	if !reflect.DeepEqual(none.ExifInfo, exifinfo.ExifInfo{}) {
		t.Error("the photo without a row: ExifInfo is not the zero value")
	}
	if !none.DateOriginal.Equal(none.Date) {
		t.Error("the photo without a row: DateOriginal is not Date")
	}
	e := nulls.ExifInfo
	for _, c := range []struct {
		field string
		ok    bool
	}{
		{"FlashFired false", e.FlashFired != nil && !*e.FlashFired},
		{"ISO nil", e.ISO == nil},
		{"CameraMake empty", e.CameraMake == ""},
		{"Date nil", e.Date == nil},
		{"DateOriginal is Date", nulls.DateOriginal.Equal(nulls.Date)},
	} {
		if !c.ok {
			t.Errorf("the row of NULLs: want %s", c.field)
		}
	}
	if second.ExifInfo.CameraMake != "Second Make" {
		t.Error("the photo with two rows: CameraMake is not the one of the second row")
	}
}

// As upstream (photosdb.py:2498-2516), the rows of the fingerprint query
// override the first pass, and the last row of a photo wins.
func TestLoadFingerprintAvailability(t *testing.T) {
	t.Parallel()

	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), baseAsset(3, "A-3")},
		resources: []sqlread.Resource{
			{PK: 1, AssetPK: 1, Subtype: nullInt(1), Local: nullInt(1)},
			{PK: 2, AssetPK: 2, Subtype: nullInt(1), Local: nullInt(0)},
			{PK: 3, AssetPK: 3, Subtype: nullInt(1), Local: nullInt(0)},
		},
		available: []sqlread.Availability{
			{PhotoUUID: nullString("A-1"), Local: nullInt(0)},
			{PhotoUUID: nullString("A-2"), Local: nullInt(0)},
			{PhotoUUID: nullString("A-2"), Local: nullInt(1)},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := []bool{c.byUUID["A-1"].IsMissing, c.byUUID["A-2"].IsMissing, c.byUUID["A-3"].IsMissing}; !slices.Equal(got, []bool{true, false, true}) {
		t.Errorf("IsMissing = %v, want [true false true]", got)
	}
}

func TestLoadSkippedAndUUID(t *testing.T) {
	t.Parallel()

	skipped := baseAsset(3, "A-3")
	skipped.HasAttributes = false
	c, err := load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1"), skipped, baseAsset(2, "A-2")}})
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
		_, err := load(tables{assets: tt.assets})
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
	c, err = load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1"), skippedNoUUID, skippedDup}})
	if err != nil {
		t.Fatalf("assets without attributes, NULL and repeated ZUUID: %v", err)
	}
	if c.skipped != 2 || len(c.photos) != 1 || c.byUUID["A-1"] == nil || c.byUUID["A-1"].UUID != "A-1" {
		t.Errorf("load: skipped %d, %d photos; want 2, 1 (A-1)", c.skipped, len(c.photos))
	}
}

// The readers of keyword, album and person rows give no order that load may
// rely on.
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
			kind := []int64{sqlread.KindUserAlbum, sqlread.KindProject}[r.IntN(2)]
			in.members = append(in.members, sqlread.Member{AssetPK: a.PK, AlbumUUID: fmt.Sprintf("AL-%d-%d", kind, r.IntN(6)), Kind: kind})
		}
		for range r.IntN(3) {
			name := []sql.NullString{{}, nullString(""), nullString("Cafe\U00000301"), nullString("p1"), nullString("p2")}[r.IntN(5)]
			in.faces = append(in.faces, sqlread.Face{AssetPK: a.PK, FullName: name})
		}
	}
	want, err := load(in)
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
		if !slices.IsSorted(p.Persons) {
			t.Errorf("%s: Persons not sorted", p.UUID)
		}
		if !slices.IsSorted(p.ProjectUUIDs) || len(slices.Compact(slices.Clone(p.ProjectUUIDs))) != len(p.ProjectUUIDs) {
			t.Errorf("%s: ProjectUUIDs %q not sorted or repeated", p.UUID, p.ProjectUUIDs)
		}
		if slices.ContainsFunc(p.AlbumUUIDs, func(u string) bool { return !strings.HasPrefix(u, "AL-2-") }) ||
			slices.ContainsFunc(p.ProjectUUIDs, func(u string) bool { return !strings.HasPrefix(u, "AL-1508-") }) {
			t.Errorf("%s: AlbumUUIDs %q or ProjectUUIDs %q hold a member of the other ZKIND", p.UUID, p.AlbumUUIDs, p.ProjectUUIDs)
		}
	}
	for range 50 {
		s := tables{
			assets: in.assets, resources: in.resources, keywords: slices.Clone(in.keywords),
			faces: slices.Clone(in.faces), members: slices.Clone(in.members),
		}
		r.Shuffle(len(s.keywords), func(i, j int) { s.keywords[i], s.keywords[j] = s.keywords[j], s.keywords[i] })
		r.Shuffle(len(s.faces), func(i, j int) { s.faces[i], s.faces[j] = s.faces[j], s.faces[i] })
		r.Shuffle(len(s.members), func(i, j int) { s.members[i], s.members[j] = s.members[j], s.members[i] })
		got, err := load(s)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("load of shuffled rows differs from load of the rows in order")
		}
	}
}
