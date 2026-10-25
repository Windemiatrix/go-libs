package sqlread

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // the "sqlite" database/sql driver
)

const attempts = 3

// DB is an open, checked, read-only copy of a Photos.sqlite database.
type DB struct {
	db  *sql.DB
	dir string // the copy directory, removed by Close
}

// Tests set afterCopy to damage one attempt (attempts count from 1);
// production code never sets it.
var afterCopy func(attempt int, copyPath string)

// Open copies dbPath and its -wal (when present) into a new directory under
// tempDir ("" is os.TempDir()), opens the copy read-only and runs
// quick_check on every ordinary table. A failed open or check is retried
// with a fresh copy, three attempts in total. Before copying, Open removes
// copies left under tempDir by processes that no longer run.
//
// The -shm file is not copied: SQLite rebuilds the WAL index from -wal on
// the first connection to the copy, and a -shm taken at another moment than
// -wal could only be stale. A tempDir inside the library is refused by the
// caller, photosdb.Open.
func Open(ctx context.Context, dbPath, tempDir string) (*DB, error) {
	src, err := source(dbPath)
	if err != nil {
		return nil, err
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tmp, err := RealPath(tempDir)
	if err != nil {
		return nil, fmt.Errorf("sqlread: temporary directory: %w", err)
	}
	sweep(tmp, os.Getpid(), processAlive)

	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		dir, copyPath, err := makeCopy(ctx, src, tmp)
		if err != nil {
			return nil, err
		}
		if afterCopy != nil {
			afterCopy(attempt, copyPath)
		}
		db, err := openChecked(ctx, copyPath)
		if err == nil {
			return &DB{db: db, dir: dir}, nil
		}
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			return nil, fmt.Errorf("sqlread: %s: %w", dbPath, errors.Join(err, rmErr))
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("sqlread: %w", ctxErr)
		}
		last = err
	}
	return nil, fmt.Errorf("sqlread: %s: quick_check failed after %d attempts: %w", dbPath, attempts, last)
}

// SQLite keeps the -wal next to the real file, so the copy takes it from
// there.
func source(dbPath string) (string, error) {
	src, err := RealPath(dbPath)
	if err != nil {
		return "", fmt.Errorf("sqlread: %w", err)
	}
	return src, nil
}

// RealPath is the absolute path of an existing file with symbolic links
// resolved.
func RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// The DSN is a URI built by net/url, so that a space, "#", "?" or "%" in the
// path cannot end the path or add parameters. The database comes from
// outside, so trusted_schema is off: functions named in its views, triggers
// and indexes run only if SQLite marks them innocuous.
func openChecked(ctx context.Context, path string) (*sql.DB, error) {
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=trusted_schema(0)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := quickCheck(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// A failed scan is reported with the number of the row and without the error
// of database/sql, whose text holds the value.
func (d *DB) readRows(ctx context.Context, reader, query string, scan func(*sql.Rows) error, args ...any) error {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlread: %s: %w", reader, err)
	}
	row := 0
	for rows.Next() {
		row++
		if err := scan(rows); err != nil {
			_ = rows.Close() // the scan error is the one reported
			return fmt.Errorf("sqlread: %s: row %d: unexpected value type", reader, row)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("sqlread: %s: %w", reader, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlread: %s: %w", reader, err)
	}
	return nil
}

// Close closes the database and removes the copy. A second call returns nil.
func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	err := errors.Join(d.db.Close(), os.RemoveAll(d.dir))
	d.db = nil
	return err
}
