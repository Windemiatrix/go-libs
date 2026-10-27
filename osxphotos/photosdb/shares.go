package photosdb

import (
	"database/sql"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// SharedMomentInfo returns the shared moment of p, the ZSHARE row of
// ZASSET.ZMOMENTSHARE: osxphotos shared_moment_info (photoinfo.py:1606-1615);
// nil without that row. The value is shared: do not modify it.
func (db *DB) SharedMomentInfo(p *photoinfo.PhotoInfo) *photoinfo.ShareInfo {
	return db.momentShares[p.UUID]
}

// ShareInfo returns the share of p in a shared library, the ZSHARE row of
// ZASSET.ZLIBRARYSCOPE, a key that upstream calls a guess: osxphotos share_info
// (photoinfo.py:1617-1626, shareinfo.py:156-165); nil without that row. The
// value is shared: do not modify it.
func (db *DB) ShareInfo(p *photoinfo.PhotoInfo) *photoinfo.ShareInfo {
	return db.libraryShares[p.UUID]
}

// ShareParticipantInfo returns the participants of the shared library that
// ZASSETCONTRIBUTOR links to p: osxphotos share_participant_info
// (photoinfo.py:1634-1639, shareparticipant.py:21-49), in
// ZSHAREPARTICIPANT.Z_PK order, a participant once per ZASSETCONTRIBUTOR row
// that links it to p. It is nil without a participant and also when the name
// archive of one of them does not decode, as upstream returns [] on that
// error. The slice is shared: do not modify it.
func (db *DB) ShareParticipantInfo(p *photoinfo.PhotoInfo) []photoinfo.ShareParticipant {
	return db.participants[p.UUID]
}

// ShareParticipants returns the given and the family name of each
// participant of ShareParticipantInfo, joined by a space even when one of
// them is "": osxphotos share_participants (photoinfo.py:1641-1649).
func (db *DB) ShareParticipants(p *photoinfo.PhotoInfo) []string {
	var names []string
	for _, s := range db.participants[p.UUID] {
		names = append(names, s.NameComponents.GivenName+" "+s.NameComponents.FamilyName)
	}
	return names
}

// Upstream builds the list of a photo in one expression inside the try of
// shareparticipant.py:45-49, so one archive that does not decode empties it.
// The strings other than the name are passed as they are, without NFC.
func loadParticipants(rows []sqlread.Participant) map[string][]photoinfo.ShareParticipant {
	out := map[string][]photoinfo.ShareParticipant{}
	failed := map[string]bool{}
	for _, r := range rows {
		name, ok := nameComponents(r.NameComponents)
		p := photoinfo.ShareParticipant{
			PK:               r.PK,
			AcceptanceStatus: optionalInt(r.AcceptanceStatus), IsCurrentUser: optionalInt(r.IsCurrentUser), ExitState: optionalInt(r.ExitState),
			Permission: optionalInt(r.Permission), Person: optionalInt(r.Person), Z54Share: optionalInt(r.Z54Share), Share: optionalInt(r.Share),
			EmailAddress: r.EmailAddress.String, ParticipantID: r.ParticipantID.String, PhoneNumber: r.PhoneNumber.String,
			UserIdentifier: r.UserIdentifier.String, UUID: r.UUID.String,
			NameComponents: name,
		}
		for _, u := range r.AssetUUIDs {
			out[u] = append(out[u], p)
			if !ok {
				failed[u] = true
			}
		}
	}
	for u := range failed {
		delete(out, u)
	}
	return out
}

// Upstream: shareparticipant.py:90-127, 153-187. The bool is false where the
// unarchiving or the attribute access of upstream raises: not a property
// list, no root object, a root without NSnameComponentsPrivate, or a
// reference outside $objects, which bpylist2 indexes.
func nameComponents(archive []byte) (photoinfo.PersonNameComponents, bool) {
	keyed, _ := plistAny(archive).(map[string]any)
	objects, _ := keyed["$objects"].([]any)
	top, _ := keyed["$top"].(map[string]any)
	outside := false
	object := func(ref any) any {
		uid, isUID := ref.(plist.UID)
		if isUID && uint64(uid) >= uint64(len(objects)) {
			outside = true
			return nil
		}
		if !isUID || objects[uid] == "$null" {
			return nil
		}
		return objects[uid]
	}
	person, _ := object(top["root"]).(map[string]any)
	data, ok := object(person["NSnameComponentsPrivate"]).(map[string]any)
	part := func(key string) string {
		s, _ := object(data[key]).(string)
		return nfc(s)
	}
	n := photoinfo.PersonNameComponents{
		GivenName: part("NSgivenName"), MiddleName: part("NSmiddleName"), FamilyName: part("NSfamilyName"),
		NamePrefix: part("NSnamePrefix"), NameSuffix: part("NSnameSuffix"), Nickname: part("NSnickname"),
		PhoneticRepresentation: part("NSphoneticRepresentation"),
	}
	if !ok || outside {
		return photoinfo.PersonNameComponents{}, false
	}
	return n, true
}

// Upstream joins ZSHARE to the asset of the ZUUID (shareinfo.py:140-146); only
// the asset with attributes is the photo of that ZUUID (load), so another asset
// with the same ZUUID adds no share.
func loadShares(rows []sqlread.Share, assets []sqlread.Asset) (moment, library map[string]*photoinfo.ShareInfo) {
	byPK := make(map[int64]*photoinfo.ShareInfo, len(rows))
	for _, r := range rows {
		byPK[r.PK] = shareInfo(r)
	}
	moment, library = map[string]*photoinfo.ShareInfo{}, map[string]*photoinfo.ShareInfo{}
	for _, a := range assets {
		if s := byPK[a.MomentShare.Int64]; a.HasAttributes && a.MomentShare.Valid && s != nil {
			moment[a.UUID.String] = s
		}
		if s := byPK[a.Scope.Int64]; a.HasAttributes && a.Scope.Valid && s != nil {
			library[a.UUID.String] = s
		}
	}
	return moment, library
}

// Upstream passes the strings of the row as they are, without the NFC of
// other text (shareinfo.py:146-147).
func shareInfo(r sqlread.Share) *photoinfo.ShareInfo {
	return &photoinfo.ShareInfo{
		PK: r.PK,

		CloudDeleteState: optionalInt(r.CloudDeleteState), LocalPublishState: optionalInt(r.LocalPublishState), PublicPermission: optionalInt(r.PublicPermission),
		ScopeType: optionalInt(r.ScopeType), Status: optionalInt(r.Status), TrashedState: optionalInt(r.TrashedState),
		AutoSharePolicy: optionalInt(r.AutoSharePolicy), CloudItemCount: optionalInt(r.CloudItemCount), CloudLocalState: optionalInt(r.CloudLocalState),
		CloudPhotoCount: optionalInt(r.CloudPhotoCount), CloudVideoCount: optionalInt(r.CloudVideoCount), ExitState: optionalInt(r.ExitState),
		PreviewState: optionalInt(r.PreviewState), ScopeSyncingState: optionalInt(r.ScopeSyncingState), ParticipantCloudUpdateState: optionalInt(r.ParticipantCloudUpdateState),
		AssetCount: optionalInt(r.AssetCount), ForceSyncAttempted: optionalInt(r.ForceSyncAttempted), PhotosCount: optionalInt(r.PhotosCount),
		VideosCount: optionalInt(r.VideosCount), ShouldIgnoreBudgets: optionalInt(r.ShouldIgnoreBudgets), ShouldNotifyOnUploadCompletion: optionalInt(r.ShouldNotifyOnUploadCompletion),
		ExitSource: optionalInt(r.ExitSource), ExitType: optionalInt(r.ExitType), UploadedPhotosCount: optionalInt(r.UploadedPhotosCount),
		UploadedVideosCount: optionalInt(r.UploadedVideosCount), CountOfAssetsAddedByCameraSmartSharing: optionalInt(r.CountOfAssetsAddedByCameraSmartSharing),

		CreationDate: localTime(r.CreationDate), ExpiryDate: localTime(r.ExpiryDate), TrashedDate: localTime(r.TrashedDate),
		StartDate: localTime(r.StartDate), EndDate: localTime(r.EndDate),

		LastParticipantAssetTrashNotificationDate:       localTime(r.LastParticipantAssetTrashNotificationDate),
		LastParticipantAssetTrashNotificationViewedDate: localTime(r.LastParticipantAssetTrashNotificationViewedDate),

		ScopeIdentifier: r.ScopeIdentifier.String, Title: r.Title.String, UUID: r.UUID.String,
		ShareURL: r.ShareURL.String, OriginatingScopeIdentifier: r.OriginatingScopeIdentifier.String,
		RulesData: r.RulesData, PreviewData: r.PreviewData, ThumbnailImageData: r.ThumbnailImageData,
	}
}

// photos_datetime_local gives the time zone of the computer
// (photos_datetime.py:70-89). Upstream does not convert a date of 0, which
// stays the number 0 (shareinfo.py:74-75), a value no *time.Time holds.
func localTime(v sql.NullFloat64) *time.Time {
	t := optionalTime(v)
	if t == nil || v.Float64 == 0 {
		return nil
	}
	local := t.In(time.Local)
	return &local
}
