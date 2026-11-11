package golden

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestTemplateLines(t *testing.T) {
	t.Parallel()

	t.Run("the file of the repository", func(t *testing.T) {
		t.Parallel()
		got, err := TemplateLines(testlib.RepoDir(t, "golden"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 518 || got["t001"] != "{name}" || got["t501"] != "{moment}" {
			t.Errorf("got %d templates, t001 = %q, t501 = %q; want 518, {name}, {moment}", len(got), got["t001"], got["t501"])
		}
	})
	t.Run("comments and empty lines are skipped, the ids follow the lines", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "templates.txt"), "# t001 comment\n\n{a}\n# t002 comment\n{b}\n\n{c:05d}\n")
		got, err := TemplateLines(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]string{"t001": "{a}", "t002": "{b}", "t003": "{c:05d}"}; !reflect.DeepEqual(got, want) {
			t.Errorf("TemplateLines = %v, want %v", got, want)
		}
	})
	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()
		_, err := TemplateLines(filepath.Join(t.TempDir(), "SECRET-DIR"))
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "templates.txt") || strings.Contains(err.Error(), "SECRET-DIR") {
			t.Errorf("error = %v, want a missing templates.txt without its directory", err)
		}
	})
}

func libraryWithTemplates(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "lib", "templates.json"), content)
	return dir
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	const (
		u1 = "00000000-0000-0000-0000-000000000001"
		u2 = "00000000-0000-0000-0000-000000000002"
	)

	t.Run("the reference of the repository", func(t *testing.T) {
		t.Parallel()
		got, err := Templates(testlib.RepoDir(t, "golden"), "synthetic")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("no photo in the templates of synthetic")
		}
		missing := 0
		for _, values := range got {
			if _, ok := values["t501"]; !ok {
				missing++
			}
		}
		if missing != 0 {
			t.Errorf("%d of %d photos have no t501", missing, len(got))
		}
	})

	t.Run("a string is one value and a list several", func(t *testing.T) {
		t.Parallel()
		dir := libraryWithTemplates(t, `[{"uuid": "`+u1+`", "t001": "a", "t002": ["b", "c"]}, {"uuid": "`+u2+`", "t001": "", "t002": []}]`)
		got, err := Templates(dir, "lib")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]map[string][]string{
			u1: {"t001": {"a"}, "t002": {"b", "c"}},
			u2: {"t001": {""}, "t002": {}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Templates = %v, want %v", got, want)
		}
	})

	for _, c := range []struct {
		name, content, wantErr, mustNotContain string
	}{
		{"not an array", `{"uuid": "SECRET-A"}`, "templates.json: not one JSON array", "SECRET"},
		{"element is not an object", `["SECRET-B"]`, "templates.json: element 1 is not an object", "SECRET"},
		{"no uuid", `[{"t001": "a"}]`, "element 1: uuid is not a string", ""},
		{"uuid is a number", `[{"uuid": 5, "t001": "a"}]`, "element 1: uuid is not a string", ""},
		{"uuid is null", `[{"uuid": null, "t001": "a"}]`, "element 1: uuid is not a string", ""},
		{"a number", `[{"uuid": "` + u1 + `", "t001": "a"}, {"uuid": "` + u2 + `", "t001": "b", "t002": 12345}]`,
			"element 2: id t002 is neither a string nor a list of strings", "12345"},
		{"null", `[{"uuid": "` + u1 + `", "t003": null}]`, "element 1: id t003 is neither a string nor a list of strings", ""},
		{"a list with null", `[{"uuid": "` + u1 + `", "t006": ["SECRET-E", null]}]`,
			"element 1: id t006 is neither a string nor a list of strings", "SECRET"},
		{"the lowest bad id is named", `[{"uuid": "` + u1 + `", "t009": 1, "t008": 2, "t007": 3, "t005": 4, "t004": 5, "t003": 6, "t002": 7}]`,
			"element 1: id t002 is neither", ""},
		{"a list with a number", `[{"uuid": "` + u1 + `", "t004": ["SECRET-C", 7]}]`,
			"element 1: id t004 is neither a string nor a list of strings", "SECRET"},
		{"an object", `[{"uuid": "` + u1 + `", "t005": {"x": "SECRET-D"}}]`,
			"element 1: id t005 is neither a string nor a list of strings", "SECRET"},
		{"a repeated uuid", `[{"uuid": "` + u1 + `"}, {"uuid": "` + u1 + `"}]`, "element 2: uuid repeated", u1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Templates(libraryWithTemplates(t, c.content), "lib")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want one with %q", err, c.wantErr)
			}
			if c.mustNotContain != "" && strings.Contains(err.Error(), c.mustNotContain) {
				t.Errorf("error %q repeats %q", err, c.mustNotContain)
			}
		})
	}

	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()
		_, err := Templates(filepath.Join(t.TempDir(), "SECRET-DIR"), "lib")
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "templates.json") || strings.Contains(err.Error(), "SECRET-DIR") {
			t.Errorf("error = %v, want a missing templates.json without its directory", err)
		}
	})

	// The name is checked before any file is opened: with a directory that
	// does not exist the error is still about the name.
	for _, name := range []string{"", "a/b", `a\b`, ".", "..", "../x"} {
		t.Run("library "+name, func(t *testing.T) {
			t.Parallel()
			_, err := Templates(filepath.Join(t.TempDir(), "absent"), name)
			if err == nil || !strings.Contains(err.Error(), "one directory name") {
				t.Errorf("Templates(%q) error = %v, want a complaint about the name", name, err)
			}
		})
	}
}
