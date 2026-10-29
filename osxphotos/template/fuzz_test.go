package template

import (
	"errors"
	"reflect"
	"testing"
)

func depthOf(t *Template) int {
	d := 0
	for _, seg := range t.Segments {
		f := seg.Field
		if f == nil {
			continue
		}
		for _, sub := range []*Template{f.Combine, f.Bool, f.Default} {
			if sub != nil {
				d = max(d, depthOf(sub))
			}
		}
		if f.Conditional != nil {
			d = max(d, depthOf(&f.Conditional.Value))
		}
	}
	return d + 1
}

func FuzzParse(f *testing.F) {
	for _, tt := range parseCases() {
		f.Add(tt.in)
	}
	for _, tt := range parseErrors() {
		f.Add(tt.in)
	}
	f.Add("\xff{a\xff:\xff}")
	f.Fuzz(func(t *testing.T, s string) {
		got, err := Parse(s)
		switch {
		case err != nil && !errors.Is(err, ErrSyntax):
			t.Fatalf("Parse(%q): %v does not wrap ErrSyntax", s, err)
		case err == nil && got == nil:
			t.Fatalf("Parse(%q) = nil, nil", s)
		case err == nil && depthOf(got) > 32:
			t.Fatalf("Parse(%q) is %d statements deep, more than 32", s, depthOf(got))
		}
		if err != nil {
			return
		}
		if back, err := Parse(got.String()); err != nil || !reflect.DeepEqual(back, got) {
			t.Fatalf("Parse(%q) = %s, %v; want %s, the tree of %q", got.String(), dump(back), err, dump(got), s)
		}
	})
}
