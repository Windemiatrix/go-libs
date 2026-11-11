package template

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The directives of C strftime in the C locale, the locale of the reference;
// upstream takes the locale of the user (phototemplate.py:50).
var strftimeLayouts = map[rune]string{
	'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'H': "15", 'M': "04", 'S': "05", 'j': "002",
	'B': "January", 'b': "Jan", 'A': "Monday", 'a': "Mon", 'p': "PM", 'I': "03",
}

// strftime is _strftime (phototemplate.py:1749-1769, 1791); upstream hands a
// directive outside the table to the C library of the platform, so here it is
// a construct not ported. naive is a datetime without tzinfo, as the
// datetime.now() of {today} (910-912), whose %z and %Z are empty. limit is
// what is left of maxRenderBytes, a bound upstream does not have.
func strftime(t time.Time, format string, naive bool, limit int) (string, error) {
	var b []byte
	yday, wday := t.YearDay()-1, int(t.Weekday())
	for {
		text, rest, found := strings.Cut(format, "%")
		if b = append(b, text...); len(b) > limit {
			return "", errTooLong
		} else if !found {
			return string(b), nil
		} else if rest == "" {
			return "", unsupported(ErrUnsupportedConstruct, "strftime %")
		}
		d, n := utf8.DecodeRuneInString(rest)
		format = rest[n:]
		switch {
		case strftimeLayouts[d] != "":
			b = t.AppendFormat(b, strftimeLayouts[d])
		case d == 'f':
			b = fmt.Appendf(b, "%06d", t.Nanosecond()/1000)
		case d == 'U':
			b = fmt.Appendf(b, "%02d", (yday+7-wday)/7)
		case d == 'W':
			b = fmt.Appendf(b, "%02d", (yday+7-(wday+6)%7)/7)
		case d == 'w':
			b = strconv.AppendInt(b, int64(wday), 10)
		case d == 's': // int(dt.timestamp()) cuts toward zero, Unix floors (1764)
			sec := t.Unix()
			if sec < 0 && t.Nanosecond() != 0 {
				sec++
			}
			b = strconv.AppendInt(b, sec, 10)
		case d == 'z' || d == 'Z':
			if !naive {
				b = append(b, zone(t, d)...)
			}
		case d == '%':
			b = append(b, '%')
		default:
			return "", unsupported(ErrUnsupportedConstruct, "strftime %"+string(d))
		}
	}
}

// zone is %z or %Z of a datetime with a datetime.timezone, the zone photosdb
// gives to a date without a zone name: Python prints the seconds of an offset
// that has them, and names such a zone UTC±HH:MM, or UTC without an offset
// (timezone_str of CPython 3.12 Modules/_datetimemodule.c).
func zone(t time.Time, directive rune) string {
	name, off := t.Zone()
	layout := "-07:00"
	if off%60 != 0 {
		layout = "-07:00:00"
	}
	switch offset := t.Format(layout); {
	case directive == 'z':
		return strings.ReplaceAll(offset, ":", "")
	case name != "":
		return name
	case off == 0:
		return "UTC"
	default:
		return "UTC" + offset
	}
}
