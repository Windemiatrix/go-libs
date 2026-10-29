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

func cond(op, value string) *Conditional {
	return &Conditional{Operator: op, Value: *stmt(lit(value))}
}

type nesting struct {
	open string
	wrap func(*Template) *Field
}

var nestings = []nesting{
	{"{a,", func(t *Template) *Field { return &Field{Name: "a", Default: t} }},
	{"{a&", func(t *Template) *Field { return &Field{Name: "a", Combine: t} }},
	{"{a?", func(t *Template) *Field { return &Field{Name: "a", Bool: t} }},
	{"{a contains ", func(t *Template) *Field {
		return &Field{Name: "a", Conditional: &Conditional{Operator: "contains", Value: *t}}
	}},
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
		{"{keyword|capitalize|parens}", stmt(braced(&Field{Name: "keyword", Filters: []string{"capitalize", "parens"}}))},
		{"{folder_album(>)}", stmt(braced(&Field{Name: "folder_album", Arg: ptr(">")}))},
		{"{folder_album()}", stmt(braced(&Field{Name: "folder_album", Arg: ptr("")}))},
		{"{album[/,-]}", stmt(braced(&Field{Name: "album", FindReplace: []Pair{{"/", "-"}}}))},
		{"{album[/,-|:,-]}", stmt(braced(&Field{Name: "album", FindReplace: []Pair{{"/", "-"}, {":", "-"}}}))},
		{"{keyword matches Beach}", stmt(braced(&Field{Name: "keyword", Conditional: cond("matches", "Beach")}))},
		{"{keyword contains Beach}", stmt(braced(&Field{Name: "keyword", Conditional: cond("contains", "Beach")}))},
		{"{photo.score.overall > 0.7}", stmt(braced(&Field{Name: "photo.score.overall", Conditional: cond(">", "0.7")}))},
		{"{keyword|lower contains beach}", stmt(braced(&Field{Name: "keyword", Filters: []string{"lower"}, Conditional: cond("contains", "beach")}))},
		{"{keyword|lower not contains beach}", stmt(braced(&Field{
			Name:        "keyword",
			Filters:     []string{"lower"},
			Conditional: &Conditional{Negate: true, Operator: "contains", Value: *stmt(lit("beach"))},
		}))},
		{"{keyword|lower matches travel|vacation?Travel-Photos,Not-Travel-Photos}", stmt(braced(&Field{
			Name:        "keyword",
			Filters:     []string{"lower"},
			Conditional: cond("matches", "travel|vacation"),
			Bool:        stmt(lit("Travel-Photos")),
			Default:     stmt(lit("Not-Travel-Photos")),
		}))},
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
		{"{var:pipe,{pipe}}{title[-,%pipe]}", stmt(
			braced(&Field{Name: "var", Subfield: "pipe", Default: stmt(braced(named("pipe")))}),
			braced(&Field{Name: "title", FindReplace: []Pair{{"-", "%pipe"}}}),
		)},
		{"{var:myvar,{template1&{template2,},}}{%myvar|uniq}", stmt(
			braced(&Field{Name: "var", Subfield: "myvar", Default: stmt(braced(&Field{
				Name:    "template1",
				Combine: stmt(braced(&Field{Name: "template2", Default: stmt()})),
				Default: stmt(),
			}))}),
			braced(&Field{Name: "%myvar", Filters: []string{"uniq"}}),
		)},
		{"{var:year,{created.year}}{original_name}-{%year}", stmt(
			braced(&Field{Name: "var", Subfield: "year", Default: stmt(braced(named("created.year")))}),
			Segment{Field: named("original_name"), Post: "-"},
			braced(named("%year")),
		)},
		{"{var:name,Katie}{person contains {%name}?{%name},Not-{%name}}", stmt(
			braced(&Field{Name: "var", Subfield: "name", Default: stmt(lit("Katie"))}),
			braced(&Field{
				Name:        "person",
				Conditional: &Conditional{Operator: "contains", Value: *stmt(braced(named("%name")))},
				Bool:        stmt(braced(named("%name"))),
				Default:     stmt(Segment{Pre: "Not-", Field: named("%name")}),
			}),
		)},
		{"{title[:,%%]}", stmt(braced(&Field{Name: "title", FindReplace: []Pair{{":", "%%"}}}))},
		{"{title contains Foo?{title}{percent},{title}}", stmt(braced(&Field{
			Name:        "title",
			Conditional: cond("contains", "Foo"),
			Bool:        stmt(braced(named("title")), braced(named("percent"))),
			Default:     stmt(braced(named("title"))),
		}))},
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
		{"{photo.original_filename|path.stem}", stmt(braced(&Field{Name: "photo.original_filename", Filters: []string{"path.stem"}}))},
		{"{folder_album|filter(contains Events)}", stmt(braced(&Field{Name: "folder_album", Filters: []string{"filter(contains Events)"}}))},

		{"{keyword == a|b}", stmt(braced(&Field{Name: "keyword", Conditional: cond("==", "a|b")}))},
		{"{title contains }", stmt(braced(&Field{Name: "title", Conditional: &Conditional{Operator: "contains"}}))},
		{"{title  contains   x}", stmt(braced(&Field{Name: "title", Conditional: cond("contains", "x")}))},
		{"{a < =1}", stmt(braced(&Field{Name: "a", Conditional: cond("<", "=1")}))},
		{"{a contains x&y}", stmt(braced(&Field{Name: "a", Conditional: cond("contains", "x&y")}))},
		{"{a}x{b}", stmt(Segment{Field: named("a"), Post: "x"}, braced(named("b")))},
		{"a{b}", stmt(Segment{Pre: "a", Field: named("b")})},
		{"{title|}", stmt(braced(named("title")))},
		{"{title|[a,b]}", stmt(braced(&Field{Name: "title", FindReplace: []Pair{{"a", "b"}}}))},
		{"{title|chop(1, ?|{})}", stmt(braced(&Field{Name: "title", Filters: []string{"chop(1, ?|{})"}}))},
		{"{a|b.c:d/e}", stmt(braced(&Field{Name: "a", Filters: []string{"b.c:d/e"}}))},
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
		{"{t[a,b,c]}", stmt(braced(&Field{Name: "t", FindReplace: []Pair{{"a,b", "c"}}}))},
		{"{t[,x]}", stmt(braced(&Field{Name: "t", FindReplace: []Pair{{"", "x"}}}))},
		{"{t[a,]}", stmt(braced(&Field{Name: "t", FindReplace: []Pair{{"a", ""}}}))},
		{"{t[]}", stmt(braced(&Field{Name: "t", FindReplace: []Pair{}}))},
		{"{t[a,{x}]}", stmt(braced(&Field{Name: "t", FindReplace: []Pair{{"a", "{x}"}}}))},
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
		{"{,+a:b(c)|d[e,f] not == g?h,i}", stmt(braced(&Field{
			Delim:       ptr(","),
			Name:        "a",
			Subfield:    "b",
			Arg:         ptr("c"),
			Filters:     []string{"d"},
			FindReplace: []Pair{{"e", "f"}},
			Conditional: &Conditional{Negate: true, Operator: "==", Value: *stmt(lit("g"))},
			Bool:        stmt(lit("h")),
			Default:     stmt(lit("i")),
		}))},
		{"{; +name:sub(arg)|f1|f2[a,b|c,d]&x?y,z}", stmt(braced(&Field{
			Delim:       ptr("; "),
			Name:        "name",
			Subfield:    "sub",
			Arg:         ptr("arg"),
			Filters:     []string{"f1", "f2"},
			FindReplace: []Pair{{"a", "b"}, {"c", "d"}},
			Combine:     stmt(lit("x")),
			Bool:        stmt(lit("y")),
			Default:     stmt(lit("z")),
		}))},
		{"{; +name:sub(arg)|f1|f2[a,b|c,d] not contains v?y,z}", stmt(braced(&Field{
			Delim:       ptr("; "),
			Name:        "name",
			Subfield:    "sub",
			Arg:         ptr("arg"),
			Filters:     []string{"f1", "f2"},
			FindReplace: []Pair{{"a", "b"}, {"c", "d"}},
			Conditional: &Conditional{Negate: true, Operator: "contains", Value: *stmt(lit("v"))},
			Bool:        stmt(lit("y")),
			Default:     stmt(lit("z")),
		}))},
	}
	for _, k := range nestings {
		cases = append(cases, parseCase{nested(31, k), nestedWant(31, k)}, parseCase{nested(32, k), nestedWant(32, k)})
	}
	for _, op := range []string{"contains", "matches", "startswith", "endswith", "<=", ">=", "<", ">", "==", "!="} {
		cases = append(cases,
			parseCase{"{a " + op + " 1}", stmt(braced(&Field{Name: "a", Conditional: cond(op, "1")}))},
			parseCase{"{a not " + op + " 1}", stmt(braced(&Field{
				Name:        "a",
				Conditional: &Conditional{Negate: true, Operator: op, Value: *stmt(lit("1"))},
			}))},
		)
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
		{"{title }", 7, "an operator"},
		{"{title|lower|}", 13, "a filter"},
		{"{title|a b}", 9, "an operator"},
		{"{title|chop(1}", 14, `")"`},
		{"{title[a]}", 8, `","`},
		{"{t[a,b|]}", 7, `","`},
		{"{t[a,b}", 7, `"]"`},
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
		{"{title contains}", 15, `" "`},
		{"{a notcontains b}", 3, "an operator"},
		{"{a not  contains b}", 7, "an operator"},
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
