package template

import "time"

// Photo is what Render reads from a photo: the values of the template fields
// of osxphotos by their upstream name (phototemplate.py:930-1000, 1261-1343).
// photosdb implements it for a PhotoInfo, an application for its own fields.
type Photo interface {
	// Field returns the value of name, dotted names whole ("place.name.city",
	// "photo.score.overall"), or false when the photo does not give it: Render
	// then reports ErrUnsupportedField. Render never asks for a name that
	// upstream does not have, one that runs code or a program, or one it
	// renders itself.
	Field(name string) (Value, bool)
}

// Value is the value of one field; only the part the field uses is set.
// Strings holds the values in the order of upstream (sorted for album, keyword,
// person and the other multi-valued fields); empty is the None of upstream,
// and a single-valued field never holds "" (phototemplate.py:1012). Time is
// created or modified, nil when the photo has none (952-956). Int is id.
// Albums is folder_album and folder_album_project: Render joins the folder
// names and the title with the path separator and sorts them (1287-1313).
type Value struct {
	Strings []string
	Time    *time.Time
	Int     *int64
	Albums  []AlbumRef
}

// AlbumRef is one album of a photo: its folder names from the top, none for an
// album at the top level, and its title (albuminfo.py:220-227).
type AlbumRef struct {
	Folders []string
	Title   string
}
