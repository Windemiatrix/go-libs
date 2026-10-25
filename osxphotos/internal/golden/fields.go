package golden

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// A rule compares one key of fields.txt. ref turns the value of the
// reference into the canonical form, got does the same for the Go side; the
// canonical forms are written as compact JSON and compared as strings.
type rule struct {
	key   string
	album bool // needs Library.Albums

	ref func(o Object) (any, error)
	// skip, when set, tells that the key is not compared for this object.
	skip func(o Object) (bool, error)
	// got receives the canonical reference value because the Go side of
	// folders depends on it (two albums with one title). Nil for uuid.
	got func(e *env, p *photoinfo.PhotoInfo, want any) (any, error)
}

// rules maps a key of fields.txt to its rule. TestKeysCoverFields checks that
// the keys are exactly those of fields.txt.
var rules = buildRules()

// PhotoKeys returns, sorted, the keys of fields.txt that Compare computes
// from Library.Photos alone; AlbumKeys returns those that also need
// Library.Albums. Together they are exactly the keys of fields.txt.
func PhotoKeys() []string { return keysOf(false) }

// AlbumKeys returns, sorted, the keys that need Library.Albums: album_info,
// albums, burst_album_info, burst_albums and folders.
func AlbumKeys() []string { return keysOf(true) }

func keysOf(album bool) []string {
	var keys []string
	for k, r := range rules {
		if r.album == album {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Defaults of upstream for a missing date (photos_datetime.py:19-21 and the
// 0 of photosdb.py:1937-1938, which is 2001-01-01 in Core Data seconds).
var (
	defaultDateAdded    = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultCreationDate = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
)

// unknownPerson is how upstream names a person without a name
// (photosdb.py:1790-1796, _constants.py:337).
const unknownPerson = "_UNKNOWN_"

func buildRules() map[string]rule {
	list := []rule{
		// uuid has no Go side: the sets of objects are compared by Compare itself.
		{key: "uuid", ref: func(o Object) (any, error) { return decode[string](o, "uuid", false, "a string") }},
		albumInfoRule("album_info", func(p *photoinfo.PhotoInfo) []string { return p.AlbumUUIDs }),
		albumInfoRule("burst_album_info", func(p *photoinfo.PhotoInfo) []string { return p.BurstAlbumUUIDs }),
		albumsRule("albums", "album_info", func(p *photoinfo.PhotoInfo) []string { return p.AlbumUUIDs }),
		albumsRule("burst_albums", "burst_album_info", func(p *photoinfo.PhotoInfo) []string { return p.BurstAlbumUUIDs }),
		foldersRule(),
		burstPhotosRule(),
		persons(),
		keywords(),
		location(),
		orientation(),
		value("incloud", true, "a boolean or null", func(p *photoinfo.PhotoInfo) *bool { return p.InCloud }),
		value("tzoffset", true, "an integer or null", func(p *photoinfo.PhotoInfo) *int { return p.TZOffset }),
		value("latitude", true, "a number or null", func(p *photoinfo.PhotoInfo) *float64 { return p.Latitude }),
		value("longitude", true, "a number or null", func(p *photoinfo.PhotoInfo) *float64 { return p.Longitude }),

		moment("date", true, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return &p.Date }),
		moment("date_added", false, defaultDateAdded, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateAdded }),
		moment("date_modified", false, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateModified }),
		moment("date_trashed", false, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateTrashed }),

		// Booleans. A null in the reference is a mistake of the form.
		flag("burst", func(p *photoinfo.PhotoInfo) bool { return p.Burst }),
		flag("burst_default_pick", func(p *photoinfo.PhotoInfo) bool { return p.BurstDefaultPick }),
		flag("burst_key", func(p *photoinfo.PhotoInfo) bool { return p.BurstKey }),
		flag("burst_selected", func(p *photoinfo.PhotoInfo) bool { return p.BurstSelected }),
		flag("favorite", func(p *photoinfo.PhotoInfo) bool { return p.Favorite }),
		flag("hidden", func(p *photoinfo.PhotoInfo) bool { return p.Hidden }),
		flag("intrash", func(p *photoinfo.PhotoInfo) bool { return p.InTrash }),
		flag("isphoto", func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto }),
		flag("ismovie", func(p *photoinfo.PhotoInfo) bool { return p.IsMovie }),
		flag("live_photo", func(p *photoinfo.PhotoInfo) bool { return p.Live }),
		flag("screenshot", func(p *photoinfo.PhotoInfo) bool { return p.Screenshot }),
		flag("screen_recording", func(p *photoinfo.PhotoInfo) bool { return p.ScreenRecording }),
		flag("slow_mo", func(p *photoinfo.PhotoInfo) bool { return p.SlowMo }),
		flag("time_lapse", func(p *photoinfo.PhotoInfo) bool { return p.TimeLapse }),
		flag("isreference", func(p *photoinfo.PhotoInfo) bool { return p.IsReference }),
		flag("hasadjustments", func(p *photoinfo.PhotoInfo) bool { return p.HasAdjustments }),
		flag("has_raw", func(p *photoinfo.PhotoInfo) bool { return p.HasRAW }),
		flag("raw_original", func(p *photoinfo.PhotoInfo) bool { return p.RawOriginal }),
		flag("ismissing", func(p *photoinfo.PhotoInfo) bool { return p.IsMissing }),
		flag("iscloudasset", func(p *photoinfo.PhotoInfo) bool { return p.IsCloudAsset }),
		flag("israw", func(p *photoinfo.PhotoInfo) bool { return p.IsRAW }),
		flag("shared", func(p *photoinfo.PhotoInfo) bool { return p.Shared }),
		flag("shared_library", func(p *photoinfo.PhotoInfo) bool { return p.SharedLibrary }),
		flag("syndicated", func(p *photoinfo.PhotoInfo) bool { return p.Syndicated }),
		flag("saved_to_library", func(p *photoinfo.PhotoInfo) bool { return p.SavedToLibrary }),

		// Strings: strict, or null is "".
		text("filename", false, func(p *photoinfo.PhotoInfo) string { return p.Filename }),
		text("original_filename", false, func(p *photoinfo.PhotoInfo) string { return p.OriginalFilename }),
		text("uti", false, func(p *photoinfo.PhotoInfo) string { return p.UTI }),
		text("uti_original", false, func(p *photoinfo.PhotoInfo) string { return p.UTIOriginal }),
		text("uti_edited", true, func(p *photoinfo.PhotoInfo) string { return p.UTIEdited }),
		text("uti_raw", true, func(p *photoinfo.PhotoInfo) string { return p.UTIRaw }),
		text("cloud_guid", true, func(p *photoinfo.PhotoInfo) string { return p.CloudMasterGUID }),
		text("description", true, func(p *photoinfo.PhotoInfo) string { return p.Description }),
		text("fingerprint", true, func(p *photoinfo.PhotoInfo) string { return p.Fingerprint }),
		text("title", true, func(p *photoinfo.PhotoInfo) string { return p.Title }),

		// Integers: null is 0.
		number("width", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Width) }),
		number("height", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Height) }),
		number("original_width", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalWidth) }),
		number("original_height", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalHeight) }),
		number("original_orientation", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalOrientation) }),
		number("original_filesize", func(p *photoinfo.PhotoInfo) int64 { return p.OriginalFilesize }),
	}
	m := make(map[string]rule, len(list))
	for _, r := range list {
		if _, dup := m[r.key]; dup {
			panic("golden: two rules for the key " + r.key)
		}
		m[r.key] = r
	}
	return m
}

// simple builds a rule whose Go side depends on the photo alone.
func simple(key string, ref func(Object) (any, error), got func(*photoinfo.PhotoInfo) any) rule {
	return rule{
		key: key,
		ref: ref,
		got: func(_ *env, p *photoinfo.PhotoInfo, _ any) (any, error) { return got(p), nil },
	}
}

// value compares a key whose reference value is read as T. null is the zero
// value of T (nil for a pointer) when nullOK, a mistake of the form otherwise.
func value[T any](key string, nullOK bool, want string, get func(*photoinfo.PhotoInfo) T) rule {
	return simple(key,
		func(o Object) (any, error) { return decode[T](o, key, nullOK, want) },
		func(p *photoinfo.PhotoInfo) any { return get(p) })
}

// flag compares a boolean; a null in the reference is a mistake of the form
// (upstream gives None only for Photos before 7, which the reference does
// not cover).
func flag(key string, get func(*photoinfo.PhotoInfo) bool) rule {
	return value(key, false, "a boolean", get)
}

// text compares a string; null is "" when nullOK.
func text(key string, nullOK bool, get func(*photoinfo.PhotoInfo) string) rule {
	want := "a string"
	if nullOK {
		want += " or null"
	}
	return value(key, nullOK, want, get)
}

// number compares an integer; null is 0. A number written with a fraction or
// an exponent (3024.0) is not an integer: osxphotos writes these keys from
// integer columns.
func number(key string, get func(*photoinfo.PhotoInfo) int64) rule {
	return value(key, true, "an integer", get)
}

// moment compares a date as a moment. required: null is a mistake of the
// form. def is what upstream gives for a missing date (the Go nil); the zero
// time means that null and nil are the same.
func moment(key string, required bool, def time.Time, get func(*photoinfo.PhotoInfo) *time.Time) rule {
	return simple(key,
		func(o Object) (any, error) {
			t, err := refTime(o, key, !required)
			if err != nil {
				return nil, err
			}
			return goMoment(t, time.Time{}), nil
		},
		func(p *photoinfo.PhotoInfo) any { return goMoment(get(p), def) })
}

// orientation is not compared for an adjusted photo: upstream reads the
// adjustments plist, which the test libraries do not have, and gives 0
// (photoinfo.py:1390-1404).
func orientation() rule {
	r := number("orientation", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Orientation) })
	r.skip = func(o Object) (bool, error) { return decode[bool](o, "hasadjustments", false, "a boolean") }
	return r
}

func keywords() rule {
	return simple("keywords",
		func(o Object) (any, error) { return refStrings(o, "keywords") },
		func(p *photoinfo.PhotoInfo) any { return strs(p.Keywords) }) // not sorted: the order is the one to check
}

// persons: the reference lists names, the Go side only tells whether there
// are people with a name.
func persons() rule {
	return simple("persons",
		func(o Object) (any, error) {
			names, err := refStrings(o, "persons")
			if err != nil {
				return nil, err
			}
			return slices.ContainsFunc(names, func(n string) bool { return n != "" && n != unknownPerson }), nil
		},
		func(p *photoinfo.PhotoInfo) any { return p.HasPersons })
}

func location() rule {
	const want = "a list of two numbers or nulls"
	return simple("location",
		func(o Object) (any, error) {
			pair, err := decode[[]*float64](o, "location", false, want)
			if err != nil {
				return nil, err
			}
			if len(pair) != 2 {
				return nil, shape("location", want)
			}
			return pair, nil
		},
		func(p *photoinfo.PhotoInfo) any { return []*float64{p.Latitude, p.Longitude} })
}

// burstPhotosRule: the other frames of the burst of the photo, sorted. They
// are found among all photos of the library, selected or not, trashed or not.
func burstPhotosRule() rule {
	return rule{
		key: "burst_photos",
		ref: func(o Object) (any, error) { return refStrings(o, "burst_photos") },
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			out := []string{}
			if !p.Burst {
				return out, nil
			}
			for _, id := range e.bursts[p.BurstUUID] {
				if id != p.UUID {
					out = append(out, id)
				}
			}
			return out, nil
		},
	}
}

// refAlbum is an element of album_info or burst_album_info of the reference.
type refAlbum struct {
	uuid   string
	title  *string // nil is null
	shared bool    // owner is not null: a shared album, which the Go side does not read
	elem   map[string]any
}

// refAlbums reads a list of albums of the reference.
func refAlbums(o Object, key string) ([]refAlbum, error) {
	items, err := decode[[]json.RawMessage](o, key, false, "a list of objects")
	if err != nil {
		return nil, err
	}
	out := make([]refAlbum, len(items))
	for i, it := range items {
		if out[i], err = refAlbumElem(it, fmt.Sprintf("%s[%d]", key, i)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// albumElemKeys are the keys of an element of album_info that the rules read.
var albumElemKeys = []string{"uuid", "title", "parent", "folder_names", "creation_date", "start_date", "end_date", "owner"}

func refAlbumElem(raw json.RawMessage, path string) (refAlbum, error) {
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return refAlbum{}, shape(path, "an object")
	}
	for _, k := range albumElemKeys {
		if _, ok := keys[k]; !ok {
			return refAlbum{}, shape(path+"."+k, "a value (the key is missing)")
		}
	}
	var e struct {
		UUID         *string   `json:"uuid"`
		Title        *string   `json:"title"`
		Parent       *string   `json:"parent"`
		FolderNames  []*string `json:"folder_names"`
		CreationDate *string   `json:"creation_date"`
		StartDate    *string   `json:"start_date"`
		EndDate      *string   `json:"end_date"`
		Owner        any       `json:"owner"`
	}
	switch {
	case json.Unmarshal(raw, &e) != nil:
		return refAlbum{}, shape(path, "an album with values of the types osxphotos writes")
	case e.UUID == nil:
		return refAlbum{}, shape(path+".uuid", "a string")
	case e.FolderNames == nil:
		return refAlbum{}, shape(path+".folder_names", "a list of strings")
	}
	elem := map[string]any{
		"uuid":         *e.UUID,
		"title":        deref(e.Title),
		"parent":       deref(e.Parent),
		"folder_names": derefAll(e.FolderNames),
	}
	for _, d := range []struct {
		key string
		s   *string
	}{{"creation_date", e.CreationDate}, {"start_date", e.StartDate}, {"end_date", e.EndDate}} {
		t, err := parseRef(d.s, path+"."+d.key)
		if err != nil {
			return refAlbum{}, err
		}
		elem[d.key] = goMoment(t, time.Time{})
	}
	return refAlbum{uuid: *e.UUID, title: e.Title, shared: e.Owner != nil, elem: elem}, nil
}

// keptAndDropped splits the elements into user albums and shared albums
// (general rule 8) and returns the names of both groups, a name being
// name(title).
func keptAndDropped(albums []refAlbum, name func(*string) string) (kept []refAlbum, keptNames, droppedNames map[string]bool) {
	keptNames, droppedNames = map[string]bool{}, map[string]bool{}
	for _, a := range albums {
		if a.shared {
			droppedNames[name(a.title)] = true
			continue
		}
		kept = append(kept, a)
		keptNames[name(a.title)] = true
	}
	return kept, keptNames, droppedNames
}

// albumsName and foldersName are the names of an album without a title: ""
// in a list of titles and "null" among the keys of folders (json.dumps writes
// the key None as null).
func albumsName(t *string) string { return deref(t) }

func foldersName(t *string) string {
	if t == nil {
		return "null"
	}
	return *t
}

// albumInfoRule compares the albums of a photo as a set by uuid.
func albumInfoRule(key string, uuids func(*photoinfo.PhotoInfo) []string) rule {
	return rule{
		key:   key,
		album: true,
		ref: func(o Object) (any, error) {
			albums, err := refAlbums(o, key)
			if err != nil {
				return nil, err
			}
			kept, _, _ := keptAndDropped(albums, albumsName)
			byUUID := map[string]map[string]any{}
			for _, a := range kept {
				if _, dup := byUUID[a.uuid]; !dup {
					byUUID[a.uuid] = a.elem
				}
			}
			return sortedElems(byUUID), nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			albums, err := e.userAlbums(uuids(p))
			if err != nil {
				return nil, err
			}
			byUUID := map[string]map[string]any{}
			for _, a := range albums {
				byUUID[a.UUID] = e.elems[a.UUID]
			}
			return sortedElems(byUUID), nil
		},
	}
}

// sortedElems lists the elements in the order of their uuid; the list is
// never nil.
func sortedElems(byUUID map[string]map[string]any) []any {
	out := make([]any, 0, len(byUUID))
	for _, id := range slices.Sorted(maps.Keys(byUUID)) {
		out = append(out, byUUID[id])
	}
	return out
}

// albumsRule compares the titles of the albums of a photo: sorted, without
// repeats. infoKey is the list of albums of the reference the titles belong
// to (the titles of shared albums are removed by it).
func albumsRule(key, infoKey string, uuids func(*photoinfo.PhotoInfo) []string) rule {
	return rule{
		key:   key,
		album: true,
		ref: func(o Object) (any, error) {
			albums, err := refAlbums(o, infoKey)
			if err != nil {
				return nil, err
			}
			_, keptNames, droppedNames := keptAndDropped(albums, albumsName)
			titles, err := refStrings(o, key)
			if err != nil {
				return nil, err
			}
			out := []string{}
			for _, t := range titles {
				if droppedNames[t] && !keptNames[t] {
					continue
				}
				out = append(out, t)
			}
			return out, nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			albums, err := e.userAlbums(uuids(p))
			if err != nil {
				return nil, err
			}
			set := map[string]bool{}
			for _, a := range albums {
				set[a.Title] = true
			}
			titles := slices.Sorted(maps.Keys(set))
			if titles == nil {
				titles = []string{} // written as [], not null
			}
			return titles, nil
		},
	}
}

// foldersRule compares the dictionary "title → folder names" of album_info.
func foldersRule() rule {
	return rule{
		key:   "folders",
		album: true,
		ref: func(o Object) (any, error) {
			albums, err := refAlbums(o, "album_info")
			if err != nil {
				return nil, err
			}
			_, keptNames, droppedNames := keptAndDropped(albums, foldersName)
			const want = "an object of lists of strings"
			dict, err := decode[map[string][]*string](o, "folders", false, want)
			if err != nil {
				return nil, err
			}
			out := map[string]any{}
			for title, names := range dict {
				if names == nil {
					return nil, shape("folders", want)
				}
				if !droppedNames[title] || keptNames[title] {
					out[title] = derefAll(names)
				}
			}
			return out, nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, want any) (any, error) {
			albums, err := e.userAlbums(p.AlbumUUIDs)
			if err != nil {
				return nil, err
			}
			candidates := map[string][][]string{}
			for _, a := range albums {
				title := a.Title
				if title == "" {
					title = "null"
				}
				candidates[title] = append(candidates[title], strs(a.FolderNames))
			}
			wantDict, _ := want.(map[string]any)
			out := map[string]any{}
			for title, list := range candidates {
				// The reference keeps one value per title; with several albums of one
				// title it has to match the folders of one of them.
				chosen := slices.MinFunc(list, slices.Compare[[]string])
				if w, ok := wantDict[title].([]string); ok && slices.ContainsFunc(list, func(c []string) bool { return slices.Equal(c, w) }) {
					chosen = w
				}
				out[title] = chosen
			}
			return out, nil
		},
	}
}
