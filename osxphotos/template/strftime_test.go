package template

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// The values are those of datetime.strftime of CPython 3.12 in the C locale, except where noted.
func TestStrftime(t *testing.T) {
	t.Parallel()
	cet, plus2 := time.FixedZone("CET", 3600), time.FixedZone("", 7200)
	at := func(hour int, loc *time.Location) time.Time {
		return time.Date(2020, 3, 22, hour, 5, 9, 123456000, loc)
	}
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	// 22 March 2020 is a Sunday, the 82nd day of the year.
	for _, loc := range []*time.Location{plus2, cet} {
		for format, want := range map[string]string{
			"%Y": "2020", "%y": "20", "%m": "03", "%d": "22", "%H": "14", "%M": "05", "%S": "09", "%f": "123456",
			"%j": "082", "%B": "March", "%b": "Mar", "%A": "Sunday", "%a": "Sun", "%p": "PM", "%I": "02",
			"%U": "12", "%W": "11", "%w": "0", "%%": "%", "100%%": "100%", "plain": "plain", "": "",
			"%Y-%m-%d-%H%M%S": "2020-03-22-140509", "a%%Yb": "a%Yb", "\U000000e9%d\U000000e9": "\U000000e922\U000000e9",
		} {
			if got, err := strftime(at(14, loc), format, false, maxRenderBytes); err != nil || got != want {
				t.Errorf("strftime(%v, %q) = %q, %v; want %q", at(14, loc), format, got, err, want)
			}
		}
	}
	for _, tt := range []struct {
		t      time.Time
		format string
		want   string
	}{
		{at(14, plus2), "%z %Z", "+0200 UTC+02:00"},
		{at(14, cet), "%z %Z", "+0100 CET"},
		{at(14, time.FixedZone("+03", 10800)), "%z %Z", "+0300 +03"},
		{at(14, time.UTC), "%z %Z", "+0000 UTC"},
		{at(14, time.FixedZone("", 0)), "%z %Z", "+0000 UTC"},
		{at(14, time.FixedZone("", 3661)), "%z %Z", "+010101 UTC+01:01:01"},
		{at(14, time.FixedZone("", -(5*3600+1800))), "%z %Z", "-0530 UTC-05:30"},
		{at(14, time.FixedZone("x", 10800)), "%z %Z", "+0300 x"},
		{at(0, plus2), "%I %p", "12 AM"},
		{at(12, plus2), "%I %p", "12 PM"},
		{at(23, plus2), "%I %p %H", "11 PM 23"},
		{time.Date(2020, 3, 22, 1, 2, 3, 5000, plus2), "%f", "000005"},
		// %Y below 1000 is zero-padded by decision: upstream gives what the C library
		// of the platform gives, and glibc gives 999.
		{day(999, 1, 2), "%Y %y %j", "0999 99 002"},
		// 1 January: 2020 a Wednesday, 2023 a Sunday, 2024 a Monday
		{day(2020, 1, 1), "%U %W %w %j", "00 00 3 001"},
		{day(2023, 1, 1), "%U %W %w %a", "01 00 0 Sun"},
		{day(2024, 1, 1), "%U %W %w %A", "00 01 1 Monday"},
		{day(2020, 12, 31), "%U %W %j %B %b", "52 52 366 December Dec"},
	} {
		if got, err := strftime(tt.t, tt.format, false, maxRenderBytes); err != nil || got != tt.want {
			t.Errorf("strftime(%v, %q) = %q, %v; want %q", tt.t, tt.format, got, err, tt.want)
		}
	}
	for format, directive := range map[string]string{
		"%c": "%c", "%G": "%G", "%-d": "%-", "%Y%": "%", "%": "%", "%Y-%x": "%x", "%\U000000e9": "%\U000000e9",
	} {
		got, err := strftime(at(14, plus2), format, false, maxRenderBytes)
		if want := ErrUnsupportedConstruct.Error() + ": strftime " + directive; !errors.Is(err, ErrUnsupportedConstruct) || fmt.Sprint(err) != want || got != "" {
			t.Errorf("strftime(%q) = %q, %v; want %q", format, got, err, want)
		}
	}
	// "UTC+02:00UTC+02:00" is 18 bytes.
	for limit, want := range map[int]error{17: ErrTooManyValues, 18: nil} {
		if _, err := strftime(at(14, plus2), "%Z%Z", false, limit); !errors.Is(err, want) {
			t.Errorf("strftime(%q) with limit %d: %v; want %v", "%Z%Z", limit, err, want)
		}
	}
}

// test_template_strftime.py of osxphotos 0.78.1: the epoch of the date in its
// own zone, whatever the zone of the computer.
func TestStrftimeEpoch(t *testing.T) {
	t.Parallel()
	dst := time.Date(2022, 5, 24, 14, 20, 0, 0, time.FixedZone("", -7*3600))
	for _, tt := range []struct {
		t      time.Time
		format string
		want   string
	}{
		{dst, "%s", "1653427200"},
		{dst.In(time.FixedZone("IST", 19800)), "%s", "1653427200"},
		{dst, "%Y-%m-%d_%s", "2022-05-24_1653427200"},
		{dst, "%%s", "%s"},
		{dst, "%%%s", "%1653427200"},
		{dst, "%%s_%s", "%s_1653427200"},
		// str(int(dt.timestamp())): int cuts toward zero
		{time.Date(1969, 12, 31, 23, 59, 59, 500000000, time.UTC), "%s", "0"},
		{time.Date(1969, 12, 31, 23, 59, 58, 1000, time.UTC), "%s", "-1"},
		{time.Date(1969, 12, 31, 23, 59, 58, 0, time.UTC), "%s", "-2"},
		{time.Date(2020, 1, 1, 0, 0, 0, 999999000, time.UTC), "%s", "1577836800"},
	} {
		if got, err := strftime(tt.t, tt.format, false, maxRenderBytes); err != nil || got != tt.want {
			t.Errorf("strftime(%v, %q) = %q, %v; want %q", tt.t, tt.format, got, err, tt.want)
		}
	}
}

func TestRenderStrftime(t *testing.T) {
	t.Parallel()
	created := time.Date(2020, 3, 22, 14, 5, 9, 123456000, time.FixedZone("", 7200))
	modified := time.Date(2021, 11, 25, 6, 5, 4, 0, time.UTC)
	named := time.Date(2020, 3, 22, 14, 5, 9, 0, time.FixedZone("+03", 10800))
	p := func(modified *time.Time) fakePhoto {
		return fakePhoto{"created": {Time: &created}, "modified": {Time: modified}, "keyword": strs("%Y", "%m")}
	}
	today := RenderOptions{Today: created}
	monday := RenderOptions{Today: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)}
	o := RenderOptions{}
	one := func(s string) []string { return []string{s} }
	checkRender(t, []renderCase{
		{"t034", "{created.month}", p(nil), o, one("March"), nil},
		{"t035", "{created.mon}", p(nil), o, one("Mar"), nil},
		{"t037", "{created.dow}", p(nil), o, one("Sunday"), nil},
		{"t048", "{modified.month}", p(&modified), o, one("November"), nil},
		{"t049", "{modified.mon}", p(&modified), o, one("Nov"), nil},
		{"t051", "{modified.dow}", p(&modified), o, one("Thursday"), nil},
		{"t051 without modified", "{modified.dow}", p(nil), o, one("Sunday"), nil},
		{"today.dow", "{today.dow}", fakePhoto{}, monday, one("Monday"), nil},
		{"today.month", "{today.month}-{today.mon}", fakePhoto{}, monday, one("January-Jan"), nil},
		// phototemplate.py:1787-1791: the format is the first value of the default
		{"t042", "{created.strftime}", p(nil), o, one("_"), nil},
		{"t056", "{modified.strftime}", p(&modified), o, one("_"), nil},
		{"t057", "{created.strftime,%Y-%m-%d-%H%M%S}", p(nil), o, one("2020-03-22-140509"), nil},
		{"t058", "{created.strftime,%Y-%U}", p(nil), o, one("2020-12"), nil},
		{"t059", "{created.strftime,%y %B %b}", p(nil), o, one("20 March Mar"), nil},
		{"t060", "{created.strftime,%A %a %j}", p(nil), o, one("Sunday Sun 082"), nil},
		{"t061", "{created.strftime,%I:%M:%S %p}", p(nil), o, one("02:05:09 PM"), nil},
		{"t062", "{created.strftime,%H:%M:%S.%f}", p(nil), o, one("14:05:09.123456"), nil},
		{"t063", "{created.strftime,%W %w %U}", p(nil), o, one("11 0 12"), nil},
		{"zone without a name", "{created.strftime,%z %Z}", p(nil), o, one("+0200 UTC+02:00"), nil},
		{"t064 named zone", "{created.strftime,%z %Z}", fakePhoto{"created": {Time: &named}}, o, one("+0300 +03"), nil},
		{"t065", "{created.strftime,100%%}", p(nil), o, one("100%"), nil},
		{"t066", "{created.strftime,{created.year}x%m}", p(nil), o, one("2020x03"), nil},
		{"t067", "{created.strftime,plain}", p(nil), o, one("plain"), nil},
		{"t069", "{modified.strftime,%Y-%m-%d-%H%M%S}", p(&modified), o, one("2021-11-25-060504"), nil},
		{"t069 without modified", "{modified.strftime,%Y}", p(nil), o, one("2020"), nil},
		{"t071", "{modified.strftime,%z %Z}", p(&modified), o, one("+0000 UTC"), nil},
		{"epoch", "{created.strftime,%s}", p(nil), o, one("1584878709"), nil},
		{"year and epoch", "{created.strftime,%Y_%s}", p(nil), o, one("2020_1584878709"), nil},
		{"escaped epoch", "{created.strftime,%%s}", p(nil), o, one("%s"), nil},
		{"modified epoch", "{modified.strftime,%s}", p(&modified), o, one("1637820304"), nil},
		{"modified epoch without modified", "{modified.strftime,%s}", p(nil), o, one("1584878709"), nil},
		{"today epoch", "{today.strftime,%s}", fakePhoto{}, today, one("1584878709"), nil},
		{"today", "{today.strftime,%Y-%U}", fakePhoto{}, today, one("2020-12"), nil},
		// self.today is a naive datetime.now() (phototemplate.py:910-912)
		{"today without a zone", "{today.strftime,%z %Z}", fakePhoto{}, today, one(" "), nil},
		{"today zone only", "{today.strftime,%z%Z}", fakePhoto{}, today, one("%z%Z"), nil},
		{"today year and zone", "{today.strftime,%Y%z}", fakePhoto{}, today, one("2020"), nil},
		{"first value of the default", "{created.strftime,{keyword}}", p(nil), o, one("2020"), nil},
		{"created without a time", "{created.strftime}", fakePhoto{"created": {}}, o, one("_"), nil},
		// without values the field takes its default (phototemplate.py:770-772)
		{"created without a time and a format", "{created.strftime,%Y}", fakePhoto{"created": {}}, o, one("%Y"), nil},
		// "" is None (phototemplate.py:1012): the bool takes the default
		{"empty format", "{created.strftime,}", p(nil), o, one(""), nil},
		{"empty format and a bool", "{created.strftime?Y,}", p(nil), o, one(""), nil},
		{"format and a bool", "{created.strftime?Y,%Y}", p(nil), o, one("Y"), nil},
	})
}

func TestRenderStrftimeErrors(t *testing.T) {
	t.Parallel()
	created := time.Date(2020, 3, 22, 14, 5, 9, 0, time.UTC)
	p := fakePhoto{"created": {Time: &created}}
	for in, tt := range map[string]struct {
		photo fakePhoto
		want  error
		names string
	}{
		"{today.strftime,%c}":     {fakePhoto{}, ErrUnsupportedConstruct, "strftime %c"},
		"{created.strftime,%Y%}":  {p, ErrUnsupportedConstruct, "strftime %"},
		"{created.strftime,%-d}":  {p, ErrUnsupportedConstruct, "strftime %-"},
		"{modified.strftime,%Y}":  {p, ErrUnsupportedField, "modified"},
		"{created.strftime,%Y}":   {fakePhoto{}, ErrUnsupportedField, "created"},
		"{created.month}":         {fakePhoto{}, ErrUnsupportedField, "created"},
		"{created.strftime.x,%Y}": {p, ErrRender, "created.strftime.x"},
		"{today.strftime.}":       {fakePhoto{}, ErrRender, "today.strftime."},
	} {
		got, _, err := render(t, in, tt.photo, RenderOptions{})
		if want := tt.want.Error() + ": " + tt.names; !errors.Is(err, tt.want) || fmt.Sprint(err) != want || got != nil {
			t.Errorf("Render(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
