package query

import (
	"errors"
	"fmt"
)

// ErrIncompatibleOptions is IncompatibleQueryOptions (photoquery.py:43-44).
var ErrIncompatibleOptions = errors.New("query: incompatible options")

// Validate reports two options that query_options_from_kwargs refuses together
// (photoquery.py:285-327); Apply does not call it, as photo_query does not.
// The check of title, description, place and keyword with their no_ options
// (photoquery.py:329-336) is not ported: its key "no_{option}" lacks the f of
// an f-string, so upstream never refuses anything there.
func (o Options) Validate() error {
	for _, p := range []struct {
		a, b string
		both bool
	}{
		{"burst", "not-burst", o.Burst && o.NotBurst},
		{"cloudasset", "not-cloudasset", o.CloudAsset && o.NotCloudAsset},
		{"edited", "not-edited", o.Edited && o.NotEdited},
		{"favorite", "not-favorite", o.Favorite && o.NotFavorite},
		{"hidden", "not-hidden", o.Hidden && o.NotHidden},
		{"in-album", "not-in-album", o.InAlbum && o.NotInAlbum},
		{"incloud", "not-incloud", o.InCloud && o.NotInCloud},
		{"is-reference", "not-reference", o.IsReference && o.NotReference},
		{"keyword", "no-keyword", len(o.Keyword) > 0 && o.NoKeyword},
		{"live", "not-live", o.Live && o.NotLive},
		{"location", "no-location", o.Location && o.NoLocation},
		{"missing", "not-missing", o.Missing && o.NotMissing},
		{"only-photos", "only-movies", o.OnlyPhotos && o.OnlyMovies},
		{"screenshot", "not-screenshot", o.Screenshot && o.NotScreenshot},
		{"shared", "not-shared", o.Shared && o.NotShared},
		{"deleted", "deleted-only", o.Deleted && o.DeletedOnly},
		{"syndicated", "not-syndicated", o.Syndicated && o.NotSyndicated},
		{"saved-to-library", "not-saved-to-library", o.SavedToLibrary && o.NotSavedToLibrary},
		{"shared-library", "not-shared-library", o.SharedLibrary && o.NotSharedLibrary},
	} {
		if p.both {
			return fmt.Errorf("%w: --%s and --%s are mutually exclusive", ErrIncompatibleOptions, p.a, p.b)
		}
	}
	return nil
}
