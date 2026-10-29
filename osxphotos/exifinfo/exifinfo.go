package exifinfo

import "time"

// ExifInfo is the EXIF data of a photo as Photos stored it on import, the
// ExifInfo of osxphotos (exifinfo.py:14-39), from one ZEXTENDEDATTRIBUTES row.
// A pointer is nil and a string "" where upstream has None: a NULL column, or
// no row at all (exifinfo_factory(None), exifinfo.py:42-45), in which case
// every field is zero. The data describes the original file, not the image in
// the library: upstream says ExifInfo does not read the file.
//
// The order of the fields is that of the keys of exif_info in
// internal/golden.
type ExifInfo struct {
	FlashFired *bool // ZFLASHFIRED == 1; nil without a row (exifinfo.py:49)

	ISO, MeteringMode, SampleRate, TrackFormat, WhiteBalance *int // ZISO, ZMETERINGMODE, ZSAMPLERATE, ZTRACKFORMAT, ZWHITEBALANCE (exifinfo.py:48-53)

	Aperture, BitRate, Duration, ExposureBias, FocalLength, FPS *float64 // ZAPERTURE, ZBITRATE, ZDURATION (seconds), ZEXPOSUREBIAS, ZFOCALLENGTH, ZFPS (exifinfo.py:54-59)

	Latitude, Longitude, ShutterSpeed *float64 // ZLATITUDE, ZLONGITUDE, ZSHUTTERSPEED (exifinfo.py:60-62)

	CameraMake, CameraModel, Codec, LensModel string // ZCAMERAMAKE, ZCAMERAMODEL, ZCODEC, ZLENSMODEL, not normalized to NFC, as upstream (exifinfo.py:63-66)

	Date     *time.Time // ZDATECREATED in the zone of TZName or TZOffset, as photoinfo.PhotoInfo.Date; nil when NULL or outside the years 1-9999, not 1970-01-01 (exifinfo.py:70-75)
	TZOffset *int       // ZTIMEZONEOFFSET, seconds east of UTC
	TZName   string     // ZTIMEZONENAME
}
