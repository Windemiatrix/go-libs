// Package uti maps file name extensions to Uniform Type Identifiers with
// the table of osxphotos (uti.py:40-514). Data only.
package uti

import (
	"encoding/csv"
	"strings"
)

// Read as csv.DictReader of uti.py:521-526 reads it: the first line is the
// header, the number of fields is not checked, and of two lines with one
// extension the last one wins.
var table = func() map[string]string {
	r := csv.NewReader(strings.NewReader(utiCSV))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		panic(err) // utiCSV is a constant: an error fails every test of the package
	}
	m := make(map[string]string, len(recs))
	for _, rec := range recs[1:] {
		m[rec[0]] = rec[1]
	}
	return m
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
