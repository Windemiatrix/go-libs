package golden

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

func searchOf(e *env, p *photoinfo.PhotoInfo, normalized bool) searchinfo.SearchInfo {
	if normalized {
		return e.search[p.UUID].Normalized()
	}
	return e.search[p.UUID]
}

// labels and labels_normalized of a photo (photoinfo.py:1651-1669, 2166, 2226),
// sorted by SearchInfo as upstream sorts them (searchinfo.py:401-413).
func labelsRule(key string, normalized bool) rule {
	return rule{
		key:         key,
		needsAlbums: true,
		ref:         func(o Object) (any, error) { return refStrings(o, key) },
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			return strs(searchOf(e, p, normalized).Labels()), nil
		},
	}
}

// Upstream writes asdict for every photo of Photos 5 and later, [] and "" for
// one without terms (photoinfo.py:2229-2236; searchinfo.py:347-413), so null,
// {} and null for a key are errors of the form. The asdict of the zero
// SearchInfo tells which of the 44 keys hold a list.
func searchInfoRule(key string, normalized bool) rule {
	zero := searchinfo.SearchInfo{}.AsDict()
	return rule{
		key:         key,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
			raw, err := decode[map[string]json.RawMessage](o, key, false, "an object")
			if err != nil {
				return nil, err
			}
			out := make(map[string]any, len(zero))
			for _, k := range slices.Sorted(maps.Keys(zero)) {
				path := key + "." + k
				if _, isList := zero[k].([]string); isList {
					var list []*string
					list, err = decodeAt[[]*string](raw, k, path, false, "a list of strings")
					out[k] = derefAll(list)
				} else {
					out[k], err = decodeAt[string](raw, k, path, false, "a string")
				}
				if err != nil {
					return nil, err
				}
			}
			return out, nil
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			d := searchOf(e, p, normalized).AsDict()
			for k, v := range d {
				if list, isList := v.([]string); isList {
					d[k] = strs(list)
				}
			}
			return d, nil
		},
	}
}
