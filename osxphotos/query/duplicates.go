package query

import (
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Duplicates returns the photos of photos, other than p, with the signature of
// p: osxphotos PhotoInfo.duplicates (photoinfo.py:1426-1439,
// photosdb/photosdb.py:3411-3420), in the order of photos. The signature is
// OriginalFilesize, the moment of Date, Height, Width, UTI and AdjustmentType
// (both nil or equal). A photo with the UUID of p is not a duplicate. The
// result is nil when there is none.
func Duplicates(p *photoinfo.PhotoInfo, photos []*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
	s := signatureOf(p)
	var out []*photo
	for _, q := range photos {
		if q.UUID != p.UUID && signatureOf(q) == s {
			out = append(out, q)
		}
	}
	return out
}

type signature struct {
	size          int64
	date          time.Time
	height, width int
	uti           string
	adjusted      bool
	adjustment    int
}

func signatureOf(p *photo) signature {
	// A map key: with one Location and no monotonic reading, == of dates is
	// Equal (time.Time).
	s := signature{size: p.OriginalFilesize, date: p.Date.UTC().Round(0), height: p.Height, width: p.Width, uti: p.UTI}
	if p.AdjustmentType != nil {
		s.adjusted, s.adjustment = true, *p.AdjustmentType
	}
	return s
}

// withDuplicates is the duplicate step of photoquery.py:754-771. The
// duplicates come from the whole catalog, as get_photo does
// (photoinfo.py:1435); the signature index is that of photosdb.py:2358-2363.
func withDuplicates(in, catalog []*photo, o Options) []*photo {
	bySignature := map[signature][]*photo{}
	for _, p := range catalog {
		s := signatureOf(p)
		bySignature[s] = append(bySignature[s], p)
	}
	var out []*photo
	for _, p := range Sort(in) {
		if dups := Duplicates(p, bySignature[signatureOf(p)]); len(dups) > 0 {
			out = append(append(out, p), Sort(dups)...)
		}
	}
	return slices.DeleteFunc(withoutRepeatedUUIDs(out), func(p *photo) bool { return !inTrashSelection(o, p) })
}
