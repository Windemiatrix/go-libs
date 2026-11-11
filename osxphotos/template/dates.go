package template

import "strconv"

// DateTimeFormatter in the C locale of the reference (datetime_formatter.py:14-72); "" is the date (phototemplate.py:1781-1783).
var dateLayouts = map[string]string{
	"": "2006-01-02", "date": "2006-01-02", "yy": "06", "mm": "01", "month": "January", "mon": "Jan", "dd": "02",
	"dow": "Monday", "doy": "002", "hour": "15", "min": "04", "sec": "05",
}

// format_date_field (phototemplate.py:1772-1800); modified without a time is created (950-956).
func (s *rendering) date(name, family, part string, def []string) ([]string, error) {
	layout, ok := dateLayouts[part]
	if !ok && part != "year" && part != "strftime" || name == family+"." { // ValueError (1784-1785, 1797-1800)
		return nil, unsupported(ErrRender, name)
	}
	t := &s.o.Today
	if family != "today" {
		v, ok := s.p.Field(family)
		if ok && v.Time == nil && family == "modified" {
			return s.date(name, "created", part, def)
		} else if !ok {
			return nil, unsupported(ErrUnsupportedField, family)
		} else if t = v.Time; t == nil {
			return nil, nil
		}
	}
	switch {
	case part == "year": // f"{dt.year}" is not padded (datetime_formatter.py:19-22)
		return []string{strconv.Itoa(t.Year())}, nil
	case part != "strftime":
		return []string{t.Format(layout)}, nil
	case len(def) == 0: // args is the default (919, 951, 955); without it, None (1788-1789)
		return nil, nil
	}
	out, err := strftime(*t, def[0], family == "today", s.bytesLeft)
	if err != nil || out == "" { // "" is None (1012)
		return nil, err
	}
	return []string{out}, nil
}
