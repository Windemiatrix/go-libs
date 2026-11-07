package template

import (
	"cmp"
	"errors"
	"fmt"
	"math"
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

// format is get_format_values (phototemplate.py:1397-1420).
func (s *rendering) format(f *Field, def []string) ([]string, error) {
	typ, spec, ok := strings.Cut(f.Subfield, ":")
	if !ok || typ != "int" && typ != "float" && typ != "str" { // SyntaxError (1405-1411)
		return nil, fmt.Errorf("%w: format takes {format:TYPE:FORMAT,values}, TYPE int, float or str", ErrRender)
	}
	spec, err := s.expand(spec, "format string")
	if err == nil && strings.ContainsAny(spec, "{}") { // str.format reads the braces as its own syntax, a field or a literal (1661-1662)
		err = errSpec
	}
	if err != nil {
		return nil, inField(err, f)
	}
	out := make([]string, len(def))
	for i, v := range def {
		x, ok := pyFloat(v)
		switch {
		case typ == "str":
			out[i], err = s.formatValue(v, spec)
		// int(float(v)) (1414): ValueError of NaN, OverflowError of an infinity, and an int of Python is not bounded by int64
		case !ok || typ == "int" && (math.IsNaN(x) || x < -(1<<63) || x >= 1<<63):
			err = fmt.Errorf("%w: format:%s of a value that is not a number of that type", ErrRender, typ)
		case typ == "int":
			out[i], err = s.formatValue(int64(x), spec)
		default:
			out[i], err = s.formatValue(x, spec)
		}
		if err != nil {
			return nil, inField(err, f)
		}
	}
	return out, nil
}

// inField names the format as the template writes it: the expanded format may
// hold the value of a variable, such as a title.
func inField(err error, f *Field) error {
	if err != nil {
		err = fmt.Errorf("%w in {%s:%s}", err, f.Name, f.Subfield)
	}
	return err
}

// formatValue is format_str_value (phototemplate.py:1657-1662): format(v, spec)
// of Python, str(v) for "". v is an int64, a float64 or a string; the caller
// converts any other integer to int64. A width, or the precision of a number,
// beyond the bytes left to the render is errTooLong before anything is built,
// since the template sets them and upstream has no bound.
func (s *rendering) formatValue(v any, spec string) (string, error) {
	sp, err := parseSpec(spec)
	_, isStr := v.(string)
	if err != nil {
		return "", err
	} else if sp.width > s.bytesLeft/len(sp.fill) || !isStr && sp.precision > s.bytesLeft { // a precision only cuts a string
		return "", errTooLong
	}
	var out string
	switch v := v.(type) {
	case int64:
		out, err = formatInt(v, sp)
	case float64:
		out, err = formatFloat(v, sp)
	default:
		out, err = formatString(v.(string), sp)
	}
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
		return formatFloat(float64(v), sp)
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
	return number(v < 0, prefix, digits, "", sp), nil
}

// formatFloat is format_float_internal of CPython.
func formatFloat(f float64, sp formatSpec) (string, error) {
	switch {
	case sp.typ != "" && !strings.ContainsAny("eEfFgGn%", sp.typ):
		return "", fmt.Errorf("%w: unknown format code of a float", ErrRender)
	case sp.typ == "n" || sp.z: // z, the coercion of -0.0, is of Python 3.11, and osxphotos runs on 3.10 too
		return "", unsupported(ErrUnsupportedConstruct, "format code n or z")
	}
	neg, p := math.Signbit(f) && !math.IsNaN(f), sp.precision // Python prints no sign of a NaN
	if p < 0 {
		p = 6
	}
	if f = math.Abs(f); sp.typ == "%" {
		f *= 100
	}
	var s string
	switch t := strings.ToLower(sp.typ); {
	case math.IsInf(f, 0) || math.IsNaN(f) || t == "" && sp.precision < 0:
		s = FloatString(f)
	case t == "e":
		s = strconv.FormatFloat(f, 'e', p, 64)
	case t == "f" || t == "%":
		s = strconv.FormatFloat(f, 'f', p, 64)
	case t == "g":
		s = pyG(f, max(p, 1), max(p, 1), sp.alt)
	default: // a fixed number whose ".0" would exceed the precision takes the exponent
		if s = pyG(f, max(p, 1), max(p, 1)-1, sp.alt); !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
	}
	if sp.alt && !strings.ContainsAny(s, ".in") {
		i := strings.IndexByte(s+"e", 'e')
		s = s[:i] + "." + s[i:]
	}
	if sp.typ == "%" {
		s += "%"
	}
	if strings.ContainsAny("EFG", sp.typ) {
		s = strings.ToUpper(s)
	}
	i := len(s) - len(strings.TrimLeft(s, "0123456789"))
	return number(neg, "", s[:i], s[i:], sp), nil
}

// pyG is the code 'g' of format_float_short of CPython (Python/pystrtod.c).
func pyG(f float64, p, limit int, alt bool) string {
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', p-1, 64), "e")
	if e, _ := strconv.Atoi(exp); -4 <= e && e < limit {
		mant, exp = strconv.FormatFloat(f, 'f', p-1-e, 64), ""
	} else {
		exp = "e" + exp
	}
	if !alt && strings.Contains(mant, ".") {
		mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
	}
	return mant + exp
}

// number is fill_number of CPython: with the fill "0" and "=" the padding is
// zeros grouped with the digits, as in format(1234567, "010_d"), 01_234_567.
func number(neg bool, prefix, digits, tail string, sp formatSpec) string {
	sign := strings.TrimPrefix(sp.sign, "-")
	if neg {
		sign = "-"
	}
	if sp.align == "" && sp.zero {
		sp.align = "="
	}
	if sp.grouping != "" && digits != "" { // inf has no digits to group
		minWidth := 0
		if sp.fill == "0" && sp.align == "=" {
			minWidth = sp.width - len(sign+prefix+tail)
		}
		digits = group(digits, sp, minWidth)
	}
	return pad(sign+prefix, digits+tail, sp, cmp.Or(sp.align, ">"))
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

// formatString is format_string_internal of CPython; the precision counts code points.
func formatString(v string, sp formatSpec) (string, error) {
	switch {
	case sp.typ != "" && sp.typ != "s":
		return "", fmt.Errorf("%w: unknown format code of a string", ErrRender)
	case sp.sign != "" || sp.z || sp.alt || sp.align == "=" || sp.grouping != "":
		return "", fmt.Errorf("%w: sign, z, #, = or grouping of a string", ErrRender)
	}
	for i := range v {
		if sp.precision == 0 {
			v = v[:i]
			break
		}
		sp.precision--
	}
	return pad("", v, sp, cmp.Or(sp.align, "<")), nil
}
