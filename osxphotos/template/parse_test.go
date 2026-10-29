package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type parseCase struct {
	in   string
	want *Template
}

func stmt(segs ...Segment) *Template { return &Template{Segments: segs} }

func lit(s string) Segment { return Segment{Pre: s} }

func braced(f *Field) Segment { return Segment{Field: f} }

func named(name string) *Field { return &Field{Name: name} }

type nesting struct {
	open string
	wrap func(*Template) *Field
}

var nestings = []nesting{
	{"{a,", func(t *Template) *Field { return &Field{Name: "a", Default: t} }},
	{"{a&", func(t *Template) *Field { return &Field{Name: "a", Combine: t} }},
	{"{a?", func(t *Template) *Field { return &Field{Name: "a", Bool: t} }},
}

func nested(n int, k nesting) string {
	return strings.Repeat(k.open, n-1) + "{a}" + strings.Repeat("}", n-1)
}

func nestedWant(n int, k nesting) *Template {
	f := named("a")
	for range n - 1 {
		f = k.wrap(stmt(braced(f)))
	}
	return stmt(braced(f))
}

// The examples of phototemplate.md and phototemplate.py, then the readings of
// phototemplate.tx where the grammar is easy to misread.
func parseCases() []parseCase {
	cases := []parseCase{
		{"", &Template{}},
		{"Holidays 2024/05", stmt(lit("Holidays 2024/05"))},
		{"{title}", stmt(braced(named("title")))},
		{"The title of the photo is {title}", stmt(Segment{Pre: "The title of the photo is ", Field: named("title")})},
		{"{keyword}", stmt(braced(named("keyword")))},
		{"{,+keyword}", stmt(braced(&Field{Delim: ptr(","), Name: "keyword"}))},
		{"{; +keyword}", stmt(braced(&Field{Delim: ptr("; "), Name: "keyword"}))},
		{"{+keyword}", stmt(braced(&Field{Delim: ptr(""), Name: "keyword"}))},
		{"{folder_album(>)}", stmt(braced(&Field{Name: "folder_album", Arg: ptr(">")}))},
		{"{folder_album()}", stmt(braced(&Field{Name: "folder_album", Arg: ptr("")}))},
		{"{favorite?Favorite-{original_name},{original_name}}", stmt(braced(&Field{
			Name:    "favorite",
			Bool:    stmt(Segment{Pre: "Favorite-", Field: named("original_name")}),
			Default: stmt(braced(named("original_name"))),
		}))},
		{"{created.year&{folder_album,}}", stmt(braced(&Field{
			Name:    "created.year",
			Combine: stmt(braced(&Field{Name: "folder_album", Default: stmt()})),
		}))},
		{"{template1&{template2&{template3,},},}", stmt(braced(&Field{
			Name: "template1",
			Combine: stmt(braced(&Field{
				Name:    "template2",
				Combine: stmt(braced(&Field{Name: "template3", Default: stmt()})),
				Default: stmt(),
			})),
			Default: stmt(),
		}))},
		{"{hdr?ISHDR,NOTHDR}", stmt(braced(&Field{Name: "hdr", Bool: stmt(lit("ISHDR")), Default: stmt(lit("NOTHDR"))}))},
		{"{title,I have no title}", stmt(braced(&Field{Name: "title", Default: stmt(lit("I have no title"))}))},
		{"{created.strftime,%Y-%m-%d-%H%M%S}", stmt(braced(&Field{Name: "created.strftime", Default: stmt(lit("%Y-%m-%d-%H%M%S"))}))},
		{"{media_type,video=vidéo;time_lapse=vidéo_accélérée}", stmt(braced(&Field{
			Name:    "media_type",
			Default: stmt(lit("video=vidéo;time_lapse=vidéo_accélérée")),
		}))},
		{"{created.year}/{openbrace}{title}{closebrace}", stmt(
			Segment{Field: named("created.year"), Post: "/"},
			braced(named("openbrace")), braced(named("title")), braced(named("closebrace")),
		)},
		{"{var:year,{created.year}}{original_name}-{%year}", stmt(
			braced(&Field{Name: "var", Subfield: "year", Default: stmt(braced(named("created.year")))}),
			Segment{Field: named("original_name"), Post: "-"},
			braced(named("%year")),
		)},
		{"{counter:05d(1)}", stmt(braced(&Field{Name: "counter", Subfield: "05d", Arg: ptr("1")}))},
		{"{counter(1,11,2)}", stmt(braced(&Field{Name: "counter", Arg: ptr("1,11,2")}))},
		{"{counter(,,2)}", stmt(braced(&Field{Name: "counter", Arg: ptr(",,2")}))},
		{"{album_seq:05d(1)}", stmt(braced(&Field{Name: "album_seq", Subfield: "05d", Arg: ptr("1")}))},
		{"{format:float:.1f,{exiftool:EXIF:FocalLength}}", stmt(braced(&Field{
			Name:     "format",
			Subfield: "float:.1f",
			Default:  stmt(braced(&Field{Name: "exiftool", Subfield: "EXIF:FocalLength"})),
		}))},
		{"{exiftool:EXIF:Make}", stmt(braced(&Field{Name: "exiftool", Subfield: "EXIF:Make"}))},
		{"{function:file.py::function_name}", stmt(braced(&Field{Name: "function", Subfield: "file.py::function_name"}))},
		{"{detected_text:0.5}", stmt(braced(&Field{Name: "detected_text", Subfield: "0.5"}))},

		{"{a}x{b}", stmt(Segment{Field: named("a"), Post: "x"}, braced(named("b")))},
		{"a{b}", stmt(Segment{Pre: "a", Field: named("b")})},
		{"{title,a+b}", stmt(braced(&Field{Delim: ptr("title,a"), Name: "b"}))},
		{"{a+b+c}", stmt(braced(&Field{Delim: ptr("a+b"), Name: "c"}))},
		{"{created.strftime,%Y+%m}", stmt(braced(&Field{Delim: ptr("created.strftime,%Y"), Name: "%m"}))},
		{"{a,+b}", stmt(braced(&Field{Delim: ptr("a,"), Name: "b"}))},
		{"{title,x+}", stmt(braced(&Field{Name: "title", Default: stmt(lit("x+"))}))},
		{"{a%b}", stmt(braced(named("a%b")))},
		{"{x\U000000b2\U00002167}", stmt(braced(named("x\U000000b2\U00002167")))},
		{`{a:b\ c}`, stmt(braced(&Field{Name: "a", Subfield: `b\ c`}))},
		{"{a:b\\\tc}", stmt(braced(&Field{Name: "a", Subfield: "b\\\tc"}))},
		{"{a:b\\\U0000001cc}", stmt(braced(&Field{Name: "a", Subfield: "b\\\U0000001cc"}))},
		{"{a:./-~'\"%@#^\U00002019}", stmt(braced(&Field{Name: "a", Subfield: "./-~'\"%@#^\U00002019"}))},
		{"{title?A}", stmt(braced(&Field{Name: "title", Bool: stmt(lit("A"))}))},
		{"{title,}", stmt(braced(&Field{Name: "title", Default: stmt()}))},
		{"{a?x&y,z}", stmt(braced(&Field{Name: "a", Bool: stmt(lit("x&y")), Default: stmt(lit("z"))}))},
		{"{a&}", stmt(braced(&Field{Name: "a", Combine: stmt()}))},
		{"{+a:b(c)&x?y,z}", stmt(braced(&Field{
			Delim:    ptr(""),
			Name:     "a",
			Subfield: "b",
			Arg:      ptr("c"),
			Combine:  stmt(lit("x")),
			Bool:     stmt(lit("y")),
			Default:  stmt(lit("z")),
		}))},
	}
	for _, k := range nestings {
		cases = append(cases, parseCase{nested(31, k), nestedWant(31, k)}, parseCase{nested(32, k), nestedWant(32, k)})
	}
	return cases
}

func dump(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func TestParseExamples(t *testing.T) {
	t.Parallel()

	for _, tt := range parseCases() {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.in)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %s, %v; want %s", tt.in, dump(got), err, dump(tt.want))
			}
		})
	}
}

func TestParseCutField(t *testing.T) {
	t.Parallel()

	for _, tt := range parseCases() {
		if s := tt.want.Segments; len(s) != 1 || s[0].Field == nil || s[0].Pre+s[0].Post != "" {
			continue
		}
		for k := 1; k < len(tt.in); k++ {
			if _, err := Parse(tt.in[:k]); !errors.Is(err, ErrSyntax) {
				t.Errorf("Parse(%q) = _, %v; want an error that wraps ErrSyntax", tt.in[:k], err)
			}
		}
	}
}

type parseError struct {
	in     string
	offset int
	want   string
}

func parseErrors() []parseError {
	const depth = "at most 32 nested statements"
	errs := []parseError{
		{"a,b", 1, "end of input"},
		{"a?b", 1, "end of input"},
		{"a}b", 1, "end of input"},
		{"{title", 6, `"}"`},
		{"{title}}", 7, "end of input"},
		{"{}", 1, "a field name"},
		{"{ title}", 1, "a field name"},
		{"{title(}", 7, `")"`},
		{"{title(a(b)}", 8, `")"`},
		{"{a(x{y)}", 4, `")"`},
		{"{title:}", 7, "a subfield"},
		{"{a:b", 4, `"}"`},
		{"{a:b\\", 4, `"}"`},
		{"{a,b,c}", 4, `"}"`},
		{"{a?x?y}", 4, `"}"`},
		{"{a%}", 3, "a field name"},
		{"{%}", 2, "a field name"},
		{"{a%%b}", 3, "a field name"},
		{"{title+}", 6, `"}"`},
		{"{a+", 2, `"}"`},
		{"{+}", 2, "a field name"},
		{"{a+b{c}}", 4, `"}"`},
		{`{f:\ a}`, 3, "a subfield"},
		{`{a:b\ \ c}`, 6, `"}"`},
		{"{e\U00000301}", 2, `"}"`},
		{"{a&{b}", 6, `"}"`},
		{nested(100, nestings[0]), 32 * len(nestings[0].open), depth},
	}
	for _, k := range nestings {
		errs = append(errs, parseError{nested(33, k), 32 * len(k.open), depth})
	}
	return errs
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range parseErrors() {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.in)
			want := fmt.Sprintf("template: syntax error at %d: expected %s", tt.offset, tt.want)
			if got != nil || !errors.Is(err, ErrSyntax) || err.Error() != want {
				t.Errorf("Parse(%q) = %s, %v; want the error %q", tt.in, dump(got), err, want)
			}
		})
	}
}
