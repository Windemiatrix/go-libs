package template

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FILTER_VALUES without the arguments (phototemplate.py:278-318, 1058).
var filterNames = set(`lower upper strip titlecase capitalize braces parens brackets shell_quote function split
	autosplit chop chomp sort rsort reverse uniq join append prepend appends prepends remove slice sslice filter path int float`)

// phototemplate.py:1065-1078.
var needsArgs = set("append appends chomp chop filter prepend prepends remove slice split sslice")

// phototemplate.py:1080-1097.
var valueFilters = map[string]func(string) string{
	"lower": pyLower, "upper": pyUpper, "capitalize": pyCapitalize, "titlecase": pyTitle, "shell_quote": shellQuote,
	"strip": func(v string) string { return strings.TrimFunc(v, isSpace) }, "braces": func(v string) string { return "{" + v + "}" },
	"parens": func(v string) string { return "(" + v + ")" }, "brackets": func(v string) string { return "[" + v + "]" },
}

// re.search of phototemplate.py:1050: "." of RE2, as of Python, stops at "\n".
var filterArgsRe = regexp.MustCompile(`\(.*\)`)

// The bound on the values is not upstream's: split can make one value into many (phototemplate.py:669-670).
func (s *rendering) filters(names, vals []string) ([]string, error) {
	for _, name := range names {
		var err error
		if vals, err = s.filter(name, vals); err != nil {
			return nil, err
		} else if len(vals) > s.o.MaxResults {
			return nil, fmt.Errorf("%w: more than %d", ErrTooManyValues, s.o.MaxResults)
		}
	}
	return vals, nil
}

// get_filter_values (phototemplate.py:1046-1184). vals belong to the field
// being rendered, so a filter may change them in place.
func (s *rendering) filter(name string, vals []string) ([]string, error) {
	if strings.HasPrefix(name, "function:") { // runs Python code (1180-1181): its arguments are not even expanded
		return nil, unsupported(ErrUnsupportedField, "filter function")
	}
	var arg string
	if filterArgsRe.MatchString(name) {
		name, arg, _ = strings.Cut(name, "(")
		arg = strings.TrimRight(arg, ")")
	}
	before, _, _ := strings.Cut(name, ":")
	family, _, _ := strings.Cut(name, ".")
	if !filterNames[before] && !filterNames[family] {
		return nil, fmt.Errorf("%w: unknown filter %s", ErrRender, name)
	}
	arg, err := s.expand(arg, "filter arguments")
	if err != nil {
		return nil, err
	} else if needsArgs[name] && arg == "" {
		return nil, fmt.Errorf("%w: %s requires arguments", ErrRender, name)
	}
	if f, ok := valueFilters[name]; ok {
		return vals, s.each(vals, f)
	}
	switch name {
	case "split", "autosplit":
		return s.split(name, arg, vals)
	case "sort":
		slices.Sort(vals)
	case "rsort":
		slices.Sort(vals)
		slices.Reverse(vals)
	case "reverse":
		slices.Reverse(vals)
	case "uniq":
		seen := map[string]bool{}
		vals = slices.DeleteFunc(vals, func(v string) bool {
			dup := seen[v]
			seen[v] = true
			return dup
		})
	case "join":
		return s.join(vals, arg)
	case "append":
		vals = append(vals, arg)
	case "prepend":
		vals = append([]string{arg}, vals...)
	case "appends":
		return vals, s.each(vals, func(v string) string { return v + arg })
	case "prepends":
		return vals, s.each(vals, func(v string) string { return arg + v })
	case "remove":
		vals = slices.DeleteFunc(vals, func(v string) bool { return v == arg })
	case "slice", "sslice", "chop", "chomp":
		return s.slice(name, arg, vals)
	case "filter":
		return keep(vals, arg)
	case "int", "float":
		return s.numbers(name, vals)
	default:
		if strings.HasPrefix(name, "path") {
			return s.paths(name, vals)
		}
		return nil, nil // the name passes the check of upstream but matches no filter (1182-1183)
	}
	return vals, nil
}

func (s *rendering) each(vals []string, f func(string) string) error {
	for i, v := range vals {
		vals[i] = f(v)
		if s.bytesLeft -= len(vals[i]); s.bytesLeft < 0 {
			return errTooLong
		}
	}
	return nil
}

func (s *rendering) join(vals []string, sep string) ([]string, error) {
	n := len(sep) * len(vals)
	for _, v := range vals {
		n += len(v)
	}
	if s.bytesLeft -= n; s.bytesLeft < 0 {
		return nil, errTooLong
	}
	return []string{strings.Join(vals, sep)}, nil
}

func (s *rendering) split(name, sep string, vals []string) ([]string, error) {
	var out []string
	for _, v := range vals {
		parts := strings.SplitSeq(v, sep)
		if name == "autosplit" { // "," and ";" become spaces for str.split() (1120-1126)
			parts = strings.FieldsFuncSeq(v, func(r rune) bool { return isSpace(r) || r == ',' || r == ';' })
		}
		for p := range parts {
			// Checked in the loop, so that one long value cannot make millions of parts first.
			if len(out) >= s.o.MaxResults {
				return nil, fmt.Errorf("%w: more than %d", ErrTooManyValues, s.o.MaxResults)
			}
			out = append(out, p)
		}
	}
	return out, nil
}
