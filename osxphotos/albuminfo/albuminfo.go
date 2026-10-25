// Package albuminfo describes albums and folders of a Photos library as plain
// values. It does no I/O.
package albuminfo

import "time"

// AlbumInfo is one user album.
type AlbumInfo struct {
	UUID                             string
	CloudGUID                        string     // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title                            string     // in NFC
	ParentUUID                       string     // the enclosing folder; "" at the top level
	FolderNames                      []string   // titles of the enclosing folders, outermost first
	PhotoUUIDs                       []string   // UUIDs of the member photos, sorted; the order of the album is not kept
	CreationDate, StartDate, EndDate *time.Time // nil without a value; osxphotos gives 2001-01-01 for a missing creation date
	InTrash                          bool       // extension: not in osxphotos — ZTRASHEDSTATE is not 0, NULL included, as osxphotos reads it (photosdb.py:1936); osxphotos keeps no album in the trash in its lists
}

// FolderInfo is one folder (albuminfo.py:395-527).
type FolderInfo struct {
	UUID       string
	CloudGUID  string        // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title      string        // in NFC
	ParentUUID string        // the enclosing folder; "" at the top level and for a folder without a parent
	Subfolders []*FolderInfo // the folders in it outside the trash, in UUID order
	Albums     []*AlbumInfo  // the user albums in it outside the trash, in UUID order
}
