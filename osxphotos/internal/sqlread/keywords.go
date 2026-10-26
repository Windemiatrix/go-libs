package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Keyword is one keyword of an asset (photosdb.py:1998-2003).
type Keyword struct {
	AssetPK int64          // ZADDITIONALASSETATTRIBUTES.ZASSET
	Title   sql.NullString // ZKEYWORD.ZTITLE
}

// The order is that of the Scan in Keywords.
func keywordSelect() []column {
	return []column{{"x", "ZADDITIONALASSETATTRIBUTES", "ZASSET"}, {"k", "ZKEYWORD", "ZTITLE"}}
}

func keywordColumns(s schema.Schema) []column {
	return append(keywordSelect(),
		column{"x", "ZADDITIONALASSETATTRIBUTES", "Z_PK"},
		column{"j", "Z_1KEYWORDS", "Z_1ASSETATTRIBUTES"},
		column{"j", "Z_1KEYWORDS", s.KeywordColumn},
		column{"k", "ZKEYWORD", "Z_PK"},
	)
}

// Keywords returns the keywords of the assets, one row per link of
// Z_1KEYWORDS, ordered by asset, then by the link.
func (d *DB) Keywords(ctx context.Context, s schema.Schema) ([]Keyword, error) {
	q := "SELECT " + selectList(keywordSelect()) + ` FROM ZADDITIONALASSETATTRIBUTES x
JOIN Z_1KEYWORDS j ON j.Z_1ASSETATTRIBUTES = x.Z_PK
JOIN ZKEYWORD k ON k.Z_PK = j.` + s.KeywordColumn + `
WHERE x.ZASSET IS NOT NULL
ORDER BY x.ZASSET, k.Z_PK`
	var out []Keyword
	err := d.readRows(ctx, "keywords", q, func(rows *sql.Rows) error {
		var k Keyword
		if err := rows.Scan(&k.AssetPK, &k.Title); err != nil {
			return err
		}
		out = append(out, k)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// KeywordTitles returns ZKEYWORD.ZTITLE of every keyword, those no asset has
// as well, repeated titles kept (photosdb.py:490). A NULL title is an invalid
// value.
func (d *DB) KeywordTitles(ctx context.Context) ([]sql.NullString, error) {
	var out []sql.NullString
	err := d.readRows(ctx, "keyword titles", "SELECT ZTITLE FROM ZKEYWORD", func(rows *sql.Rows) error {
		var title sql.NullString
		if err := rows.Scan(&title); err != nil {
			return err
		}
		out = append(out, title)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
