package template

import (
	"reflect"
	"testing"
)

// The tree keeps no "|" before no filter and no extra spaces around an
// operator (phototemplate.tx:74, 85, 88), so these examples print otherwise.
var printedAs = map[string]string{
	"{title|}":              "{title}",
	"{title|[a,b]}":         "{title[a,b]}",
	"{title  contains   x}": "{title contains x}",
}

func TestPrintRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tt := range parseCases() {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			want, ok := printedAs[tt.in]
			if !ok {
				want = tt.in
			}
			s := tt.want.String()
			got, err := Parse(s)
			if s != want || err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("String() of %s = %q, parsed to %s, %v; want %q", dump(tt.want), s, dump(got), err, want)
			}
		})
	}
}

// Parse returns no field with both a conditional and "&": the value of the
// conditional takes the "&" (phototemplate.tx:37, 89). Only a field built by
// hand shows every part in the order of phototemplate.tx:19-34, and its text
// does not parse back to it.
func TestPrintOrder(t *testing.T) {
	t.Parallel()

	f := &Field{
		Delim:       ptr("; "),
		Name:        "name",
		Subfield:    "sub",
		Arg:         ptr("arg"),
		Filters:     []string{"f1", "f2"},
		FindReplace: []Pair{{"a", "b"}, {"c", "d"}},
		Conditional: &Conditional{Negate: true, Operator: "contains", Value: *stmt(lit("v"))},
		Combine:     stmt(lit("x")),
		Bool:        stmt(lit("y")),
		Default:     stmt(lit("z")),
	}
	const want = "a{; +name:sub(arg)|f1|f2[a,b|c,d] not contains v&x?y,z}b"
	if got := stmt(Segment{Pre: "a", Field: f, Post: "b"}).String(); got != want {
		t.Errorf("String() = %q; want %q", got, want)
	}
}

func TestFields(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Holidays 2024/05", nil},
		{"{var:year,{created.year}}{original_name}-{%year}", []string{"var", "original_name", "%year"}},
		{"{a}{a}", []string{"a", "a"}},
		{"{a == {b}?{c},{d}}-{e&{f}}", []string{"a", "e"}},
	} {
		got, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q) = _, %v", tt.in, err)
		}
		if names := got.Fields(); !reflect.DeepEqual(names, tt.want) {
			t.Errorf("Parse(%q).Fields() = %q; want %q", tt.in, names, tt.want)
		}
	}
}
