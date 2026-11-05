package momentinfo

import "time"

// MomentInfo is a moment of the library, the MomentInfo of osxphotos
// (momentinfo.py:5-55): one ZMOMENT row (photosdb/photosdb.py:2697-2785).
// The dates are in the fixed zone of ZTIMEZONEOFFSET (0 when NULL), and
// 1970-01-01T00:00:00Z when the row has none, an unreadable one or an offset
// outside (-86400, 86400) seconds, as photos_datetime with default=True gives
// (2760-2771). Title and Subtitle are in NFKC, "" for NULL (2773-2783): Photos
// writes a no-break space in them. Latitude and Longitude are nil when both
// columns are -180, the "no location" of Photos (2749-2758), and each is nil
// when its column is NULL.
type MomentInfo struct {
	PK                                         int64 // Z_PK
	Title, Subtitle                            string
	StartDate, EndDate, Date, ModificationDate time.Time // ZSTARTDATE, ZENDDATE, ZREPRESENTATIVEDATE, ZMODIFICATIONDATE
	Latitude, Longitude                        *float64  // ZAPPROXIMATELATITUDE, ZAPPROXIMATELONGITUDE
}
