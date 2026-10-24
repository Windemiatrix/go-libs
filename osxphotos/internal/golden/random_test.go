package golden

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// The code of this file builds the reference of osxphotos from Go values
// without the rules of the package: it is a second, independent
// implementation of what osxphotos writes (a Python isoformat, sorted
// lists, a dictionary that keeps the last value of a title).

var (
	pickTypes   = []int{0, 0, 2, 4, 8, 12, 16, 24}
	wordPool    = []string{"alpha", "Beta", "gamma", "δelta", "日本", "emoji \U0001F600", "tilde ～", "Zulu", "a b", "x\"y", "<&>"}
	titlePool   = []string{"Holidays", "Family", "Work", "Дача", "Album \U0001F600"}
	folderPool  = []string{"2023", "2024", "Trips"}
	refOffsets  = []int{0, 3600, -25200, 19800, 20700, 5608, -34200}
	randomStart = time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC)
)

// pyTime writes a moment as Python datetime.isoformat does in the given offset.
func pyTime(t time.Time, offset int) string {
	l := t.In(time.FixedZone("", offset))
	s := l.Format("2006-01-02T15:04:05")
	if us := l.Nanosecond() / 1000; us != 0 {
		s += fmt.Sprintf(".%06d", us)
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	s += fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
	if offset%60 != 0 {
		s += fmt.Sprintf(":%02d", offset%60)
	}
	return s
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonStrings(list []string) string {
	b, _ := json.Marshal(list)
	return string(b)
}

// pyFloat writes a float as Python repr does (shortest digits).
func pyFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func randomTime(r *rand.Rand) time.Time {
	sec := r.Int64N(35 * 365 * 86400)
	return randomStart.Add(time.Duration(sec)*time.Second + time.Duration(r.IntN(1_000_000))*time.Microsecond)
}

// randomLibrary builds n photos and their albums. Every UUID is synthetic.
func randomLibrary(r *rand.Rand, n int) Library {
	var lib Library
	for i := 0; i < 14; i++ {
		a := &albuminfo.AlbumInfo{
			UUID:    fmt.Sprintf("00000000-0000-0000-0001-%012x", i+1),
			Title:   titlePool[r.IntN(len(titlePool))],
			InTrash: r.IntN(5) == 0,
		}
		if i == 0 {
			a.Title = "" // an album without a title
		}
		if r.IntN(2) == 0 {
			a.ParentUUID = fmt.Sprintf("00000000-0000-0000-0002-%012x", r.IntN(3))
			a.FolderNames = []string{folderPool[r.IntN(len(folderPool))]}
			if r.IntN(2) == 0 {
				a.FolderNames = append(a.FolderNames, "Sub "+folderPool[r.IntN(len(folderPool))])
			}
		}
		for _, d := range []**time.Time{&a.CreationDate, &a.StartDate, &a.EndDate} {
			if r.IntN(3) != 0 {
				*d = ptr(randomTime(r))
			}
		}
		lib.Albums = append(lib.Albums, a)
	}
	albumList := func() []string {
		var out []string
		for k := r.IntN(5); k > 0; k-- {
			out = append(out, lib.Albums[r.IntN(len(lib.Albums))].UUID)
		}
		return out
	}

	for i := 0; i < n; i++ {
		p := &photoinfo.PhotoInfo{
			UUID:             fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1),
			Filename:         fmt.Sprintf("IMG_%04d.HEIC", i),
			OriginalFilename: wordPool[r.IntN(len(wordPool))] + ".jpg",
			UTI:              "public.heic", UTIOriginal: []string{"public.jpeg", "public.heic"}[r.IntN(2)],
			Date:  randomTime(r),
			Width: r.IntN(5000), Height: r.IntN(5000), OriginalWidth: r.IntN(5000), OriginalHeight: r.IntN(5000),
			Orientation: r.IntN(9), OriginalOrientation: r.IntN(9), OriginalFilesize: r.Int64N(1 << 40),
			AlbumUUIDs: albumList(), BurstAlbumUUIDs: albumList(),
		}
		switch x := r.IntN(100); {
		case x < 90:
			p.IsPhoto = true
		case x < 97:
			p.IsMovie = true
		}
		for _, f := range []*bool{&p.Live, &p.HasRAW, &p.IsRAW, &p.RawOriginal, &p.Screenshot, &p.Hidden, &p.Favorite,
			&p.HasAdjustments, &p.Shared, &p.Syndicated, &p.SavedToLibrary, &p.SharedLibrary, &p.IsMissing, &p.IsCloudAsset, &p.IsReference} {
			*f = r.IntN(4) == 0
		}
		p.InTrash = r.IntN(8) == 0
		if p.HasAdjustments {
			p.Orientation = 0 // what osxphotos gives; the rule does not compare it anyway
		}
		if r.IntN(5) == 0 {
			p.Burst = true
			p.BurstUUID = fmt.Sprintf("BURST-%d", r.IntN(25))
			if r.IntN(10) == 0 {
				p.BurstUUID = ""
			}
			p.BurstPickType = pickTypes[r.IntN(len(pickTypes))]
			p.BurstSelected = p.BurstPickType&8 != 0
			p.BurstKey = p.BurstPickType&16 != 0
		}
		if r.IntN(5) != 0 {
			p.TZOffset = ptr(refOffsets[r.IntN(len(refOffsets))])
		}
		if r.IntN(5) != 0 {
			p.DateAdded = ptr(randomTime(r))
		}
		if r.IntN(2) == 0 {
			p.DateModified = ptr(randomTime(r))
		}
		if p.InTrash && r.IntN(5) != 0 {
			p.DateTrashed = ptr(randomTime(r))
		}
		if r.IntN(3) == 0 {
			p.InCloud = ptr(r.IntN(2) == 0)
		}
		if r.IntN(2) == 0 {
			p.Latitude, p.Longitude = ptr(r.Float64()*180-90), ptr(r.Float64()*360-180)
			switch r.IntN(10) {
			case 0:
				p.Latitude = ptr(0.0)
			case 1:
				p.Longitude = ptr(1e-5)
			}
		}
		for _, f := range []*string{&p.CloudMasterGUID, &p.Fingerprint, &p.Title, &p.Description, &p.UTIRaw} {
			if r.IntN(2) == 0 {
				*f = wordPool[r.IntN(len(wordPool))]
			}
		}
		for k := r.IntN(4); k > 0; k-- {
			p.Keywords = append(p.Keywords, wordPool[r.IntN(len(wordPool))])
		}
		sort.Strings(p.Keywords) // byte order, the same as code point order
		p.HasPersons = r.IntN(3) == 0
		lib.Photos = append(lib.Photos, p)
	}
	return lib
}

// pickedUp reports whether upstream photos() returns the photo: written
// again here, from the bits of the pick type.
func pickedUp(p *photoinfo.PhotoInfo) bool {
	unselectedFrame := p.Burst && p.BurstPickType != 0 && p.BurstPickType&8 == 0 && p.BurstPickType&16 == 0
	return !unselectedFrame && (p.IsPhoto || p.IsMovie)
}

// encodeAlbum writes an element of album_info.
func encodeAlbum(a *albuminfo.AlbumInfo) string {
	date := func(t *time.Time, dflt string) string {
		if t == nil {
			return dflt
		}
		return jsonString(pyTime(*t, 0))
	}
	title := "null"
	if a.Title != "" {
		title = jsonString(a.Title)
	}
	parent := "null"
	if a.ParentUUID != "" {
		parent = jsonString(a.ParentUUID)
	}
	return fmt.Sprintf(`{"creation_date": %s, "end_date": %s, "folder_list": [], "folder_names": %s, "owner": null, "parent": %s, "sort_order": null, "start_date": %s, "title": %s, "uuid": %s}`,
		date(a.CreationDate, `"2001-01-01T00:00:00+00:00"`), date(a.EndDate, "null"), jsonStrings(append([]string{}, a.FolderNames...)),
		parent, date(a.StartDate, "null"), title, jsonString(a.UUID))
}

// encodeShared writes a shared album with a title of its own.
func encodeShared(n int) (elem, title string) {
	title = fmt.Sprintf("Shared %d", n)
	return fmt.Sprintf(`{"creation_date": "2020-03-03T03:03:03+00:00", "end_date": null, "folder_list": [], "folder_names": ["Shared Folder"], "owner": "Owner A", "parent": null, "sort_order": null, "start_date": null, "title": %s, "uuid": "00000000-0000-0000-0003-%012x"}`,
		jsonString(title), n), title
}

// encode writes the reference object of a photo, as osxphotos would. shared
// is the number of shared albums to add to its album lists, 0 for none.
func encode(r *rand.Rand, lib Library, p *photoinfo.PhotoInfo, shared int) map[string]string {
	byUUID := map[string]*albuminfo.AlbumInfo{}
	for _, a := range lib.Albums {
		byUUID[a.UUID] = a
	}
	albumsOf := func(uuids []string, extra int) (info string, titles string, folders string) {
		var list []*albuminfo.AlbumInfo
		seen := map[string]bool{}
		for _, id := range uuids {
			if a := byUUID[id]; !a.InTrash && !seen[id] {
				seen[id] = true
				list = append(list, a)
			}
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].Title < list[j].Title })
		var elems []string
		set := map[string]bool{}
		dict := map[string][]string{}
		for _, a := range list {
			elems = append(elems, encodeAlbum(a))
			set[a.Title] = true
			dict[a.Title] = append([]string{}, a.FolderNames...)
		}
		for k := 0; k < extra; k++ {
			e, title := encodeShared(k + 1)
			elems = append(elems, e)
			set[title] = true
			dict[title] = []string{"Shared Folder"}
		}
		var names []string
		for t := range set {
			names = append(names, t)
		}
		sort.Strings(names)
		var parts []string
		for t, v := range dict {
			key := t
			if t == "" {
				key = "null"
			}
			parts = append(parts, jsonString(key)+": "+jsonStrings(v))
		}
		sort.Strings(parts)
		// A title that is null is written as null in the list of titles.
		titleJSON := make([]string, len(names))
		for i, n := range names {
			titleJSON[i] = jsonString(n)
			if n == "" {
				titleJSON[i] = "null"
			}
		}
		return "[" + strings.Join(elems, ", ") + "]", "[" + strings.Join(titleJSON, ", ") + "]", "{" + strings.Join(parts, ", ") + "}"
	}

	m := map[string]string{}
	m["album_info"], m["albums"], m["folders"] = albumsOf(p.AlbumUUIDs, shared)
	m["burst_album_info"], m["burst_albums"], _ = albumsOf(p.BurstAlbumUUIDs, shared)

	var others []string
	if p.Burst {
		for _, q := range lib.Photos {
			if q.Burst && q.BurstUUID == p.BurstUUID && q.UUID != p.UUID {
				others = append(others, q.UUID)
			}
		}
	}
	sort.Strings(others)
	m["burst_photos"] = jsonStrings(append([]string{}, others...))

	b := func(v bool) string { return strconv.FormatBool(v) }
	str := func(s string) string {
		if s == "" && r.IntN(2) == 0 {
			return "null"
		}
		return jsonString(s)
	}
	num := func(n int64) string {
		if n == 0 && r.IntN(2) == 0 {
			return "null"
		}
		return strconv.FormatInt(n, 10)
	}
	date := func(t *time.Time, dflt string) string {
		if t == nil {
			return dflt
		}
		return jsonString(pyTime(*t, 0))
	}
	pick := p.BurstPickType
	m["burst"], m["burst_key"], m["burst_selected"] = b(p.Burst), b(pick&16 != 0), b(pick&8 != 0)
	m["burst_default_pick"] = b(pick&4 != 0)
	if !p.Burst { // the Go fields BurstKey and BurstSelected follow the pick type only for frames
		m["burst_key"], m["burst_selected"] = b(p.BurstKey), b(p.BurstSelected)
	}
	m["cloud_guid"], m["description"], m["fingerprint"], m["title"], m["uti_raw"] =
		str(p.CloudMasterGUID), str(p.Description), str(p.Fingerprint), str(p.Title), str(p.UTIRaw)
	m["date"] = jsonString(pyTime(p.Date, refOffsets[r.IntN(len(refOffsets))]))
	m["date_added"] = date(p.DateAdded, `"1970-01-01T00:00:00+00:00"`)
	m["date_modified"], m["date_trashed"] = date(p.DateModified, "null"), date(p.DateTrashed, "null")
	m["favorite"], m["hidden"], m["intrash"] = b(p.Favorite), b(p.Hidden), b(p.InTrash)
	m["isphoto"], m["ismovie"], m["live_photo"], m["screenshot"] = b(p.IsPhoto), b(p.IsMovie), b(p.Live), b(p.Screenshot)
	m["isreference"], m["hasadjustments"], m["has_raw"], m["raw_original"] = b(p.IsReference), b(p.HasAdjustments), b(p.HasRAW), b(p.RawOriginal)
	m["ismissing"], m["iscloudasset"], m["israw"] = b(p.IsMissing), b(p.IsCloudAsset), b(p.IsRAW)
	m["shared"], m["shared_library"], m["syndicated"], m["saved_to_library"] = b(p.Shared), b(p.SharedLibrary), b(p.Syndicated), b(p.SavedToLibrary)
	m["filename"], m["original_filename"], m["uti"], m["uti_original"] = jsonString(p.Filename), jsonString(p.OriginalFilename), jsonString(p.UTI), jsonString(p.UTIOriginal)
	m["width"], m["height"] = num(int64(p.Width)), num(int64(p.Height))
	m["original_width"], m["original_height"] = num(int64(p.OriginalWidth)), num(int64(p.OriginalHeight))
	m["original_orientation"], m["original_filesize"] = num(int64(p.OriginalOrientation)), num(p.OriginalFilesize)
	m["orientation"] = num(int64(p.Orientation))
	if p.HasAdjustments {
		m["orientation"] = "0" // upstream reads a plist that the libraries do not have
	}
	m["incloud"] = "null"
	if p.InCloud != nil {
		m["incloud"] = b(*p.InCloud)
	}
	m["keywords"] = jsonStrings(append([]string{}, p.Keywords...))
	lat, lon := "null", "null"
	if p.Latitude != nil {
		lat = pyFloat(*p.Latitude)
	}
	if p.Longitude != nil {
		lon = pyFloat(*p.Longitude)
	}
	m["latitude"], m["longitude"], m["location"] = lat, lon, "["+lat+", "+lon+"]"
	switch {
	case p.HasPersons:
		m["persons"] = jsonStrings([]string{"Person A", "_UNKNOWN_"})
	case r.IntN(2) == 0:
		m["persons"] = jsonStrings([]string{"_UNKNOWN_"})
	default:
		m["persons"] = "[]"
	}
	m["tzoffset"] = "null"
	if p.TZOffset != nil {
		m["tzoffset"] = strconv.Itoa(*p.TZOffset)
	}
	m["uuid"] = jsonString(p.UUID)
	return m
}

// randomReference builds the reference of lib: the photos upstream returns,
// in query.json or deleted.json by the trash flag, in the order of
// osxphotos.
func randomReference(r *rand.Rand, lib Library) *Reference {
	type entry struct {
		p   *photoinfo.PhotoInfo
		obj Object
	}
	var query, deleted []entry
	for _, p := range lib.Photos {
		if !pickedUp(p) {
			continue
		}
		shared := 0
		if r.IntN(8) == 0 {
			shared = 1 + r.IntN(2)
		}
		e := entry{p, refObj(encode(r, lib, p, shared))}
		if p.InTrash {
			deleted = append(deleted, e)
		} else {
			query = append(query, e)
		}
	}
	less := func(es []entry) func(i, j int) bool {
		return func(i, j int) bool {
			a, b := es[i].p, es[j].p
			if !a.Date.Equal(b.Date) {
				return a.Date.Before(b.Date)
			}
			ad, bd := time.Unix(0, 0), time.Unix(0, 0)
			if a.DateAdded != nil {
				ad = *a.DateAdded
			}
			if b.DateAdded != nil {
				bd = *b.DateAdded
			}
			if !ad.Equal(bd) {
				return ad.Before(bd)
			}
			return a.UUID < b.UUID
		}
	}
	sort.Slice(query, less(query))
	sort.Slice(deleted, less(deleted))
	ref := &Reference{Library: "random"}
	for _, e := range query {
		ref.Query = append(ref.Query, e.obj)
	}
	for _, e := range deleted {
		ref.Deleted = append(ref.Deleted, e.obj)
	}
	return ref
}

// perturbations change one place of a photo; want is the keys that must then
// differ. ok tells whether the photo suits the perturbation.
type perturbation struct {
	name string
	want []string
	ok   func(p *photoinfo.PhotoInfo) bool
	do   func(p *photoinfo.PhotoInfo)
}

func perturbations(users map[string]bool) []perturbation {
	all := func(*photoinfo.PhotoInfo) bool { return true }
	notBurst := func(p *photoinfo.PhotoInfo) bool { return !p.Burst }
	var out []perturbation
	flag := func(key string, f func(p *photoinfo.PhotoInfo) *bool) {
		out = append(out, perturbation{key, []string{key}, all, func(p *photoinfo.PhotoInfo) { *f(p) = !*f(p) }})
	}
	flag("favorite", func(p *photoinfo.PhotoInfo) *bool { return &p.Favorite })
	flag("hidden", func(p *photoinfo.PhotoInfo) *bool { return &p.Hidden })
	flag("intrash", func(p *photoinfo.PhotoInfo) *bool { return &p.InTrash })
	flag("live_photo", func(p *photoinfo.PhotoInfo) *bool { return &p.Live })
	flag("screenshot", func(p *photoinfo.PhotoInfo) *bool { return &p.Screenshot })
	flag("isreference", func(p *photoinfo.PhotoInfo) *bool { return &p.IsReference })
	flag("hasadjustments", func(p *photoinfo.PhotoInfo) *bool { return &p.HasAdjustments })
	flag("has_raw", func(p *photoinfo.PhotoInfo) *bool { return &p.HasRAW })
	flag("raw_original", func(p *photoinfo.PhotoInfo) *bool { return &p.RawOriginal })
	flag("ismissing", func(p *photoinfo.PhotoInfo) *bool { return &p.IsMissing })
	flag("iscloudasset", func(p *photoinfo.PhotoInfo) *bool { return &p.IsCloudAsset })
	flag("israw", func(p *photoinfo.PhotoInfo) *bool { return &p.IsRAW })
	flag("shared", func(p *photoinfo.PhotoInfo) *bool { return &p.Shared })
	flag("shared_library", func(p *photoinfo.PhotoInfo) *bool { return &p.SharedLibrary })
	flag("syndicated", func(p *photoinfo.PhotoInfo) *bool { return &p.Syndicated })
	flag("saved_to_library", func(p *photoinfo.PhotoInfo) *bool { return &p.SavedToLibrary })
	flag("persons", func(p *photoinfo.PhotoInfo) *bool { return &p.HasPersons })
	flag("burst_key", func(p *photoinfo.PhotoInfo) *bool { return &p.BurstKey })
	flag("burst_selected", func(p *photoinfo.PhotoInfo) *bool { return &p.BurstSelected })
	str := func(key string, f func(p *photoinfo.PhotoInfo) *string) {
		out = append(out, perturbation{key, []string{key}, all, func(p *photoinfo.PhotoInfo) { *f(p) += "x" }})
	}
	str("filename", func(p *photoinfo.PhotoInfo) *string { return &p.Filename })
	str("original_filename", func(p *photoinfo.PhotoInfo) *string { return &p.OriginalFilename })
	str("uti", func(p *photoinfo.PhotoInfo) *string { return &p.UTI })
	str("uti_original", func(p *photoinfo.PhotoInfo) *string { return &p.UTIOriginal })
	str("uti_raw", func(p *photoinfo.PhotoInfo) *string { return &p.UTIRaw })
	str("cloud_guid", func(p *photoinfo.PhotoInfo) *string { return &p.CloudMasterGUID })
	str("description", func(p *photoinfo.PhotoInfo) *string { return &p.Description })
	str("fingerprint", func(p *photoinfo.PhotoInfo) *string { return &p.Fingerprint })
	str("title", func(p *photoinfo.PhotoInfo) *string { return &p.Title })
	num := func(key string, f func(p *photoinfo.PhotoInfo) *int) {
		out = append(out, perturbation{key, []string{key}, all, func(p *photoinfo.PhotoInfo) { *f(p)++ }})
	}
	num("width", func(p *photoinfo.PhotoInfo) *int { return &p.Width })
	num("height", func(p *photoinfo.PhotoInfo) *int { return &p.Height })
	num("original_width", func(p *photoinfo.PhotoInfo) *int { return &p.OriginalWidth })
	num("original_height", func(p *photoinfo.PhotoInfo) *int { return &p.OriginalHeight })
	num("original_orientation", func(p *photoinfo.PhotoInfo) *int { return &p.OriginalOrientation })
	out = append(out,
		perturbation{"original_filesize", []string{"original_filesize"}, all, func(p *photoinfo.PhotoInfo) { p.OriginalFilesize++ }},
		perturbation{"orientation", []string{"orientation"}, func(p *photoinfo.PhotoInfo) bool { return !p.HasAdjustments }, func(p *photoinfo.PhotoInfo) { p.Orientation++ }},
		perturbation{"orientation of an adjusted photo", nil, func(p *photoinfo.PhotoInfo) bool { return p.HasAdjustments }, func(p *photoinfo.PhotoInfo) { p.Orientation++ }},
		perturbation{"date", []string{"date"}, all, func(p *photoinfo.PhotoInfo) { p.Date = p.Date.Add(time.Microsecond) }},
		perturbation{"date by a nanosecond", []string{"date"}, all, func(p *photoinfo.PhotoInfo) { p.Date = p.Date.Add(time.Nanosecond) }},
		perturbation{"date_added", []string{"date_added"}, all, func(p *photoinfo.PhotoInfo) { p.DateAdded = ptr(randomStart.Add(time.Hour)) }},
		perturbation{"date_modified", []string{"date_modified"}, all, func(p *photoinfo.PhotoInfo) {
			if p.DateModified == nil {
				p.DateModified = ptr(randomStart)
			} else {
				p.DateModified = nil
			}
		}},
		perturbation{"date_trashed", []string{"date_trashed"}, all, func(p *photoinfo.PhotoInfo) {
			if p.DateTrashed == nil {
				p.DateTrashed = ptr(randomStart)
			} else {
				p.DateTrashed = nil
			}
		}},
		perturbation{"tzoffset", []string{"tzoffset"}, all, func(p *photoinfo.PhotoInfo) {
			if p.TZOffset == nil {
				p.TZOffset = ptr(0)
			} else {
				p.TZOffset = nil
			}
		}},
		perturbation{"incloud", []string{"incloud"}, all, func(p *photoinfo.PhotoInfo) {
			if p.InCloud == nil {
				p.InCloud = ptr(false)
			} else {
				p.InCloud = nil
			}
		}},
		perturbation{"latitude", []string{"latitude", "location"}, all, func(p *photoinfo.PhotoInfo) {
			if p.Latitude == nil {
				p.Latitude = ptr(0.0)
			} else {
				p.Latitude = nil
			}
		}},
		perturbation{"longitude", []string{"location", "longitude"}, all, func(p *photoinfo.PhotoInfo) {
			if p.Longitude == nil {
				p.Longitude = ptr(0.0)
			} else {
				p.Longitude = nil
			}
		}},
		perturbation{"keywords", []string{"keywords"}, all, func(p *photoinfo.PhotoInfo) { p.Keywords = append(slices.Clone(p.Keywords), "zzz") }},
		perturbation{"burst_default_pick", []string{"burst_default_pick"}, notBurst, func(p *photoinfo.PhotoInfo) { p.BurstPickType ^= 4 }},
		perturbation{"burst", []string{"burst"}, func(p *photoinfo.PhotoInfo) bool { return !p.Burst }, func(p *photoinfo.PhotoInfo) { p.Burst = true; p.BurstUUID = "BURST-NEW" }},
		perturbation{"album_info", []string{"album_info", "albums", "folders"}, func(p *photoinfo.PhotoInfo) bool {
			return slices.ContainsFunc(p.AlbumUUIDs, func(id string) bool { return users[id] })
		}, func(p *photoinfo.PhotoInfo) { p.AlbumUUIDs = nil }},
	)
	return out
}

// TestCompareRandom: 200 random photos give no differences, and one change of
// one place gives the differences of the keys that read it.
func TestCompareRandom(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3} {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			r := rand.New(rand.NewPCG(seed, seed*7919))
			lib := randomLibrary(r, 200)
			users := map[string]bool{} // albums that are not in the trash
			for _, a := range lib.Albums {
				if !a.InTrash {
					users[a.UUID] = true
				}
			}
			ref := randomReference(r, lib)
			if err := ref.CheckOrder(); err != nil {
				t.Fatalf("the order of the random reference: %v", err)
			}
			if len(ref.Query) < 100 || len(ref.Deleted) == 0 {
				t.Fatalf("%d objects in query.json and %d in deleted.json: the generator is off", len(ref.Query), len(ref.Deleted))
			}
			rep, err := Compare(ref, lib, everyKey(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.OK() {
				var sb strings.Builder
				_ = rep.WriteDetails(&sb)
				t.Fatalf("%d differences, first lines:\n%s", len(rep.Diffs), firstLines(sb.String(), 8))
			}

			// One change of one photo that is in the set.
			for _, pt := range perturbations(users) {
				idx := slices.IndexFunc(lib.Photos, func(p *photoinfo.PhotoInfo) bool {
					return pickedUp(p) && pt.ok(p) && pickedUpAfter(p, pt)
				})
				if idx < 0 {
					t.Errorf("%s: no photo suits", pt.name)
					continue
				}
				p := lib.Photos[idx]
				// Only the object of the photo is in the reference: the others
				// are then differences of uuid, which are left out. This keeps
				// the loop short; the whole library is still in Library.Photos
				// for the bursts.
				one := &Reference{Library: ref.Library}
				for _, o := range slices.Concat(ref.Query, ref.Deleted) {
					if id, _ := decodeString(o["uuid"]); id == p.UUID {
						one.Query = []Object{o}
					}
				}
				saved := *p
				pt.do(p)
				rep, err := Compare(one, lib, everyKey(), nil)
				*p = saved
				if err != nil {
					t.Errorf("%s: %v", pt.name, err)
					continue
				}
				set := map[string]bool{}
				for _, d := range rep.Diffs {
					if d.UUID == p.UUID {
						set[d.Field] = true
					}
				}
				want := slices.Clone(pt.want)
				slices.Sort(want)
				if got := slices.Sorted(maps.Keys(set)); !slices.Equal(got, want) {
					t.Errorf("%s: differences in %v, want %v", pt.name, got, want)
				}
			}
		})
	}
}

// pickedUpAfter tells that the perturbation keeps the photo in the set.
func pickedUpAfter(p *photoinfo.PhotoInfo, pt perturbation) bool {
	c := *p
	pt.do(&c)
	return pickedUp(&c)
}

func firstLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "")
}

// TestCompareOrderIndependent: the order of the objects of the reference and
// of lib.Photos does not change the report.
func TestCompareOrderIndependent(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(5, 6))
	lib := randomLibrary(r, 120)
	ref := randomReference(r, lib)
	// Differences of several kinds.
	for i := 0; i < len(lib.Photos); i += 7 {
		lib.Photos[i].Title += "x"
		lib.Photos[i].Width++
	}
	lib.Photos = append(lib.Photos, &photoinfo.PhotoInfo{UUID: "00000000-0000-0000-0009-000000000001", IsPhoto: true})
	ref.Query = ref.Query[1:]

	want, err := Compare(ref, lib, everyKey(), nil)
	if err != nil || len(want.Diffs) < 10 {
		t.Fatalf("Compare: %v, %d differences", err, len(want.Diffs))
	}
	rev := Library{Photos: slices.Clone(lib.Photos), Albums: slices.Clone(lib.Albums)}
	slices.Reverse(rev.Photos)
	slices.Reverse(rev.Albums)
	rref := &Reference{Library: ref.Library, Query: slices.Clone(ref.Query), Deleted: slices.Clone(ref.Deleted)}
	slices.Reverse(rref.Query)
	slices.Reverse(rref.Deleted)
	got, err := Compare(rref, rev, slices.Clone(everyKey()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the report depends on the order of the input")
	}
}

// TestCompareKeepsInput: Compare does not change what it is given, and two
// Compare calls on one Library may run at once.
func TestCompareKeepsInput(t *testing.T) {
	t.Parallel()
	lib, ref := fixtureLib(), fixtureRef()
	lib.Photos[0].Keywords = []string{"beta", "alpha"} // unsorted lists that a careless sort would reorder
	lib.Photos[0].AlbumUUIDs = []string{albumA3, albumA1, albumA3}
	snapshotLib := fixtureLib()
	snapshotLib.Photos[0].Keywords = []string{"beta", "alpha"}
	snapshotLib.Photos[0].AlbumUUIDs = []string{albumA3, albumA1, albumA3}
	snapshotRef := fixtureRef()

	for i := 0; i < 4; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			if _, err := Compare(ref, lib, everyKey(), []Divergence{{Library: "lib", UUID: "*", Field: "title", Want: []byte(`"x"`), Got: []byte(`"y"`)}}); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(func() {
		if !reflect.DeepEqual(lib, snapshotLib) {
			t.Error("Compare changed the Library")
		}
		if !reflect.DeepEqual(ref, snapshotRef) {
			t.Error("Compare changed the Reference")
		}
	})
}
