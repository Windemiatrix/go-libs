// Package gen writes a synthetic Photos library for tests: a minimal
// .photoslibrary package whose database/Photos.sqlite is created from the DDL
// of a real Photos schema (no data) and filled with generated rows. The same
// Config gives the same database content.
package gen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// DefaultSchema is the schema of DefaultConfig: a file of ddl/ without ".sql".
const DefaultSchema = "photos11_1"

// MaxAssets is the upper bound of Config.Assets.
const MaxAssets = 200_000

var (
	// ErrInvalidConfig is wrapped by every error about the Config, and by the
	// error for an output path that contains "?".
	ErrInvalidConfig = errors.New("gen: invalid config")
	// ErrOutputExists is wrapped by the error of Generate when the output
	// path exists and is not an empty directory.
	ErrOutputExists = errors.New("gen: output path exists and is not an empty directory")
)

// Config describes the library. Shares are fractions of Assets in [0, 1]; the
// number of assets of a kind is 0 for a zero share, otherwise
// max(1, round(share × Assets)), so every kind with a non-zero share is
// present whenever Assets > 0. The rules that these counts and
// SameContentPairs fit into Assets are checked only when Assets > 0.
type Config struct {
	// Schema is the DDL name in ddl/, e.g. "photos11_1".
	Schema string

	// Assets is the number of ZASSET rows of every kind, 0..MaxAssets; 0
	// gives an empty library (the root folder only).
	Assets int

	// Seed seeds every random choice.
	Seed uint64

	// Mutually exclusive kinds; the rest are ordinary photos.
	Videos, LivePhotos, Screenshots, RAWPairs, BurstFrames float64

	// Flags; Hidden and Trashed never overlap.
	Trashed, Hidden, Favorites, Edited float64

	// CloudOnly: every resource of the asset is only in iCloud.
	CloudOnly float64

	// SharedLibrary: the asset is in the shared library scope.
	SharedLibrary float64

	// Albums: the rest of the visible assets is in exactly one album.
	NoAlbum, SeveralAlbums float64

	// Original file names.
	DuplicateNames, NonASCIINames float64

	// Creation dates.
	SameSecond, ZeroDates float64

	// SameContentPairs is the number of pairs of ordinary photos with one
	// fingerprint, UTI and size.
	SameContentPairs int

	// AssetsPerAlbum and AlbumsPerFolder give the numbers of albums,
	// max(4, ⌈Assets / AssetsPerAlbum⌉), and folders,
	// max(1, ⌈albums / AlbumsPerFolder⌉).
	AssetsPerAlbum, AlbumsPerFolder int
}

// DefaultConfig returns DefaultSchema with 1000 assets, seed 1 and shares
// close to a real library (rounded) or synthetic.
func DefaultConfig() Config {
	return Config{
		Schema:           DefaultSchema,
		Assets:           1000,
		Seed:             1,
		Videos:           0.05,
		LivePhotos:       0.003,
		Screenshots:      0.003,
		RAWPairs:         0.04,
		BurstFrames:      0.001,
		Trashed:          0.02,
		Hidden:           0.001,
		Favorites:        0.001,
		Edited:           0.1,
		CloudOnly:        0.9,
		SharedLibrary:    0.65,
		NoAlbum:          0.45,
		SeveralAlbums:    0.01,
		DuplicateNames:   0.2,
		NonASCIINames:    0.03,
		SameSecond:       0.1,
		ZeroDates:        0.004,
		SameContentPairs: 1,
		AssetsPerAlbum:   40,
		AlbumsPerFolder:  25,
	}
}

// Generate writes the library into dir: dir must not exist (its parent must)
// or must be an empty directory. On failure it removes what it created and
// returns the error.
func Generate(ctx context.Context, dir string, cfg Config) error {
	sc, err := cfg.validate()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	// modernc.org/sqlite takes "?" in a file name as the start of parameters.
	if strings.Contains(abs, "?") {
		return fmt.Errorf("%w: the output path contains '?': %s", ErrInvalidConfig, abs)
	}
	d, err := loadDDL(cfg.Schema)
	if err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	p := buildPlan(cfg)
	out, err := createOutput(abs)
	if err != nil {
		return err
	}
	if err := write(ctx, out.db, sc, d, p); err != nil {
		return errors.Join(fmt.Errorf("gen: %w", err), out.remove())
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidConfig}, args...)...)
}

// sumSlack absorbs the rounding of float64 sums of shares such as 0.99 + 0.01.
const sumSlack = 1e-9

// validate checks the Config before anything touches the file system.
func (cfg Config) validate() (schema, error) {
	sc, ok := schemas[cfg.Schema]
	if !ok {
		return schema{}, invalid("unknown schema %q", cfg.Schema)
	}
	if cfg.Assets < 0 || cfg.Assets > MaxAssets {
		return schema{}, invalid("Assets %d is not in [0, %d]", cfg.Assets, MaxAssets)
	}
	shares := []struct {
		name  string
		value float64
	}{
		{"Videos", cfg.Videos}, {"LivePhotos", cfg.LivePhotos}, {"Screenshots", cfg.Screenshots},
		{"RAWPairs", cfg.RAWPairs}, {"BurstFrames", cfg.BurstFrames}, {"Trashed", cfg.Trashed},
		{"Hidden", cfg.Hidden}, {"Favorites", cfg.Favorites}, {"Edited", cfg.Edited},
		{"CloudOnly", cfg.CloudOnly}, {"SharedLibrary", cfg.SharedLibrary}, {"NoAlbum", cfg.NoAlbum},
		{"SeveralAlbums", cfg.SeveralAlbums}, {"DuplicateNames", cfg.DuplicateNames},
		{"NonASCIINames", cfg.NonASCIINames}, {"SameSecond", cfg.SameSecond}, {"ZeroDates", cfg.ZeroDates},
	}
	for _, s := range shares {
		if math.IsNaN(s.value) || s.value < 0 || s.value > 1 {
			return schema{}, invalid("%s %v is not in [0, 1]", s.name, s.value)
		}
	}
	if sum := cfg.Videos + cfg.LivePhotos + cfg.Screenshots + cfg.RAWPairs + cfg.BurstFrames; sum > 1+sumSlack {
		return schema{}, invalid("the shares of the kinds add up to %v, more than 1", sum)
	}
	if cfg.Hidden+cfg.Trashed > 1+sumSlack {
		return schema{}, invalid("Hidden + Trashed is %v, more than 1", cfg.Hidden+cfg.Trashed)
	}
	if cfg.NoAlbum+cfg.SeveralAlbums > 1+sumSlack {
		return schema{}, invalid("NoAlbum + SeveralAlbums is %v, more than 1", cfg.NoAlbum+cfg.SeveralAlbums)
	}
	if cfg.AssetsPerAlbum < 1 {
		return schema{}, invalid("AssetsPerAlbum %d is less than 1", cfg.AssetsPerAlbum)
	}
	if cfg.AlbumsPerFolder < 1 {
		return schema{}, invalid("AlbumsPerFolder %d is less than 1", cfg.AlbumsPerFolder)
	}
	if cfg.SameContentPairs < 0 {
		return schema{}, invalid("SameContentPairs %d is negative", cfg.SameContentPairs)
	}
	n := cfg.Assets
	if n == 0 {
		return sc, nil // an empty library: nothing has to fit
	}
	c := countsOf(cfg)
	if need := n - c.photos + 2*cfg.SameContentPairs; need > n {
		return schema{}, invalid("the kinds (%d videos, %d live photos, %d screenshots, %d RAW pairs, "+
			"%d burst frames) and %d pairs of the same content need %d assets, have %d",
			c.videos, c.live, c.screenshots, c.raw, c.frames, cfg.SameContentPairs, need, n)
	}
	if need := c.trashed + c.hidden; need > n {
		return schema{}, invalid("%d trashed and %d hidden assets need %d assets, have %d", c.trashed, c.hidden, need, n)
	}
	// Album 1 holds exactly one asset, besides the assets outside albums
	// (hidden burst frames among them) and the ones in several albums.
	if need := c.outside + 1 + c.several; need > n {
		return schema{}, invalid("%d assets outside albums, 1 in album 1 and %d in several albums need %d assets, have %d",
			c.outside, c.several, need, n)
	}
	return sc, nil
}

// output is the library directory being created; created lists what this
// call created, in order, for the cleanup.
type output struct {
	db      string
	created []string
}

// createOutput creates dir (or takes an empty one), its three directories
// and an empty database file.
func createOutput(dir string) (*output, error) {
	out := &output{db: filepath.Join(dir, "database", "Photos.sqlite")}
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Mkdir, not MkdirAll: nothing is created above dir.
		if err := os.Mkdir(dir, 0o700); err != nil {
			return nil, fmt.Errorf("gen: %w", err)
		}
		out.created = append(out.created, dir)
	case err != nil:
		return nil, fmt.Errorf("gen: %w", err)
	case !info.IsDir(): // a file or a symbolic link, even to a directory
		return nil, fmt.Errorf("%w: %s", ErrOutputExists, dir)
	default:
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("gen: %w", err)
		}
		if len(entries) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrOutputExists, dir)
		}
	}
	for _, name := range []string{"database", "originals", "resources"} {
		p := filepath.Join(dir, name)
		if err := os.Mkdir(p, 0o700); err != nil {
			return nil, errors.Join(fmt.Errorf("gen: %w", err), out.remove())
		}
		out.created = append(out.created, p)
	}
	// SQLite opens an empty file as a new database and gives its journal the
	// same permissions.
	f, err := os.OpenFile(out.db, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the path is dir and fixed names
	if err != nil {
		return nil, errors.Join(fmt.Errorf("gen: %w", err), out.remove())
	}
	out.created = append(out.created, out.db)
	if err := f.Close(); err != nil {
		return nil, errors.Join(fmt.Errorf("gen: %w", err), out.remove())
	}
	return out, nil
}

// remove deletes what this call created, newest first, and the rollback
// journal of the database; it never removes anything recursively.
func (o *output) remove() error {
	var errs []error
	if err := os.Remove(o.db + "-journal"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	for i := len(o.created) - 1; i >= 0; i-- {
		if err := os.Remove(o.created[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
