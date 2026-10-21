package gen

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// tree lists the paths under dir with their permissions, "d" for directories.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		kind := "f"
		if d.IsDir() {
			kind = "d"
		}
		out = append(out, kind+" "+info.Mode().Perm().String()+" "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOutputLayout(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 30
	dir, _ := generate(t, cfg)
	want := []string{
		"d -rwx------ .",
		"d -rwx------ database",
		"f -rw------- database/Photos.sqlite",
		"d -rwx------ originals",
		"d -rwx------ resources",
	}
	if got := tree(t, dir); !slices.Equal(got, want) {
		t.Errorf("library tree:\n got %q\nwant %q", got, want)
	}
}

func TestOutputExistingDir(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 30

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := Generate(t.Context(), dir, cfg); err != nil {
			t.Fatalf("Generate into an empty directory: %v", err)
		}
		if _, err := os.Stat(dbPath(dir)); err != nil {
			t.Error(err)
		}
	})
	t.Run("directory with a file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := tree(t, dir)
		if err := Generate(t.Context(), dir, cfg); !errors.Is(err, ErrOutputExists) {
			t.Fatalf("Generate into a directory with a file: got %v, want ErrOutputExists", err)
		}
		if got := tree(t, dir); !slices.Equal(got, before) {
			t.Errorf("the directory changed:\n got %q\nwant %q", got, before)
		}
	})
	t.Run("file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Generate(t.Context(), path, cfg); !errors.Is(err, ErrOutputExists) {
			t.Fatalf("Generate into a file: got %v, want ErrOutputExists", err)
		}
	})
	t.Run("symlink to a directory", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(parent, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := Generate(t.Context(), link, cfg); !errors.Is(err, ErrOutputExists) {
			t.Fatalf("Generate into a symlink: got %v, want ErrOutputExists", err)
		}
		if got := tree(t, target); len(got) != 1 {
			t.Errorf("the link target changed: %q", got)
		}
	})
	t.Run("no parent", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		dir := filepath.Join(parent, "missing", "lib")
		if err := Generate(t.Context(), dir, cfg); err == nil {
			t.Fatal("Generate without a parent directory: no error")
		}
		if got := tree(t, parent); len(got) != 1 {
			t.Errorf("something was created: %q", got)
		}
	})
	t.Run("question mark", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		dir := filepath.Join(parent, "lib?mode=ro")
		if err := Generate(t.Context(), dir, cfg); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Generate into a path with '?': got %v, want ErrInvalidConfig", err)
		}
		if got := tree(t, parent); len(got) != 1 {
			t.Errorf("something was created: %q", got)
		}
	})
}

// TestOutputRelative is not parallel: it changes the working directory.
func TestOutputRelative(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Assets = 30
	dir := t.TempDir()
	t.Chdir(dir)
	if err := Generate(t.Context(), "lib", cfg); err != nil {
		t.Fatalf("Generate into a relative path: %v", err)
	}
	if _, err := os.Stat(dbPath(filepath.Join(dir, "lib"))); err != nil {
		t.Error(err)
	}
}

// TestOutputCanceled: a canceled context fails Generate after it has created
// the files, and Generate removes them.
func TestOutputCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cfg := DefaultConfig()
	cfg.Assets = 30

	parent := t.TempDir()
	dir := filepath.Join(parent, "lib")
	if err := Generate(ctx, dir, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate with a canceled context: got %v, want context.Canceled", err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the output directory is left behind: %v", err)
	}

	empty := t.TempDir()
	if err := Generate(ctx, empty, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate with a canceled context: got %v, want context.Canceled", err)
	}
	if got := tree(t, empty); len(got) != 1 {
		t.Errorf("the empty directory is not empty after the failure: %q", got)
	}
}

// journalCtx cancels itself the first time Err is called while the rollback
// journal of the database exists, that is inside the insert transaction:
// it stands in for Ctrl-C in the middle of the inserts.
type journalCtx struct {
	context.Context
	cancel   context.CancelFunc
	journal  string
	canceled atomic.Bool
}

func (c *journalCtx) Err() error {
	if _, err := os.Lstat(c.journal); err == nil {
		c.canceled.Store(true)
		c.cancel()
	}
	return c.Context.Err()
}

// TestOutputCanceledInsert: a context canceled between the inserts makes
// Generate roll back the transaction and remove everything it created.
// Whoever checks the context first inside the transaction, database/sql or
// the generator, cancels it.
func TestOutputCanceledInsert(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 2 * ctxCheckEvery // past the generator's own check at asset 1000
	parent := t.TempDir()
	dir := filepath.Join(parent, "lib")
	inner, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &journalCtx{Context: inner, cancel: cancel, journal: dbPath(dir) + "-journal"}
	if err := Generate(ctx, dir, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate canceled in the inserts: got %v, want context.Canceled", err)
	}
	if !ctx.canceled.Load() {
		t.Fatal("the context was never checked inside the insert transaction")
	}
	if got := tree(t, parent); len(got) != 1 {
		t.Errorf("something is left behind: %q", got)
	}
}

func TestConfigInvalid(t *testing.T) {
	t.Parallel()
	with := func(f func(*Config)) Config {
		c := DefaultConfig()
		f(&c)
		return c
	}
	tests := []struct {
		name string
		cfg  Config
		msg  string // part of the error message
	}{
		{"unknown schema", with(func(c *Config) { c.Schema = "photos0" }), "photos0"},
		{"empty schema", with(func(c *Config) { c.Schema = "" }), "schema"},
		{"negative assets", with(func(c *Config) { c.Assets = -1 }), "Assets"},
		{"too many assets", with(func(c *Config) { c.Assets = MaxAssets + 1 }), "Assets"},
		{"NaN share", with(func(c *Config) { c.Videos = math.NaN() }), "Videos"},
		{"infinite share", with(func(c *Config) { c.CloudOnly = math.Inf(1) }), "CloudOnly"},
		{"negative share", with(func(c *Config) { c.Favorites = -0.1 }), "Favorites"},
		{"share above 1", with(func(c *Config) { c.ZeroDates = 1.1 }), "ZeroDates"},
		{"kinds above 1", with(func(c *Config) { c.Videos, c.RAWPairs = 0.6, 0.5 }), "kinds"},
		{"hidden and trashed above 1", with(func(c *Config) { c.Hidden, c.Trashed = 0.6, 0.5 }), "Hidden"},
		{"albums above 1", with(func(c *Config) { c.NoAlbum, c.SeveralAlbums = 0.9, 0.2 }), "NoAlbum"},
		{"no assets per album", with(func(c *Config) { c.AssetsPerAlbum = 0 }), "AssetsPerAlbum"},
		{"no albums per folder", with(func(c *Config) { c.AlbumsPerFolder = 0 }), "AlbumsPerFolder"},
		{"negative pairs", with(func(c *Config) { c.SameContentPairs = -1 }), "SameContentPairs"},
		{"negative pairs, empty", with(func(c *Config) { c.Assets, c.SameContentPairs = 0, -1 }), "SameContentPairs"},
		// 30 assets: 2 videos, 1 live, 1 screenshot, 1 RAW pair, 5 burst frames; 20 photos
		{"too many pairs", with(func(c *Config) { c.Assets, c.SameContentPairs = 30, 11 }), "need 32"},
		// 1 video, 1 live, 1 screenshot, 1 RAW pair, 5 burst frames, 2 for the pair
		{"kinds do not fit", with(func(c *Config) { c.Assets = 10 }), "need 11"},
		{"flags do not fit", with(func(c *Config) {
			*c = zeroConfig(1)
			c.Trashed, c.Hidden = 0.5, 0.5
		}), "need 2"},
		// 15 outside albums, 1 in album 1, 1 in several, of 15
		{"albums do not fit", with(func(c *Config) { c.Assets, c.NoAlbum = 15, 0.99 }), "need 17"},
		{"no album 1", with(func(c *Config) { *c = zeroConfig(10); c.NoAlbum = 1 }), "need 11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "lib")
			err := Generate(t.Context(), dir, tt.cfg)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("got %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("error %q does not mention %q", err, tt.msg)
			}
			if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the output directory was created: %v", err)
			}
		})
	}
}

func TestConfigValid(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 11} {
		cfg := zeroConfig(n)
		cfg.SameContentPairs = n / 2
		dir := filepath.Join(t.TempDir(), "lib")
		if err := Generate(t.Context(), dir, cfg); err != nil {
			t.Errorf("Generate %d assets: %v", n, err)
		}
	}
	// The smallest default library: the kinds, the pair and the albums fit.
	cfg := DefaultConfig()
	cfg.Assets = 12
	_, db := generate(t, cfg)
	assertInvariants(t, db, schemas[DefaultSchema], false)
}
