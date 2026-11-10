package template

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

// The values are those of format(value, spec) of CPython 3.12.
func TestFormatValue(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		v    any
		spec string
		want string
	}{
		// integers
		{int64(42), "05d", "00042"},
		{int64(42), "d", "42"},
		{int64(42), "5", "   42"},
		{int64(255), "x", "ff"},
		{int64(255), "X", "FF"},
		{int64(8), "o", "10"},
		{int64(10), "b", "1010"},
		{int64(255), "#x", "0xff"},
		{int64(255), "#X", "0XFF"},
		{int64(8), "#o", "0o10"},
		{int64(5), "#b", "0b101"},
		{int64(42), "#d", "42"},
		{int64(42), "+#x", "+0x2a"},
		{int64(42), "^7", "  42   "},
		{int64(42), "~^9", "~~~42~~~~"},
		{int64(42), "\U000000e9^6", "\U000000e9\U000000e942\U000000e9\U000000e9"},
		{int64(42), "<<5", "42<<<"},
		{int64(42), ">>5", ">>>42"},
		{int64(42), "-d", "42"},
		{int64(42), "+d", "+42"},
		{int64(42), " d", " 42"},
		{int64(-42), "+d", "-42"},
		{int64(-42), " d", "-42"},
		{int64(-42), "05d", "-0042"},
		{int64(-42), "x", "-2a"},
		{int64(-255), "#x", "-0xff"},
		{int64(-255), "*=#8x", "-0x***ff"},
		{int64(255), "#010x", "0x000000ff"},
		{int64(1000000), "_d", "1_000_000"},
		{int64(1234567), "010_d", "01_234_567"},
		{int64(1234567), ",d", "1,234,567"},
		{int64(1234567), ",", "1,234,567"},
		{int64(0xABCDEF), "_x", "ab_cdef"},
		{int64(0xABCDEF), "#_X", "0XAB_CDEF"},
		{int64(255), "_b", "1111_1111"},
		{int64(255), "#012_x", "0x0_0000_00ff"},
		{int64(1234), "08,d", "0,001,234"},
		{int64(1234), "07,d", "001,234"},
		{int64(-1234), "08,d", "-001,234"},
		{int64(1234567), "*=12,d", "***1,234,567"},
		{int64(1234567), "<010,d", "1,234,5670"},
		{int64(42), "<5", "42   "},
		{int64(42), ">5", "   42"},
		{int64(42), "<05", "42000"},
		{int64(42), "x<05", "42xxx"},
		{int64(42), "^05", "04200"},
		{int64(42), "=5", "   42"},
		{int64(-42), "=5", "-  42"},
		{int64(42), "0=5", "00042"},
		{int64(42), "00", "42"},
		{int64(0), "05d", "00000"},
		{int64(42), "", "42"},
		{int64(math.MinInt64), "", "-9223372036854775808"},
		{int64(math.MinInt64), "x", "-8000000000000000"},
		{int64(42), "f", "42.000000"},
		{int64(5), ".2f", "5.00"},
		{int64(42), "e", "4.200000e+01"},
		{int64(42), "%", "4200.000000%"},
		{int64(42), "E", "4.200000E+01"},
		{int64(42), "F", "42.000000"},
		{int64(42), "g", "42"},
		{int64(42), "G", "42"},
		// floats
		{3.14159, ".1f", "3.1"},
		{3.14159, "08.3f", "0003.142"},
		{12345.678, ".2e", "1.23e+04"},
		{0.00001234, "g", "1.234e-05"},
		{12345.678, "E", "1.234568E+04"},
		{1.5, "F", "1.500000"},
		{0.5, "%", "50.000000%"},
		{0.5, ".0%", "50%"},
		{0.5, "#.0%", "50.%"},
		{2.0, "f", "2.000000"},
		{3.14159, "10.2f", "      3.14"},
		{3.14159, "^10.2f", "   3.14   "},
		{3.14159, ".3", "3.14"},
		{1.0, "g", "1"},
		{1.0, "#g", "1.00000"},
		{1.0, ".0g", "1"},
		{2.5, ".0g", "2"},
		{0.0001, "g", "0.0001"},
		{100.0, ".2g", "1e+02"},
		{123456789.0, "g", "1.23457e+08"},
		{0.0, "g", "0"},
		{0.000123456, ".3G", "0.000123"},
		{1.5e-10, "G", "1.5E-10"},
		{1e16, "", "1e+16"},
		{0.5, "", "0.5"},
		{2.0, "", "2.0"},
		{2.0, ".3", "2.0"},
		{2.0, ".0", "2e+00"},
		{2.5, ".0", "2e+00"},
		{123.0, ".3", "1.23e+02"},
		{1234.5, ".3", "1.23e+03"},
		{2.0, "#.3", "2.00"},
		{1.0, "#.0f", "1."},
		{1.0, "#.0e", "1.e+00"},
		{1.0, "#.1g", "1."},
		{0.5, ".0f", "0"},
		{2.5, ".0f", "2"},
		{1.5, "e", "1.500000e+00"},
		{1e100, ".1e", "1.0e+100"},
		{1234567.891, ",.2f", "1,234,567.89"},
		{1234.5678, "012,.2f", "0,001,234.57"},
		{1234.5, "_", "1_234.5"},
		{math.Copysign(0, -1), "f", "-0.000000"},
		{-1.5, "+.1f", "-1.5"},
		{1.5, "+.1f", "+1.5"},
		{1.5, " .1f", " 1.5"},
		{-1.5, "08.2f", "-0001.50"},
		{math.Inf(1), "f", "inf"},
		{math.Inf(1), "F", "INF"},
		{math.Inf(-1), "", "-inf"},
		{math.Inf(1), "%", "inf%"},
		{math.Inf(-1), "08f", "-0000inf"},
		{math.Inf(1), "010,f", "0000000inf"},
		{math.Inf(1), "#f", "inf"},
		{math.NaN(), "g", "nan"},
		{math.NaN(), "#g", "nan"},
		{math.NaN(), "+f", "+nan"},
		{math.Copysign(math.NaN(), -1), "f", "nan"},
		{math.NaN(), "E", "NAN"},
		// strings
		{"abc", "s", "abc"},
		{"mid", "^9", "   mid   "},
		{"mid", "~^9", "~~~mid~~~"},
		{"abcdef", ".3", "abc"},
		{"abc", "10", "abc       "},
		{"mid", "#^9", "###mid###"},
		{"abc", "^4", "abc "},
		{"abc", "<5s", "abc  "},
		{"ab", "05", "ab000"},
		{"x", "\U000000e9^3", "\U000000e9x\U000000e9"},
		{"h\U000000e9llo", ".2", "h\U000000e9"},
		{"\U000000e9", ">3", "  \U000000e9"},
		{"abc", ".0", ""},
		{"abc", "", "abc"},
		{"", "", ""},
	} {
		s := &rendering{bytesLeft: maxRenderBytes}
		got, err := s.formatValue(tt.v, tt.spec)
		if err != nil || got != tt.want || s.bytesLeft != maxRenderBytes-len(tt.want) {
			t.Errorf("format(%#v, %q) = %q, %v, %d bytes left; want %q, %d bytes left",
				tt.v, tt.spec, got, err, s.bytesLeft, tt.want, maxRenderBytes-len(tt.want))
		}
	}
}

func TestFormatValueErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		v    any
		spec string
		want error
	}{
		{int64(255), ",x", ErrRender},
		{int64(255), ",b", ErrRender},
		{int64(42), ".2d", ErrRender},
		{int64(42), ".2", ErrRender},
		{int64(42), "q", ErrRender},
		{int64(42), "s", ErrRender},
		{int64(42), "\xff", ErrRender},
		{int64(42), "dd", ErrRender},
		{int64(42), ".", ErrRender},
		{int64(42), "5.", ErrRender},
		{int64(42), ",_d", ErrRender},
		{int64(42), "_,d", ErrRender},
		{int64(42), "99999999999999999999", ErrRender},
		{int64(42), "c", ErrUnsupportedConstruct},
		{int64(42), "n", ErrUnsupportedConstruct},
		{int64(42), "z", ErrRender},
		{int64(42), "zc", ErrRender},
		{int64(42), "zf", ErrUnsupportedConstruct},
		{1.0, "z.2f", ErrUnsupportedConstruct},
		{1.0, "z+f", ErrRender},
		{1.0, ".99999999999999999999f", ErrRender},
		{1.0, ".f", ErrRender},
		{1.0, "5.e", ErrRender},
		{3.5, "d", ErrRender},
		{1.0, "x", ErrRender},
		{1.0, "c", ErrRender},
		{1.0, "s", ErrRender},
		{1.0, "n", ErrUnsupportedConstruct},
		{"abc", "+s", ErrRender},
		{"abc", "#", ErrRender},
		{"abc", "=5", ErrRender},
		{"abc", ",", ErrRender},
		{"abc", "_", ErrRender},
		{"abc", "d", ErrRender},
		{"abc", "c", ErrRender},
		{"abc", "n", ErrRender},
		{"abc", "z", ErrRender},
	} {
		s := &rendering{bytesLeft: maxRenderBytes}
		if got, err := s.formatValue(tt.v, tt.spec); !errors.Is(err, tt.want) || got != "" {
			t.Errorf("format(%#v, %q) = %q, %v; want %v", tt.v, tt.spec, got, err, tt.want)
		}
	}
}

// The width and the precision of a number come from the template and are
// charged before the string is built: 123 of width 4 with a fill of two bytes
// is 5 bytes, but 4 fills would be 8. A precision only cuts a string.
func TestFormatValueBudget(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		v    any
		spec string
		left int
		want error
	}{
		{int64(1), "10", 10, nil},
		{int64(1), "11", 10, ErrTooManyValues},
		{int64(1), "\U000000e9<5", 10, nil},
		{int64(1), "\U000000e9<6", 10, ErrTooManyValues},
		{int64(123), "\U000000e9<4", 7, ErrTooManyValues},
		{int64(123), "\U000000e9<4", 8, nil},
		{int64(123456), "", 5, ErrTooManyValues},
		{int64(1), "999999999", maxRenderBytes, ErrTooManyValues},
		{"abcdef", ".99999999", 10, nil},
		{1.0, ".8f", 10, nil},
		{1.0, ".20g", 10, ErrTooManyValues},
		{1.0, ".999999999f", maxRenderBytes, ErrTooManyValues},
	} {
		s := &rendering{bytesLeft: tt.left}
		got, err := s.formatValue(tt.v, tt.spec)
		if !errors.Is(err, tt.want) || tt.want == nil && s.bytesLeft != tt.left-len(got) {
			t.Errorf("format(%#v, %q) with %d bytes left = %q, %v, %d bytes left; want %v",
				tt.v, tt.spec, tt.left, got, err, s.bytesLeft, tt.want)
		}
	}
}

// The zeros of a wide grouped format are built at once: one allocation per
// group would be a quarter of a million for a width of 1,000,000. Not
// parallel, since AllocsPerRun counts the allocations of the whole process.
func TestFormatValueGroupAllocs(t *testing.T) {
	s := &rendering{}
	allocs := testing.AllocsPerRun(1, func() {
		s.bytesLeft = maxRenderBytes
		if _, err := s.formatValue(int64(1), "01000000_d"); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 10 {
		t.Errorf("format(1, \"01000000_d\") made %v allocations, want at most 10", allocs)
	}
}

func TestRenderFormat(t *testing.T) {
	t.Parallel()
	o, none := RenderOptions{}, fakePhoto{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"t344", "{format:int:05d,42}", none, o, one("00042"), nil},
		{"t357 cuts a decimal", "{format:int:05d,3.9}", none, o, one("00003"), nil},
		{"int cuts toward zero", "{format:int:d,-3.9}", none, o, one("-3"), nil},
		{"int of an exponent", "{format:int:d,1e3}", none, o, one("1000"), nil},
		{"int of spaces", "{format:int:d, 7 }", none, o, one("7"), nil},
		{"int at the bound", "{format:int:d,-9223372036854775808}", none, o, one("-9223372036854775808"), nil},
		{"t377 float of a field", "{format:float:.2f,{created.year}}", conditionPhoto(), o, one("2019.00"), nil},
		{"str of a field", "{format:str:~^8,{title}}", conditionPhoto(), o, one("~Beach~~"), nil},
		{"str of a long precision", "{format:str:.99999999,abc}", none, o, one("abc"), nil},
		{"every value in order", "{format:int:03d,{keyword}}", fakePhoto{"keyword": strs("1", "22")}, o, []string{"001", "022"}, nil},
		{"sign through a variable", "{var:f,+.1f}{format:float:%f,{keyword}}", fakePhoto{"keyword": strs("3", "-2")}, o,
			[]string{"+3.0", "-2.0"}, nil},
		{"empty str", "{format:str:s,}", none, o, one(""), nil},
		{"no values", "{format:int:d}", none, o, one("_"), nil},
		// expand_variables_to_str of the format (phototemplate.py:1422)
		{"variable in the value", "{var:v,42}{format:int:05d,{%v}}", none, o, one("00042"), nil},
		{"variable in the format", "{var:f,05d}{format:int:%f,42}", none, o, one("00042"), nil},
		// a subfield takes no "," and no "<", ">", "=", "+": only a variable gives them
		{"grouping through a variable", "{var:f,{comma}d}{format:int:%f,1234567}", none, o, one("1,234,567"), nil},
		{"alignment through a variable", "{var:f,>6}{format:int:%f,42}", none, o, one("    42"), nil},
		{"unmatched", "{format.x:int:d,1}", none, o, nil, []string{"format.x"}},
		// format_str_value of the primary key (phototemplate.py:977-978)
		{"id zero padded", "{id:05d}", examplePhoto(), o, one("00042"), nil},
		{"id hex", "{id:x}", examplePhoto(), o, one("2a"), nil},
		{"id alt hex", "{id:#x}", examplePhoto(), o, one("0x2a"), nil},
		{"id centered", "{id:~^9}", examplePhoto(), o, one("~~~42~~~~"), nil},
	})
}

func TestRenderFormatErrors(t *testing.T) {
	t.Parallel()
	ex := examplePhoto()
	for _, tt := range []struct {
		in    string
		photo fakePhoto
		want  error
		names string
	}{
		// SyntaxError (phototemplate.py:1408-1414)
		{"{format}", fakePhoto{}, ErrRender, "format"},
		{"{format:int}", fakePhoto{}, ErrRender, "format"},
		{"{format:bool:d,1}", fakePhoto{}, ErrRender, "format"},
		// int(float(v)) and float(v) (1417-1419)
		{"{format:int:d,x}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,inf}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,nan}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,1e30}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,-1e30}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,-inf}", fakePhoto{}, ErrRender, "format:int"},
		// as a float, 9223372036854775807 is 2**63, past int64
		{"{format:int:d,9223372036854775807}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:int:d,9223372036854775808}", fakePhoto{}, ErrRender, "format:int"},
		{"{format:float:f,x}", fakePhoto{}, ErrRender, "format:float"},
		{"{format:float:d,1}", fakePhoto{}, ErrRender, "{format:float:d}"},
		{"{format:int:c,1}", fakePhoto{}, ErrUnsupportedConstruct, "{format:int:c}"},
		{"{format:int:%nope,1}", fakePhoto{}, ErrRender, `"nope"`},
		// str.format reads a brace of the format (phototemplate.py:1664-1665)
		{"{var:f,{closebrace}^9}{format:int:%f,42}", fakePhoto{}, ErrRender, "{format:int:%f}"},
		{"{var:f,{openbrace}^9}{format:int:%f,42}", fakePhoto{}, ErrRender, "{format:int:%f}"},
		{"{format:str:999999999,x}", fakePhoto{}, ErrTooManyValues, "{format:str:999999999}"},
		{"{var:w,>999999999}{format:int:%w,1}", fakePhoto{}, ErrTooManyValues, "{format:int:%w}"},
		{"{format:float:.999999999f,1}", fakePhoto{}, ErrTooManyValues, "bytes"},
		{"{id:.2d}", ex, ErrRender, "{id:.2d}"},
		{"{id:05d}", fakePhoto{}, ErrUnsupportedField, "id"},
		{"{id:05d}", fakePhoto{"id": strs("42")}, ErrUnsupportedField, "id"},
	} {
		got, _, err := render(t, tt.in, tt.photo, RenderOptions{})
		if !errors.Is(err, tt.want) || !strings.Contains(fmt.Sprint(err), tt.names) || got != nil {
			t.Errorf("Render(%q) = %q, %v; want an error that wraps %v and names %q", tt.in, got, err, tt.want, tt.names)
		}
	}
}
