package golden

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const cloudKey = "cloud_metadata"

// Numbers stay literals until refNumber reads them: a plain decode would round
// an integer above 2^53.
type cloudObject map[string]any

func (c *cloudObject) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode((*map[string]any)(c))
}

// json() writes the property list of a photo as it is, {} for a photo without
// one and for one that does not decode (photoinfo.py:1739-1760, 2222). The Go
// side is the decoded property list, whose integers are int64 or uint64: both
// sides are compared by value, so a real that is a whole number equals the
// integer of that value.
func cloudMetadataRule() rule {
	return rule{
		key:         cloudKey,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
			obj, err := decode[cloudObject](o, cloudKey, false, "an object")
			if err != nil {
				return nil, err
			}
			return cloudTree(map[string]any(obj), refNumber)
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			return cloudTree(e.cloudMeta[p.UUID], goLeaf)
		},
	}
}

// cloudTree copies v with leaf applied to everything but dictionaries and
// lists, so that neither side is changed.
func cloudTree(v any, leaf func(any) (any, error)) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			var err error
			if out[k], err = cloudTree(e, leaf); err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			var err error
			if out[i], err = cloudTree(e, leaf); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return leaf(v)
}

func refNumber(v any) (any, error) {
	n, ok := v.(json.Number)
	if !ok {
		return v, nil
	}
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	if u, err := strconv.ParseUint(n.String(), 10, 64); err == nil {
		return u, nil
	}
	f, err := n.Float64()
	if err != nil {
		return nil, shape(cloudKey, "numbers that fit a double")
	}
	return f, nil
}

// json.dumps of upstream cannot write bytes, so the reference has none. A real
// of four bytes is a double for Python. A date of a property list is a naive
// UTC datetime there, written by isoformat() with six digits of fraction when
// the microseconds are not 0.
func goLeaf(v any) (any, error) {
	switch x := v.(type) {
	case []byte:
		return nil, shape(cloudKey, "values that json.dumps writes")
	case float32:
		return float64(x), nil
	case time.Time:
		x = x.UTC()
		if x.Nanosecond() >= int(time.Microsecond) {
			return x.Format("2006-01-02T15:04:05.000000"), nil
		}
		return x.Format("2006-01-02T15:04:05"), nil
	}
	return v, nil
}
