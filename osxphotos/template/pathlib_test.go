package template

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// test_filepath_none of test_template.py (osxphotos 0.78.1, #1541): the
// properties of an unset path are never read (phototemplate.py:1037-1039).
func TestRenderUnsetPath(t *testing.T) {
	t.Parallel()
	o := RenderOptions{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"filepath", "{filepath}", fakePhoto{}, o, one("_"), nil},
		{"filepath.suffix", "{filepath.suffix}", fakePhoto{}, o, one("_"), nil},
		{"filepath.stem", "{filepath.stem}", fakePhoto{}, o, one("_"), nil},
		{"empty default", "{filepath,}", fakePhoto{}, o, one(""), nil},
		{"default", "{filepath,NONE}", fakePhoto{}, o, one("NONE"), nil},
		{"property and default", "{filepath.suffix,NONE}", fakePhoto{}, o, one("NONE"), nil},
		{"bool", "{filepath?yes,no}", fakePhoto{}, o, one("no"), nil},
		{"property and bool", "{filepath.suffix?yes,no}", fakePhoto{}, o, one("no"), nil},
		{"condition", "{filepath.suffix contains aae?matched,}IMG", fakePhoto{}, o, one("IMG"), nil},
		{"filepath.parent", "{filepath.parent}", fakePhoto{}, o, one("_"), nil},
		{"export_dir", "{export_dir}", fakePhoto{}, o, one("_"), nil},
		{"export_dir.name", "{export_dir.name}", fakePhoto{}, o, one("_"), nil},
		{"none str", "{filepath}", fakePhoto{}, RenderOptions{NoneStr: "-"}, one("-"), nil},
		{"suffixes", "{filepath.suffixes}", fakePhoto{}, o, one("_"), nil},
		{"parts", "{export_dir.parts}", fakePhoto{}, o, one("_"), nil},
		{"unknown property", "{export_dir.parents}", fakePhoto{}, o, one("_"), nil},
		{"property of a string", "{filepath.stem.x}", fakePhoto{}, o, one("_"), nil},
		{"empty property", "{filepath.}", fakePhoto{}, o, one("_"), nil},
	})
}

// test_filepath and test_export_dir of test_template.py (osxphotos 0.78.1).
func TestRenderPathField(t *testing.T) {
	t.Parallel()
	o := RenderOptions{Filepath: "/foo/bar.jpeg", ExportDir: "/foo/bar"}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"filepath", "{filepath}", fakePhoto{}, o, one("/foo/bar.jpeg"), nil},
		{"filepath.name", "{filepath.name}", fakePhoto{}, o, one("bar.jpeg"), nil},
		{"filepath.parent", "{filepath.parent}", fakePhoto{}, o, one("/foo"), nil},
		{"filepath.parent.name", "{filepath.parent.name}", fakePhoto{}, o, one("foo"), nil},
		{"filepath.stem", "{filepath.stem}", fakePhoto{}, o, one("bar"), nil},
		{"filepath.suffix", "{filepath.suffix}", fakePhoto{}, o, one(".jpeg"), nil},
		{"export_dir", "{export_dir}", fakePhoto{}, o, one("/foo/bar"), nil},
		{"export_dir.name", "{export_dir.name}", fakePhoto{}, o, one("bar"), nil},
		{"export_dir.parent", "{export_dir.parent}", fakePhoto{}, o, one("/foo"), nil},
		{"export_dir.stem", "{export_dir.stem}", fakePhoto{}, o, one("bar"), nil},
		{"export_dir.suffix", "{export_dir.suffix}", fakePhoto{}, o, one(""), nil},
	})
	for in, want := range map[string]error{
		"{filepath.foo}":       ErrRender,
		"{export_dir.foo}":     ErrRender,
		"{export_dir.parents}": ErrRender,
		"{filepath.stem.x}":    ErrRender,
		"{filepath.}":          ErrRender,
		"{filepath.suffixes}":  ErrUnsupportedConstruct,
		"{export_dir.parts}":   ErrUnsupportedConstruct,
	} {
		name := strings.Trim(in, "{}")
		if got, _, err := render(t, in, fakePhoto{}, o); !errors.Is(err, want) || !strings.Contains(fmt.Sprint(err), name) || got != nil {
			t.Errorf("Render(%q) = %q, %v; want an error that wraps %v and names %q", in, got, err, want, name)
		}
	}
}
