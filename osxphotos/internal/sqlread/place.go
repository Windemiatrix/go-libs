package sqlread

import (
	"context"
	"database/sql"
)

// ReverseLocation is the ZREVERSELOCATIONDATA of one ZADDITIONALASSETATTRIBUTES
// row (photosdb.py:2088, 2120). Data is nil for an archive over 1 MiB.
type ReverseLocation struct {
	AssetPK int64 // ZASSET
	Data    []byte
}

func placeSelect() []column {
	x := func(name string) column { return column{"x", "ZADDITIONALASSETATTRIBUTES", name} }
	return []column{x("Z_PK"), x("ZASSET"), x("ZREVERSELOCATIONDATA")}
}

// ReverseLocations returns the rows that have a ZREVERSELOCATIONDATA, ordered
// by ZASSET, then Z_PK.
func (d *DB) ReverseLocations(ctx context.Context) ([]ReverseLocation, error) {
	// The archive comes from outside: one over maxPlistSize of photosdb, which
	// refuses it anyway, stays in SQLite. length() of a text counts characters.
	q := "SELECT x.ZASSET, CASE WHEN length(CAST(x.ZREVERSELOCATIONDATA AS BLOB)) <= 1048576 THEN x.ZREVERSELOCATIONDATA END" +
		" FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET IS NOT NULL AND x.ZREVERSELOCATIONDATA IS NOT NULL ORDER BY x.ZASSET, x.Z_PK"
	var out []ReverseLocation
	err := d.readRows(ctx, "reverse locations", q, func(rows *sql.Rows) error {
		var r ReverseLocation
		if err := rows.Scan(&r.AssetPK, &r.Data); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
