package golden

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// refLibraries are the libraries that have reference data: the test
// libraries and the synthetic one.
func refLibraries(t testing.TB) []string {
	t.Helper()
	return append(testlib.Libraries(t), "synthetic")
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// pair writes query.json and deleted.json into a new directory and returns
// the two paths.
func pair(t testing.TB, query, deleted string) (queryPath, deletedPath string) {
	t.Helper()
	dir := t.TempDir()
	queryPath, deletedPath = filepath.Join(dir, "query.json"), filepath.Join(dir, "deleted.json")
	writeFile(t, queryPath, query)
	writeFile(t, deletedPath, deleted)
	return queryPath, deletedPath
}

func TestReadFields(t *testing.T) {
	t.Parallel()
	t.Run("the file of the repository", func(t *testing.T) {
		t.Parallel()
		keys, err := ReadFields(filepath.Join(Dir(t), "fields.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 56 || keys[0] != "album_info" || keys[len(keys)-1] != "width" {
			t.Errorf("got %d keys from %q to %q, want 56 from album_info to width", len(keys), keys[0], keys[len(keys)-1])
		}
	})
	t.Run("comments and empty lines are skipped, the order is kept", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "fields.txt")
		writeFile(t, path, "# comment\n\nb\n# more\na\nuuid\n")
		keys, err := ReadFields(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"b", "a", "uuid"}; !slices.Equal(keys, want) {
			t.Errorf("ReadFields = %v, want %v", keys, want)
		}
	})
	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()
		_, err := ReadFields(filepath.Join(t.TempDir(), "SECRET-DIR", "fields.txt"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist", err)
		}
		if strings.Contains(err.Error(), "SECRET-DIR") {
			t.Errorf("the error names the directory: %v", err)
		}
	})
}

func TestLoadRealData(t *testing.T) {
	t.Parallel()
	dir := Dir(t)
	fields, err := ReadFields(filepath.Join(dir, "fields.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Test-13.0.0", "synthetic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ref, err := Load(dir, name)
			if err != nil {
				t.Fatal(err)
			}
			if ref.Library != name || len(ref.Query) == 0 {
				t.Fatalf("Library = %q with %d objects in query.json", ref.Library, len(ref.Query))
			}
			for _, obj := range slices.Concat(ref.Query, ref.Deleted) {
				if len(obj) != len(fields) {
					t.Fatalf("an object has %d keys, want the %d of fields.txt", len(obj), len(fields))
				}
				for k := range obj {
					if !slices.Contains(fields, k) {
						t.Fatalf("key %s is not in fields.txt", k)
					}
				}
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()
	const fields = "# keys\ntitle\nuuid\n"
	good := func(uuid string) string { return `{"title": "t", "uuid": "` + uuid + `", "extra": 1}` }
	const (
		u1 = "00000000-0000-0000-0000-000000000001"
		u2 = "00000000-0000-0000-0000-000000000002"
	)

	cases := []struct {
		name           string
		query, deleted string
		want           string // part of the error; "" is success
		wantNot        string // must not be in the error
		noQueryFile    bool
		noDeletedFile  bool
	}{
		{name: "valid", query: "[" + good(u1) + "]", deleted: "[" + good(u2) + "]"},
		{name: "empty arrays", query: "[]", deleted: "[]"},
		{name: "whitespace around the array", query: " \n[]\n ", deleted: "[]"},
		{name: "no query.json", noQueryFile: true, deleted: "[]", want: "query.json"},
		{name: "no deleted.json", query: "[]", noDeletedFile: true, want: "deleted.json"},
		{name: "empty file", query: "", deleted: "[]", want: "query.json: the file is empty"},
		{name: "not JSON", query: "[{", deleted: "[]", want: "query.json: not one valid JSON value"},
		{name: "data after the array", query: "[] []", deleted: "[]", want: "query.json: not one valid JSON value"},
		{name: "garbage after the array", query: "[]x", deleted: "[]", want: "query.json: not one valid JSON value"},
		{name: "not an array", query: `{"uuid": "SECRET-A"}`, deleted: "[]", want: "query.json: not a JSON array", wantNot: "SECRET"},
		{name: "null", query: "null", deleted: "[]", want: "query.json: not a JSON array"},
		{name: "element is not an object", query: "[" + good(u1) + `, "SECRET-B"]`, deleted: "[]", want: "query.json: element 2 is not an object", wantNot: "SECRET"},
		{name: "no uuid", query: `[{"title": "t"}]`, deleted: "[]", want: "query.json: element 1: missing key uuid"},
		{name: "uuid is not a string", query: `[{"title": "t", "uuid": 7}]`, deleted: "[]", want: "element 1: uuid is not a non-empty string"},
		{name: "uuid is an object", query: `[{"title": "t", "uuid": {"k": "SECRET-U"}}]`, deleted: "[]", want: "element 1: uuid is not a non-empty string", wantNot: "SECRET"},
		{name: "uuid is empty", query: `[{"title": "t", "uuid": ""}]`, deleted: "[]", want: "element 1: uuid is not a non-empty string"},
		{name: "no key of fields.txt", query: `[{"uuid": "` + u1 + `"}]`, deleted: "[]", want: "element 1: missing key title", wantNot: u1},
		{name: "deleted.json has the error", query: "[]", deleted: `[` + good(u1) + `, {"uuid": "` + u2 + `"}]`, want: "deleted.json: element 2: missing key title", wantNot: u2},
		{name: "uuid repeats in query.json", query: "[" + good(u1) + "," + good(u1) + "]", deleted: "[]", want: "query.json: element 2: the uuid repeats that of query.json element 1", wantNot: u1},
		{name: "uuid is in both files", query: "[" + good(u1) + "]", deleted: "[" + good(u2) + "," + good(u1) + "]", want: "deleted.json: element 2: the uuid repeats that of query.json element 1", wantNot: u1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "fields.txt"), fields)
			lib := filepath.Join(dir, "lib")
			if err := os.Mkdir(lib, 0o700); err != nil {
				t.Fatal(err)
			}
			if !c.noQueryFile {
				writeFile(t, filepath.Join(lib, "query.json"), c.query)
			}
			if !c.noDeletedFile {
				writeFile(t, filepath.Join(lib, "deleted.json"), c.deleted)
			}
			ref, err := Load(dir, "lib")
			if c.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if ref.Library != "lib" {
					t.Errorf("Library = %q", ref.Library)
				}
				for _, o := range slices.Concat(ref.Query, ref.Deleted) {
					if _, ok := o["extra"]; ok || len(o) != 2 {
						t.Errorf("object %v: only the keys of fields.txt are kept", o)
					}
				}
				return
			}
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
			if c.wantNot != "" && strings.Contains(err.Error(), c.wantNot) {
				t.Errorf("error %q repeats %q", err, c.wantNot)
			}
		})
	}

	t.Run("fields.txt is missing", func(t *testing.T) {
		t.Parallel()
		if _, err := Load(t.TempDir(), "lib"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error = %v, want os.ErrNotExist", err)
		}
	})

	// The name of a library is checked before any file is opened: with a
	// directory that does not exist the error is still about the name.
	for _, name := range []string{"", "a/b", `a\b`, ".", "..", "../x"} {
		t.Run("library "+name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(filepath.Join(t.TempDir(), "absent"), name)
			if err == nil || !strings.Contains(err.Error(), "one directory name") {
				t.Errorf("Load(%q) error = %v, want a complaint about the name", name, err)
			}
		})
	}
}

func TestLoadFiles(t *testing.T) {
	t.Parallel()
	q, d := pair(t,
		`[{"title": "t", "uuid": "00000000-0000-0000-0000-000000000001", "library": "SECRET-L", "path": "/Users/x/a.jpg"}]`,
		`[]`)
	ref, err := LoadFiles("local", q, d, []string{"title"})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Library != "local" || len(ref.Query) != 1 || len(ref.Deleted) != 0 {
		t.Fatalf("got %+v", ref)
	}
	got := ref.Query[0]
	if len(got) != 2 || got["title"] == nil || got["uuid"] == nil {
		t.Errorf("object keeps %d keys, want title and uuid only", len(got))
	}
	if _, err := LoadFiles("local", q, d, []string{"title", "date"}); err == nil || !strings.Contains(err.Error(), "missing key date") {
		t.Errorf("error = %v, want missing key date", err)
	}
	if _, err := LoadFiles("local", filepath.Join(t.TempDir(), "SECRET-DIR", "q.json"), d, nil); !errors.Is(err, os.ErrNotExist) ||
		strings.Contains(err.Error(), "SECRET-DIR") {
		t.Errorf("error = %v, want a missing file without its directory", err)
	}
}

// object renders an object of the reference with a date, a date_added and a uuid.
func object(date, added, uuid string) Object {
	o := Object{"uuid": json.RawMessage(`"` + uuid + `"`), "date": json.RawMessage(`"` + date + `"`)}
	if added == "" {
		o["date_added"] = json.RawMessage("null")
	} else {
		o["date_added"] = json.RawMessage(`"` + added + `"`)
	}
	return o
}

func TestReferenceOrder(t *testing.T) {
	t.Parallel()
	t.Run("the reference data", func(t *testing.T) {
		t.Parallel()
		dir := Dir(t)
		for _, name := range refLibraries(t) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ref, err := Load(dir, name)
				if err != nil {
					t.Fatal(err)
				}
				if err := ref.CheckOrder(); err != nil {
					t.Error(err)
				}
			})
		}
	})

	const (
		u1 = "00000000-0000-0000-0000-000000000001"
		u2 = "00000000-0000-0000-0000-000000000002"
	)
	cases := []struct {
		name  string
		query []Object
		want  string // "" is ordered
	}{
		{"empty", nil, ""},
		{"one object", []Object{object("2020-01-01T00:00:00+00:00", "", u1)}, ""},
		{"by date", []Object{object("2020-01-01T00:00:00+00:00", "", u1), object("2020-01-01T00:00:01+00:00", "", u1+"0")}, ""},
		{"swapped by date", []Object{object("2020-01-01T00:00:01+00:00", "", u1), object("2020-01-01T00:00:00+00:00", "", u2)}, "query.json: element 2 is out of order"},
		{"dates are moments, not strings", []Object{
			object("2020-01-01T10:00:00+05:00", "", u2), // 05:00 UTC
			object("2020-01-01T01:00:00-05:00", "", u1), // 06:00 UTC
		}, ""},
		{"same moment in other offsets is a tie", []Object{
			object("2020-01-01T10:00:00+05:00", "2020-01-01T00:00:00+00:00", u1),
			object("2020-01-01T05:00:00+00:00", "2020-01-02T00:00:00+00:00", u2),
		}, ""},
		{"tie by date, then date_added", []Object{
			object("2020-01-01T00:00:00+00:00", "2020-01-02T00:00:00+00:00", u1),
			object("2020-01-01T00:00:00+00:00", "2020-01-01T00:00:00+00:00", u2),
		}, "element 2 is out of order"},
		{"null date_added is the epoch", []Object{
			object("2020-01-01T00:00:00+00:00", "", u2),
			object("2020-01-01T00:00:00+00:00", "1970-01-01T00:00:01+00:00", u1),
		}, ""},
		{"epoch date_added equals null", []Object{
			object("2020-01-01T00:00:00+00:00", "", u2),
			object("2020-01-01T00:00:00+00:00", "1970-01-01T00:00:00+00:00", u1),
		}, "element 2 is out of order"},
		{"tie by date and date_added, then uuid", []Object{
			object("2020-01-01T00:00:00+00:00", "2020-01-01T00:00:00+00:00", u2),
			object("2020-01-01T00:00:00+00:00", "2020-01-01T00:00:00+00:00", u1),
		}, "element 2 is out of order"},
		{"uuid in bytes, not in letters", []Object{
			object("2020-01-01T00:00:00+00:00", "", "B"),
			object("2020-01-01T00:00:00+00:00", "", "a"),
		}, ""},
		{"a date that is not a date", []Object{{"uuid": json.RawMessage(`"x"`), "date": json.RawMessage(`"SECRET-DATE"`), "date_added": json.RawMessage("null")}}, "query.json: element 1: date: want an ISO 8601 string"},
		{"no date_added", []Object{{"uuid": json.RawMessage(`"x"`), "date": json.RawMessage(`"2020-01-01T00:00:00+00:00"`)}}, "date_added: want a value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := (&Reference{Query: c.query}).CheckOrder()
			switch {
			case c.want == "" && err != nil:
				t.Errorf("CheckOrder = %v, want nil", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Errorf("CheckOrder = %v, want an error with %q", err, c.want)
			}
			if err != nil && strings.Contains(err.Error(), "SECRET") {
				t.Errorf("the error repeats the value: %v", err)
			}
		})
	}

	t.Run("deleted.json is checked, apart from query.json", func(t *testing.T) {
		t.Parallel()
		ref := &Reference{
			Query:   []Object{object("2020-01-02T00:00:00+00:00", "", u2)},
			Deleted: []Object{object("2020-01-01T00:00:00+00:00", "", u1)}, // earlier than query.json: fine
		}
		if err := ref.CheckOrder(); err != nil {
			t.Errorf("CheckOrder = %v, want nil: the files are ordered separately", err)
		}
		ref.Deleted = []Object{object("2020-01-02T00:00:00+00:00", "", u2), object("2020-01-01T00:00:00+00:00", "", u1)}
		if err := ref.CheckOrder(); err == nil || !strings.Contains(err.Error(), "deleted.json: element 2") {
			t.Errorf("CheckOrder = %v, want an error about deleted.json element 2", err)
		}
	})
}
