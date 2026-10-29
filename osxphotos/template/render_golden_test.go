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

// The templates of templates.txt whose value does not depend on the photo:
// osxphotos gave every photo the same value, which Render must give for a
// photo without fields.
var literalIDs = []string{
	"t126", "t127", "t128", "t129", "t130", "t131", "t132", "t133", "t134", "t135",
	"t136", "t137", "t138", "t139", "t140", "t141", "t142", "t143", "t144", "t145",
	"t146", "t147", "t148", "t149", "t150", "t151", "t152", "t153", "t154", "t155",
	"t156", "t157", "t158", "t159",
	"t301", "t302", "t303", "t304", "t305", "t307", "t308", "t309", "t310", "t312",
	"t313",
	"t314", "t315", "t316", "t317", "t318", "t319", "t320", "t321", "t322", "t323",
	"t324", "t325", "t326", "t327", "t328", "t329", "t330", "t332", "t333", "t334",
	"t335", "t336", "t341", "t343",
	"t391", "t392", "t393", "t395", "t396", "t397", "t402",
}

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

func TestRenderGoldenLiterals(t *testing.T) {
	dir := testlib.RepoDir(t, "golden")
	templates := goldenTemplates(t, dir)
	objects := goldenObjects(t, filepath.Join(dir, "synthetic", "templates.json"))

	for _, id := range literalIDs {
		k, err := strconv.Atoi(id[1:])
		if err != nil || k < 1 || k > len(templates) {
			t.Fatalf("%s is not an id of the %d templates of templates.txt", id, len(templates))
		}
		tmpl, err := Parse(templates[k-1])
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		got, unmatched, err := NewRenderer().Render(tmpl, fakePhoto{}, RenderOptions{})
		if err != nil || unmatched != nil {
			t.Errorf("%s: Render(%q) = %q, %q, %v; want no error and no unmatched", id, templates[k-1], got, unmatched, err)
			continue
		}
		for i, obj := range objects {
			raw, ok := obj[id]
			if !ok {
				t.Fatalf("%s: element %d of templates.json has no value", id, i+1)
			}
			if want := oracleValues(t, raw); !slices.Equal(got, want) {
				t.Errorf("%s: Render(%q) = %q, osxphotos gave %q for element %d", id, templates[k-1], got, want, i+1)
				break
			}
		}
	}
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
