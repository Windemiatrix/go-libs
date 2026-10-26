// Package photoinfo describes one photo or video of a Photos library as a
// plain value. It does no I/O.
package photoinfo

import "time"

// PhotoInfo is one asset of the library. Fields marked "extension" have no
// counterpart in osxphotos.
type PhotoInfo struct {
	UUID            string
	CloudAssetGUID  string // extension: not in osxphotos — ZASSET.ZCLOUDASSETGUID; "" when absent
	CloudMasterGUID string // osxphotos cloud_guid

	// osxphotos fingerprint (photoinfo.py:1772-1775); "" when absent. Read from
	// ZMASTERFINGERPRINT before Photos 10 and from ZORIGINALSTABLEHASH from
	// Photos 10 (_constants.py:176, 193, 210): values of the two columns do not
	// compare, so compare fingerprints only between libraries on the same side
	// of Info().PhotosVersion 10.
	Fingerprint string

	Filename         string
	OriginalFilename string // Filename when the database has none, as osxphotos

	Date            time.Time  // the moment, in the fixed zone of TZOffset (UTC when TZOffset is nil); 1970-01-01T00:00:00Z when the database has no date, as osxphotos
	TZOffset        *int       // seconds east of UTC; nil when unknown (osxphotos then uses UTC)
	TZName          string     // the time zone name of Date, such as "Europe/Paris"; "" when unknown; Date does not use it
	DateAdded       *time.Time // nil when the database has no value
	DateModified    *time.Time // ZASSET.ZADJUSTMENTTIMESTAMP, as osxphotos; nil when the database has no value
	DateTrashed     *time.Time // nil when the database has no value
	DateModifiedRaw *time.Time // extension: not in osxphotos — ZASSET.ZMODIFICATIONDATE

	Width, Height, Orientation                         int
	OriginalWidth, OriginalHeight, OriginalOrientation int
	OriginalFilesize                                   int64

	UTI, UTIOriginal, UTIRaw string
	UTIEdited                string // UTI when HasAdjustments, otherwise "" (photoinfo.py:929-930)
	Path, PathLive, PathRaw  string // inside the library; "" when the file is not on disk

	PathEdited, PathEditedLive               string   // the edited version and its Live video inside the library; "" when there is no file (photoinfo.py:323-382, 495-512)
	AdjustmentsPath, OriginalAdjustmentsPath string   // the adjustments plist and the original .aae inside the library; "" when there is no file (photoinfo.py:743-781)
	PathDerivatives                          []string // previews inside the library by size, the largest first and equal sizes by name, then the thumbnail of masters/ whatever its size; the first two are swapped when the first is a .mov of a photo; nil when there are none (photoinfo.py:1172-1274)

	IsPhoto, IsMovie, Live, HasRAW, IsRAW, RawOriginal, Screenshot bool

	SlowMo, TimeLapse, ScreenRecording bool // ZKINDSUBTYPE 101, 102, 103 (photosdb.py:2237-2240)

	HDR      bool // ZHDRTYPE 3 (photosdb.py:2248-2249)
	Panorama bool // ZKINDSUBTYPE 1 or ZHDRTYPE 6 (photosdb.py:2257-2258)
	Portrait bool // ZDEPTHTYPE not 0; NULL is a portrait too, as in osxphotos (photosdb.py:2250-2251)
	Selfie   bool // taken with the front camera (photosdb.py:2260-2261)
	Visible  bool // ZVISIBILITYSTATE 0; NULL is not visible (photosdb.py:2304-2307)
	Spatial  int  // ZSPATIALTYPE: 1 a spatial capture, 2 a photo converted to spatial; 0 when NULL or the library has no such column (photoinfo.py:1311-1320)

	SharedMoment       bool   // part of a shared moment (photoinfo.py:1601-1604)
	ExternalEdit       bool   // edited in another app (photoinfo.py:807-810)
	AdjustmentType     *int   // osxphotos adjustment_type: the value HasAdjustments is read from; nil when NULL (photoinfo.py:802-805)
	CloudOwnerHashedID string // the hashed iCloud ID of the owner of a shared photo; "" when absent (photoinfo.py:1767-1770)
	ImportedByName     string // osxphotos imported_by[0], such as "Camera"; "" when unknown (photoinfo.py:682-694)
	ImportedByBundleID string // osxphotos imported_by[1], such as "com.apple.camera"; "" when unknown

	Burst, BurstSelected, BurstKey bool
	BurstUUID                      string
	BurstPickType                  int  // ZASSET.ZAVALANCHEPICKTYPE; NULL is 0
	BurstDefaultPick               bool // BurstPickType has BurstPickDefault (photoinfo.py:1021-1029)

	Hidden, Favorite, InTrash, HasAdjustments bool

	Shared, Syndicated, SavedToLibrary, SharedLibrary bool
	LibraryScopeShareState, LibraryScope              int // extension: not in osxphotos

	InCloud                              *bool // nil when the asset has no cloud master
	IsMissing, IsCloudAsset, IsReference bool

	Title, Description  string
	Keywords            []string // ascending
	Persons             []string // the person of each face, ascending, repeats kept; UnknownPerson for a person without a name (photoinfo.py:597-603)
	Latitude, Longitude *float64

	AlbumUUIDs   []string   // albums of the asset, trashed albums included; ascending
	ProjectUUIDs []string   // projects (ZKIND 1508) of the asset, trashed ones included; ascending (photoinfo.py:696-703, 2022-2034)
	ImportUUID   string     // ZUUID of the ZGENERICALBUM row of ZASSET.ZIMPORTSESSION, of any ZKIND (DB.Import finds only ZKIND 1506); "" without one (photoinfo.py:673-680)
	Resources    []Resource // extension: not in osxphotos — in Z_PK order
}

// UnknownPerson is the name of a person without a name in Persons
// (_constants.py:337); a person named "_UNKNOWN_" gives the same value, as in
// osxphotos.
const UnknownPerson = "_UNKNOWN_"

// Resource is one ZINTERNALRESOURCE row of an asset.
// extension: not in osxphotos.
type Resource struct {
	Type               int   // ZRESOURCETYPE
	Subtype            int   // ZDATASTORESUBTYPE
	Size               int64 // ZDATALENGTH
	LocalAvailability  int   // ZLOCALAVAILABILITY
	RemoteAvailability int   // ZREMOTEAVAILABILITY
}

// Bits of BurstPickType, ZASSET.ZAVALANCHEPICKTYPE (_constants.py:676-681).
const (
	BurstPickDefault  = 4  // BURST_DEFAULT_PICK: Photos picked the frame as the key before any selection
	BurstPickSelected = 8  // BURST_SELECTED: the frame is selected
	BurstPickKey      = 16 // BURST_KEY: the key frame of the burst
)
