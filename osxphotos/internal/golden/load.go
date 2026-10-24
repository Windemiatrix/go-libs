package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	queryFile   = "query.json"
	deletedFile = "deleted.json"
	fieldsFile  = "fields.txt"
)

// Dir returns testdata/golden of the repository, found relative to this
// source file (as internal/testlib finds testdata/libraries). It skips t
// when the directory is not available, for example in the module zip.
func Dir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("golden: cannot locate the package source")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "golden")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("golden: reference data is not available (%v); it is in the repository, not in the module zip", err)
	}
	return dir
}

// ReadFields returns the keys of a fields.txt in file order: one key per
// line, "#" comments and empty lines skipped. The format itself is checked
// by the layout test of internal/testlib.
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

// readFile reads a file and, on failure, names it by its base name and keeps
// the cause (not the whole path, which may be the owner's).
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
// every key of fields.txt and a non-empty string uuid; a uuid may appear only
// once in query.json and deleted.json together. Errors name the file, the
// element number and the key, never a UUID or a value.
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
	seen := map[string]string{} // uuid → "<file> element <n>"
	if ref.Query, err = loadObjects(queryPath, queryFile, keep, seen); err != nil {
		return nil, err
	}
	if ref.Deleted, err = loadObjects(deletedPath, deletedFile, keep, seen); err != nil {
		return nil, err
	}
	return ref, nil
}

// loadObjects reads one file. name is how the file is called in errors.
func loadObjects(path, name string, keep []string, seen map[string]string) ([]Object, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
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
	out := make([]Object, 0, len(items))
	for i, item := range items {
		n := i + 1
		full, ok := decodeObject(item)
		if !ok {
			return nil, fmt.Errorf("golden: %s: element %d is not an object", name, n)
		}
		obj := make(Object, len(keep))
		for _, k := range keep {
			v, ok := full[k]
			if !ok {
				return nil, fmt.Errorf("golden: %s: element %d: missing key %s", name, n, k)
			}
			obj[k] = v
		}
		id, ok := decodeString(obj["uuid"])
		if !ok || id == "" {
			return nil, fmt.Errorf("golden: %s: element %d: uuid is not a non-empty string", name, n)
		}
		here := fmt.Sprintf("%s element %d", name, n)
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("golden: %s: element %d: the uuid repeats that of %s", name, n, prev)
		}
		seen[id] = here
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

func orderKeyOf(obj Object) (orderKey, error) {
	var k orderKey
	var err error
	if k.date, err = momentOf(obj, "date", false); err != nil {
		return k, err
	}
	if k.added, err = momentOf(obj, "date_added", true); err != nil {
		return k, err
	}
	if k.uuid, err = refString(obj, "uuid", false); err != nil {
		return k, err
	}
	return k, nil
}

// momentOf reads a date key as a time; null is 1970-01-01T00:00:00Z when
// nullOK, as the sort key of upstream does.
func momentOf(obj Object, key string, nullOK bool) (time.Time, error) {
	raw, err := field(obj, key)
	if err != nil {
		return time.Time{}, err
	}
	if nullOK && isNull(raw) {
		return defaultDateAdded, nil
	}
	return parseRefMoment(raw, key)
}
