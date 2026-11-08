package photosdb

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

type searchReader func(*sqlread.DB, context.Context) ([]sqlread.SearchTerm, error)

// Upstream takes psi.sqlite when it exists with a size, else leo.sqlite, else
// nothing with a warning (_photosdb_process_searchinfo.py:154-181). A
// directory of that name has a size and reaches sqlread.Open, which refuses
// what is not a regular file.
func openSearch(ctx context.Context, dbPath, tmp string) ([]sqlread.SearchTerm, error) {
	dir := filepath.Join(filepath.Dir(dbPath), "search")
	for _, search := range []struct {
		name string
		read searchReader
	}{{"psi.sqlite", (*sqlread.DB).SearchTerms}, {"leo.sqlite", (*sqlread.DB).LeoTerms}} {
		path := filepath.Join(dir, search.name)
		size, err := searchSize(path)
		if err != nil {
			return nil, err
		}
		if size > 0 {
			return readSearch(ctx, path, tmp, search.read)
		}
	}
	return nil, nil
}

// A missing file is no search database, as exists() of upstream takes it. Any
// other error of stat fails Open: taking a database that cannot be seen as
// absent would silently lose its terms.
func searchSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("photosdb: search database: %w", err)
	}
	return fi.Size(), nil
}

func readSearch(ctx context.Context, path, tmp string, read searchReader) (terms []sqlread.SearchTerm, err error) {
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
// (photosdb/_photosdb_process_searchinfo.py:359-366): the labels of the search
// database, sorted, each once, nil without any; upstream keeps the order of
// first occurrence.
func (db *DB) Labels() []string { return slices.Sorted(maps.Keys(db.LabelCounts())) }

// LabelsNormalized returns osxphotos labels_normalized
// (photosdb/_photosdb_process_searchinfo.py:369-376) as Labels does, of the
// normalized (lower case) labels.
func (db *DB) LabelsNormalized() []string {
	return slices.Sorted(maps.Keys(db.LabelCountsNormalized()))
}

// LabelCounts returns osxphotos labels_as_dict
// (photosdb/_photosdb_process_searchinfo.py:379-388): each label of the search
// database with its number of records, an asset that has a label twice
// counted twice, as upstream counts; an empty map without labels. A Go map
// does not keep the order by count of upstream. The labels and the normalized
// labels are counted apart, where upstream starts both lists anew when either
// of the two is new (264-269, 346-351).
func (db *DB) LabelCounts() map[string]int { return db.labelCounts(false) }

// LabelCountsNormalized returns osxphotos labels_normalized_as_dict
// (photosdb/_photosdb_process_searchinfo.py:391-399) as LabelCounts does, of
// the normalized labels.
func (db *DB) LabelCountsNormalized() map[string]int { return db.labelCounts(true) }

func (db *DB) labelCounts(normalized bool) map[string]int {
	counts := map[string]int{}
	for _, records := range db.search {
		for _, r := range records {
			if r.Category != searchinfo.CategoryLabel {
				continue
			}
			if normalized {
				counts[r.Normalized]++
			} else {
				counts[r.Content]++
			}
		}
	}
	return counts
}
