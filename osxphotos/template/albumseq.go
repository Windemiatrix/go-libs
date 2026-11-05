package template

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// album_seq and folder_album_seq (phototemplate.py:979-992).
func (s *rendering) albumSeq(f *Field, stem string) ([]string, error) {
	if s.o.DestPath == "" { // 987-988
		return nil, nil
	}
	v, ok := s.p.Field("album_seq")
	if !ok {
		return nil, unsupported(ErrUnsupportedField, "album_seq")
	}
	// pathlib.Path(dest_path).name and _get_album_by_name (982, 1665-1670)
	name := s.pathProperty(s.o.DestPath, []string{"name"})
	match := func(a AlbumRef) bool { return a.Title == name }
	if stem == "folder_album_seq" { // _get_album_by_path (1673-1681)
		match = func(a AlbumRef) bool { return strings.HasSuffix(s.o.DestPath, albumPath(a)) }
	}
	i := slices.IndexFunc(v.Albums, match)
	if i < 0 {
		return nil, nil
	}
	if v.Albums[i].Index < 0 { // photo_index raises ValueError (albuminfo.py:285-287)
		return nil, fmt.Errorf("%w: %s of a photo that is not in its album", ErrRender, f.Name)
	}
	index := int64(v.Albums[i].Index)
	var start int64
	var err error
	if f.Arg != nil { // int(field_arg) only after a match (989-990)
		start, err = strconv.ParseInt(strings.TrimFunc(*f.Arg, isSpace), 10, 64)
	}
	if err != nil || index+start < start { // the int of Python has no bound
		return nil, fmt.Errorf("%w: %s takes (start) as an integer within int64", ErrRender, f.Name)
	}
	out, err := s.formatValue(index+start, f.Subfield)
	return []string{out}, inField(err, f)
}

// _get_album_by_path builds the path as {folder_album} does, but with "/" and
// sanitize_dirname whatever the options, and "/title" at the top level (1676-1678).
func albumPath(a AlbumRef) string {
	folders := make([]string, len(a.Folders))
	for i, folder := range a.Folders {
		folders[i] = sanitizePathpart(folder)
	}
	return strings.Join(folders, "/") + "/" + sanitizePathpart(a.Title)
}
