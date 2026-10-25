package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	queryFile   = "query.json"
	deletedFile = "deleted.json"
	fieldsFile  = "fields.txt"
)

// ReadFields returns the keys of a fields.txt in file order: one key per
// line, "#" comments and empty lines skipped.
func ReadFields(path string) ([]string, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var keys []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys = append(keys, line)
	}
	return keys, nil
}

// A failed file is named by its base name only: the whole path may be the owner's.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file of testdata/golden or a path the caller names
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return nil, fmt.Errorf("golden: %s: %w", filepath.Base(path), err)
	}
	return data, nil
}

func readArray(path, name string) ([]json.RawMessage, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if data = bytes.TrimSpace(data); len(data) == 0 || data[0] != '[' || json.Unmarshal(data, &items) != nil {
		return nil, fmt.Errorf("golden: %s: not one JSON array", name)
	}
	return items, nil
}

// Object is one element of query.json or deleted.json: key → raw JSON value.
type Object map[string]json.RawMessage

// Reference is the osxphotos output for one library.
type Reference struct {
	Library string   // directory under testdata/golden, "synthetic", or "local" (diff-local)
	Query   []Object // query.json, in file order
	Deleted []Object // deleted.json, in file order
}

// Load reads dir/<library>/query.json and deleted.json and keeps only the
// keys of dir/fields.txt. library must be one directory name (not empty, no
// "/", not "." or ".."), checked before any file is opened. Each file must
// hold one JSON array of objects and nothing after it; each object must have
// every key of fields.txt and uuid. The form of uuid is checked by Compare.
// Errors name the file, the element number and the key, never a UUID or a
// value.
func Load(dir, library string) (*Reference, error) {
	if library == "" || library == "." || library == ".." || strings.ContainsAny(library, `/\`) {
		return nil, errors.New("golden: the library must be one directory name")
	}
	fields, err := ReadFields(filepath.Join(dir, fieldsFile))
	if err != nil {
		return nil, err
	}
	return LoadFiles(library, filepath.Join(dir, library, queryFile), filepath.Join(dir, library, deletedFile), fields)
}

// LoadFiles reads the two files from explicit paths (diff-local) and keeps
// only the keys in fields; library names the reference in reports. The
// checks and errors are those of Load.
func LoadFiles(library, queryPath, deletedPath string, fields []string) (*Reference, error) {
	keep := append(slices.Clone(fields), "uuid")
	ref := &Reference{Library: library}
	var err error
	if ref.Query, err = loadObjects(queryPath, queryFile, keep); err != nil {
		return nil, err
	}
	if ref.Deleted, err = loadObjects(deletedPath, deletedFile, keep); err != nil {
		return nil, err
	}
	return ref, nil
}

func loadObjects(path, name string, keep []string) ([]Object, error) {
	items, err := readArray(path, name)
	if err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(items))
	for i, item := range items {
		var full Object
		if json.Unmarshal(item, &full) != nil {
			return nil, fmt.Errorf("golden: %s: element %d is not an object", name, i+1)
		}
		obj := make(Object, len(keep))
		for _, k := range keep {
			v, ok := full[k]
			if !ok {
				return nil, fmt.Errorf("golden: %s: element %d: missing key %s", name, i+1, k)
			}
			obj[k] = v
		}
		out = append(out, obj)
	}
	return out, nil
}

// CheckOrder returns an error naming the file and the element number of the
// first object of Query or Deleted out of the osxphotos order: date, then
// date_added, then uuid, ascending, dates as moments (photoquery.py:888-898).
func (r *Reference) CheckOrder() error {
	for _, part := range []struct {
		file    string
		objects []Object
	}{{queryFile, r.Query}, {deletedFile, r.Deleted}} {
		var prev orderKey
		for i, obj := range part.objects {
			cur, err := orderKeyOf(obj)
			if err != nil {
				return fmt.Errorf("golden: %s: element %d: %w", part.file, i+1, err)
			}
			if i > 0 && cur.less(prev) {
				return fmt.Errorf("golden: %s: element %d is out of order (date, date_added, uuid)", part.file, i+1)
			}
			prev = cur
		}
	}
	return nil
}

// orderKey is the sort key of photoquery.py:888-898.
type orderKey struct {
	date, added time.Time
	uuid        string
}

func (a orderKey) less(b orderKey) bool {
	if c := a.date.Compare(b.date); c != 0 {
		return c < 0
	}
	if c := a.added.Compare(b.added); c != 0 {
		return c < 0
	}
	return a.uuid < b.uuid
}

// A null date_added is 1970-01-01T00:00:00Z, as in the sort key of upstream.
func orderKeyOf(obj Object) (orderKey, error) {
	date, err := refTime(obj, "date", false)
	if err != nil {
		return orderKey{}, err
	}
	added, err := refTime(obj, "date_added", true)
	if err != nil {
		return orderKey{}, err
	}
	if added == nil {
		added = &defaultDateAdded
	}
	uuid, err := decode[string](obj, "uuid", false, "a string")
	if err != nil {
		return orderKey{}, err
	}
	return orderKey{date: *date, added: *added, uuid: uuid}, nil
}
