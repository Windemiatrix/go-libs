// Package albuminfo describes albums and folders of a Photos library as plain
// values. It does no I/O.
package albuminfo

import "time"

// AlbumSortOrder is the sort order of an album, osxphotos AlbumSortOrder
// (_constants.py:707-714).
type AlbumSortOrder int

// The values of AlbumSortOrder are those of osxphotos.
const (
	AlbumSortUnknown     AlbumSortOrder = 0
	AlbumSortManual      AlbumSortOrder = 1
	AlbumSortNewestFirst AlbumSortOrder = 2
	AlbumSortOldestFirst AlbumSortOrder = 3
	AlbumSortTitle       AlbumSortOrder = 5
)

// AlbumInfo is one user album.
type AlbumInfo struct {
	UUID                             string
	CloudGUID                        string         // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title                            string         // in NFC
	ParentUUID                       string         // the enclosing folder; "" at the top level
	FolderNames                      []string       // titles of the enclosing folders, outermost first
	PhotoUUIDs                       []string       // UUIDs of the member photos, sorted; the order of the album is not kept
	CreationDate, StartDate, EndDate *time.Time     // nil without a value; osxphotos gives 2001-01-01 for a missing creation date
	InTrash                          bool           // extension: not in osxphotos — ZTRASHEDSTATE is not 0, NULL included, as osxphotos reads it (photosdb.py:1936); osxphotos keeps no album in the trash in its lists
	SortOrder                        AlbumSortOrder // albuminfo.py:237-256; the order of PhotoUUIDs is not this one
	LibraryListOrder                 []int          // the place in the sidebar of Photos, of the enclosing folders first, for comparing as a sort key (albuminfo.py:258-278); [9999999] without a stored order
}

// FolderInfo is one folder (albuminfo.py:395-527).
type FolderInfo struct {
	UUID             string
	CloudGUID        string        // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title            string        // in NFC
	ParentUUID       string        // the enclosing folder; "" at the top level and for a folder without a parent
	Subfolders       []*FolderInfo // the folders in it outside the trash, in UUID order
	Albums           []*AlbumInfo  // the user albums in it outside the trash, in UUID order
	LibraryListOrder []int         // as AlbumInfo.LibraryListOrder (albuminfo.py:493-513)
}
