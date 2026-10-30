package template

import (
	"errors"
	"fmt"
	"math"
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
		// create_slice and int() (phototemplate.py:1106-1119, 1703-1722); a ValueError of slice even without values
		{"{keyword|slice(::0)}", ErrRender, "slice step"},
		{"{descr|slice(::0)}", ErrRender, "slice step"},
		{"{keyword|sslice(1::0)}", ErrRender, "sslice step"},
		{"{keyword|slice(1:2:3:4)}", ErrRender, "slice"},
		{"{keyword|slice(1:2:3:)}", ErrRender, "slice"},
		{"{keyword|slice(x)}", ErrRender, "slice"},
		{"{keyword|slice( )}", ErrRender, "slice"},
		{"{keyword|slice(1.5:)}", ErrRender, "slice"},
		{"{keyword|chop(x)}", ErrRender, "chop"},
		{"{keyword|chop(1:2)}", ErrRender, "chop"},
		{"{keyword|chomp(1.0)}", ErrRender, "chomp"},
		{"{keyword|chomp(1:2)}", ErrRender, "chomp"},
		// filter_predicate (phototemplate.py:1190-1233)
		{"{keyword|filter(contains)}", ErrRender, "operator and a value"},
		{"{keyword|filter( )}", ErrRender, "operator and a value"},
		{"{keyword|filter(not)}", ErrRender, "operator and a value"},
		{"{keyword|filter(not not  )}", ErrRender, "operator and a value"},
		{"{keyword|filter(nope a)}", ErrRender, "unknown operator"},
		{"{keyword|filter(not = a)}", ErrRender, "unknown operator"},
		{"{var:l,1;5;x}{%l|split(;)|filter(> 2)}", ErrRender, ">"},
		{"{var:l,5}{%l|filter(< 1|x)}", ErrRender, "<"},
		{"{var:l,x}{%l|filter(>= 1|2)}", ErrRender, ">="},
		// the variable is not at the start of the argument, so it stays "%n" (t402)
		{"{var:n,2}{var:l,1;5}{%l|split(;)|filter(> %n)}", ErrRender, ">"},
		// int(float("inf")) raises OverflowError, which values_to_int does not catch (1725-1731)
		{"{var:i,inf}{%i|int}", ErrRender, "int"},
		{"{var:i,-1e400}{%i|split(;)|int}", ErrRender, "int"},
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
		{"{title" + strings.Repeat("|sslice(0:)", 15) + "?Y,N}", fakePhoto{"title": strs(mib)}, nil},
		{"{title" + strings.Repeat("|sslice(0:)", 17) + "?Y,N}", fakePhoto{"title": strs(mib)}, ErrTooManyValues},
		{"{title" + strings.Repeat("|chop(1)", 17) + "?Y,N}", fakePhoto{"title": strs(mib)}, ErrTooManyValues},
		{"{title" + strings.Repeat("|chomp(-1048576)", 17) + "?Y,N}", fakePhoto{"title": strs(mib)}, ErrTooManyValues},
		// int(1e300) has 301 digits: 10000 of them are 3 MB
		{"{keyword" + strings.Repeat("|int", 5) + "?Y,N}", keywords(slices.Repeat([]string{"1e300"}, DefaultMaxResults)...), nil},
		{"{keyword" + strings.Repeat("|int", 6) + "?Y,N}", keywords(slices.Repeat([]string{"1e300"}, DefaultMaxResults)...), ErrTooManyValues},
		// 10000 strings of 21 to 23 bytes are 210 to 230 kB
		{"{keyword" + strings.Repeat("|float", 60) + "?Y,N}",
			keywords(slices.Repeat([]string{"1.2345678901234567e-300"}, DefaultMaxResults)...), nil},
		{"{keyword" + strings.Repeat("|float", 100) + "?Y,N}",
			keywords(slices.Repeat([]string{"1.2345678901234567e-300"}, DefaultMaxResults)...), ErrTooManyValues},
	} {
		got, _, err := render(t, tt.in, tt.photo, RenderOptions{})
		if !errors.Is(err, tt.want) || tt.want == nil && !slices.Equal(got, []string{"Y"}) {
			t.Errorf("Render(%q) = %d values, %v; want %v", tt.in, len(got), err, tt.want)
		}
	}
}

// slice.indices of Python, as list(range(n))[start:end:step] gives them.
func TestPySlice(t *testing.T) {
	t.Parallel()
	p := func(i int) *int { return &i }
	seq := func(n int) []int {
		xs := make([]int, n)
		for i := range xs {
			xs[i] = i
		}
		return xs
	}
	for _, tt := range []struct {
		n    int
		b    [3]*int
		want []int
	}{
		{6, [3]*int{p(1), p(3), nil}, []int{1, 2}},
		{6, [3]*int{p(1), p(5), p(2)}, []int{1, 3}},
		{6, [3]*int{p(1), nil, nil}, []int{1, 2, 3, 4, 5}},
		{6, [3]*int{nil, p(-1), nil}, []int{0, 1, 2, 3, 4}},
		{6, [3]*int{nil, nil, p(-1)}, []int{5, 4, 3, 2, 1, 0}},
		{6, [3]*int{p(2), nil, nil}, []int{2, 3, 4, 5}},
		{6, [3]*int{p(-2), nil, nil}, []int{4, 5}},
		{6, [3]*int{p(10), nil, nil}, nil},
		{6, [3]*int{p(-10), nil, nil}, []int{0, 1, 2, 3, 4, 5}},
		{6, [3]*int{nil, nil, p(2)}, []int{0, 2, 4}},
		{6, [3]*int{p(5), p(1), p(-1)}, []int{5, 4, 3, 2}},
		{6, [3]*int{nil, nil, p(-2)}, []int{5, 3, 1}},
		{6, [3]*int{p(10), nil, p(-1)}, []int{5, 4, 3, 2, 1, 0}},
		{6, [3]*int{p(-10), nil, p(-1)}, nil},
		{6, [3]*int{nil, p(-10), p(-1)}, []int{5, 4, 3, 2, 1, 0}},
		{6, [3]*int{nil, p(10), p(-1)}, nil},
		{6, [3]*int{p(3), p(3), nil}, nil},
		{6, [3]*int{p(4), p(2), nil}, nil},
		{6, [3]*int{p(1), p(-1), p(math.MaxInt)}, []int{1}},
		{6, [3]*int{nil, nil, p(math.MinInt)}, []int{5}},
		{6, [3]*int{p(math.MinInt), p(math.MaxInt), nil}, []int{0, 1, 2, 3, 4, 5}},
		{6, [3]*int{p(math.MaxInt), p(math.MinInt), p(-1)}, []int{5, 4, 3, 2, 1, 0}},
		{1, [3]*int{nil, nil, p(-3)}, []int{0}},
		{0, [3]*int{nil, nil, nil}, nil},
		{0, [3]*int{nil, nil, p(-1)}, nil},
		{0, [3]*int{p(-1), p(1), p(math.MaxInt)}, nil},
	} {
		if got := pySlice(seq(tt.n), tt.b); !slices.Equal(got, tt.want) {
			t.Errorf("pySlice(%d, %s) = %v, want %v", tt.n, sliceString(tt.b), got, tt.want)
		}
	}
}

func sliceString(b [3]*int) string {
	parts := make([]string, len(b))
	for i, p := range b {
		if p != nil {
			parts[i] = fmt.Sprint(*p)
		}
	}
	return strings.Join(parts, ":")
}

func TestSlice(t *testing.T) {
	t.Parallel()
	k := func() fakePhoto { return keywords("a", "b", "c", "d", "e", "f") }
	o, none := RenderOptions{}, fakePhoto{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		// phototemplate.py:1162-1168, 1703-1722
		{"slice with a step", "{keyword|slice(::2)}", k(), o, []string{"a", "c", "e"}, nil},
		{"slice before the start", "{keyword|slice(-10:)}", k(), o, []string{"a", "b", "c", "d", "e", "f"}, nil},
		{"slice past the end", "{keyword|slice(10:)}", k(), o, one("_"), nil},
		{"slice of int() with spaces", "{var:s, 1 : 3}{keyword|slice(%s)}", k(), o, []string{"b", "c"}, nil},
		{"slice of int() with U+001C", "{var:s,\U0000001c4\U0000001c}{keyword|slice(%s)}", k(), o, []string{"e", "f"}, nil},
		{"slice with a plus sign", "{var:s,{title}}{keyword|slice(%s)}", with(k(), "title", strs(" 1 :+3")), o, []string{"b", "c"}, nil},
		{"slice of empty parts", "{keyword|slice(:)}", k(), o, []string{"a", "b", "c", "d", "e", "f"}, nil},
		{"slice with an empty step", "{keyword|slice(4::)}", k(), o, []string{"e", "f"}, nil},
		{"slice of one number", "{keyword|slice(-1)}", k(), o, one("f"), nil},
		{"slice of no values", "{descr|slice(1:)}", k(), o, one("_"), nil},
		{"sslice backwards", "{var:w,abcdef}{%w|sslice(5:1:-1)}", none, o, one("fedc"), nil},
		{"sslice backwards with a step", "{var:w,abcdef}{%w|sslice(::-2)}", none, o, one("fdb"), nil},
		{"sslice of every value", "{keyword|sslice(1)}", keywords("ab", "c"), o, []string{"b", ""}, nil},
		{"t292 sslice by code points", "{var:w,h\U000000e9llo}{%w|sslice(1:3)}", none, o, one("\U000000e9l"), nil},
		// sslice raises the ValueError of a step 0 only on a value (1167-1168)
		{"sslice step 0 of no values", "{descr|sslice(::0)}", k(), o, one("_"), nil},
		// v[:-n] and v[n:] (1106-1119)
		{"t293 chop by code points", "{var:w,h\U000000e9llo}{%w|chop(1)}", none, o, one("h\U000000e9ll"), nil},
		{"t294 chomp by code points", "{var:w,h\U000000e9llo}{%w|chomp(2)}", none, o, one("llo"), nil},
		{"chop of a negative number keeps the start", "{var:w,hello}{%w|chop(-1)}", none, o, one("h"), nil},
		{"chomp of a negative number keeps the end", "{var:w,hello}{%w|chomp(-1)}", none, o, one("o"), nil},
		{"chop(0)", "{var:w,hello}{%w|chop(0)}", none, o, one("hello"), nil},
		{"chomp(0)", "{var:w,hello}{%w|chomp(0)}", none, o, one("hello"), nil},
		{"chop past the length", "{var:w,hello}{%w|chop(10)}", none, o, one(""), nil},
		{"chomp before the start", "{var:w,hello}{%w|chomp(-10)}", none, o, one("hello"), nil},
		{"chop of every value", "{keyword|chop( 1 )}", keywords("ab", "c", ""), o, []string{"a", "", ""}, nil},
		// int() has no bound, and slice.indices clamps to the length
		{"sslice past 64 bits", "{var:w,abc}{%w|sslice(:99999999999999999999)}", none, o, one("abc"), nil},
		{"chop of the smallest int", "{var:w,hello}{%w|chop(-9223372036854775808)}", none, o, one("hello"), nil},
		{"chop past 64 bits", "{var:w,hello}{%w|chop(99999999999999999999)}", none, o, one(""), nil},
	})
}

func TestPredicate(t *testing.T) {
	t.Parallel()
	k := func() fakePhoto { return keywords("a", "b", "ab", "1", "5", "x") }
	n := func() fakePhoto { return keywords("1", "5") }
	o := RenderOptions{}
	all := []string{"a", "b", "ab", "1", "5", "x"}
	checkRender(t, []renderCase{
		// phototemplate.py:1186-1235
		{"contains", "{keyword|filter(contains a)}", k(), o, []string{"a", "ab"}, nil},
		{"not contains", "{keyword|filter(not contains a)}", k(), o, []string{"b", "1", "5", "x"}, nil},
		{"not not", "{keyword|filter(not not contains a)}", k(), o, []string{"a", "ab"}, nil},
		{"not not not", "{keyword|filter(not  not not contains a)}", k(), o, []string{"b", "1", "5", "x"}, nil},
		{"matches an alternative", "{keyword|filter(matches a|b)}", k(), o, []string{"a", "b"}, nil},
		{"== a", "{keyword|filter(== a)}", k(), o, []string{"a"}, nil},
		{"== splits on the pipe, unlike a condition", "{keyword|filter(== a|b)}", k(), o, []string{"a", "b"}, nil},
		{"!= a", "{keyword|filter(!= a)}", k(), o, []string{"b", "ab", "1", "5", "x"}, nil},
		{"!= of two alternatives holds for every value", "{keyword|filter(!= a|b)}", k(), o, all, nil},
		{"startswith", "{keyword|filter(startswith a)}", k(), o, []string{"a", "ab"}, nil},
		{"endswith", "{keyword|filter(endswith b)}", k(), o, []string{"b", "ab"}, nil},
		{"spaces around the operator", "{keyword|filter(  contains   a)}", k(), o, []string{"a", "ab"}, nil},
		{"U+001C separates the operator", "{keyword|filter(contains\U0000001ca)}", k(), o, []string{"a", "ab"}, nil},
		// str.split(None, 1) keeps the spaces at the end of the value
		{"space after the value", "{keyword|filter(contains a )}", k(), o, []string{"_"}, nil},
		{"empty alternative", "{keyword|filter(contains |q)}", k(), o, all, nil},
		{"nothing matches", "{keyword|filter(matches q)}", k(), o, []string{"_"}, nil},
		{"> 2", "{keyword|filter(> 2)}", n(), o, []string{"5"}, nil},
		{"not > 2", "{keyword|filter(not > 2)}", n(), o, []string{"1"}, nil},
		{"<= an alternative", "{keyword|filter(<= 1|5)}", n(), o, []string{"1", "5"}, nil},
		{">= 5", "{keyword|filter(>= 5)}", n(), o, []string{"5"}, nil},
		{"< 5", "{keyword|filter(< 5)}", n(), o, []string{"1"}, nil},
		{"nan is below nothing", "{keyword|filter(< 2)}", keywords("nan", "1"), o, []string{"1"}, nil},
		// any() stops at the first match (1204-1207)
		{"an alternative after a match is not read", "{keyword|filter(< 1|x)}", keywords("0"), o, []string{"0"}, nil},
		{"the argument from a variable", "{var:n,> 2}{keyword|filter(%n)}", n(), o, []string{"5"}, nil},
		// upstream reads the argument for each value (1171)
		{"unknown operator without values", "{descr|filter(nope a)}", k(), o, []string{"_"}, nil},
		{"no value without values", "{descr|filter(contains)}", k(), o, []string{"_"}, nil},
	})
}

func TestNumbers(t *testing.T) {
	t.Parallel()
	o, none := RenderOptions{}, fakePhoto{}
	checkRender(t, []renderCase{
		// phototemplate.py:1725-1740
		{"int", "{keyword|int}",
			keywords("1.9", "-1.9", "x", "3", "1e3", "-0.5", " 7 ", "nan", "1e20", "1_000", "", "0x10"), o,
			[]string{"1", "-1", "3", "1000", "0", "7", "100000000000000000000", "1000"}, nil},
		{"int gives the exact digits of the double", "{var:i,1e23}{%i|int}", none, o, []string{"99999999999999991611392"}, nil},
		{"int of nan", "{var:i,nan}{%i|int}", none, o, []string{"_"}, nil},
		{"int of no values", "{descr|int}", keywords(), o, []string{"_"}, nil},
		{"float", "{keyword|float}",
			keywords("1", "1e16", "inf", "nan", "x", "-0", "1e-5", "-inf", " 2 ", "1e400", "0.1", "-Infinity", "-nan"), o,
			[]string{"1.0", "1e+16", "inf", "nan", "-0.0", "1e-05", "-inf", "2.0", "inf", "0.1", "-inf"}, nil},
		{"int then filter", "{var:i,1.9;-1.9;x;3;1e3}{%i|split(;)|int|filter(> 1)}", none, o, []string{"3", "1000"}, nil},
	})
}

// repr of a float of Python 3 (float_repr_style "short").
func TestPyFloatRepr(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		f    float64
		want string
	}{
		{1, "1.0"}, {1000, "1000.0"}, {1e16, "1e+16"}, {1e15, "1000000000000000.0"}, {9999999999999998, "9999999999999998.0"},
		{0.0001, "0.0001"}, {0.00001, "1e-05"}, {0.1, "0.1"}, {123456789.125, "123456789.125"}, {1.5e300, "1.5e+300"},
		{-1.5e-7, "-1.5e-07"}, {5e-324, "5e-324"}, {math.MaxFloat64, "1.7976931348623157e+308"}, {0, "0.0"},
		{math.Copysign(0, -1), "-0.0"}, {math.Inf(1), "inf"}, {math.Inf(-1), "-inf"}, {math.NaN(), "nan"},
	} {
		if got := pyFloatRepr(tt.f); got != tt.want {
			t.Errorf("pyFloatRepr(%v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}
