package sqlread

import (
	"context"
	"database/sql"
)

// Values of ZGENERICALBUM.ZKIND (_constants.py:364-369).
const (
	KindUserAlbum     = 2
	KindImportSession = 1506
	KindProject       = 1508
	KindRootFolder    = 3999
	KindFolder        = 4000
)

// Album is one user album, import session, project, folder or root folder of
// ZGENERICALBUM (photosdb.py:1904-1948).
type Album struct {
	PK                  int64           // Z_PK
	UUID                string          // ZUUID
	Kind                int64           // ZKIND: one of the Kind constants
	Parent              sql.NullInt64   // ZPARENTFOLDER: Z_PK of the enclosing folder
	ParentUUID          sql.NullString  // ZUUID of the ZPARENTFOLDER row, of any ZKIND; NULL without that row or its ZUUID
	Title, CloudGUID    sql.NullString  // ZTITLE, ZCLOUDGUID
	Trashed             sql.NullInt64   // ZTRASHEDSTATE
	Created, Start, End sql.NullFloat64 // ZCREATIONDATE, ZSTARTDATE, ZENDDATE
	SortKey             sql.NullInt64   // ZCUSTOMSORTKEY
	SortAscending       sql.NullInt64   // ZCUSTOMSORTASCENDING
	ListOrder           sql.NullInt64   // Z_FOK_PARENTFOLDER: the place among the rows of one parent folder
}

// The order is that of the Scan in Albums.
func albumSelect() []column {
	g := func(name string) column { return column{"g", "ZGENERICALBUM", name} }
	return []column{
		g("Z_PK"), g("ZUUID"), g("ZKIND"), g("ZPARENTFOLDER"), {"p", "ZGENERICALBUM", "ZUUID"}, g("ZTITLE"), g("ZCLOUDGUID"),
		g("ZTRASHEDSTATE"), g("ZCREATIONDATE"), g("ZSTARTDATE"), g("ZENDDATE"),
		g("ZCUSTOMSORTKEY"), g("ZCUSTOMSORTASCENDING"), g("Z_FOK_PARENTFOLDER"),
	}
}

// Albums returns the user albums, import sessions, projects, folders and root
// folders that have a ZUUID, ordered by Z_PK. A row without ZUUID is left out,
// as Members leaves out its memberships.
func (d *DB) Albums(ctx context.Context) ([]Album, error) {
	q := "SELECT " + selectList(albumSelect()) +
		" FROM ZGENERICALBUM g LEFT JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER" +
		" WHERE g.ZKIND IN (?, ?, ?, ?, ?) AND g.ZUUID IS NOT NULL ORDER BY g.Z_PK"
	var out []Album
	err := d.readRows(ctx, "albums", q, func(rows *sql.Rows) error {
		var a Album
		if err := rows.Scan(&a.PK, &a.UUID, &a.Kind, &a.Parent, &a.ParentUUID, &a.Title, &a.CloudGUID,
			&a.Trashed, &a.Created, &a.Start, &a.End, &a.SortKey, &a.SortAscending, &a.ListOrder); err != nil {
			return err
		}
		out = append(out, a)
		return nil
	}, KindUserAlbum, KindImportSession, KindProject, KindRootFolder, KindFolder)
	if err != nil {
		return nil, err
	}
	return out, nil
}
