package photokit

import "time"

// MediaType is PHAssetMediaType (photokit.py:354-357). A value without a
// constant is kept as PhotoKit gives it.
type MediaType int

// The values of PHAssetMediaType, pinned by the _Static_assert of bridge.m.
const (
	MediaTypeUnknown MediaType = iota
	MediaTypeImage
	MediaTypeVideo
	MediaTypeAudio
)

// The PHAssetMediaSubtype bits of PhotoAsset (photokit.py:364-409), pinned by the _Static_assert of bridge.m.
const (
	subtypePanorama      = 1 << 0
	subtypeHDR           = 1 << 1
	subtypeScreenshot    = 1 << 2
	subtypeLive          = 1 << 3
	subtypeDepthEffect   = 1 << 4
	subtypeStreamed      = 1 << 16
	subtypeHighFrameRate = 1 << 17
	subtypeTimelapse     = 1 << 18
	// The SDK names this bit PHAssetMediaSubtypeVideoScreenRecording; upstream reads PHAssetMediaSubtypePhotoScreenRecording, which the SDK does not declare (photokit.py:383).
	subtypeScreenRecording = 1 << 19
)

// Location is the coordinate of the CLLocation of an asset, in degrees
// (photokit.py:446-449).
type Location struct{ Latitude, Longitude float64 }

// Asset is a PHAsset with the properties of PhotoAsset (photokit.py:285-464),
// read when it is fetched: a change in Photos after that shows in the next
// fetch only. LocalIdentifier is what upstream calls uuid (290-293): the UUID
// of the library with the suffix PhotoKit appends, such as "/L0/001".
// MediaType and Subtypes are PHAssetMediaType and the PHAssetMediaSubtype mask
// as PhotoKit gives them (354-362); IsPhoto, IsMovie and IsAudio follow
// MediaType (295-308); the flags from Panorama to Portrait are bits of Subtypes
// (364-409): SlowMo is a high frame rate video, Portrait a depth effect photo.
// OriginalFilename, RAWFilename, UTIRaw and HasAdjustments come from the
// resources (310-352, 484-493): OriginalFilename is that of the first Photo
// resource of a photo and of the first Video one of anything else, RAWFilename
// and UTIRaw those of the first AlternatePhoto one of a photo, "" without one;
// HasAdjustments is whether one resource is AdjustmentData, for any media type.
// BurstID is "" outside a burst (411-419). Date and DateModified are nil when
// PhotoKit gives none (436-444), Location is nil without one (446-449).
type Asset struct {
	LocalIdentifier string
	MediaType       MediaType
	Subtypes        uint64

	IsPhoto, IsMovie, IsAudio bool

	OriginalFilename, RAWFilename, UTIRaw string
	HasAdjustments                        bool

	Panorama, HDR, Screenshot, ScreenRecording, Live bool
	Streamed, SlowMo, TimeLapse, Portrait            bool

	BurstID    string
	Burst      bool
	SourceType int

	PixelWidth, PixelHeight int
	Date, DateModified      *time.Time
	Location                *Location
	Duration                time.Duration
	Favorite, Hidden        bool
}

// ResourceType is PHAssetResourceType. A value without a constant is kept as
// PhotoKit gives it: a newer macOS may add one.
type ResourceType int

// The values of PHAssetResourceType, pinned by the _Static_assert of bridge.m.
const (
	ResourcePhoto ResourceType = iota + 1
	ResourceVideo
	ResourceAudio
	ResourceAlternatePhoto // the RAW of a RAW+JPEG pair (photokit.py:324-334)
	ResourceFullSizePhoto
	ResourceFullSizeVideo
	ResourceAdjustmentData
	ResourceAdjustmentBasePhoto
	ResourcePairedVideo // the video of a Live Photo
	ResourceFullSizePairedVideo
	ResourceAdjustmentBasePairedVideo
	ResourceAdjustmentBaseVideo
)

// Resource is a PHAssetResource of an asset (photokit.py:769-772); UTI is its
// uniformTypeIdentifier. A string is "" when PhotoKit gives none.
type Resource struct {
	Type                  ResourceType
	OriginalFilename, UTI string
}
