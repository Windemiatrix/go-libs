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
	// Import when the library has no album of the request.
	ErrNotFound = errors.New("photokit: no asset found")
	// ErrMediaType is wrapped, with the number PhotoKit gave, when a fetched
	// asset is neither an image, a video nor a Live Photo
	// (PhotoKitMediaTypeError, photokit.py:1309-1319).
	ErrMediaType = errors.New("photokit: unknown media type")
	// ErrInvalidName is returned by Export when the name is not one element of
	// a path: empty, ".", "..", or with "/" or NUL in it.
	ErrInvalidName = errors.New("photokit: the name is not one path element")
	// ErrNoResource is wrapped by the error of Export when the asset has no
	// resource of the type (PhotoKitExportError, photokit.py:583-586).
	ErrNoResource = bridge.ErrNoResource
	// ErrExport is wrapped, with the domain and code of the NSError, when
	// PhotoKit fails an export (PhotoKitExportError, photokit.py:719-723). The
	// description of the NSError is left out: it may name a file.
	ErrExport = errors.New("photokit: export failed")
	// ErrImportFile is wrapped, with the name of the field, when a path of an
	// ImportRequest is not an absolute path of a regular file, when neither or
	// both of Photo and Video are set, or when PairedVideo or AlternatePhoto is
	// set without Photo. Nothing was changed.
	// extension: not in osxphotos
	ErrImportFile = errors.New("photokit: not a file to import")
	// ErrImport is wrapped when PhotoKit fails an import, with the domain and
	// code of the NSError, or reports one without an identifier. The
	// description of the NSError is left out: it may name a file.
	// extension: not in osxphotos
	ErrImport = errors.New("photokit: import failed")
	// ErrUserCanceled is returned when the user refuses the dialog of a change
	// (PHPhotosErrorUserCancelled, 3072); nothing was changed.
	// extension: not in osxphotos
	ErrUserCanceled = errors.New("photokit: the user canceled the change")
	// ErrOutcomeUnknown is wrapped, together with the error of ctx, when ctx
	// ends before PhotoKit reports the outcome of a change: the change may
	// still complete. Read the library again before the next change.
	// extension: not in osxphotos
	ErrOutcomeUnknown = errors.New("photokit: the outcome of the change is unknown")
)
