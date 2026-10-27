package photosdb

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/uti"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const (
	// ZASSET.ZKIND (photosdb.py:2197-2204).
	kindPhoto, kindMovie = 0, 1

	// ZASSET.ZKINDSUBTYPE (photosdb.py:2234-2236, 2258).
	subtypePanorama, subtypeLive, subtypeScreen = 1, 2, 10

	// ZASSET.ZHDRTYPE of an HDR photo and of a panorama (photosdb.py:2249, 2258).
	hdrTypeHDR, hdrTypePanorama = 3, 6

	// ZADDITIONALASSETATTRIBUTES.ZCAMERACAPTUREDEVICE of the front camera (photosdb.py:2261).
	frontCamera = 1

	// ZASSET.ZVISIBILITYSTATE of a visible asset (photosdb.py:2307).
	visibleState = 0

	// ZASSET.ZDEPTHTYPE of a photo without depth (photosdb.py:2251).
	noDepth = 0

	// ZUNMANAGEDADJUSTMENT.ZADJUSTMENTFORMATIDENTIFIER of an edit in another app (photoinfo.py:810).
	externalEditFormat = "com.apple.Photos.externalEdit"

	// ZASSET.ZKINDSUBTYPE of slow-motion, time-lapse and screen-recording videos (photosdb.py:2231-2240).
	subtypeSlowMo, subtypeTimeLapse, subtypeScreenRecording = 101, 102, 103

	// ZASSET.ZSAVEDASSETTYPE of a referenced file (photosdb.py:2315).
	savedReference = 10

	// ZCLOUDMASTER.ZCLOUDLOCALSTATE of a master in iCloud (photosdb.py:2534).
	cloudInCloud = 3

	// ZLATITUDE and ZLONGITUDE of an asset without a location (photosdb.py:2174-2181).
	noCoordinate = -180.0

	// ZINTERNALRESOURCE.ZDATASTORESUBTYPE of the originals (photosdb.py:2459)
	// and of a RAW resource (photosdb.py:2550).
	resOriginal, resSecond, resRAW = 1, 3, 17

	// ZRESOURCETYPE of the RAW resource of a RAW+JPEG pair.
	resTypeRAW = 4

	// Part of the UTI of every RAW format (photoinfo.py:1368-1371).
	rawUTIMarker = "raw-image"

	// ZLOCALAVAILABILITY of a resource on disk (photosdb.py:2492, 2513).
	localAvailable = 1

	// ZDATASTORESUBTYPE of the video of a shared album, UUID.medium.MP4 (_constants.py:373).
	resSharedVideo = 8

	// ZORIGINALRESOURCECHOICE when the RAW is the original (photosdb.py:2288).
	rawOriginalChoice = 1

	// ZTRASHEDSTATE of an asset in the trash, and ZHIDDEN and ZFAVORITE when set.
	trashedState, flagSet = 1, 1

	// ZSYNDICATIONHISTORY of a photo not saved to the library (photoinfo.py:1597).
	syndicationHistoryNotSaved = 0
)

type tables struct {
	assets    []sqlread.Asset
	resources []sqlread.Resource
	keywords  []sqlread.Keyword
	faces     []sqlread.Face
	members   []sqlread.Member
	albums    []sqlread.Album
	shares    []sqlread.Share

	collectionShares []sqlread.Album // CollectionShare rows of ZSHARE: their PK is a ZSHARE.Z_PK
	shareMembers     []sqlread.Member
}

type catalog struct {
	photos  []*photoinfo.PhotoInfo // ZUUID order
	byUUID  map[string]*photoinfo.PhotoInfo
	skipped int // assets without ZADDITIONALASSETATTRIBUTES
}

// osxphotos normalizes the text of the database to NFC (unicode.py:12, 76-92).
func nfc(s string) string { return norm.NFC.String(s) }

// The inner join of upstream drops assets without an attributes row
// (photosdb.py:2086-2090). An asset without ZUUID, or two with one ZUUID, is
// an error although upstream keeps the last row (photosdb.py:2356): an
// ambiguous identity is worse for a caller that synchronizes than a refusal.
//
// The rows come in the order of their readers: the assets by ZUUID, the
// resources of an asset by Z_PK. catalog.photos and PhotoInfo.Resources keep
// that order.
//
// load never indexes by a value of the database, so no content makes it panic.
func load(t tables) (*catalog, error) {
	resources := groupResources(t.resources)
	keywords := groupKeywords(t.keywords)
	albums, projects := groupMembers(t.members)
	persons := groupFaces(t.faces)
	inShare := make(map[int64]bool, len(t.shareMembers))
	for _, m := range t.shareMembers {
		inShare[m.AssetPK] = true
	}

	c := &catalog{byUUID: make(map[string]*photoinfo.PhotoInfo, len(t.assets))}
	var noUUID, dupUUID int
	for _, a := range t.assets {
		if !a.HasAttributes {
			c.skipped++
			continue
		}
		if a.UUID.String == "" {
			noUUID++
			continue
		}
		if _, dup := c.byUUID[a.UUID.String]; dup {
			dupUUID++
			continue
		}
		p := photo(a, resources[a.PK])
		if inShare[a.PK] && p.IsMovie { // photosdb.py:2518-2519, 2918-2942; the highest Z_PK decides, as in isMissing
			for _, r := range resources[a.PK] {
				if is(r.Subtype, resSharedVideo) {
					p.IsMissing = !is(r.Local, localAvailable)
				}
			}
		}
		p.Keywords = keywords[a.PK]
		p.AlbumUUIDs = albums[a.PK]
		p.ProjectUUIDs = projects[a.PK]
		p.Persons = persons[a.PK]
		c.photos = append(c.photos, p)
		c.byUUID[p.UUID] = p
	}
	if noUUID > 0 {
		return nil, fmt.Errorf("photosdb: %d assets without ZASSET.ZUUID", noUUID)
	}
	if dupUUID > 0 {
		return nil, fmt.Errorf("photosdb: %d assets with the ZASSET.ZUUID of another asset", dupUUID)
	}
	return c, nil
}

func photo(a sqlread.Asset, res []sqlread.Resource) *photoinfo.PhotoInfo {
	p := &photoinfo.PhotoInfo{
		UUID:           a.UUID.String,
		CloudAssetGUID: a.CloudAssetGUID.String,
		Fingerprint:    a.Fingerprint.String,

		Filename: nfc(a.Filename.String),

		Date:            assetDate(a.Created, a.TZOffset, a.TZName),
		TZOffset:        optionalInt(a.TZOffset),
		TZName:          a.TZName.String,
		DateAdded:       optionalTime(a.Added),
		DateModified:    optionalTime(a.Adjusted),
		DateTrashed:     optionalTime(a.Trashed),
		DateModifiedRaw: optionalTime(a.Modified),

		Width:               intOf(a.Width),
		Height:              intOf(a.Height),
		Orientation:         intOf(a.Orientation),
		OriginalWidth:       intOf(a.OriginalWidth),
		OriginalHeight:      intOf(a.OriginalHeight),
		OriginalOrientation: intOf(a.OriginalOrientation),
		OriginalFilesize:    a.OriginalFilesize.Int64,

		UTI: a.UTI.String,

		IsPhoto:     a.Kind.Valid && a.Kind.Int64 == kindPhoto,
		IsMovie:     a.Kind.Valid && a.Kind.Int64 == kindMovie,
		Live:        is(a.KindSubtype, subtypeLive),
		Screenshot:  is(a.KindSubtype, subtypeScreen),
		RawOriginal: is(a.OriginalResourceChoice, rawOriginalChoice),

		SlowMo:          is(a.KindSubtype, subtypeSlowMo),
		TimeLapse:       is(a.KindSubtype, subtypeTimeLapse),
		ScreenRecording: is(a.KindSubtype, subtypeScreenRecording),

		HDR:      is(a.HDRType, hdrTypeHDR),
		Panorama: is(a.KindSubtype, subtypePanorama) || is(a.HDRType, hdrTypePanorama),
		Selfie:   is(a.CameraCaptureDevice, frontCamera),
		Visible:  is(a.VisibilityState, visibleState),
		Spatial:  intOf(a.SpatialType),
		// Python None != 0 (photosdb.py:2251).
		Portrait: !is(a.DepthType, noDepth),

		SharedMoment:       a.MomentShare.Valid && a.MomentShare.Int64 != 0,
		ExternalEdit:       a.AdjustmentFormat.String == externalEditFormat,
		AdjustmentType:     optionalInt(a.Adjustments),
		CloudOwnerHashedID: a.CloudOwnerHashedID.String,
		ImportedByName:     a.ImportedByName.String,
		ImportedByBundleID: a.ImportedByBundleID.String,

		Burst:         a.BurstUUID.Valid,
		BurstUUID:     a.BurstUUID.String,
		BurstPickType: intOf(a.BurstPickType),

		Hidden:         is(a.Hidden, flagSet),
		Favorite:       is(a.Favorite, flagSet),
		InTrash:        is(a.TrashedState, trashedState),
		HasAdjustments: a.Adjustments.Valid && a.Adjustments.Int64 != 0,

		Shared:     a.CloudBatchPublishDate.Valid,
		Syndicated: a.SyndicationIdentifier.Valid,
		// Python None != 0: a NULL history counts as saved (photoinfo.py:1597).
		SavedToLibrary:         !is(a.SyndicationHistory, syndicationHistoryNotSaved),
		SharedLibrary:          a.ScopeParticipation.Valid && a.ScopeParticipation.Int64 != 0,
		LibraryScopeShareState: intOf(a.ScopeShareState),

		IsReference: is(a.SavedAssetType, savedReference),

		Title:       nfc(a.Title.String),
		Description: nfc(a.Description.String),

		ImportUUID: a.ImportUUID.String,
	}
	p.OriginalFilename = nfc(a.OriginalFilename.String)
	if p.OriginalFilename == "" {
		p.OriginalFilename = p.Filename // photoinfo.py:140
	}
	p.IsCloudAsset = p.CloudAssetGUID != ""
	if a.HasMaster {
		inCloud := is(a.CloudLocalState, cloudInCloud)
		p.InCloud = &inCloud
		p.CloudMasterGUID = a.CloudMasterGUID.String
	}
	p.BurstSelected = p.BurstPickType&photoinfo.BurstPickSelected != 0
	p.BurstKey = p.BurstPickType&photoinfo.BurstPickKey != 0
	p.BurstDefaultPick = p.BurstPickType&photoinfo.BurstPickDefault != 0
	if !(a.Latitude.Valid && a.Latitude.Float64 == noCoordinate && a.Longitude.Valid && a.Longitude.Float64 == noCoordinate) {
		p.Latitude, p.Longitude = optionalFloat(a.Latitude), optionalFloat(a.Longitude)
	}

	if p.HasAdjustments { // photoinfo.py:929-930
		p.UTIEdited = p.UTI
	}

	p.UTIOriginal = p.UTI
	if !p.Shared { // photoinfo.py:908-917
		if u := uti.ForExtension(suffix(p.OriginalFilename)); u != "" {
			p.UTIOriginal = u
		}
	}
	p.IsRAW = strings.Contains(p.UTIOriginal, rawUTIMarker)

	p.HasRAW = hasRAW(res)
	p.IsMissing = isMissing(res, p.Fingerprint)
	for _, r := range res {
		p.Resources = append(p.Resources, photoinfo.Resource{
			Type:               intOf(r.Type),
			Subtype:            intOf(r.Subtype),
			Size:               r.DataLength.Int64,
			LocalAvailability:  intOf(r.Local),
			RemoteAvailability: intOf(r.Remote),
		})
	}
	return p
}

// Data store subtype 17 of any resource type is a RAW resource, as upstream
// (photosdb.py:2537-2577).
// extension: not in osxphotos — the subtype 3 rule.
func hasRAW(res []sqlread.Resource) bool {
	for _, r := range res {
		if is(r.Subtype, resRAW) || is(r.Subtype, resSecond) && is(r.Type, resTypeRAW) {
			return true
		}
	}
	return false
}

// isMissing follows photosdb.py:2441-2516; the resources are ordered by Z_PK.
// Upstream reads the resources of subtypes 1 and 3, then the resources whose
// ZFINGERPRINT is the fingerprint of the asset, of any asset, and the last row
// read wins. Here the second pass reads only the resources of the asset, and
// the last row is the one with the highest Z_PK.
func isMissing(res []sqlread.Resource, fingerprint string) bool {
	var last *sqlread.Resource
	if fingerprint != "" {
		for i := range res {
			if res[i].Fingerprint.Valid && res[i].Fingerprint.String == fingerprint {
				last = &res[i]
			}
		}
	}
	if last == nil {
		for i := range res {
			if is(res[i].Subtype, resOriginal) || is(res[i].Subtype, resSecond) {
				last = &res[i]
			}
		}
	}
	return last != nil && !is(last.Local, localAvailable)
}

func groupResources(rows []sqlread.Resource) map[int64][]sqlread.Resource {
	out := map[int64][]sqlread.Resource{}
	for _, r := range rows {
		out[r.AssetPK] = append(out[r.AssetPK], r)
	}
	return out
}

// A NULL title is "" and a repeated keyword stays repeated
// (photosdb.py:2005-2010).
func groupKeywords(rows []sqlread.Keyword) map[int64][]string {
	out := map[int64][]string{}
	for _, k := range rows {
		out[k.AssetPK] = append(out[k.AssetPK], nfc(k.Title.String))
	}
	for _, g := range out {
		slices.Sort(g)
	}
	return out
}

// Upstream names a person without a name "_UNKNOWN_" and lists a name once
// per face (photosdb.py:1792-1796, 1858-1864; photoinfo.py:597-603).
func groupFaces(rows []sqlread.Face) map[int64][]string {
	names := map[int64][]string{}
	for _, f := range rows {
		name := nfc(f.FullName.String)
		if name == "" {
			name = photoinfo.UnknownPerson
		}
		names[f.AssetPK] = append(names[f.AssetPK], name)
	}
	for _, g := range names {
		slices.Sort(g)
	}
	return names
}

// Upstream keeps both kinds in one list per photo and splits it by ZKIND
// (photoinfo.py:2022-2034).
func groupMembers(rows []sqlread.Member) (albums, projects map[int64][]string) {
	albums, projects = map[int64][]string{}, map[int64][]string{}
	for _, m := range rows {
		out := albums
		if m.Kind == sqlread.KindProject {
			out = projects
		}
		out[m.AssetPK] = append(out[m.AssetPK], m.AlbumUUID)
	}
	for _, out := range []map[int64][]string{albums, projects} {
		for pk, g := range out {
			out[pk] = sortedSet(g)
		}
	}
	return albums, projects
}

func sortedSet(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	slices.Sort(s)
	return slices.Compact(s)
}

func is(v sql.NullInt64, want int64) bool { return v.Valid && v.Int64 == want }

// int is 64 bits on every platform this module supports (macOS on amd64 and arm64).
func intOf(v sql.NullInt64) int { return int(v.Int64) } //nolint:gosec // G115: int is 64 bits on the supported platforms

func optionalInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := intOf(v)
	return &i
}

func optionalFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}
