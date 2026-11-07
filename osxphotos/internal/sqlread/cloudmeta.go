package sqlread

import (
	"context"
	"database/sql"
)

// CloudMetadata is one ZCLOUDMASTERMEDIAMETADATA row by the asset of its
// master, the row of osxphotos cloud_metadata (photoinfo.py:1747-1753). Data
// is nil for a value over 1 MiB; the rows of one PK share it, so it is not to
// be modified.
type CloudMetadata struct {
	PK, AssetPK int64 // Z_PK, ZASSET.Z_PK of the asset whose ZMASTER is the master of the row
	Data        []byte
}

func cloudMetaSelect() []column {
	md := func(name string) column { return column{"md", "ZCLOUDMASTERMEDIAMETADATA", name} }
	return []column{md("Z_PK"), md("ZCLOUDMASTER"), md("ZDATA")}
}

// CloudMetadata returns the rows whose master is the ZMASTER of an asset,
// ordered by the asset, then Z_PK; a row without an asset is left out, as the
// inner joins of upstream leave it.
func (d *DB) CloudMetadata(ctx context.Context) ([]CloudMetadata, error) {
	// The value comes from outside: one over maxPlistSize of photosdb, which
	// refuses it anyway, stays in SQLite. length() of a text counts characters.
	q := "SELECT md.Z_PK, a.Z_PK, CASE WHEN length(CAST(md.ZDATA AS BLOB)) <= 1048576 THEN md.ZDATA END" +
		" FROM ZCLOUDMASTERMEDIAMETADATA md JOIN ZCLOUDMASTER m ON m.Z_PK = md.ZCLOUDMASTER JOIN ZASSET a ON a.ZMASTER = m.Z_PK" +
		" WHERE md.ZDATA IS NOT NULL ORDER BY a.Z_PK, md.Z_PK"
	var out []CloudMetadata
	// A master of many assets comes once per asset: one copy is kept, or a
	// crafted library would hold a copy of 1 MiB for every asset.
	byPK := map[int64][]byte{}
	err := d.readRows(ctx, "cloud metadata", q, func(rows *sql.Rows) error {
		var r CloudMetadata
		if err := rows.Scan(&r.PK, &r.AssetPK, &r.Data); err != nil {
			return err
		}
		if data, ok := byPK[r.PK]; ok {
			r.Data = data
		} else {
			byPK[r.PK] = r.Data
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
