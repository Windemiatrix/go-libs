package template

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// formatSpec is a format_spec of Python, [[fill]align][sign][#][0][width][,|_][.precision][type],
// as CPython 3.12 reads it (Python/formatter_unicode.c); an absent precision is -1.
type formatSpec struct {
	fill, align, sign, grouping, typ string
	alt, zero, z                     bool
	width, precision                 int
}

var errSpec = fmt.Errorf("%w: invalid format specification", ErrRender)

func inField(err error, f *Field) error {
	if err != nil {
		err = fmt.Errorf("%w in {%s:%s}", err, f.Name, f.Subfield)
	}
	return err
}

// formatValue is format_str_value (phototemplate.py:1657-1662) of an int64:
// format(v, spec) of Python, str(v) for "". A width beyond the bytes left to the
// render is errTooLong before anything is built, since the template sets it and
// upstream has no bound.
func (s *rendering) formatValue(v int64, spec string) (string, error) {
	sp, err := parseSpec(spec)
	if err != nil {
		return "", err
	} else if sp.width > s.bytesLeft/len(sp.fill) {
		return "", errTooLong
	}
	out, err := formatInt(v, sp)
	if s.bytesLeft -= len(out); s.bytesLeft < 0 {
		return "", errTooLong
	}
	return out, err
}

func parseSpec(spec string) (sp formatSpec, err error) {
	take := func(set string) (c string) {
		if spec != "" && strings.Contains(set, spec[:1]) {
			c, spec = spec[:1], spec[1:]
		}
		return c
	}
	digits := func() (int, error) { // a number beyond int fails, as one beyond Py_ssize_t in Python
		k := len(spec) - len(strings.TrimLeft(spec, "0123456789"))
		n, err := strconv.Atoi(spec[:k])
		spec = spec[k:]
		return n, err
	}
	if _, n := utf8.DecodeRuneInString(spec); n < len(spec) && strings.Contains("<>=^", spec[n:n+1]) {
		sp.fill, spec = spec[:n], spec[n:]
	}
	sp.align, sp.sign = take("<>=^"), take("+- ")
	sp.z, sp.alt = take("z") != "", take("#") != ""
	if sp.zero = sp.fill == "" && take("0") != ""; sp.zero { // after a fill, "0" is a digit of the width
		sp.fill = "0"
	}
	sp.fill, sp.precision = cmp.Or(sp.fill, " "), -1
	if sp.width, err = digits(); errors.Is(err, strconv.ErrRange) {
		return sp, errSpec
	}
	if sp.grouping = take(",_"); take(".") != "" {
		if sp.precision, err = digits(); err != nil {
			return sp, errSpec
		}
	}
	sp.typ = spec
	groups := spec == "" || strings.ContainsAny("defgEG%F", spec) || sp.grouping == "_" && strings.ContainsAny("boxX", spec)
	if utf8.RuneCountInString(spec) > 1 || sp.grouping != "" && !groups {
		return sp, errSpec
	}
	return sp, nil
}

// formatInt is format_long_internal of CPython.
func formatInt(v int64, sp formatSpec) (string, error) {
	base := 10
	switch {
	case strings.ContainsAny("eEfFgG%", sp.typ):
		return "", unsupported(ErrUnsupportedConstruct, "format code of a float for an integer")
	case sp.typ != "" && !strings.ContainsAny("dboxXcn", sp.typ):
		return "", fmt.Errorf("%w: unknown format code of an integer", ErrRender)
	case sp.precision >= 0 || sp.z:
		return "", fmt.Errorf("%w: precision or z of an integer", ErrRender)
	case sp.typ == "c" || sp.typ == "n":
		return "", unsupported(ErrUnsupportedConstruct, "format code c or n")
	case sp.typ == "b":
		base = 2
	case sp.typ == "o":
		base = 8
	case sp.typ == "x" || sp.typ == "X":
		base = 16
	}
	prefix, digits := "", strings.TrimPrefix(strconv.FormatInt(v, base), "-")
	if sp.alt && base != 10 {
		prefix = "0" + sp.typ
	}
	if sp.typ == "X" {
		digits = strings.ToUpper(digits)
	}
	return number(v < 0, prefix, digits, sp), nil
}

// number is fill_number of CPython: with the fill "0" and "=" the padding is
// zeros grouped with the digits, as in format(1234567, "010_d"), 01_234_567.
func number(neg bool, prefix, digits string, sp formatSpec) string {
	sign := strings.TrimPrefix(sp.sign, "-")
	if neg {
		sign = "-"
	}
	if sp.align == "" && sp.zero {
		sp.align = "="
	}
	if sp.grouping != "" {
		minWidth := 0
		if sp.fill == "0" && sp.align == "=" {
			minWidth = sp.width - len(sign+prefix)
		}
		digits = group(digits, sp, minWidth)
	}
	return pad(sign+prefix, digits, sp, cmp.Or(sp.align, ">"))
}

// group is _PyUnicode_InsertThousandsGrouping of CPython: the zeros up to
// minWidth are digits too, the fewest that reach it with their separators, so
// "08,d" of 1234 is 0,001,234, 9 wide. One builder keeps the memory of a wide
// format within the byte budget.
func group(digits string, sp formatSpec, minWidth int) string {
	size := 3
	if strings.ContainsAny("boxX", sp.typ) {
		size = 4
	}
	digits = strings.Repeat("0", max(minWidth-(minWidth-1)/(size+1)-len(digits), 0)) + digits
	var b strings.Builder
	b.Grow(len(digits) + len(digits)/size)
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%size == 0 {
			b.WriteString(sp.grouping)
		}
		b.WriteByte(digits[i])
	}
	return b.String()
}

// pad counts code points, as len of a str of Python; "^" puts the odd fill on the right.
func pad(head, body string, sp formatSpec, align string) string {
	n, left := max(sp.width-utf8.RuneCountInString(head+body), 0), 0
	switch align {
	case "^":
		left = n / 2
	case ">":
		left = n
	case "=":
		head, n = head+strings.Repeat(sp.fill, n), 0
	}
	return strings.Repeat(sp.fill, left) + head + body + strings.Repeat(sp.fill, n-left)
}
