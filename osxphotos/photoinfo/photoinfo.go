// Package photoinfo describes one photo or video of a Photos library as a
// plain value. It does no I/O.
package photoinfo

import "time"

// PhotoInfo is one asset of the library. Fields marked "extension" have no
// counterpart in osxphotos.
type PhotoInfo struct {
	UUID              string
	CloudAssetGUID    string // extension: not in osxphotos — ZASSET.ZCLOUDASSETGUID; "" when absent
	CloudMasterGUID   string // osxphotos cloud_guid
	Fingerprint       string
	FingerprintSchema string // extension: not in osxphotos — the column Fingerprint was read from; values of different columns do not compare

	Filename         string
	OriginalFilename string // Filename when the database has none, as osxphotos

	Date            time.Time // the moment; the asset's offset is TZOffset
	TZOffset        *int      // seconds east of UTC; nil when unknown (osxphotos then uses UTC)
	DateAdded       *time.Time
	DateModified    *time.Time // ZASSET.ZADJUSTMENTTIMESTAMP, as osxphotos
	DateTrashed     *time.Time
	DateModifiedRaw *time.Time // extension: not in osxphotos — ZASSET.ZMODIFICATIONDATE

	Width, Height, Orientation                         int
	OriginalWidth, OriginalHeight, OriginalOrientation int
	OriginalFilesize                                   int64

	UTI, UTIOriginal, UTIRaw string
	Path, PathLive, PathRaw  string // inside the library; "" when the file is not on disk

	IsPhoto, IsMovie, Live, HasRAW, IsRAW, RawOriginal, Screenshot bool

	Burst, BurstSelected, BurstKey bool
	BurstUUID                      string
	BurstPickType                  int // ZASSET.ZAVALANCHEPICKTYPE; NULL is 0

	Hidden, Favorite, InTrash, HasAdjustments bool

	Shared, Syndicated, SavedToLibrary, SharedLibrary bool
	LibraryScopeShareState, LibraryScope              int // extension: not in osxphotos

	InCloud                              *bool // nil when the asset has no cloud master
	IsMissing, IsCloudAsset, IsReference bool

	Title, Description  string
	Keywords            []string
	HasPersons          bool // extension: not in osxphotos — osxphotos lists the names of the persons
	Latitude, Longitude *float64

	AlbumUUIDs      []string   // albums of the asset, trashed albums included
	BurstAlbumUUIDs []string   // extension: not in osxphotos — own albums plus those of the burst's key frame
	Resources       []Resource // extension: not in osxphotos
}

// Resource is one ZINTERNALRESOURCE row of an asset.
// extension: not in osxphotos.
type Resource struct {
	Type               int   // ZRESOURCETYPE
	Subtype            int   // ZDATASTORESUBTYPE
	Size               int64 // ZDATALENGTH
	LocalAvailability  int   // ZLOCALAVAILABILITY
	RemoteAvailability int   // ZREMOTEAVAILABILITY
}

// Burst pick type values of ZASSET.ZAVALANCHEPICKTYPE (_constants.py:676-680).
const (
	burstPickNone = 0  // BURST_PICK_TYPE_NONE
	burstSelected = 8  // BURST_SELECTED
	burstKey      = 16 // BURST_KEY
)

// BurstUnselected reports a burst frame that osxphotos photos() leaves out:
// not selected (pick&8), not the key frame (pick&16) and not without a pick
// (0) — photosdb.py:3347-3351.
// extension: not in osxphotos.
func (p *PhotoInfo) BurstUnselected() bool {
	pick := p.BurstPickType
	return p.Burst && pick&burstSelected == 0 && pick&burstKey == 0 && pick != burstPickNone
}
