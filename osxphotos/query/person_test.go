package query

import (
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// photoquery.py:467-470, 996-1021: equality with an element of persons.
func TestPersonFilter(t *testing.T) {
	t.Parallel()
	applyCheckNFD(t)

	photos := []*photoinfo.PhotoInfo{
		{UUID: "P-1", IsPhoto: true, Persons: []string{"Person A", "Person B"}},
		{UUID: "P-2", IsPhoto: true, Persons: []string{applyNFC, "_UNKNOWN_"}},
		{UUID: "P-3", IsPhoto: true},
		{UUID: "P-4", IsPhoto: true, InTrash: true, Persons: []string{"Person A"}},
	}
	applyRun(t, []applyCase{
		{"one name", Options{Person: []string{"Person A"}}, photos, []string{"P-1"}},
		{"not a substring", Options{Person: []string{"Person"}}, photos, nil},
		{"several names", Options{Person: []string{"Person B", "_UNKNOWN_"}}, photos, []string{"P-1", "P-2"}},
		{"an NFD value", Options{Person: []string{applyNFD}}, photos, []string{"P-2"}},
		{"case kept", Options{Person: []string{"person a"}}, photos, nil},
		{"ignore case", Options{Person: []string{"PERSON A"}, IgnoreCase: true}, photos, []string{"P-1"}},
		{"with the trash", Options{Person: []string{"Person A"}, Deleted: true}, photos, []string{"P-1", "P-4"}},
		{"an empty list", Options{Person: []string{}}, photos, []string{"P-1", "P-2", "P-3"}},
	}, nil)
}
