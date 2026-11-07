package photosdb

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// Fields returns p as a template.Photo: the values of the template fields of
// osxphotos read from p and the library (phototemplate.py:930-1000,
// 1261-1317), on demand, by their upstream name. photo.X gives the property X
// of PhotoInfo by its upstream name (1346-1382): a float as str() of Python, a
// date as isoformat() in the zone of the value, so DateAdded, DateModified and
// DateTrashed in UTC, and a DateAdded the database does not have as
// 1970-01-01T00:00:00+00:00; photo.score.X, photo.exif_info.X and
// photo.place.X give the scalars of those objects. place.X, photo.place.X
// whatever X is, and exif.X give no value for a photo without a place or EXIF
// data, as upstream None (957-966, 1358-1359, 1779-1780). A name the port does not give yet, such as searchinfo.X, label,
// label_normalized or photo.labels, gives false, and so does photo.X of an
// object, a method or a property upstream does not have, where upstream
// renders str() of the object or fails (1360-1363, 1377-1382). The Photo is
// valid as long as db is; the caller may change the values it gives.
func (db *DB) Fields(p *photoinfo.PhotoInfo) template.Photo { return photoFields{db, p} }

// RenderTemplate renders t for p with one Renderer of the library, as
// osxphotos PhotoInfo.render_template does (photoinfo.py:1866-1880): {counter}
// grows from photo to photo for as long as db lives; a caller that needs to
// reset it renders with its own template.Renderer and Fields(p). t is parsed
// once by the caller, where upstream parses the string on each call. The
// results and the errors are those of template.Renderer.Render.
func (db *DB) RenderTemplate(p *photoinfo.PhotoInfo, t *template.Template, o template.RenderOptions) (values, unmatched []string, err error) {
	return db.renderer.Render(t, db.Fields(p), o)
}

type photoFields struct {
	db *DB
	p  *photoinfo.PhotoInfo
}

func (f photoFields) Field(name string) (template.Value, bool) {
	db, p := f.db, f.p
	if property, ok := strings.CutPrefix(name, "photo."); ok {
		return photoProperty(db, p, property)
	}
	if field, ok := strings.CutPrefix(name, "place."); ok {
		return placeField(p, field)
	}
	switch name {
	case "name":
		return singleValue(p.Filename), true
	case "original_name":
		return singleValue(p.OriginalFilename), true
	case "title":
		return singleValue(p.Title), true
	case "descr", "caption":
		return singleValue(p.Description), true
	case "media_type":
		return singleValue(mediaKindOf(p)), true
	case "photo_or_video": // 1547-1550
		if p.IsPhoto {
			return singleValue("photo"), true
		}
		return singleValue("video"), true
	case "hdr":
		return flagValue(p.HDR, name), true
	case "edited":
		return flagValue(p.HasAdjustments, name), true
	case "favorite":
		return flagValue(p.Favorite, name), true
	case "created":
		d := p.Date
		return template.Value{Time: &d}, true
	case "modified":
		if p.DateModified == nil {
			return template.Value{}, true
		}
		d := *p.DateModified
		return template.Value{Time: &d}, true
	case "uuid":
		return singleValue(p.UUID), true
	case "id":
		pk := p.PK
		return template.Value{Int: &pk}, true
	case "imported_by.name":
		return singleValue(p.ImportedByName), true
	case "imported_by.id":
		return singleValue(p.ImportedByBundleID), true
	case "exif.camera_make", "exif.camera_model", "exif.lens_model": // 961-966
		return nestedProperty(p, "exif_info", strings.TrimPrefix(name, "exif."))
	case "moment": // 971-972
		m, _ := db.MomentInfoOf(p)
		return singleValue(m.Title), true
	case "burst": // 995-1000
		if key, ok := db.BurstKeyPhoto(p); ok {
			return singleValue(key.OriginalFilename), true
		}
		return template.Value{}, true
	// burst_albums and burst_album_info are albums and album_info for a photo
	// outside a burst (photoinfo.py:646-671), so the Burst*Of methods stand for
	// both branches of phototemplate.py:1262-1266, 1270, 1290-1293.
	case "album":
		return template.Value{Strings: db.BurstAlbumTitlesOf(p)}, true
	case "project":
		return template.Value{Strings: projectTitles(db.ProjectInfoOf(p))}, true
	case "album_project": // upstream also appends the projects to the cached albums of a photo outside a burst (1270-1271)
		v := slices.Concat(db.BurstAlbumTitlesOf(p), projectTitles(db.ProjectInfoOf(p)))
		slices.Sort(v)
		return template.Value{Strings: v}, true
	case "keyword":
		return multiValue(p.Keywords), true
	case "person":
		return multiValue(p.Persons), true
	case "comment": // f"{comment.user}: …" prints the user None as "None" (1314-1317)
		var v []string
		for _, c := range db.Comments(p) {
			v = append(v, cmp.Or(c.User, "None")+": "+c.Text)
		}
		slices.Sort(v)
		return template.Value{Strings: v}, true
	case "folder_album":
		return albumRefsOf(db.BurstAlbumInfoOf(p)), true
	case "folder_album_project":
		return albumRefsOf(slices.Concat(db.BurstAlbumInfoOf(p), db.ProjectInfoOf(p))), true
	case "album_seq": // album_info, not the albums of the burst (983, 985)
		albums := db.AlbumInfoOf(p)
		v := albumRefsOf(albums)
		for i, a := range albums {
			v.Albums[i].Index, _ = db.PhotoIndex(a, p) // -1 is the ValueError of photo_index (albuminfo.py:285-287)
		}
		return v, true
	}
	return template.Value{}, false
}

// {photo.X}: phototemplate.py:1346-1382.
func photoProperty(db *DB, p *photoinfo.PhotoInfo, name string) (template.Value, bool) {
	if object, property, ok := strings.Cut(name, "."); ok {
		return nestedProperty(p, object, property)
	}
	flags := map[string]bool{
		"burst": p.Burst, "burst_default_pick": p.BurstDefaultPick, "burst_key": p.BurstKey, "burst_selected": p.BurstSelected,
		"external_edit": p.ExternalEdit, "favorite": p.Favorite, "has_raw": p.HasRAW, "hasadjustments": p.HasAdjustments,
		"hdr": p.HDR, "hidden": p.Hidden, "incloud": p.InCloud != nil && *p.InCloud, "intrash": p.InTrash,
		"iscloudasset": p.IsCloudAsset, "ismissing": p.IsMissing, "ismovie": p.IsMovie, "isphoto": p.IsPhoto,
		"israw": p.IsRAW, "isreference": p.IsReference, "live_photo": p.Live, "panorama": p.Panorama,
		"portrait": p.Portrait, "raw_original": p.RawOriginal, "saved_to_library": p.SavedToLibrary,
		"screen_recording": p.ScreenRecording, "screenshot": p.Screenshot, "selfie": p.Selfie, "shared": p.Shared,
		"shared_library": p.SharedLibrary, "shared_moment": p.SharedMoment, "slow_mo": p.SlowMo,
		"syndicated": p.Syndicated, "time_lapse": p.TimeLapse, "visible": p.Visible, "flagged": false, // flagged: iPhoto only (photoinfo.py:817-820)
	}
	if on, ok := flags[name]; ok {
		return flagValue(on, name), true
	}
	scalars := map[string]string{
		"uuid": p.UUID, "filename": p.Filename, "original_filename": p.OriginalFilename, "title": p.Title,
		"description": p.Description, "tzname": p.TZName, "uti": p.UTI, "uti_original": p.UTIOriginal, "uti_edited": p.UTIEdited,
		"uti_raw": p.UTIRaw, "cloud_guid": p.CloudMasterGUID, "cloud_owner_hashed_id": p.CloudOwnerHashedID,
		"fingerprint": p.Fingerprint, "owner": p.Owner, "path": p.Path, "path_edited": p.PathEdited,
		"path_live_photo": p.PathLive, "path_raw": p.PathRaw, "path_edited_live_photo": p.PathEditedLive,
		// pathlib.Path, whose str() is the path (photoinfo.py:743-781)
		"adjustments_path": p.AdjustmentsPath, "original_adjustments_path": p.OriginalAdjustmentsPath,
		"height": strconv.Itoa(p.Height), "width": strconv.Itoa(p.Width), "orientation": strconv.Itoa(p.Orientation),
		"original_height": strconv.Itoa(p.OriginalHeight), "original_width": strconv.Itoa(p.OriginalWidth),
		"original_orientation": strconv.Itoa(p.OriginalOrientation), "original_filesize": strconv.FormatInt(p.OriginalFilesize, 10),
		"spatial": strconv.Itoa(p.Spatial), "rating": "0", // rating: iPhoto only (photoinfo.py:822-825)
		"tzoffset": formatOrNone(p.TZOffset, strconv.Itoa), "adjustment_type": formatOrNone(p.AdjustmentType, strconv.Itoa),
		"latitude": formatOrNone(p.Latitude, template.FloatString), "longitude": formatOrNone(p.Longitude, template.FloatString),
		"date": isoformat(p.Date), "date_original": isoformat(p.DateOriginal), "date_modified": formatOrNone(p.DateModified, isoformat),
		// DEFAULT_DATETIME_LOCAL for a date_added the database does not have (photosdb/photosdb.py:2317, photos_datetime.py:77-78)
		"date_added": isoformat(*cmp.Or(p.DateAdded, &epoch)), "date_trashed": formatOrNone(p.DateTrashed, isoformat),
	}
	if s, ok := scalars[name]; ok {
		return singleValue(s), true
	}
	switch name {
	case "albums":
		return noneDropped(db.AlbumTitlesOf(p)...), true
	case "burst_albums":
		return noneDropped(db.BurstAlbumTitlesOf(p)...), true
	case "keywords":
		return multiValue(p.Keywords), true
	case "persons":
		return multiValue(p.Persons), true
	case "path_derivatives":
		return multiValue(p.PathDerivatives), true
	case "location":
		return noneDropped(scalars["latitude"], scalars["longitude"]), true
	case "imported_by":
		return noneDropped(p.ImportedByName, p.ImportedByBundleID), true
	}
	return template.Value{}, false
}

// scoreinfo.py:14-40, exifinfo.py:18-39, placeinfo.py:583-601. Without a place
// the chain of getattr stops at None, whatever follows
// (phototemplate.py:1358-1359).
func nestedProperty(p *photoinfo.PhotoInfo, object, name string) (template.Value, bool) {
	var values map[string]string
	switch object {
	case "score":
		s, f := p.Score, template.FloatString
		values = map[string]string{
			"overall": f(s.Overall), "curation": f(s.Curation), "promotion": f(s.Promotion),
			"highlight_visibility": f(s.HighlightVisibility), "behavioral": f(s.Behavioral), "failure": f(s.Failure),
			"harmonious_color": f(s.HarmoniousColor), "immersiveness": f(s.Immersiveness), "interaction": f(s.Interaction),
			"interesting_subject": f(s.InterestingSubject), "intrusive_object_presence": f(s.IntrusiveObjectPresence),
			"lively_color": f(s.LivelyColor), "low_light": f(s.LowLight), "noise": f(s.Noise),
			"pleasant_camera_tilt": f(s.PleasantCameraTilt), "pleasant_composition": f(s.PleasantComposition),
			"pleasant_lighting": f(s.PleasantLighting), "pleasant_pattern": f(s.PleasantPattern),
			"pleasant_perspective": f(s.PleasantPerspective), "pleasant_post_processing": f(s.PleasantPostProcessing),
			"pleasant_reflection": f(s.PleasantReflection), "pleasant_symmetry": f(s.PleasantSymmetry),
			"sharply_focused_subject": f(s.SharplyFocusedSubject), "tastefully_blurred": f(s.TastefullyBlurred),
			"well_chosen_subject": f(s.WellChosenSubject), "well_framed_subject": f(s.WellFramedSubject),
			"well_timed_shot": f(s.WellTimedShot),
		}
	case "exif_info":
		e := p.ExifInfo
		whole := func(v *int) string { return formatOrNone(v, strconv.Itoa) }
		num := func(v *float64) string { return formatOrNone(v, template.FloatString) }
		values = map[string]string{
			"iso": whole(e.ISO), "metering_mode": whole(e.MeteringMode), "sample_rate": whole(e.SampleRate),
			"track_format": whole(e.TrackFormat), "white_balance": whole(e.WhiteBalance), "tzoffset": whole(e.TZOffset),
			"aperture": num(e.Aperture), "bit_rate": num(e.BitRate), "duration": num(e.Duration),
			"exposure_bias": num(e.ExposureBias), "focal_length": num(e.FocalLength), "fps": num(e.FPS),
			"latitude": num(e.Latitude), "longitude": num(e.Longitude), "shutter_speed": num(e.ShutterSpeed),
			"camera_make": e.CameraMake, "camera_model": e.CameraModel, "codec": e.Codec, "lens_model": e.LensModel,
			"flash_fired": flagName(e.FlashFired != nil && *e.FlashFired, "flash_fired"), "date": formatOrNone(e.Date, isoformat), "tzname": e.TZName,
		}
	case "place":
		if p.Place == nil {
			return template.Value{}, true
		}
		pl := p.Place
		values = map[string]string{"name": pl.Name, "country_code": pl.CountryCode, "address_str": pl.AddressStr, "ishome": flagName(pl.IsHome, "ishome")}
	}
	s, ok := values[name]
	return singleValue(s), ok
}

// get_place_value (phototemplate.py:1772-1819)
func placeField(p *photoinfo.PhotoInfo, name string) (template.Value, bool) {
	pl := cmp.Or(p.Place, &placeinfo.PlaceInfo{})
	s, ok := map[string]string{
		"name": pl.Name, "country_code": pl.CountryCode, "address": pl.AddressStr,
		"name.country": firstOf(pl.Names.Country), "name.state_province": firstOf(pl.Names.StateProvince),
		"name.city": firstOf(pl.Names.City), "name.area_of_interest": firstOf(pl.Names.AreaOfInterest),
		"address.street": pl.Address.Street, "address.city": pl.Address.City, "address.state_province": pl.Address.StateProvince,
		"address.postal_code": pl.Address.PostalCode, "address.country": pl.Address.Country,
		"address.country_code": pl.Address.ISOCountryCode,
	}[name]
	return singleValue(s), ok
}

// phototemplate.py:1790-1803
func firstOf(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// nil is None, which gives no value (phototemplate.py:1365-1366), as "" does
// for singleValue.
func formatOrNone[T any](v *T, format func(T) string) string {
	if v == nil {
		return ""
	}
	return format(*v)
}

// None in a list is dropped (phototemplate.py:1376, 663); PhotoInfo holds it as "".
func noneDropped(s ...string) template.Value {
	return template.Value{Strings: slices.DeleteFunc(s, func(v string) bool { return v == "" })}
}

// datetime.isoformat() of an aware datetime (phototemplate.py:1371-1372), in
// the zone of t. Python prints the seconds of an offset, which a zone of the
// photo may have and the layout -07:00 of time drops.
func isoformat(t time.Time) string {
	layout := "2006-01-02T15:04:05"
	if t.Nanosecond() != 0 {
		layout += ".000000"
	}
	layout += "-07:00"
	if _, offset := t.Zone(); offset%60 != 0 {
		layout += ":00"
	}
	return t.Format(layout)
}

// get_media_type (phototemplate.py:1552-1577) tests the flags in this order.
func mediaKindOf(p *photoinfo.PhotoInfo) string {
	switch {
	case p.Selfie:
		return "selfie"
	case p.TimeLapse:
		return "time_lapse"
	case p.Panorama:
		return "panorama"
	case p.SlowMo:
		return "slow_mo"
	case p.Screenshot:
		return "screenshot"
	case p.ScreenRecording:
		return "screen_recording"
	case p.Portrait:
		return "portrait"
	case p.Live:
		return "live_photo"
	case p.Burst:
		return "burst"
	case p.IsMovie:
		return "video"
	}
	return "photo"
}

// A single-valued field never holds "" (phototemplate.py:1012).
func singleValue(s string) template.Value {
	if s == "" {
		return template.Value{}
	}
	return template.Value{Strings: []string{s}}
}

// The caller of Fields may change the values: the slices of p stay out of its reach.
func multiValue(s []string) template.Value { return template.Value{Strings: slices.Clone(s)} }

// phototemplate.py:942-949
func flagValue(on bool, name string) template.Value { return singleValue(flagName(on, name)) }

// phototemplate.py:1367-1368
func flagName(on bool, name string) string {
	if on {
		return name
	}
	return ""
}

// Upstream keeps a repeated project title (phototemplate.py:1268).
func projectTitles(projects []*albuminfo.AlbumInfo) []string {
	var out []string
	for _, a := range projects {
		out = append(out, a.Title)
	}
	return out
}

func albumRefsOf(albums []*albuminfo.AlbumInfo) template.Value {
	var refs []template.AlbumRef
	for _, a := range albums {
		refs = append(refs, template.AlbumRef{Folders: slices.Clone(a.FolderNames), Title: a.Title})
	}
	return template.Value{Albums: refs}
}
