package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// The order is that of the Scan in NamedPersonAssets.
func personSelect(s schema.Schema) []column {
	return []column{{"f", "ZDETECTEDFACE", s.FaceAssetColumn}}
}

func personColumns(s schema.Schema) []column {
	return append(personSelect(s),
		column{"f", "ZDETECTEDFACE", s.FacePersonColumn},
		column{"p", "ZPERSON", "Z_PK"},
		column{"p", "ZPERSON", "ZFULLNAME"},
	)
}

// NamedPersonAssets returns, sorted and distinct, the ZASSET.Z_PK of the
// assets with a detected face of a person whose ZFULLNAME is not empty
// (photosdb.py:1790-1796, 1844-1853).
func (d *DB) NamedPersonAssets(ctx context.Context, s schema.Schema) ([]int64, error) {
	q := "SELECT DISTINCT " + selectList(personSelect(s)) + " FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f." +
		s.FacePersonColumn + " WHERE f." + s.FaceAssetColumn +
		" IS NOT NULL AND p.ZFULLNAME IS NOT NULL AND p.ZFULLNAME != '' ORDER BY 1"
	var out []int64
	err := d.readRows(ctx, "named person assets", q, func(rows *sql.Rows) error {
		var pk int64
		if err := rows.Scan(&pk); err != nil {
			return err
		}
		out = append(out, pk)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
