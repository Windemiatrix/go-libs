package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
var upstreamRef = regexp.MustCompile(`^[a-z0-9_/]+\.py:[0-9]+(-[0-9]+)?$`)

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
//     lines;
//   - want and got, when present, are kept in the canonical form (compact
//     JSON, sorted keys), the form of Diff; they are required for uuid "*":
//     such a record without values would hide the whole key of a library;
//   - library "local" is only for uuid "*", with true, false, null or a number
//     as values: the file is public and has no UUIDs and no strings of the
//     library of its owner;
//   - no two records have the same library, uuid, field, want and got.
//
// Errors name the record number, never its content.
func LoadDivergences(path string) ([]Divergence, error) {
	const name = "divergences"
	items, err := readArray(path, name)
	if err != nil {
		return nil, err
	}
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
		if err := d.normalize(); err != nil {
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

// normalize writes want and got in the canonical form and applies the rules
// of LoadDivergences to the record.
func (d *Divergence) normalize() error {
	for _, raw := range []*json.RawMessage{&d.Want, &d.Got} {
		if len(*raw) == 0 {
			continue
		}
		var v any
		if json.Unmarshal(*raw, &v) != nil {
			return errors.New("want or got is a number out of range")
		}
		s, err := canonical(v)
		if err != nil {
			return err
		}
		*raw = json.RawMessage(s)
	}
	switch {
	case d.Library == "" || d.Library == "." || d.Library == ".." || strings.ContainsAny(d.Library, `/\`):
		return errors.New("library must be one directory name")
	case d.UUID == "":
		return errors.New("uuid is empty (use \"*\" for all objects)")
	case !isKey(d.Field):
		return errors.New("field is not a key of fields.txt")
	case strings.TrimSpace(d.Reason) == "":
		return errors.New("reason is empty")
	case !upstreamRef.MatchString(d.Upstream):
		return errors.New("upstream is not a path from the osxphotos package and a line, such as photosdb/photosdb.py:10-12")
	case d.UUID == allObjects && (len(d.Want) == 0 || len(d.Got) == 0):
		return errors.New(`uuid "*" needs both want and got`)
	case d.Library == localLibrary && d.UUID != allObjects:
		return errors.New(`library "local" is only for uuid "*"`)
	case d.Library == localLibrary && (!isScalar(d.Want) || !isScalar(d.Got)):
		return errors.New(`library "local" takes true, false, null or a number as want and got`)
	}
	return nil
}

func isKey(field string) bool {
	_, ok := rules[field]
	return ok
}

// isScalar reports true, false, null or a number in the canonical form.
func isScalar(raw json.RawMessage) bool {
	return len(raw) > 0 && !strings.ContainsRune(`"[{`, rune(raw[0]))
}
