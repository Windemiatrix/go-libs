package photosdb

import "github.com/Windemiatrix/go-libs/osxphotos/photoinfo"

// BurstPhotos returns the other photos of the burst of p, those in the trash
// and the unselected frames included, in UUID order; nil when there are none:
// osxphotos PhotoInfo.burst_photos (photoinfo.py:1031-1044). The values are
// shared: do not modify them.
func (db *DB) BurstPhotos(p *photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
	if !p.Burst {
		return nil
	}
	var others []*photoinfo.PhotoInfo
	for _, f := range db.bursts[p.BurstUUID] {
		if f.UUID != p.UUID {
			others = append(others, f)
		}
	}
	return others
}

// BurstKeyPhoto returns the key photo of the burst of p, p itself when it is
// the key: osxphotos PhotoInfo.burst_key_photo (photoinfo.py:1009-1019).
// Upstream iterates a set, so a burst with several key photos gives any of
// them; here it is the first by UUID. The value is shared: do not modify it.
func (db *DB) BurstKeyPhoto(p *photoinfo.PhotoInfo) (*photoinfo.PhotoInfo, bool) {
	if !p.Burst {
		return nil, false
	}
	if p.BurstKey {
		return p, true
	}
	for _, f := range db.bursts[p.BurstUUID] {
		if f.BurstKey {
			return f, true
		}
	}
	return nil, false
}

// PhotosByUUID returns the photos with these UUIDs in their order, unknown
// ones left out, repeats kept, those in the trash included; nil when there are
// none: osxphotos photos_by_uuid (photosdb/photosdb.py:3378-3394). The values
// are shared: do not modify them.
func (db *DB) PhotosByUUID(uuids []string) []*photoinfo.PhotoInfo {
	var photos []*photoinfo.PhotoInfo
	for _, u := range uuids {
		if p, ok := db.byUUID[u]; ok {
			photos = append(photos, p)
		}
	}
	return photos
}

// indexBursts groups the frames of the bursts by BurstUUID, "" included
// (photosdb/photosdb.py:2212-2220), in the order of photos. Built once in
// Open: a scan of the catalog per photo would be quadratic.
func indexBursts(photos []*photoinfo.PhotoInfo) map[string][]*photoinfo.PhotoInfo {
	bursts := map[string][]*photoinfo.PhotoInfo{}
	for _, p := range photos {
		if p.Burst {
			bursts[p.BurstUUID] = append(bursts[p.BurstUUID], p)
		}
	}
	return bursts
}
