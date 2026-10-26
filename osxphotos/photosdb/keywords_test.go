package photosdb

import (
	"database/sql"
	"maps"
	"slices"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func keywordPhoto(keywords ...string) *photoInfo { return &photoInfo{Keywords: keywords} }

func TestKeywordQueries(t *testing.T) {
	t.Parallel()

	db := &DB{
		photos: []*photoInfo{
			keywordPhoto("b", "b", "a"), keywordPhoto("a", "c"), keywordPhoto(),
			{Keywords: []string{"t"}, InTrash: true},
			{Keywords: []string{"u"}, Burst: true, BurstPickType: 4},
		},
		unusedKeywords: []string{"", "x", "x"},
	}
	if got, want := db.Keywords(), []string{"a", "b", "c", "t", "u"}; !slices.Equal(got, want) {
		t.Errorf("Keywords = %q, want %q", got, want)
	}
	wantCounts := map[string]int{"a": 2, "b": 2, "c": 1, "t": 1, "u": 1, "": 0, "x": 0}
	counts := db.KeywordCounts()
	if !maps.Equal(counts, wantCounts) {
		t.Errorf("KeywordCounts = %v, want %v", counts, wantCounts)
	}
	counts["a"] = 99
	delete(counts, "x")
	if again := db.KeywordCounts(); !maps.Equal(again, wantCounts) {
		t.Errorf("KeywordCounts after a change of the returned map = %v, want %v", again, wantCounts)
	}
	got := db.KeywordsWithoutPhoto()
	if want := []string{"", "x", "x"}; !slices.Equal(got, want) {
		t.Errorf("KeywordsWithoutPhoto = %q, want %q", got, want)
	}
	got[0] = "changed"
	if again := db.KeywordsWithoutPhoto(); again[0] != "" {
		t.Errorf("KeywordsWithoutPhoto shares its slice with the DB: %q", again)
	}

	empty := &DB{}
	if k, c, u := empty.Keywords(), empty.KeywordCounts(), empty.KeywordsWithoutPhoto(); k != nil || c == nil || len(c) != 0 || u != nil {
		t.Errorf("empty DB: Keywords %q, KeywordCounts %v, KeywordsWithoutPhoto %q; want nil slices and a non-nil empty map", k, c, u)
	}
}

func TestKeywordsWithoutPhotoRule(t *testing.T) {
	t.Parallel()

	// Escapes: an editor may normalize a literal itself.
	usedNFD, usedNFC := "Cafe\U00000301", "Caf\U000000e9"
	freeNFD, freeNFC := "Ne\U00000301e", "N\U000000e9e"
	if usedNFD == usedNFC || freeNFD == freeNFC {
		t.Fatal("a pair of the test is not in two forms")
	}
	titles := []sql.NullString{
		nullString(usedNFD), nullString(freeNFD), {}, nullString("z"), nullString("z"), nullString("b"),
	}
	photos := []*photoInfo{keywordPhoto(usedNFC, "b")}

	if got, want := keywordsWithoutPhoto(titles, photos), []string{"", freeNFC, "z", "z"}; !slices.Equal(got, want) {
		t.Errorf("keywordsWithoutPhoto = %q, want %q", got, want)
	}
	if got := keywordsWithoutPhoto(titles[5:], photos); got != nil {
		t.Errorf("every keyword has a photo: keywordsWithoutPhoto = %q, want nil", got)
	}
	if got := keywordsWithoutPhoto(titles[2:3], []*photoInfo{keywordPhoto("")}); got != nil {
		t.Errorf("a NULL title and a photo with the keyword \"\": keywordsWithoutPhoto = %q, want nil", got)
	}
}

// Upstream counts only the links of assets with a ZADDITIONALASSETATTRIBUTES row (photosdb/photosdb.py:1998-2003).
func keywordCheck(t *testing.T, lib, column string) (used, unused int) {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	src := roDB(t, lib)
	linked := "SELECT COALESCE(k.ZTITLE, '') FROM Z_1KEYWORDS j JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = j.Z_1ASSETATTRIBUTES " +
		"JOIN ZASSET a ON a.Z_PK = x.ZASSET JOIN ZKEYWORD k ON k.Z_PK = j." + column

	counts := map[string]int{}
	for _, title := range testlib.Rows(t, src, linked) {
		counts[nfc(title)]++
	}
	var wantUnused []string
	for _, title := range testlib.Rows(t, src, "SELECT COALESCE(ZTITLE, '') FROM ZKEYWORD") {
		if k := nfc(title); counts[k] == 0 {
			wantUnused = append(wantUnused, k)
		}
	}
	slices.Sort(wantUnused)

	if got, want := db.Keywords(), slices.Sorted(maps.Keys(counts)); !slices.Equal(got, want) {
		t.Errorf("Keywords = %q, want %q", got, want)
	}
	if got := db.KeywordsWithoutPhoto(); !slices.Equal(got, wantUnused) {
		t.Errorf("KeywordsWithoutPhoto = %q, want %q", got, wantUnused)
	}
	wantCounts := maps.Clone(counts)
	for _, k := range wantUnused {
		wantCounts[k] = 0
	}
	if got := db.KeywordCounts(); !maps.Equal(got, wantCounts) {
		t.Errorf("KeywordCounts = %v, want %v", got, wantCounts)
	}
	return len(counts), len(wantUnused)
}

func TestKeywordsLibraries(t *testing.T) {
	t.Parallel()

	used, unused, freed := 0, 0, 0
	for _, name := range testlib.Libraries(t) {
		column := schemaByID(t, libraryModels[name].id).KeywordColumn
		t.Run(name, func(t *testing.T) {
			u, n := keywordCheck(t, testlib.OpenPristine(t, name), column)
			used += u
			unused += n
		})
		t.Run(name+" without the links of a keyword", func(t *testing.T) {
			lib := testlib.OpenPristine(t, name)
			change(t, lib, "DELETE FROM Z_1KEYWORDS WHERE "+column+" = (SELECT MIN("+column+") FROM Z_1KEYWORDS)")
			_, n := keywordCheck(t, lib, column)
			freed += n
		})
	}
	t.Logf("%d keywords with photos, %d keywords without photos; %d without photos after the links of one keyword are removed", used, unused, freed)
	if used == 0 || freed == 0 {
		t.Errorf("%d keywords with photos, %d without photos after the removal; want some of each", used, freed)
	}
}
