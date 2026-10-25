package uti

import (
	"maps"
	"strings"
	"testing"
)

// dataLines is the number of data lines of UTI_CSV, uti.py:41-513.
const dataLines = 473

func TestTable(t *testing.T) {
	t.Parallel()

	if got := strings.Count(utiCSV, "\n") - 1; got != dataLines {
		t.Errorf("utiCSV has %d data lines, want %d (uti.py:41-513)", got, dataLines)
	}
	// "orf" is the only extension listed twice (uti.py:234-235).
	if len(table) != dataLines-1 {
		t.Errorf("table has %d extensions, want %d", len(table), dataLines-1)
	}
	// Upstream looks up ext.lower() (uti.py:571): a key with an upper-case
	// letter could never be found, and the table has none.
	for k := range maps.Keys(table) {
		if k != strings.ToLower(k) {
			t.Errorf("table key %q is not in lower case", k)
		}
	}
}

func TestForExtension(t *testing.T) {
	t.Parallel()

	// src is the line of uti.py the value comes from.
	tests := []struct{ ext, want, src string }{
		{"jpg", "public.jpeg", "uti.py:180"},
		{".JPG", "public.jpeg", "uti.py:180, 571, 624-625"},
		{"JPG", "public.jpeg", "uti.py:180, 571"},
		{".Jpg", "public.jpeg", "uti.py:180, 571, 624-625"},
		{"jpeg", "public.jpeg", "uti.py:417"},
		{"heic", "public.heic", "uti.py:406"},
		{"dng", "com.adobe.raw-image", "uti.py:126"},
		{"cr2", "com.canon.cr2-raw-image", "uti.py:111"},
		{"mov", "com.apple.quicktime-movie", "uti.py:201"},
		{"mp4", "public.mpeg-4", "uti.py:207"},
		{"png", "public.png", "uti.py:265"},
		{"orf", "com.olympus.or-raw-image", "uti.py:235, the last of two lines"},
		{"afploc", "com.apple.afp-internet-location", "uti.py:513, the last line"},
		{"c", "public.c-source", "uti.py:41, the first line"},
		{"xyz9", "", "not in the table"},
		{"", "", "uti.py:620-621"},
		{".", "", "uti.py:624-625, then no extension"},
		{"..jpg", "", "only one leading '.' is removed, uti.py:624-625"},
		{"extension", "", "the header is not a line of the table"},
	}
	for _, tt := range tests {
		if got := ForExtension(tt.ext); got != tt.want {
			t.Errorf("ForExtension(%q) = %q, want %q (%s)", tt.ext, got, tt.want, tt.src)
		}
	}
}
