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
		v    int64
		spec string
		want string
	}{
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
		v    int64
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
		{int64(42), "e", ErrUnsupportedConstruct},
		{int64(42), "E", ErrUnsupportedConstruct},
		{int64(42), "f", ErrUnsupportedConstruct},
		{int64(42), "F", ErrUnsupportedConstruct},
		{int64(42), "g", ErrUnsupportedConstruct},
		{int64(42), "G", ErrUnsupportedConstruct},
		{int64(42), "%", ErrUnsupportedConstruct},
	} {
		s := &rendering{bytesLeft: maxRenderBytes}
		if got, err := s.formatValue(tt.v, tt.spec); !errors.Is(err, tt.want) || got != "" {
			t.Errorf("format(%#v, %q) = %q, %v; want %v", tt.v, tt.spec, got, err, tt.want)
		}
	}
}

// The width comes from the template and is charged before the string is
// built: 123 of width 4 with a fill of two bytes is 5 bytes, but 4 fills would
// be 8.
func TestFormatValueBudget(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		v    int64
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
	o := RenderOptions{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
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
