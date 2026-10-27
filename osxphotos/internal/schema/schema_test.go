package schema

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestForModelBoundaries pins the model ranges by literal values, not by the
// table itself, so that a typo in data.go is red here.
func TestForModelBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model  int
		wantID string // "" = no schema
	}{
		{0, ""},
		{-1, ""},
		{15999, ""},
		{16000, ""},
		{16319, ""},
		{16320, "photos8"},
		{16999, "photos8"},
		{17000, "photos9"},
		{17599, "photos9"},
		{17600, "photos9_14_6"},
		{17999, "photos9_14_6"},
		{18000, ""},
		{18200, ""},
		{18201, "photos10"},
		{18999, "photos10"},
		{19000, ""},
		{19062, ""},
		{19063, "photos11"},
		{19319, "photos11"},
		{19320, "photos11_1"},
		{19607, "photos11_1"},
		{19999, "photos11_1"},
		{20000, ""},
		{269999999, ""},
		{270000000, "photos12"},
		{270008501, "photos12"},
		{270999999, "photos12"},
		{271000000, ""},
	}
	for _, tt := range tests {
		s, ok := ForModel(tt.model)
		if tt.wantID == "" {
			if ok {
				t.Errorf("ForModel(%d) = %q, true; want no schema", tt.model, s.ID)
			}
			continue
		}
		if !ok || s.ID != tt.wantID {
			t.Errorf("ForModel(%d) = %q, %v; want %q, true", tt.model, s.ID, ok, tt.wantID)
		}
	}

	// The gaps: macOS 15 beta 1 (photos10b1 upstream) and the models between
	// photos10 and photos11.
	for _, gap := range [][2]int{{18000, 18200}, {19000, 19062}} {
		for model := gap[0]; model <= gap[1]; model++ {
			if s, ok := ForModel(model); ok {
				t.Errorf("ForModel(%d) = %q, true; want no schema (gap %d-%d)", model, s.ID, gap[0], gap[1])
			}
		}
	}
}

// The versions are the literals of photosdb_utils.py:138-159, written out
// independently of data.go.
func TestPhotosVersion(t *testing.T) {
	t.Parallel()

	want := map[string]float64{
		"photos8":      8,
		"photos9":      9,
		"photos9_14_6": 9.6,
		"photos10":     10,
		"photos11":     11,
		"photos11_1":   11.1,
		"photos12":     12,
	}
	if len(All()) != len(want) {
		t.Errorf("All() has %d schemas, the literal table %d: add the version of each new schema", len(All()), len(want))
	}
	for _, s := range All() {
		if v, ok := want[s.ID]; !ok || s.PhotosVersion != v {
			t.Errorf("%s.PhotosVersion = %v, want %v (listed: %v)", s.ID, s.PhotosVersion, v, ok)
		}
	}
}

func TestForModelReturnsCopy(t *testing.T) {
	t.Parallel()

	s, ok := ForModel(19607)
	if !ok {
		t.Fatal("ForModel(19607): no schema")
	}
	s.MemberTable = "Z_0ASSETS"
	again, _ := ForModel(19607)
	if again.MemberTable == "Z_0ASSETS" {
		t.Error("a change of the ForModel result is visible in the next call")
	}
}

var namePattern = regexp.MustCompile(`^Z[A-Z0-9_]+$`)

func TestTable(t *testing.T) {
	t.Parallel()

	all := All()
	wantIDs := []string{"photos8", "photos9", "photos9_14_6", "photos10", "photos11", "photos11_1", "photos12"}
	var ids []string
	for i, s := range all {
		ids = append(ids, s.ID)
		if s.MinModel > s.MaxModel {
			t.Errorf("%s: MinModel %d > MaxModel %d", s.ID, s.MinModel, s.MaxModel)
		}
		if i > 0 {
			prev := all[i-1]
			if prev.MinModel >= s.MinModel {
				t.Errorf("%s (min %d) does not follow %s (min %d) in model order", s.ID, s.MinModel, prev.ID, prev.MinModel)
			}
			if prev.MaxModel >= s.MinModel {
				t.Errorf("%s (%d-%d) overlaps %s (%d-%d)", s.ID, s.MinModel, s.MaxModel, prev.ID, prev.MinModel, prev.MaxModel)
			}
		}
		for field, name := range names(s) {
			if !namePattern.MatchString(name) {
				t.Errorf("%s.%s = %q, want a name matching %s", s.ID, field, name, namePattern)
			}
		}
	}
	if !slices.Equal(ids, wantIDs) {
		t.Fatalf("All() IDs = %q, want %q", ids, wantIDs)
	}
	for _, s := range all {
		checkMeaning(t, s)
	}

	all[0].ID = "changed"
	all[0].MemberTable = "Z_0ASSETS"
	if next := All(); next[0].ID != "photos8" || next[0].MemberTable != "Z_28ASSETS" {
		t.Error("a change of the All() result is visible in the next call")
	}
}

var (
	memberTablePattern = regexp.MustCompile(`^Z_[0-9]+ASSETS$`)
	keywordPattern     = regexp.MustCompile(`^Z_[0-9]+KEYWORDS$`)
)

// Written out independently of data.go (_constants.py:129-264): the
// fingerprint and adjustments columns change with photos10, the face columns
// with photos9.
var eraColumns = map[string][4]string{ // fingerprint, adjustments, face person, face asset
	"photos8":      {"ZMASTERFINGERPRINT", "ZHASADJUSTMENTS", "ZPERSON", "ZASSET"},
	"photos9":      {"ZMASTERFINGERPRINT", "ZHASADJUSTMENTS", "ZPERSONFORFACE", "ZASSETFORFACE"},
	"photos9_14_6": {"ZMASTERFINGERPRINT", "ZHASADJUSTMENTS", "ZPERSONFORFACE", "ZASSETFORFACE"},
	"photos10":     {"ZORIGINALSTABLEHASH", "ZADJUSTMENTSSTATE", "ZPERSONFORFACE", "ZASSETFORFACE"},
	"photos11":     {"ZORIGINALSTABLEHASH", "ZADJUSTMENTSSTATE", "ZPERSONFORFACE", "ZASSETFORFACE"},
	"photos11_1":   {"ZORIGINALSTABLEHASH", "ZADJUSTMENTSSTATE", "ZPERSONFORFACE", "ZASSETFORFACE"},
	"photos12":     {"ZORIGINALSTABLEHASH", "ZADJUSTMENTSSTATE", "ZPERSONFORFACE", "ZASSETFORFACE"},
}

// Two names of one table swapped with each other must be red: in the
// database both columns exist, and only the meaning tells them apart.
func checkMeaning(t *testing.T, s Schema) {
	t.Helper()
	if s.MemberAssetColumn != "Z_3ASSETS" {
		t.Errorf("%s.MemberAssetColumn = %q, want Z_3ASSETS", s.ID, s.MemberAssetColumn)
	}
	if s.MemberOrderColumn != "Z_FOK_3ASSETS" {
		t.Errorf("%s.MemberOrderColumn = %q, want Z_FOK_3ASSETS", s.ID, s.MemberOrderColumn)
	}
	if !memberTablePattern.MatchString(s.MemberTable) {
		t.Errorf("%s.MemberTable = %q, want a name matching %s", s.ID, s.MemberTable, memberTablePattern)
	}
	if want := strings.Replace(s.MemberTable, "ASSETS", "ALBUMS", 1); s.MemberAlbumColumn != want {
		t.Errorf("%s.MemberAlbumColumn = %q, want %q (the album column of %s)", s.ID, s.MemberAlbumColumn, want, s.MemberTable)
	}
	if !keywordPattern.MatchString(s.KeywordColumn) {
		t.Errorf("%s.KeywordColumn = %q, want a name matching %s", s.ID, s.KeywordColumn, keywordPattern)
	}
	want, ok := eraColumns[s.ID]
	if !ok {
		t.Errorf("%s: no entry in eraColumns", s.ID)
		return
	}
	got := [4]string{s.FingerprintColumn, s.AdjustmentsColumn, s.FacePersonColumn, s.FaceAssetColumn}
	if got != want {
		t.Errorf("%s: fingerprint, adjustments, face person, face asset columns = %q, want %q", s.ID, got, want)
	}
}

func names(s Schema) map[string]string {
	return map[string]string{
		"MemberTable":       s.MemberTable,
		"MemberAlbumColumn": s.MemberAlbumColumn,
		"MemberAssetColumn": s.MemberAssetColumn,
		"MemberOrderColumn": s.MemberOrderColumn,
		"KeywordColumn":     s.KeywordColumn,
		"FingerprintColumn": s.FingerprintColumn,
		"AdjustmentsColumn": s.AdjustmentsColumn,
		"FacePersonColumn":  s.FacePersonColumn,
		"FaceAssetColumn":   s.FaceAssetColumn,

		"ParticipantShareColumn": s.ParticipantShareColumn,
	}
}

// Written out independently of data.go: ZSHAREPARTICIPANT_ZXX_SHARE of
// _constants.py:144, 161, 178, 212, 229, 246, 263.
func TestParticipantShareColumn(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"photos8":      "Z54_SHARE",
		"photos9":      "Z54_SHARE",
		"photos9_14_6": "Z55_SHARE",
		"photos10":     "Z61_SHARE",
		"photos11":     "Z65_SHARE",
		"photos11_1":   "Z66_SHARE",
		"photos12":     "Z71_SHARE",
	}
	if len(All()) != len(want) {
		t.Errorf("All() has %d schemas, the literal table %d", len(All()), len(want))
	}
	for _, s := range All() {
		if s.ParticipantShareColumn != want[s.ID] {
			t.Errorf("%s.ParticipantShareColumn = %q, want %q", s.ID, s.ParticipantShareColumn, want[s.ID])
		}
	}
}
