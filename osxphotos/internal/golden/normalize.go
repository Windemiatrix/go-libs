package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
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
	raw, ok := o[key]
	if !ok {
		var zero T
		return zero, shape(key, "a value (the key is missing)")
	}
	return decodeRaw[T](raw, key, nullOK, want)
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

func goMoment(t *time.Time, missingDefault time.Time) any {
	switch {
	case t != nil:
		return momentString(*t)
	case !missingDefault.IsZero():
		return momentString(missingDefault)
	default:
		return nil
	}
}

// The copy is never nil: it is written as [], not null.
func strs(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}
