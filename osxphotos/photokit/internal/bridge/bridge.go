// Package bridge is the boundary between photokit and PhotoKit, so that the
// Go code of photokit is tested without Photos.
package bridge

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrNotSupported is returned by every method of the bridge of a build
	// without PhotoKit; photokit exports it.
	ErrNotSupported = errors.New("photokit: not supported on this platform")
	// ErrNoResource is returned by RequestResource when the asset has no
	// resource of the type and by RequestImage, RequestVideo and
	// ExportComposition when there is no such asset, and delivered by Done of
	// RequestImage when PhotoKit gives no data; photokit exports it.
	ErrNoResource = errors.New("photokit: the asset has no resource of that type")
	// ErrCanceled is delivered by Done after Cancel, whatever PhotoKit reports
	// then.
	ErrCanceled = errors.New("photokit: the request was canceled")
	// ErrNoAlbum is returned by Import and the collection methods when an
	// album or a collection is not found or cannot take the change; nothing
	// was changed.
	ErrNoAlbum = errors.New("photokit: no such album")
	// ErrNoAsset is returned by AlbumAssets for an asset not found, by
	// DeleteAssets for one not found or that cannot be deleted, and by
	// UpdateAsset for one not found or that cannot be changed; nothing was
	// changed.
	ErrNoAsset = errors.New("photokit: no such asset")
	// ErrNoComposition is delivered by Done of ExportComposition when the
	// current AVAsset of the asset is neither an AVComposition of two tracks
	// (photokit.py:1005-1009) nor an AVURLAsset of a file URL; photokit reports it
	// as ErrMediaType.
	ErrNoComposition = errors.New("photokit: the asset is not a slow-motion composition")
)

// Bridge is what photokit needs from PhotoKit. A status is a
// PHAuthorizationStatus value for read-write access. No fetch finds a hidden
// asset: upstream fetches with the default PHFetchOptions
// (photokit.py:1328-1331, 1375-1379). Every identifier and title given to a
// method is UTF-8 without NUL, which photokit checks: the bridge checks only
// the state of PhotoKit.
type Bridge interface {
	AuthorizationStatus() (int, error)
	// RequestAuthorization blocks up to timeout, then returns
	// context.DeadlineExceeded.
	RequestAuthorization(timeout time.Duration) (int, error)
	// FetchAssets and FetchBurst return the assets in the order of PhotoKit;
	// none found is not an error. FetchBurst returns every frame with all,
	// otherwise the selected ones and the key photo.
	FetchAssets(ids []string) ([]AssetRecord, error)
	FetchBurst(burstID string, all bool) ([]AssetRecord, error)
	// AssetResources of an unknown id gives none, not an error.
	AssetResources(id string) ([]ResourceRecord, error)
	// RequestResource starts requestDataForAssetResource with
	// networkAccessAllowed for the first resource of type typ of the asset id;
	// no such asset or resource is ErrNoResource.
	RequestResource(id string, typ int, sink ResourceSink) (Request, error)
	// RequestImage starts requestImageDataAndOrientationForAsset for the asset
	// id with version, networkAccessAllowed and HighQualityFormat; no such
	// asset is ErrNoResource. The data goes to sink until Done; Cancel stops the
	// request at its next report of progress, which PhotoKit gives only while it
	// downloads. Done delivers ErrNoResource when PhotoKit gives no data
	// (photokit.py:626-627).
	RequestImage(id string, version int, sink ImageSink) (Request, error)
	// RequestVideo starts requestAVAssetForVideo for the asset id with
	// version, a PHVideoRequestOptionsVersion, networkAccessAllowed and
	// HighQualityFormat; no such asset is ErrNoResource. path, which must not
	// be nil, gets the file of an AVURLAsset of a file URL once, on a queue of
	// PhotoKit before Done; it is not called for an AVAsset without a file, such
	// as an AVComposition.
	// Cancel is cancelImageRequest:.
	RequestVideo(id string, version int, path func(string)) (Request, error)
	// ExportComposition starts requestAVAssetForVideo for the asset id as
	// RequestVideo does, then renders the AVComposition it gives into the file
	// path, which must not exist, through AVAssetExportSession with
	// AVAssetExportPresetHighestQuality, AVFileTypeQuickTimeMovie and
	// shouldOptimizeForNetworkUse (photokit.py:866-873). An AVURLAsset of a file
	// URL is not rendered: file, which must not be nil, gets its path as path of
	// RequestVideo does, and nothing is written to path. No such asset is ErrNoResource. Done delivers
	// nil once the session has completed or file has its path, the
	// *PhotoKitError of the request or of a failed session, ErrNoComposition for
	// an AVAsset that is neither a composition of two tracks nor such a file, or
	// ErrCanceled after Cancel, which cancels the request or the session,
	// whichever runs, and keeps a session that has not started from starting.
	ExportComposition(id string, version int, path string, file func(string)) (Request, error)
	// Import runs one performChanges that creates the asset of r and adds it to
	// its albums, and blocks until the completion handler, or up to timeout,
	// then returns context.DeadlineExceeded while the change goes on; so does an
	// exception in a change that PhotoKit commits. It returns the LocalIdentifier
	// of the created asset; a failed change is a *PhotoKitError.
	Import(r ImportRecord, timeout time.Duration) (string, error)
	// Collections returns the user folders and albums, parents first, in the
	// order of PhotoKit; none with an error.
	Collections() ([]CollectionRecord, error)
	// CreateCollection, which returns the LocalIdentifier of the collection,
	// RenameCollection, DeleteCollections, AlbumAssets and DeleteAssets run one
	// performChanges each and wait and fail as Import does.
	CreateCollection(title, parent string, folder bool, timeout time.Duration) (string, error)
	RenameCollection(id, title string, timeout time.Duration) error
	DeleteCollections(ids []string, timeout time.Duration) error
	AlbumAssets(album string, assets []string, add bool, timeout time.Duration) error
	DeleteAssets(ids []string, timeout time.Duration) error
	// UpdateAsset runs one performChanges that changes the asset id as r says
	// and waits and fails as Import does; an asset that is not found or cannot
	// be changed is ErrNoAsset and nothing was changed.
	UpdateAsset(id string, r PatchRecord, timeout time.Duration) error
	// CloudIdentifiers returns the pairs of the assets ids that have a cloud
	// identifier; LocalIdentifiers those of the cloud identifiers cloudIDs the
	// library resolves. None is not an error.
	CloudIdentifiers(ids []string) ([]IdentifierPair, error)
	LocalIdentifiers(cloudIDs []string) ([]IdentifierPair, error)
}

// IdentifierPair is one asset by its LocalIdentifier and its cloud identifier.
type IdentifierPair struct{ Local, Cloud string }

// PatchRecord is an AssetPatch of photokit for the bridge: each value holds
// only with its Has flag; the date is seconds since 1970, the coordinates
// degrees.
type PatchRecord struct {
	HasCreationDate     bool
	CreationDate        float64
	HasFavorite         bool
	Favorite            bool
	HasHidden           bool
	Hidden              bool
	HasLocation         bool
	Latitude, Longitude float64
}

// CollectionRecord is a Collection of photokit for the bridge.
type CollectionRecord struct {
	LocalIdentifier, Title, Parent string
	IsFolder                       bool
}

// ImportRecord is an ImportRequest of photokit for the bridge: a path or the
// name is "" when not given, the date is seconds since 1970 and the coordinates
// are degrees, each held only with its Has flag. Albums and a non-empty
// OriginalFilename are as Bridge says; a path is taken as it is.
type ImportRecord struct {
	Photo, Video, PairedVideo, AlternatePhoto, OriginalFilename string
	HasCreationDate                                             bool
	CreationDate                                                float64
	HasLocation                                                 bool
	Latitude, Longitude                                         float64
	Favorite, Hidden                                            bool
	Albums                                                      []string
}

// ResourceSink receives a resource on a queue of PhotoKit. Write gets the
// chunks in order and does not keep p; after an error of Write the bridge
// calls it no more and the caller cancels the request. Progress gets the share
// received, from 0 to 1.
type ResourceSink interface {
	Write(p []byte) error
	Progress(fraction float64)
}

// ImageSink receives one image: Write gets the bytes once, Info the record,
// both on a queue of the bridge before Done, neither when PhotoKit gives no
// data.
type ImageSink interface {
	ResourceSink
	Info(ImageRecord)
}

// ImageRecord is the ImageInfo of photokit for the bridge; Metadata is the
// binary property list of the CGImageSource properties, nil when PhotoKit gave
// none or they could not be serialized.
type ImageRecord struct {
	UTI         string
	Orientation int
	Degraded    bool
	Path        string
	Metadata    []byte
}

// Request is a running RequestResource, RequestImage, RequestVideo or
// ExportComposition. Done delivers at most once, without waiting for a
// receiver: nil, a *PhotoKitError, ErrNoResource of an image without data,
// ErrNoComposition, the error of an Objective-C exception in a handler of an
// image or a video or of a session that ends neither completed nor failed, or
// ErrCanceled after Cancel, since PhotoKit calls the completion handler after
// a cancel too, unless a stalled iCloud download holds it back. A second
// Cancel does nothing.
type Request interface {
	Cancel()
	Done() <-chan error
}

// PhotoKitError is the NSError of a completion handler by its domain and code
// only: its description may name a file.
type PhotoKitError struct {
	Domain string
	Code   int64
}

// Error implements error.
func (e *PhotoKitError) Error() string { return fmt.Sprintf("%s code %d", e.Domain, e.Code) }

// ResourceRecord is a PHAssetResource as the bridge reads it; a string is ""
// when PhotoKit gives none.
type ResourceRecord struct {
	Type                  int
	OriginalFilename, UTI string
}

// AssetRecord is a PHAsset as the bridge reads it; photokit makes an Asset of
// it. A date is nil without a value, the coordinates are nil without a
// location.
type AssetRecord struct {
	LocalIdentifier         string
	MediaType               int
	MediaSubtypes           uint64
	BurstIdentifier         string
	SourceType              int
	PixelWidth, PixelHeight int
	CreationDate            *time.Time
	ModificationDate        *time.Time
	Latitude, Longitude     *float64
	Duration                time.Duration
	Favorite, Hidden        bool
}

// Half of int64 in nanoseconds, so that dispatch_time cannot overflow when it
// adds the current time.
const maxTimeoutMS = math.MaxInt64 / 2 / int64(time.Millisecond)

func timeoutMS(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	// Rounded down, a positive timeout under 1 ms would be 0, which does not
	// wait at all.
	return min(max(d.Milliseconds(), 1), maxTimeoutMS)
}
