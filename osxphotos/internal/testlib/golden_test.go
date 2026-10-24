package testlib

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/gen"
)

// goldenLibs are the libraries of the trees built by validGolden.
var goldenLibs = []string{"Test-13.0.0", "Test-14.6.0"}

// goldenFields is a valid fields.txt. The objects of validGolden use only
// album_info, title and uuid, so a case may break the line of another key.
const goldenFields = "# Keys kept in testdata/golden.\n# Second comment line.\n\nalbum_info\ndate\ntitle\nuuid\n"

// objects renders a JSON array of objects with the uuids prefix-1, prefix-2, ...
func objects(prefix string, n int) string {
	if n == 0 {
		return "[]"
	}
	var items []string
	for i := 1; i <= n; i++ {
		id := prefix + "-" + string(rune('0'+i))
		items = append(items, `{"uuid": "`+id+`", "title": "t", "album_info": [{"title": "Album"}]}`)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

// validGolden is a complete tree: fields.txt and, for every library and for
// synthetic, a non-empty query.json and a non-empty deleted.json.
func validGolden() fstest.MapFS {
	m := fstest.MapFS{"fields.txt": {Data: []byte(goldenFields)}}
	for _, dir := range append(slices.Clone(goldenLibs), syntheticDir) {
		m[dir+"/query.json"] = &fstest.MapFile{Data: []byte(objects(dir+"-q", 2))}
		m[dir+"/deleted.json"] = &fstest.MapFile{Data: []byte(objects(dir+"-d", 1))}
	}
	return m
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestCheckGolden(t *testing.T) {
	t.Parallel()

	valid := map[string]func(m fstest.MapFS){
		"complete tree": func(fstest.MapFS) {},
		"fields.txt only": func(m fstest.MapFS) {
			for name := range m {
				if name != "fields.txt" {
					delete(m, name)
				}
			}
		},
		"fields.txt and the optional files": func(m fstest.MapFS) {
			for name := range m {
				if name != "fields.txt" {
					delete(m, name)
				}
			}
			m["NORMALIZATION.md"] = file("# Normalization\n")
			m["divergences.json"] = file("[]")
		},
		"deleted.json of a library may be empty": func(m fstest.MapFS) {
			m["Test-13.0.0/deleted.json"] = file("[]")
		},
		"a file of exactly the size limit": func(m fstest.MapFS) {
			m["Test-13.0.0/deleted.json"] = file("[]" + strings.Repeat(" ", 8<<20-2))
		},
	}
	for name, mutate := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			t.Parallel()
			m := validGolden()
			mutate(m)
			if errs := checkGolden(m, goldenLibs); len(errs) != 0 {
				t.Errorf("checkGolden = %v, want no errors", errs)
			}
		})
	}

	// Every case breaks exactly one rule and must give exactly one error.
	cases := []struct {
		name   string
		mutate func(m fstest.MapFS)
		want   string // a part of the only error
	}{
		// The root.
		{"unknown library", func(m fstest.MapFS) { m["Owner/query.json"] = file(objects("o", 1)) }, "Owner"},
		{"library not in the list", func(m fstest.MapFS) { m["Test-13.0.0/raw.json"] = file("[]") }, "raw.json"},
		{"photoslibrary directory", func(m fstest.MapFS) { m["Owner.photoslibrary/database/Photos.sqlite"] = file("x") }, "Owner.photoslibrary"},
		{"nested directory", func(m fstest.MapFS) { m["Test-13.0.0/sub/query.json"] = file(objects("s", 1)) }, "sub"},
		{"database in the root", func(m fstest.MapFS) { m["Photos.sqlite"] = file("SQLite format 3") }, "Photos.sqlite"},
		{"report in the root", func(m fstest.MapFS) { m["owner-report.md"] = file("# report") }, "owner-report.md"},
		{"YAML divergences", func(m fstest.MapFS) { m["divergences.yaml"] = file("[]") }, "divergences.yaml"},
		{"Finder file", func(m fstest.MapFS) { m[".DS_Store"] = file("x") }, ".DS_Store"},
		{"symbolic link in a library", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = &fstest.MapFile{Data: []byte("/etc/passwd"), Mode: fs.ModeSymlink}
		}, "symbolic link"},
		{"symbolic link in the root", func(m fstest.MapFS) {
			m["NORMALIZATION.md"] = &fstest.MapFile{Data: []byte("fields.txt"), Mode: fs.ModeSymlink}
		}, "symbolic link"},
		{"symbolic link instead of a library directory", func(m fstest.MapFS) {
			delete(m, "Test-13.0.0/query.json")
			delete(m, "Test-13.0.0/deleted.json")
			m["Test-13.0.0"] = &fstest.MapFile{Data: []byte("Test-14.6.0"), Mode: fs.ModeSymlink}
		}, "must be a directory, found a symbolic link"},
		{"file instead of a library directory", func(m fstest.MapFS) {
			delete(m, "Test-14.6.0/query.json")
			delete(m, "Test-14.6.0/deleted.json")
			m["Test-14.6.0"] = file("x")
		}, "must be a directory, found a file"},
		{"large root file", func(m fstest.MapFS) {
			m["NORMALIZATION.md"] = file(strings.Repeat("x", 1<<20+1))
		}, "NORMALIZATION.md"},
		{"large data file", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(strings.Repeat(" ", 8<<20+1))
		}, "larger"},

		// fields.txt.
		{"no fields.txt", func(m fstest.MapFS) { delete(m, "fields.txt") }, "fields.txt: missing"},
		{"fields.txt with a bad key", func(m fstest.MapFS) {
			m["fields.txt"] = file("Bad-Key\n" + goldenFields)
		}, "Bad-Key"},
		{"fields.txt with a repeated key", func(m fstest.MapFS) {
			m["fields.txt"] = file(goldenFields + "uuid\n")
		}, "duplicate"},
		{"fields.txt with a carriage return", func(m fstest.MapFS) {
			m["fields.txt"] = file(strings.Replace(goldenFields, "date\n", "date\r\n", 1))
		}, "line 5"},
		{"fields.txt without uuid", func(m fstest.MapFS) {
			m["fields.txt"] = file(strings.Replace(goldenFields, "uuid\n", "", 1))
		}, "uuid"},
		{"fields.txt out of order", func(m fstest.MapFS) {
			m["fields.txt"] = file("# c\ntitle\ndate\nalbum_info\nuuid\n")
		}, "order"},
		{"fields.txt without keys", func(m fstest.MapFS) {
			m["fields.txt"] = file("# c\n\n")
		}, "no keys"},
		{"fields.txt with a space", func(m fstest.MapFS) {
			m["fields.txt"] = file(strings.Replace(goldenFields, "date\n", "date \n", 1))
		}, "line 5"},

		// The content of a data file.
		{"empty file", func(m fstest.MapFS) { m["Test-13.0.0/query.json"] = file("") }, "empty"},
		{"invalid JSON", func(m fstest.MapFS) { m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"`) }, "JSON"},
		{"object instead of an array", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`{"uuid": "a"}`)
		}, "array"},
		{"element is not an object", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"}, "b"]`)
		}, "element 2"},
		{"element without uuid", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"}, {"title": "t"}]`)
		}, "uuid"},
		{"empty uuid", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": ""}]`)
		}, "uuid"},
		{"uuid is not a string", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": 7}]`)
		}, "uuid"},
		{"repeated uuid in a file", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a"}, {"uuid": "a"}]`)
		}, "duplicate"},
		{"key that is not in fields.txt", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "library": "x"}]`)
		}, "library"},
		{"query.json is empty", func(m fstest.MapFS) { m["Test-13.0.0/query.json"] = file("[]") }, "no objects"},
		{"query.json and deleted.json share a uuid", func(m fstest.MapFS) {
			m["Test-13.0.0/deleted.json"] = file(`[{"uuid": "Test-13.0.0-q-1"}]`)
		}, "both"},
		{"synthetic without trashed objects", func(m fstest.MapFS) { m["synthetic/deleted.json"] = file("[]") }, "no objects"},

		// Paths inside values, at any depth.
		{"path in album_info", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "album_info": [{"title": "t", "path": "/Users/x/a.jpg"}]}]`)
		}, "album_info[0].path"},
		{"photoslibrary in a value", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "Photos Library.photoslibrary"}]`)
		}, "title"},
		{"Volumes path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/Volumes/Disk/a.jpg"}]`)
		}, "title"},
		{"private path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/private/var/a"}]`)
		}, "title"},
		{"var path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/var/folders/a"}]`)
		}, "title"},
		{"tmp path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/tmp/a"}]`)
		}, "title"},
		{"home path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/home/runner/a"}]`)
		}, "title"},
		{"Library path", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "title": "/Library/a"}]`)
		}, "title"},
		{"path in a list of strings", func(m fstest.MapFS) {
			m["Test-13.0.0/query.json"] = file(`[{"uuid": "a", "album_info": [["ok", "/Users/x"]]}]`)
		}, "album_info[0][1]"},

		// All or nothing.
		{"a library without a directory", func(m fstest.MapFS) {
			delete(m, "Test-14.6.0/query.json")
			delete(m, "Test-14.6.0/deleted.json")
		}, "Test-14.6.0: missing"},
		{"synthetic without a directory", func(m fstest.MapFS) {
			delete(m, "synthetic/query.json")
			delete(m, "synthetic/deleted.json")
		}, "synthetic: missing"},
		{"library without deleted.json", func(m fstest.MapFS) { delete(m, "Test-13.0.0/deleted.json") }, "deleted.json: missing"},
		{"library without query.json", func(m fstest.MapFS) { delete(m, "Test-13.0.0/query.json") }, "query.json: missing"},
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

// TestCheckGoldenKeepsValuesOut: an error about a path tells where it is, not
// what it is; test output must not carry the data of a library.
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
	t.Run("every uuid is known", func(t *testing.T) {
		t.Parallel()
		if errs := checkUUIDs(validGolden(), known); len(errs) != 0 {
			t.Errorf("checkUUIDs = %v, want no errors", errs)
		}
	})
	t.Run("a library that is not in known is not checked", func(t *testing.T) {
		t.Parallel()
		if errs := checkUUIDs(validGolden(), nil); len(errs) != 0 {
			t.Errorf("checkUUIDs = %v, want no errors", errs)
		}
	})
	t.Run("a uuid of another library", func(t *testing.T) {
		t.Parallel()
		m := validGolden()
		m["Test-13.0.0/query.json"] = file(`[{"uuid": "Test-13.0.0-q-1"}, {"uuid": "FROM-THE-OWNER"}]`)
		errs := checkUUIDs(m, known)
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Test-13.0.0/query.json") {
			t.Fatalf("checkUUIDs = %v, want one error about Test-13.0.0/query.json", errs)
		}
		if strings.Contains(errs[0].Error(), "FROM-THE-OWNER") {
			t.Errorf("the error repeats the uuid: %v", errs[0])
		}
	})
	t.Run("a uuid of synthetic", func(t *testing.T) {
		t.Parallel()
		m := validGolden()
		m["synthetic/deleted.json"] = file(`[{"uuid": "FROM-THE-OWNER"}]`)
		errs := checkUUIDs(m, known)
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "synthetic/deleted.json") {
			t.Fatalf("checkUUIDs = %v, want one error about synthetic/deleted.json", errs)
		}
	})
	t.Run("a file that checkGolden reports is skipped", func(t *testing.T) {
		t.Parallel()
		m := validGolden()
		m["Test-13.0.0/query.json"] = file("not JSON")
		if errs := checkUUIDs(m, known); len(errs) != 0 {
			t.Errorf("checkUUIDs = %v, want no errors", errs)
		}
	})
}

// TestGoldenLayout checks testdata/golden of the repository. Until the
// reference data is committed it holds fields.txt only.
func TestGoldenLayout(t *testing.T) {
	libs := Libraries(t) // skips the test without testdata, as in a module zip
	dir := filepath.Join(librariesDir(t), "..", "golden")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata/golden is missing: %v", err)
	}
	fsys := os.DirFS(dir)
	for _, err := range checkGolden(fsys, libs) {
		t.Error(err)
	}

	known := map[string]map[string]bool{}
	for _, name := range append(slices.Clone(libs), syntheticDir) {
		if info, err := fs.Stat(fsys, name); err != nil || !info.IsDir() {
			continue
		}
		if name == syntheticDir {
			known[name] = syntheticUUIDs(t)
		} else {
			known[name] = libraryUUIDs(t, name)
		}
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
	if len(libs) == 0 {
		t.Fatal("no test libraries")
	}
	for _, name := range libs {
		if got := libraryUUIDs(t, name); len(got) == 0 {
			t.Errorf("%s: no ZASSET.ZUUID values", name)
		}
	}
	if got, want := len(syntheticUUIDs(t)), gen.DefaultConfig().Assets; got != want {
		t.Errorf("synthetic library: %d uuid values, want %d", got, want)
	}
}

// libraryUUIDs returns ZASSET.ZUUID of the named test library (without migrations).
func libraryUUIDs(t *testing.T, name string) map[string]bool {
	t.Helper()
	return assetUUIDs(t, filepath.Join(OpenPristine(t, name), "database", "Photos.sqlite"))
}

// syntheticUUIDs returns ZASSET.ZUUID of the library of gen.DefaultConfig.
func syntheticUUIDs(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Synthetic.photoslibrary")
	if err := gen.Generate(t.Context(), dir, gen.DefaultConfig()); err != nil {
		t.Fatalf("gen.Generate: %v", err)
	}
	return assetUUIDs(t, filepath.Join(dir, "database", "Photos.sqlite"))
}

func assetUUIDs(t *testing.T, dbPath string) map[string]bool {
	t.Helper()
	// Read only, as AGENTS.md requires for a copy of Photos.sqlite: closing a
	// read-write connection could checkpoint the WAL into the copy.
	u := url.URL{Scheme: "file", Path: dbPath, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(), "SELECT ZUUID FROM ZASSET")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var id sql.NullString
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id.Valid {
			out[id.String] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

const (
	// syntheticDir holds the reference data of the library of gen.DefaultConfig.
	syntheticDir = "synthetic"

	maxRootFile int64 = 1 << 20 // fields.txt, NORMALIZATION.md, divergences.json
	maxDataFile int64 = 8 << 20 // query.json, deleted.json
)

var (
	// rootFiles are the only files allowed in the root of testdata/golden.
	rootFiles = []string{"fields.txt", "NORMALIZATION.md", "divergences.json"}
	// dataFiles are the only files allowed in the directory of a library.
	dataFiles = []string{"query.json", "deleted.json"}
	// hostPrefixes start a string that is a path of the machine that ran osxphotos.
	hostPrefixes = []string{"/Users/", "/Volumes/", "/private/", "/var/", "/tmp/", "/home/", "/Library/"}

	fieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// checkGolden returns the problems of a testdata/golden tree: fsys is rooted
// at testdata/golden, libraries are the names of testdata/libraries.
//
// Each problem is reported once and its consequences are not: an entry that
// cannot be read is not parsed, an unknown directory is not entered. The
// messages say where the problem is and never repeat a value of the data.
func checkGolden(fsys fs.FS, libraries []string) []error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return []error{err}
	}
	var errs []error

	keys, fieldsErrs := readFields(fsys, entries)
	errs = append(errs, fieldsErrs...)

	expected := append(slices.Clone(libraries), syntheticDir)
	seen := map[string]bool{} // directories with the name of a library, of any kind
	hasData := false
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == "fields.txt":
			// Read above.
		case slices.Contains(expected, name):
			seen[name] = true
			if !e.IsDir() {
				errs = append(errs, fmt.Errorf("%s: must be a directory, found a %s", name, kind(e)))
				continue
			}
			hasData = true
			errs = append(errs, checkDataDir(fsys, name, keys)...)
		case e.IsDir():
			errs = append(errs, fmt.Errorf("%s: unexpected directory; only the test libraries and %s are allowed", name, syntheticDir))
		case !slices.Contains(rootFiles, name):
			errs = append(errs, fmt.Errorf("%s: unexpected %s; the root allows only %s", name, kind(e), strings.Join(rootFiles, ", ")))
		default:
			if err := checkRegular(e, maxRootFile); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}
	// All or nothing: with any data there is data of every library and of synthetic.
	if hasData {
		for _, name := range expected {
			if !seen[name] {
				errs = append(errs, fmt.Errorf("%s: missing; the reference data is all or nothing", name))
			}
		}
	}
	return errs
}

// kind names the type of a directory entry for a message.
func kind(e fs.DirEntry) string {
	switch {
	case e.Type()&fs.ModeSymlink != 0:
		return "symbolic link"
	case e.IsDir():
		return "directory"
	case e.Type().IsRegular():
		return "file"
	default:
		return "special file"
	}
}

// checkRegular reports an entry that is not a regular file or is larger than max.
// Entries come from ReadDir, so a symbolic link is seen as a link.
func checkRegular(e fs.DirEntry, limit int64) error {
	if !e.Type().IsRegular() {
		if e.Type()&fs.ModeSymlink != 0 {
			return errors.New("symbolic links are not allowed")
		}
		return fmt.Errorf("must be a regular file, found a %s", kind(e))
	}
	info, err := e.Info()
	if err != nil {
		return err
	}
	if info.Size() > limit {
		return fmt.Errorf("is larger than %d bytes", limit)
	}
	return nil
}

// readFields checks fields.txt and returns its keys; nil when the file is
// missing, unusable or has no keys, so that the keys of the data are not
// judged against it.
func readFields(fsys fs.FS, entries []fs.DirEntry) (map[string]bool, []error) {
	for _, e := range entries {
		if e.Name() != "fields.txt" {
			continue
		}
		if err := checkRegular(e, maxRootFile); err != nil {
			return nil, []error{fmt.Errorf("fields.txt: %w", err)}
		}
		data, err := fs.ReadFile(fsys, "fields.txt")
		if err != nil {
			return nil, []error{err}
		}
		return parseFields(string(data))
	}
	return nil, []error{errors.New("fields.txt: missing")}
}

// parseFields checks the format of fields.txt: comments, empty lines and one
// key per line, sorted, without repeats, with uuid among them.
func parseFields(text string) (map[string]bool, []error) {
	var errs []error
	keys := map[string]bool{}
	prev, unsorted := "", false
	for i, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !fieldKey.MatchString(line) {
			errs = append(errs, fmt.Errorf("fields.txt: line %d: %q is not a key (%s)", i+1, line, fieldKey))
			continue
		}
		if keys[line] {
			errs = append(errs, fmt.Errorf("fields.txt: line %d: duplicate key %q", i+1, line))
		}
		if line < prev && !unsorted {
			unsorted = true
			errs = append(errs, fmt.Errorf("fields.txt: line %d: keys must be in alphabetical order, %q comes after %q", i+1, line, prev))
		}
		keys[line], prev = true, max(prev, line)
	}
	switch {
	case len(keys) == 0:
		return nil, append(errs, errors.New("fields.txt: no keys"))
	case !keys["uuid"]:
		errs = append(errs, errors.New("fields.txt: the key uuid is required"))
	}
	return keys, errs
}

// checkDataDir checks the files of the directory of one library. keys are the
// keys of fields.txt, nil to skip that check.
func checkDataDir(fsys fs.FS, dir string, keys map[string]bool) []error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return []error{err}
	}
	var errs []error
	present := map[string]bool{}
	uuids := map[string]map[string]bool{}
	for _, e := range entries {
		p := dir + "/" + e.Name()
		if !slices.Contains(dataFiles, e.Name()) {
			errs = append(errs, fmt.Errorf("%s: unexpected %s; a library allows only %s", p, kind(e), strings.Join(dataFiles, ", ")))
			continue
		}
		present[e.Name()] = true
		if err := checkRegular(e, maxDataFile); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		// query.json always has objects, and so has deleted.json of the synthetic
		// library (gen.DefaultConfig trashes some assets); deleted.json of a
		// test library may be empty.
		needObjects := e.Name() == "query.json" || dir == syntheticDir
		ids, fileErrs := checkDataFile(fsys, p, needObjects, keys)
		errs = append(errs, fileErrs...)
		if len(fileErrs) == 0 {
			uuids[e.Name()] = ids
		}
	}
	for _, name := range dataFiles {
		if !present[name] {
			errs = append(errs, fmt.Errorf("%s/%s: missing", dir, name))
		}
	}
	shared := 0
	for id := range uuids["deleted.json"] {
		if uuids["query.json"][id] {
			shared++
		}
	}
	if shared > 0 {
		errs = append(errs, fmt.Errorf("%s: %d uuid values are in both query.json and deleted.json", dir, shared))
	}
	return errs
}

// checkDataFile checks one query.json or deleted.json and returns the set of its uuid values.
func checkDataFile(fsys fs.FS, p string, needObjects bool, keys map[string]bool) (map[string]bool, []error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, []error{err}
	}
	elems, err := parseObjects(data)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", p, err)}
	}
	if needObjects && len(elems) == 0 {
		return nil, []error{fmt.Errorf("%s: no objects", p)}
	}

	var (
		errs      []error
		seen      = map[string]bool{}
		noUUID    []int
		repeated  int
		unknown   = map[string]bool{}
		hostFirst string
		hostCount int
	)
	for i, el := range elems {
		id, ok := el["uuid"].(string)
		switch {
		case !ok || id == "":
			noUUID = append(noUUID, i+1)
		case seen[id]:
			repeated++
		default:
			seen[id] = true
		}
		if keys != nil {
			for k := range el {
				if !keys[k] && k != "uuid" {
					unknown[k] = true
				}
			}
		}
		for _, k := range slices.Sorted(maps.Keys(el)) {
			if where, found := findHostPath(el[k], k); found {
				if hostCount == 0 {
					hostFirst = fmt.Sprintf("element %d, %s", i+1, where)
				}
				hostCount++
			}
		}
	}
	if len(noUUID) > 0 {
		errs = append(errs, fmt.Errorf("%s: %d elements have no uuid string, the first is element %d", p, len(noUUID), noUUID[0]))
	}
	if repeated > 0 {
		errs = append(errs, fmt.Errorf("%s: %d duplicate uuid values", p, repeated))
	}
	if len(unknown) > 0 {
		errs = append(errs, fmt.Errorf("%s: keys that are not in fields.txt: %s", p, strings.Join(slices.Sorted(maps.Keys(unknown)), ", ")))
	}
	if hostCount > 0 {
		errs = append(errs, fmt.Errorf("%s: %d values are paths of a machine or of a library package, the first at %s", p, hostCount, hostFirst))
	}
	return seen, errs
}

// parseObjects parses a JSON array of objects.
func parseObjects(data []byte) ([]map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("is empty")
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("is not valid JSON: %w", err)
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, errors.New("is not a JSON array")
	}
	out := make([]map[string]any, len(list))
	for i, v := range list {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("element %d is not an object", i+1)
		}
		out[i] = obj
	}
	return out, nil
}

// findHostPath looks for a string value, at any depth, that starts with the
// path of a machine or contains ".photoslibrary", and returns where it is
// (keys and indexes, never the value).
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

// checkUUIDs returns an error for every data file in a directory of known
// that has a uuid which is not in the set of that directory. A file that
// cannot be read or parsed is left to checkGolden.
func checkUUIDs(fsys fs.FS, known map[string]map[string]bool) []error {
	var errs []error
	for _, dir := range slices.Sorted(maps.Keys(known)) {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !slices.Contains(dataFiles, e.Name()) || !e.Type().IsRegular() {
				continue
			}
			p := dir + "/" + e.Name()
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
				if id, ok := el["uuid"].(string); ok && !known[dir][id] {
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
