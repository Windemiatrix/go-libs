package template

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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
		// phototemplate.md:84
		{"md84 one pair", "{album[/,-]}", with(ex(), "album", strs("a/b:c/d")), RenderOptions{}, []string{"a-b:c-d"}, nil},
		{"md84 two pairs", "{album[/,-|:,-]}", with(ex(), "album", strs("a/b:c/d")), RenderOptions{}, []string{"a-b-c-d"}, nil},
		{"find and replace each value", "{keyword[o,0]}", ex(), RenderOptions{}, []string{"f00", "bar"}, nil},
		{"find and replace after delim", "{,+keyword[,,;]}", ex(), RenderOptions{}, []string{"foo;bar"}, nil},
		// phototemplate.md:116, 126, 137
		{"md116 favorite", "{favorite?Favorite-{original_name},{original_name}}", ex(), RenderOptions{}, []string{"Favorite-IMG_0001"}, nil},
		{"md116 not favorite", "{favorite?Favorite-{original_name},{original_name}}", with(ex(), "favorite", none), RenderOptions{}, []string{"IMG_0001"}, nil},
		{"md126 hdr", "{hdr?ISHDR,NOTHDR}", ex(), RenderOptions{}, []string{"ISHDR"}, nil},
		{"md126 not hdr", "{hdr?ISHDR,NOTHDR}", with(ex(), "hdr", none), RenderOptions{}, []string{"NOTHDR"}, nil},
		{"bool without default", "{hdr?ISHDR}", with(ex(), "hdr", none), RenderOptions{}, []string{""}, nil},
		{"md137 no title", "{title}", with(ex(), "title", none), RenderOptions{}, []string{"_"}, nil},
		{"md137 default", "{title,I have no title}", with(ex(), "title", none), RenderOptions{}, []string{"I have no title"}, nil},
		{"md137 default unused", "{title,I have no title}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
		// combine
		{"combine", "{title&{keyword,}}", ex(), RenderOptions{}, []string{"My Photo Title", "foo", "bar"}, nil},
		{"combine drops empty strings", "{title&{descr,}}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
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
		{"delim and default", "{,+keyword,none}", with(ex(), "keyword", none), RenderOptions{}, []string{"none"}, nil},
		{"delim and default unused", "{,+keyword,none}", ex(), RenderOptions{}, []string{"foo,bar"}, nil},
		// t164-t169 with and without a title
		{"t164 title", "{title?}", ex(), RenderOptions{}, []string{""}, nil},
		{"t164", "{title?}", with(ex(), "title", none), RenderOptions{}, []string{""}, nil},
		{"t165 title", "{title,}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
		{"t165", "{title,}", with(ex(), "title", none), RenderOptions{}, []string{""}, nil},
		{"t166 title", "{title&}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
		{"t166", "{title&}", with(ex(), "title", none), RenderOptions{}, []string{"_"}, nil},
		{"t167 title", "{title?,}", ex(), RenderOptions{}, []string{""}, nil},
		{"t167", "{title?,}", with(ex(), "title", none), RenderOptions{}, []string{""}, nil},
		{"t168 title", "{title?Y,}", ex(), RenderOptions{}, []string{"Y"}, nil},
		{"t168", "{title?Y,}", with(ex(), "title", none), RenderOptions{}, []string{""}, nil},
		{"t169 title", "{title?,N}", ex(), RenderOptions{}, []string{""}, nil},
		{"t169", "{title?,N}", with(ex(), "title", none), RenderOptions{}, []string{"N"}, nil},
		{"t172", "{title,{descr,{original_name}}}", with(ex(), "title", none), RenderOptions{}, []string{"IMG_0001"}, nil},
		{"t173 keywords", "{keyword?has keywords,no keywords}", ex(), RenderOptions{}, []string{"has keywords"}, nil},
		{"t173", "{keyword?has keywords,no keywords}", with(ex(), "keyword", none), RenderOptions{}, []string{"no keywords"}, nil},
		{"t174 keywords", "{keyword&{album}?T,F}", ex(), RenderOptions{}, []string{"T"}, nil},
		// The combined {album} renders "_" without albums, so the bool value is taken.
		{"t174", "{keyword&{album}?T,F}", with(with(ex(), "keyword", none), "album", none), RenderOptions{}, []string{"T"}, nil},
		{"t175", "{; +keyword|lower[a,b]&{album}?Y,N}", ex(), RenderOptions{}, []string{"Y"}, nil},
		// phototemplate.py:669-670, 765-766
		{"filters do not touch combine", "{keyword|upper&{album}}", ex(), RenderOptions{}, []string{"FOO", "BAR", "Album1"}, nil},
		// fields without photo data
		{"strip", "{strip,  padded  }", fakePhoto{}, RenderOptions{}, []string{"padded"}, nil},
		{"strip of every value", "{strip,{keyword}}", with(ex(), "keyword", strs(" a ", "\U0000001cb\t")), RenderOptions{}, []string{"a", "b"}, nil},
		{"strip without text", "{strip}", fakePhoto{}, RenderOptions{}, []string{"_"}, nil},
		{"export_dir", "{export_dir}", fakePhoto{}, RenderOptions{ExportDir: "/export"}, []string{"/export"}, nil},
		{"export_dir unset", "{export_dir}", fakePhoto{}, RenderOptions{}, []string{"None"}, nil},
		{"filepath", "{filepath}", fakePhoto{}, RenderOptions{Filepath: "/export/IMG_0001.HEIC"}, []string{"/export/IMG_0001.HEIC"}, nil},
		{"filepath unset", "{filepath}", fakePhoto{}, RenderOptions{}, []string{"None"}, nil},
		{"edited_version", "{edited_version}", fakePhoto{}, RenderOptions{EditedVersion: true}, []string{"edited_version"}, nil},
		{"edited_version unset", "{edited_version}", fakePhoto{}, RenderOptions{}, []string{"_"}, nil},
		{"osxphotos_version", "{osxphotos_version}", fakePhoto{}, RenderOptions{}, []string{"0.77.2"}, nil},
		{"punctuation", "{openbrace}{tab}{crlf}", fakePhoto{}, RenderOptions{}, []string{"{\t\r\n"}, nil},
		{"id", "{id}", ex(), RenderOptions{}, []string{"42"}, nil},
		{"photo property", "{photo.title}", ex(), RenderOptions{}, []string{"My Photo Title"}, nil},
		// options
		{"expand inplace", "{keyword}", ex(), RenderOptions{ExpandInplace: true}, []string{"foo,bar"}, nil},
		{"expand inplace sep", "{keyword}", ex(), RenderOptions{ExpandInplace: true, InplaceSep: ";"}, []string{"foo;bar"}, nil},
		{"expand inplace and delim", "{-+keyword}", ex(), RenderOptions{ExpandInplace: true, InplaceSep: ";"}, []string{"foo-bar"}, nil},
		{"strip option", " {title} ", ex(), RenderOptions{Strip: true}, []string{"My Photo Title"}, nil},
		{"strip option nested", "x{title? {title} ,x}y", ex(), RenderOptions{Strip: true}, []string{"xMy Photo Titley"}, nil},
		{"no strip", "x{title? {title} ,x}y", ex(), RenderOptions{}, []string{"x My Photo Title y"}, nil},
		{"none str", "{title}", with(ex(), "title", none), RenderOptions{NoneStr: "-"}, []string{"-"}, nil},
		// statements
		{"empty", "", ex(), RenderOptions{}, nil, nil},
		{"literal", "literal text", ex(), RenderOptions{}, []string{"literal text"}, nil},
		// The unknown field drops its own post-text "b" too (phototemplate.py:659-661).
		{"unmatched then field", "a{nosuch}b{title}", ex(), RenderOptions{}, []string{"My Photo Title"}, []string{"nosuch"}},
		{"unmatched", "{nosuch}", ex(), RenderOptions{}, nil, []string{"nosuch"}},
		{"unmatched last", "{keyword}{nosuch}", ex(), RenderOptions{}, nil, []string{"nosuch"}},
		{"unmatched in default", "{title,{nosuch}}", with(ex(), "title", none), RenderOptions{}, []string{"_"}, []string{"nosuch"}},
		{"unmatched dotted", "{album.x}", ex(), RenderOptions{}, nil, []string{"album.x"}},
		// shell_quote is multi-valued: upstream matches its name whole (phototemplate.py:875)
		{"unmatched dotted multi-valued", "{shell_quote.x}", ex(), RenderOptions{}, nil, []string{"shell_quote.x"}},
	}
}

func TestRenderExamples(t *testing.T) {
	t.Parallel()
	checkRender(t, renderCases())
}

func checkRender(t *testing.T, cases []renderCase) {
	t.Helper()
	for _, tt := range cases {
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
// capacity: without slices.Clone of v.Strings in values, [o,0] and & write
// "f00" and "My Photo Title" into keywords; without slices.Clone of
// a.Folders, the title "Album1" lands in folders[1].
func TestRenderKeepsPhotoValues(t *testing.T) {
	t.Parallel()
	const in = "{keyword[o,0]&{title}}/{folder_album}/{title,{nosuch}}"
	want := []string{
		"f00/Folder1/Album1/My Photo Title",
		"bar/Folder1/Album1/My Photo Title",
		"My Photo Title/Folder1/Album1/My Photo Title",
	}
	wantUnmatched := []string{"nosuch"}
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
		{"x{keyword,{title}}", missing, ErrUnsupportedField, "title"},
		{"{id:05d}", missing, ErrUnsupportedConstruct, "id"},
		// variables (phototemplate.py:643-656, 793-837)
		{"{%a}", missing, ErrRender, `"a"`},
		{"{var:a,x}{%b}", missing, ErrRender, `"b"`},
		{"{var:a}", missing, ErrRender, "var"},
		{"{var,x}", missing, ErrRender, "var"},
		{"{var:a,{nosuch}}", missing, ErrRender, "var"},
		{"{keyword[%nope,x]}", missing, ErrRender, `"nope"`},
		{"{keyword[x,%nope]}", missing, ErrRender, `"nope"`},
		{"{%nope+keyword}", missing, ErrRender, `"nope"`},
		{"{var:k,{keyword}}{keyword[o,%k]}", examplePhoto(), ErrRender, "find/replace"},
		{"{var:k,{keyword}}{%k+keyword}", examplePhoto(), ErrRender, "delim"},
		// upstream never ends: every round adds a "-"
		{"{var:v,%v-}{keyword[o,%v]}", missing, ErrRender, "settle"},
		// upstream ends only when memory does: each step doubles or squares the value
		{"{var:v,%v%v}{%v+keyword}", missing, ErrTooManyValues, "bytes"},
		{"{var:a,aaaa}{var:b,{%a[a,%a]}}{var:c,{%b[a,%b]}}{var:d,{%c[a,%c]}}{%d[a,%d]}", missing, ErrTooManyValues, "bytes"},
		{"{var:a,x}" + strings.Repeat("{var:a,{%a}{%a}}", 24), missing, ErrTooManyValues, "bytes"},
		// every reference to a shared value counts
		{"{var:a,x}" + strings.Repeat("{var:a,{%a}{%a}}", 15) + strings.Repeat("{var:a,{%a&{%a}}}", 10), missing, ErrTooManyValues, "bytes"},
		// the condition drops the joined string, so only the join can count it
		{"{var:a,x}" + strings.Repeat("{var:a,{%a&{%a}}}", 13) + "{var:s,x}" + strings.Repeat("{var:s,{%s}{%s}}", 12) + "{%s+%a contains q?Y,N}",
			missing, ErrTooManyValues, "bytes"},
		{"{,+keyword?Y,N}", keywords(slices.Repeat([]string{strings.Repeat("a", 1<<20)}, 20)...), ErrTooManyValues, "bytes"},
		// conditions (phototemplate.py:705-725)
		{"{keyword > x}", missing, ErrRender, ">"},
		{"{id > {nosuch}?Y,N}", conditionPhoto(), ErrRender, ">"},
		{"{var:n,0x1p4}{%n > 15?Y,N}", missing, ErrRender, ">"},
		{"{var:n,0X1P4}{%n > 15?Y,N}", missing, ErrRender, ">"},
		{"{id > x?Y,N}", conditionPhoto(), ErrRender, ">"},
		{"{id > 1|2?Y,N}", conditionPhoto(), ErrRender, ">"},
		{"{id >= {keyword}?Y,N}", with(conditionPhoto(), "keyword", strs("1", "2")), ErrRender, ">="},
		{"{keyword < 9?Y,N}", with(conditionPhoto(), "keyword", strs("x", "5")), ErrRender, "<"},
		{"{var:n,0x10}{%n <= 99?Y,N}", missing, ErrRender, "<="},
		// dates (phototemplate.py:950-956, 1743-1769)
		{"{created.year}", missing, ErrUnsupportedField, "created"},
		{"{modified}", missing, ErrUnsupportedField, "modified"},
		{"{modified.year}", fakePhoto{"modified": {}}, ErrUnsupportedField, "created"},
		{"{created.strftime,%Y}", missing, ErrUnsupportedConstruct, "created.strftime"},
		{"{created.strftime}", missing, ErrUnsupportedConstruct, "created.strftime"},
		{"{created.month}", missing, ErrUnsupportedConstruct, "created.month"},
		{"{modified.mon}", missing, ErrUnsupportedConstruct, "modified.mon"},
		{"{today.dow}", missing, ErrUnsupportedConstruct, "today.dow"},
		{"{created.year.x}", missing, ErrRender, "created.year.x"},
		{"{today.nope}", missing, ErrRender, "today.nope"},
		{"{created.}", missing, ErrRender, "created."},
		{"{today.}", missing, ErrRender, "today."},
		{"{format:int:05d,42}", missing, ErrUnsupportedConstruct, "format"},
		{"{counter}", missing, ErrUnsupportedConstruct, "counter"},
		{"{counter.a}", missing, ErrUnsupportedConstruct, "counter.a"},
		{"{album_seq}", missing, ErrUnsupportedConstruct, "album_seq"},
		{"{folder_album_seq(1)}", missing, ErrUnsupportedConstruct, "folder_album_seq"},
		{"{media_type,video=v}", missing, ErrUnsupportedConstruct, "media_type"},
		{"{photo_or_video,photo=p}", missing, ErrUnsupportedConstruct, "photo_or_video"},
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

// A keyword is personal data, so an error names the operator or the field and not the value.
func TestRenderErrorHidesValues(t *testing.T) {
	t.Parallel()
	for in, keyword := range map[string]string{
		"{keyword < 9?Y,N}":                     "Holidays",
		"{var:k,{keyword}}{keyword[o,%k]}":      "%Holidays",
		"{var:k,{keyword}}{keyword|join(%k)}":   "%Holidays",
		"{keyword|filter(> 1)}":                 "Holidays",
		"{var:k,{keyword}}{keyword|filter(%k)}": "Holidays a",
		"{var:k,{keyword}}{keyword|sslice(%k)}": "Holidays",
		"{keyword|int}":                         "Infinity",
	} {
		_, _, err := render(t, in, with(conditionPhoto(), "keyword", strs(keyword)), RenderOptions{})
		if msg := fmt.Sprint(err); !errors.Is(err, ErrRender) || strings.Contains(msg, "Holidays") || strings.Contains(msg, keyword) {
			t.Errorf("Render(%q) = %v; want ErrRender without the keyword", in, err)
		}
	}
}

// A Render builds at most 16 MiB of strings, whether one title holds them or
// variables keep them: each b holds 64 values of 32 KiB, 2 MiB.
func TestRenderByteBudget(t *testing.T) {
	t.Parallel()
	kept := "{var:c,x}" + strings.Repeat("{var:c,{%c&{%c}}}", 6) + "{var:r,y}" + strings.Repeat("{var:r,{%r}{%r}}", 15)
	for i := range 10 {
		kept += fmt.Sprintf("{var:b%d,{%%c}{%%r}}", i)
	}
	for _, tt := range []struct {
		in    string
		title int
		want  error
	}{
		{"{title}", 1 << 24, nil},
		{"{title}", 1<<24 + 1, ErrTooManyValues},
		{kept, 0, ErrTooManyValues},
	} {
		got, _, err := render(t, tt.in, fakePhoto{"title": strs(strings.Repeat("a", tt.title))}, RenderOptions{})
		if !errors.Is(err, tt.want) || tt.want == nil && (len(got) != 1 || len(got[0]) != tt.title) {
			t.Errorf("Render(%.30q) with a title of %d bytes = %d values, %v; want %v", tt.in, tt.title, len(got), err, tt.want)
		}
	}
}

// A chain of n variables expands in n+1 rounds, and vars.go allows 32.
func TestRenderVariablesSettle(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]error{31: nil, 32: ErrRender} {
		in := "{var:a0,x}"
		for i := 1; i <= n; i++ {
			in += fmt.Sprintf("{var:a%d,%%a%d}", i, i-1)
		}
		in += fmt.Sprintf("{var:p,a-b}{%%p[-,%%a%d]}", n)
		if got, _, err := render(t, in, fakePhoto{}, RenderOptions{}); !errors.Is(err, want) || want == nil && !slices.Equal(got, []string{"axb"}) {
			t.Errorf("Render of a chain of %d variables = %q, %v; want %v", n, got, err, want)
		}
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
		{"nested over the limit", "{title,{keyword}{album}}", 3, 2, 5, false},
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

// conditionPhoto is the photo of the conditions of phototemplate.md:103-111
// and templates.txt (t096-t125).
func conditionPhoto() fakePhoto {
	created := time.Date(2019, 6, 1, 12, 0, 0, 0, time.FixedZone("x", 3*3600))
	id := int64(42)
	return fakePhoto{
		"keyword": strs("Beach", "BeachDay", "travel"),
		"person":  strs("travel", "Beach", "BeachDay"),
		"album":   strs("Holidays", "travel"),
		"uuid":    strs("1A000000-0000-4000-8000-00000000000B"),
		"title":   strs("Beach"),
		"descr":   {},
		"created": {Time: &created},
		"id":      {Int: &id},
	}
}

func TestRenderConditions(t *testing.T) {
	t.Parallel()
	c, o := conditionPhoto, RenderOptions{}
	yes, no := []string{"Y"}, []string{"N"}
	checkRender(t, []renderCase{
		// phototemplate.md:103-111
		{"md103", "{keyword matches Beach}", c(), o, []string{"True"}, nil},
		{"matches is equality", "{keyword matches Beac}", c(), o, []string{"_"}, nil},
		{"md104", "{keyword contains Beach}", c(), o, []string{"True"}, nil},
		{"md106", "{keyword|lower contains beach}", c(), o, []string{"True"}, nil},
		{"md111", "{keyword|lower matches travel|vacation?T,F}", c(), o, []string{"T"}, nil},
		{"no alternative matches", "{keyword matches vacation|holiday?T,F}", c(), o, []string{"F"}, nil},
		{"contains an alternative", "{keyword contains Sea|Day?Y,N}", c(), o, yes, nil},
		// t102-t109
		{"t102", "{uuid contains A?Y,N}", c(), o, yes, nil},
		{"t103", "{uuid not contains A?Y,N}", c(), o, no, nil},
		{"contains nothing", "{uuid contains Z?Y,N}", c(), o, no, nil},
		{"not contains nothing", "{uuid not contains Z?Y,N}", c(), o, yes, nil},
		{"t104", "{uuid startswith 1A?Y,N}", c(), o, yes, nil},
		{"startswith the end", "{uuid startswith B?Y,N}", c(), o, no, nil},
		{"t105", "{uuid not startswith 1?Y,N}", c(), o, no, nil},
		{"t106", "{uuid endswith B?Y,N}", c(), o, yes, nil},
		{"endswith the start", "{uuid endswith 1A?Y,N}", c(), o, no, nil},
		{"t107", "{uuid not endswith B?Y,N}", c(), o, no, nil},
		{"t108", "{keyword matches Beach?Y,N}", c(), o, yes, nil},
		{"t109", "{keyword not matches Beach?Y,N}", c(), o, no, nil},
		// t110-t121 with a photo of 2019
		{"<= below", "{created.year <= 2020?Y,N}", c(), o, yes, nil},
		{"<= equal", "{created.year <= 2019?Y,N}", c(), o, yes, nil},
		{"<= above", "{created.year <= 2018?Y,N}", c(), o, no, nil},
		{"not <=", "{created.year not <= 2019?Y,N}", c(), o, no, nil},
		{">= equal", "{created.year >= 2019?Y,N}", c(), o, yes, nil},
		{">= below", "{created.year >= 2020?Y,N}", c(), o, no, nil},
		{"not >=", "{created.year not >= 2020?Y,N}", c(), o, yes, nil},
		{"< equal", "{created.year < 2019?Y,N}", c(), o, no, nil},
		{"< above", "{created.year < 2020?Y,N}", c(), o, yes, nil},
		{"not <", "{created.year not < 2020?Y,N}", c(), o, no, nil},
		{"> equal", "{created.year > 2019?Y,N}", c(), o, no, nil},
		{"> below", "{created.year > 2018?Y,N}", c(), o, yes, nil},
		{"not >", "{created.year not > 2019?Y,N}", c(), o, yes, nil},
		{"== equal", "{created.year == 2019?Y,N}", c(), o, yes, nil},
		{"== compares strings", "{created.year == 2019.0?Y,N}", c(), o, no, nil},
		{"not ==", "{created.year not == 2019?Y,N}", c(), o, no, nil},
		{"!= equal", "{created.year != 2019?Y,N}", c(), o, no, nil},
		{"!= other", "{created.year != 2020?Y,N}", c(), o, yes, nil},
		{"not !=", "{created.year not != 2020?Y,N}", c(), o, no, nil},
		// t122, t124, t125
		{"t122", "{id > 100?Y,N}", c(), o, no, nil},
		{"id above a decimal", "{id > 41.5?Y,N}", c(), o, yes, nil},
		{"t124", "{keyword contains {title}?Y,N}", c(), o, yes, nil},
		{"t125", "{album contains {keyword}?Y,N}", c(), o, yes, nil},
		// == and != compare sorted lists and keep "|" (phototemplate.py:742-755)
		{"== keeps the pipe", "{keyword == Beach|BeachDay|travel?Y,N}", c(), o, no, nil},
		{"== a|b", "{keyword == a|b}", c(), o, []string{"_"}, nil},
		{"== sorted lists", "{keyword == {person}?Y,N}", c(), o, yes, nil},
		{"!= sorted lists", "{keyword != {person}?Y,N}", c(), o, no, nil},
		{"== other lists", "{keyword == {album}?Y,N}", c(), o, no, nil},
		// any value passes; the ones after it are not parsed (phototemplate.py:712-716)
		{"> any value", "{keyword > 3?Y,N}", with(c(), "keyword", strs("1", "5", "x")), o, yes, nil},
		{"> without values", "{descr > x?Y,N}", c(), o, no, nil},
		{"not > without values", "{descr not > x?Y,N}", c(), o, yes, nil},
		// float() of Python
		{"float of spaces", "{var:n, 5 }{%n > 4?Y,N}", c(), o, yes, nil},
		{"float of an exponent", "{var:n,1e3}{%n >= 1000?Y,N}", c(), o, yes, nil},
		{"float of underscores", "{var:n,1_000}{%n > 999?Y,N}", c(), o, yes, nil},
		{"float of an overflow", "{var:n,1e400}{%n > 1e308?Y,N}", c(), o, yes, nil},
		{"float of inf", "{var:n,-inf}{%n < -1e308?Y,N}", c(), o, yes, nil},
		{"float of nan", "{var:n,nan}{%n >= 0?Y,N}", c(), o, no, nil},
		{"float of U+001C", "{var:n,\U0000001c5}{%n > 4?Y,N}", c(), o, yes, nil},
		// after [find,replace], before & (phototemplate.py:685, 765)
		{"after find and replace", "{title[Beach,Sea] matches Sea?Y,N}", c(), o, yes, nil},
		// NON_TEMPLATE_STRING does not stop at "&" (phototemplate.tx:36-38): the value is "Nope&Beach", not a combine
		{"& inside the value", "{title contains Nope&{title}}", c(), o, []string{"_"}, nil},
		{"unmatched in the value", "{title contains {nosuch}?Y,N}", c(), o, no, []string{"nosuch"}},
	})
}

func TestRenderVariables(t *testing.T) {
	t.Parallel()
	ex, o, none := examplePhoto, RenderOptions{}, Value{}
	created := time.Date(2019, 6, 1, 12, 0, 0, 0, time.FixedZone("x", 3*3600))
	checkRender(t, []renderCase{
		// phototemplate.md:155-163
		{"md159 year", "{var:year,{created.year}}{original_name}-{%year}", with(ex(), "created", Value{Time: &created}), o,
			[]string{"IMG_0001-2019"}, nil},
		{"md159 person", "{var:name,Katie}{person contains {%name}?{%name},Not-{%name}}", with(ex(), "person", strs("Katie")), o,
			[]string{"Katie"}, nil},
		{"md159 no person", "{var:name,Katie}{person contains {%name}?{%name},Not-{%name}}", ex(), o, []string{"Not-Katie"}, nil},
		{"md155", "{var:pipe,{pipe}}{title[-,%pipe]}", with(ex(), "title", strs("a-b")), o, []string{"a|b"}, nil},
		{"md163 escaped percent", "{title[:,%%]}", with(ex(), "title", strs("a:b")), o, []string{"a%b"}, nil},
		{"md163 Foo", "{title contains Foo?{title}{percent},{title}}", with(ex(), "title", strs("Foo")), o, []string{"Foo%"}, nil},
		{"md163 no Foo", "{title contains Foo?{title}{percent},{title}}", ex(), o, []string{"My Photo Title"}, nil},
		// assignment (phototemplate.py:648-656, 770)
		{"assignment alone", "{var:a,b}", ex(), o, []string{""}, nil},
		{"assignment in text", "x{var:a,b}y", ex(), o, []string{"xy"}, nil},
		{"empty value dropped", "{var:a,}{%a}", ex(), o, []string{"_"}, nil},
		{"empty values dropped", "{var:a,{keyword}}{%a}", with(ex(), "keyword", strs("", "x", "")), o, []string{"x"}, nil},
		{"redefined", "{var:a,x}{var:a,y}{%a}", ex(), o, []string{"y"}, nil},
		{"multi-valued", "{var:a,{keyword}}{%a}", ex(), o, []string{"foo", "bar"}, nil},
		{"value kept after a change", "{var:p,a-b}{%p[-,x]}{%p}", ex(), o, []string{"axba-b"}, nil},
		{"bool value of an assignment", "{var:a?x,b}{%a}", ex(), o, []string{"bb"}, nil},
		// expand_variables (phototemplate.py:807-837)
		{"delim", "{var:d,-}{%d+keyword}", ex(), o, []string{"foo-bar"}, nil},
		{"replace", "{var:d,-}{var:l,a;b}{%l[;,%d]}", ex(), o, []string{"a-b"}, nil},
		{"find", "{var:f,o}{keyword[%f,0]}", ex(), o, []string{"f00", "bar"}, nil},
		{"t402 middle of the text", "{var:v,X}{var:p,a-b}{%p[-,a%v]}", ex(), o, []string{"aa%vb"}, nil},
		{"escaped percent before", "{var:v,X}{var:p,a-b}{%p[-,%%%v]}", ex(), o, []string{"a%Xb"}, nil},
		{"last pair kept", "{var:v,X}{var:p,a-b}{%p[-,%%%%%v]}", ex(), o, []string{"a%Xb"}, nil},
		{"escaped variable", "{var:v,X}{var:p,a-b}{%p[-,%%v]}", ex(), o, []string{"a%vb"}, nil},
		{"every occurrence", "{var:v,X}{var:p,a-b}{%p[-,%v%vw]}", ex(), o, []string{"aXXwb"}, nil},
		{"only the first name", "{var:v,X}{var:vw,Y}{var:p,a-b}{%p[-,%vw%v]}", ex(), o, []string{"aY%vb"}, nil},
		{"variable in a variable", "{var:v,X}{var:w,%v}{var:p,a-b}{%p[-,%w]}", ex(), o, []string{"aXb"}, nil},
		{"self reference settles", "{var:v,%v}{var:p,a-b}{%p[-,%v]}", ex(), o, []string{"a%vb"}, nil},
		{"non-ASCII name", "{var:\U000000e9,X}{var:p,a-b}{%p[-,%\U000000e9]}", ex(), o, []string{"aXb"}, nil},
		// upstream re.sub reads the backslashes of the value (phototemplate.py:826), Go's ReplaceAllString its "$"
		{"value is literal", `{var:v,\d$1$v}{var:p,a-b}{%p[-,%v]}`, ex(), o, []string{`a\d$1$vb`}, nil},
		{"not expanded without values", "{title[%nope,x]}", with(ex(), "title", none), o, []string{"_"}, nil},
	})
}

func TestRenderVariablesPerRender(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	for _, tt := range []struct {
		in   string
		want error
	}{{"{var:a,x}{%a}", nil}, {"{%a}", ErrRender}} {
		tmpl, err := Parse(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.Render(tmpl, fakePhoto{}, RenderOptions{}); !errors.Is(err, tt.want) {
			t.Errorf("Render(%q) = %v, want %v", tt.in, err, tt.want)
		}
	}
}

// The values of an expansion are bounded as the strings of a statement are.
func TestRenderExpandMaxResults(t *testing.T) {
	t.Parallel()
	const in = "{var:v,{album}}{var:k,{keyword}}{title[My,%k]}"
	p := with(with(examplePhoto(), "keyword", strs("%v", "w")), "album", strs("1", "2"))
	for limit, want := range map[int]error{3: ErrTooManyValues, 4: ErrRender} {
		if got, _, err := render(t, in, p, RenderOptions{MaxResults: limit}); !errors.Is(err, want) {
			t.Errorf("Render(%q) with at most %d = %q, %v; want %v", in, limit, got, err, want)
		}
	}
}

func TestRenderDates(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("x", 3*3600)
	created := time.Date(2020, 2, 4, 19, 7, 38, 0, zone)
	modified := time.Date(2021, 11, 25, 6, 5, 4, 0, zone)
	old := time.Date(999, 1, 2, 3, 4, 5, 0, zone)
	today := RenderOptions{Today: time.Date(2001, 1, 1, 0, 0, 0, 0, zone)}
	p := func(modified *time.Time) fakePhoto {
		return fakePhoto{"created": {Time: &created}, "modified": {Time: modified}, "edited": strs("edited"),
			"original_name": strs("IMG_0001"), "title": strs("My Photo Title")}
	}
	o := RenderOptions{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"t029", "{created}", p(nil), o, one("2020-02-04"), nil},
		{"t030", "{created.date}", p(nil), o, one("2020-02-04"), nil},
		{"t031", "{created.year}", p(nil), o, one("2020"), nil},
		{"t032", "{created.yy}", p(nil), o, one("20"), nil},
		{"t033", "{created.mm}", p(nil), o, one("02"), nil},
		{"t036", "{created.dd}", p(nil), o, one("04"), nil},
		{"t038", "{created.doy}", p(nil), o, one("035"), nil},
		{"t039 in the zone of the photo", "{created.hour}", p(nil), o, one("19"), nil},
		{"t040", "{created.min}", p(nil), o, one("07"), nil},
		{"t041", "{created.sec}", p(nil), o, one("38"), nil},
		{"t043 without modified", "{modified}", p(nil), o, one("2020-02-04"), nil},
		{"t045 without modified", "{modified.year}", p(nil), o, one("2020"), nil},
		{"t043", "{modified}", p(&modified), o, one("2021-11-25"), nil},
		{"t044", "{modified.date}", p(&modified), o, one("2021-11-25"), nil},
		{"t045", "{modified.year}", p(&modified), o, one("2021"), nil},
		{"t046", "{modified.yy}", p(&modified), o, one("21"), nil},
		{"t047", "{modified.mm}", p(&modified), o, one("11"), nil},
		{"t050", "{modified.dd}", p(&modified), o, one("25"), nil},
		{"t052", "{modified.doy}", p(&modified), o, one("329"), nil},
		{"t053", "{modified.hour}", p(&modified), o, one("06"), nil},
		{"t054", "{modified.min}", p(&modified), o, one("05"), nil},
		{"t055", "{modified.sec}", p(&modified), o, one("04"), nil},
		{"t068", "{created.year}-{created.mm}-{created.dd}", p(nil), o, one("2020-02-04"), nil},
		{"t331 md:151", "{created.year}/{openbrace}{title}{closebrace}", p(nil), o, one("2020/{My Photo Title}"), nil},
		{"t178 edited", "{edited?{created.year},{original_name}}", p(nil), o, one("2020"), nil},
		{"t178", "{edited?{created.year},{original_name}}", with(p(nil), "edited", Value{}), o, one("IMG_0001"), nil},
		// f"{dt.year}" is not padded, date().isoformat() is (datetime_formatter.py:14-22)
		{"year before 1000", "{created.year}", with(p(nil), "created", Value{Time: &old}), o, one("999"), nil},
		{"date before 1000", "{created}", with(p(nil), "created", Value{Time: &old}), o, one("0999-01-02"), nil},
		{"yy before 1000", "{created.yy}", with(p(nil), "created", Value{Time: &old}), o, one("99"), nil},
		{"doy in January", "{created.doy}", with(p(nil), "created", Value{Time: &old}), o, one("002"), nil},
		{"created without a time", "{created}", fakePhoto{"created": {}}, o, one("_"), nil},
		{"today", "{today}", fakePhoto{}, today, one("2001-01-01"), nil},
		{"today.year", "{today.year}", fakePhoto{}, today, one("2001"), nil},
		{"today.doy", "{today.doy}", fakePhoto{}, today, one("001"), nil},
		{"today and created", "{today.yy}-{created.yy}", p(nil), today, one("01-20"), nil},
	})
}

// Without RenderOptions.Today the time is the clock's, so only its form is checked.
func TestRenderTodayNow(t *testing.T) {
	t.Parallel()
	got, _, err := render(t, "{today}/{today.sec}", fakePhoto{}, RenderOptions{})
	if err != nil || len(got) != 1 || strings.HasPrefix(got[0], "0001-") || !regexp.MustCompile(`^\d{4}-\d\d-\d\d/\d\d$`).MatchString(got[0]) {
		t.Errorf("Render({today}/{today.sec}) = %q, %v; want one string of the form 2006-01-02/05, not of the zero time", got, err)
	}
}

func TestRenderRace(t *testing.T) {
	t.Parallel()
	created := time.Date(2020, 2, 4, 19, 7, 38, 0, time.FixedZone("x", 3*3600))
	p := with(examplePhoto(), "created", Value{Time: &created})
	var templates []*Template
	for _, in := range []string{
		"{var:a,{keyword}}{%a[o,0]}-{created.year}",
		"{keyword contains foo?{title},none}",
		"{title}/{folder_album}/{today.year}",
		"{var:d,-}{%d+keyword}",
		"{keyword|lower|upper|titlecase|capitalize}",
		"{keyword|sslice(::-1)|filter(not contains o)|chop(1)|slice(::-1)}",
	} {
		tmpl, err := Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		templates = append(templates, tmpl)
	}
	r, o := NewRenderer(), RenderOptions{Today: created}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 100 {
				if _, _, err := r.Render(templates[(g+i)%len(templates)], p, o); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
