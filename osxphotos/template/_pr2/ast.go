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
// (phototemplate.tx:19-34):
// {delim+name:subfield(arg)|filters[find,replace] conditional&combine?bool,default}.
// A nil pointer is an absent part; a non-nil pointer to an empty value is a
// part that is present but blank, which renders differently from an absent
// one (phototemplate.py:570-622).
type Field struct {
	Delim       *string      // before "+"; "" for "{+name}" (phototemplate.tx:40-49)
	Name        string       // FIELD_WORD+, such as "title", "created.year", "%var" (51-57)
	Subfield    string       // after ":"; "" only when absent (59-69)
	Arg         *string      // between "(" and ")"; "" for "()" (101-107)
	Filters     []string     // after "|", such as "lower" or "chop(1)"; nil for "{title}" and "{title|}" alike (71-80)
	FindReplace []Pair       // between "[" and "]"; nil without brackets, empty for "[]" (109-129)
	Conditional *Conditional // after " "; nil when absent (83-99)
	Combine     *Template    // after "&"; empty for a bare "&" (131-136)
	Bool        *Template    // after "?"; empty for a bare "?" (138-143)
	Default     *Template    // after ","; empty for a bare "," (145-150)
}

// Pair is a find,replace pair; Replace is "" when absent
// (phototemplate.tx:117-129).
type Pair struct{ Find, Replace string }

// Conditional is " not? operator value" (phototemplate.tx:83-99). Operator is
// one of contains, matches, startswith, endswith, <=, >=, <, >, == and !=.
// Value keeps "|" in its text, since the text of a statement admits it
// (phototemplate.tx:37, 89); the renderer splits it for the four string
// operators only (phototemplate.py:727-732).
type Conditional struct {
	Negate   bool
	Operator string
	Value    Template
}
