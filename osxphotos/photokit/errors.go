package photokit

import (
	"errors"
	"fmt"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Status is PHAuthorizationStatus of the process for read-write access to the
// Photos library (photokit.py:120-121, 139-160).
type Status int

// The values of PHAuthorizationStatus.
const (
	StatusNotDetermined Status = iota // the user has not been asked
	StatusRestricted
	StatusDenied
	StatusAuthorized
	StatusLimited // a selection of photos; PhotoKit on macOS reports it, Photos does not offer it
)

var statusNames = [...]string{"not determined", "restricted", "denied", "authorized", "limited"}

// String returns Status(N) for a value without a constant.
func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return fmt.Sprintf("Status(%d)", int(s))
	}
	return statusNames[s]
}

var (
	// ErrNotSupported is returned by every call of a build without the
	// PhotoKit bridge.
	ErrNotSupported = bridge.ErrNotSupported
	// ErrNotAuthorized is wrapped by the error of Open when the process may
	// not access the whole library (PhotoKitAuthError, photokit.py:174-175,
	// 1200-1204).
	ErrNotAuthorized = errors.New("photokit: access to the Photos library is not authorized")
	// ErrNotFound is returned when a fetch or Resources finds no asset
	// (PhotoKitFetchFailed, photokit.py:1244-1246, 1292-1294), and wrapped by
	// Import when an album of the request is missing or cannot take an asset,
	// by the collection methods for a collection, a parent, an asset or an
	// identifier that is missing, not a valid string or cannot take the change,
	// by Delete for an asset that is missing or cannot be deleted, and by Update
	// for an asset that is missing or cannot be changed; by Delete, Update,
	// CloudIdentifiers and LocalIdentifiers also for an identifier that is not a
	// valid string, never for a missing asset, which the two leave out.
	ErrNotFound = errors.New("photokit: not found")
	// ErrMediaType is wrapped, with the number PhotoKit gave, when a fetched
	// asset is neither an image, a video nor a Live Photo
	// (PhotoKitMediaTypeError, photokit.py:1309-1319), by ExportImage for an
	// asset that is not a photo and by ExportVideo for one that is not a video.
	ErrMediaType = errors.New("photokit: unknown media type")
	// ErrInvalidName is returned by Export, ExportImage and ExportVideo when the
	// name is not one element of a path: empty, ".", "..", or with "/" or NUL
	// in it.
	ErrInvalidName = errors.New("photokit: the name is not one path element")
	// ErrInvalidVersion is wrapped, with the number, by the error of
	// ExportImage and ExportVideo for a Version other than the three constants
	// (ValueError, photokit.py:644-649, 980-985).
	ErrInvalidVersion = errors.New("photokit: invalid version")
	// ErrNoResource is wrapped by the error of Export when the asset has no
	// resource of the type (PhotoKitExportError, photokit.py:583-586), and by
	// that of ExportImage when PhotoKit no longer finds the asset or gives no
	// data (photokit.py:591-592), and by that of ExportVideo when PhotoKit no
	// longer finds the asset or gives no file, or one that does not exist or is
	// not a regular file (898-899, 904-905).
	ErrNoResource = bridge.ErrNoResource
	// ErrExport is wrapped, with the domain and code of the NSError, when
	// PhotoKit fails an export or a request of a version (PhotoKitExportError,
	// photokit.py:719-723). The description of the NSError is left out: it may
	// name a file.
	ErrExport = errors.New("photokit: export failed")
	// ErrImportFile is wrapped, with the name of the field, when a path of an
	// ImportRequest is not an absolute path of a regular file, when neither or
	// both of Photo and Video are set, when PairedVideo or AlternatePhoto is set
	// without Photo, or when OriginalFilename holds a NUL or is not UTF-8.
	// Nothing was changed.
	// extension: not in osxphotos
	ErrImportFile = errors.New("photokit: not a file to import")
	// ErrImport is wrapped when PhotoKit fails an import, with the domain and
	// code of the NSError, or reports one without an identifier. The
	// description of the NSError is left out: it may name a file.
	// extension: not in osxphotos
	ErrImport = errors.New("photokit: import failed")
	// ErrUserCanceled is returned when the user refuses the dialog of a change
	// (PHPhotosErrorUserCancelled, 3072); nothing was changed. Delete,
	// DeleteCollections and Update with Hidden true ask the user.
	// extension: not in osxphotos
	ErrUserCanceled = errors.New("photokit: the user canceled the change")
	// ErrOutcomeUnknown is wrapped, together with the error of ctx, when ctx
	// ends before PhotoKit reports the outcome of a change, which may still
	// complete, and with context.DeadlineExceeded when PhotoKit commits a change
	// that raised an exception. Read the library again before the next change.
	// extension: not in osxphotos
	ErrOutcomeUnknown = errors.New("photokit: the outcome of the change is unknown")
	// ErrInvalidTitle is returned when a title is empty, not UTF-8 or holds a NUL.
	// extension: not in osxphotos
	ErrInvalidTitle = errors.New("photokit: the title is empty or not a valid string")
	// ErrChange is wrapped when PhotoKit fails a change of a collection, a
	// deletion of assets or an update of an asset, with the domain and code of
	// the NSError, or creates a collection without an identifier. The
	// description of the NSError is left out: it may name a collection or a file.
	// extension: not in osxphotos
	ErrChange = errors.New("photokit: the change failed")
)
