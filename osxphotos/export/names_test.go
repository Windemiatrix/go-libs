package export

import (
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func TestSplitSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, stem, suffix string }{
		{"a.b.c", "a.b", ".c"},
		{".hidden", ".hidden", ""},
		{"a.", "a.", ""},
		{"a", "a", ""},
		{"IMG_0001.HEIC", "IMG_0001", ".HEIC"},
		{"name with spaces.jpeg", "name with spaces", ".jpeg"},
		{".a.b", ".a", ".b"},
	}
	for _, tt := range tests {
		if stem, suffix := splitSuffix(tt.name); stem != tt.stem || suffix != tt.suffix {
			t.Errorf("splitSuffix(%q) = %q, %q, want %q, %q", tt.name, stem, suffix, tt.stem, tt.suffix)
		}
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	const nfd, nfc = "Cafe\U00000301.jpg", "Caf\U000000e9.jpg"
	if nfd == nfc {
		t.Fatal("the NFD and NFC spellings are the same bytes")
	}

	tests := []struct {
		desc string
		p    photoinfo.PhotoInfo
		name string
		want Files
	}{
		{
			desc: "nothing but the name",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC"},
			name: "IMG_0001.HEIC",
			want: Files{Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "live photo",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", Live: true},
			name: "IMG_0001.HEIC",
			want: Files{Live: "IMG_0001.mov", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "RAW takes the extension of PathRaw as it is",
			p:    photoinfo.PhotoInfo{HasRAW: true, PathRaw: "originals/0/IMG_0001_4.DNG", UTIRaw: "com.panasonic.raw-image"},
			name: "IMG_0001.HEIC",
			want: Files{RAW: "IMG_0001.DNG", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "RAW without PathRaw takes the extension of UTIRaw",
			p:    photoinfo.PhotoInfo{HasRAW: true, UTIRaw: "com.adobe.raw-image"},
			name: "IMG_0001.HEIC",
			want: Files{RAW: "IMG_0001.dng", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "RAW with a UTI the table does not have",
			p:    photoinfo.PhotoInfo{HasRAW: true, UTIRaw: "public.camera-raw-image"},
			name: "IMG_0001.HEIC",
			want: Files{RAW: "IMG_0001.raw", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "RAW without PathRaw and UTIRaw",
			p:    photoinfo.PhotoInfo{HasRAW: true},
			name: "IMG_0001.HEIC",
			want: Files{RAW: "IMG_0001.raw", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "a dot in the directory of PathRaw is no extension",
			p:    photoinfo.PhotoInfo{HasRAW: true, PathRaw: "/a/b.c/IMG"},
			name: "IMG_0001.HEIC",
			want: Files{RAW: "IMG_0001", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "PathRaw without HasRAW gives no RAW",
			p:    photoinfo.PhotoInfo{PathRaw: "originals/0/IMG_0001_4.DNG"},
			name: "IMG_0001.HEIC",
			want: Files{Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "preview takes the first derivative",
			p:    photoinfo.PhotoInfo{PathDerivatives: []string{"derivatives/0/IMG_0001_1.jpeg", "derivatives/0/IMG_0001_2.mov"}},
			name: "IMG_0001.HEIC",
			want: Files{Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "preview keeps the extension of the derivative",
			p:    photoinfo.PhotoInfo{PathDerivatives: []string{"derivatives/0/IMG_0001_1.PNG"}},
			name: "IMG_0001.HEIC",
			want: Files{Preview: "IMG_0001_preview.PNG"},
		},
		{
			desc: "a dot in the directory of the derivative is no extension",
			p:    photoinfo.PhotoInfo{PathDerivatives: []string{"/a/b.c/IMG"}},
			name: "IMG_0001.HEIC",
			want: Files{Preview: "IMG_0001_preview"},
		},
		{
			desc: "adjustments",
			p:    photoinfo.PhotoInfo{HasAdjustments: true},
			name: "IMG_0001.HEIC",
			want: Files{AAE: "IMG_0001.AAE", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "original adjustments",
			p:    photoinfo.PhotoInfo{OriginalAdjustmentsPath: "resources/renders/0/IMG_0001.aae"},
			name: "IMG_0001.HEIC",
			want: Files{OriginalAAE: "IMG_0001_O.AAE", Preview: "IMG_0001_preview.jpeg"},
		},
		{
			desc: "companions follow the name, not the original filename",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.heic", Live: true},
			name: "holiday.jpg",
			want: Files{Live: "holiday.mov", Preview: "holiday_preview.jpeg"},
		},
		{
			desc: "only the last extension is cut",
			p:    photoinfo.PhotoInfo{Live: true},
			name: "my.holiday.jpg",
			want: Files{Live: "my.holiday.mov", Preview: "my.holiday_preview.jpeg"},
		},
		{
			desc: "names are in NFC",
			p: photoinfo.PhotoInfo{
				Live:                    true,
				HasRAW:                  true,
				UTIRaw:                  "com.adobe.raw-image",
				HasAdjustments:          true,
				OriginalAdjustmentsPath: "resources/renders/0/IMG_0001.aae",
			},
			name: nfd,
			want: Files{
				Live:        "Caf\U000000e9.mov",
				RAW:         "Caf\U000000e9.dng",
				Preview:     "Caf\U000000e9_preview.jpeg",
				AAE:         "Caf\U000000e9.AAE",
				OriginalAAE: "Caf\U000000e9_O.AAE",
			},
		},
		{
			desc: "an empty name gives an empty stem",
			p:    photoinfo.PhotoInfo{Live: true},
			name: "",
			want: Files{Live: ".mov", Preview: "_preview.jpeg"},
		},
	}
	for _, tt := range tests {
		if got := Names(&tt.p, tt.name); got != tt.want {
			t.Errorf("%s: Names = %+v, want %+v", tt.desc, got, tt.want)
		}
	}
}

func TestNamesCompanions(t *testing.T) {
	t.Parallel()

	live := photoinfo.PhotoInfo{
		OriginalFilename:        "IMG_0001.HEIC",
		Live:                    true,
		HasAdjustments:          true,
		OriginalAdjustmentsPath: "resources/renders/0/IMG_0001.aae",
	}
	want := Files{
		Live:        "IMG_0001 (1).mov",
		Preview:     "IMG_0001 (1)_preview.jpeg",
		AAE:         "IMG_0001 (1).AAE",
		OriginalAAE: "IMG_0001 (1)_O.AAE",
	}
	if got := Names(&live, "IMG_0001 (1).heic"); got != want {
		t.Errorf("Names of a numbered name = %+v, want %+v", got, want)
	}

	raw := photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", HasRAW: true, UTIRaw: "com.adobe.raw-image"}
	if got := Names(&raw, "IMG_0001_edited.jpeg").RAW; got != "IMG_0001_edited.dng" {
		t.Errorf("RAW of an edited name = %q, want IMG_0001_edited.dng", got)
	}
}

func TestEditedName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc string
		p    photoinfo.PhotoInfo
		want string
	}{
		{
			desc: "extension of PathEdited before the UTI",
			p: photoinfo.PhotoInfo{
				OriginalFilename: "IMG_0001.HEIC",
				PathEdited:       "resources/renders/0/IMG_0001_1_201_a.jpeg",
				UTIEdited:        "public.heic",
				UTI:              "public.heic",
			},
			want: "IMG_0001_edited.jpeg",
		},
		{
			desc: "UTIEdited without PathEdited",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", UTIEdited: "public.heic"},
			want: "IMG_0001_edited.heic",
		},
		{
			desc: "UTI when UTIEdited is empty",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", UTI: "public.jpeg"},
			want: "IMG_0001_edited.jpeg",
		},
		{
			desc: "UTIEdited before UTI",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", UTIEdited: "public.heic", UTI: "public.jpeg"},
			want: "IMG_0001_edited.heic",
		},
		{
			desc: "a UTI the table does not have",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", UTIEdited: "com.example.none"},
			want: "IMG_0001_edited.",
		},
		{
			desc: "only the last extension of the original is cut",
			p:    photoinfo.PhotoInfo{OriginalFilename: "a.b.c", PathEdited: "x/y.jpeg"},
			want: "a.b_edited.jpeg",
		},
		{
			desc: "a dot in the directory of PathEdited is no extension",
			p:    photoinfo.PhotoInfo{OriginalFilename: "IMG_0001.HEIC", PathEdited: "/a/b.c/IMG"},
			want: "IMG_0001_edited",
		},
		{
			desc: "NFC",
			p:    photoinfo.PhotoInfo{OriginalFilename: "Cafe\U00000301.heic", PathEdited: "x/y.jpeg"},
			want: "Caf\U000000e9_edited.jpeg",
		},
	}
	for _, tt := range tests {
		if got := EditedName(&tt.p); got != tt.want {
			t.Errorf("%s: EditedName = %q, want %q", tt.desc, got, tt.want)
		}
	}
}
