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
	fileOnly := RenderOptions{Filepath: "/foo/bar.jpeg"}
	dirOnly := RenderOptions{ExportDir: "/foo/bar"}
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
		{"export_dir with only filepath set", "{export_dir}", fakePhoto{}, fileOnly, one("_"), nil},
		{"export_dir.name with only filepath set", "{export_dir.name}", fakePhoto{}, fileOnly, one("_"), nil},
		{"export_dir.parts with only filepath set", "{export_dir.parts}", fakePhoto{}, fileOnly, one("_"), nil},
		{"filepath with only filepath set", "{filepath}", fakePhoto{}, fileOnly, one("/foo/bar.jpeg"), nil},
		{"filepath with only export_dir set", "{filepath}", fakePhoto{}, dirOnly, one("_"), nil},
		{"filepath.stem with only export_dir set", "{filepath.stem}", fakePhoto{}, dirOnly, one("_"), nil},
		{"filepath.suffixes with only export_dir set", "{filepath.suffixes}", fakePhoto{}, dirOnly, one("_"), nil},
		{"export_dir with only export_dir set", "{export_dir}", fakePhoto{}, dirOnly, one("/foo/bar"), nil},
	})
}

// A set path still checks its properties (phototemplate.py:1642-1652); the
// values are those of TestPathlib.
func TestRenderSetPathErrors(t *testing.T) {
	t.Parallel()
	o := RenderOptions{Filepath: "/foo/bar.jpeg", ExportDir: "/foo/bar"}
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
