package golden

import (
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

// burstDefaultPick is BURST_DEFAULT_PICK (_constants.py:678).
const burstDefaultPick = 4

func buildRules() map[string]rule {
	list := []rule{
		// uuid has no Go side: the sets of objects are compared by Compare itself.
		{key: "uuid", ref: func(o Object) (any, error) { return refString(o, "uuid", false) }},
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
		optionalBool("incloud", func(p *photoinfo.PhotoInfo) *bool { return p.InCloud }),
		optionalInt("tzoffset", func(p *photoinfo.PhotoInfo) *int { return p.TZOffset }),
		optionalFloat("latitude", func(p *photoinfo.PhotoInfo) *float64 { return p.Latitude }),
		optionalFloat("longitude", func(p *photoinfo.PhotoInfo) *float64 { return p.Longitude }),

		moment("date", true, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return &p.Date }),
		moment("date_added", false, defaultDateAdded, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateAdded }),
		moment("date_modified", false, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateModified }),
		moment("date_trashed", false, time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateTrashed }),

		// Booleans. A null in the reference is a mistake of the form.
		flag("burst", func(p *photoinfo.PhotoInfo) bool { return p.Burst }),
		flag("burst_default_pick", func(p *photoinfo.PhotoInfo) bool { return p.BurstPickType&burstDefaultPick != 0 }),
		flag("burst_key", func(p *photoinfo.PhotoInfo) bool { return p.BurstKey }),
		flag("burst_selected", func(p *photoinfo.PhotoInfo) bool { return p.BurstSelected }),
		flag("favorite", func(p *photoinfo.PhotoInfo) bool { return p.Favorite }),
		flag("hidden", func(p *photoinfo.PhotoInfo) bool { return p.Hidden }),
		flag("intrash", func(p *photoinfo.PhotoInfo) bool { return p.InTrash }),
		flag("isphoto", func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto }),
		flag("ismovie", func(p *photoinfo.PhotoInfo) bool { return p.IsMovie }),
		flag("live_photo", func(p *photoinfo.PhotoInfo) bool { return p.Live }),
		flag("screenshot", func(p *photoinfo.PhotoInfo) bool { return p.Screenshot }),
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

func flag(key string, get func(*photoinfo.PhotoInfo) bool) rule {
	return simple(key,
		func(o Object) (any, error) { return refBool(o, key) },
		func(p *photoinfo.PhotoInfo) any { return get(p) })
}

func text(key string, nullOK bool, get func(*photoinfo.PhotoInfo) string) rule {
	return simple(key,
		func(o Object) (any, error) { return refString(o, key, nullOK) },
		func(p *photoinfo.PhotoInfo) any { return get(p) })
}

func number(key string, get func(*photoinfo.PhotoInfo) int64) rule {
	return simple(key,
		func(o Object) (any, error) { return refInt(o, key, true) },
		func(p *photoinfo.PhotoInfo) any { return get(p) })
}

func optionalBool(key string, get func(*photoinfo.PhotoInfo) *bool) rule {
	return simple(key,
		func(o Object) (any, error) {
			raw, err := field(o, key)
			if err != nil {
				return nil, err
			}
			if isNull(raw) {
				return nil, nil // null is a value of the reference
			}
			b, ok := decodeBool(raw)
			if !ok {
				return nil, shape(key, "a boolean or null")
			}
			return b, nil
		},
		func(p *photoinfo.PhotoInfo) any {
			if b := get(p); b != nil {
				return *b
			}
			return nil
		})
}

func optionalInt(key string, get func(*photoinfo.PhotoInfo) *int) rule {
	return simple(key,
		func(o Object) (any, error) { return refIntOrNull(o, key) },
		func(p *photoinfo.PhotoInfo) any {
			if n := get(p); n != nil {
				return int64(*n)
			}
			return nil
		})
}

func optionalFloat(key string, get func(*photoinfo.PhotoInfo) *float64) rule {
	return simple(key,
		func(o Object) (any, error) {
			raw, err := field(o, key)
			if err != nil {
				return nil, err
			}
			return floatOrNull(raw, key)
		},
		func(p *photoinfo.PhotoInfo) any { return floatPtr(get(p)) })
}

func floatPtr(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// moment compares a date as a moment. required: null is a mistake of the
// form. def is what upstream gives for a missing date (the Go nil); the zero
// time means that null and nil are the same.
func moment(key string, required bool, def time.Time, get func(*photoinfo.PhotoInfo) *time.Time) rule {
	return simple(key,
		func(o Object) (any, error) { return refMoment(o, key, !required) },
		func(p *photoinfo.PhotoInfo) any { return goMoment(get(p), def) })
}

// orientation is not compared for an adjusted photo: upstream reads the
// adjustments plist, which the test libraries do not have, and gives 0
// (photoinfo.py:1390-1404).
func orientation() rule {
	r := number("orientation", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Orientation) })
	r.skip = func(o Object) (bool, error) { return refBool(o, "hasadjustments") }
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
	return simple("location",
		func(o Object) (any, error) {
			raw, err := field(o, "location")
			if err != nil {
				return nil, err
			}
			items, ok := decodeList(raw)
			if !ok || len(items) != 2 {
				return nil, shape("location", "a list of two numbers or nulls")
			}
			out := make([]any, 2)
			for i, it := range items {
				if out[i], err = floatOrNull(it, fmt.Sprintf("location[%d]", i)); err != nil {
					return nil, err
				}
			}
			return out, nil
		},
		func(p *photoinfo.PhotoInfo) any { return []any{floatPtr(p.Latitude), floatPtr(p.Longitude)} })
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
	raw, err := field(o, key)
	if err != nil {
		return nil, err
	}
	items, ok := decodeList(raw)
	if !ok {
		return nil, shape(key, "a list of objects")
	}
	out := make([]refAlbum, len(items))
	for i, it := range items {
		a, err := refAlbumElem(it, fmt.Sprintf("%s[%d]", key, i))
		if err != nil {
			return nil, err
		}
		out[i] = a
	}
	return out, nil
}

func refAlbumElem(raw []byte, path string) (refAlbum, error) {
	obj, ok := decodeObject(raw)
	if !ok {
		return refAlbum{}, shape(path, "an object")
	}
	get := func(name string) ([]byte, error) {
		v, ok := obj[name]
		if !ok {
			return nil, shape(path+"."+name, "a value (the key is missing)")
		}
		return v, nil
	}
	var a refAlbum
	var err error
	elem := map[string]any{}

	var v []byte
	if v, err = get("uuid"); err != nil {
		return a, err
	}
	if a.uuid, err = stringValue(v, path+".uuid", false); err != nil {
		return a, err
	}
	elem["uuid"] = a.uuid

	if v, err = get("title"); err != nil {
		return a, err
	}
	if !isNull(v) {
		s, err := stringValue(v, path+".title", false)
		if err != nil {
			return a, err
		}
		a.title = &s
	}
	elem["title"] = deref(a.title)

	if v, err = get("parent"); err != nil {
		return a, err
	}
	if elem["parent"], err = stringValue(v, path+".parent", true); err != nil {
		return a, err
	}

	if v, err = get("folder_names"); err != nil {
		return a, err
	}
	if elem["folder_names"], err = stringsValue(v, path+".folder_names"); err != nil {
		return a, err
	}

	for _, d := range []struct {
		name     string
		nullable bool
	}{{"creation_date", true}, {"start_date", true}, {"end_date", true}} {
		if v, err = get(d.name); err != nil {
			return a, err
		}
		if elem[d.name], err = momentValue(v, path+"."+d.name, d.nullable); err != nil {
			return a, err
		}
	}

	if v, err = get("owner"); err != nil {
		return a, err
	}
	a.shared = !isNull(v)
	a.elem = elem
	return a, nil
}

// deref is the title of the reference as the Go side writes it: null is "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
			raw, err := field(o, "folders")
			if err != nil {
				return nil, err
			}
			dict, ok := decodeObject(raw)
			if !ok {
				return nil, shape("folders", "an object")
			}
			out := map[string]any{}
			for title, v := range dict {
				if droppedNames[title] && !keptNames[title] {
					continue
				}
				names, err := stringsValue(v, "folders.value")
				if err != nil {
					return nil, err
				}
				out[title] = names
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
