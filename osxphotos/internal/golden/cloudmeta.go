package golden

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const cloudKey = "cloud_metadata"

// json() writes the property list of a photo through its default
// (photoinfo.py:2222, 2269-2288), {} for a photo without one and for one that
// does not decode (photoinfo.py:1739-1760). The Go side is the decoded property
// list, whose integers are int64 or uint64: both sides are compared by value,
// so a real that is a whole number equals the integer of that value.
func cloudMetadataRule() rule {
	return rule{
		key:         cloudKey,
		needsAlbums: true,
		ref: func(o Object) (any, error) {
			raw, ok := o[cloudKey]
			if !ok {
				return nil, shape(cloudKey, "a value (the key is missing)")
			}
			// Numbers stay literals until refNumber reads them: a plain decode
			// would round an integer above 2^53.
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			var v any
			if dec.Decode(&v) != nil {
				return nil, shape(cloudKey, "a JSON value")
			}
			return cloudTree(v, refNumber)
		},
		got: func(e *env, p *photoinfo.PhotoInfo, _ any) (any, error) {
			return cloudTree(e.cloudMeta[p.UUID], goLeaf)
		},
	}
}

// A copy: Compare leaves its input as it is.
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

// json() hands json.dumps a default that returns None for anything but a date
// (photoinfo.py:2269-2271), so bytes and a UID are null. A real of four bytes
// is a double for Python. A date of a property list is a naive UTC datetime
// rounded to the microsecond (timedelta rounds), written by isoformat() with
// six digits of fraction when the microseconds are not 0.
func goLeaf(v any) (any, error) {
	switch x := v.(type) {
	case string, bool, int64, uint64, float64:
		return v, nil
	case float32:
		return float64(x), nil
	case time.Time:
		x = x.UTC().Round(time.Microsecond)
		if x.Nanosecond() != 0 {
			return x.Format("2006-01-02T15:04:05.000000"), nil
		}
		return x.Format("2006-01-02T15:04:05"), nil
	}
	return nil, nil
}
