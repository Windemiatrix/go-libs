package sqlread

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Share is one ZSHARE row (shareinfo.py:16-61), its fields in the order of
// shareColumns.
type Share struct {
	PK int64 // Z_PK

	CloudDeleteState, LocalPublishState, PublicPermission, ScopeType, Status, TrashedState sql.NullInt64
	AutoSharePolicy, CloudItemCount, CloudLocalState, CloudPhotoCount, CloudVideoCount     sql.NullInt64
	ExitState, ParticipantCloudUpdateState, PreviewState, ScopeSyncingState, AssetCount    sql.NullInt64
	ForceSyncAttempted, PhotosCount, ShouldIgnoreBudgets, ShouldNotifyOnUploadCompletion   sql.NullInt64
	UploadedPhotosCount, UploadedVideosCount, VideosCount                                  sql.NullInt64

	CreationDate, ExpiryDate, TrashedDate           sql.NullFloat64
	LastParticipantAssetTrashNotificationDate       sql.NullFloat64
	LastParticipantAssetTrashNotificationViewedDate sql.NullFloat64
	EndDate, StartDate                              sql.NullFloat64

	ScopeIdentifier, Title, UUID, OriginatingScopeIdentifier, ShareURL sql.NullString

	RulesData, PreviewData, ThumbnailImageData []byte

	ExitSource, CountOfAssetsAddedByCameraSmartSharing, ExitType sql.NullInt64
}

// _SHARE_INFO_COLUMNS of shareinfo.py:82-126.
var shareColumns = []string{
	"Z_PK", "ZCLOUDDELETESTATE", "ZLOCALPUBLISHSTATE", "ZPUBLICPERMISSION", "ZSCOPETYPE", "ZSTATUS", "ZTRASHEDSTATE",
	"ZAUTOSHAREPOLICY", "ZCLOUDITEMCOUNT", "ZCLOUDLOCALSTATE", "ZCLOUDPHOTOCOUNT", "ZCLOUDVIDEOCOUNT",
	"ZEXITSTATE", "ZPARTICIPANTCLOUDUPDATESTATE", "ZPREVIEWSTATE", "ZSCOPESYNCINGSTATE", "ZASSETCOUNT",
	"ZFORCESYNCATTEMPTED", "ZPHOTOSCOUNT", "ZSHOULDIGNOREBUDGETS", "ZSHOULDNOTIFYONUPLOADCOMPLETION",
	"ZUPLOADEDPHOTOSCOUNT", "ZUPLOADEDVIDEOSCOUNT", "ZVIDEOSCOUNT", "ZCREATIONDATE", "ZEXPIRYDATE", "ZTRASHEDDATE",
	"ZLASTPARTICIPANTASSETTRASHNOTIFICATIONDATE", "ZLASTPARTICIPANTASSETTRASHNOTIFICATIONVIEWEDDATE", "ZENDDATE", "ZSTARTDATE",
	"ZSCOPEIDENTIFIER", "ZTITLE", "ZUUID", "ZORIGINATINGSCOPEIDENTIFIER", "ZSHAREURL",
	"ZRULESDATA", "ZPREVIEWDATA", "ZTHUMBNAILIMAGEDATA", "ZEXITSOURCE", "ZCOUNTOFASSETSADDEDBYCAMERASMARTSHARING", "ZEXITTYPE",
}

// Shares returns every ZSHARE row, ordered by Z_PK; nil without a ZSHARE
// table. A column that the table does not have, such as ZUPLOADEDPHOTOSCOUNT,
// removed in macOS 26.1, is NULL, as upstream reads it (shareinfo.py:129-139).
func (d *DB) Shares(ctx context.Context) ([]Share, error) {
	have, err := d.columns(ctx, "ZSHARE")
	if err != nil || len(have) == 0 {
		return nil, err
	}
	list := make([]string, len(shareColumns))
	for i, c := range shareColumns {
		list[i] = "NULL"
		if slices.Contains(have, c) {
			list[i] = c
		}
	}
	var out []Share
	err = d.readRows(ctx, "shares", "SELECT "+strings.Join(list, ", ")+" FROM ZSHARE ORDER BY Z_PK", func(rows *sql.Rows) error {
		var s Share
		if err := rows.Scan(&s.PK,
			&s.CloudDeleteState, &s.LocalPublishState, &s.PublicPermission, &s.ScopeType, &s.Status, &s.TrashedState,
			&s.AutoSharePolicy, &s.CloudItemCount, &s.CloudLocalState, &s.CloudPhotoCount, &s.CloudVideoCount,
			&s.ExitState, &s.ParticipantCloudUpdateState, &s.PreviewState, &s.ScopeSyncingState, &s.AssetCount,
			&s.ForceSyncAttempted, &s.PhotosCount, &s.ShouldIgnoreBudgets, &s.ShouldNotifyOnUploadCompletion,
			&s.UploadedPhotosCount, &s.UploadedVideosCount, &s.VideosCount, &s.CreationDate, &s.ExpiryDate, &s.TrashedDate,
			&s.LastParticipantAssetTrashNotificationDate, &s.LastParticipantAssetTrashNotificationViewedDate, &s.EndDate, &s.StartDate,
			&s.ScopeIdentifier, &s.Title, &s.UUID, &s.OriginatingScopeIdentifier, &s.ShareURL,
			&s.RulesData, &s.PreviewData, &s.ThumbnailImageData, &s.ExitSource, &s.CountOfAssetsAddedByCameraSmartSharing, &s.ExitType); err != nil {
			return err
		}
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Participant is one ZSHAREPARTICIPANT row of get_share_participants
// (shareparticipant.py:21-49) with the assets that its query links it to.
type Participant struct {
	PK int64 // Z_PK

	AcceptanceStatus, IsCurrentUser, ExitState, Permission, Person, Z54Share, Share sql.NullInt64

	EmailAddress, ParticipantID, PhoneNumber, UserIdentifier, UUID sql.NullString

	NameComponents []byte // nil also for an archive over 1 MiB

	AssetUUIDs []string // ZASSET.ZUUID of each ZASSETCONTRIBUTOR row, in its Z_PK order; a repeated link is repeated
}

// Participants returns the ZSHAREPARTICIPANT rows that ZASSETCONTRIBUTOR
// names, ordered by Z_PK, with the assets of the joins of
// get_share_participants; nil when a table or column of that query is
// missing, where upstream catches the error of the query
// (shareparticipant.py:45-49).
func (d *DB) Participants(ctx context.Context, s schema.Schema) ([]Participant, error) {
	cols := []string{"Z_PK", "ZACCEPTANCESTATUS", "ZISCURRENTUSER", "ZEXITSTATE", "ZPERMISSION", "ZPERSON", s.ParticipantShareColumn,
		"ZSHARE", "ZEMAILADDRESS", "ZPARTICIPANTID", "ZPHONENUMBER", "ZUSERIDENTIFIER", "ZUUID", "ZNAMECOMPONENTS"}
	err := d.CheckColumns(ctx, map[string][]string{"ZSHAREPARTICIPANT": cols, "ZASSETCONTRIBUTOR": {"ZPARTICIPANT", "Z3LIBRARYSCOPEASSETCONTRIBUTORS"}})
	if errors.Is(err, ErrMissingColumn) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// A participant is read once, not once per linked photo as in the join of
	// upstream: an archive of a crafted library linked to many photos would
	// multiply. The bound in bytes is maxPlistSize of photosdb, which refuses a
	// larger archive anyway; length() of a text counts characters.
	q := "SELECT " + strings.Join(cols[:len(cols)-1], ", ") +
		", CASE WHEN length(CAST(ZNAMECOMPONENTS AS BLOB)) <= 1048576 THEN ZNAMECOMPONENTS END" +
		" FROM ZSHAREPARTICIPANT WHERE Z_PK IN (SELECT ZPARTICIPANT FROM ZASSETCONTRIBUTOR) ORDER BY Z_PK"
	var out []Participant
	index := map[int64]int{}
	err = d.readRows(ctx, "share participants", q, func(rows *sql.Rows) error {
		var p Participant
		if err := rows.Scan(&p.PK, &p.AcceptanceStatus, &p.IsCurrentUser, &p.ExitState, &p.Permission, &p.Person, &p.Z54Share, &p.Share,
			&p.EmailAddress, &p.ParticipantID, &p.PhoneNumber, &p.UserIdentifier, &p.UUID, &p.NameComponents); err != nil {
			return err
		}
		index[p.PK] = len(out)
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	q = "SELECT p.Z_PK, a.ZUUID FROM ZSHAREPARTICIPANT p JOIN ZASSETCONTRIBUTOR c ON c.ZPARTICIPANT = p.Z_PK" +
		" JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = c.Z3LIBRARYSCOPEASSETCONTRIBUTORS" +
		" JOIN ZASSET a ON a.Z_PK = x.ZASSET WHERE a.ZUUID IS NOT NULL ORDER BY c.Z_PK"
	err = d.readRows(ctx, "share participant assets", q, func(rows *sql.Rows) error {
		var pk int64
		var asset string
		if err := rows.Scan(&pk, &asset); err != nil {
			return err
		}
		if i, ok := index[pk]; ok {
			out[i].AssetUUIDs = append(out[i].AssetUUIDs, asset)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
