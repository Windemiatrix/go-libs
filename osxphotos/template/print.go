package template

import "strings"

// String prints t in the template language, the parts of a field in the order
// of phototemplate.tx:19-34. Parse(t.String()) equals t for every t that Parse
// returns, though the text may differ from the parsed one: the tree keeps no
// "|" before no filter and no extra spaces around an operator
// (phototemplate.tx:74, 85, 88).
func (t *Template) String() string {
	var b strings.Builder
	for _, seg := range t.Segments {
		b.WriteString(seg.Pre)
		if seg.Field != nil {
			b.WriteString(seg.Field.text())
		}
		b.WriteString(seg.Post)
	}
	return b.String()
}

// Fields returns the field names of the top statement in order, repeats
// included, as PhotoTemplateParser.fields does (phototemplate.py:427-430):
// without the fields of nested statements and without checking that a name
// is a field; nil when there are none.
func (t *Template) Fields() []string {
	var names []string
	for _, seg := range t.Segments {
		if seg.Field != nil {
			names = append(names, seg.Field.Name)
		}
	}
	return names
}

func (f *Field) text() string {
	s := "{"
	if f.Delim != nil {
		s += *f.Delim + "+"
	}
	s += f.Name
	if f.Subfield != "" {
		s += ":" + f.Subfield
	}
	if f.Arg != nil {
		s += "(" + *f.Arg + ")"
	}
	if f.Filters != nil {
		s += "|" + strings.Join(f.Filters, "|")
	}
	if f.FindReplace != nil {
		pairs := make([]string, len(f.FindReplace))
		for k, p := range f.FindReplace {
			pairs[k] = p.Find + "," + p.Replace
		}
		s += "[" + strings.Join(pairs, "|") + "]"
	}
	if c := f.Conditional; c != nil {
		s += " "
		if c.Negate {
			s += "not "
		}
		s += c.Operator + " " + c.Value.String()
	}
	return s + part("&", f.Combine) + part("?", f.Bool) + part(",", f.Default) + "}"
}

func part(sep string, t *Template) string {
	if t == nil {
		return ""
	}
	return sep + t.String()
}
