package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// userAlbumKind is ZGENERICALBUM.ZKIND of a user album (_constants.py:364).
const userAlbumKind = 2

// Member is one asset of a user album (ZKIND 2, _constants.py:364), trashed
// albums included (photosdb.py:1873-1901).
type Member struct {
	AssetPK   int64  // schema.Schema.MemberAssetColumn
	AlbumUUID string // ZGENERICALBUM.ZUUID
}

// memberSelect is the select list of Members, in the order of the scan.
func memberSelect(s schema.Schema) []column {
	return []column{{"m", s.MemberTable, s.MemberAssetColumn}, {"g", "ZGENERICALBUM", "ZUUID"}}
}

// memberColumns are the columns of Members: the select list and the join
// and filter columns.
func memberColumns(s schema.Schema) []column {
	return append(memberSelect(s),
		column{"m", s.MemberTable, s.MemberAlbumColumn},
		column{"g", "ZGENERICALBUM", "Z_PK"},
		column{"g", "ZGENERICALBUM", "ZKIND"},
	)
}

// Members returns the memberships of the user albums, ordered by asset, then
// by album. Shared albums (ZKIND 1505) are not user albums and are left out.
func (d *DB) Members(ctx context.Context, s schema.Schema) ([]Member, error) {
	q := "SELECT " + selectList(memberSelect(s)) + " FROM " + s.MemberTable +
		" m JOIN ZGENERICALBUM g ON g.Z_PK = m." + s.MemberAlbumColumn +
		" WHERE g.ZKIND = ? AND m." + s.MemberAssetColumn + " IS NOT NULL AND g.ZUUID IS NOT NULL" +
		" ORDER BY m." + s.MemberAssetColumn + ", g.ZUUID"
	var out []Member
	err := d.readRows(ctx, "members", q, func(rows *sql.Rows) error {
		var m Member
		if err := rows.Scan(&m.AssetPK, &m.AlbumUUID); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	}, userAlbumKind)
	if err != nil {
		return nil, err
	}
	return out, nil
}
