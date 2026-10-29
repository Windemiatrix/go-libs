package template

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Render renders t for p as PhotoTemplate.render does (phototemplate.py:471-513):
// one string per combination of the values of the multi-valued fields (774-783),
// none for an empty template (509-511). unmatched are the names that upstream
// does not have (881-883); the statement up to such a name gives no strings (661).
// A field that upstream has but p does not give, and the never ported
// {exiftool}, {function}, {detected_text} and {osxphotos_cmd_line}, give an error
// that wraps ErrUnsupportedField and names the field; a part of the language not
// ported yet wraps ErrUnsupportedConstruct; more than o.MaxResults strings,
// ErrTooManyValues.
func (r *Renderer) Render(t *Template, p Photo, o RenderOptions) (values, unmatched []string, err error) {
	o.NoneStr = cmp.Or(o.NoneStr, "_")
	o.InplaceSep = cmp.Or(o.InplaceSep, ",")
	o.PathSep = cmp.Or(o.PathSep, "/")
	o.MaxResults = cmp.Or(o.MaxResults, DefaultMaxResults)
	s := &rendering{p: p, o: o}
	if values, err = s.statement(t); err != nil {
		return nil, nil, err
	}
	return values, s.unmatched, nil
}

type rendering struct {
	p         Photo
	o         RenderOptions
	unmatched []string
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
	vals, ok, err := s.values(f)
	if err != nil || !ok {
		return nil, ok, err
	}
	// rendering {title,x} as {title} would be a wrong value, not an error
	if f.Combine != nil || f.Bool != nil || f.Default != nil || len(f.Filters) > 0 || len(f.FindReplace) > 0 || f.Conditional != nil {
		return nil, false, unsupported(ErrUnsupportedConstruct, "a part of "+f.Name)
	}
	if (s.o.ExpandInplace || f.Delim != nil) && len(vals) > 0 {
		vals = []string{strings.Join(vals, *cmp.Or(f.Delim, &s.o.InplaceSep))}
	}
	if len(vals) == 0 {
		vals = []string{s.o.NoneStr}
	}
	return vals, true, nil
}

// values is get_field_values (phototemplate.py:839-884).
func (s *rendering) values(f *Field) ([]string, bool, error) {
	name := f.Name
	stem, _, _ := strings.Cut(name, ".")
	switch {
	case name == "var" || strings.HasPrefix(name, "%"): // before get_field_values (phototemplate.py:643-656)
		return nil, false, unsupported(ErrUnsupportedConstruct, name)
	case !upstreamField(name, stem): // ahead of later: upstream puts shell_quote.x in unmatched (849-883)
		s.unmatched = append(s.unmatched, name)
		return nil, false, nil
	case later[stem], name == "id" && f.Subfield != "":
		return nil, false, unsupported(ErrUnsupportedConstruct, name)
	}
	v, ok := s.p.Field(name)
	if !ok {
		return nil, false, unsupported(ErrUnsupportedField, name)
	}
	vals := v.Strings
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
