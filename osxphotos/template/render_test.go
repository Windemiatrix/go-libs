package template

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type fakePhoto map[string]Value

func (p fakePhoto) Field(name string) (Value, bool) {
	v, ok := p[name]
	return v, ok
}

// countingPhoto gives every field a value and counts the questions.
type countingPhoto struct{ calls int }

func (p *countingPhoto) Field(string) (Value, bool) {
	p.calls++
	return Value{Strings: []string{"x"}}, true
}

func strs(s ...string) Value { return Value{Strings: s} }

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return out
}

// examplePhoto is the photo of the examples of phototemplate.md.
func examplePhoto() fakePhoto {
	id := int64(42)
	return fakePhoto{
		"title":         strs("My Photo Title"),
		"descr":         {},
		"keyword":       strs("foo", "bar"),
		"album":         strs("Album1"),
		"person":        {},
		"favorite":      strs("favorite"),
		"hdr":           strs("hdr"),
		"original_name": strs("IMG_0001"),
		"folder_album":  {Albums: []AlbumRef{{Folders: []string{"Folder1"}, Title: "Album1"}}},
		"id":            {Int: &id},
		"photo.title":   strs("My Photo Title"),
	}
}

func with(p fakePhoto, name string, v Value) fakePhoto {
	p[name] = v
	return p
}

func render(t *testing.T, s string, p Photo, o RenderOptions) ([]string, []string, error) {
	t.Helper()
	tmpl, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return NewRenderer().Render(tmpl, p, o)
}

type renderCase struct {
	name      string
	in        string
	photo     fakePhoto
	opts      RenderOptions
	want      []string
	unmatched []string
}

func renderCases() []renderCase {
	ex := examplePhoto
	none := Value{}
	return []renderCase{
		// phototemplate.md:13, 19-24
		{"md13", "The title of the photo is {title}", ex(), RenderOptions{}, []string{"The title of the photo is My Photo Title"}, nil},
		{"md19", "{keyword}", ex(), RenderOptions{}, []string{"foo", "bar"}, nil},
		{"md22", "{,+keyword}", ex(), RenderOptions{}, []string{"foo,bar"}, nil},
		{"md23", "{; +keyword}", ex(), RenderOptions{}, []string{"foo; bar"}, nil},
		{"md24", "{+keyword}", ex(), RenderOptions{}, []string{"foobar"}, nil},
		// phototemplate.md:78-82; "()" is the default separator, as t080 of templates.json shows
		{"md78", "{folder_album}", ex(), RenderOptions{}, []string{"Folder1/Album1"}, nil},
		{"md81", "{folder_album(>)}", ex(), RenderOptions{}, []string{"Folder1>Album1"}, nil},
		{"md82", "{folder_album()}", ex(), RenderOptions{}, []string{"Folder1/Album1"}, nil},
		{"path sep option", "{folder_album}", ex(), RenderOptions{PathSep: ":"}, []string{"Folder1:Album1"}, nil},
		{"albums sorted", "{folder_album}", with(ex(), "folder_album", Value{Albums: []AlbumRef{{Title: "b"}, {Folders: []string{"A", "B"}, Title: "z"}}}),
			RenderOptions{}, []string{"A/B/z", "b"}, nil},
		// products (phototemplate.py:774-783): the values of the later field vary slowest
		{"product of two", "{keyword}_{album}", with(with(ex(), "keyword", strs("k1", "k2")), "album", strs("a1", "a2")),
			RenderOptions{}, []string{"k1_a1", "k2_a1", "k1_a2", "k2_a2"}, nil},
		{"product of three", "{keyword}/{album}/{person}",
			with(with(with(ex(), "keyword", strs("k1", "k2")), "album", strs("a1", "a2")), "person", strs("p1", "p2")),
			RenderOptions{}, []string{
				"k1/a1/p1", "k2/a1/p1", "k1/a2/p1", "k2/a2/p1",
				"k1/a1/p2", "k2/a1/p2", "k1/a2/p2", "k2/a2/p2",
			}, nil},
		{"single times multi", "{title}-{keyword}", ex(), RenderOptions{}, []string{"My Photo Title-foo", "My Photo Title-bar"}, nil},
		// phototemplate.md:137
		{"md137 no title", "{title}", with(ex(), "title", none), RenderOptions{}, []string{"_"}, nil},
		{"id", "{id}", ex(), RenderOptions{}, []string{"42"}, nil},
		{"photo property", "{photo.title}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
		// options
		{"expand inplace", "{keyword}", ex(), RenderOptions{ExpandInplace: true}, []string{"foo,bar"}, nil},
		{"expand inplace sep", "{keyword}", ex(), RenderOptions{ExpandInplace: true, InplaceSep: ";"}, []string{"foo;bar"}, nil},
		{"expand inplace and delim", "{-+keyword}", ex(), RenderOptions{ExpandInplace: true, InplaceSep: ";"}, []string{"foo-bar"}, nil},
		{"strip option", " {title} ", ex(), RenderOptions{Strip: true}, []string{"My Photo Title"}, nil},
		{"none str", "{title}", with(ex(), "title", none), RenderOptions{NoneStr: "-"}, []string{"-"}, nil},
		// statements
		{"empty", "", ex(), RenderOptions{}, nil, nil},
		{"literal", "literal text", ex(), RenderOptions{}, []string{"literal text"}, nil},
		// The unknown field drops its own post-text "b" too (phototemplate.py:659-661).
		{"unmatched then field", "a{nosuch}b{title}", ex(), RenderOptions{}, []string{"My Photo Title"}, []string{"nosuch"}},
		{"unmatched", "{nosuch}", ex(), RenderOptions{}, nil, []string{"nosuch"}},
		{"unmatched last", "{keyword}{nosuch}", ex(), RenderOptions{}, nil, []string{"nosuch"}},
		{"unmatched dotted", "{album.x}", ex(), RenderOptions{}, nil, []string{"album.x"}},
		// shell_quote is multi-valued: upstream matches its name whole (phototemplate.py:875)
		{"unmatched dotted not rendered yet", "{shell_quote.x}", ex(), RenderOptions{}, nil, []string{"shell_quote.x"}},
	}
}

func TestRenderExamples(t *testing.T) {
	t.Parallel()

	for _, tt := range renderCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, unmatched, err := render(t, tt.in, tt.photo, tt.opts)
			if err != nil || !slices.Equal(got, tt.want) || !slices.Equal(unmatched, tt.unmatched) {
				t.Errorf("Render(%q) = %q, %q, %v; want %q, %q, nil", tt.in, got, unmatched, err, tt.want, tt.unmatched)
			}
		})
	}
}

func TestRenderEmptyIsNil(t *testing.T) {
	t.Parallel()
	got, unmatched, err := render(t, "", examplePhoto(), RenderOptions{})
	if got != nil || unmatched != nil || err != nil {
		t.Errorf("Render(\"\") = %#v, %#v, %v; want nil, nil, nil", got, unmatched, err)
	}
}

// The slices of a Photo belong to its implementation and may have spare
// capacity: without slices.Clone of a.Folders, the title "Album1" lands in
// folders[1].
func TestRenderKeepsPhotoValues(t *testing.T) {
	t.Parallel()
	const in = "{keyword}/{folder_album}/{title}"
	want := []string{
		"foo/Folder1/Album1/My Photo Title",
		"bar/Folder1/Album1/My Photo Title",
	}
	wantUnmatched := []string(nil)
	keywords := []string{"foo", "bar", "spare"}
	folders := []string{"Folder1", "spare"}
	p := with(with(examplePhoto(), "keyword", Value{Strings: keywords[:2]}),
		"folder_album", Value{Albums: []AlbumRef{{Folders: folders[:1], Title: "Album1"}}})

	got, unmatched, err := render(t, in, p, RenderOptions{})
	if err != nil || !slices.Equal(got, want) || !slices.Equal(unmatched, wantUnmatched) {
		t.Fatalf("Render(%q) = %q, %q, %v; want %q, %q, nil", in, got, unmatched, err, want, wantUnmatched)
	}
	for i := range got {
		got[i] = "changed"
	}
	for i := range unmatched {
		unmatched[i] = "changed"
	}
	if !slices.Equal(keywords, []string{"foo", "bar", "spare"}) || !slices.Equal(folders, []string{"Folder1", "spare"}) ||
		p["title"].Strings[0] != "My Photo Title" {
		t.Errorf("after Render and a change of its result: keywords %q, folders %q, title %q; want them untouched",
			keywords, folders, p["title"].Strings)
	}

	if got, _, err = render(t, in, p, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	keywords[0], folders[0] = "changed", "changed"
	p["title"].Strings[0] = "changed"
	if !slices.Equal(got, want) {
		t.Errorf("after a change of the photo, the result of Render = %q, want %q", got, want)
	}
}

func TestRenderErrors(t *testing.T) {
	t.Parallel()

	missing := fakePhoto{"keyword": strs("foo")}
	cases := []struct {
		in    string
		photo Photo
		want  error
		names string
	}{
		{"{exiftool:EXIF:Make}", &countingPhoto{}, ErrUnsupportedField, "exiftool"},
		{"{function:f.py::g}", &countingPhoto{}, ErrUnsupportedField, "function"},
		{"{detected_text}", &countingPhoto{}, ErrUnsupportedField, "detected_text"},
		{"{osxphotos_cmd_line}", &countingPhoto{}, ErrUnsupportedField, "osxphotos_cmd_line"},
		{"{place.name}", missing, ErrUnsupportedField, "place.name"},
		{"{photo.score.overall}", missing, ErrUnsupportedField, "photo.score.overall"},
		{"{title}", missing, ErrUnsupportedField, "title"},
		// upstream raises ValueError for a dotted name of a single-valued field it does not handle (1001-1003)
		{"{title.x}", missing, ErrUnsupportedField, "title.x"},
		{"{media_type}", missing, ErrUnsupportedField, "media_type"},
		{"{folder_album}", missing, ErrUnsupportedField, "folder_album"},
		{"{keyword&x}", missing, ErrUnsupportedConstruct, "keyword"},
		{"{var:a,b}", missing, ErrUnsupportedConstruct, "var"},
		{"{%a}", missing, ErrUnsupportedConstruct, "%a"},
		{"{id:05d}", missing, ErrUnsupportedConstruct, "id"},
		{"{created.year}", missing, ErrUnsupportedConstruct, "created.year"},
		{"{modified}", missing, ErrUnsupportedConstruct, "modified"},
		{"{today.year}", missing, ErrUnsupportedConstruct, "today.year"},
		{"{shell_quote,x}", missing, ErrUnsupportedConstruct, "shell_quote"},
		{"{format:int:05d,42}", missing, ErrUnsupportedConstruct, "format"},
		{"{counter}", missing, ErrUnsupportedConstruct, "counter"},
		{"{counter.a}", missing, ErrUnsupportedConstruct, "counter.a"},
		{"{album_seq}", missing, ErrUnsupportedConstruct, "album_seq"},
		{"{folder_album_seq(1)}", missing, ErrUnsupportedConstruct, "folder_album_seq"},
		{"{export_dir.stem}", missing, ErrUnsupportedConstruct, "export_dir.stem"},
		{"{filepath.name}", missing, ErrUnsupportedConstruct, "filepath.name"},
	}
	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, _, err := render(t, tt.in, tt.photo, RenderOptions{})
			if !errors.Is(err, tt.want) || !strings.Contains(fmt.Sprint(err), tt.names) || got != nil {
				t.Errorf("Render(%q) = %q, %v; want an error that wraps %v and names %q", tt.in, got, err, tt.want, tt.names)
			}
			if c, ok := tt.photo.(*countingPhoto); ok && c.calls != 0 {
				t.Errorf("Render(%q) asked the photo %d times; a field that runs code must not reach it", tt.in, c.calls)
			}
		})
	}
}

func TestRenderMaxResults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		keywords int
		albums   int
		max      int
		ok       bool
	}{
		{"at the limit", "{keyword}", 3, 0, 3, true},
		{"one over the limit", "{keyword}", 4, 0, 3, false},
		{"product at the limit", "{keyword}{album}", 100, 100, 10000, true},
		{"product over the limit", "{keyword}{album}", 101, 101, 10000, false},
		{"default at the limit", "{keyword}{album}", 100, 100, 0, true},
		{"default over the limit", "{keyword}", DefaultMaxResults + 1, 0, 0, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := fakePhoto{"title": {}, "keyword": strs(numbered("k", tt.keywords)...), "album": strs(numbered("a", tt.albums)...)}
			got, _, err := render(t, tt.in, p, RenderOptions{MaxResults: tt.max})
			want := max(tt.keywords, 1) * max(tt.albums, 1)
			switch {
			case tt.ok && (err != nil || len(got) != want):
				t.Errorf("Render(%q) gave %d values, %v; want %d values", tt.in, len(got), err, want)
			case !tt.ok && (!errors.Is(err, ErrTooManyValues) || got != nil):
				t.Errorf("Render(%q) gave %d values, %v; want ErrTooManyValues", tt.in, len(got), err)
			}
		})
	}
}
