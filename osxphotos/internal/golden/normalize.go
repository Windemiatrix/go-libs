package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
)

// A shapeError names the key and the form, never the value: the reference may
// be the library of its owner.
type shapeError struct {
	path string
	want string
}

func (e *shapeError) Error() string { return e.path + ": want " + e.want }

func shape(path, want string) error { return &shapeError{path: path, want: want} }

// The error of encoding/json names the value, so it is not kept.
var errUnsupported = errors.New("the Go value cannot be written as JSON (NaN or infinity)")

// The form in which the two sides are compared and in which Diff and
// divergences.json keep values.
func canonical(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", errUnsupported
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// json.Unmarshal would turn a null into the zero value without an error. The
// error is a shapeError, because the errors of encoding/json repeat the input.
func decodeRaw[T any](raw json.RawMessage, path string, nullOK bool, want string) (T, error) {
	var v T
	if (isNull(raw) && !nullOK) || json.Unmarshal(raw, &v) != nil {
		return v, shape(path, want)
	}
	return v, nil
}

// A missing key is a mistake of the input, not a null.
func decode[T any](o Object, key string, nullOK bool, want string) (T, error) {
	return decodeAt[T](o, key, key, nullOK, want)
}

func decodeAt[T any](m map[string]json.RawMessage, key, path string, nullOK bool, want string) (T, error) {
	raw, ok := m[key]
	if !ok {
		var zero T
		return zero, shape(path, "a value (the key is missing)")
	}
	return decodeRaw[T](raw, path, nullOK, want)
}

// A null in the list is "" as in the sort key of upstream (photoinfo.py:2285).
func refStrings(o Object, key string) ([]string, error) {
	list, err := decode[[]*string](o, key, false, "a list of strings")
	if err != nil {
		return nil, err
	}
	return derefAll(list), nil
}

// The result is never nil: it is written as [], not null.
func derefAll(list []*string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = deref(s)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// The second layout is for an offset with seconds, which time.RFC3339Nano does
// not read. The error of time.Parse repeats the input, so it is dropped.
func parseMoment(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-07:00:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func momentString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseRef(s *string, path string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	t, ok := parseMoment(*s)
	if !ok {
		return nil, shape(path, "an ISO 8601 string")
	}
	return &t, nil
}

func refTime(o Object, key string, nullOK bool) (*time.Time, error) {
	s, err := decode[*string](o, key, nullOK, "an ISO 8601 string")
	if err != nil {
		return nil, err
	}
	return parseRef(s, key)
}

func momentOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return momentString(*t)
}

// The copy is never nil: it is written as [], not null.
func strs(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// The sort order of an album and bytes are null, as the default of json()
// writes what json has no form for (photoinfo.py:2269-2271); a date is a
// moment by rule 2. The result is a copy: Compare leaves its input as it is.
func pyValue(v any) any {
	switch x := v.(type) {
	case time.Time:
		return momentString(x)
	case albuminfo.AlbumSortOrder, []byte:
		return nil
	case json.Number:
		// The reference decodes the same number as a float, so 1.0 equals 1.
		f, _ := x.Float64()
		return f
	case []string:
		return strs(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = pyValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = pyValue(e)
		}
		return out
	}
	return v
}

// The reference reads null as "" for these strings (rule 3), where DB.AsDict
// gives nil for a string the port holds as "", as None upstream.
func emptyIfNull(v any) any {
	if v == nil {
		return ""
	}
	return pyValue(v)
}

// emptyFor is emptyIfNull for keys of an object that pyValue has copied. An
// empty object stays {}: upstream writes it for a missing place or session. A
// missing key stays missing, so that it is a difference, not "".
func emptyFor(v any, keys ...string) any {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return v
	}
	for _, k := range keys {
		if e, ok := m[k]; ok && e == nil {
			m[k] = ""
		}
	}
	return m
}

func eachEmptyFor(v any, keys ...string) any {
	list, ok := pyValue(v).([]any)
	if !ok {
		return pyValue(v)
	}
	for i, e := range list {
		list[i] = emptyFor(e, keys...)
	}
	return list
}

// A list of strings of DB.AsDict holds nil for None, which the sort key of
// json() takes as "" (photoinfo.py:2285), as does the reference side.
func goStrings(v any, sorted bool) any {
	var out []string
	switch x := v.(type) {
	case []string:
		out = strs(x)
	case []any:
		out = make([]string, len(x))
		for i, e := range x {
			s, ok := e.(string)
			if !ok && e != nil {
				return pyValue(v)
			}
			out[i] = s
		}
	default:
		return pyValue(v)
	}
	if sorted {
		slices.Sort(out)
	}
	return out
}
