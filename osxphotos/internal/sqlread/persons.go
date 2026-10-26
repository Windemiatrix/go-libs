package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Face is one detected face of an asset with a person (photosdb.py:1844-1869).
type Face struct {
	AssetPK  int64          // schema.Schema.FaceAssetColumn
	FullName sql.NullString // ZPERSON.ZFULLNAME
}

// The order is that of the Scan in Faces.
func personSelect(s schema.Schema) []column {
	return []column{{"f", "ZDETECTEDFACE", s.FaceAssetColumn}, {"p", "ZPERSON", "ZFULLNAME"}}
}

func personColumns(s schema.Schema) []column {
	return append(personSelect(s),
		column{"f", "ZDETECTEDFACE", s.FacePersonColumn},
		column{"p", "ZPERSON", "Z_PK"},
	)
}

// Faces returns the detected faces that have an asset and a person, one row
// per face, in no order (photosdb.py:1844-1853).
func (d *DB) Faces(ctx context.Context, s schema.Schema) ([]Face, error) {
	q := "SELECT " + selectList(personSelect(s)) + " FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f." +
		s.FacePersonColumn + " WHERE f." + s.FaceAssetColumn + " IS NOT NULL"
	var out []Face
	err := d.readRows(ctx, "faces", q, func(rows *sql.Rows) error {
		var f Face
		if err := rows.Scan(&f.AssetPK, &f.FullName); err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
