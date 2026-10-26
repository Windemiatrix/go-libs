package sqlread

import (
	"context"
	"database/sql"
)

// Values of ZGENERICALBUM.ZKIND (_constants.py:364-369).
const (
	KindUserAlbum     = 2
	KindSharedAlbum   = 1505
	KindImportSession = 1506
	KindProject       = 1508
	KindRootFolder    = 3999
	KindFolder        = 4000
)

// Album is one user album, shared album, import session, project, folder or
// root folder of ZGENERICALBUM (photosdb.py:1904-1948).
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
	Owner               sql.NullString  // the full name of the person of ZCLOUDOWNERHASHEDPERSONID (albuminfo.py:113-133); NULL when not found
}

// The order is that of the Scan in Albums.
func albumSelect() []column {
	g := func(name string) column { return column{"g", "ZGENERICALBUM", name} }
	return []column{
		g("Z_PK"), g("ZUUID"), g("ZKIND"), g("ZPARENTFOLDER"), {"p", "ZGENERICALBUM", "ZUUID"}, g("ZTITLE"), g("ZCLOUDGUID"),
		g("ZTRASHEDSTATE"), g("ZCREATIONDATE"), g("ZSTARTDATE"), g("ZENDDATE"),
		g("ZCUSTOMSORTKEY"), g("ZCUSTOMSORTASCENDING"), g("Z_FOK_PARENTFOLDER"), g("ZCLOUDOWNERHASHEDPERSONID"),
	}
}

// Albums returns the user albums, shared albums, import sessions, projects,
// folders and root folders that have a ZUUID, ordered by Z_PK. A row without
// ZUUID is left out, as Members leaves out its memberships.
func (d *DB) Albums(ctx context.Context) ([]Album, error) {
	names, err := d.personNames(ctx)
	if err != nil {
		return nil, err
	}
	q := "SELECT " + selectList(albumSelect()) +
		" FROM ZGENERICALBUM g LEFT JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER" +
		" WHERE g.ZKIND IN (?, ?, ?, ?, ?, ?) AND g.ZUUID IS NOT NULL ORDER BY g.Z_PK"
	var out []Album
	err = d.readRows(ctx, "albums", q, func(rows *sql.Rows) error {
		var a Album
		var owner sql.NullString
		if err := rows.Scan(&a.PK, &a.UUID, &a.Kind, &a.Parent, &a.ParentUUID, &a.Title, &a.CloudGUID,
			&a.Trashed, &a.Created, &a.Start, &a.End, &a.SortKey, &a.SortAscending, &a.ListOrder, &owner); err != nil {
			return err
		}
		a.Owner = names[owner.String]
		out = append(out, a)
		return nil
	}, KindUserAlbum, KindSharedAlbum, KindImportSession, KindProject, KindRootFolder, KindFolder)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func personNameColumns() []column {
	i := func(name string) column { return column{"", "ZCLOUDSHAREDALBUMINVITATIONRECORD", name} }
	g := func(name string) column { return column{"", "ZGENERICALBUM", name} }
	return []column{
		i("ZINVITEEHASHEDPERSONID"), i("ZINVITEEFIRSTNAME"), i("ZINVITEELASTNAME"), i("ZINVITEEFULLNAME"),
		g("ZCLOUDOWNERHASHEDPERSONID"), g("ZCLOUDOWNERFIRSTNAME"), g("ZCLOUDOWNERLASTNAME"), g("ZCLOUDOWNERFULLNAME"),
	}
}

// The query of _db_hashed_person_id, verbatim (_photosdb_process_comments.py:52-73).
const hashedPersonQuery = `
        SELECT DISTINCT
        ZINVITEEHASHEDPERSONID AS HASHEDPERSONID,
        ZINVITEEFIRSTNAME AS FIRSTNAME,
        ZINVITEELASTNAME AS LASTNAME,
        ZINVITEEFULLNAME AS FULLNAME
        FROM ZCLOUDSHAREDALBUMINVITATIONRECORD
        WHERE HASHEDPERSONID IS NOT NULL
        AND HASHEDPERSONID != ''
        AND NOT (FIRSTNAME IS NULL AND LASTNAME IS NULL)
        UNION
        SELECT DISTINCT
        ZCLOUDOWNERHASHEDPERSONID AS HASHEDPERSONID,
        ZCLOUDOWNERFIRSTNAME AS FIRSTNAME,
        ZCLOUDOWNERLASTNAME AS LASTNAME,
        ZCLOUDOWNERFULLNAME AS FULLNAME
        FROM ZGENERICALBUM
        WHERE HASHEDPERSONID IS NOT NULL
        AND HASHEDPERSONID != ''
        AND NOT (FIRSTNAME IS NULL AND LASTNAME IS NULL)
        `

// personNames is the full_name of _db_hashed_person_id; of several rows of
// one ID the last wins, as upstream (_photosdb_process_comments.py:82-89).
// ORDER BY fixes the order of the UNION as SQLite returns it.
func (d *DB) personNames(ctx context.Context) (map[string]sql.NullString, error) {
	names := map[string]sql.NullString{}
	err := d.readRows(ctx, "person names", hashedPersonQuery+"ORDER BY 1, 2, 3, 4", func(rows *sql.Rows) error {
		var id string
		var first, last, full sql.NullString
		if err := rows.Scan(&id, &first, &last, &full); err != nil {
			return err
		}
		names[id] = full
		return nil
	})
	return names, err
}
