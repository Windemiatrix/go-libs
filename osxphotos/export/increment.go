package export

import (
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// StemKey is the key under which increment_filename_with_count compares
// names (utils.py:428, 433): the stem of name as pathlib.Path.stem, in NFC
// and lower case. The caller builds taken of IncrementName from the StemKey
// of every file of the directory.
func StemKey(name string) string {
	stem, _ := splitSuffix(name)
	return fold(stem)
}

// IncrementName ports increment_filename_with_count (utils.py:399-437) for
// one name without a directory: it returns name, or name with " (N)" before
// the extension, for the first N from count on for which the stem of name with
// " (N)" appended, in NFC and lower case, is not in taken. The candidate is
// not split again (430-435), so "a." gives "a. (1)" even when a.jpg is in the
// directory. count 0 tries the plain name first (430). An extension never
// matters: foo.png in taken blocks foo.jpeg (426-433;
// photoexporter.py:607-610). The second result is the N used, 0 for the
// plain name. taken is finite, so the loop ends after at most len(taken)+1
// candidates; a nil taken is an empty directory. The result is in NFC and is
// name as given apart from the count: name comes from outside, and the caller
// sanitizes it.
func IncrementName(name string, count int, taken map[string]bool) (string, int) {
	stem, suffix := splitSuffix(name)
	candidate := stem
	if count != 0 {
		candidate = withCount(stem, count)
	}
	for taken[fold(candidate)] {
		count++
		candidate = withCount(stem, count)
	}
	return norm.NFC.String(candidate + suffix), count
}

func withCount(stem string, count int) string {
	return stem + " (" + strconv.Itoa(count) + ")"
}

// fold is the form in which names are compared: upstream compares the NFD
// form in lower case (utils.py:431-433), and one form on both sides is enough.
func fold(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}
