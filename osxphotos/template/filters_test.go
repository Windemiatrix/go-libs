package template

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func keywords(vals ...string) fakePhoto { return fakePhoto{"keyword": strs(vals...), "descr": {}} }

func TestFilters(t *testing.T) {
	t.Parallel()
	k := func() fakePhoto { return keywords("Foo", "bar", "Foo", "baz") }
	o, none := RenderOptions{}, fakePhoto{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		// phototemplate.py:1080-1097
		{"lower", "{keyword|lower}", k(), o, []string{"foo", "bar", "foo", "baz"}, nil},
		{"upper", "{keyword|upper}", k(), o, []string{"FOO", "BAR", "FOO", "BAZ"}, nil},
		{"lower ignores its arguments", "{keyword|lower(x)}", k(), o, []string{"foo", "bar", "foo", "baz"}, nil},
		{"t266 capitalize", "{var:t,hello wORLD}{%t|capitalize}", none, o, one("Hello world"), nil},
		{"t267 titlecase", "{var:t,hello wORLD}{%t|titlecase}", none, o, one("Hello World"), nil},
		{"t403 titlecase after an apostrophe", "{var:t,they're o'neil}{%t|titlecase}", none, o, one("They'Re O'Neil"), nil},
		{"titlecase after a digit", "{var:t,1st 2ND}{%t|titlecase}", none, o, one("1St 2Nd"), nil},
		{"titlecase after U+00AA, which is cased", "{var:t,\U000000aab}{%t|titlecase}", none, o, one("\U000000aab"), nil},
		{"titlecase after U+2160, which is cased", "{var:t,\U00002160a}{%t|titlecase}", none, o, one("\U00002160a"), nil},
		{"titlecase after a title case letter", "{var:t,\U000001c5a}{%t|titlecase}", none, o, one("\U000001c5a"), nil},
		// UPSTREAM.md: str.title gives σ for the second and third sigma
		{"titlecase lowers each word apart",
			"{var:t,\U00000391\U000003a3 \U00000391\U000003a3'\U00000392 \U00000391\U000003a3'\U000003a3}{%t|titlecase}", none, o,
			one("\U00000391\U000003c2 \U00000391\U000003c2'\U00000392 \U00000391\U000003c2'\U000003a3"), nil},
		{"t404 upper of a sharp s", "{var:t,stra\U000000dfe}{%t|upper}", none, o, one("STRASSE"), nil},
		{"lower of U+0130", "{var:t,\U00000130}{%t|lower}", none, o, one("i\U00000307"), nil},
		{"lower of a final sigma", "{var:t,\U000003a3\U00000391\U000003a3}{%t|lower}", none, o, one("\U000003c3\U000003b1\U000003c2"), nil},
		{"capitalize to a title case letter", "{var:t,\U000001c6X}{%t|capitalize}", none, o, one("\U000001c5x"), nil},
		{"capitalize of a sharp s", "{var:t,\U000000dfA}{%t|capitalize}", none, o, one("Ssa"), nil},
		{"capitalize keeps a final sigma", "{var:t,\U000003a3\U000003a3}{%t|capitalize}", none, o, one("\U000003a3\U000003c2"), nil},
		{"capitalize of an empty value", "{keyword|capitalize}", keywords(""), o, one(""), nil},
		{"titlecase of an empty value", "{keyword|titlecase}", keywords(""), o, one(""), nil},
		{"strip", "{keyword|strip}", keywords(" a ", "\U0000001cb\t"), o, []string{"a", "b"}, nil},
		{"braces", "{keyword|braces}", k(), o, []string{"{Foo}", "{bar}", "{Foo}", "{baz}"}, nil},
		{"parens", "{keyword|parens}", k(), o, []string{"(Foo)", "(bar)", "(Foo)", "(baz)"}, nil},
		{"brackets", "{keyword|brackets}", k(), o, []string{"[Foo]", "[bar]", "[Foo]", "[baz]"}, nil},
		{"shell_quote", "{keyword|shell_quote}",
			keywords("My file.jpeg", "plain", "it's", "", "a;b", "$x", "`x`", "a\nb", "\U00000444\U00000430\U00000439\U0000043b",
				"@%+=:,./-_aZ09", "\U000000e9"), o,
			[]string{"'My file.jpeg'", "plain", `'it'"'"'s'`, "''", "'a;b'", "'$x'", "'`x`'", "'a\nb'",
				"'\U00000444\U00000430\U00000439\U0000043b'", "@%+=:,./-_aZ09", "'\U000000e9'"}, nil},
		// phototemplate.py:1098-1161
		{"split", "{keyword|split(;)}", keywords("a;;b", "", "c"), o, []string{"a", "", "b", "", "c"}, nil},
		{"split by a string", "{keyword|split(, )}", keywords("a, b,c"), o, []string{"a", "b,c"}, nil},
		{"autosplit", "{keyword|autosplit}", keywords("a,b;c d", "a , b", "  "), o, []string{"a", "b", "c", "d", "a", "b"}, nil},
		{"sort", "{keyword|sort}", k(), o, []string{"Foo", "Foo", "bar", "baz"}, nil},
		{"rsort", "{keyword|rsort}", k(), o, []string{"baz", "bar", "Foo", "Foo"}, nil},
		{"reverse", "{keyword|reverse}", k(), o, []string{"baz", "Foo", "bar", "Foo"}, nil},
		{"uniq keeps the first", "{keyword|uniq}", k(), o, []string{"Foo", "bar", "baz"}, nil},
		{"join", "{keyword|join(,)}", k(), o, one("Foo,bar,Foo,baz"), nil},
		{"join()", "{keyword|join()}", k(), o, one("FoobarFoobaz"), nil},
		{"join without arguments", "{keyword|join}", k(), o, one("FoobarFoobaz"), nil},
		{"join of no values", "{descr|join(,)}", k(), o, one(""), nil},
		{"append", "{keyword|append(z)}", k(), o, []string{"Foo", "bar", "Foo", "baz", "z"}, nil},
		{"append to no values", "{descr|append(x)}", k(), o, one("x"), nil},
		{"prepend", "{keyword|prepend(z)}", k(), o, []string{"z", "Foo", "bar", "Foo", "baz"}, nil},
		{"appends", "{keyword|appends(z)}", k(), o, []string{"Fooz", "barz", "Fooz", "bazz"}, nil},
		{"prepends", "{keyword|prepends(z)}", k(), o, []string{"zFoo", "zbar", "zFoo", "zbaz"}, nil},
		{"remove", "{keyword|remove(Foo)}", k(), o, []string{"bar", "baz"}, nil},
		{"remove nothing", "{keyword|remove(x)}", k(), o, []string{"Foo", "bar", "Foo", "baz"}, nil},
		{"chain", "{keyword|lower|sort|join(;)}", k(), o, one("bar;baz;foo;foo"), nil},
		{"argument from a variable", "{var:d,-}{keyword|join(%d)}", k(), o, one("Foo-bar-Foo-baz"), nil},
		// a name that passes the check but matches no filter gives no values (1057-1063, 1182-1183)
		{"lower.x", "{keyword|lower.x}", k(), o, one("_"), nil},
		{"int:y", "{keyword|int:y}", k(), o, one("_"), nil},
		{"function without a colon", "{keyword|function}", k(), o, one("_"), nil},
		// after delim, before [find,replace] (phototemplate.py:665-683)
		{"after delim", "{,+keyword|upper}", k(), o, one("FOO,BAR,FOO,BAZ"), nil},
		{"sort after delim", "{,+keyword|sort}", k(), o, one("Foo,bar,Foo,baz"), nil},
		{"before find and replace", "{keyword|upper[O,0]}", k(), o, []string{"F00", "BAR", "F00", "BAZ"}, nil},
		// the field shell_quote (phototemplate.py:1340-1341)
		{"field shell_quote", "{shell_quote,My file}", none, o, one("'My file'"), nil},
		{"field shell_quote drops empty values", "{shell_quote,{keyword}}", keywords("a b", "", "c"), o, []string{"'a b'", "c"}, nil},
		{"field shell_quote without text", "{shell_quote}", none, o, one("_"), nil},
		{"field shell_quote falls back to its default", "{shell_quote|lower.x,a b}", none, o, one("a b"), nil},
	})
}

// Parse stops the arguments at the first ")", but a Template may be built by
// hand; upstream strips every ")" at the end (phototemplate.py:1052).
func TestFilterArgumentsEndParens(t *testing.T) {
	t.Parallel()
	tmpl := &Template{Segments: []Segment{{Field: &Field{Name: "descr", Filters: []string{"append(x))"}}}}}
	got, _, err := NewRenderer().Render(tmpl, keywords(), RenderOptions{})
	if err != nil || !slices.Equal(got, []string{"x"}) {
		t.Errorf("Render of {descr|append(x))} = %q, %v; want [x]", got, err)
	}
}

func TestFilterErrors(t *testing.T) {
	t.Parallel()
	p := keywords("Foo", "bar")
	for _, tt := range []struct {
		in    string
		want  error
		names string
	}{
		{"{keyword|nope}", ErrRender, "nope"},
		{"{keyword|nope(x)}", ErrRender, "nope"},
		{"{keyword|lowerx}", ErrRender, "lowerx"},
		{"{keyword|pathx}", ErrRender, "pathx"},
		// re.search(r"\(.*\)") does not cross a newline, so the name keeps the parentheses (1050, 1063)
		{"{keyword|split(\n)}", ErrRender, "unknown filter"},
		{"{keyword|split}", ErrRender, "split"},
		{"{keyword|split()}", ErrRender, "split"},
		{"{keyword|append}", ErrRender, "append"},
		{"{keyword|prepend()}", ErrRender, "prepend"},
		{"{keyword|appends}", ErrRender, "appends"},
		{"{keyword|prepends()}", ErrRender, "prepends"},
		{"{keyword|remove}", ErrRender, "remove"},
		{"{keyword|chop}", ErrRender, "chop"},
		{"{keyword|chomp}", ErrRender, "chomp"},
		{"{keyword|slice}", ErrRender, "slice"},
		{"{keyword|sslice}", ErrRender, "sslice"},
		{"{keyword|filter}", ErrRender, "filter"},
		{"{keyword|join(%nope)}", ErrRender, `"nope"`},
		{"{var:k,{keyword}}{keyword|join(%k)}", ErrRender, "filter arguments"},
		{"{keyword|function:f.py::g}", ErrUnsupportedField, "function"},
		{"{keyword|function:f.py::g(%nope)}", ErrUnsupportedField, "function"},
		{"{keyword|slice(1:)}", ErrUnsupportedConstruct, "slice"},
		{"{keyword|sslice(1)}", ErrUnsupportedConstruct, "sslice"},
		{"{keyword|chop(1)}", ErrUnsupportedConstruct, "chop"},
		{"{keyword|chomp(1)}", ErrUnsupportedConstruct, "chomp"},
		{"{keyword|filter(contains a)}", ErrUnsupportedConstruct, "filter"},
		{"{keyword|int}", ErrUnsupportedConstruct, "int"},
		{"{keyword|float}", ErrUnsupportedConstruct, "float"},
		{"{keyword|path}", ErrUnsupportedConstruct, "path"},
		{"{keyword|path.stem}", ErrUnsupportedConstruct, "path.stem"},
		{"{keyword|path:x}", ErrUnsupportedConstruct, "path:x"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, _, err := render(t, tt.in, p, RenderOptions{})
			if !errors.Is(err, tt.want) || !strings.Contains(fmt.Sprint(err), tt.names) || got != nil {
				t.Errorf("Render(%q) = %q, %v; want an error that wraps %v and names %q", tt.in, got, err, tt.want, tt.names)
			}
		})
	}
}

// The filters count the strings they build against the 16 MiB of a render and
// the values against MaxResults; the bool value drops what they built, so only
// the filter can count it.
func TestFilterBounds(t *testing.T) {
	t.Parallel()
	mib := strings.Repeat("a", 1<<20)
	big := fakePhoto{"title": strs(mib), "keyword": strs(numbered("k", 20)...)}
	for _, tt := range []struct {
		in    string
		photo fakePhoto
		want  error
	}{
		{"{var:a,{title}}{keyword|appends(x)?Y,N}", big, nil},
		{"{var:a,{title}}{keyword|appends(%a)?Y,N}", big, ErrTooManyValues},
		{"{var:a,{title}}{keyword|prepends(%a)?Y,N}", big, ErrTooManyValues},
		{"{var:a,{title}}{keyword|join(%a)?Y,N}", big, ErrTooManyValues},
		{"{keyword?Y,N}", keywords(slices.Repeat([]string{mib}, 20)...), nil},
		{"{keyword|upper?Y,N}", keywords(slices.Repeat([]string{mib}, 20)...), ErrTooManyValues},
		{"{keyword|join?Y,N}", keywords(slices.Repeat([]string{mib}, 20)...), ErrTooManyValues},
		{"{title" + strings.Repeat("|append(x)|join", 17) + "?Y,N}", fakePhoto{"title": strs(mib)}, ErrTooManyValues},
		{"{shell_quote?Y,{keyword}}", keywords(slices.Repeat([]string{mib}, 10)...), ErrTooManyValues},
		{"{title|split(,)|join?Y,N}", fakePhoto{"title": strs(strings.Repeat(",", DefaultMaxResults-1))}, nil},
		{"{title|split(,)|join?Y,N}", fakePhoto{"title": strs(strings.Repeat(",", DefaultMaxResults))}, ErrTooManyValues},
		{"{title|autosplit|join?Y,N}", fakePhoto{"title": strs(strings.Repeat("a ", DefaultMaxResults+1))}, ErrTooManyValues},
		{"{keyword|append(x)|join?Y,N}", keywords(numbered("k", DefaultMaxResults-1)...), nil},
		{"{keyword|append(x)|join?Y,N}", keywords(numbered("k", DefaultMaxResults)...), ErrTooManyValues},
	} {
		got, _, err := render(t, tt.in, tt.photo, RenderOptions{})
		if !errors.Is(err, tt.want) || tt.want == nil && !slices.Equal(got, []string{"Y"}) {
			t.Errorf("Render(%q) = %d values, %v; want %v", tt.in, len(got), err, tt.want)
		}
	}
}
