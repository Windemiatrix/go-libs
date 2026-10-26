package sqlread

import (
	"context"
	"database/sql"
	"slices"
	"strings"
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
