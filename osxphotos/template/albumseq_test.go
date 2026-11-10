package template

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func albumSeqPhoto() fakePhoto {
	return fakePhoto{
		"album_seq": {Albums: []AlbumRef{
			{Folders: []string{"F"}, Title: "A", Index: 2},
			{Title: "A", Index: 5},
			{Folders: []string{"G/H"}, Title: "B/C", Index: 7},
			{Folders: []string{"D", "E"}, Title: "Holidays", Index: 0},
		}},
		// a burst: folder_album gives other albums than album_seq (phototemplate.py:983, 985, 1290-1295)
		"folder_album": {Albums: []AlbumRef{{Folders: []string{"F"}, Title: "Burst", Index: 9}}},
	}
}

func TestRenderAlbumSeq(t *testing.T) {
	t.Parallel()
	p := albumSeqPhoto
	dest := func(path string) RenderOptions { return RenderOptions{DestPath: path} }
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"no dest path", "{album_seq}", p(), RenderOptions{}, one("_"), nil},
		{"no dest path folder", "{folder_album_seq}", p(), RenderOptions{}, one("_"), nil},
		{"no dest path and a bad start", "{album_seq(x)}", p(), RenderOptions{}, one("_"), nil},
		// no album is read without dest_path (980-988)
		{"no dest path, no field", "{album_seq}", fakePhoto{}, RenderOptions{}, one("_"), nil},
		{"no dest path, untitled album", "{album_seq}", fakePhoto{"album_seq": {Albums: []AlbumRef{{Index: 4}}}}, RenderOptions{}, one("_"), nil},
		// the first album of the title (1670-1673)
		{"by name", "{album_seq}", p(), dest("/out/A"), one("2"), nil},
		{"trailing slash", "{album_seq}", p(), dest("/out/A/"), one("2"), nil},
		{"relative", "{album_seq}", p(), dest("A"), one("2"), nil},
		{"no album of the name", "{album_seq}", p(), dest("/out/X"), one("_"), nil},
		{"no album and a bad start", "{album_seq(x)}", p(), dest("/out/X"), one("_"), nil},
		// the None of 986 takes the default and the bool (770-772)
		{"default when no album", "{album_seq,none}", p(), dest("/out/X"), one("none"), nil},
		{"bool when no album", "{album_seq?Y,N}", p(), dest("/out/X"), one("N"), nil},
		{"root", "{album_seq}", p(), dest("/"), one("_"), nil},
		{"by the name, not the folders", "{album_seq}", p(), dest("/out/D/E/Holidays"), one("0"), nil},
		// album_info.title == album, the raw title (1671)
		{"raw title by name", "{album_seq}", p(), dest("/out/B:C"), one("_"), nil},
		{"start", "{album_seq(1)}", p(), dest("/out/A"), one("3"), nil},
		{"negative start", "{album_seq(-3)}", p(), dest("/out/A"), one("-1"), nil},
		{"format", "{album_seq:03d}", p(), dest("/out/A"), one("002"), nil},
		{"format and start", "{album_seq:03d(1)}", p(), dest("/out/A"), one("003"), nil},
		{"spaces around the start", "{album_seq( 1 )}", p(), dest("/out/A"), one("3"), nil},
		{"dotted name", "{album_seq.x}", p(), dest("/out/A"), one("2"), nil},
		// the sanitized folders and title end the path (1676-1684)
		{"by path", "{folder_album_seq}", p(), dest("/out/F/A"), one("2"), nil},
		{"by path without folders", "{folder_album_seq}", p(), dest("/out/A"), one("5"), nil},
		{"by sanitized path", "{folder_album_seq}", p(), dest("/out/G:H/B:C"), one("7"), nil},
		{"not by the raw path", "{folder_album_seq}", p(), dest("/out/G/H/B/C"), one("_"), nil},
		{"by path of two folders", "{folder_album_seq(1)}", p(), dest("/out/D/E/Holidays"), one("1"), nil},
		{"by path, not by name", "{folder_album_seq}", p(), dest("/out/X/Holidays"), one("_"), nil},
		{"not folder_album", "{folder_album_seq}", p(), dest("/out/F/Burst"), one("_"), nil},
		// "F/A" with no leading "/" (1680), "/A" at the top level (1681)
		{"folders without a leading slash", "{folder_album_seq}", p(), dest("/out/XF/A"), one("2"), nil},
		{"top level is /title", "{folder_album_seq}", p(), dest("/out/XA"), one("_"), nil},
		// endswith of dest_path as given (1682)
		{"raw path, trailing slash", "{folder_album_seq}", p(), dest("/out/F/A/"), one("_"), nil},
		{"filename", "{album_seq}", p(), RenderOptions{DestPath: "/out/A", Filename: true}, one("2"), nil},
		{"in text", "IMG_{album_seq:04d}", p(), dest("/out/A"), one("IMG_0002"), nil},
	})
}

func TestRenderAlbumSeqErrors(t *testing.T) {
	t.Parallel()
	o := RenderOptions{DestPath: "/out/Holidays"}
	holidays := func(index int) fakePhoto {
		return with(albumSeqPhoto(), "album_seq", Value{Albums: []AlbumRef{{Title: "Holidays", Index: index}}})
	}
	for _, tt := range []struct {
		in    string
		photo fakePhoto
		want  error
		names string
	}{
		// the name asked of the Photo, as {modified.year} names created
		{"{album_seq}", fakePhoto{}, ErrUnsupportedField, ": album_seq"},
		{"{folder_album_seq}", fakePhoto{"folder_album": {}}, ErrUnsupportedField, ": album_seq"},
		// photo_index raises ValueError (albuminfo.py:285-287)
		{"{album_seq}", holidays(-1), ErrRender, "album_seq"},
		{"{folder_album_seq}", holidays(-1), ErrRender, "folder_album_seq"},
		// int(field_arg) raises ValueError (990)
		{"{album_seq(x)}", albumSeqPhoto(), ErrRender, "album_seq"},
		{"{album_seq()}", albumSeqPhoto(), ErrRender, "album_seq"},
		{"{album_seq(1.5)}", albumSeqPhoto(), ErrRender, "album_seq"},
		{"{folder_album_seq(x)}", holidays(1), ErrRender, "folder_album_seq"},
		// the int of Python has no bound; UPSTREAM.md has the int64 one, as for counter
		{"{album_seq(9223372036854775808)}", albumSeqPhoto(), ErrRender, "album_seq"},
		{"{album_seq(9223372036854775807)}", holidays(1), ErrRender, "album_seq"},
		{"{album_seq:.2d}", albumSeqPhoto(), ErrRender, "album_seq"},
	} {
		got, _, err := render(t, tt.in, tt.photo, o)
		if msg := fmt.Sprint(err); !errors.Is(err, tt.want) || !strings.Contains(msg, tt.names) || strings.Contains(msg, "Holidays") || got != nil {
			t.Errorf("Render(%q) = %q, %v; want %v that names %q and not the path", tt.in, got, err, tt.want, tt.names)
		}
	}
}
