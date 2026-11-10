package photosdb

import (
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/uti"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// The codes of ZINTERNALRESOURCE.ZCOMPACTUTI (photosdb/photosdb_utils.py:223-270).
var compactUTICodes = map[string]string{
	"1":  "public.jpeg",
	"3":  "public.heic",
	"4":  "public.heif",
	"5":  "public.avci",
	"6":  "public.png",
	"7":  "com.compuserve.gif",
	"8":  "public.tiff",
	"9":  "com.adobe.raw-image",
	"10": "com.sony.arw-raw-image",
	"11": "com.canon.cr2-raw-image",
	"12": "com.canon.crw-raw-image",
	"13": "com.olympus.raw-image",
	"14": "com.panasonic.rw2-raw-image",
	"15": "com.panasonic.raw-image",
	"16": "com.pentax.raw-image",
	"17": "com.nikon.raw-image",
	"18": "com.samsung.raw-image",
	"19": "com.leafamerica.raw-image",
	"20": "com.hasselblad.3fr-raw-image",
	"21": "com.fuji.raw-image",
	"22": "com.hasselblad.fff-raw-image",
	"23": "com.apple.quicktime-movie",
	"24": "public.mpeg-4",
	"25": "com.apple.m4v-video",
	"26": "public.3gpp",
	"27": "public.avi",
	"28": "com.apple.m4a-audio",
	"29": "com.apple.coreaudio-format",
	"30": "com.microsoft.waveform-audio",
	"31": "public.aifc-audio",
	"32": "public.aiff-audio",
	"33": "public.mp3",
	"34": "public.ac3-audio",
	"35": "com.apple.property-list",
	"36": "public.data",
	"37": "com.apple.photos.apple-adjustment-envelope",
	"38": "public.avif",
	"39": "public.jpeg-xl",
}

// uti_for_compact_uti (photosdb/photosdb_utils.py:273-283); "" where upstream
// has None.
func utiForCompactUTI(code string) string {
	if u, ok := strings.CutPrefix(code, "_"); ok {
		return u
	}
	return compactUTICodes[code]
}

// uti_if_raw (photosdb/photosdb_utils.py:286-293).
func utiIfRaw(u string) string {
	if strings.Contains(u, rawUTIMarker) {
		return u
	}
	return ""
}

// setRawUTI follows photosdb/photosdb.py:2544-2581, 2707-2715. Of several RAW
// resources the highest Z_PK decides, as in isMissing; upstream keeps the
// last row read.
func setRawUTI(p *photoinfo.PhotoInfo, res []sqlread.Resource) (code string, unresolved bool) {
	var raw *sqlread.Resource
	for i := range res {
		if is(res[i].Subtype, resRAW) {
			raw = &res[i]
		}
	}
	if raw == nil {
		return "", false
	}
	p.HasRAW = true
	u := utiForCompactUTI(raw.CompactUTI.String)
	p.UTIRaw = utiIfRaw(u)
	return raw.CompactUTI.String, u == "" && raw.CompactUTI.Valid
}

// learnRawUTIs follows photosdb/photosdb.py:2716-2729: an unresolved code
// takes the most common RAW type of the files on disk with that code. Of
// equal counts the type met first in UUID order wins; upstream takes the
// first in the order of the asset rows. A photo with its RAW file keeps the
// type of the file (photoinfo.py:940-944).
func learnRawUTIs(c *catalog) {
	type pair struct{ code, uti string }
	counts := map[pair]int{}
	var order []pair
	for _, p := range c.photos {
		code, ok := c.rawCodes[p.UUID]
		if !ok || p.PathRaw == "" {
			continue
		}
		k := pair{code, utiIfRaw(uti.ForExtension(suffix(p.PathRaw)))}
		if k.uti == "" {
			continue
		}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	best := map[string]pair{}
	for _, k := range order {
		if b, ok := best[k.code]; !ok || counts[k] > counts[b] {
			best[k.code] = k
		}
	}
	for _, p := range c.photos {
		if code, ok := c.rawCodes[p.UUID]; ok && p.PathRaw == "" {
			p.UTIRaw = best[code].uti
		}
	}
}
