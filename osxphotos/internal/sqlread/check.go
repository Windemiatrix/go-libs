package sqlread

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// The check of the whole database is not used: since SQLite 3.44 it also
// connects to virtual tables, and fails on a full-text index whose tokenizer
// this SQLite does not have, such as the one that exists only inside the
// Photos app (TestQuickCheckVirtualTable).
func quickCheck(ctx context.Context, db *sql.DB) error {
	tables, err := ordinaryTables(ctx, db)
	if err != nil {
		return err
	}
	for _, table := range tables {
		var res string
		if err := db.QueryRowContext(ctx, "SELECT quick_check FROM pragma_quick_check(?)", table).Scan(&res); err != nil {
			return fmt.Errorf("quick_check %s: %w", table, err)
		}
		if res != "ok" {
			return fmt.Errorf("quick_check %s: %s", table, res)
		}
	}
	return nil
}

func ordinaryTables(ctx context.Context, db *sql.DB) (names []string, err error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	return names, nil
}
