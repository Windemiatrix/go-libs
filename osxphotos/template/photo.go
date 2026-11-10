package template

import "time"

// Photo is what Render reads from a photo: the values of the template fields
// of osxphotos by their upstream name (phototemplate.py:930-1000, 1264-1346).
// photosdb implements it for a PhotoInfo, an application for its own fields.
// name, original_name and burst give the file name (filename,
// original_filename, the original_filename of burst_key_photo for a burst and
// none otherwise): Render takes the stem (931-933, 995-1000). shortuuid is
// never asked for: Render makes it of uuid (975-976). album_seq gives the
// albums of album_info with their Index and is read for folder_album_seq too
// (979-992): unlike folder_album, not the albums of the burst. Render drops
// the person "_UNKNOWN_" (1278-1281).
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
// and a single-valued field never holds "" (phototemplate.py:1012), except a
// string attribute of photo.X, which renders even when "" (1372-1373). Time is
// created or modified, nil when the photo has none (952-956). Int is id.
// Albums is folder_album and folder_album_project, whose folder names and
// title Render joins with the path separator and sorts (1290-1316), and
// album_seq, in the order of album_info, whose first match Render takes
// (1670, 1678).
type Value struct {
	Strings []string
	Time    *time.Time
	Int     *int64
	Albums  []AlbumRef
}

// AlbumRef is one album of a photo: its folder names from the top, none for an
// album at the top level, its title (albuminfo.py:220-227) and, for album_seq,
// the position of the photo in the album, from 0 (photo_index,
// albuminfo.py:280-287); a negative Index is ErrRender, as photo_index raises.
type AlbumRef struct {
	Folders []string
	Title   string
	Index   int
}
