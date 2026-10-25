package query

import (
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
)

// escapedSlash is the placeholder of upstream for "//" (photoquery.py:932):
// with the same one, a path that holds it means what it means upstream.
const escapedSlash = "\x00ESCAPED_SLASH\x00"

// albumPaths are the paths of Album or Folder, each split into titles,
// outermost first; several paths are "or" (photoquery.py:927-993).
type albumPaths struct {
	paths [][]string
	fold  caseFold
}

func parseAlbumPaths(paths []string, fold caseFold) albumPaths {
	ps := albumPaths{fold: fold}
	for _, s := range paths {
		ps.paths = append(ps.paths, parseAlbumPath(norm.NFC.String(s), fold))
	}
	return ps
}

// parseAlbumPath splits a path at "/"; "//" stands for a "/" inside a title
// (photoquery.py:930-941).
func parseAlbumPath(s string, fold caseFold) []string {
	parts := strings.Split(strings.ReplaceAll(s, "//", escapedSlash), "/")
	for i, part := range parts {
		parts[i] = fold.apply(strings.ReplaceAll(part, escapedSlash, "/"))
	}
	return parts
}

// name reports a path to the album: its folders, then its title
// (photoquery.py:973-988).
func (ps albumPaths) name(a *albuminfo.AlbumInfo) bool {
	n := len(a.FolderNames)
	return slices.ContainsFunc(ps.paths, func(p []string) bool {
		return len(p) == n+1 && p[n] == ps.fold.apply(a.Title) && ps.prefix(p[:n], a.FolderNames)
	})
}

// enclose reports a path of folders that the album lies in, directly or in a
// subfolder (photoquery.py:954-972).
func (ps albumPaths) enclose(a *albuminfo.AlbumInfo) bool {
	return slices.ContainsFunc(ps.paths, func(p []string) bool { return ps.prefix(p, a.FolderNames) })
}

func (ps albumPaths) prefix(parts, names []string) bool {
	if len(names) < len(parts) {
		return false
	}
	for i, part := range parts {
		if ps.fold.apply(names[i]) != part {
			return false
		}
	}
	return true
}
