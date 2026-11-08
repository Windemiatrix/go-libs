package albuminfo

import (
	"cmp"
	"time"
)

// Upstream reads a missing creation date as 0 in Core Data seconds
// (photosdb.py:1937-1938); the port takes it in UTC.
var noCreationDate = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// AsDict is asdict of AlbumInfo (albuminfo.py:135-143, 289-297), without
// photos, as upstream. A nil date and an empty owner, title or parent are nil,
// as None upstream; a nil CreationDate is 2001-01-01T00:00:00Z; a nil list is [].
// sort_order is the AlbumSortOrder, a plain Enum upstream (_constants.py:707,
// albuminfo.py:295), which its json writes as null (photoinfo.py:2269-2271).
// parent is the parent row, nil at the top level; upstream gives None also
// when that row holds no folder and no user album outside the trash
// (albuminfo.py:296, 525-527), as for some projects, which the value does not
// know. The map and its lists are new on every call.
func (a *AlbumInfo) AsDict() map[string]any {
	return map[string]any{
		"uuid": a.UUID, "creation_date": *cmp.Or(a.CreationDate, &noCreationDate),
		"start_date": orNil(a.StartDate), "end_date": orNil(a.EndDate), "owner": strOrNil(a.Owner),
		"title": strOrNil(a.Title), "folder_names": listOf(a.FolderNames), "folder_list": listOf(a.FolderUUIDs),
		"sort_order": a.SortOrder, "parent": strOrNil(a.ParentUUID),
	}
}

// AsDict is asdict of ImportInfo (albuminfo.py:350-358), without photos, as
// upstream; the dates and the title as in AlbumInfo.AsDict. The map is new on
// every call.
func (s *ImportInfo) AsDict() map[string]any {
	return map[string]any{
		"uuid": s.UUID, "creation_date": *cmp.Or(s.CreationDate, &noCreationDate),
		"start_date": orNil(s.StartDate), "end_date": orNil(s.EndDate), "title": strOrNil(s.Title),
	}
}

// AsDict is asdict of FolderInfo (albuminfo.py:515-523): an empty title or
// parent is nil, as None upstream, parent at the top level; subfolders and
// albums are UUIDs in the order of Subfolders and Albums, [] when empty. The
// map is new on every call.
func (f *FolderInfo) AsDict() map[string]any {
	subfolders, albums := make([]string, 0, len(f.Subfolders)), make([]string, 0, len(f.Albums))
	for _, s := range f.Subfolders {
		subfolders = append(subfolders, s.UUID)
	}
	for _, a := range f.Albums {
		albums = append(albums, a.UUID)
	}
	return map[string]any{
		"title": strOrNil(f.Title), "uuid": f.UUID, "parent": strOrNil(f.ParentUUID),
		"subfolders": subfolders, "albums": albums,
	}
}

// A nil pointer in an any is not nil, which None is.
func orNil[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// The port holds None of upstream as "" here.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// json writes a nil slice as null, where upstream writes the empty list as [].
func listOf(s []string) []string { return append([]string{}, s...) }
