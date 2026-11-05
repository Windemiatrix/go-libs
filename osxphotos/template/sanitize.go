package template

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MAX_FILENAME_LEN and MAX_DIRNAME_LEN (_constants.py:631-634), in code
// points, as len of a Python str counts.
const (
	maxFilenameLen = 240
	maxDirnameLen  = 255
)

// The default replacement of sanitize_filename and sanitize_dirname, which the
// renderer never overrides (phototemplate.py:531, 1007, 1009).
const slashReplacement = ":"

// SanitizeFilename returns name as one file name, as sanitize_filename does
// (path_utils.py:39-67): in NFC (unicode.py:76-92), then "/" replaced by
// replacement, then cut to 240 code points (_constants.py:631). The cut takes
// the end of the name when it has no "."; else the end of the part before the
// last "." when that part is at least as long as the excess, which may leave
// nothing of it before the "."; else the end of the extension, keeping that
// part whole, so the result may exceed 240 code points, as upstream (60-64).
// "" stays "". Nothing else changes: ".", "..", NUL, control characters and a
// leading "-" pass through, as upstream, and the caller keeps the name inside
// its directory; a replacement that holds "/" leaves a "/".
func SanitizeFilename(name, replacement string) string {
	name = strings.ReplaceAll(norm.NFC.String(name), "/", replacement)
	drop := utf8.RuneCountInString(name) - maxFilenameLen
	i := strings.LastIndexByte(name, '.')
	switch {
	case drop <= 0:
		return name
	case i < 0:
		return dropRunes(name, drop)
	case drop > utf8.RuneCountInString(name[:i]):
		return name[:i+1] + dropRunes(name[i+1:], drop)
	}
	return dropRunes(name[:i], drop) + name[i:]
}

// SanitizeDirname returns name as one directory name, as sanitize_dirname and
// sanitize_pathpart do (path_utils.py:70-103): "/" replaced by replacement,
// then NFC, then cut to its first 255 code points (_constants.py:634). The
// order differs from SanitizeFilename, as upstream: a replacement next to a
// combining mark may compose with it here and not there. "" stays ""; ".",
// "..", NUL and control characters pass through, as upstream: a title ".." is
// the directory "..", and the caller keeps the path inside its directory; a
// replacement that holds "/" leaves a "/".
func SanitizeDirname(name, replacement string) string {
	name = norm.NFC.String(strings.ReplaceAll(name, "/", replacement))
	if utf8.RuneCountInString(name) > maxDirnameLen {
		return string([]rune(name)[:maxDirnameLen])
	}
	return name
}

// s[:-n] of Python for n > 0, which is "" for an n past the start.
func dropRunes(s string, n int) string {
	r := []rune(s)
	return string(r[:max(len(r)-n, 0)])
}

// sanitize_dirname is sanitize_pathpart behind its own check of "" (path_utils.py:80-81).
func sanitizePathpart(v string) string { return SanitizeDirname(v, slashReplacement) }

// sanitize_pathpart with Filename, sanitize_dirname with Dirname but for
// folder_album and folder_album_project, whose parts are sanitized in values
// before the join (phototemplate.py:1005-1009, 1039-1042, 1299-1310, 1388-1393).
func (s *rendering) sanitize(name string, vals []string) error {
	if s.o.Filename || (s.o.Dirname && name != "folder_album" && name != "folder_album_project") {
		return s.each(vals, sanitizePathpart)
	}
	return nil
}
