package photosdb

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// AsDict returns p as osxphotos PhotoInfo.asdict(shallow) does
// (photoinfo.py:2101-2115, 2117-2251): the keys of 2132-2203, and without
// shallow those of 2207-2249 too. A value is as upstream has it: a date is a
// time.Time, None is nil, also for a string that p holds as "", a list is a
// non-nil slice, also inside search_info, an object is its AsDict or an empty
// map where upstream has {}. rating is 0 (photoinfo.py:822-825) and library is
// Info().LibraryPath; media_analysis and ai_caption are nil: they come from a
// database of another program, which is not read. date_added without a value
// is 1970-01-01T00:00:00Z, where upstream takes midnight in the zone of the
// computer (photos_datetime.py:19-21). date_added, date_modified and
// date_trashed, and the dates of albums and of the import session, are in UTC,
// where upstream has the zone of the computer (photosdb/photosdb.py:2161, 2297,
// 2321; albuminfo.py:76-107). In folders an album without a title has the key "",
// where upstream has None. The map is new on every call and holds no reference
// into the library.
func (db *DB) AsDict(p *photoinfo.PhotoInfo, shallow bool) map[string]any {
	albums := db.AlbumInfoOf(p)
	folders := map[string]any{}
	for _, a := range albums {
		folders[a.Title] = pyList(a.FolderNames)
	}
	place := map[string]any{}
	if p.Place != nil {
		place = p.Place.AsDict()
	}
	search := db.SearchInfoOf(p)
	faces := pyDicts(db.FaceInfoOf(p), func(f personinfo.FaceInfo) map[string]any { return f.AsDict(p.Width, p.Height, p.Orientation) })
	d := map[string]any{
		"albums": pyTitles(db.AlbumTitlesOf(p)), "burst": p.Burst, "cloud_guid": pyNone(p.CloudMasterGUID),
		"cloud_owner_hashed_id": pyNone(p.CloudOwnerHashedID), "comments": pyDicts(db.Comments(p), photoinfo.CommentInfo.AsDict),
		"date_added": *cmp.Or(p.DateAdded, &epoch), "date_modified": pyNoneOf(p.DateModified), "date_trashed": pyNoneOf(p.DateTrashed),
		"date": p.Date, "description": pyNone(p.Description), "exif_info": p.ExifInfo.AsDict(), "external_edit": p.ExternalEdit,
		"face_info": faces, "favorite": p.Favorite, "filename": p.Filename, "fingerprint": pyNone(p.Fingerprint), "folders": folders,
		"has_raw": p.HasRAW, "hasadjustments": p.HasAdjustments, "hdr": p.HDR, "height": p.Height, "hidden": p.Hidden,
		"incloud": pyNoneOf(p.InCloud), "intrash": p.InTrash, "iscloudasset": p.IsCloudAsset, "ismissing": p.IsMissing,
		"ismovie": p.IsMovie, "isphoto": p.IsPhoto, "israw": p.IsRAW, "isreference": p.IsReference,
		"keywords": pyList(p.Keywords), "labels": pyList(search.Labels()), "latitude": pyNoneOf(p.Latitude),
		"library": db.info.LibraryPath, "likes": pyDicts(db.Likes(p), photoinfo.LikeInfo.AsDict), "live_photo": p.Live,
		"location": []any{pyNoneOf(p.Latitude), pyNoneOf(p.Longitude)}, "longitude": pyNoneOf(p.Longitude),
		"orientation": p.Orientation, "original_filename": p.OriginalFilename, "original_filesize": p.OriginalFilesize,
		"original_height": p.OriginalHeight, "original_orientation": p.OriginalOrientation, "original_width": p.OriginalWidth,
		"owner": pyNone(p.Owner), "panorama": p.Panorama, "path_edited_live_photo": pyNone(p.PathEditedLive),
		"path_edited": pyNone(p.PathEdited), "path_live_photo": pyNone(p.PathLive), "path_raw": pyNone(p.PathRaw),
		"path": pyNone(p.Path), "persons": pyList(p.Persons), "place": place, "portrait": p.Portrait,
		"raw_original": p.RawOriginal, "score": p.Score.AsDict(), "screenshot": p.Screenshot, "selfie": p.Selfie,
		"shared": p.Shared, "slow_mo": p.SlowMo, "time_lapse": p.TimeLapse, "title": pyNone(p.Title),
		"tzoffset": pyNoneOf(p.TZOffset), "uti_edited": pyNone(p.UTIEdited), "uti_original": p.UTIOriginal,
		"uti_raw": pyNone(p.UTIRaw), "uti": p.UTI, "uuid": p.UUID, "visible": p.Visible, "width": p.Width,
	}
	if shallow {
		return d
	}
	importInfo := map[string]any{}
	if s, ok := db.Import(p.ImportUUID); ok {
		importInfo = s.AsDict()
	}
	burst := []string{}
	for _, f := range db.BurstPhotos(p) {
		burst = append(burst, f.UUID)
	}
	cloud := db.CloudMetadataOf(p)
	if cloud == nil {
		cloud = map[string]any{}
	}
	maps.Copy(d, map[string]any{
		"album_info": pyDicts(albums, (*albuminfo.AlbumInfo).AsDict), "path_derivatives": pyList(p.PathDerivatives),
		"adjustments": pyAdjustments(db.Adjustments(p)), "adjustment_type": pyNoneOf(p.AdjustmentType),
		"burst_album_info": pyDicts(db.BurstAlbumInfoOf(p), (*albuminfo.AlbumInfo).AsDict),
		"burst_albums":     pyTitles(db.BurstAlbumTitlesOf(p)), "burst_default_pick": p.BurstDefaultPick, "burst_key": p.BurstKey,
		"burst_photos": burst, "burst_selected": p.BurstSelected, "cloud_metadata": cloud, "import_info": importInfo,
		"labels_normalized": pyList(search.Normalized().Labels()),
		"person_info":       pyDicts(db.PersonInfoOf(p), personinfo.PersonInfo.AsDict),
		"project_info":      pyDicts(db.ProjectInfoOf(p), (*albuminfo.AlbumInfo).AsDict),
		"search_info":       pyLists(search.AsDict()), "search_info_normalized": pyLists(search.Normalized().AsDict()),
		"syndicated": p.Syndicated, "saved_to_library": p.SavedToLibrary, "shared_moment": p.SharedMoment,
		"shared_library": p.SharedLibrary, "rating": 0, "screen_recording": p.ScreenRecording, "spatial": p.Spatial,
		"date_original": p.DateOriginal, "tzname": pyNone(p.TZName), "media_analysis": nil, "ai_caption": nil,
		"original_adjustments": pyAdjustments(db.OriginalAdjustments(p)),
		"imported_by":          []any{pyNone(p.ImportedByName), pyNone(p.ImportedByBundleID)},
	})
	return d
}

// PhotoInfo holds None of upstream as "" or a nil pointer; a nil pointer in an
// any is not nil, which None is.
func pyNone(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func pyNoneOf[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// json writes a nil slice as null, where upstream writes the empty list as [].
func pyList(s []string) []string { return append([]string{}, s...) }

// SearchInfo.AsDict gives an empty list as nil, where upstream has [].
func pyLists(d map[string]any) map[string]any {
	for k, v := range d {
		if l, ok := v.([]string); ok {
			d[k] = pyList(l)
		}
	}
	return d
}

// The title of an album without one is None upstream (photoinfo.py:640-643).
func pyTitles(titles []string) []any {
	out := make([]any, len(titles))
	for i, t := range titles {
		out[i] = pyNone(t)
	}
	return out
}

func pyDicts[T any](items []T, asDict func(T) map[string]any) []any {
	out := make([]any, len(items))
	for i, v := range items {
		out[i] = asDict(v)
	}
	return out
}

// photoinfo.py:2210-2212, 2246-2248.
func pyAdjustments(a *adjustmentsinfo.AdjustmentsInfo) map[string]any {
	if a == nil {
		return map[string]any{}
	}
	return a.AsDict()
}

// JSON returns the bytes of osxphotos PhotoInfo.json(indent, shallow)
// (photoinfo.py:2253-2288) for AsDict(p, shallow). The lists at the top level
// are sorted, None as "", except location and lists of maps (2277-2283), and
// written as json.dumps(sort_keys=True, indent=indent) with ensure_ascii writes
// them (2284-2286); indent 0 or below is None of upstream. A float is its
// repr() or NaN, Infinity, -Infinity, a date its isoformat(), naive in UTC in
// cloud_metadata as plistlib gives it; bytes, a UID and the sort order of an
// album are null, as the default of json() returns None for them (2267-2269).
// The bytes hold paths of the machine and personal data (names, comments,
// places): do not log them. JSON panics on a value of a type that AsDict and
// CloudMetadataOf do not give, a mistake of the port. The bytes equal those of
// upstream only where AsDict equals asdict (see AsDict).
func (db *DB) JSON(p *photoinfo.PhotoInfo, shallow bool, indent int) []byte {
	return dumps(db.AsDict(p, shallow), indent)
}

// Hexdigest is osxphotos PhotoInfo.hexdigest (photoinfo.py:1731-1735,
// 2294-2318): the BLAKE2b-160 hex digest of JSON(p, true, 0) without
// face_info, a list of objects whose order upstream does not fix, and visible,
// which follows the state of the Photos app (2297-2300). It equals the digest
// of upstream only where AsDict equals asdict (see AsDict): not when upstream
// opens the library by another path or the computer is not in UTC.
func (db *DB) Hexdigest(p *photoinfo.PhotoInfo) string {
	d := db.AsDict(p, true)
	delete(d, "face_info")
	delete(d, "visible")
	return hexdigest(string(dumps(d, 0)))
}

// dumps is the body of json and _json_hexdigest (photoinfo.py:2277-2286,
// 2311-2318). It sorts the lists of d in place: AsDict makes them for the call.
func dumps(d map[string]any, indent int) []byte {
	for k, v := range d {
		switch l := v.(type) {
		case []string:
			slices.Sort(l)
		case []any:
			if k == "location" || len(l) == 0 {
				continue
			}
			if _, isDict := l[0].(map[string]any); !isDict {
				slices.SortStableFunc(l, func(a, b any) int {
					x, _ := a.(string)
					y, _ := b.(string)
					return strings.Compare(x, y)
				})
			}
		}
	}
	naiveDates(d["cloud_metadata"])
	return pyJSON(d, indent)
}

// plistlib gives a date of a property list as a naive datetime, rounded to the
// microsecond as timedelta rounds, which isoformat writes without an offset;
// the other dates of asdict are aware.
func naiveDates(v any) any {
	switch x := v.(type) {
	case time.Time:
		return strings.TrimSuffix(isoformat(x.UTC().Round(time.Microsecond)), "+00:00")
	case []any:
		for i, e := range x {
			x[i] = naiveDates(e)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = naiveDates(e)
		}
	}
	return v
}

// pyJSON is json.dumps(v, sort_keys=True, indent=indent) of Python
// (json/encoder.py) for the types of AsDict and CloudMetadataOf. Any other
// type is a mistake of the port, not of the data, so it panics.
func pyJSON(v any, indent int) []byte {
	var b bytes.Buffer
	writeJSON(&b, v, indent, 0)
	return b.Bytes()
}

func writeJSON(b *bytes.Buffer, v any, indent, level int) {
	switch x := v.(type) {
	case nil, []byte, plist.UID, albuminfo.AlbumSortOrder: // default returns None (photoinfo.py:2267-2269); AlbumSortOrder is an Enum
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case uint64:
		b.WriteString(strconv.FormatUint(x, 10))
	case float32: // a real of four bytes of a property list, a double for Python
		b.WriteString(jsonFloat(float64(x)))
	case float64:
		b.WriteString(jsonFloat(x))
	case json.Number: // json.loads gives an int of any size for a literal without a fraction or an exponent (adjustmentsinfo.py:94)
		if i, ok := new(big.Int).SetString(string(x), 10); ok {
			b.WriteString(i.String())
		} else {
			f, _ := x.Float64() // out of range is an infinity, as float() of Python gives
			b.WriteString(jsonFloat(f))
		}
	case string:
		writeString(b, x)
	case time.Time:
		writeString(b, isoformat(x))
	case []string:
		writeItems(b, '[', ']', len(x), indent, level, func(i int) { writeString(b, x[i]) })
	case []any:
		writeItems(b, '[', ']', len(x), indent, level, func(i int) { writeJSON(b, x[i], indent, level+1) })
	case map[string]any:
		keys := slices.Sorted(maps.Keys(x))
		writeItems(b, '{', '}', len(keys), indent, level, func(i int) {
			writeString(b, keys[i])
			b.WriteString(": ")
			writeJSON(b, x[keys[i]], indent, level+1)
		})
	default:
		panic(fmt.Sprintf("photosdb: json has no form for a %T", v)) // the type only: a value may be personal data
	}
}

// _iterencode_list and _iterencode_dict of json/encoder.py: an empty one has
// no newline.
func writeItems(b *bytes.Buffer, left, right byte, n, indent, level int, item func(int)) {
	head, sep, tail := "", ", ", ""
	if indent > 0 && n > 0 {
		tail = "\n" + strings.Repeat(" ", indent*level)
		head = tail + strings.Repeat(" ", indent)
		sep = "," + head
	}
	b.WriteByte(left)
	b.WriteString(head)
	for i := range n {
		if i > 0 {
			b.WriteString(sep)
		}
		item(i)
	}
	b.WriteString(tail)
	b.WriteByte(right)
}

// floatstr of json/encoder.py: repr() but for NaN and the infinities.
func jsonFloat(f float64) string {
	s := template.FloatString(f)
	return cmp.Or(jsonNonFinite[s], s)
}

var jsonNonFinite = map[string]string{"nan": "NaN", "inf": "Infinity", "-inf": "-Infinity"}

// py_encode_basestring_ascii of json/encoder.py: hex digits in lower case, a
// character beyond the BMP as a surrogate pair.
func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		if e, ok := jsonEscapes[r]; ok {
			b.WriteString(e)
		} else if r >= ' ' && r <= '~' {
			b.WriteRune(r)
		} else {
			for _, u := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(b, "\\u%04x", u)
			}
		}
	}
	b.WriteByte('"')
}

var jsonEscapes = map[rune]string{'"': `\"`, '\\': `\\`, '\n': `\n`, '\r': `\r`, '\t': `\t`, '\b': `\b`, '\f': `\f`}
