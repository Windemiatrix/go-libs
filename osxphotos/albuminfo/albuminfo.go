// Package albuminfo describes albums and folders of a Photos library as plain
// values. It does no I/O.
package albuminfo

import "time"

// AlbumInfo is one user album.
type AlbumInfo struct {
	UUID                             string
	CloudGUID                        string // extension: not in osxphotos — ZGENERICALBUM.ZCLOUDGUID; "" when absent
	Title                            string
	ParentUUID                       string   // the enclosing folder; "" at the top level
	FolderNames                      []string // titles of the enclosing folders, outermost first
	PhotoUUIDs                       []string // members, in no particular order
	CreationDate, StartDate, EndDate *time.Time
	InTrash                          bool
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
