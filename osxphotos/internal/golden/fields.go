package golden

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

type rule struct {
	key string

	ref  func(o Object) (any, error)
	skip func(o Object) (bool, error)
	// got receives the dictionary of the photo and the canonical reference
	// value: folders reads album_info too and depends on the reference (two
	// albums with one title).
	got func(d map[string]any, want any) (any, error)
}

var rules = buildRules()

// Keys returns the keys of fields.txt, sorted.
func Keys() []string { return slices.Sorted(maps.Keys(rules)) }

// The default of upstream for a missing date (photos_datetime.py:19-21), which
// the order of the reference takes as this moment.
var defaultDateTimeLocal = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func buildRules() map[string]rule {
	list := []rule{
		// uuid has no Go side: the sets of objects are compared by Compare itself.
		{key: "uuid", ref: func(o Object) (any, error) { return decode[string](o, "uuid", false, "a string") }},
		albumInfoRule("album_info"),
		albumInfoRule("burst_album_info"),
		projectInfoRule(),
		stringList("albums", false),
		stringList("burst_albums", false),
		foldersRule(),
		importInfoRule(),
		commentsRule("comments", true),
		commentsRule("likes", false),
		personInfoRule(),
		faceInfoRule(),
		cloudMetadataRule(),
		stringList("labels", false),
		stringList("labels_normalized", false),
		searchInfoRule("search_info"),
		searchInfoRule("search_info_normalized"),
		stringList("burst_photos", true),
		stringList("persons", false),
		stringList("keywords", false),
		stringList("path_derivatives", true),
		location(),
		orientation(),
		value[*bool]("incloud", true, "a boolean or null", pyValue),
		value[*int]("tzoffset", true, "an integer or null", pyValue),
		value[*float64]("latitude", true, "a number or null", pyValue),
		value[*float64]("longitude", true, "a number or null", pyValue),
		value[*int]("adjustment_type", true, "an integer or null", pyValue),
		importedBy(),
		adjustmentsRule("adjustments"),
		adjustmentsRule("original_adjustments"),

		zonedDate("date"),
		zonedDate("date_original"),
		exifInfoRule(),
		scoreRule(),
		placeRule(),
		moment("date_added"),
		moment("date_modified"),
		moment("date_trashed"),

		flag("burst"),
		flag("burst_default_pick"),
		flag("burst_key"),
		flag("burst_selected"),
		flag("favorite"),
		flag("hidden"),
		flag("intrash"),
		flag("isphoto"),
		flag("ismovie"),
		flag("live_photo"),
		flag("screenshot"),
		flag("screen_recording"),
		flag("slow_mo"),
		flag("time_lapse"),
		flag("isreference"),
		flag("hasadjustments"),
		flag("has_raw"),
		flag("raw_original"),
		flag("ismissing"),
		flag("iscloudasset"),
		flag("israw"),
		flag("shared"),
		flag("shared_library"),
		flag("syndicated"),
		flag("saved_to_library"),
		flag("hdr"),
		flag("panorama"),
		flag("portrait"),
		flag("selfie"),
		flag("visible"),
		flag("shared_moment"),
		flag("external_edit"),

		text("filename", false),
		text("original_filename", false),
		text("uti", false),
		text("uti_original", false),
		text("uti_edited", true),
		text("uti_raw", true),
		text("cloud_guid", true),
		text("description", true),
		text("fingerprint", true),
		text("title", true),
		text("tzname", true),
		text("cloud_owner_hashed_id", true),
		text("owner", true),

		number("width"),
		number("height"),
		number("original_width"),
		number("original_height"),
		number("original_orientation"),
		number("original_filesize"),
		number("spatial"),
		// iPhoto only: upstream gives 0 for Photos (photoinfo.py:822-825).
		number("rating"),
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

func simple(key string, ref func(Object) (any, error), got func(v any) any) rule {
	return rule{
		key: key,
		ref: ref,
		got: func(d map[string]any, _ any) (any, error) { return got(d[key]), nil },
	}
}

func value[T any](key string, nullOK bool, want string, got func(any) any) rule {
	return simple(key, func(o Object) (any, error) { return decode[T](o, key, nullOK, want) }, got)
}

// A null boolean is a mistake of the form: upstream gives None only for Photos
// before 7, which the reference does not cover.
func flag(key string) rule { return value[bool](key, false, "a boolean", pyValue) }

// The reference reads null as "" where nullOK (rule 3), and so does the Go
// side, where DB.AsDict gives nil for a string the port holds as "".
func text(key string, nullOK bool) rule {
	if nullOK {
		return value[string](key, true, "a string or null", emptyIfNull)
	}
	return value[string](key, false, "a string", pyValue)
}

// A number written with a fraction or an exponent (3024.0) is not an integer:
// osxphotos writes these keys from integer columns.
func number(key string) rule { return value[int64](key, true, "an integer", pyValue) }

// Upstream gives these dates in the zone of the computer that wrote the
// reference (photos_datetime_local, photosdb/photosdb.py:2161, 2297, 2321), so
// only the moment counts.
func moment(key string) rule {
	return simple(key,
		func(o Object) (any, error) {
			t, err := refTime(o, key, true)
			if err != nil {
				return nil, err
			}
			return momentOrNil(t), nil
		},
		pyValue)
}

// Upstream gives date and date_original in the zone of the photo
// (photos_datetime.py:47-67), so the offset counts too.
func zonedDate(key string) rule {
	return simple(key,
		func(o Object) (any, error) {
			t, err := refTime(o, key, false)
			if err != nil {
				return nil, err
			}
			return zonedMoment(*t), nil
		},
		func(v any) any {
			t, _ := v.(time.Time)
			return zonedMoment(t)
		})
}

func zonedMoment(t time.Time) []any {
	_, offset := t.Zone()
	return []any{momentString(t), offset}
}

func zonedOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return zonedMoment(*t)
}

// The keys of exif_info in the order of the fields of exifinfo.ExifInfo, which
// the rule reads by position (exifinfo.py:18-39).
var exifKeys = []string{
	"flash_fired", "iso", "metering_mode", "sample_rate", "track_format", "white_balance",
	"aperture", "bit_rate", "duration", "exposure_bias", "focal_length", "fps",
	"latitude", "longitude", "shutter_speed", "camera_make", "camera_model", "codec", "lens_model",
	"date", "tzoffset", "tzname",
}

// json() writes the object of a photo without a row too, with every value
// null (photoinfo.py:1700-1702, 2121); null is nil for a number and a flag and
// "" for a string. date is the pair of zonedDate, nil for null.
func exifInfoRule() rule {
	const key = "exif_info"
	fields := reflect.TypeOf(photoinfo.PhotoInfo{}.ExifInfo)
	return simple(key,
		func(o Object) (any, error) {
			raw, err := decode[map[string]json.RawMessage](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			out := make(map[string]any, len(exifKeys))
			for i, k := range exifKeys {
				path := key + "." + k
				m, ok := raw[k]
				if !ok {
					return nil, shape(path, "a value (the key is missing)")
				}
				if k == "date" {
					s, err := decodeRaw[*string](m, path, true, "an ISO 8601 string or null")
					if err != nil {
						return nil, err
					}
					t, err := parseRef(s, path)
					if err != nil {
						return nil, err
					}
					out[k] = zonedOrNil(t)
					continue
				}
				v := reflect.New(fields.Field(i).Type)
				if json.Unmarshal(m, v.Interface()) != nil {
					return nil, shape(path, "a value of the type osxphotos writes")
				}
				out[k] = v.Elem().Interface()
			}
			return out, nil
		},
		func(v any) any {
			out := emptyFor(pyValue(v), "camera_make", "camera_model", "codec", "lens_model", "tzname")
			raw, _ := v.(map[string]any)
			if t, ok := raw["date"].(time.Time); ok {
				out.(map[string]any)["date"] = zonedMoment(t)
			}
			return out
		})
}

// The keys of score in the order of the fields of scoreinfo.ScoreInfo
// (scoreinfo.py:14-40).
var scoreKeys = []string{
	"overall", "curation", "promotion", "highlight_visibility", "behavioral", "failure", "harmonious_color",
	"immersiveness", "interaction", "interesting_subject", "intrusive_object_presence", "lively_color", "low_light",
	"noise", "pleasant_camera_tilt", "pleasant_composition", "pleasant_lighting", "pleasant_pattern",
	"pleasant_perspective", "pleasant_post_processing", "pleasant_reflection", "pleasant_symmetry",
	"sharply_focused_subject", "tastefully_blurred", "well_chosen_subject", "well_framed_subject", "well_timed_shot",
}

// json() writes the object for every photo, zeros for a photo without a row
// (photoinfo.py:1505-1535, 2126, 2188). A null is None: a NULL column, and
// behavioral and interaction from Photos 11 on
// (_photosdb_process_scoreinfo.py:62-76); DB.AsDict gives nil there.
func scoreRule() rule {
	const key = "score"
	return simple(key,
		func(o Object) (any, error) {
			raw, err := decode[map[string]json.RawMessage](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			out := make(map[string]any, len(scoreKeys))
			for _, k := range scoreKeys {
				path := key + "." + k
				m, ok := raw[k]
				if !ok {
					return nil, shape(path, "a value (the key is missing)")
				}
				if out[k], err = decodeRaw[*float64](m, path, true, "a number or null"); err != nil {
					return nil, err
				}
			}
			return out, nil
		},
		pyValue)
}

// The keys of place.names and place.address in the order of the fields of
// placeinfo.Names and placeinfo.Address, which the rule reads by position
// (placeinfo.py:30-42, 51-74).
var (
	placeNameKeys = []string{
		"field0", "country", "state_province", "sub_administrative_area", "city", "field5",
		"additional_city_info", "ocean", "area_of_interest", "inland_water", "field10", "region",
		"sub_throughfare", "field13", "postal_code", "field15", "field16", "street_address", "body_of_water",
	}
	addressKeys = []string{"street", "sub_locality", "city", "sub_administrative_area", "state_province", "postal_code", "country", "iso_country_code"}
)

// json() writes {} for a photo without a place (photoinfo.py:2125) and sorts
// only the lists at the top level, so the lists of names keep the order of
// area (2277-2283). null is "" for a string and false for ishome; address is
// an object also without a postal address (placeinfo.py:622-625, 708-716).
func placeRule() rule {
	const key = "place"
	return simple(key,
		func(o Object) (any, error) {
			raw, err := decode[map[string]json.RawMessage](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			if len(raw) == 0 {
				return placeValue(nil), nil
			}
			pl, err := decodePlace(raw)
			if err != nil {
				return nil, err
			}
			return placeValue(&pl), nil
		},
		func(v any) any {
			out := emptyFor(pyValue(v), "name", "country_code", "address_str")
			if m, ok := out.(map[string]any); ok {
				emptyFor(m["address"], addressKeys...)
			}
			return out
		})
}

func decodePlace(raw map[string]json.RawMessage) (placeinfo.PlaceInfo, error) {
	const want = "a string or null"
	names, namesErr := decodeAt[map[string]json.RawMessage](raw, "names", "place.names", false, "an object")
	address, addressErr := decodeAt[map[string]json.RawMessage](raw, "address", "place.address", false, "an object")
	name, nameErr := decodeAt[*string](raw, "name", "place.name", true, want)
	code, codeErr := decodeAt[*string](raw, "country_code", "place.country_code", true, want)
	home, homeErr := decodeAt[*bool](raw, "ishome", "place.ishome", true, "a boolean or null")
	str, strErr := decodeAt[*string](raw, "address_str", "place.address_str", true, want)
	if err := errors.Join(namesErr, addressErr, nameErr, codeErr, homeErr, strErr); err != nil {
		return placeinfo.PlaceInfo{}, err
	}
	pl := placeinfo.PlaceInfo{Name: deref(name), CountryCode: deref(code), IsHome: home != nil && *home, AddressStr: deref(str)}
	lists := reflect.ValueOf(&pl.Names).Elem()
	for i, k := range placeNameKeys {
		list, err := decodeAt[[]*string](names, k, "place.names."+k, false, "a list of strings")
		if err != nil {
			return placeinfo.PlaceInfo{}, err
		}
		lists.Field(i).Set(reflect.ValueOf(derefAll(list)))
	}
	parts := reflect.ValueOf(&pl.Address).Elem()
	for i, k := range addressKeys {
		s, err := decodeAt[*string](address, k, "place.address."+k, true, want)
		if err != nil {
			return placeinfo.PlaceInfo{}, err
		}
		parts.Field(i).SetString(deref(s))
	}
	return pl, nil
}

func placeValue(pl *placeinfo.PlaceInfo) map[string]any {
	if pl == nil {
		return map[string]any{}
	}
	names, address := map[string]any{}, map[string]any{}
	lists, parts := reflect.ValueOf(pl.Names), reflect.ValueOf(pl.Address)
	for i, k := range placeNameKeys {
		names[k] = strs(lists.Field(i).Interface().([]string))
	}
	for i, k := range addressKeys {
		address[k] = parts.Field(i).String()
	}
	return map[string]any{
		"name": pl.Name, "names": names, "country_code": pl.CountryCode,
		"ishome": pl.IsHome, "address_str": pl.AddressStr, "address": address,
	}
}

// Not compared for an adjusted photo: upstream reads the adjustments plist,
// which the test libraries do not have, and gives 0 (photoinfo.py:1388-1402).
func orientation() rule {
	r := number("orientation")
	r.skip = func(o Object) (bool, error) { return decode[bool](o, "hasadjustments", false, "a boolean") }
	return r
}

// json() writes {} for a photo without the file (photoinfo.py:2210-2212,
// 2246-2248). asdict writes timestamp as the isoformat() of a UTC datetime
// (adjustmentsinfo.py:44-46, 212-214), where the Go side holds a time.Time:
// both are compared as a moment (rule 2).
func adjustmentsRule(key string) rule {
	return simple(key,
		func(o Object) (any, error) {
			m, err := decode[map[string]any](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			if s, ok := m["timestamp"].(string); ok {
				t, ok := parseMoment(s)
				if !ok {
					return nil, shape(key+".timestamp", "an ISO 8601 string")
				}
				m["timestamp"] = momentString(t)
			}
			return m, nil
		},
		pyValue)
}

// The reference list is sorted by upstream json() (photoinfo.py:2277-2283);
// sorted sorts a list that DB.AsDict leaves to json().
func stringList(key string, sorted bool) rule {
	return simple(key,
		func(o Object) (any, error) { return refStrings(o, key) },
		func(v any) any { return goStrings(v, sorted) })
}

func location() rule {
	return simple("location",
		func(o Object) (any, error) { return refPair[*float64](o, "location", "a list of two numbers or nulls") },
		pyValue)
}

// photoinfo.py:682-694. json() sorts the pair, None as "" (photoinfo.py:2277-2283),
// so the order of name and bundle ID is not in the reference.
func importedBy() rule {
	return simple("imported_by",
		func(o Object) (any, error) {
			pair, err := refPair[*string](o, "imported_by", "a list of two strings or nulls")
			if err != nil {
				return nil, err
			}
			return derefAll(pair), nil
		},
		func(v any) any { return goStrings(v, true) })
}

func refPair[T any](o Object, key, want string) ([]T, error) {
	pair, err := decode[[]T](o, key, false, want)
	if err == nil && len(pair) != 2 {
		err = shape(key, want)
	}
	return pair, err
}

// The asdict of CommentInfo and LikeInfo (commentinfo.py:10-32), unsorted as
// upstream json() (photoinfo.py:2282); a date is in the computer's zone
// (_photosdb_process_comments.py:130), so only its moment counts.
func commentsRule(key string, hasText bool) rule {
	return simple(key,
		func(o Object) (any, error) {
			want := "a list of " + key + " as osxphotos writes them"
			items, err := decode[[]struct {
				Datetime, User, Text *string
				IsMine               *bool
			}](o, key, false, want)
			if err != nil {
				return nil, err
			}
			out := make([]any, len(items))
			for i, it := range items {
				t, err := parseRef(it.Datetime, key)
				if err != nil || t == nil || it.IsMine == nil || (hasText && it.Text == nil) {
					return nil, shape(key, want)
				}
				elem := map[string]any{"datetime": momentString(*t), "user": deref(it.User), "ismine": *it.IsMine}
				if it.Text != nil {
					elem["text"] = *it.Text
				}
				out[i] = elem
			}
			return out, nil
		},
		func(v any) any { return eachEmptyFor(v, "user") })
}

// The asdict of PersonInfo (personinfo.py:112-125), a person once per face.
func personInfoRule() rule {
	const key = "person_info"
	return simple(key,
		func(o Object) (any, error) {
			items, err := decode[[]json.RawMessage](o, key, false, "a list of objects")
			if err != nil {
				return nil, err
			}
			persons := make([]any, len(items))
			for i, it := range items {
				p, err := decodePerson(it, fmt.Sprintf("%s[%d]", key, i))
				if err != nil {
					return nil, err
				}
				// A literal, not PersonInfo.AsDict: the Go side is that, and a wrong key
				// of it would be wrong on both sides.
				persons[i] = map[string]any{
					"uuid": p.UUID, "name": p.Name, "displayname": p.DisplayName, "keyface": p.KeyFace, "facecount": p.FaceCount,
					"keyphoto": p.KeyPhotoUUID, "favorite": p.Favorite, "sort_order": p.SortOrder, "feature_less": p.FeatureLess,
				}
			}
			return personElems(persons), nil
		},
		personElems)
}

func decodePerson(raw json.RawMessage, path string) (personinfo.PersonInfo, error) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return personinfo.PersonInfo{}, shape(path, "an object")
	}
	const str, integer = "a string or null", "an integer or null"
	uuid, uuidErr := decodeAt[string](m, "uuid", path+".uuid", false, "a string")
	name, nameErr := decodeAt[string](m, "name", path+".name", false, "a string")
	display, displayErr := decodeAt[string](m, "displayname", path+".displayname", true, str)
	keyFace, keyFaceErr := decodeAt[int64](m, "keyface", path+".keyface", true, integer)
	faceCount, faceCountErr := decodeAt[int](m, "facecount", path+".facecount", true, integer)
	keyPhoto, keyPhotoErr := decodeAt[string](m, "keyphoto", path+".keyphoto", true, str)
	favorite, favoriteErr := decodeAt[bool](m, "favorite", path+".favorite", false, "a boolean")
	sortOrder, sortOrderErr := decodeAt[int](m, "sort_order", path+".sort_order", true, integer)
	featureLess, featureLessErr := decodeAt[bool](m, "feature_less", path+".feature_less", false, "a boolean")
	if err := errors.Join(uuidErr, nameErr, displayErr, keyFaceErr, faceCountErr, keyPhotoErr, favoriteErr, sortOrderErr, featureLessErr); err != nil {
		return personinfo.PersonInfo{}, err
	}
	return personinfo.PersonInfo{
		UUID: uuid, Name: name, DisplayName: display, KeyFace: keyFace, FaceCount: faceCount,
		KeyPhotoUUID: keyPhoto, Favorite: favorite, SortOrder: sortOrder, FeatureLess: featureLess,
	}, nil
}

// json() sorts no list of objects, and upstream sorts the persons of a photo
// by name only, equal names in the order of SQLite (photoinfo.py:605-611): both
// sides are compared in order, persons of equal name sorted by uuid.
func personElems(v any) any {
	out := eachEmptyFor(v, "uuid", "displayname", "keyphoto")
	if list, ok := out.([]any); ok {
		sortRuns(list, "uuid")
	}
	return out
}

// The asdict of FaceInfo (personinfo.py:455-496), sorted by upstream by name
// only, equal names in the order of SQLite (photoinfo.py:613-623): faces of
// equal name are compared sorted by _pk. A number of the reference is a
// float64, which equals an integer of the Go side of that value.
func faceInfoRule() rule {
	const key = "face_info"
	return simple(key,
		func(o Object) (any, error) {
			items, err := decode[[]map[string]any](o, key, false, "a list of objects")
			if err != nil {
				return nil, err
			}
			faces := make([]any, len(items))
			for i, it := range items {
				faces[i] = it
			}
			return faceElems(faces), nil
		},
		faceElems)
}

func faceElems(v any) any {
	out := eachEmptyFor(v, "uuid", "name", "asset_uuid")
	if list, ok := out.([]any); ok {
		sortRuns(list, "_pk")
	}
	return out
}

// The canonical form is that of both sides: a number of the reference is a
// float64, one of the Go side may be an integer.
func sortRuns(list []any, then string) {
	field := func(e any, k string) string {
		m, _ := e.(map[string]any)
		s, _ := canonical(m[k]) // a value of the reference or of DB.AsDict, not NaN
		return s
	}
	for i := 0; i < len(list); {
		j := i + 1
		for j < len(list) && field(list[j], "name") == field(list[i], "name") {
			j++
		}
		slices.SortStableFunc(list[i:j], func(a, b any) int { return strings.Compare(field(a, then), field(b, then)) })
		i = j
	}
}

type refAlbum struct {
	uuid string
	elem map[string]any
}

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

var albumElemKeys = []string{"uuid", "title", "parent", "folder_names", "folder_list", "sort_order", "creation_date", "start_date", "end_date", "owner"}

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
		FolderList   []*string `json:"folder_list"`
		SortOrder    any       `json:"sort_order"`
		CreationDate *string   `json:"creation_date"`
		StartDate    *string   `json:"start_date"`
		EndDate      *string   `json:"end_date"`
		Owner        *string   `json:"owner"`
	}
	switch {
	case json.Unmarshal(raw, &e) != nil:
		return refAlbum{}, shape(path, "an album with values of the types osxphotos writes")
	case e.UUID == nil:
		return refAlbum{}, shape(path+".uuid", "a string")
	case e.FolderNames == nil:
		return refAlbum{}, shape(path+".folder_names", "a list of strings")
	case e.FolderList == nil:
		return refAlbum{}, shape(path+".folder_list", "a list of strings")
	}
	elem := map[string]any{
		"uuid":         *e.UUID,
		"title":        deref(e.Title),
		"owner":        deref(e.Owner),
		"parent":       deref(e.Parent),
		"folder_names": derefAll(e.FolderNames),
		"folder_list":  derefAll(e.FolderList),
		"sort_order":   e.SortOrder,
	}
	for _, d := range []struct {
		key string
		s   *string
	}{{"creation_date", e.CreationDate}, {"start_date", e.StartDate}, {"end_date", e.EndDate}} {
		t, err := parseRef(d.s, path+"."+d.key)
		if err != nil {
			return refAlbum{}, err
		}
		elem[d.key] = momentOrNil(t)
	}
	return refAlbum{uuid: *e.UUID, elem: elem}, nil
}

func albumInfoRule(key string) rule {
	return simple(key,
		func(o Object) (any, error) {
			albums, err := refAlbums(o, key)
			if err != nil {
				return nil, err
			}
			byUUID := map[string]map[string]any{}
			for _, a := range albums {
				if _, dup := byUUID[a.uuid]; !dup {
					byUUID[a.uuid] = a.elem
				}
			}
			return sortedElems(byUUID), nil
		},
		func(v any) any { return goAlbums(v) })
}

// The albums of a photo are a set by uuid (rule 5).
func goAlbums(v any, drop ...string) any {
	list, ok := eachEmptyFor(v, "title", "owner", "parent").([]any)
	if !ok {
		return pyValue(v)
	}
	byUUID := map[string]map[string]any{}
	for _, e := range list {
		m, _ := e.(map[string]any)
		for _, k := range drop {
			delete(m, k)
		}
		id, _ := m["uuid"].(string)
		if _, dup := byUUID[id]; !dup {
			byUUID[id] = m
		}
	}
	return sortedElems(byUUID)
}

// A project is an album without folders (albuminfo.py:368-392) and has the
// keys of an album (photoinfo.py:2226), so the reference side is that of
// album_info. Its parent is not compared: osxphotos writes it only when the
// parent row holds a folder or a user album outside the trash
// (albuminfo.py:296, 525-527), which depends on the other rows under that
// parent, not on the project; TestProjects checks ParentUUID against the
// database.
func projectInfoRule() rule {
	const key = "project_info"
	r := albumInfoRule(key)
	ref := r.ref
	r.ref = func(o Object) (any, error) {
		v, err := ref(o)
		if err != nil {
			return nil, err
		}
		elems, _ := v.([]any)
		for _, el := range elems {
			m, _ := el.(map[string]any) // new on every call of ref
			delete(m, "parent")
		}
		return elems, nil
	}
	r.got = func(d map[string]any, _ any) (any, error) { return goAlbums(d[key], "parent"), nil }
	return r
}

func sortedElems(byUUID map[string]map[string]any) []any {
	out := make([]any, 0, len(byUUID))
	for _, id := range slices.Sorted(maps.Keys(byUUID)) {
		out = append(out, byUUID[id])
	}
	return out
}

var importElemKeys = []string{"uuid", "title", "creation_date", "start_date", "end_date"}

// A photo without a session is {} (photoinfo.py:2221-2223); a session is the
// asdict of albuminfo.py:350-358.
func importInfoRule() rule {
	const key = "import_info"
	return simple(key,
		func(o Object) (any, error) {
			raw, err := decode[map[string]json.RawMessage](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			if len(raw) == 0 {
				return map[string]any{}, nil
			}
			for _, k := range importElemKeys {
				if _, ok := raw[k]; !ok {
					return nil, shape(key+"."+k, "a value (the key is missing)")
				}
			}
			uuid, err := decodeRaw[string](raw["uuid"], key+".uuid", false, "a string")
			if err != nil {
				return nil, err
			}
			title, err := decodeRaw[*string](raw["title"], key+".title", true, "a string or null")
			if err != nil {
				return nil, err
			}
			elem := map[string]any{"uuid": uuid, "title": deref(title)}
			for _, d := range importElemKeys[2:] {
				s, err := decodeRaw[*string](raw[d], key+"."+d, true, "an ISO 8601 string")
				if err != nil {
					return nil, err
				}
				t, err := parseRef(s, key+"."+d)
				if err != nil {
					return nil, err
				}
				elem[d] = momentOrNil(t)
			}
			return elem, nil
		},
		func(v any) any { return emptyFor(pyValue(v), "title") })
}

// json.dumps writes the key None of an album without a title as null
// (photoinfo.py:2123, 2284-2286).
func foldersRule() rule {
	return rule{
		key: "folders",
		ref: func(o Object) (any, error) {
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
				out[title] = derefAll(names)
			}
			return out, nil
		},
		got: func(d map[string]any, want any) (any, error) {
			out := map[string]any{}
			folders, _ := d["folders"].(map[string]any)
			for title, names := range folders {
				out[cmp.Or(title, "null")] = pyValue(names)
			}
			// The reference keeps one value per title, of the last album in the
			// order of upstream; with several albums of one title it has to
			// match the folders of one of them.
			wantDict, _ := want.(map[string]any)
			albums, _ := pyValue(d["album_info"]).([]any)
			for _, el := range albums {
				m, _ := el.(map[string]any)
				title, _ := m["title"].(string)
				title = cmp.Or(title, "null")
				names, _ := m["folder_names"].([]string)
				w, ok := wantDict[title].([]string)
				if _, kept := out[title]; kept && ok && slices.Equal(names, w) {
					out[title] = w
				}
			}
			return out, nil
		},
	}
}
