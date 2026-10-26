// Package albuminfo describes albums and folders of a Photos library as plain
// values. It does no I/O.
package albuminfo

import "time"

// AlbumSortOrder is the sort order of an album, osxphotos AlbumSortOrder
// (_constants.py:707-714).
type AlbumSortOrder int

// AlbumSortUnknown is also the value of a sort key or direction that osxphotos
// does not name (albuminfo.py:243-256).
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
	PhotoUUIDs                       []string       // UUIDs of the member photos, sorted; DB.AlbumPhotos gives the order of the album
	CreationDate, StartDate, EndDate *time.Time     // nil without a value; osxphotos gives 2001-01-01 for a missing creation date
	InTrash                          bool           // extension: not in osxphotos — ZTRASHEDSTATE is not 0, NULL included, as osxphotos reads it (photosdb.py:1936); osxphotos keeps no album in the trash in its lists
	SortOrder                        AlbumSortOrder // albuminfo.py:237-256; DB.AlbumPhotos applies it, PhotoUUIDs does not
	LibraryListOrder                 []int          // the place in the sidebar of Photos, of the enclosing folders first, for comparing as a sort key (albuminfo.py:258-278); 9999999 stands for a row without a stored order, and the folders above it are dropped, as upstream (albuminfo.py:266-278, 501-513), so [9999999, 6] when the enclosing folder has none
}

// ImportInfo is one import session (albuminfo.py:300-365).
type ImportInfo struct {
	UUID                             string
	Title                            string     // in NFC
	CreationDate, StartDate, EndDate *time.Time // nil without a value; osxphotos gives 2001-01-01 for a missing creation date
	PhotoUUIDs                       []string   // UUIDs of the photos imported in it, those in the trash included, sorted; nil without photos
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
