package golden

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

type rule struct {
	key         string
	needsAlbums bool

	ref  func(o Object) (any, error)
	skip func(o Object) (bool, error)
	// got receives the canonical reference value because the Go side of
	// folders depends on it (two albums with one title).
	got func(e *env, p *photoinfo.PhotoInfo, want any) (any, error)
}

var rules = buildRules()

// PhotoKeys returns, sorted, the keys of fields.txt that Compare computes
// from Library.Photos alone; AlbumKeys returns those that also need the other
// fields of Library. Together they are exactly the keys of fields.txt.
func PhotoKeys() []string { return keysOf(false) }

// AlbumKeys returns, sorted, the keys of fields.txt that need more of Library
// than Library.Photos.
func AlbumKeys() []string { return keysOf(true) }

func keysOf(needsAlbums bool) []string {
	var keys []string
	for k, r := range rules {
		if r.needsAlbums == needsAlbums {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Defaults of upstream for a missing date (photos_datetime.py:19-21 and the
// 0 of photosdb.py:1937-1938, which is 2001-01-01 in Core Data seconds).
var (
	defaultDateTimeLocal = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultCreationDate  = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
)

func buildRules() map[string]rule {
	list := []rule{
		// uuid has no Go side: the sets of objects are compared by Compare itself.
		{key: "uuid", ref: func(o Object) (any, error) { return decode[string](o, "uuid", false, "a string") }},
		albumInfoRule("album_info", ownAlbums),
		albumInfoRule("burst_album_info", burstAlbums),
		albumsRule("albums", ownAlbums),
		albumsRule("burst_albums", burstAlbums),
		foldersRule(),
		importInfoRule(),
		projectInfoRule(),
		commentsRule("comments", true, func(e *env) map[string][]photoinfo.CommentInfo { return e.comments },
			func(c photoinfo.CommentInfo) map[string]any {
				return commentElem(c.Datetime, c.User, c.IsMine, &c.Text)
			}),
		commentsRule("likes", false, func(e *env) map[string][]photoinfo.LikeInfo { return e.likes },
			func(l photoinfo.LikeInfo) map[string]any { return commentElem(l.Datetime, l.User, l.IsMine, nil) }),
		personInfoRule(),
		burstPhotosRule(),
		stringList("persons", func(p *photoinfo.PhotoInfo) []string { return p.Persons }),
		stringList("keywords", func(p *photoinfo.PhotoInfo) []string { return p.Keywords }),
		location(),
		orientation(),
		value("incloud", true, "a boolean or null", func(p *photoinfo.PhotoInfo) *bool { return p.InCloud }),
		value("tzoffset", true, "an integer or null", func(p *photoinfo.PhotoInfo) *int { return p.TZOffset }),
		value("latitude", true, "a number or null", func(p *photoinfo.PhotoInfo) *float64 { return p.Latitude }),
		value("longitude", true, "a number or null", func(p *photoinfo.PhotoInfo) *float64 { return p.Longitude }),
		value("adjustment_type", true, "an integer or null", func(p *photoinfo.PhotoInfo) *int { return p.AdjustmentType }),
		importedBy(),

		zonedDate("date", func(p *photoinfo.PhotoInfo) time.Time { return p.Date }),
		zonedDate("date_original", func(p *photoinfo.PhotoInfo) time.Time { return p.DateOriginal }),
		exifInfoRule(),
		scoreRule(),
		placeRule(),
		moment("date_added", defaultDateTimeLocal, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateAdded }),
		moment("date_modified", time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateModified }),
		moment("date_trashed", time.Time{}, func(p *photoinfo.PhotoInfo) *time.Time { return p.DateTrashed }),

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
		flag("hdr", func(p *photoinfo.PhotoInfo) bool { return p.HDR }),
		flag("panorama", func(p *photoinfo.PhotoInfo) bool { return p.Panorama }),
		flag("portrait", func(p *photoinfo.PhotoInfo) bool { return p.Portrait }),
		flag("selfie", func(p *photoinfo.PhotoInfo) bool { return p.Selfie }),
		flag("visible", func(p *photoinfo.PhotoInfo) bool { return p.Visible }),
		flag("shared_moment", func(p *photoinfo.PhotoInfo) bool { return p.SharedMoment }),
		flag("external_edit", func(p *photoinfo.PhotoInfo) bool { return p.ExternalEdit }),

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
		text("tzname", true, func(p *photoinfo.PhotoInfo) string { return p.TZName }),
		text("cloud_owner_hashed_id", true, func(p *photoinfo.PhotoInfo) string { return p.CloudOwnerHashedID }),
		text("owner", true, func(p *photoinfo.PhotoInfo) string { return p.Owner }),

		number("width", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Width) }),
		number("height", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Height) }),
		number("original_width", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalWidth) }),
		number("original_height", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalHeight) }),
		number("original_orientation", func(p *photoinfo.PhotoInfo) int64 { return int64(p.OriginalOrientation) }),
		number("original_filesize", func(p *photoinfo.PhotoInfo) int64 { return p.OriginalFilesize }),
		number("spatial", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Spatial) }),
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

func simple(key string, ref func(Object) (any, error), got func(*photoinfo.PhotoInfo) any) rule {
	return rule{
		key: key,
		ref: ref,
		got: func(_ *env, p *photoinfo.PhotoInfo, _ any) (any, error) { return got(p), nil },
	}
}

func value[T any](key string, nullOK bool, want string, get func(*photoinfo.PhotoInfo) T) rule {
	return simple(key,
		func(o Object) (any, error) { return decode[T](o, key, nullOK, want) },
		func(p *photoinfo.PhotoInfo) any { return get(p) })
}

// A null boolean is a mistake of the form: upstream gives None only for Photos
// before 7, which the reference does not cover.
func flag(key string, get func(*photoinfo.PhotoInfo) bool) rule {
	return value(key, false, "a boolean", get)
}

func text(key string, nullOK bool, get func(*photoinfo.PhotoInfo) string) rule {
	want := "a string"
	if nullOK {
		want += " or null"
	}
	return value(key, nullOK, want, get)
}

// A number written with a fraction or an exponent (3024.0) is not an integer:
// osxphotos writes these keys from integer columns.
func number(key string, get func(*photoinfo.PhotoInfo) int64) rule {
	return value(key, true, "an integer", get)
}

// Upstream gives these dates in the zone of the computer that wrote the
// reference (photos_datetime_local, photosdb/photosdb.py:2157, 2293, 2317), so
// only the moment counts.
func moment(key string, missingDefault time.Time, get func(*photoinfo.PhotoInfo) *time.Time) rule {
	return simple(key,
		func(o Object) (any, error) {
			t, err := refTime(o, key, true)
			if err != nil {
				return nil, err
			}
			return goMoment(t, time.Time{}), nil
		},
		func(p *photoinfo.PhotoInfo) any { return goMoment(get(p), missingDefault) })
}

// Upstream gives date and date_original in the zone of the photo
// (photos_datetime.py:47-67), so the offset counts too.
func zonedDate(key string, get func(*photoinfo.PhotoInfo) time.Time) rule {
	return simple(key,
		func(o Object) (any, error) {
			t, err := refTime(o, key, false)
			if err != nil {
				return nil, err
			}
			return zonedMoment(*t), nil
		},
		func(p *photoinfo.PhotoInfo) any { return zonedMoment(get(p)) })
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
// null (photoinfo.py:1702-1704, 2123); null is nil for a number and a flag and
// "" for a string. date is the pair of zonedDate, nil for null.
func exifInfoRule() rule {
	const key = "exif_info"
	fields := reflect.TypeOf(photoinfo.PhotoInfo{}.ExifInfo)
	return rule{
		key: key,
		ref: func(o Object) (any, error) {
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
		got: func(_ *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			v := reflect.ValueOf(p.ExifInfo)
			out := make(map[string]any, len(exifKeys))
			for i, k := range exifKeys {
				out[k] = v.Field(i).Interface()
			}
			out["date"] = zonedOrNil(p.ExifInfo.Date)
			return out, nil
		},
	}
}

// The keys of score in the order of the fields of scoreinfo.ScoreInfo, which
// the rule reads by position (scoreinfo.py:14-40).
var scoreKeys = []string{
	"overall", "curation", "promotion", "highlight_visibility", "behavioral", "failure", "harmonious_color",
	"immersiveness", "interaction", "interesting_subject", "intrusive_object_presence", "lively_color", "low_light",
	"noise", "pleasant_camera_tilt", "pleasant_composition", "pleasant_lighting", "pleasant_pattern",
	"pleasant_perspective", "pleasant_post_processing", "pleasant_reflection", "pleasant_symmetry",
	"sharply_focused_subject", "tastefully_blurred", "well_chosen_subject", "well_framed_subject", "well_timed_shot",
}

// json() writes the object for every photo, zeros for a photo without a row
// (photoinfo.py:1507-1537, 2128, 2190). A null is 0: upstream keeps a NULL
// column as None and selects null for behavioral and interaction from Photos
// 11 on (_photosdb_process_scoreinfo.py:62-76), where the module has 0.
func scoreRule() rule {
	const key = "score"
	return rule{
		key: key,
		ref: func(o Object) (any, error) {
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
				if out[k], err = decodeRaw[float64](m, path, true, "a number or null"); err != nil {
					return nil, err
				}
			}
			return out, nil
		},
		got: func(_ *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			v := reflect.ValueOf(p.Score)
			out := make(map[string]any, len(scoreKeys))
			for i, k := range scoreKeys {
				out[k] = v.Field(i).Float()
			}
			return out, nil
		},
	}
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

// json() writes {} for a photo without a place (photoinfo.py:2127) and sorts
// only the lists at the top level, so the lists of names keep the order of
// area (2279-2285). null is "" for a string and false for ishome; address is
// an object also without a postal address (placeinfo.py:622-625, 708-716).
func placeRule() rule {
	const key = "place"
	return rule{
		key: key,
		ref: func(o Object) (any, error) {
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
		got: func(_ *env, p *photoinfo.PhotoInfo, _ any) (any, error) { return placeValue(p.Place), nil },
	}
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
// which the test libraries do not have, and gives 0 (photoinfo.py:1390-1404).
func orientation() rule {
	r := number("orientation", func(p *photoinfo.PhotoInfo) int64 { return int64(p.Orientation) })
	r.skip = func(o Object) (bool, error) { return decode[bool](o, "hasadjustments", false, "a boolean") }
	return r
}

// The reference list is sorted by upstream json() (photoinfo.py:2279-2285).
func stringList(key string, get func(*photoinfo.PhotoInfo) []string) rule {
	return simple(key,
		func(o Object) (any, error) { return refStrings(o, key) },
		func(p *photoinfo.PhotoInfo) any { return strs(get(p)) })
}

func location() rule {
	return simple("location",
		func(o Object) (any, error) { return refPair[*float64](o, "location", "a list of two numbers or nulls") },
		func(p *photoinfo.PhotoInfo) any { return []*float64{p.Latitude, p.Longitude} })
}

// photoinfo.py:682-694. json() sorts the pair, None as "" (photoinfo.py:2279-2285),
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
		func(p *photoinfo.PhotoInfo) any {
			return slices.Sorted(slices.Values([]string{p.ImportedByName, p.ImportedByBundleID}))
		})
}

func refPair[T any](o Object, key, want string) ([]T, error) {
	pair, err := decode[[]T](o, key, false, want)
	if err == nil && len(pair) != 2 {
		err = shape(key, want)
	}
	return pair, err
}

// The asdict of CommentInfo and LikeInfo (commentinfo.py:10-32), unsorted as
// upstream json() (photoinfo.py:2284); a date is in the computer's zone
// (_photosdb_process_comments.py:130), so only its moment counts, nil as date_added.
func commentsRule[T any](key string, hasText bool, byPhoto func(*env) map[string][]T, elem func(T) map[string]any) rule {
	return rule{
		key:         key,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
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
				out[i] = commentElem(t, deref(it.User), *it.IsMine, it.Text)
			}
			return out, nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			out := []any{}
			for _, v := range byPhoto(e)[p.UUID] {
				out = append(out, elem(v))
			}
			return out, nil
		},
	}
}

func commentElem(date *time.Time, user string, mine bool, text *string) map[string]any {
	elem := map[string]any{"datetime": goMoment(date, defaultDateTimeLocal), "user": user, "ismine": mine}
	if text != nil {
		elem["text"] = *text
	}
	return elem
}

// The asdict of PersonInfo (personinfo.py:110-123). json() sorts no list of
// objects, and upstream sorts the persons of a photo by name only, equal names
// in the order of SQLite (photoinfo.py:605-611): both sides are compared in
// order, persons of equal name sorted by uuid, and a person stays once per face.
func personInfoRule() rule {
	const key = "person_info"
	return rule{
		key:         key,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
			items, err := decode[[]json.RawMessage](o, key, false, "a list of objects")
			if err != nil {
				return nil, err
			}
			persons := make([]personinfo.PersonInfo, len(items))
			for i, it := range items {
				if persons[i], err = decodePerson(it, fmt.Sprintf("%s[%d]", key, i)); err != nil {
					return nil, err
				}
			}
			return personElems(persons), nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) { return personElems(e.persons[p.UUID]), nil },
	}
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

// The result is never nil: it is written as [], not null. The list of Library
// is not sorted in place: Compare leaves its input as it is.
func personElems(persons []personinfo.PersonInfo) []any {
	sorted := slices.Clone(persons)
	for i := 0; i < len(sorted); {
		j := i + 1
		for j < len(sorted) && sorted[j].Name == sorted[i].Name {
			j++
		}
		slices.SortStableFunc(sorted[i:j], func(a, b personinfo.PersonInfo) int { return strings.Compare(a.UUID, b.UUID) })
		i = j
	}
	out := make([]any, len(sorted))
	for i, p := range sorted {
		out[i] = map[string]any{
			"uuid": p.UUID, "name": p.Name, "displayname": p.DisplayName, "keyface": p.KeyFace, "facecount": p.FaceCount,
			"keyphoto": p.KeyPhotoUUID, "favorite": p.Favorite, "sort_order": p.SortOrder, "feature_less": p.FeatureLess,
		}
	}
	return out
}

// The frames are found among all photos of the library, selected or not,
// trashed or not.
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
		Owner        *string   `json:"owner"`
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
		"owner":        deref(e.Owner),
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
	return refAlbum{uuid: *e.UUID, elem: elem}, nil
}

type albumUUIDs func(*env, *photoinfo.PhotoInfo) []string

func ownAlbums(_ *env, p *photoinfo.PhotoInfo) []string { return p.AlbumUUIDs }

func burstAlbums(e *env, p *photoinfo.PhotoInfo) []string { return e.burstAlbums[p.UUID] }

func albumInfoRule(key string, uuids albumUUIDs) rule {
	return rule{
		key:         key,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
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
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			return e.elemsOf(members(e.albums, errAlbumMissing, uuids(e, p)))
		},
	}
}

// A project is an album without folders (albuminfo.py:368-392) and has the
// keys of an album (photoinfo.py:2228), so the reference side is that of
// album_info. Its parent is not compared: osxphotos writes it only when the
// parent row holds a folder or a user album outside the trash
// (albuminfo.py:296, 525-527), which depends on the other rows under that
// parent, not on the project; TestProjects checks ParentUUID against the
// database. newEnv leaves it out of the Go side.
func projectInfoRule() rule {
	r := albumInfoRule("project_info", nil)
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
	r.got = func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
		return e.elemsOf(members(e.projects, errProjectMissing, p.ProjectUUIDs))
	}
	return r
}

func (e *env) elemsOf(albums []*albuminfo.AlbumInfo, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	byUUID := map[string]map[string]any{}
	for _, a := range albums {
		byUUID[a.UUID] = e.goAlbumElems[a.UUID]
	}
	return sortedElems(byUUID), nil
}

func sortedElems(byUUID map[string]map[string]any) []any {
	out := make([]any, 0, len(byUUID))
	for _, id := range slices.Sorted(maps.Keys(byUUID)) {
		out = append(out, byUUID[id])
	}
	return out
}

func albumsRule(key string, uuids albumUUIDs) rule {
	return rule{
		key:         key,
		needsAlbums: true,
		ref:         func(o Object) (any, error) { return refStrings(o, key) },
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			albums, err := members(e.albums, errAlbumMissing, uuids(e, p))
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

var importElemKeys = []string{"uuid", "title", "creation_date", "start_date", "end_date"}

// A photo without a session is {} (photoinfo.py:2223-2225); a session is the
// asdict of albuminfo.py:350-358.
func importInfoRule() rule {
	const key = "import_info"
	return rule{
		key:         key,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
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
				elem[d] = goMoment(t, time.Time{})
			}
			return elem, nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			if p.ImportUUID == "" {
				return map[string]any{}, nil
			}
			elem, ok := e.goImportElems[p.ImportUUID]
			if !ok {
				return nil, errImportMissing
			}
			return elem, nil
		},
	}
}

func foldersRule() rule {
	return rule{
		key:         "folders",
		needsAlbums: true,
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
		got: func(e *env, p *photoinfo.PhotoInfo, want any) (any, error) {
			albums, err := members(e.albums, errAlbumMissing, p.AlbumUUIDs)
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
