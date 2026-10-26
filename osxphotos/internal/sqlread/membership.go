package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Member is one asset of a user album (ZKIND 2), of a shared album (ZKIND
// 1505) or of a project (ZKIND 1508, _constants.py:364-366), trashed ones
// included (photosdb.py:1873-1901).
type Member struct {
	AssetPK   int64         // schema.Schema.MemberAssetColumn
	AlbumUUID string        // ZGENERICALBUM.ZUUID
	Order     sql.NullInt64 // schema.Schema.MemberOrderColumn: the position in the album (ALBUM_SORT_ORDER)
	Kind      int64         // ZGENERICALBUM.ZKIND: KindUserAlbum, KindSharedAlbum or KindProject
}

// The order is that of the Scan in Members.
func memberSelect(s schema.Schema) []column {
	return []column{
		{"m", s.MemberTable, s.MemberAssetColumn}, {"g", "ZGENERICALBUM", "ZUUID"},
		{"m", s.MemberTable, s.MemberOrderColumn}, {"g", "ZGENERICALBUM", "ZKIND"},
	}
}

func memberColumns(s schema.Schema) []column {
	return append(memberSelect(s),
		column{"m", s.MemberTable, s.MemberAlbumColumn},
		column{"g", "ZGENERICALBUM", "Z_PK"},
	)
}

// Members returns the memberships of the user albums, the shared albums and
// the projects, ordered by asset, then by album.
func (d *DB) Members(ctx context.Context, s schema.Schema) ([]Member, error) {
	q := "SELECT " + selectList(memberSelect(s)) + " FROM " + s.MemberTable +
		" m JOIN ZGENERICALBUM g ON g.Z_PK = m." + s.MemberAlbumColumn +
		" WHERE g.ZKIND IN (?, ?, ?) AND m." + s.MemberAssetColumn + " IS NOT NULL AND g.ZUUID IS NOT NULL" +
		" ORDER BY m." + s.MemberAssetColumn + ", g.ZUUID"
	var out []Member
	err := d.readRows(ctx, "members", q, func(rows *sql.Rows) error {
		var m Member
		if err := rows.Scan(&m.AssetPK, &m.AlbumUUID, &m.Order, &m.Kind); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	}, KindUserAlbum, KindSharedAlbum, KindProject)
	if err != nil {
		return nil, err
	}
	return out, nil
}
