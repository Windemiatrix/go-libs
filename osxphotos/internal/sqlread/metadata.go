package sqlread

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Metadata returns Z_PLIST and Z_UUID of the Z_METADATA row with the highest
// Z_VERSION (the row osxphotos reads, photosdb_utils.py:101). A NULL Z_UUID
// is "". Z_PLIST is returned as stored; a NULL Z_PLIST is nil.
func (d *DB) Metadata(ctx context.Context) (plist []byte, libraryUUID string, err error) {
	var id sql.NullString
	err = d.db.QueryRowContext(ctx, "SELECT Z_PLIST, Z_UUID FROM Z_METADATA ORDER BY Z_VERSION DESC LIMIT 1").Scan(&plist, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", errors.New("sqlread: Z_METADATA has no rows")
	}
	if err != nil {
		return nil, "", fmt.Errorf("sqlread: Z_METADATA: %w", err)
	}
	return plist, id.String, nil
}
