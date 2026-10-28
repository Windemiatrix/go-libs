package sqlread

import (
	"context"
	"database/sql"
)

// Resource is one ZINTERNALRESOURCE row (photosdb.py:2449-2459, 2498-2504, 2540-2550).
type Resource struct {
	PK, AssetPK   int64          // Z_PK, ZASSET
	Type, Subtype sql.NullInt64  // ZRESOURCETYPE, ZDATASTORESUBTYPE
	DataLength    sql.NullInt64  // ZDATALENGTH
	Local, Remote sql.NullInt64  // ZLOCALAVAILABILITY, ZREMOTEAVAILABILITY
	Fingerprint   sql.NullString // ZFINGERPRINT
}

// The order is that of the Scan in Resources. ZCOMPACTUTI is not read: it holds
// a small code, not a UTI, and upstream does not use it for Photos 7 and later
// (photosdb.py:2454, 2490).
func resourceSelect() []column {
	r := func(name string) column { return column{"r", "ZINTERNALRESOURCE", name} }
	return []column{
		r("Z_PK"), r("ZASSET"), r("ZRESOURCETYPE"), r("ZDATASTORESUBTYPE"), r("ZDATALENGTH"),
		r("ZLOCALAVAILABILITY"), r("ZREMOTEAVAILABILITY"), r("ZFINGERPRINT"),
	}
}

// Resources returns every row, ordered by ZASSET, then Z_PK; AssetPK is 0 for
// a row without ZASSET, which upstream still joins on ZFINGERPRINT
// (photosdb.py:2497-2516).
func (d *DB) Resources(ctx context.Context) ([]Resource, error) {
	q := "SELECT " + selectList(resourceSelect()) +
		" FROM ZINTERNALRESOURCE r ORDER BY r.ZASSET, r.Z_PK"
	var out []Resource
	err := d.readRows(ctx, "resources", q, func(rows *sql.Rows) error {
		var r Resource
		var asset sql.NullInt64
		if err := rows.Scan(&r.PK, &asset, &r.Type, &r.Subtype, &r.DataLength,
			&r.Local, &r.Remote, &r.Fingerprint); err != nil {
			return err
		}
		r.AssetPK = asset.Int64
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
