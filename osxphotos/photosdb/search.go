package photosdb

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

// Upstream takes psi.sqlite when it exists with a size, else leo.sqlite, else
// nothing with a warning (_photosdb_process_searchinfo.py:154-181). A
// directory of that name has a size and reaches sqlread.Open, which refuses
// what is not a regular file.
func openSearch(ctx context.Context, dbPath, tmp string) ([]sqlread.SearchTerm, error) {
	dir := filepath.Join(filepath.Dir(dbPath), "search")
	if psi := filepath.Join(dir, "psi.sqlite"); hasSize(psi) {
		return readSearch(ctx, psi, tmp, (*sqlread.DB).SearchTerms)
	}
	if leo := filepath.Join(dir, "leo.sqlite"); hasSize(leo) {
		return readSearch(ctx, leo, tmp, (*sqlread.DB).LeoTerms)
	}
	return nil, nil
}

func hasSize(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

func readSearch(ctx context.Context, path, tmp string, read func(*sqlread.DB, context.Context) ([]sqlread.SearchTerm, error)) (terms []sqlread.SearchTerm, err error) {
	src, err := sqlread.Open(ctx, path, tmp)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, src.Close()) }()
	terms, err = read(src, ctx)
	if errors.Is(err, sqlread.ErrMissingColumn) {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedSchema, err)
	}
	return terms, err
}

// SearchInfoOf returns the search terms of p, osxphotos search_info
// (photoinfo.py:1539-1552; searchinfo.py): the records of psi.sqlite, or of
// leo.sqlite when psi.sqlite is missing or empty, read once in Open
// (photosdb/_photosdb_process_searchinfo.py:126-181). A photo without
// records, and every photo of a library without a search database, has a zero
// SearchInfo. SearchInfoOf(p).Normalized() is search_info_normalized.
func (db *DB) SearchInfoOf(p *photoinfo.PhotoInfo) searchinfo.SearchInfo {
	return searchinfo.New(db.search[p.UUID])
}

// Labels returns osxphotos PhotosDB.labels
// (photosdb/_photosdb_process_searchinfo.py:359-366): the labels of the photos
// of the library, sorted, each once, nil without any; upstream keeps the order
// of first occurrence.
func (db *DB) Labels() []string { return slices.Sorted(maps.Keys(db.LabelCounts())) }

// LabelsNormalized returns osxphotos labels_normalized
// (photosdb/_photosdb_process_searchinfo.py:369-376) as Labels does, of the
// normalized (lower case) labels.
func (db *DB) LabelsNormalized() []string {
	return slices.Sorted(maps.Keys(db.LabelCountsNormalized()))
}

// LabelCounts returns osxphotos labels_as_dict
// (photosdb/_photosdb_process_searchinfo.py:379-388): each label of the photos
// of the library with its number of records, a photo that has a label twice
// counted twice, as upstream counts; an empty map without labels. A Go map
// does not keep the order by count of upstream.
func (db *DB) LabelCounts() map[string]int { return db.labelCounts(false) }

// LabelCountsNormalized returns osxphotos labels_normalized_as_dict
// (photosdb/_photosdb_process_searchinfo.py:391-399) as LabelCounts does, of
// the normalized labels.
func (db *DB) LabelCountsNormalized() map[string]int { return db.labelCounts(true) }

func (db *DB) labelCounts(normalized bool) map[string]int {
	counts := map[string]int{}
	for _, records := range db.search {
		for _, r := range records {
			label := r.Content
			if normalized {
				label = r.Normalized
			}
			if r.Category == searchinfo.CategoryLabel {
				counts[label]++
			}
		}
	}
	return counts
}
