package golden

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

// Upstream writes asdict for every photo of Photos 5 and later, [] and "" for
// one without terms (photoinfo.py:2227-2234; searchinfo.py:347-413), so null,
// {} and null for a key are errors of the form. The asdict of the zero
// SearchInfo tells which of the 44 keys hold a list.
func searchInfoRule(key string) rule {
	zero := searchinfo.SearchInfo{}.AsDict()
	return simple(key,
		func(o Object) (any, error) {
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
		pyValue)
}
