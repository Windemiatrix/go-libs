// Package uti maps file name extensions to Uniform Type Identifiers, and
// back to their preferred extensions, with the table of osxphotos
// (uti.py:40-514). Data only.
package uti

import (
	"encoding/csv"
	"strings"
)

// Read as csv.DictReader of uti.py:521-526 reads it: the first line is the
// header, the number of fields is not checked, and of two lines with one
// extension, or with one UTI, the last one wins.
var table, extByUTI = func() (map[string]string, map[string]string) {
	r := csv.NewReader(strings.NewReader(utiCSV))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		panic(err) // utiCSV is a constant: an error fails every test of the package
	}
	exts := make(map[string]string, len(recs))
	prefs := make(map[string]string, len(recs))
	for _, rec := range recs[1:] {
		exts[rec[0]] = rec[1]
		prefs[rec[1]] = rec[2]
	}
	return exts, prefs
}()

// ForExtension returns the UTI of ext, with or without the leading ".",
// compared in lower case (uti.py:569-573, 617-653), or "" when the table
// has none. The CoreServices and mdls fallbacks of osxphotos are not ported.
func ForExtension(ext string) string {
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		return ""
	}
	return table[strings.ToLower(ext)]
}
