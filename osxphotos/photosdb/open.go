package photosdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
)

// Open reads the Photos library at libraryPath (the .photoslibrary
// directory) into memory.
//
// Open reads a copy of database/Photos.sqlite with its -wal file, made in a
// new directory under o.TempDir, never the original. -shm is not copied:
// SQLite rebuilds the index of the log from -wal. An unknown model or a
// missing table or column gives an error that wraps ErrUnsupportedSchema.
// The copy is removed before Open returns, after an error as well; nothing of
// the copy or of the library stays open in the DB.
//
// A TempDir that is the library or lies inside it is refused before any copy
// is made: the copy would become part of the library.
//
// The copy holds the whole database. The caller cancels ctx on SIGINT and
// SIGTERM (signal.NotifyContext), so that an interrupted Open still removes
// it.
func Open(ctx context.Context, libraryPath string, o Options) (db *DB, err error) {
	dbPath := filepath.Join(libraryPath, "database", "Photos.sqlite")
	tmp := o.TempDir
	if tmp == "" {
		tmp = os.TempDir()
	}
	if err := checkTempDir(libraryPath, tmp); err != nil {
		return nil, err
	}
	src, err := sqlread.Open(ctx, dbPath, tmp)
	if err != nil {
		return nil, err
	}
	// The copy is removed on every path, a panic included.
	defer func() {
		if cerr := src.Close(); cerr != nil {
			db, err = nil, errors.Join(err, cerr)
		}
	}()

	plist, libraryUUID, err := src.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	model, s, err := detectSchema(plist)
	if err != nil {
		return nil, err
	}
	if err := src.CheckColumns(ctx, sqlread.Columns(s)); err != nil {
		if errors.Is(err, sqlread.ErrMissingColumn) {
			return nil, fmt.Errorf("%w: %w", ErrUnsupportedSchema, err)
		}
		return nil, err
	}
	t, err := readTables(ctx, src, s)
	if err != nil {
		return nil, err
	}
	c, err := load(t, s.FingerprintColumn)
	if err != nil {
		return nil, err
	}
	albums, err := loadAlbums(t.albums, t.members, t.assets, c.byUUID)
	if err != nil {
		return nil, err
	}
	folders, topFolders, err := loadFolders(t.albums, albums)
	if err != nil {
		return nil, err
	}
	if err := setPaths(libraryPath, t.assets, c); err != nil {
		return nil, err
	}
	return &DB{
		info: Info{
			LibraryPath:   libraryPath,
			ModelVersion:  model,
			SchemaID:      s.ID,
			LibraryUUID:   libraryUUID,
			SkippedAssets: c.skipped,
		},
		photos: c.photos,
		byUUID: c.byUUID,
		albums: albums,
		bursts: indexBursts(c.photos),

		folders:    folders,
		topFolders: topFolders,
	}, nil
}

// The directories are compared as files (os.SameFile), not as path strings,
// so that neither a symbolic link nor another case of a name on a
// case-insensitive file system hides the library. "<library>2" is not inside
// the library.
func checkTempDir(libraryPath, tempDir string) error {
	lib, err := os.Stat(libraryPath)
	if err != nil {
		return fmt.Errorf("photosdb: %w", err)
	}
	tmp, err := sqlread.RealPath(tempDir)
	if err != nil {
		return fmt.Errorf("photosdb: temporary directory: %w", err)
	}
	for d := tmp; ; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil && os.SameFile(fi, lib) {
			return fmt.Errorf("photosdb: the temporary directory %s is inside the library %s", tempDir, libraryPath)
		}
		if filepath.Dir(d) == d {
			return nil
		}
	}
}

func readTables(ctx context.Context, src *sqlread.DB, s schema.Schema) (t tables, err error) {
	if t.assets, err = src.Assets(ctx, s); err != nil {
		return tables{}, err
	}
	if t.resources, err = src.Resources(ctx); err != nil {
		return tables{}, err
	}
	if t.keywords, err = src.Keywords(ctx, s); err != nil {
		return tables{}, err
	}
	if t.persons, err = src.NamedPersonAssets(ctx, s); err != nil {
		return tables{}, err
	}
	if t.members, err = src.Members(ctx, s); err != nil {
		return tables{}, err
	}
	if t.albums, err = src.Albums(ctx); err != nil {
		return tables{}, err
	}
	return t, nil
}
