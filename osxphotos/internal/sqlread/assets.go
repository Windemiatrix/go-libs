package sqlread

import (
	"context"
	"database/sql"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Asset is one ZASSET row with its ZADDITIONALASSETATTRIBUTES,
// ZASSETDESCRIPTION, ZUNMANAGEDADJUSTMENT and ZCLOUDMASTER rows and the
// ZGENERICALBUM row of its import session (LEFT JOIN): the columns of
// photosdb.py:2023-2090, 2377-2412, 2423-2439, 2521-2529,
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
	HDRType, DepthType, VisibilityState        sql.NullInt64   // ZHDRTYPE, ZDEPTHTYPE, ZVISIBILITYSTATE
	CloudOwnerHashedID                         sql.NullString  // ZCLOUDOWNERHASHEDPERSONID
	Owner                                      sql.NullString  // the full name of the person of CloudOwnerHashedID, as Album.Owner (photoinfo.py:1441-1459); NULL when not found
	OwnerComponents                            [][]byte        // as Album.OwnerComponents, for CloudOwnerHashedID
	MomentShare                                sql.NullInt64   // ZMOMENTSHARE
	SpatialType                                sql.NullInt64   // ZSPATIALTYPE; NULL when ZASSET has no such column

	HasAttributes                         bool           // the ZADDITIONALASSETATTRIBUTES row exists
	CameraCaptureDevice                   sql.NullInt64  // ZCAMERACAPTUREDEVICE
	TZName                                sql.NullString // ZTIMEZONENAME
	ImportedByName, ImportedByBundleID    sql.NullString // ZIMPORTEDBYDISPLAYNAME, ZIMPORTEDBYBUNDLEIDENTIFIER
	Title, OriginalFilename               sql.NullString // ZTITLE, ZORIGINALFILENAME
	TZOffset                              sql.NullInt64  // ZTIMEZONEOFFSET
	Fingerprint                           sql.NullString // schema.Schema.FingerprintColumn
	OriginalWidth, OriginalHeight         sql.NullInt64  // ZORIGINALWIDTH, ZORIGINALHEIGHT
	OriginalOrientation, OriginalFilesize sql.NullInt64  // ZORIGINALORIENTATION, ZORIGINALFILESIZE
	OriginalResourceChoice                sql.NullInt64  // ZORIGINALRESOURCECHOICE
	SyndicationIdentifier                 sql.NullString // ZSYNDICATIONIDENTIFIER
	SyndicationHistory                    sql.NullInt64  // ZSYNDICATIONHISTORY
	Description                           sql.NullString // ZASSETDESCRIPTION.ZLONGDESCRIPTION
	AdjustmentFormat                      sql.NullString // ZUNMANAGEDADJUSTMENT.ZADJUSTMENTFORMATIDENTIFIER

	HasMaster       bool           // the ZCLOUDMASTER row of ZASSET.ZMASTER exists
	CloudLocalState sql.NullInt64  // ZCLOUDMASTER.ZCLOUDLOCALSTATE
	CloudMasterGUID sql.NullString // ZCLOUDMASTER.ZCLOUDMASTERGUID

	ImportUUID sql.NullString // ZGENERICALBUM.ZUUID of ZASSET.ZIMPORTSESSION, a row of any ZKIND
}

// The order is that of the Scan in Assets.
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
		a("ZHDRTYPE"), a("ZDEPTHTYPE"), a("ZVISIBILITYSTATE"), a("ZCLOUDOWNERHASHEDPERSONID"), a("ZMOMENTSHARE"),
		x("ZCAMERACAPTUREDEVICE"), x("ZTIMEZONENAME"), x("ZIMPORTEDBYDISPLAYNAME"), x("ZIMPORTEDBYBUNDLEIDENTIFIER"),
		{"u", "ZUNMANAGEDADJUSTMENT", "ZADJUSTMENTFORMATIDENTIFIER"},
		{"m", "ZCLOUDMASTER", "Z_PK"}, {"m", "ZCLOUDMASTER", "ZCLOUDLOCALSTATE"}, {"m", "ZCLOUDMASTER", "ZCLOUDMASTERGUID"},
		{"i", "ZGENERICALBUM", "ZUUID"},
	}
}

func assetColumns(s schema.Schema) []column {
	return append(assetSelect(s),
		column{"a", "ZASSET", "ZMASTER"},
		column{"x", "ZADDITIONALASSETATTRIBUTES", "ZASSET"},
		column{"x", "ZADDITIONALASSETATTRIBUTES", "ZASSETDESCRIPTION"},
		column{"d", "ZASSETDESCRIPTION", "Z_PK"},
		column{"x", "ZADDITIONALASSETATTRIBUTES", "ZUNMANAGEDADJUSTMENT"},
		column{"u", "ZUNMANAGEDADJUSTMENT", "Z_PK"},
		column{"a", "ZASSET", "ZIMPORTSESSION"},
		column{"i", "ZGENERICALBUM", "Z_PK"},
	)
}

// Assets returns every ZASSET row, ordered by ZUUID, then Z_PK.
//
// Upstream reads these columns in nine queries, the first one with an inner
// join of ZADDITIONALASSETATTRIBUTES. The joined rows are one to one, so one
// query with LEFT JOIN reads the same values and also returns an asset
// without its attributes row (HasAttributes false), which upstream drops.
func (d *DB) Assets(ctx context.Context, s schema.Schema) ([]Asset, error) {
	// ZSPATIALTYPE is not in Columns: upstream reads it only where it exists
	// (photosdb.py:1751-1759), not by the schema version.
	assetCols, err := d.columns(ctx, "ZASSET")
	if err != nil {
		return nil, err
	}
	spatial := "NULL"
	if slices.Contains(assetCols, "ZSPATIALTYPE") {
		spatial = "a.ZSPATIALTYPE"
	}
	people, err := d.personIndex(ctx)
	if err != nil {
		return nil, err
	}
	q := "SELECT " + selectList(assetSelect(s)) + ", " + spatial + ` FROM ZASSET a
LEFT JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK
LEFT JOIN ZASSETDESCRIPTION d ON d.Z_PK = x.ZASSETDESCRIPTION
LEFT JOIN ZUNMANAGEDADJUSTMENT u ON u.Z_PK = x.ZUNMANAGEDADJUSTMENT
LEFT JOIN ZCLOUDMASTER m ON m.Z_PK = a.ZMASTER
LEFT JOIN ZGENERICALBUM i ON i.Z_PK = a.ZIMPORTSESSION
ORDER BY a.ZUUID, a.Z_PK`
	var out []Asset
	err = d.readRows(ctx, "assets", q, func(rows *sql.Rows) error {
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
			&r.HDRType, &r.DepthType, &r.VisibilityState, &r.CloudOwnerHashedID, &r.MomentShare,
			&r.CameraCaptureDevice, &r.TZName, &r.ImportedByName, &r.ImportedByBundleID,
			&r.AdjustmentFormat,
			&mastPK, &r.CloudLocalState, &r.CloudMasterGUID, &r.ImportUUID, &r.SpatialType); err != nil {
			return err
		}
		r.HasAttributes, r.HasMaster = attrPK.Valid, mastPK.Valid
		r.Owner, r.OwnerComponents = people.name(r.CloudOwnerHashedID.String)
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
