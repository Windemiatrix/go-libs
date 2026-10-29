package testlib

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"regexp"
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

func templateObjects(prefix string, n int) string {
	var items []string
	for i := 1; i <= n; i++ {
		items = append(items, fmt.Sprintf(`{"uuid": "%s-%d", "t001": "x", "t002": ["a", "b"]}`, prefix, i))
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func validGolden() fstest.MapFS {
	m := fstest.MapFS{
		"fields.txt":       file("uuid\n"),
		"NORMALIZATION.md": file("# Normalization\n"),
		"divergences.json": file("[]"),
		"templates.txt":    file("# t001 first\n{title}\n\n# t002 second\n{keyword}\n"),
		"parse.txt":        file("{title}\n{title\n"),
		"parse.json":       file(`{"p001": {"accept": true, "fields": ["title"]}, "p002": {"accept": false}}`),
	}
	for _, dir := range goldenLibs {
		m[dir+"/query.json"] = file(objects(dir+"-q", 2))
		m[dir+"/deleted.json"] = file(objects(dir+"-d", 1))
		m[dir+"/templates.json"] = file(templateObjects(dir+"-q", 2))
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
		{"templates.json value is a number", func(m fstest.MapFS) {
			m["synthetic/templates.json"] = file(`[{"uuid": "a", "t001": 5, "t002": "x"}]`)
		}, "synthetic/templates.json: element 1, t001"},
		{"templates.json list holds a number", func(m fstest.MapFS) {
			m["synthetic/templates.json"] = file(`[{"uuid": "a", "t001": "x", "t002": ["a", 1]}]`)
		}, "synthetic/templates.json: element 1, t002"},
		{"templates.json missing t002 in one object", func(m fstest.MapFS) {
			m["synthetic/templates.json"] = file(`[{"uuid": "a", "t001": "x", "t002": "y"}, {"uuid": "b", "t001": "x"}]`)
		}, "synthetic/templates.json: element 2: missing key t002"},
		{"templates.json has an unknown key", func(m fstest.MapFS) {
			m["synthetic/templates.json"] = file(`[{"uuid": "a", "t001": "x", "t002": "y", "t003": "z"}]`)
		}, "synthetic/templates.json: element 1: keys besides"},
		{"path in a templates.json value", func(m fstest.MapFS) {
			m["synthetic/templates.json"] = file(`[{"uuid": "a", "t001": "/Users/x/a.jpg", "t002": "y"}]`)
		}, "synthetic/templates.json: 1 values are paths"},
		{"parse.json is not an object", func(m fstest.MapFS) { m["parse.json"] = file("[]") }, "parse.json: is not a JSON object"},
		{"parse.json missing a line", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true, "fields": ["title"]}}`)
		}, "parse.json: p002 is missing"},
		{"parse.json has a key of no line", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true, "fields": ["title"]}, "p002": {"accept": false}, "p003": {"accept": false}}`)
		}, "parse.json: 1 keys"},
		{"parse.json accept is not bool", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true, "fields": ["title"]}, "p002": {"accept": "no"}}`)
		}, "parse.json: p002: accept is not a bool"},
		{"parse.json accepted line without fields", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true}, "p002": {"accept": false}}`)
		}, "parse.json: p001: an accepted line"},
		{"parse.json fields hold a number", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true, "fields": ["title", 1]}, "p002": {"accept": false}}`)
		}, "parse.json: p001: an accepted line"},
		{"parse.json fields with accept false", func(m fstest.MapFS) {
			m["parse.json"] = file(`{"p001": {"accept": true, "fields": []}, "p002": {"accept": false, "fields": []}}`)
		}, "parse.json: p002: a rejected line"},
		{"comment id out of order", func(m fstest.MapFS) {
			m["templates.txt"] = file("# t002 first\n{title}\n# t002 second\n{keyword}\n")
		}, "templates.txt:1"},
		{"template line starts with #", func(m fstest.MapFS) {
			m["templates.txt"] = file("# t001 first\n#{title}\n# t002 second\n{keyword}\n")
		}, "templates.txt:2"},
		{"template without a comment", func(m fstest.MapFS) {
			m["templates.txt"] = file("# t001 first\n{title}\n{album}\n# t002 second\n{keyword}\n")
		}, "templates.txt:3"},
		{"comment without a template", func(m fstest.MapFS) {
			m["templates.txt"] = file("# t001 first\n{title}\n# t002 second\n")
		}, "templates.txt: the last id comment"},
		{"forbidden substring in templates.txt", func(m fstest.MapFS) {
			m["templates.txt"] = file("# t001 first\n{function:https://example.com/a.py::f}\n# t002 second\n{keyword}\n")
		}, "templates.txt:2"},
		{"forbidden substring in a comment of templates.txt", func(m fstest.MapFS) {
			m["templates.txt"] = file("# {exiftool}\n# t001 first\n{title}\n# t002 second\n{keyword}\n")
		}, "templates.txt:1"},
		{"forbidden substring in parse.txt", func(m fstest.MapFS) { m["parse.txt"] = file("{exiftool}\n{title\n") }, "parse.txt:1"},
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
	for name, content := range map[string]string{
		"Test-13.0.0/query.json":     `[{"uuid": "a", "title": "/Users/jane.doe/Pictures/secret.jpg"}]`,
		"Test-13.0.0/templates.json": `[{"uuid": "a", "t001": "/Users/jane.doe/Pictures/secret.jpg", "t002": "x"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := validGolden()
			m[name] = file(content)
			errs := checkGolden(m, goldenLibs)
			if len(errs) != 1 {
				t.Fatalf("checkGolden = %v, want one error", errs)
			}
			if strings.Contains(errs[0].Error(), "jane.doe") {
				t.Errorf("the error repeats the value: %v", errs[0])
			}
		})
	}
}

func TestCheckGoldenRequiresTemplateFiles(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"templates.txt", "parse.txt", "parse.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := validGolden()
			delete(m, name)
			errs := checkGolden(m, goldenLibs)
			if !slices.ContainsFunc(errs, func(err error) bool {
				return errors.Is(err, fs.ErrNotExist) && strings.Contains(err.Error(), name)
			}) {
				t.Errorf("checkGolden = %v, want an error that %s does not exist", errs, name)
			}
		})
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
	rootFiles    = []string{"fields.txt", "NORMALIZATION.md", "divergences.json", "templates.txt", "parse.txt", "parse.json"}
	dataFiles    = []string{"query.json", "deleted.json", "templates.json"}
	hostPrefixes = []string{"/Users/", "/Volumes/", "/private/", "/var/", "/tmp/", "/home/", "/Library/"}

	// The fields that download code, run a program or call Vision on the
	// runner of golden-regen; scripts/golden-build.sh refuses the same text.
	forbiddenInTemplates = []string{"{function", "|function:", "{exiftool", "{detected_text", "{osxphotos_cmd_line"}
	idComment            = regexp.MustCompile(`^# t(\d+)(?: |$)`)
)

// The messages say where a problem is and never repeat a value of the data.
func checkGolden(fsys fs.FS, libraries []string) []error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return []error{err}
	}
	templates, errs := checkTemplatesTxt(fsys)
	errs = append(errs, checkParse(fsys)...)
	for _, e := range entries {
		switch name := e.Name(); {
		case e.IsDir() && slices.Contains(libraries, name):
			errs = append(errs, checkDataDir(fsys, name, templates)...)
		case !e.Type().IsRegular() || !slices.Contains(rootFiles, name):
			errs = append(errs, fmt.Errorf("%s: unexpected entry; the root allows only the directories of the test libraries and the files %s",
				name, strings.Join(rootFiles, ", ")))
		}
	}
	return errs
}

// checkTemplatesTxt returns the number of templates: the ids t001... come from
// the comment before each template, so a skipped or doubled line shows as a
// comment out of order.
func checkTemplatesTxt(fsys fs.FS) (int, []error) {
	data, err := fs.ReadFile(fsys, "templates.txt")
	if err != nil {
		return 0, []error{err}
	}
	var errs []error
	count, awaiting := 0, false
	for i, line := range strings.Split(string(data), "\n") {
		at := fmt.Sprintf("templates.txt:%d", i+1)
		if hasForbidden(line) {
			errs = append(errs, fmt.Errorf("%s: a field that runs a program or fetches code", at))
		}
		switch {
		case line == "":
		case strings.HasPrefix(line, "#"):
			if awaiting {
				errs = append(errs, fmt.Errorf("%s: an id comment is not followed by a template; a template never starts with #", at))
				awaiting = false
			}
			if m := idComment.FindStringSubmatch(line); m != nil {
				count++
				if m[1] != fmt.Sprintf("%03d", count) {
					errs = append(errs, fmt.Errorf("%s: the comment id is out of order, want t%03d", at, count))
				}
				awaiting = true
			}
		default:
			if !awaiting {
				errs = append(errs, fmt.Errorf("%s: a template without an id comment before it", at))
			}
			awaiting = false
		}
	}
	if awaiting {
		errs = append(errs, errors.New("templates.txt: the last id comment has no template after it"))
	}
	return count, errs
}

func checkParse(fsys fs.FS) []error {
	txt, err := fs.ReadFile(fsys, "parse.txt")
	if err != nil {
		return []error{err}
	}
	var errs []error
	// The file ends with a newline, so the last element is not a line.
	lines := strings.Split(strings.TrimSuffix(string(txt), "\n"), "\n")
	for i, line := range lines {
		if hasForbidden(line) {
			errs = append(errs, fmt.Errorf("parse.txt:%d: a field that runs a program or fetches code", i+1))
		}
	}
	data, err := fs.ReadFile(fsys, "parse.json")
	if err != nil {
		return append(errs, err)
	}
	var oracle map[string]any
	if err := json.Unmarshal(data, &oracle); err != nil {
		return append(errs, fmt.Errorf("parse.json: is not a JSON object: %w", err))
	}
	for i := range lines {
		id := fmt.Sprintf("p%03d", i+1)
		entry, ok := oracle[id]
		delete(oracle, id)
		if !ok {
			errs = append(errs, fmt.Errorf("parse.json: %s is missing", id))
		} else if err := checkParseEntry(entry); err != nil {
			errs = append(errs, fmt.Errorf("parse.json: %s: %w", id, err))
		}
	}
	if len(oracle) > 0 {
		errs = append(errs, fmt.Errorf("parse.json: %d keys do not belong to a line of parse.txt", len(oracle)))
	}
	return errs
}

func checkParseEntry(entry any) error {
	obj, _ := entry.(map[string]any)
	accept, ok := obj["accept"].(bool)
	fields, hasFields := obj["fields"]
	switch {
	case !ok:
		return errors.New("accept is not a bool")
	case accept && !isStrings(fields):
		return errors.New("an accepted line needs fields, a list of strings")
	case !accept && hasFields:
		return errors.New("a rejected line has fields")
	}
	return nil
}

func hasForbidden(line string) bool {
	return slices.ContainsFunc(forbiddenInTemplates, func(s string) bool { return strings.Contains(line, s) })
}

func isStrings(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		if _, isString := item.(string); !isString {
			return false
		}
	}
	return true
}

func checkDataDir(fsys fs.FS, dir string, templates int) []error {
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
		if err := checkDataFile(fsys, p, templates); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkDataFile(fsys fs.FS, p string, templates int) error {
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
			if location, found := findHostPath(el[k], k); found {
				if hostCount == 0 {
					hostFirst = fmt.Sprintf("element %d, %s", i+1, location)
				}
				hostCount++
			}
		}
	}
	var errs []error
	if hostCount > 0 {
		errs = append(errs, fmt.Errorf("%s: %d values are paths of a machine or of a library package, the first at %s", p, hostCount, hostFirst))
	}
	if strings.HasSuffix(p, "/templates.json") {
		if err := checkTemplateValues(elems, templates); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}

// A template that renders to nothing or to several values is an empty or a
// longer list (print_photo_info.py:138-149); every other result is a string.
func checkTemplateValues(elems []map[string]any, templates int) error {
	for i, el := range elems {
		for k := 1; k <= templates; k++ {
			id := fmt.Sprintf("t%03d", k)
			v, ok := el[id]
			if !ok {
				return fmt.Errorf("element %d: missing key %s", i+1, id)
			}
			if _, isString := v.(string); !isString && !isStrings(v) {
				return fmt.Errorf("element %d, %s: the value is neither a string nor a list of strings", i+1, id)
			}
		}
		if len(el) != templates+1 {
			return fmt.Errorf("element %d: keys besides uuid and the %d template ids", i+1, templates)
		}
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

func findHostPath(v any, location string) (string, bool) {
	switch v := v.(type) {
	case string:
		if strings.Contains(v, ".photoslibrary") {
			return location, true
		}
		for _, prefix := range hostPrefixes {
			if strings.HasPrefix(v, prefix) {
				return location, true
			}
		}
	case []any:
		for i, item := range v {
			if hostLocation, found := findHostPath(item, fmt.Sprintf("%s[%d]", location, i)); found {
				return hostLocation, true
			}
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			if hostLocation, found := findHostPath(v[k], location+"."+k); found {
				return hostLocation, true
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
