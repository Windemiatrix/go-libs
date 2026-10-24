package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A shapeError says that a value of the reference has not the form the rule
// of its key expects. It names the key and the form, never the value: the
// reference may be the library of its owner.
type shapeError struct {
	path string // the key, with the index of a list element and a nested key
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

// first returns the first byte of a value, 0 for an empty one.
func first(raw json.RawMessage) byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	return raw[0]
}

func decodeString(raw json.RawMessage) (string, bool) {
	var s string
	if first(raw) != '"' || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

func decodeBool(raw json.RawMessage) (value, ok bool) {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// decodeInt accepts a JSON integer that fits int64. A number written with a
// fraction or an exponent (3024.0) is not an integer: osxphotos writes these
// keys from integer columns.
func decodeInt(raw json.RawMessage) (int64, bool) {
	n, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	return n, err == nil
}

func decodeFloat(raw json.RawMessage) (float64, bool) {
	if c := first(raw); c != '-' && (c < '0' || c > '9') {
		return 0, false
	}
	var f float64
	return f, json.Unmarshal(raw, &f) == nil
}

func decodeList(raw json.RawMessage) ([]json.RawMessage, bool) {
	var items []json.RawMessage
	if first(raw) != '[' || json.Unmarshal(raw, &items) != nil {
		return nil, false
	}
	return items, true
}

func decodeObject(raw json.RawMessage) (Object, bool) {
	var o Object
	if first(raw) != '{' || json.Unmarshal(raw, &o) != nil {
		return nil, false
	}
	return o, true
}

// field returns the value of a key of a reference object.
func field(o Object, key string) (json.RawMessage, error) {
	raw, ok := o[key]
	if !ok {
		return nil, shape(key, "a value (the key is missing)")
	}
	return raw, nil
}

// refBool reads a boolean; null is a mistake of the form (upstream gives
// None only for Photos before 7, which the reference does not cover).
func refBool(o Object, key string) (bool, error) {
	raw, err := field(o, key)
	if err != nil {
		return false, err
	}
	b, ok := decodeBool(raw)
	if !ok {
		return false, shape(key, "a boolean")
	}
	return b, nil
}

// refString reads a string; null is the empty string when nullOK.
func refString(o Object, key string, nullOK bool) (string, error) {
	raw, err := field(o, key)
	if err != nil {
		return "", err
	}
	return stringValue(raw, key, nullOK)
}

func stringValue(raw json.RawMessage, path string, nullOK bool) (string, error) {
	if nullOK && isNull(raw) {
		return "", nil
	}
	s, ok := decodeString(raw)
	if !ok {
		if nullOK {
			return "", shape(path, "a string or null")
		}
		return "", shape(path, "a string")
	}
	return s, nil
}

// refInt reads an integer; null is 0 when nullOK.
func refInt(o Object, key string, nullOK bool) (int64, error) {
	raw, err := field(o, key)
	if err != nil {
		return 0, err
	}
	if nullOK && isNull(raw) {
		return 0, nil
	}
	n, ok := decodeInt(raw)
	if !ok {
		return 0, shape(key, "an integer")
	}
	return n, nil
}

// refIntOrNull reads an integer; null stays nil.
func refIntOrNull(o Object, key string) (any, error) {
	raw, err := field(o, key)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil // null is a value of the reference
	}
	n, ok := decodeInt(raw)
	if !ok {
		return nil, shape(key, "an integer or null")
	}
	return n, nil
}

// floatOrNull reads a number or null; null stays nil.
func floatOrNull(raw json.RawMessage, path string) (any, error) {
	if isNull(raw) {
		return nil, nil // null is a value of the reference
	}
	f, ok := decodeFloat(raw)
	if !ok {
		return nil, shape(path, "a number or null")
	}
	return f, nil
}

// refStrings reads a list of strings; null in the list is "" as in the sort
// key of upstream (photoinfo.py:2285).
func refStrings(o Object, key string) ([]string, error) {
	raw, err := field(o, key)
	if err != nil {
		return nil, err
	}
	return stringsValue(raw, key)
}

func stringsValue(raw json.RawMessage, path string) ([]string, error) {
	items, ok := decodeList(raw)
	if !ok {
		return nil, shape(path, "a list of strings")
	}
	out := make([]string, len(items))
	for i, it := range items {
		s, err := stringValue(it, fmt.Sprintf("%s[%d]", path, i), true)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// parseMoment reads the ISO 8601 form of Python datetime.isoformat:
// YYYY-MM-DDTHH:MM:SS, then up to nine fraction digits, then Z or a UTC
// offset ±HH:MM or ±HH:MM:SS. time.Parse does not read the last form. The
// error of this function is a bool, because time.Parse errors repeat the
// input.
func parseMoment(s string) (time.Time, bool) {
	// The first 19 bytes have a fixed layout; at least one more byte (Z or an
	// offset) follows.
	if len(s) < 20 || s[4] != '-' || s[7] != '-' || s[10] != 'T' || s[13] != ':' || s[16] != ':' {
		return time.Time{}, false
	}
	year, month, day := digits(s[0:4]), digits(s[5:7]), digits(s[8:10])
	hour, minute, sec := digits(s[11:13]), digits(s[14:16]), digits(s[17:19])
	rest := s[19:]

	nsec := 0
	if rest[0] == '.' {
		n := 1
		for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
			n++
		}
		if n == 1 || n > 10 { // no digits, or more than nine
			return time.Time{}, false
		}
		nsec = digits(rest[1:n])
		for k := n - 1; k < 9; k++ {
			nsec *= 10
		}
		rest = rest[n:]
	}
	offset, ok := parseOffset(rest)

	if !ok || year < 1 || month < 1 || month > 12 || day < 1 || hour < 0 || hour > 23 ||
		minute < 0 || minute > 59 || sec < 0 || sec > 59 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, sec, nsec, time.UTC)
	if t.Day() != day { // 31 February and the like
		return time.Time{}, false
	}
	return t.Add(-time.Duration(offset) * time.Second), true
}

// parseOffset reads Z, ±HH:MM or ±HH:MM:SS and returns seconds east of UTC.
func parseOffset(s string) (int, bool) {
	if s == "Z" {
		return 0, true
	}
	if (len(s) != 6 && len(s) != 9) || (s[0] != '+' && s[0] != '-') || s[3] != ':' || (len(s) == 9 && s[6] != ':') {
		return 0, false
	}
	h, m, sec := digits(s[1:3]), digits(s[4:6]), 0
	if len(s) == 9 {
		sec = digits(s[7:9])
	}
	if h < 0 || h > 23 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, false
	}
	offset := h*3600 + m*60 + sec
	if s[0] == '-' {
		offset = -offset
	}
	return offset, true
}

// digits reads a string of decimal digits; -1 if there is another byte.
func digits(s string) int {
	n := 0
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// momentString is the canonical form of a moment: RFC 3339 in UTC with the
// fraction as long as needed. The offset of the value is not part of it.
func momentString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// refMoment reads an ISO 8601 string as a moment; null is nil when nullOK.
func refMoment(o Object, key string, nullOK bool) (any, error) {
	raw, err := field(o, key)
	if err != nil {
		return nil, err
	}
	return momentValue(raw, key, nullOK)
}

func momentValue(raw json.RawMessage, path string, nullOK bool) (any, error) {
	if nullOK && isNull(raw) {
		return nil, nil // null is a value of the reference
	}
	t, err := parseRefMoment(raw, path)
	if err != nil {
		return nil, err
	}
	return momentString(t), nil
}

// parseRefMoment reads a JSON string of the reference as a moment.
func parseRefMoment(raw json.RawMessage, path string) (time.Time, error) {
	s, ok := decodeString(raw)
	if !ok {
		return time.Time{}, shape(path, "an ISO 8601 string")
	}
	t, ok := parseMoment(s)
	if !ok {
		return time.Time{}, shape(path, "an ISO 8601 string")
	}
	return t, nil
}

// goMoment is the canonical form of a Go moment; nil gives def, or null when
// def is the zero time.
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
