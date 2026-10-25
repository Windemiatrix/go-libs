package testlib

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

var goldenLibs = []string{"Test-13.0.0", "synthetic"}

func objects(prefix string, n int) string {
	var items []string
	for i := 1; i <= n; i++ {
		items = append(items, fmt.Sprintf(`{"uuid": "%s-%d", "title": "t", "album_info": [{"title": "Album"}]}`, prefix, i))
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func validGolden() fstest.MapFS {
	m := fstest.MapFS{
		"fields.txt":       file("uuid\n"),
		"NORMALIZATION.md": file("# Normalization\n"),
		"divergences.json": file("[]"),
	}
	for _, dir := range goldenLibs {
		m[dir+"/query.json"] = file(objects(dir+"-q", 2))
		m[dir+"/deleted.json"] = file(objects(dir+"-d", 1))
	}
	return m
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestCheckGolden(t *testing.T) {
	t.Parallel()

	if errs := checkGolden(validGolden(), goldenLibs); len(errs) != 0 {
		t.Errorf("checkGolden of a valid tree = %v, want no errors", errs)
	}

	cases := []struct {
		name   string
		mutate func(m fstest.MapFS)
		want   string
	}{
		{"unknown library", func(m fstest.MapFS) { m["Owner/query.json"] = file(objects("o", 1)) }, "Owner"},
		{"unexpected file in a library", func(m fstest.MapFS) { m["Test-13.0.0/raw.json"] = file("[]") }, "raw.json"},
		{"photoslibrary directory", func(m fstest.MapFS) { m["Owner.photoslibrary/database/Photos.sqlite"] = file("x") }, "Owner.photoslibrary"},
		{"database in the root", func(m fstest.MapFS) { m["Photos.sqlite"] = file("SQLite format 3") }, "Photos.sqlite"},
		{"report in the root", func(m fstest.MapFS) { m["owner-report.md"] = file("# report") }, "owner-report.md"},
		{"symbolic link in a library", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = &fstest.MapFile{Data: []byte("/etc/passwd"), Mode: fs.ModeSymlink}
		}, "Test-13.0.0/query.json"},
		{"invalid JSON", func(m fstest.MapFS) { m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"`) }, "JSON"},
		{"element without uuid", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"}, {"title": "t"}]`)
		}, "element 2"},
		{"repeated uuid in a file", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"}, {"uuid": "a"}]`)
		}, "duplicate"},
		{"path in album_info", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "album_info": [{"title": "t", "path": "/Users/x/a.jpg"}]}]`)
		}, "album_info[0].path"},
		{"photoslibrary in a value", func(m fstest.MapFS) {
			m["synthetic/deleted.json"] = file(`[{"uuid": "a", "title": "Photos Library.photoslibrary"}]`)
		}, "title"},
		{"path in a list of strings", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "album_info": [["ok", "/var/folders/x"]]}]`)
		}, "album_info[0][1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := validGolden()
			c.mutate(m)
			errs := checkGolden(m, goldenLibs)
			if len(errs) != 1 {
				t.Fatalf("checkGolden = %v, want exactly one error containing %q", errs, c.want)
			}
			if !strings.Contains(errs[0].Error(), c.want) {
				t.Errorf("error %q does not contain %q", errs[0], c.want)
			}
		})
	}
}

// Test output must not carry the data of a library, so an error about a path
// tells where it is, not what it is.
func TestCheckGoldenKeepsValuesOut(t *testing.T) {
	t.Parallel()
	m := validGolden()
	m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/Users/jane.doe/Pictures/secret.jpg"}]`)
	errs := checkGolden(m, goldenLibs)
	if len(errs) != 1 {
		t.Fatalf("checkGolden = %v, want one error", errs)
	}
	if strings.Contains(errs[0].Error(), "jane.doe") {
		t.Errorf("the error repeats the value: %v", errs[0])
	}
}

func TestCheckGoldenUUIDs(t *testing.T) {
	t.Parallel()
	known := map[string]map[string]bool{
		"Test-13.0.0": {"Test-13.0.0-q-1": true, "Test-13.0.0-q-2": true, "Test-13.0.0-d-1": true},
		"synthetic":   {"synthetic-q-1": true, "synthetic-q-2": true, "synthetic-d-1": true},
	}
	if errs := checkUUIDs(validGolden(), known); len(errs) != 0 {
		t.Errorf("checkUUIDs of known uuids = %v, want no errors", errs)
	}
	m := validGolden()
	m["Test-13.0.0/query.json"] = file(`[{"uuid": "Test-13.0.0-q-1"}, {"uuid": "FROM-THE-OWNER"}]`)
	errs := checkUUIDs(m, known)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Test-13.0.0/query.json") {
		t.Fatalf("checkUUIDs = %v, want one error about Test-13.0.0/query.json", errs)
	}
	if strings.Contains(errs[0].Error(), "FROM-THE-OWNER") {
		t.Errorf("the error repeats the uuid: %v", errs[0])
	}
}

func TestGoldenLayout(t *testing.T) {
	libs := Libraries(t) // skips the test without testdata, as in a module zip
	fsys := os.DirFS(RepoDir(t, "golden"))
	for _, err := range checkGolden(fsys, libs) {
		t.Error(err)
	}
	known := map[string]map[string]bool{}
	for _, name := range libs {
		known[name] = libraryUUIDs(t, name)
	}
	for _, err := range checkUUIDs(fsys, known) {
		t.Error(err)
	}
}

// TestGoldenUUIDSources: the sets that TestGoldenLayout compares with the
// reference data are not empty (it would reject every uuid) and come from the
// same databases that golden-regen reads.
func TestGoldenUUIDSources(t *testing.T) {
	libs := Libraries(t)
	if !slices.Contains(libs, "synthetic") {
		t.Fatalf("libraries %q, want the synthetic one among them", libs)
	}
	for _, name := range libs {
		if got := libraryUUIDs(t, name); len(got) == 0 {
			t.Errorf("%s: no ZASSET.ZUUID values", name)
		}
	}
	if got := len(libraryUUIDs(t, "synthetic")); got != 1000 {
		t.Errorf("synthetic library: %d uuid values, want 1000", got)
	}
}

func libraryUUIDs(t *testing.T, name string) map[string]bool {
	t.Helper()
	// Read only, as AGENTS.md requires for a copy of Photos.sqlite: closing a
	// read-write connection could checkpoint the WAL into the copy.
	u := url.URL{Scheme: "file", Path: dbFile(OpenPristine(t, name)), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	out := map[string]bool{}
	for _, id := range Rows(t, db, "SELECT ZUUID FROM ZASSET WHERE ZUUID IS NOT NULL") {
		out[id] = true
	}
	return out
}

var (
	rootFiles    = []string{"fields.txt", "NORMALIZATION.md", "divergences.json"}
	dataFiles    = []string{"query.json", "deleted.json"}
	hostPrefixes = []string{"/Users/", "/Volumes/", "/private/", "/var/", "/tmp/", "/home/", "/Library/"}
)

// The messages say where a problem is and never repeat a value of the data.
func checkGolden(fsys fs.FS, libraries []string) []error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, e := range entries {
		switch name := e.Name(); {
		case e.IsDir() && slices.Contains(libraries, name):
			errs = append(errs, checkDataDir(fsys, name)...)
		case !e.Type().IsRegular() || !slices.Contains(rootFiles, name):
			errs = append(errs, fmt.Errorf("%s: unexpected entry; the root allows only the directories of the test libraries and the files %s",
				name, strings.Join(rootFiles, ", ")))
		}
	}
	return errs
}

func checkDataDir(fsys fs.FS, dir string) []error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, e := range entries {
		p := dir + "/" + e.Name()
		if !e.Type().IsRegular() || !slices.Contains(dataFiles, e.Name()) {
			errs = append(errs, fmt.Errorf("%s: unexpected entry; a library allows only the files %s", p, strings.Join(dataFiles, ", ")))
			continue
		}
		if err := checkDataFile(fsys, p); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkDataFile(fsys fs.FS, p string) error {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return err
	}
	elems, err := parseObjects(data)
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	hostCount, hostFirst := 0, ""
	for i, el := range elems {
		for _, k := range slices.Sorted(maps.Keys(el)) {
			if where, found := findHostPath(el[k], k); found {
				if hostCount == 0 {
					hostFirst = fmt.Sprintf("element %d, %s", i+1, where)
				}
				hostCount++
			}
		}
	}
	if hostCount > 0 {
		return fmt.Errorf("%s: %d values are paths of a machine or of a library package, the first at %s", p, hostCount, hostFirst)
	}
	return nil
}

func parseObjects(data []byte) ([]map[string]any, error) {
	var list []any
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("is not a JSON array: %w", err)
	}
	out := make([]map[string]any, len(list))
	seen := map[string]bool{}
	for i, v := range list {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("element %d is not an object", i+1)
		}
		id, ok := obj["uuid"].(string)
		switch {
		case !ok || id == "":
			return nil, fmt.Errorf("element %d has no uuid string", i+1)
		case seen[id]:
			return nil, fmt.Errorf("element %d: duplicate uuid", i+1)
		}
		seen[id] = true
		out[i] = obj
	}
	return out, nil
}

func findHostPath(v any, where string) (string, bool) {
	switch v := v.(type) {
	case string:
		if strings.Contains(v, ".photoslibrary") {
			return where, true
		}
		for _, prefix := range hostPrefixes {
			if strings.HasPrefix(v, prefix) {
				return where, true
			}
		}
	case []any:
		for i, item := range v {
			if w, found := findHostPath(item, fmt.Sprintf("%s[%d]", where, i)); found {
				return w, true
			}
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			if w, found := findHostPath(v[k], where+"."+k); found {
				return w, true
			}
		}
	}
	return "", false
}

// A file that cannot be read or parsed is left to checkGolden.
func checkUUIDs(fsys fs.FS, known map[string]map[string]bool) []error {
	var errs []error
	for _, dir := range slices.Sorted(maps.Keys(known)) {
		for _, name := range dataFiles {
			p := dir + "/" + name
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				continue
			}
			elems, err := parseObjects(data)
			if err != nil {
				continue
			}
			unknown := 0
			for _, el := range elems {
				if !known[dir][el["uuid"].(string)] {
					unknown++
				}
			}
			if unknown > 0 {
				errs = append(errs, fmt.Errorf("%s: %d uuid values are not in the database of the library", p, unknown))
			}
		}
	}
	return errs
}
