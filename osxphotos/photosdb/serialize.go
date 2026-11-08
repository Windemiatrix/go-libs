package photosdb

import (
	"cmp"
	"maps"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// AsDict returns p as osxphotos PhotoInfo.asdict(shallow) does
// (photoinfo.py:2103-2117, 2119-2253): the keys of 2134-2205, and without
// shallow those of 2209-2251 too. A value is as upstream has it: a date is a
// time.Time, None is nil, also for a string that p holds as "", a list is a
// non-nil slice, also inside search_info, an object is its AsDict or an empty
// map where upstream has {}. rating is 0 (photoinfo.py:822-825) and library is
// Info().LibraryPath; media_analysis and ai_caption are nil: they come from a
// database of another program, which is not read. date_added without a value
// is 1970-01-01T00:00:00Z, where upstream takes midnight in the zone of the
// computer (photos_datetime.py:19-21). In folders an album without a title has
// the key "", where upstream has None. The map is new on every call and holds
// no reference into the library.
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

// photoinfo.py:2212-2214, 2248-2250.
func pyAdjustments(a *adjustmentsinfo.AdjustmentsInfo) map[string]any {
	if a == nil {
		return map[string]any{}
	}
	return a.AsDict()
}
