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
// albums every user album, the albums that PhotoInfo.AlbumUUIDs refers to
// (DB.Albums()). The result is new, in the order of Sort (reversed with
// NewestFirst), each photo once unless a Function returns it twice; its
// elements are the pointers of photos. photos and albums are not modified.
func Apply(o Options, photos []*photoinfo.PhotoInfo, albums []*albuminfo.AlbumInfo) []*photoinfo.PhotoInfo {
	fs := filters(o, newPhotoAlbums(albums))
	var out []*photo
	for _, p := range photos {
		if inSelection(o, p) && !slices.ContainsFunc(fs, func(f filter) bool { return !f(p) }) {
			out = append(out, p)
		}
	}
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

// inSelection reports a photo of the photos() calls that photo_query starts
// from (photoquery.py:426-445; photosdb/photosdb.py:3268-3360).
func inSelection(o Options, p *photo) bool {
	return inTrashSelection(o, p) && !p.BurstUnselected() &&
		((p.IsPhoto && !o.OnlyMovies) || (p.IsMovie && !o.OnlyPhotos)) &&
		(len(o.UUID) == 0 || slices.Contains(o.UUID, p.UUID)) &&
		(o.FromDate.IsZero() || !p.Date.Before(o.FromDate)) &&
		(o.ToDate.IsZero() || p.Date.Before(o.ToDate))
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

// filter is one condition of photo_query on a photo.
type filter func(*photo) bool

// filters returns the conditions that o sets, in the order of photo_query
// (photoquery.py:447-857). A pair X/NotX is "if X … elif NotX": X wins.
func filters(o Options, in photoAlbums) []filter {
	fold := caseFold(o.IgnoreCase)
	inLast := time.Now().Add(-o.AddedInLast) // one cutoff for all photos (photoquery.py:835)
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
	keywords, names := searchValues(o.Keyword, fold), searchValues(o.Name, fold)

	when(len(albums.paths) > 0, func(p *photo) bool { return in.has(p, albums.name) })
	when(len(keywords) > 0, func(p *photo) bool {
		return slices.ContainsFunc(p.Keywords, func(k string) bool { return slices.Contains(keywords, fold.apply(k)) })
	})
	when(len(keywords) == 0 && o.NoKeyword, func(p *photo) bool { return len(p.Keywords) == 0 })
	when(len(folders.paths) > 0, func(p *photo) bool { return in.has(p, folders.enclose) })
	text(searchValues(o.Title, fold), o.NoTitle, func(p *photo) string { return p.Title })
	text(searchValues(o.Description, fold), o.NoDescription, func(p *photo) string { return p.Description })
	pair(o.Edited, o.NotEdited, func(p *photo) bool { return p.HasAdjustments })
	pair(o.Favorite, o.NotFavorite, func(p *photo) bool { return p.Favorite })
	pair(o.Hidden, o.NotHidden, func(p *photo) bool { return p.Hidden })
	pair(o.Missing, o.NotMissing, func(p *photo) bool { return p.Path == "" })
	pair(o.Shared, o.NotShared, func(p *photo) bool { return p.Shared })
	when(o.UTI != "", func(p *photo) bool { return strings.Contains(p.UTIOriginal, o.UTI) })
	pair(o.Burst, o.NotBurst, func(p *photo) bool { return p.Burst })
	pair(o.Live, o.NotLive, func(p *photo) bool { return p.Live })
	pair(o.Screenshot, o.NotScreenshot, func(p *photo) bool { return p.Screenshot })
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
	pair(o.Location, o.NoLocation, func(p *photo) bool { return p.Latitude != nil || p.Longitude != nil })
	when(!o.AddedAfter.IsZero(), func(p *photo) bool { return dateAdded(p).After(o.AddedAfter) })
	when(!o.AddedBefore.IsZero(), func(p *photo) bool { return dateAdded(p).Before(o.AddedBefore) })
	when(o.AddedInLast != 0, func(p *photo) bool { return dateAdded(p).After(inLast) })
	pair(o.Syndicated, o.NotSyndicated, func(p *photo) bool { return p.Syndicated })
	pair(o.SavedToLibrary, o.NotSavedToLibrary, func(p *photo) bool { return p.SavedToLibrary })
	when(o.SavedToLibrary || o.NotSavedToLibrary, func(p *photo) bool { return p.Syndicated }) // photoquery.py:844-847
	pair(o.SharedLibrary, o.NotSharedLibrary, func(p *photo) bool { return p.SharedLibrary })
	return fs
}

// timeOfDay is date.time() of Python: the wall clock in the zone of d
// (photoquery.py:676-680).
func timeOfDay(d time.Time) time.Duration {
	h, m, s := d.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(d.Nanosecond())
}

// withBursts adds to in the other frames of the bursts it holds, taken from
// every photo of catalog, in the trash or unselected too; the frames that are
// IsMissing only with missing. Each UUID stays once, at its first place
// (photoquery.py:863-883; photoinfo.py:1031-1044).
func withBursts(in, catalog []*photo, missing bool) []*photo {
	frames := map[string][]*photo{}
	for _, p := range catalog {
		if p.Burst && (missing || !p.IsMissing) {
			frames[p.BurstUUID] = append(frames[p.BurstUUID], p)
		}
	}
	out := slices.Clone(in)
	for _, p := range in {
		if p.Burst {
			out = append(out, frames[p.BurstUUID]...)
		}
	}
	seen := make(map[string]bool, len(out))
	return slices.DeleteFunc(out, func(p *photo) bool {
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

// searchValues are the values of a text option in NFC, as photo_query
// normalizes them (photoquery.py:447-455), case folded.
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

// photoAlbums are the albums of a photo as photo_query sees them: those of
// AlbumUUIDs outside the trash (photoinfo.py:655-662, 2029-2034).
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

// has reports an album of the photo for which match holds.
func (in photoAlbums) has(p *photo, match func(*albuminfo.AlbumInfo) bool) bool {
	return slices.ContainsFunc(p.AlbumUUIDs, func(id string) bool {
		a := in[id]
		return a != nil && match(a)
	})
}
