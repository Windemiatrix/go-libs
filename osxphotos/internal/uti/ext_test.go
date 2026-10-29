package uti

import (
	"encoding/csv"
	"strings"
	"testing"
)

func TestPreferredExtension(t *testing.T) {
	t.Parallel()

	tests := []struct{ uti, want, src string }{
		{"public.mpeg", "mpg", "uti.py:202"},
		{"com.apple.quicktime-movie", "mov", "uti.py:73"},
		{"public.mpeg-4", "mp4", "uti.py:207"},
		{"com.panasonic.raw-image", "raw", "uti.py:280"},
		{"public.tiff", "tiff", "uti.py:324"},
		{"com.adobe.raw-image", "dng", "uti.py:126"},
		{"public.heic", "heic", "uti.py:406"},
		{"public.jpeg", "jpeg", "uti.py:178"},
		{"PUBLIC.JPEG", "jpeg", "uti.py:578"},
		{"public.camera-raw-image", "", "not in the table"},
		{"com.example.none", "", "not in the table"},
		{"", "", "uti.py:611-614"},
	}
	for _, tt := range tests {
		if got := PreferredExtension(tt.uti); got != tt.want {
			t.Errorf("PreferredExtension(%q) = %q, want %q (%s)", tt.uti, got, tt.want, tt.src)
		}
	}
}

// TestPreferredExtensionTable walks the table on its own, so a wrong column in
// the builder fails here.
func TestPreferredExtensionTable(t *testing.T) {
	t.Parallel()

	r := csv.NewReader(strings.NewReader(utiCSV))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs[1:] {
		if got := PreferredExtension(rec[1]); got != rec[2] {
			t.Errorf("PreferredExtension(%q) = %q, want %q", rec[1], got, rec[2])
		}
	}
}
