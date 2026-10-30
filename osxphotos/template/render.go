package template

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Render renders t for p as PhotoTemplate.render does (phototemplate.py:471-513):
// one string per combination of the values of the multi-valued fields (774-783),
// none for an empty template (509-511). unmatched are the names that upstream
// does not have (881-883); the statement up to such a name gives no strings (661).
// A field that upstream has but p does not give, and the never ported
// {exiftool}, {function}, {detected_text}, {osxphotos_cmd_line} and the filter function:,
// give an error that wraps ErrUnsupportedField and names the field; a part of the language not
// ported yet, ErrUnsupportedConstruct; more than o.MaxResults strings, or more than
// 16 MiB of strings built, ErrTooManyValues; a SyntaxError or ValueError of upstream, or
// variables that do not settle, ErrRender. The variables of {var} live for one Render.
func (r *Renderer) Render(t *Template, p Photo, o RenderOptions) (values, unmatched []string, err error) {
	o.NoneStr = cmp.Or(o.NoneStr, "_")
	o.InplaceSep = cmp.Or(o.InplaceSep, ",")
	o.PathSep = cmp.Or(o.PathSep, "/")
	o.MaxResults = cmp.Or(o.MaxResults, DefaultMaxResults)
	o.Today = cmp.Or(o.Today, time.Now()) // once for every {today}, as self.today (phototemplate.py:910-912)
	s := &rendering{p: p, o: o, variables: map[string][]string{}, bytesLeft: maxRenderBytes}
	if values, err = s.statement(t); err != nil {
		return nil, nil, err
	}
	return values, s.unmatched, nil
}

type rendering struct {
	p         Photo
	o         RenderOptions
	unmatched []string
	variables map[string][]string
	bytesLeft int
}

// _render_statement (phototemplate.py:515-539).
func (s *rendering) statement(t *Template) ([]string, error) {
	var results []string
	for _, seg := range t.Segments {
		var err error
		if results, err = s.segment(seg, results); err != nil {
			return nil, err
		}
	}
	if s.o.Strip {
		for i, v := range results {
			results[i] = strings.TrimFunc(v, isSpace)
		}
	}
	return results, nil
}

// _render_template_string (phototemplate.py:541-791).
func (s *rendering) segment(seg Segment, results []string) ([]string, error) {
	if len(results) == 0 {
		results = []string{""} // after an unmatched field too (550)
	}
	rendered := []string{seg.Pre + seg.Post}
	if seg.Field != nil {
		vals, ok, err := s.field(seg.Field)
		if err != nil || !ok {
			return nil, err
		}
		if len(vals) > 0 {
			rendered = make([]string, len(vals))
			for i, v := range vals {
				rendered[i] = seg.Pre + v + seg.Post
			}
		}
	}
	if len(rendered)*len(results) > s.o.MaxResults {
		return nil, fmt.Errorf("%w: more than %d", ErrTooManyValues, s.o.MaxResults)
	}
	out := make([]string, 0, len(rendered)*len(results))
	for _, ren := range rendered {
		for _, res := range results {
			if s.bytesLeft -= len(res) + len(ren); s.bytesLeft < 0 {
				return nil, errTooLong
			}
			out = append(out, res+ren)
		}
	}
	return out, nil
}

// field returns false for a name that upstream does not have.
func (s *rendering) field(f *Field) ([]string, bool, error) {
	if never[f.Name] {
		return nil, false, unsupported(ErrUnsupportedField, f.Name)
	}
	var err error
	sep := s.o.InplaceSep
	if f.Delim != nil { // phototemplate.py:570-575
		if sep, err = s.expand(*f.Delim, "delim"); err != nil {
			return nil, false, err
		}
	}
	var parts [3][]string // phototemplate.py:577-622
	for i, t := range []*Template{f.Combine, f.Bool, f.Default} {
		if t != nil && len(t.Segments) == 0 {
			parts[i] = []string{""}
		} else if t != nil {
			if parts[i], err = s.statement(t); err != nil {
				return nil, false, err
			}
		}
	}
	combine, boolVal, def := parts[0], parts[1], parts[2]
	var cond []string
	if f.Conditional != nil { // phototemplate.py:625-634
		if cond, err = s.statement(&f.Conditional.Value); err != nil {
			return nil, false, err
		}
	}
	vals, ok := []string(nil), true
	switch { // phototemplate.py:643-661
	case strings.HasPrefix(f.Name, "%"):
		vals, err = s.variable(f.Name[1:])
	case f.Name == "var" && (f.Subfield == "" || len(def) == 0):
		err = fmt.Errorf("%w: var takes a name and a value, {var:name,value}", ErrRender)
	case f.Name == "var":
		def = slices.DeleteFunc(def, func(v string) bool { return v == "" }) // phototemplate.py:654
		s.variables[f.Subfield] = def
	default:
		vals, ok, err = s.values(f, def)
	}
	if err != nil || !ok {
		return nil, ok, err
	}
	if (s.o.ExpandInplace || f.Delim != nil) && len(vals) > 0 {
		if vals, err = s.join(vals, sep); err != nil {
			return nil, false, err
		}
	}
	if vals, err = s.filters(f.Filters, vals); err != nil {
		return nil, false, err
	}
	for i := range vals {
		for _, pair := range f.FindReplace { // phototemplate.py:672-683
			find, errFind := s.expand(pair.Find, "find/replace")
			replace, err := s.expand(pair.Replace, "find/replace")
			if err = cmp.Or(errFind, err); err != nil {
				return nil, false, err
			}
			if s.bytesLeft -= len(vals[i]) + strings.Count(vals[i], find)*len(replace); s.bytesLeft < 0 {
				return nil, false, errTooLong
			}
			vals[i] = strings.ReplaceAll(vals[i], find, replace)
		}
	}
	if f.Conditional != nil {
		match, err := condition(f.Conditional.Operator, vals, cond)
		if err != nil {
			return nil, false, err
		}
		if vals = nil; match != f.Conditional.Negate {
			vals = []string{"True"}
		}
	}
	for _, v := range combine {
		if v != "" {
			vals = append(vals, v)
		}
	}
	switch {
	case f.Bool != nil && len(vals) > 0:
		vals = boolVal
	case f.Bool != nil, len(vals) == 0 && len(def) > 0 && f.Name != "var":
		vals = def
	case len(vals) == 0 && f.Name != "var": // phototemplate.py:770-772
		vals = []string{s.o.NoneStr}
	}
	return vals, true, nil
}

// values is get_field_values (phototemplate.py:839-884); the caller may change
// the slice it returns.
func (s *rendering) values(f *Field, def []string) ([]string, bool, error) {
	name := f.Name
	stem, part, _ := strings.Cut(name, ".")
	switch {
	case !upstreamField(name, stem): // ahead of later: upstream puts format.x in unmatched (849-883)
		s.unmatched = append(s.unmatched, name)
		return nil, false, nil
	case later[stem],
		f.Default != nil && (name == "media_type" || name == "photo_or_video"), name == "id" && f.Subfield != "":
		return nil, false, unsupported(ErrUnsupportedConstruct, name)
	case stem == "created" || stem == "modified" || stem == "today":
		vals, err := s.date(name, stem, part)
		return vals, true, err
	case punctuation[name] != "":
		return []string{punctuation[name]}, true, nil
	case name == "osxphotos_version":
		return []string{UpstreamVersion}, true, nil
	case name == "edited_version" && s.o.EditedVersion:
		return []string{name}, true, nil
	case name == "edited_version":
		return nil, true, nil
	case pathFields[stem]:
		vals, err := s.pathField(name, stem)
		return vals, true, err
	case name == "strip":
		out := make([]string, len(def))
		for i, v := range def {
			out[i] = strings.TrimFunc(v, isSpace)
		}
		return out, true, nil
	case name == "shell_quote": // phototemplate.py:1340-1341
		out := slices.DeleteFunc(slices.Clone(def), func(v string) bool { return v == "" })
		return out, true, s.each(out, shellQuote)
	}
	v, ok := s.p.Field(name)
	if !ok {
		return nil, false, unsupported(ErrUnsupportedField, name)
	}
	vals := slices.Clone(v.Strings)
	if v.Int != nil {
		vals = append(vals, strconv.FormatInt(*v.Int, 10))
	}
	sep := s.o.PathSep
	if f.Arg != nil && *f.Arg != "" { // path_sep or self.path_sep (1255)
		sep = *f.Arg
	}
	albums := make([]string, len(v.Albums))
	for i, a := range v.Albums {
		albums[i] = strings.Join(append(slices.Clone(a.Folders), a.Title), sep)
	}
	slices.Sort(albums)
	return append(vals, albums...), true, nil
}

// str.strip of Python: str.isspace takes U+001C-U+001F too, unicode.IsSpace does not.
func isSpace(r rune) bool { return unicode.IsSpace(r) || 0x1c <= r && r <= 0x1f }

func unsupported(err error, what string) error { return fmt.Errorf("%w: %s", err, what) }
