package template

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxDepth bounds the recursion on untrusted text; upstream is bounded only by
// the recursion limit of Python.
const maxDepth = 32

// OPERATOR tries "<=" before "<" (phototemplate.tx:97-99).
var operators = []string{"contains", "matches", "startswith", "endswith", "<=", ">=", "<", ">", "==", "!="}

// Parse parses a template statement as PhotoTemplateParser.parse does
// (phototemplate.py:421-425); "" is a Template without segments
// (phototemplate.py:509-511). The error wraps ErrSyntax and names the byte
// offset at which the grammar cannot go on and what it expects there; a
// statement nested deeper than 32, the top one being 1, is an error too.
func Parse(s string) (t *Template, err error) {
	defer func() {
		if r := recover(); r != nil {
			t, err = nil, r.(error)
		}
	}()
	p := parser{s: s}
	t, i := p.statement(0, 1)
	if i < len(s) {
		fail(i, "end of input")
	}
	return t, nil
}

type parser struct{ s string }

// Every failure of the grammar is final: the rule tried next cannot start
// where a failed one stopped. So fail unwinds to Parse, as in text/template/parse.
func fail(i int, want string) {
	panic(fmt.Errorf("%w at %d: expected %s", ErrSyntax, i, want))
}

func (p *parser) at(i int, lit string) bool { return strings.HasPrefix(p.s[i:], lit) }

func (p *parser) expect(i int, lit string) int {
	if !p.at(i, lit) {
		fail(i, `"`+lit+`"`)
	}
	return i + len(lit)
}

func (p *parser) run(i int, in func(rune) bool) int {
	return len(p.s) - len(strings.TrimLeftFunc(p.s[i:], in))
}

func (p *parser) spaces(i int) int { return len(p.s) - len(strings.TrimLeft(p.s[i:], " ")) }

func notIn(set string) func(rune) bool {
	return func(r rune) bool { return !strings.ContainsRune(set, r) }
}

func wordOr(set string) func(rune) bool {
	return func(r rune) bool { return isWord(r) || strings.ContainsRune(set, r) }
}

// \w of Python's re for str: str.isalnum() or "_".
func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

func ptr(s string) *string { return &s }

func (p *parser) statement(i, depth int) (*Template, int) {
	if depth > maxDepth {
		fail(i, "at most 32 nested statements")
	}
	t := &Template{}
	for start := -1; start != i; {
		start = i
		seg := Segment{Pre: p.s[i:p.run(i, notIn("{},?"))]}
		if i += len(seg.Pre); p.at(i, "{") {
			seg.Field, i = p.field(i+1, depth)
		}
		seg.Post = p.s[i:p.run(i, notIn("{},?"))]
		if i += len(seg.Post); i > start {
			t.Segments = append(t.Segments, seg)
		}
	}
	return t, i
}

func (p *parser) field(i, depth int) (*Field, int) {
	f := &Field{}
	f.Delim, i = p.delim(i)
	f.Name = p.s[i:p.run(i, wordOr(".%"))]
	if f.Name == "" {
		fail(i, "a field name")
	}
	// FIELD_WORD+ stops at a "%" that no "." or \w follows (phototemplate.tx:51-57).
	if k := strings.Index(f.Name+"%", "%%"); k >= 0 {
		fail(i+k+1, "a field name")
	}
	if i += len(f.Name); p.at(i, ":") {
		f.Subfield, i = p.subfield(i + 1)
	}
	if p.at(i, "(") {
		j := p.expect(p.run(i+1, notIn("(){}")), ")")
		f.Arg, i = ptr(p.s[i+1:j-1]), j
	}
	if p.at(i, "|") {
		f.Filters, i = p.filters(i + 1)
	}
	if p.at(i, "[") {
		f.FindReplace, i = p.findReplace(i + 1)
	}
	if p.at(i, " ") {
		f.Conditional, i = p.conditional(i, depth)
	}
	f.Combine, i = p.sub(i, "&", depth)
	f.Bool, i = p.sub(i, "?", depth)
	f.Default, i = p.sub(i, ",", depth)
	return f, p.expect(i, "}")
}

func (p *parser) sub(i int, sep string, depth int) (*Template, int) {
	if !p.at(i, sep) {
		return nil, i
	}
	return p.statement(i+1, depth+1)
}

// The greedy DELIM_WORD backs off from the first brace to the last "+" that
// "%" or \w follows (phototemplate.tx:47-49); a bare "+" is a Delim too (43).
func (p *parser) delim(i int) (*string, int) {
	for k := p.run(i, notIn("{}")) - 1; k > i; k-- {
		if r, _ := utf8.DecodeRuneInString(p.s[k+1:]); p.s[k] == '+' && (r == '%' || isWord(r)) {
			return ptr(p.s[i:k]), k + 1
		}
	}
	if p.at(i, "+") {
		return ptr(""), i + 1
	}
	return nil, i
}

// A SUBFIELD_WORD may end in "\" and a space (phototemplate.tx:66-69); \s of
// Python is str.isspace, which unlike unicode.IsSpace takes U+001C-U+001F.
func (p *parser) subfield(i int) (string, int) {
	start, word := i, wordOr(".:/-~'\"%@#^\U00002019")
	for j := p.run(i, word); j > i; j = p.run(i, word) {
		i = j
		r, n := utf8.DecodeRuneInString(p.s[min(i+1, len(p.s)):])
		if p.at(i, `\`) && (unicode.IsSpace(r) || 0x1c <= r && r <= 0x1f) {
			i += 1 + n
		}
	}
	if i == start {
		fail(i, "a subfield")
	}
	return p.s[start:i], i
}

// The list after "|" is optional, a filter after a "|" between filters is not
// (phototemplate.tx:71-80).
func (p *parser) filters(i int) ([]string, int) {
	var out []string
	for sep := ""; p.at(i, sep); sep = "|" {
		start := i + len(sep)
		if i = p.run(start, wordOr(".:/")); i == start {
			if sep == "" {
				break
			}
			fail(i, "a filter")
		}
		// The "(" group of FILTER_FUNCTION fails at the end of the input, and
		// no rule after a filter starts with "(" (phototemplate.tx:19-34, 79).
		if p.at(i, "(") {
			j := strings.IndexByte(p.s[i:], ')')
			if j < 0 {
				fail(len(p.s), `")"`)
			}
			i += j + 1
		}
		out = append(out, p.s[start:i])
	}
	return out, i
}

// FIND_WORD is greedy: it ends at the last "," (phototemplate.tx:123-125).
func (p *parser) findReplace(i int) ([]Pair, int) {
	pairs := []Pair{}
	for sep := ""; !p.at(i, "]") && p.at(i, sep); sep = "|" {
		start := i + len(sep)
		i = p.run(start, notIn("[]|"))
		c := strings.LastIndexByte(p.s[:i], ',')
		if c < start {
			fail(i, `","`)
		}
		pairs = append(pairs, Pair{Find: p.s[start:c], Replace: p.s[c+1 : i]})
	}
	return pairs, p.expect(i, "]")
}

func (p *parser) conditional(i, depth int) (*Conditional, int) {
	c := &Conditional{}
	if i = p.spaces(i); p.at(i, "not ") {
		c.Negate, i = true, i+len("not ")
	}
	k := slices.IndexFunc(operators, func(op string) bool { return p.at(i, op) })
	if k < 0 {
		fail(i, "an operator")
	}
	c.Operator = operators[k]
	v, i := p.statement(p.spaces(p.expect(i+len(c.Operator), " ")), depth+1)
	c.Value = *v
	return c, i
}
