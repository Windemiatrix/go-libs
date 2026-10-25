package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// A shapeError says that a value of the reference has not the form the rule
// of its key expects. It names the key and the form, never the value: the
// reference may be the library of its owner.
type shapeError struct {
	path string // the key, or the key and the index of an album
	want string
}

func (e *shapeError) Error() string { return e.path + ": want " + e.want }

func shape(path, want string) error { return &shapeError{path: path, want: want} }

// errUnsupported says that a Go value cannot be written as JSON (NaN or
// infinity). The error of encoding/json names the value, so it is not kept.
var errUnsupported = errors.New("the Go value cannot be written as JSON (NaN or infinity)")

// canonical writes v as compact JSON with sorted object keys and without
// HTML escaping: the form in which the two sides are compared and Diff and
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

// isNull reports a JSON null.
func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// decodeRaw reads a value of the reference as T. null is the zero value of T
// when nullOK and a mistake of the form otherwise: json.Unmarshal would give
// the zero value without an error. The error is a shapeError, because the
// errors of encoding/json repeat the input.
func decodeRaw[T any](raw json.RawMessage, path string, nullOK bool, want string) (T, error) {
	var v T
	if (isNull(raw) && !nullOK) || json.Unmarshal(raw, &v) != nil {
		return v, shape(path, want)
	}
	return v, nil
}

// decode reads the value of a key of a reference object as decodeRaw does. A
// missing key is a mistake of the input, not a null.
func decode[T any](o Object, key string, nullOK bool, want string) (T, error) {
	raw, ok := o[key]
	if !ok {
		var zero T
		return zero, shape(key, "a value (the key is missing)")
	}
	return decodeRaw[T](raw, key, nullOK, want)
}

// refStrings reads a list of strings; null in the list is "" as in the sort
// key of upstream (photoinfo.py:2285).
func refStrings(o Object, key string) ([]string, error) {
	list, err := decode[[]*string](o, key, false, "a list of strings")
	if err != nil {
		return nil, err
	}
	return derefAll(list), nil
}

// derefAll returns the strings of list, null being "". The result is never
// nil (it is written as [], not null).
func derefAll(list []*string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = deref(s)
	}
	return out
}

// deref is a string of the reference as the Go side writes it: null is "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseMoment reads the ISO 8601 form of Python datetime.isoformat. The
// second layout is for an offset with seconds, which time.RFC3339Nano does
// not read. The error of time.Parse repeats the input, so it is dropped.
func parseMoment(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-07:00:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// momentString is the canonical form of a moment: RFC 3339 in UTC with the
// fraction as long as needed. The offset of the value is not part of it.
func momentString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// parseRef reads a string of the reference as a moment; nil stays nil.
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

// refTime reads the value of a date key as a moment; null is nil when nullOK.
func refTime(o Object, key string, nullOK bool) (*time.Time, error) {
	s, err := decode[*string](o, key, nullOK, "an ISO 8601 string")
	if err != nil {
		return nil, err
	}
	return parseRef(s, key)
}

// goMoment is the canonical form of a moment; nil gives def, or null when def
// is the zero time.
func goMoment(t *time.Time, def time.Time) any {
	switch {
	case t != nil:
		return momentString(*t)
	case !def.IsZero():
		return momentString(def)
	default:
		return nil
	}
}

// strs returns a copy of s that is never nil (it is written as [], not null).
func strs(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}
