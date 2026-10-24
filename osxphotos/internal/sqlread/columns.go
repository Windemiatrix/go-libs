package sqlread

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ErrMissingColumn is wrapped by CheckColumns for a missing table or column.
var ErrMissingColumn = errors.New("sqlread: missing table or column")

// CheckColumns returns nil when every table of want exists and has every
// listed column (pragma_table_info), otherwise an error wrapping
// ErrMissingColumn that names the first missing element, tables in sorted
// order, columns in the given order. Names are compared exactly, case
// included.
func (d *DB) CheckColumns(ctx context.Context, want map[string][]string) error {
	for _, table := range slices.Sorted(maps.Keys(want)) {
		have, err := d.columns(ctx, table)
		if err != nil {
			return err
		}
		if len(have) == 0 {
			return fmt.Errorf("%w: table %s", ErrMissingColumn, table)
		}
		for _, c := range want[table] {
			if !slices.Contains(have, c) {
				return fmt.Errorf("%w: %s.%s", ErrMissingColumn, table, c)
			}
		}
	}
	return nil
}

// columns returns the column names of an ordinary table, none for a missing
// table. SQLite finds a table name regardless of case, so the table is
// matched in sqlite_master, where "=" compares exactly.
func (d *DB) columns(ctx context.Context, table string) (names []string, err error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT p.name FROM sqlite_master AS m, pragma_table_info(m.name) AS p WHERE m.type = 'table' AND m.name = ?",
		table)
	if err != nil {
		return nil, fmt.Errorf("sqlread: columns of %s: %w", table, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("sqlread: columns of %s: %w", table, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlread: columns of %s: %w", table, err)
	}
	return names, nil
}
