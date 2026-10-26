package photosdb

import (
	"database/sql"
	"maps"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Keywords returns the keywords of the photos, each once, sorted by bytes:
// osxphotos keywords (photosdb/photosdb.py:547-551), which keeps the order of
// the rows. Photos in the trash count. It is nil without keywords.
func (db *DB) Keywords() []string {
	return slices.Sorted(maps.Keys(keywordCounts(db.photos)))
}

// KeywordCounts returns, for each keyword, the number of times photos have
// it, a repeat on one photo counted, with 0 for KeywordsWithoutPhoto: osxphotos
// keywords_as_dict (photosdb/photosdb.py:473-481). Upstream sorts it by count;
// a Go map has no order. The map is new on every call and never nil.
func (db *DB) KeywordCounts() map[string]int {
	counts := keywordCounts(db.photos)
	for _, k := range db.unusedKeywords {
		counts[k] = 0
	}
	return counts
}

// KeywordsWithoutPhoto returns the keywords of the library that no photo has,
// sorted by bytes, a title repeated in the library repeated here: osxphotos
// keywords_without_photo (photosdb/photosdb.py:483-496), which keeps the order
// of the rows. A keyword without a title is "". The slice is new on every
// call; it is nil when every keyword has a photo.
func (db *DB) KeywordsWithoutPhoto() []string {
	return slices.Clone(db.unusedKeywords)
}

func keywordCounts(photos []*photoinfo.PhotoInfo) map[string]int {
	counts := map[string]int{}
	for _, p := range photos {
		for _, k := range p.Keywords {
			counts[k]++
		}
	}
	return counts
}

func keywordsWithoutPhoto(titles []sql.NullString, photos []*photoinfo.PhotoInfo) []string {
	used := keywordCounts(photos)
	var out []string
	for _, t := range titles {
		if k := nfc(t.String); used[k] == 0 {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}
