package sqlread

import (
	"context"
	"database/sql"
	"slices"
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
// root folder of ZGENERICALBUM (photosdb.py:1904-1948), or a shared album of
// ZSHARE (CollectionShares).
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
	OwnerComponents     [][]byte        // the ZSHAREPARTICIPANT.ZNAMECOMPONENTS of an owner ID without a row for Owner, in Z_PK order (_photosdb_process_comments.py:155-187)
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
	people, err := d.personIndex(ctx)
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
		a.Owner, a.OwnerComponents = people.name(owner.String)
		out = append(out, a)
		return nil
	}, KindUserAlbum, KindSharedAlbum, KindImportSession, KindProject, KindRootFolder, KindFolder)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CollectionShares returns the CollectionShare rows of ZSHARE that have a ZUUID,
// in Z_PK order, as albums of KindSharedAlbum whose PK is a ZSHARE.Z_PK, and
// the memberships of their assets by ZCLOUDBATCHPUBLISHDATE, NULL first, then
// Z_PK, numbered in Order. Both are nil without the entity (photosdb.py:1950-1956, 2787-2916).
func (d *DB) CollectionShares(ctx context.Context) ([]Album, []Member, error) {
	var ent sql.NullInt64
	err := d.readRows(ctx, "collection share entity", "SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare' LIMIT 1",
		func(rows *sql.Rows) error { return rows.Scan(&ent) })
	if err != nil || !ent.Valid {
		return nil, nil, err
	}
	// Photos keeps no order of a shared album (photosdb.py:2815-2842).
	var members []Member
	q := "SELECT a.Z_PK, s.ZUUID FROM ZASSET a JOIN ZSHARE s ON s.Z_PK = a.ZCOLLECTIONSHARE" +
		" WHERE s.Z_ENT = ? AND s.ZUUID IS NOT NULL ORDER BY a.ZCLOUDBATCHPUBLISHDATE, a.Z_PK"
	err = d.readRows(ctx, "collection share members", q, func(rows *sql.Rows) error {
		m := Member{Order: sql.NullInt64{Int64: int64(len(members)), Valid: true}, Kind: KindSharedAlbum}
		if err := rows.Scan(&m.AssetPK, &m.AlbumUUID); err != nil {
			return err
		}
		members = append(members, m)
		return nil
	}, ent.Int64)
	if err != nil {
		return nil, nil, err
	}
	people, err := d.personIndex(ctx)
	if err != nil {
		return nil, nil, err
	}
	// Role 1 is the owner (_constants.py:372); of several, the highest Z_PK stands for the last row of upstream (photosdb.py:2844-2861).
	role, owner := "ZROLE", "NULL"
	if slices.Contains(people.participantColumns, "ZCPLROLE") {
		role = "ZCPLROLE"
	}
	if slices.Contains(people.participantColumns, role) && slices.Contains(people.participantColumns, "ZHASHEDPERSONID") {
		owner = "(SELECT p.ZHASHEDPERSONID FROM ZSHAREPARTICIPANT p WHERE p.ZSHARE = s.Z_PK AND p." + role + " = 1 ORDER BY p.Z_PK DESC LIMIT 1)"
	}
	// Upstream reads the state with bool(): NULL is outside the trash, unlike
	// in ZGENERICALBUM (photosdb.py:2906).
	q = "SELECT s.Z_PK, s.ZUUID, s.ZTITLE, COALESCE(s.ZTRASHEDSTATE, 0), s.ZCREATIONDATE, s.ZSTARTDATE, s.ZENDDATE," +
		" s.ZCUSTOMSORTKEY, s.ZCUSTOMSORTASCENDING, " + owner + " FROM ZSHARE s WHERE s.Z_ENT = ? AND s.ZUUID IS NOT NULL ORDER BY s.Z_PK"
	var albums []Album
	err = d.readRows(ctx, "collection shares", q, func(rows *sql.Rows) error {
		a := Album{Kind: KindSharedAlbum}
		var id sql.NullString
		if err := rows.Scan(&a.PK, &a.UUID, &a.Title, &a.Trashed, &a.Created, &a.Start, &a.End, &a.SortKey, &a.SortAscending, &id); err != nil {
			return err
		}
		a.Owner, a.OwnerComponents = people.name(id.String)
		albums = append(albums, a)
		return nil
	}, ent.Int64)
	if err != nil {
		return nil, nil, err
	}
	return albums, members, nil
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

// personIndex is _db_hashed_person_id (_photosdb_process_comments.py:82-89, 155-187).
type personIndex struct {
	names              map[string]sql.NullString
	components         map[string][][]byte
	participantColumns []string
}

// ORDER BY fixes the order of the UNION as SQLite returns it. Z_PK is the order
// of the table scan of the SELECT DISTINCT of upstream; without DISTINCT a
// repeated archive is only tried again, to the same result. The length bound is
// maxPlistSize of photosdb, which refuses a larger archive anyway.
func (d *DB) personIndex(ctx context.Context) (*personIndex, error) {
	if d.people != nil {
		return d.people, nil
	}
	p := &personIndex{names: map[string]sql.NullString{}, components: map[string][][]byte{}}
	err := d.readRows(ctx, "person names", hashedPersonQuery+"ORDER BY 1, 2, 3, 4", func(rows *sql.Rows) error {
		var id string
		var first, last, full sql.NullString
		if err := rows.Scan(&id, &first, &last, &full); err != nil {
			return err
		}
		p.names[id] = full
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.participantColumns, err = d.columns(ctx, "ZSHAREPARTICIPANT")
	if err != nil {
		return nil, err
	}
	if slices.Contains(p.participantColumns, "ZHASHEDPERSONID") && slices.Contains(p.participantColumns, "ZNAMECOMPONENTS") {
		err = d.readRows(ctx, "participant names", "SELECT ZHASHEDPERSONID, ZNAMECOMPONENTS FROM ZSHAREPARTICIPANT "+
			"WHERE ZHASHEDPERSONID IS NOT NULL AND ZHASHEDPERSONID != '' AND ZNAMECOMPONENTS IS NOT NULL AND length(ZNAMECOMPONENTS) <= 1048576 ORDER BY Z_PK",
			func(rows *sql.Rows) error {
				var id string
				var archive []byte
				if err := rows.Scan(&id, &archive); err != nil {
					return err
				}
				p.components[id] = append(p.components[id], archive)
				return nil
			})
		if err != nil {
			return nil, err
		}
	}
	d.people = p
	return p, nil
}

// Upstream adds a participant only under an ID that the UNION does not hold
// (_photosdb_process_comments.py:172-174).
func (p personIndex) name(id string) (sql.NullString, [][]byte) {
	if full, ok := p.names[id]; ok {
		return full, nil
	}
	return sql.NullString{}, p.components[id]
}
