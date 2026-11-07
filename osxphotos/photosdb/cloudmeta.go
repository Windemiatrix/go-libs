package photosdb

import "github.com/Windemiatrix/go-libs/osxphotos/photoinfo"

// CloudMetadataOf returns the media metadata iCloud stored for the master of
// p, osxphotos cloud_metadata (photoinfo.py:1739-1760): the property list of
// ZCLOUDMASTERMEDIAMETADATA.ZDATA of the ZCLOUDMASTER row of ZASSET.ZMASTER,
// as howett.net/plist decodes it (string, int64 or uint64, float32 or float64, bool,
// time.Time in UTC, []byte, []any, map[string]any). It is nil for a photo
// without a master or a row, for data over 1 MiB or refused by the limits of
// the property list reader, and for a root that is not a dictionary, where
// upstream gives {} for any failure. The map is new on every call: the caller
// may change it.
func (db *DB) CloudMetadataOf(p *photoinfo.PhotoInfo) map[string]any {
	// The raw value is kept and decoded here: the decoded map of every photo
	// would be several times the size of its property list.
	m, _ := plistAny(db.cloudMeta[p.UUID]).(map[string]any)
	return m
}
