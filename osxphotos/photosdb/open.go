package photosdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
)

// Open reads the Photos library at libraryPath into memory. libraryPath is
// the .photoslibrary directory or a database file: a file named photos.db, in
// any letter case, stands for the Photos.sqlite beside it. Any other file is
// read as the database; a missing path and a photos.db without Photos.sqlite
// beside it are errors. The library of a database file is the parent of its
// directory, unless o.LibraryPath is set (photosdb/photosdb.py:349-385,
// 452-458).
//
// Open reads a copy of the database with its -wal file, made in a new
// directory under o.TempDir, never the original. -shm is not copied: SQLite
// rebuilds the index of the log from -wal. An unknown model or a missing
// table or column gives an error that wraps ErrUnsupportedSchema. The copy is
// removed before Open returns, after an error as well; nothing of the copy or
// of the library stays open in the DB.
//
// A TempDir inside the library derived from libraryPath, inside
// o.LibraryPath or inside the library of the database file with its symbolic
// links resolved is refused before any copy is made: the copy would become
// part of a library. A database outside a .photoslibrary therefore needs a
// TempDir outside the grandparent directory of the file, which is taken as a
// library even when o.LibraryPath is set.
//
// The copy holds the whole database. The caller cancels ctx on SIGINT and
// SIGTERM (signal.NotifyContext), so that an interrupted Open still removes
// it.
func Open(ctx context.Context, libraryPath string, o Options) (db *DB, err error) {
	dbPath, located, err := locate(libraryPath)
	if err != nil {
		return nil, err
	}
	library := located
	if o.LibraryPath != "" {
		library = o.LibraryPath
	}
	resolved, err := sqlread.RealPath(dbPath)
	if err != nil {
		return nil, fmt.Errorf("photosdb: %w", err)
	}
	tmp := o.TempDir
	if tmp == "" {
		tmp = os.TempDir()
	}
	if err := checkTempDir(tmp, located, library, filepath.Dir(filepath.Dir(resolved))); err != nil {
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
	c, err := load(t)
	if err != nil {
		return nil, err
	}
	titles, err := src.KeywordTitles(ctx)
	if err != nil {
		return nil, err
	}
	albumPhotos := membershipOrder(t.members, t.assets, c.byUUID)
	albums, err := loadAlbums(t.albums, albumPhotos)
	if err != nil {
		return nil, err
	}
	folders, topFolders, err := loadFolders(t.albums, albums)
	if err != nil {
		return nil, err
	}
	if err := setPaths(library, t.assets, c); err != nil {
		return nil, err
	}
	return &DB{
		info: Info{
			LibraryPath:   library,
			DBPath:        dbPath,
			ModelVersion:  model,
			PhotosVersion: s.PhotosVersion,
			SchemaID:      s.ID,
			LibraryUUID:   libraryUUID,
			SkippedAssets: c.skipped,
		},
		photos: c.photos,
		byUUID: c.byUUID,
		albums: albums,
		bursts: indexBursts(c.photos),

		albumPhotos: albumPhotos,

		unusedKeywords: keywordsWithoutPhoto(titles, c.photos),

		folders:    folders,
		topFolders: topFolders,
	}, nil
}

// Upstream resolves symbolic links (photosdb.py:349-350); they are kept here.
func locate(path string) (dbPath, library string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("photosdb: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("photosdb: %w", err)
	}
	if fi.IsDir() {
		return filepath.Join(abs, "database", "Photos.sqlite"), path, nil
	}
	if strings.ToLower(fi.Name()) == "photos.db" {
		abs = filepath.Join(filepath.Dir(abs), "Photos.sqlite")
	}
	return abs, filepath.Dir(filepath.Dir(abs)), nil
}

// The directories are compared as files (os.SameFile), not as path strings,
// so that neither a symbolic link nor another case of a name on a
// case-insensitive file system hides the library. "<library>2" is not inside
// the library.
func checkTempDir(tempDir string, protected ...string) error {
	dirs := make([]os.FileInfo, len(protected))
	for i, p := range protected {
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("photosdb: %w", err)
		}
		dirs[i] = fi
	}
	tmp, err := sqlread.RealPath(tempDir)
	if err != nil {
		return fmt.Errorf("photosdb: temporary directory: %w", err)
	}
	for d := tmp; ; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil {
			for i, dir := range dirs {
				if os.SameFile(fi, dir) {
					return fmt.Errorf("photosdb: the temporary directory %s is inside the library %s", tempDir, protected[i])
				}
			}
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
	if t.faces, err = src.Faces(ctx, s); err != nil {
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
