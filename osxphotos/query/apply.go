package query

import (
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// photo keeps each predicate of filters on one line.
type photo = photoinfo.PhotoInfo

// Apply returns the photos that o selects, as photo_query does over a PhotosDB
// (photoquery.py:417-885). photos is every photo of a library (DB.Photos()),
// albums its user and shared albums; albums in the trash are ignored.
// The result is new, in the order of Sort (reversed with
// NewestFirst), each photo once unless a Function returns it twice and
// BurstPhotos is off; its elements are the pointers of photos. photos and
// albums are not modified.
func Apply(o Options, photos []*photoinfo.PhotoInfo, albums []*albuminfo.AlbumInfo) []*photoinfo.PhotoInfo {
	before, after := filters(o, newPhotoAlbums(albums))
	var out []*photo
	for _, p := range photos {
		if inSelection(o, p) && passes(before, p) {
			out = append(out, p)
		}
	}
	if o.Duplicate {
		out = withDuplicates(out, photos, o)
	}
	out = slices.DeleteFunc(out, func(p *photo) bool { return !passes(after, p) })
	for _, f := range o.Function {
		out = f(out)
	}
	if o.BurstPhotos { // after the functions, as photoquery.py:863 (ref #640)
		out = withBursts(out, photos, o.MissingBursts)
	}
	slices.SortStableFunc(out, comparePhotos)
	if o.NewestFirst { // the keys of Sort are unique, so this is sorted(reverse=True)
		slices.Reverse(out)
	}
	return out
}

// photo_query starts from the photos() calls (photoquery.py:426-445;
// photosdb/photosdb.py:3268-3360).
func inSelection(o Options, p *photo) bool {
	return inTrashSelection(o, p) && !burstUnselected(p) &&
		((p.IsPhoto && !o.OnlyMovies) || (p.IsMovie && !o.OnlyPhotos)) &&
		(len(o.UUID) == 0 || slices.Contains(o.UUID, p.UUID)) &&
		(o.FromDate.IsZero() || !p.Date.Before(o.FromDate)) &&
		(o.ToDate.IsZero() || p.Date.Before(o.ToDate))
}

// photosdb.py:3347-3351.
func burstUnselected(p *photo) bool {
	return p.Burst && p.BurstPickType != 0 && p.BurstPickType&(photoinfo.BurstPickSelected|photoinfo.BurstPickKey) == 0
}

func inTrashSelection(o Options, p *photo) bool {
	switch {
	case o.DeletedOnly:
		return p.InTrash
	case o.Deleted:
		return true
	default:
		return !p.InTrash
	}
}

type filter func(*photo) bool

func passes(fs []filter, p *photo) bool {
	return !slices.ContainsFunc(fs, func(f filter) bool { return !f(p) })
}

// The conditions are in the order of photo_query (photoquery.py:447-857),
// split where the duplicate step runs (photoquery.py:754-771). A pair X/NotX
// is "if X … elif NotX": X wins.
func filters(o Options, in photoAlbums) (before, after []filter) {
	fold := caseFold(o.IgnoreCase)
	var fs []filter
	when := func(set bool, f filter) {
		if set {
			fs = append(fs, f)
		}
	}
	pair := func(x, notX bool, has filter) {
		when(x, has)
		when(!x && notX, func(p *photo) bool { return !has(p) })
	}
	text := func(values []string, none bool, field func(*photo) string) {
		when(len(values) > 0, func(p *photo) bool { return field(p) != "" && containsAny(fold.apply(field(p)), values) })
		when(len(values) == 0 && none, func(p *photo) bool { return field(p) == "" })
	}
	albums, folders := parseAlbumPaths(o.Album, fold), parseAlbumPaths(o.Folder, fold)
	keywords, persons, names := searchValues(o.Keyword, fold), searchValues(o.Person, fold), searchValues(o.Name, fold)

	when(len(albums.paths) > 0, func(p *photo) bool { return in.has(p, albums.namesAlbum) })
	when(len(keywords) > 0, func(p *photo) bool { return fold.anyEqual(p.Keywords, keywords) })
	when(len(keywords) == 0 && o.NoKeyword, func(p *photo) bool { return len(p.Keywords) == 0 })
	when(len(persons) > 0, func(p *photo) bool { return fold.anyEqual(p.Persons, persons) })
	when(len(folders.paths) > 0, func(p *photo) bool { return in.has(p, folders.enclosesAlbum) })
	text(searchValues(o.Title, fold), o.NoTitle, func(p *photo) string { return p.Title })
	text(searchValues(o.Description, fold), o.NoDescription, func(p *photo) string { return p.Description })
	pair(o.Edited, o.NotEdited, func(p *photo) bool { return p.HasAdjustments })
	when(o.ExternalEdit, func(p *photo) bool { return p.ExternalEdit })
	pair(o.Favorite, o.NotFavorite, func(p *photo) bool { return p.Favorite })
	pair(o.Hidden, o.NotHidden, func(p *photo) bool { return p.Hidden })
	pair(o.Missing, o.NotMissing, func(p *photo) bool { return p.Path == "" })
	pair(o.Shared, o.NotShared, func(p *photo) bool { return p.Shared })
	when(o.UTI != "", func(p *photo) bool { return strings.Contains(p.UTIOriginal, o.UTI) })
	pair(o.Burst, o.NotBurst, func(p *photo) bool { return p.Burst })
	pair(o.Live, o.NotLive, func(p *photo) bool { return p.Live })
	pair(o.Portrait, o.NotPortrait, func(p *photo) bool { return p.Portrait })
	pair(o.Spatial, o.NotSpatial, func(p *photo) bool { return p.Spatial != 0 })
	pair(o.Screenshot, o.NotScreenshot, func(p *photo) bool { return p.Screenshot })
	pair(o.ScreenRecording, o.NotScreenRecording, func(p *photo) bool { return p.ScreenRecording })
	pair(o.SlowMo, o.NotSlowMo, func(p *photo) bool { return p.SlowMo })
	pair(o.TimeLapse, o.NotTimeLapse, func(p *photo) bool { return p.TimeLapse })
	pair(o.HDR, o.NotHDR, func(p *photo) bool { return p.HDR })
	pair(o.Selfie, o.NotSelfie, func(p *photo) bool { return p.Selfie })
	pair(o.Panorama, o.NotPanorama, func(p *photo) bool { return p.Panorama })
	pair(o.CloudAsset, o.NotCloudAsset, func(p *photo) bool { return p.IsCloudAsset })
	pair(o.InCloud, o.NotInCloud, func(p *photo) bool { return p.InCloud != nil && *p.InCloud })
	when(o.InCloud || o.NotInCloud, func(p *photo) bool { return !p.Shared }) // photoquery.py:648-651
	when(o.HasRAW, func(p *photo) bool { return p.HasRAW })
	pair(o.IsReference, o.NotReference, func(p *photo) bool { return p.IsReference })
	pair(o.InAlbum, o.NotInAlbum, func(p *photo) bool { return in.has(p, func(*albuminfo.AlbumInfo) bool { return true }) })
	when(o.FromTime != nil, func(p *photo) bool { return timeOfDay(p.Date) >= *o.FromTime })
	when(o.ToTime != nil, func(p *photo) bool { return timeOfDay(p.Date) < *o.ToTime })
	when(len(o.Year) > 0, func(p *photo) bool { return slices.Contains(o.Year, p.Date.Year()) })
	// Photos 5 and later: OriginalFilename only (photoquery.py:693-697).
	when(len(names) > 0, func(p *photo) bool { return containsAny(fold.apply(p.OriginalFilename), names) })
	when(o.MinSize != 0, func(p *photo) bool { return p.OriginalFilesize >= o.MinSize })
	when(o.MaxSize != 0, func(p *photo) bool { return p.OriginalFilesize <= o.MaxSize })
	before, fs = fs, nil
	pair(o.Location, o.NoLocation, func(p *photo) bool { return p.Latitude != nil || p.Longitude != nil })
	when(!o.AddedAfter.IsZero(), func(p *photo) bool { return dateAdded(p).After(o.AddedAfter) })
	when(!o.AddedBefore.IsZero(), func(p *photo) bool { return dateAdded(p).Before(o.AddedBefore) })
	if o.AddedInLast != 0 {
		inLast := time.Now().Add(-o.AddedInLast) // one cutoff for all photos (photoquery.py:835)
		fs = append(fs, func(p *photo) bool { return dateAdded(p).After(inLast) })
	}
	pair(o.Syndicated, o.NotSyndicated, func(p *photo) bool { return p.Syndicated })
	pair(o.SavedToLibrary, o.NotSavedToLibrary, func(p *photo) bool { return p.SavedToLibrary })
	when(o.SavedToLibrary || o.NotSavedToLibrary, func(p *photo) bool { return p.Syndicated }) // photoquery.py:844-847
	pair(o.SharedMoment, o.NotSharedMoment, func(p *photo) bool { return p.SharedMoment })
	pair(o.SharedLibrary, o.NotSharedLibrary, func(p *photo) bool { return p.SharedLibrary })
	return before, fs
}

// timeOfDay is date.time() of Python: the wall clock in the zone of d
// (photoquery.py:676-680).
func timeOfDay(d time.Time) time.Duration {
	h, m, s := d.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(d.Nanosecond())
}

// withBursts follows photoquery.py:863-883 and photoinfo.py:1031-1044.
func withBursts(in, catalog []*photo, missing bool) []*photo {
	frames := framesOfBursts(catalog, burstsHeldBy(in), missing)
	return withoutRepeatedUUIDs(append(slices.Clone(in), frames...))
}

func burstsHeldBy(photos []*photo) map[string]bool {
	held := map[string]bool{}
	for _, p := range photos {
		if p.Burst {
			held[p.BurstUUID] = true
		}
	}
	return held
}

func framesOfBursts(catalog []*photo, bursts map[string]bool, missing bool) []*photo {
	var frames []*photo
	for _, p := range catalog {
		if p.Burst && bursts[p.BurstUUID] && (missing || !p.IsMissing) {
			frames = append(frames, p)
		}
	}
	return frames
}

func withoutRepeatedUUIDs(photos []*photo) []*photo {
	seen := make(map[string]bool, len(photos))
	return slices.DeleteFunc(photos, func(p *photo) bool {
		dup := seen[p.UUID]
		seen[p.UUID] = true
		return dup
	})
}

// caseFold is ignore_case: str.lower of both sides (photoquery.py:484-491).
type caseFold bool

func (f caseFold) apply(s string) string {
	if f {
		return strings.ToLower(s)
	}
	return s
}

// anyEqual is _get_photos_by_attribute: equality, not a substring
// (photoquery.py:996-1021).
func (f caseFold) anyEqual(list, values []string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return slices.Contains(values, f.apply(s)) })
}

// searchValues normalizes as photo_query does (photoquery.py:447-455).
func searchValues(values []string, fold caseFold) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fold.apply(norm.NFC.String(v))
	}
	return out
}

func containsAny(s string, subs []string) bool {
	return slices.ContainsFunc(subs, func(sub string) bool { return strings.Contains(s, sub) })
}

// photo_query sees only the albums outside the trash (photoinfo.py:655-662,
// 2029-2034).
type photoAlbums map[string]*albuminfo.AlbumInfo

func newPhotoAlbums(albums []*albuminfo.AlbumInfo) photoAlbums {
	in := make(photoAlbums, len(albums))
	for _, a := range albums {
		if !a.InTrash {
			in[a.UUID] = a
		}
	}
	return in
}

func (in photoAlbums) has(p *photo, match func(*albuminfo.AlbumInfo) bool) bool {
	return slices.ContainsFunc(p.AlbumUUIDs, func(id string) bool {
		a := in[id]
		return a != nil && match(a)
	})
}
