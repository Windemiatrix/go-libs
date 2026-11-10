package photosdb

import (
	"database/sql"
	"slices"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/momentinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// MomentInfoOf returns the moment of p, the ZMOMENT row of ZASSET.ZMOMENT
// (osxphotos PhotoInfo.moment_info, photoinfo.py:625-631); false when p has
// none or the row is missing, where upstream gives None.
func (db *DB) MomentInfoOf(p *photoinfo.PhotoInfo) (momentinfo.MomentInfo, bool) {
	pk, ok := db.momentOf[p.UUID]
	m, found := db.moments[pk]
	return m, ok && found
}

// MomentPhotos returns the photos of Photos() in the moment m, in ZUUID
// order (osxphotos MomentInfo.photos, momentinfo.py:57-70: photos_by_uuid over
// the catalog), those in the trash included; nil for a moment without a photo
// or one the library does not have. The slice is new on every call, its values
// are shared: do not modify them.
func (db *DB) MomentPhotos(m momentinfo.MomentInfo) []*photoinfo.PhotoInfo {
	return slices.Clone(db.momentPhotos[m.PK])
}

// momentData is not part of photoinfo.PhotoInfo: upstream keeps momentID in the
// private _info of a photo, not as a property.
type momentData struct {
	moments      map[int64]momentinfo.MomentInfo
	momentOf     map[string]int64
	momentPhotos map[int64][]*photoinfo.PhotoInfo
}

// _process_moment_5 (photosdb.py:2742-2830). A moment has no time zone name:
// assetDate with an empty one is photos_datetime with default=True and the
// offset of ZTIMEZONEOFFSET. NFKC of the title turns the no-break space that
// Photos writes in it into a space (2818-2828).
func loadMoments(t tables, c *catalog) momentData {
	d := momentData{
		moments:      make(map[int64]momentinfo.MomentInfo, len(t.moments)),
		momentOf:     c.momentOf,
		momentPhotos: map[int64][]*photoinfo.PhotoInfo{},
	}
	for _, r := range t.moments {
		m := momentinfo.MomentInfo{
			PK:               r.PK,
			Title:            norm.NFKC.String(r.Title.String),
			Subtitle:         norm.NFKC.String(r.Subtitle.String),
			StartDate:        assetDate(r.Start, r.TZOffset, sql.NullString{}),
			EndDate:          assetDate(r.End, r.TZOffset, sql.NullString{}),
			Date:             assetDate(r.Representative, r.TZOffset, sql.NullString{}),
			ModificationDate: assetDate(r.Modified, r.TZOffset, sql.NullString{}),
		}
		if !noLocation(r.Latitude, r.Longitude) {
			m.Latitude, m.Longitude = optionalFloat(r.Latitude), optionalFloat(r.Longitude)
		}
		d.moments[r.PK] = m
	}
	for _, p := range c.photos {
		pk, ok := c.momentOf[p.UUID]
		if _, found := d.moments[pk]; ok && found {
			d.momentPhotos[pk] = append(d.momentPhotos[pk], p)
		}
	}
	return d
}
