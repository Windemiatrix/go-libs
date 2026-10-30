package template

import "strconv"

// DateTimeFormatter without the locale (datetime_formatter.py:14-72); "" is the date (phototemplate.py:1752-1754).
var dateLayouts = map[string]string{
	"": "2006-01-02", "date": "2006-01-02", "yy": "06", "mm": "01", "dd": "02",
	"doy": "002", "hour": "15", "min": "04", "sec": "05",
}

// format_date_field (phototemplate.py:1743-1769); modified without a time is created (950-956).
func (s *rendering) date(name, family, part string) ([]string, error) {
	layout, ok := dateLayouts[part]
	if part == "strftime" || part == "month" || part == "mon" || part == "dow" {
		return nil, unsupported(ErrUnsupportedConstruct, name)
	} else if !ok && part != "year" || name == family+"." { // ValueError (1755-1756, 1766-1769)
		return nil, unsupported(ErrRender, name)
	}
	t := &s.o.Today
	if family != "today" {
		v, ok := s.p.Field(family)
		if ok && v.Time == nil && family == "modified" {
			return s.date(name, "created", part)
		} else if !ok {
			return nil, unsupported(ErrUnsupportedField, family)
		} else if t = v.Time; t == nil {
			return nil, nil
		}
	}
	if part == "year" { // f"{dt.year}" is not padded (datetime_formatter.py:19-22)
		return []string{strconv.Itoa(t.Year())}, nil
	}
	return []string{t.Format(layout)}, nil
}
