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
	InTrash                          bool       // ZTRASHEDSTATE is not 0, NULL included, as in osxphotos
}

// FolderInfo is one folder.
type FolderInfo struct {
	UUID       string
	CloudGUID  string // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title      string
	ParentUUID string // "" at the top level
	Subfolders []*FolderInfo
	Albums     []*AlbumInfo
}
