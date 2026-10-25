package photosdb

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// A JSON null decodes as the zero value: title and description null are "",
// as upstream turns "" into None (NORMALIZATION.md). The rules put the values
// they search for in NFC, as photo_query does (photoquery.py:447-455).
type queryRef struct {
	UUID             string          `json:"uuid"`
	IsPhoto          bool            `json:"isphoto"`
	IsMovie          bool            `json:"ismovie"`
	HasAdjustments   bool            `json:"hasadjustments"`
	Favorite         bool            `json:"favorite"`
	Hidden           bool            `json:"hidden"`
	Shared           bool            `json:"shared"`
	Burst            bool            `json:"burst"`
	Live             bool            `json:"live_photo"`
	Screenshot       bool            `json:"screenshot"`
	CloudAsset       bool            `json:"iscloudasset"`
	InCloud          *bool           `json:"incloud"`
	HasRAW           bool            `json:"has_raw"`
	IsReference      bool            `json:"isreference"`
	Syndicated       bool            `json:"syndicated"`
	SavedToLibrary   bool            `json:"saved_to_library"`
	SharedLibrary    bool            `json:"shared_library"`
	ExternalEdit     bool            `json:"external_edit"`
	Portrait         bool            `json:"portrait"`
	Spatial          int             `json:"spatial"`
	ScreenRecording  bool            `json:"screen_recording"`
	SlowMo           bool            `json:"slow_mo"`
	TimeLapse        bool            `json:"time_lapse"`
	HDR              bool            `json:"hdr"`
	Selfie           bool            `json:"selfie"`
	Panorama         bool            `json:"panorama"`
	SharedMoment     bool            `json:"shared_moment"`
	Keywords         []string        `json:"keywords"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	OriginalFilename string          `json:"original_filename"`
	UTIOriginal      string          `json:"uti_original"`
	AlbumInfo        []queryRefAlbum `json:"album_info"`
	Latitude         *float64        `json:"latitude"`
	Longitude        *float64        `json:"longitude"`
}

type queryRefAlbum struct {
	Title       string   `json:"title"`
	FolderNames []string `json:"folder_names"`
	Owner       any      `json:"owner"`
}

func (a queryRefAlbum) path() []string { return append(slices.Clone(a.FolderNames), a.Title) }

// Shared albums are left out: the module does not read them (rule 8 of
// NORMALIZATION.md).
func (r queryRef) userAlbums() []queryRefAlbum {
	var out []queryRefAlbum
	for _, a := range r.AlbumInfo {
		if a.Owner == nil {
			out = append(out, a)
		}
	}
	return out
}

func queryRefs(t *testing.T, objs []golden.Object) []queryRef {
	t.Helper()
	out := make([]queryRef, len(objs))
	for i, obj := range objs {
		data, err := json.Marshal(obj)
		if err == nil {
			err = json.Unmarshal(data, &out[i])
		}
		if err != nil {
			t.Fatalf("reference element %d: %v", i+1, err)
		}
	}
	return out
}

// The objects that divergences.json covers are left out on both sides.
func querySkip(divs []golden.Divergence, library string, keys []string) func(string) bool {
	all, some := false, map[string]bool{}
	for _, d := range divs {
		if d.Library != library || (d.Field != "uuid" && !slices.Contains(keys, d.Field)) {
			continue
		}
		if d.UUID == "*" {
			all = true
		}
		some[d.UUID] = true
	}
	return func(uuid string) bool { return all || some[uuid] }
}

func queryIDs(photos []*photoinfo.PhotoInfo, skip func(string) bool) []string {
	var out []string
	for _, p := range photos {
		if skip == nil || !skip(p.UUID) {
			out = append(out, p.UUID)
		}
	}
	return out
}

func queryRefIDs(refs []queryRef, skip func(string) bool, keep func(queryRef) bool) []string {
	var out []string
	for _, r := range refs {
		if !skip(r.UUID) && keep(r) {
			out = append(out, r.UUID)
		}
	}
	return out
}

// The report holds UUIDs and numbers only.
func queryCompare(t *testing.T, name string, got, want []string) {
	t.Helper()
	if slices.Equal(got, want) {
		return
	}
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	g, w := "<end>", "<end>"
	if i < len(got) {
		g = got[i]
	}
	if i < len(want) {
		w = want[i]
	}
	t.Errorf("%s: %d photos, want %d; first difference at %d: got %s, want %s", name, len(got), len(want), i, g, w)
}

type queryCase struct {
	name, family string
	refKeys      []string
	o            query.Options
	want         func(queryRef) bool
}

type queryFlag struct {
	name      string
	refKeys   []string
	x, notX   func(*query.Options)
	has       func(queryRef) bool
	appliesTo func(queryRef) bool
}

func queryFlagCases() []queryCase {
	flags := []queryFlag{
		{"edited", []string{"hasadjustments"}, func(o *query.Options) { o.Edited = true }, func(o *query.Options) { o.NotEdited = true },
			func(r queryRef) bool { return r.HasAdjustments }, nil},
		{"favorite", []string{"favorite"}, func(o *query.Options) { o.Favorite = true }, func(o *query.Options) { o.NotFavorite = true },
			func(r queryRef) bool { return r.Favorite }, nil},
		{"hidden", []string{"hidden"}, func(o *query.Options) { o.Hidden = true }, func(o *query.Options) { o.NotHidden = true },
			func(r queryRef) bool { return r.Hidden }, nil},
		{"shared", []string{"shared"}, func(o *query.Options) { o.Shared = true }, func(o *query.Options) { o.NotShared = true },
			func(r queryRef) bool { return r.Shared }, nil},
		{"burst", []string{"burst"}, func(o *query.Options) { o.Burst = true }, func(o *query.Options) { o.NotBurst = true },
			func(r queryRef) bool { return r.Burst }, nil},
		{"live", []string{"live_photo"}, func(o *query.Options) { o.Live = true }, func(o *query.Options) { o.NotLive = true },
			func(r queryRef) bool { return r.Live }, nil},
		{"screenshot", []string{"screenshot"}, func(o *query.Options) { o.Screenshot = true }, func(o *query.Options) { o.NotScreenshot = true },
			func(r queryRef) bool { return r.Screenshot }, nil},
		{"cloud asset", []string{"iscloudasset"}, func(o *query.Options) { o.CloudAsset = true }, func(o *query.Options) { o.NotCloudAsset = true },
			func(r queryRef) bool { return r.CloudAsset }, nil},
		{"in cloud", []string{"incloud", "shared"}, func(o *query.Options) { o.InCloud = true }, func(o *query.Options) { o.NotInCloud = true },
			func(r queryRef) bool { return r.InCloud != nil && *r.InCloud }, func(r queryRef) bool { return !r.Shared }},
		{"has raw", []string{"has_raw"}, func(o *query.Options) { o.HasRAW = true }, nil,
			func(r queryRef) bool { return r.HasRAW }, nil},
		{"reference", []string{"isreference"}, func(o *query.Options) { o.IsReference = true }, func(o *query.Options) { o.NotReference = true },
			func(r queryRef) bool { return r.IsReference }, nil},
		{"location", []string{"latitude", "longitude"}, func(o *query.Options) { o.Location = true }, func(o *query.Options) { o.NoLocation = true },
			func(r queryRef) bool { return r.Latitude != nil || r.Longitude != nil }, nil},
		{"syndicated", []string{"syndicated"}, func(o *query.Options) { o.Syndicated = true }, func(o *query.Options) { o.NotSyndicated = true },
			func(r queryRef) bool { return r.Syndicated }, nil},
		{"saved to library", []string{"syndicated", "saved_to_library"},
			func(o *query.Options) { o.SavedToLibrary = true }, func(o *query.Options) { o.NotSavedToLibrary = true },
			func(r queryRef) bool { return r.SavedToLibrary }, func(r queryRef) bool { return r.Syndicated }},
		{"shared library", []string{"shared_library"}, func(o *query.Options) { o.SharedLibrary = true }, func(o *query.Options) { o.NotSharedLibrary = true },
			func(r queryRef) bool { return r.SharedLibrary }, nil},
		{"external edit", []string{"external_edit"}, func(o *query.Options) { o.ExternalEdit = true }, nil,
			func(r queryRef) bool { return r.ExternalEdit }, nil},
		{"portrait", []string{"portrait"}, func(o *query.Options) { o.Portrait = true }, func(o *query.Options) { o.NotPortrait = true },
			func(r queryRef) bool { return r.Portrait }, nil},
		{"spatial", []string{"spatial"}, func(o *query.Options) { o.Spatial = true }, func(o *query.Options) { o.NotSpatial = true },
			func(r queryRef) bool { return r.Spatial != 0 }, nil},
		{"screen recording", []string{"screen_recording"}, func(o *query.Options) { o.ScreenRecording = true },
			func(o *query.Options) { o.NotScreenRecording = true }, func(r queryRef) bool { return r.ScreenRecording }, nil},
		{"slow mo", []string{"slow_mo"}, func(o *query.Options) { o.SlowMo = true }, func(o *query.Options) { o.NotSlowMo = true },
			func(r queryRef) bool { return r.SlowMo }, nil},
		{"time lapse", []string{"time_lapse"}, func(o *query.Options) { o.TimeLapse = true }, func(o *query.Options) { o.NotTimeLapse = true },
			func(r queryRef) bool { return r.TimeLapse }, nil},
		{"hdr", []string{"hdr"}, func(o *query.Options) { o.HDR = true }, func(o *query.Options) { o.NotHDR = true },
			func(r queryRef) bool { return r.HDR }, nil},
		{"selfie", []string{"selfie"}, func(o *query.Options) { o.Selfie = true }, func(o *query.Options) { o.NotSelfie = true },
			func(r queryRef) bool { return r.Selfie }, nil},
		{"panorama", []string{"panorama"}, func(o *query.Options) { o.Panorama = true }, func(o *query.Options) { o.NotPanorama = true },
			func(r queryRef) bool { return r.Panorama }, nil},
		{"shared moment", []string{"shared_moment"}, func(o *query.Options) { o.SharedMoment = true }, func(o *query.Options) { o.NotSharedMoment = true },
			func(r queryRef) bool { return r.SharedMoment }, nil},
	}
	var cases []queryCase
	for _, f := range flags {
		in := func(r queryRef) bool { return f.appliesTo == nil || f.appliesTo(r) }
		var x, notX query.Options
		f.x(&x)
		cases = append(cases, queryCase{f.name, "flags", f.refKeys, x, func(r queryRef) bool { return in(r) && f.has(r) }})
		if f.notX != nil {
			f.notX(&notX)
			cases = append(cases, queryCase{"not " + f.name, "flags", f.refKeys, notX, func(r queryRef) bool { return in(r) && !f.has(r) }})
		}
	}
	return cases
}

func queryValues(refs []queryRef, values func(queryRef) []string) []string {
	set := map[string]bool{}
	for _, r := range refs {
		for _, v := range values(r) {
			set[v] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}

func queryTextCases(refs []queryRef) []queryCase {
	var cases []queryCase
	keywords := queryValues(refs, func(r queryRef) []string { return r.Keywords })
	for i, k := range keywords {
		cases = append(cases, queryCase{fmt.Sprintf("keyword %d", i), "keywords", []string{"keywords"}, query.Options{Keyword: []string{k}},
			func(r queryRef) bool { return slices.Contains(r.Keywords, norm.NFC.String(k)) }})
		upper := strings.ToUpper(k)
		if upper == k { // a caseless value cannot catch an ignored IgnoreCase
			continue
		}
		folded := strings.ToLower(norm.NFC.String(upper))
		cases = append(cases, queryCase{fmt.Sprintf("keyword %d in upper case, ignore case", i), "keywords", []string{"keywords"},
			query.Options{Keyword: []string{upper}, IgnoreCase: true},
			func(r queryRef) bool {
				return slices.ContainsFunc(r.Keywords, func(x string) bool { return strings.ToLower(x) == folded })
			}})
	}
	cases = append(cases, queryCase{"no keyword", "keywords", []string{"keywords"}, query.Options{NoKeyword: true},
		func(r queryRef) bool { return len(r.Keywords) == 0 }})

	for _, f := range []struct {
		name, key string
		field     func(queryRef) string
		option    func(values []string, none bool) query.Options
	}{
		{"title", "title", func(r queryRef) string { return r.Title },
			func(v []string, none bool) query.Options { return query.Options{Title: v, NoTitle: none} }},
		{"description", "description", func(r queryRef) string { return r.Description },
			func(v []string, none bool) query.Options { return query.Options{Description: v, NoDescription: none} }},
	} {
		values := queryValues(refs, func(r queryRef) []string {
			if f.field(r) == "" {
				return nil
			}
			return []string{f.field(r)}
		})
		for i, v := range values {
			cases = append(cases, queryCase{fmt.Sprintf("%s %d", f.name, i), "texts", []string{f.key}, f.option([]string{v}, false),
				func(r queryRef) bool { return f.field(r) != "" && strings.Contains(f.field(r), norm.NFC.String(v)) }})
			upper := strings.ToUpper(v)
			if upper == v { // a caseless value cannot catch an ignored IgnoreCase
				continue
			}
			folded := strings.ToLower(norm.NFC.String(upper))
			ignoreCase := f.option([]string{upper}, false)
			ignoreCase.IgnoreCase = true
			cases = append(cases, queryCase{fmt.Sprintf("%s %d in upper case, ignore case", f.name, i), "texts", []string{f.key}, ignoreCase,
				func(r queryRef) bool {
					return f.field(r) != "" && strings.Contains(strings.ToLower(f.field(r)), folded)
				}})
		}
		cases = append(cases, queryCase{"no " + f.name, "texts", []string{f.key}, f.option(nil, true),
			func(r queryRef) bool { return f.field(r) == "" }})
	}

	bases := queryValues(refs, func(r queryRef) []string {
		base := r.OriginalFilename
		if i := strings.LastIndex(base, "."); i >= 0 {
			base = base[:i]
		}
		return []string{base}
	})
	for i, b := range bases {
		cases = append(cases, queryCase{fmt.Sprintf("name %d", i), "texts", []string{"original_filename"}, query.Options{Name: []string{b}},
			func(r queryRef) bool { return strings.Contains(r.OriginalFilename, norm.NFC.String(b)) }})
	}
	utis := queryValues(refs, func(r queryRef) []string { return []string{r.UTIOriginal} })
	for i, u := range utis {
		cases = append(cases, queryCase{fmt.Sprintf("uti %d", i), "texts", []string{"uti_original"}, query.Options{UTI: u},
			func(r queryRef) bool { return strings.Contains(r.UTIOriginal, u) }})
	}
	return cases
}

// Upstream writes "/" of a title as "//" (photoquery.py:927-941). ok is false
// for a part that starts or ends with "/": its path is ambiguous upstream.
func queryPath(parts []string) (path string, ok bool) {
	escaped := make([]string, len(parts))
	for i, p := range parts {
		if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
			return "", false
		}
		escaped[i] = strings.ReplaceAll(p, "/", "//")
	}
	return strings.Join(escaped, "/"), true
}

func queryNFC(parts []string) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = norm.NFC.String(p)
	}
	return out
}

func queryAlbumCases(refs []queryRef) []queryCase {
	albums, folders := map[string][]string{}, map[string][]string{}
	for _, r := range refs {
		for _, a := range r.userAlbums() {
			if path, ok := queryPath(a.path()); ok {
				albums[path] = a.path()
			}
			for n := 1; n <= len(a.FolderNames); n++ {
				if path, ok := queryPath(a.FolderNames[:n]); ok {
					folders[path] = a.FolderNames[:n]
				}
			}
		}
	}
	keys := []string{"album_info"}
	var cases []queryCase
	for i, path := range slices.Sorted(maps.Keys(albums)) {
		parts := queryNFC(albums[path])
		cases = append(cases, queryCase{fmt.Sprintf("album %d", i), "albums", keys, query.Options{Album: []string{path}},
			func(r queryRef) bool {
				return slices.ContainsFunc(r.userAlbums(), func(a queryRefAlbum) bool { return slices.Equal(a.path(), parts) })
			}})
		if strings.ToUpper(path) == path { // a caseless value cannot catch an ignored IgnoreCase
			continue
		}
		folded := make([]string, len(parts))
		for j, p := range albums[path] {
			folded[j] = strings.ToLower(norm.NFC.String(strings.ToUpper(p)))
		}
		cases = append(cases, queryCase{fmt.Sprintf("album %d in upper case, ignore case", i), "albums", keys,
			query.Options{Album: []string{strings.ToUpper(path)}, IgnoreCase: true},
			func(r queryRef) bool {
				return slices.ContainsFunc(r.userAlbums(), func(a queryRefAlbum) bool {
					return slices.EqualFunc(a.path(), folded, func(title, want string) bool { return strings.ToLower(title) == want })
				})
			}})
	}
	for i, path := range slices.Sorted(maps.Keys(folders)) {
		parts := queryNFC(folders[path])
		cases = append(cases, queryCase{fmt.Sprintf("folder %d", i), "folders", keys, query.Options{Folder: []string{path}},
			func(r queryRef) bool {
				return slices.ContainsFunc(r.userAlbums(), func(a queryRefAlbum) bool {
					return len(a.FolderNames) >= len(parts) && slices.Equal(a.FolderNames[:len(parts)], parts)
				})
			}})
	}
	return append(cases,
		queryCase{"in album", "albums", keys, query.Options{InAlbum: true}, func(r queryRef) bool { return len(r.userAlbums()) > 0 }},
		queryCase{"not in album", "albums", keys, query.Options{NotInAlbum: true}, func(r queryRef) bool { return len(r.userAlbums()) == 0 }},
	)
}

// TestGolden checks that the Date of each photo, the threshold here, equals
// the date of the reference.
func queryDateCases(t *testing.T, db *DB, refs []queryRef) []queryCase {
	t.Helper()
	date := map[string]time.Time{}
	for _, r := range refs {
		p, ok := db.Photo(r.UUID)
		if !ok {
			t.Fatalf("reference object %s is not a photo of the library", r.UUID)
		}
		date[r.UUID] = p.Date
	}
	var cases []queryCase
	for i, r := range refs {
		d := date[r.UUID]
		if d.IsZero() { // the zero time leaves the option unset
			continue
		}
		cases = append(cases,
			queryCase{fmt.Sprintf("from date %d", i), "dates", []string{"date"}, query.Options{FromDate: d},
				func(x queryRef) bool { return !date[x.UUID].Before(d) }},
			queryCase{fmt.Sprintf("to date %d", i), "dates", []string{"date"}, query.Options{ToDate: d},
				func(x queryRef) bool { return date[x.UUID].Before(d) }})
	}
	return cases
}

func queryOtherCases(inQuery, inDeleted []queryRef) []queryCase {
	every2 := map[string]bool{}
	var uuids []string
	for i, r := range inQuery {
		if i%2 == 0 {
			every2[r.UUID] = true
			uuids = append(uuids, r.UUID)
		}
	}
	if len(inDeleted) > 0 {
		uuids = append(uuids, inDeleted[0].UUID)
	}
	uuids = append(uuids, "00000000-0000-0000-0000-000000000000")
	return []queryCase{
		{"only photos", "kinds", []string{"isphoto"}, query.Options{OnlyPhotos: true}, func(r queryRef) bool { return r.IsPhoto }},
		{"only movies", "kinds", []string{"ismovie"}, query.Options{OnlyMovies: true}, func(r queryRef) bool { return r.IsMovie }},
		{"uuid", "uuid", nil, query.Options{UUID: uuids}, func(r queryRef) bool { return every2[r.UUID] }},
	}
}

func TestQueryGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	divs, err := golden.LoadDivergences(filepath.Join(dir, "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	libs := goldenLibraries(t)
	var mu sync.Mutex
	expected, exercised := map[string]int{}, map[string]bool{}
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libs {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ref, err := golden.Load(dir, lib.name)
				if err != nil {
					t.Fatal(err)
				}
				inQuery, inDeleted := queryRefs(t, ref.Query), queryRefs(t, ref.Deleted)
				querySelection(t, db, inQuery, inDeleted, querySkip(divs, lib.name, nil))

				cases := slices.Concat(queryFlagCases(), queryTextCases(inQuery), queryAlbumCases(inQuery),
					queryDateCases(t, db, inQuery), queryOtherCases(inQuery, inDeleted))
				counts, hit := map[string]int{}, map[string]bool{}
				for _, c := range cases {
					skip := querySkip(divs, lib.name, c.refKeys)
					want := queryRefIDs(inQuery, skip, c.want)
					queryCompare(t, c.name, queryIDs(db.Query(c.o), skip), want)
					counts[c.family] += len(want)
					if c.family == "flags" && len(want) > 0 && len(want) < len(queryRefIDs(inQuery, skip, func(queryRef) bool { return true })) {
						hit[c.name] = true
					}
				}
				t.Logf("%d cases; expected photos per family: %v", len(cases), counts)
				mu.Lock()
				defer mu.Unlock()
				for f, n := range counts {
					expected[f] += n
				}
				maps.Copy(exercised, hit)
			})
		}
	})
	for _, f := range []string{"flags", "keywords", "texts", "albums", "folders", "dates"} {
		if expected[f] == 0 {
			t.Errorf("family %s: no case of any library expects a photo; the test is vacuous", f)
		}
	}
	t.Logf("expected photos per family, all libraries: %v", expected)

	// A flag case counts only when some library expects a proper subset of its
	// objects: a case that expects none or all of them passes a mutation that
	// ignores the flag. The committed libraries hold no syndicated photo, and
	// these cloud cases select all or none of the objects of each library, so
	// only these cases may stay unexercised.
	never := []string{
		"syndicated", "not syndicated", "saved to library", "not saved to library",
		"cloud asset", "not cloud asset", "not in cloud",
	}
	flagCases := queryFlagCases()
	for _, name := range never {
		if !slices.ContainsFunc(flagCases, func(c queryCase) bool { return c.name == name }) {
			t.Errorf("flag %q in never is not a flag case", name)
		}
	}
	for _, c := range flagCases {
		switch {
		case slices.Contains(never, c.name) && exercised[c.name]:
			t.Errorf("flag %q is exercised: remove it from never", c.name)
		case slices.Contains(never, c.name) || exercised[c.name]:
		default:
			t.Errorf("flag %q: no library expects a proper subset of its objects; the case is vacuous", c.name)
		}
	}
}

func querySelection(t *testing.T, db *DB, inQuery, inDeleted []queryRef, skip func(string) bool) {
	t.Helper()
	all := func(queryRef) bool { return true }
	outsideIDs := queryRefIDs(inQuery, skip, all)
	deletedIDs := queryRefIDs(inDeleted, skip, all)
	var both []*photoinfo.PhotoInfo
	for _, r := range slices.Concat(inQuery, inDeleted) {
		p, ok := db.Photo(r.UUID)
		if !ok {
			t.Fatalf("reference object %s is not a photo of the library", r.UUID)
		}
		both = append(both, p)
	}
	newest := slices.Clone(outsideIDs)
	slices.Reverse(newest)
	for _, c := range []struct {
		name string
		o    query.Options
		want []string
	}{
		{"no options", query.Options{}, outsideIDs},
		{"deleted only", query.Options{DeletedOnly: true}, deletedIDs},
		{"deleted and deleted only", query.Options{Deleted: true, DeletedOnly: true}, deletedIDs},
		{"deleted", query.Options{Deleted: true}, queryIDs(query.Sort(both), skip)},
		{"newest first", query.Options{NewestFirst: true}, newest},
	} {
		queryCompare(t, c.name, queryIDs(db.Query(c.o), skip), c.want)
	}
	if len(outsideIDs) == 0 {
		t.Error("query.json has no photos: the test is vacuous")
	}
}

// TestQueryGolden leaves out Missing because no test library has its original
// files, so every photo is missing by Path.
func TestQueryMissing(t *testing.T) {
	t.Parallel()

	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			all := queryIDs(db.Query(query.Options{}), nil)
			missing := queryIDs(db.Query(query.Options{Missing: true}), nil)
			notMissing := queryIDs(db.Query(query.Options{NotMissing: true}), nil)
			t.Logf("%d photos, %d missing, %d not missing", len(all), len(missing), len(notMissing))
			if !slices.Equal(missing, all) || len(notMissing) != 0 {
				t.Error("a test library has original files: Missing needs a test of its own")
			}
		})
	}
}

// The groups of the signature of photosdb/photosdb.py:3411-3420 over the rows
// of the main query of upstream, which joins ZADDITIONALASSETATTRIBUTES
// (photosdb/photosdb.py:2086-2090, 2358-2363).
func queryFlagsSignatureGroups(t *testing.T, lib, schemaID string) map[string][]string {
	t.Helper()
	i := slices.IndexFunc(schema.All(), func(s schema.Schema) bool { return s.ID == schemaID })
	if i < 0 {
		t.Fatalf("schema %s is not in schema.All", schemaID)
	}
	adjustments := schema.All()[i].AdjustmentsColumn
	rows, err := roDB(t, lib).QueryContext(t.Context(), "SELECT group_concat(a.ZUUID, ' ') FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK GROUP BY x.ZORIGINALFILESIZE, a.ZDATECREATED, a.ZHEIGHT, a.ZWIDTH, a.ZUNIFORMTYPEIDENTIFIER, a."+adjustments+" HAVING count(*) > 1") //nolint:gosec // G202: the column name is a constant of internal/schema
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string][]string{}
	for rows.Next() {
		var members string
		if err := rows.Scan(&members); err != nil {
			t.Fatal(err)
		}
		group := slices.Sorted(slices.Values(strings.Fields(members)))
		for _, u := range group {
			groups[u] = group
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	return groups
}

func TestQueryDuplicates(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	withDuplicates := 0
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				groups := queryFlagsSignatureGroups(t, lib.path, db.Info().SchemaID)
				for _, p := range db.photos {
					want := slices.DeleteFunc(slices.Clone(groups[p.UUID]), func(u string) bool { return u == p.UUID })
					if got := queryIDs(db.Duplicates(p), nil); !slices.Equal(got, want) {
						t.Errorf("Duplicates of %s = %q, want %q", p.UUID, got, want)
					}
				}
				t.Logf("%d photos with duplicates", len(groups))
				mu.Lock()
				defer mu.Unlock()
				withDuplicates += len(groups)
			})
		}
	})
	if withDuplicates == 0 {
		t.Error("no library has duplicates: the test is vacuous")
	}
}
