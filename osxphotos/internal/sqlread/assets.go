package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Asset is one ZASSET row with its ZADDITIONALASSETATTRIBUTES,
// ZASSETDESCRIPTION and ZCLOUDMASTER rows (LEFT JOIN): the columns of
// photosdb.py:2023-2090, 2404-2412, 2521-2529,
// _photosdb_process_shared_library.py:43-51 and
// _photosdb_process_syndicationinfo.py:42-51 in one query.
type Asset struct {
	PK                                         int64           // ZASSET.Z_PK
	UUID, Filename, Directory, UTI             sql.NullString  // ZUUID, ZFILENAME, ZDIRECTORY, ZUNIFORMTYPEIDENTIFIER
	Kind, KindSubtype                          sql.NullInt64   // ZKIND, ZKINDSUBTYPE
	Created, Modified, Adjusted                sql.NullFloat64 // ZDATECREATED, ZMODIFICATIONDATE, ZADJUSTMENTTIMESTAMP
	Added, Trashed                             sql.NullFloat64 // ZADDEDDATE, ZTRASHEDDATE
	TrashedState, Hidden, Favorite             sql.NullInt64   // ZTRASHEDSTATE, ZHIDDEN, ZFAVORITE
	Latitude, Longitude                        sql.NullFloat64 // ZLATITUDE, ZLONGITUDE
	Adjustments                                sql.NullInt64   // schema.Schema.AdjustmentsColumn
	CloudBatchPublishDate                      sql.NullFloat64 // ZCLOUDBATCHPUBLISHDATE
	BurstUUID                                  sql.NullString  // ZAVALANCHEUUID
	BurstPickType                              sql.NullInt64   // ZAVALANCHEPICKTYPE
	CloudAssetGUID                             sql.NullString  // ZCLOUDASSETGUID
	Width, Height, Orientation                 sql.NullInt64   // ZWIDTH, ZHEIGHT, ZORIENTATION
	SavedAssetType                             sql.NullInt64   // ZSAVEDASSETTYPE
	ScopeParticipation, ScopeShareState, Scope sql.NullInt64   // ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE, ZLIBRARYSCOPESHARESTATE, ZLIBRARYSCOPE

	HasAttributes                         bool           // the ZADDITIONALASSETATTRIBUTES row exists
	Title, OriginalFilename               sql.NullString // ZTITLE, ZORIGINALFILENAME
	TZOffset                              sql.NullInt64  // ZTIMEZONEOFFSET
	Fingerprint                           sql.NullString // schema.Schema.FingerprintColumn
	OriginalWidth, OriginalHeight         sql.NullInt64  // ZORIGINALWIDTH, ZORIGINALHEIGHT
	OriginalOrientation, OriginalFilesize sql.NullInt64  // ZORIGINALORIENTATION, ZORIGINALFILESIZE
	OriginalResourceChoice                sql.NullInt64  // ZORIGINALRESOURCECHOICE
	SyndicationIdentifier                 sql.NullString // ZSYNDICATIONIDENTIFIER
	SyndicationHistory                    sql.NullInt64  // ZSYNDICATIONHISTORY
	Description                           sql.NullString // ZASSETDESCRIPTION.ZLONGDESCRIPTION

	HasMaster       bool           // the ZCLOUDMASTER row of ZASSET.ZMASTER exists
	CloudLocalState sql.NullInt64  // ZCLOUDMASTER.ZCLOUDLOCALSTATE
	CloudMasterGUID sql.NullString // ZCLOUDMASTER.ZCLOUDMASTERGUID
}

// assetSelect is the select list of Assets, in the order of the scan.
func assetSelect(s schema.Schema) []column {
	a := func(name string) column { return column{"a", "ZASSET", name} }
	x := func(name string) column { return column{"x", "ZADDITIONALASSETATTRIBUTES", name} }
	return []column{
		a("Z_PK"), a("ZUUID"), a("ZFILENAME"), a("ZDIRECTORY"), a("ZUNIFORMTYPEIDENTIFIER"),
		a("ZKIND"), a("ZKINDSUBTYPE"),
		a("ZDATECREATED"), a("ZMODIFICATIONDATE"), a("ZADJUSTMENTTIMESTAMP"), a("ZADDEDDATE"), a("ZTRASHEDDATE"),
		a("ZTRASHEDSTATE"), a("ZHIDDEN"), a("ZFAVORITE"), a("ZLATITUDE"), a("ZLONGITUDE"),
		a(s.AdjustmentsColumn), a("ZCLOUDBATCHPUBLISHDATE"), a("ZAVALANCHEUUID"), a("ZAVALANCHEPICKTYPE"),
		a("ZCLOUDASSETGUID"), a("ZWIDTH"), a("ZHEIGHT"), a("ZORIENTATION"), a("ZSAVEDASSETTYPE"),
		a("ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE"), a("ZLIBRARYSCOPESHARESTATE"), a("ZLIBRARYSCOPE"),
		x("Z_PK"), x("ZTITLE"), x("ZORIGINALFILENAME"), x("ZTIMEZONEOFFSET"), x(s.FingerprintColumn),
		x("ZORIGINALWIDTH"), x("ZORIGINALHEIGHT"), x("ZORIGINALORIENTATION"), x("ZORIGINALFILESIZE"),
		x("ZORIGINALRESOURCECHOICE"), x("ZSYNDICATIONIDENTIFIER"), x("ZSYNDICATIONHISTORY"),
		{"d", "ZASSETDESCRIPTION", "ZLONGDESCRIPTION"},
		{"m", "ZCLOUDMASTER", "Z_PK"}, {"m", "ZCLOUDMASTER", "ZCLOUDLOCALSTATE"}, {"m", "ZCLOUDMASTER", "ZCLOUDMASTERGUID"},
	}
}

// assetColumns are the columns of Assets: the select list and the join
// columns.
func assetColumns(s schema.Schema) []column {
	return append(assetSelect(s),
		column{"a", "ZASSET", "ZMASTER"},
		column{"x", "ZADDITIONALASSETATTRIBUTES", "ZASSET"},
		column{"x", "ZADDITIONALASSETATTRIBUTES", "ZASSETDESCRIPTION"},
		column{"d", "ZASSETDESCRIPTION", "Z_PK"},
	)
}

// Assets returns every ZASSET row, ordered by ZUUID, then Z_PK.
//
// Upstream reads these columns in seven queries, the first one with an inner
// join of ZADDITIONALASSETATTRIBUTES. The joined rows are one to one, so one
// query with LEFT JOIN reads the same values and also returns an asset
// without its attributes row (HasAttributes false), which upstream drops.
func (d *DB) Assets(ctx context.Context, s schema.Schema) ([]Asset, error) {
	q := "SELECT " + selectList(assetSelect(s)) + ` FROM ZASSET a
LEFT JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK
LEFT JOIN ZASSETDESCRIPTION d ON d.Z_PK = x.ZASSETDESCRIPTION
LEFT JOIN ZCLOUDMASTER m ON m.Z_PK = a.ZMASTER
ORDER BY a.ZUUID, a.Z_PK`
	var out []Asset
	err := d.readRows(ctx, "assets", q, func(rows *sql.Rows) error {
		var (
			r              Asset
			attrPK, mastPK sql.NullInt64
		)
		if err := rows.Scan(&r.PK, &r.UUID, &r.Filename, &r.Directory, &r.UTI,
			&r.Kind, &r.KindSubtype,
			&r.Created, &r.Modified, &r.Adjusted, &r.Added, &r.Trashed,
			&r.TrashedState, &r.Hidden, &r.Favorite, &r.Latitude, &r.Longitude,
			&r.Adjustments, &r.CloudBatchPublishDate, &r.BurstUUID, &r.BurstPickType,
			&r.CloudAssetGUID, &r.Width, &r.Height, &r.Orientation, &r.SavedAssetType,
			&r.ScopeParticipation, &r.ScopeShareState, &r.Scope,
			&attrPK, &r.Title, &r.OriginalFilename, &r.TZOffset, &r.Fingerprint,
			&r.OriginalWidth, &r.OriginalHeight, &r.OriginalOrientation, &r.OriginalFilesize,
			&r.OriginalResourceChoice, &r.SyndicationIdentifier, &r.SyndicationHistory,
			&r.Description,
			&mastPK, &r.CloudLocalState, &r.CloudMasterGUID); err != nil {
			return err
		}
		r.HasAttributes, r.HasMaster = attrPK.Valid, mastPK.Valid
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
