package template

// Template is a parsed template statement: its segments in order
// (phototemplate.tx:9-17). An empty statement has no segments
// (phototemplate.py:509-511).
type Template struct{ Segments []Segment }

// Segment is a TemplateString of the grammar (phototemplate.tx:13-17):
// pre-text, an optional field in braces and post-text. Pre and Post hold no
// "{", "}", "," or "?" (NON_TEMPLATE_STRING, phototemplate.tx:36-38). Post
// takes all the text up to the next "{", so Pre is non-empty only in the
// first segment of a statement: "{a}x{b}" is [{Field a, Post "x"}, {Field b}]
// and "a{b}" is [{Pre "a", Field b}].
type Segment struct {
	Pre   string
	Field *Field
	Post  string
}

// Field is the part between the braces, in the order of the grammar
// (phototemplate.tx:19-34): {delim+name:subfield(arg)&combine?bool,default}.
// A nil pointer is an absent part; a non-nil pointer to an empty value is a
// part that is present but blank, which renders differently from an absent
// one (phototemplate.py:570-622).
type Field struct {
	Delim    *string   // before "+"; "" for "{+name}" (phototemplate.tx:40-49)
	Name     string    // FIELD_WORD+, such as "title", "created.year", "%var" (51-57)
	Subfield string    // after ":"; "" only when absent (59-69)
	Arg      *string   // between "(" and ")"; "" for "()" (101-107)
	Combine  *Template // after "&"; empty for a bare "&" (131-136)
	Bool     *Template // after "?"; empty for a bare "?" (138-143)
	Default  *Template // after ","; empty for a bare "," (145-150)
}
