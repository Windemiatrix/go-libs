package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Divergence is one record of divergences.json: a known difference between
// osxphotos and this module, with the reason. A record explains a Diff when
// Library, Field and UUID (or "*") match and, if given, Want and Got.
type Divergence struct {
	Library  string          `json:"library"`
	UUID     string          `json:"uuid"`
	Field    string          `json:"field"`
	Want     json.RawMessage `json:"want,omitempty"`
	Got      json.RawMessage `json:"got,omitempty"`
	Reason   string          `json:"reason"`
	Upstream string          `json:"upstream"`
}

// upstreamRef is a place in the upstream sources: a path from the osxphotos
// package, a line or a range of lines.
var upstreamRef = regexp.MustCompile(`^[a-z0-9_/]+\.py:([0-9]+)(?:-([0-9]+))?$`)

const (
	allObjects   = "*"
	localLibrary = "local"
)

// LoadDivergences reads and checks divergences.json: one JSON array and
// nothing after it, records with known keys only, and for each record:
//
//   - library is one directory name and uuid, field and reason are not empty;
//     field is a key of fields.txt;
//   - upstream is a path from the osxphotos package and a line or a range of
//     lines, the range not backwards;
//   - want and got, when present, are in the canonical form (compact JSON,
//     sorted keys), and are required for uuid "*": such a record without
//     values would hide the whole key of a library;
//   - library "local" is only for uuid "*", with true, false, null or a number
//     as values: the file is public and has no UUIDs and no strings of the
//     library of its owner;
//   - no two records have the same library, uuid, field, want and got.
//
// Errors name the record number, never its content.
func LoadDivergences(path string) ([]Divergence, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	name := "divergences"
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0:
		return nil, fmt.Errorf("golden: %s: the file is empty", name)
	case !json.Valid(data):
		return nil, fmt.Errorf("golden: %s: not one valid JSON value", name)
	case data[0] != '[':
		return nil, fmt.Errorf("golden: %s: not a JSON array", name)
	}
	var items []json.RawMessage
	_ = json.Unmarshal(data, &items) // cannot fail: data is one valid JSON array
	out := make([]Divergence, 0, len(items))
	seen := map[string]int{}
	for i, item := range items {
		n := i + 1
		dec := json.NewDecoder(bytes.NewReader(item))
		dec.DisallowUnknownFields()
		var d Divergence
		if err := dec.Decode(&d); err != nil {
			return nil, fmt.Errorf("golden: %s: record %d is not an object with the keys of a record", name, n)
		}
		if err := d.check(); err != nil {
			return nil, fmt.Errorf("golden: %s: record %d: %w", name, n, err)
		}
		id := strings.Join([]string{d.Library, d.UUID, d.Field, string(d.Want), string(d.Got)}, "\x00")
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("golden: %s: record %d repeats record %d", name, n, prev)
		}
		seen[id] = n
		out = append(out, d)
	}
	return out, nil
}

// check applies the rules of LoadDivergences to one record.
func (d Divergence) check() error {
	if d.Library == "" || d.Library == "." || d.Library == ".." || strings.ContainsAny(d.Library, `/\`) {
		return errors.New("library must be one directory name")
	}
	if d.UUID == "" {
		return errors.New("uuid is empty (use \"*\" for all objects)")
	}
	if _, ok := rules[d.Field]; !ok {
		return errors.New("field is not a key of fields.txt")
	}
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("reason is empty")
	}
	if err := checkUpstream(d.Upstream); err != nil {
		return err
	}
	for _, v := range []struct {
		name string
		raw  json.RawMessage
	}{{"want", d.Want}, {"got", d.Got}} {
		if len(v.raw) > 0 && !isCanonical(v.raw) {
			return fmt.Errorf("%s is not in the canonical form (compact JSON, sorted keys)", v.name)
		}
	}
	if d.UUID == allObjects && (len(d.Want) == 0 || len(d.Got) == 0) {
		return errors.New(`uuid "*" needs both want and got`)
	}
	if d.Library == localLibrary {
		if d.UUID != allObjects {
			return errors.New(`library "local" is only for uuid "*"`)
		}
		for _, raw := range []json.RawMessage{d.Want, d.Got} {
			if !isScalar(raw) {
				return errors.New(`library "local" takes true, false, null or a number as want and got`)
			}
		}
	}
	return nil
}

func checkUpstream(s string) error {
	m := upstreamRef.FindStringSubmatch(s)
	if m == nil {
		return errors.New("upstream is not a path from the osxphotos package and a line, such as photosdb/photosdb.py:10-12")
	}
	from, err1 := strconv.Atoi(m[1])
	to := from
	var err2 error
	if m[2] != "" {
		to, err2 = strconv.Atoi(m[2])
	}
	if err1 != nil || err2 != nil || from < 1 || to < from {
		return errors.New("upstream line numbers must start at 1 and not go backwards")
	}
	return nil
}

// isCanonical reports whether raw is what canonical writes for its value.
func isCanonical(raw json.RawMessage) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return false
	}
	s, err := canonical(asValues(v))
	return err == nil && s == string(raw)
}

// asValues replaces the numbers read as json.Number by the values the
// comparison writes: an integer that is written as such stays an integer,
// any other number is a float64 (so 1.0 becomes 1 and is not canonical).
func asValues(v any) any {
	switch v := v.(type) {
	case json.Number:
		if n, err := strconv.ParseInt(string(v), 10, 64); err == nil && strconv.FormatInt(n, 10) == string(v) {
			return n
		}
		f, _ := v.Float64() // out of range gives an infinity, which canonical refuses
		return f
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = asValues(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = asValues(e)
		}
		return out
	default:
		return v
	}
}

// isScalar reports true, false, null or a number.
func isScalar(raw json.RawMessage) bool {
	switch s := string(raw); s {
	case "true", "false", "null":
		return true
	default:
		return first(raw) == '-' || (first(raw) >= '0' && first(raw) <= '9')
	}
}
