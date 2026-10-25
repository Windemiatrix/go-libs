package query

import (
	"errors"
	"testing"
)

// validatePair is two options that photoquery.py:285-318 refuses together,
// a and b as the command line writes them.
type validatePair struct {
	a, b string
	x, y func(*Options)
}

func TestValidate(t *testing.T) {
	t.Parallel()

	pairs := []validatePair{
		{"burst", "not-burst", func(o *Options) { o.Burst = true }, func(o *Options) { o.NotBurst = true }},
		{"cloudasset", "not-cloudasset", func(o *Options) { o.CloudAsset = true }, func(o *Options) { o.NotCloudAsset = true }},
		{"edited", "not-edited", func(o *Options) { o.Edited = true }, func(o *Options) { o.NotEdited = true }},
		{"favorite", "not-favorite", func(o *Options) { o.Favorite = true }, func(o *Options) { o.NotFavorite = true }},
		{"hidden", "not-hidden", func(o *Options) { o.Hidden = true }, func(o *Options) { o.NotHidden = true }},
		{"in-album", "not-in-album", func(o *Options) { o.InAlbum = true }, func(o *Options) { o.NotInAlbum = true }},
		{"incloud", "not-incloud", func(o *Options) { o.InCloud = true }, func(o *Options) { o.NotInCloud = true }},
		{"is-reference", "not-reference", func(o *Options) { o.IsReference = true }, func(o *Options) { o.NotReference = true }},
		{"keyword", "no-keyword", func(o *Options) { o.Keyword = []string{"Beach"} }, func(o *Options) { o.NoKeyword = true }},
		{"live", "not-live", func(o *Options) { o.Live = true }, func(o *Options) { o.NotLive = true }},
		{"location", "no-location", func(o *Options) { o.Location = true }, func(o *Options) { o.NoLocation = true }},
		{"missing", "not-missing", func(o *Options) { o.Missing = true }, func(o *Options) { o.NotMissing = true }},
		{"only-photos", "only-movies", func(o *Options) { o.OnlyPhotos = true }, func(o *Options) { o.OnlyMovies = true }},
		{"screenshot", "not-screenshot", func(o *Options) { o.Screenshot = true }, func(o *Options) { o.NotScreenshot = true }},
		{"shared", "not-shared", func(o *Options) { o.Shared = true }, func(o *Options) { o.NotShared = true }},
		{"deleted", "deleted-only", func(o *Options) { o.Deleted = true }, func(o *Options) { o.DeletedOnly = true }},
		{"syndicated", "not-syndicated", func(o *Options) { o.Syndicated = true }, func(o *Options) { o.NotSyndicated = true }},
		{"saved-to-library", "not-saved-to-library", func(o *Options) { o.SavedToLibrary = true }, func(o *Options) { o.NotSavedToLibrary = true }},
		{"shared-library", "not-shared-library", func(o *Options) { o.SharedLibrary = true }, func(o *Options) { o.NotSharedLibrary = true }},
	}
	for _, p := range pairs {
		t.Run(p.a, func(t *testing.T) {
			t.Parallel()
			var x, y, both Options
			p.x(&x)
			p.y(&y)
			p.x(&both)
			p.y(&both)

			if err := x.Validate(); err != nil {
				t.Errorf("--%s alone: %v", p.a, err)
			}
			if err := y.Validate(); err != nil {
				t.Errorf("--%s alone: %v", p.b, err)
			}
			err := both.Validate()
			if !errors.Is(err, ErrIncompatibleOptions) {
				t.Fatalf("Validate = %v, want ErrIncompatibleOptions", err)
			}
			if want := "query: incompatible options: --" + p.a + " and --" + p.b + " are mutually exclusive"; err.Error() != want {
				t.Errorf("Validate = %q, want %q", err, want)
			}
		})
	}
}

func TestValidateCompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		o    Options
	}{
		{"zero value", Options{}},
		{"a flag and the not of another", Options{Burst: true, NotCloudAsset: true, Favorite: true, NotHidden: true}},
		{"a keyword and no title", Options{Keyword: []string{"Beach"}, NoTitle: true}},
		{"no keyword without a keyword", Options{NoKeyword: true, Keyword: []string{}}},
		{"deleted only with only photos", Options{DeletedOnly: true, OnlyPhotos: true}},
		{"HasRAW has no pair", Options{HasRAW: true}},
		{"values", Options{Year: []int{2020}, MinSize: 1, MaxSize: 2, Album: []string{"A"}, Folder: []string{"F"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.o.Validate(); err != nil {
				t.Errorf("Validate = %v, want nil", err)
			}
		})
	}
}

// TestValidateFirstPair checks the order of photoquery.py:285-318: the first
// pair of the list is the one named.
func TestValidateFirstPair(t *testing.T) {
	t.Parallel()

	o := Options{Favorite: true, NotFavorite: true, Burst: true, NotBurst: true, SharedLibrary: true, NotSharedLibrary: true}
	err := o.Validate()
	if want := "query: incompatible options: --burst and --not-burst are mutually exclusive"; err == nil || err.Error() != want {
		t.Errorf("Validate = %v, want %q", err, want)
	}
}
