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
	var parts [3][]string // phototemplate.py:577-622
	for i, t := range []*Template{f.Combine, f.Bool, f.Default} {
		var err error
		if t != nil && len(t.Segments) == 0 {
			parts[i] = []string{""}
		} else if t != nil {
			if parts[i], err = s.statement(t); err != nil {
				return nil, false, err
			}
		}
	}
	combine, boolVal, def := parts[0], parts[1], parts[2]
	vals, ok, err := s.values(f, def)
	if err != nil || !ok {
		return nil, ok, err
	}
	if (s.o.ExpandInplace || f.Delim != nil) && len(vals) > 0 {
		vals = []string{strings.Join(vals, *cmp.Or(f.Delim, &s.o.InplaceSep))}
	}
	if len(f.Filters) > 0 {
		return nil, false, unsupported(ErrUnsupportedConstruct, "filter "+f.Filters[0])
	}
	for i := range vals {
		for _, pair := range f.FindReplace {
			vals[i] = strings.ReplaceAll(vals[i], pair.Find, pair.Replace)
		}
	}
	if f.Conditional != nil {
		return nil, false, unsupported(ErrUnsupportedConstruct, "operator "+f.Conditional.Operator)
	}
	for _, v := range combine {
		if v != "" {
			vals = append(vals, v)
		}
	}
	switch {
	case f.Bool != nil && len(vals) > 0:
		vals = boolVal
	case f.Bool != nil, len(vals) == 0 && len(def) > 0:
		vals = def
	case len(vals) == 0:
		vals = []string{s.o.NoneStr}
	}
	return vals, true, nil
}

// values is get_field_values (phototemplate.py:839-884); the caller may change
// the slice it returns.
func (s *rendering) values(f *Field, def []string) ([]string, bool, error) {
	name := f.Name
	stem, _, dotted := strings.Cut(name, ".")
	switch {
	case name == "var" || strings.HasPrefix(name, "%"): // before get_field_values (phototemplate.py:643-656)
		return nil, false, unsupported(ErrUnsupportedConstruct, name)
	case !upstreamField(name, stem): // ahead of later: upstream puts shell_quote.x in unmatched (849-883)
		s.unmatched = append(s.unmatched, name)
		return nil, false, nil
	case later[stem], pathFields[stem] && dotted,
		f.Default != nil && (name == "media_type" || name == "photo_or_video"), name == "id" && f.Subfield != "":
		return nil, false, unsupported(ErrUnsupportedConstruct, name)
	case punctuation[name] != "":
		return []string{punctuation[name]}, true, nil
	case name == "osxphotos_version":
		return []string{UpstreamVersion}, true, nil
	case name == "edited_version" && s.o.EditedVersion:
		return []string{name}, true, nil
	case name == "edited_version":
		return nil, true, nil
	case name == "export_dir":
		return []string{cmp.Or(s.o.ExportDir, "None")}, true, nil
	case name == "filepath":
		return []string{cmp.Or(s.o.Filepath, "None")}, true, nil
	case name == "strip":
		out := make([]string, len(def))
		for i, v := range def {
			out[i] = strings.TrimFunc(v, isSpace)
		}
		return out, true, nil
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
