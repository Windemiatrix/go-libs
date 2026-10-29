package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func goldenTemplates(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, line := range parseGoldenLines(t, filepath.Join(dir, "templates.txt")) {
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func goldenObjects(t *testing.T, path string) []map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run make golden-fetch", err)
	}
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(data, &objects); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(objects) == 0 {
		t.Fatalf("%s holds no photo", path)
	}
	return objects
}

// A string is one value and a list several (print_photo_info.py:138-149).
func oracleValues(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		t.Fatalf("%s is neither a string nor a list of strings", raw)
	}
	return many
}

// The ground for rendering "{folder_album()}" as "{folder_album}": osxphotos
// gave t080 the value of t073 for every photo, and a photo in a folder shows
// that the separator was used (t079, "(>)", differs from t073). The messages
// carry no values of the library.
func TestRenderGoldenFolderAlbumEmptyArg(t *testing.T) {
	dir := testlib.RepoDir(t, "golden")
	templates := goldenTemplates(t, dir)
	for id, want := range map[string]string{"t073": "{folder_album}", "t079": "{folder_album(>)}", "t080": "{folder_album()}"} {
		k, err := strconv.Atoi(id[1:])
		if err != nil || k > len(templates) || templates[k-1] != want {
			t.Fatalf("%s of templates.txt is not %s", id, want)
		}
	}
	inFolder := 0
	for i, obj := range goldenObjects(t, filepath.Join(dir, "synthetic", "templates.json")) {
		plain, empty, other := oracleValues(t, obj["t073"]), oracleValues(t, obj["t080"]), oracleValues(t, obj["t079"])
		if !slices.Equal(plain, empty) {
			t.Errorf("element %d: t080 differs from t073", i+1)
		}
		if !slices.Equal(plain, other) {
			inFolder++
		}
	}
	if inFolder == 0 {
		t.Error("no photo of templates.json is in an album inside a folder: the check proves nothing")
	}
}
