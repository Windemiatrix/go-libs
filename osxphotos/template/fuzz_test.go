package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
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

// Whatever Parse takes, Render neither panics nor fails with an error of its
// own, keeps its bounds and gives the same result twice. A string of the
// template may be repeated in every result, hence len(s) in the bound.
func FuzzRender(f *testing.F) {
	data, err := os.ReadFile(filepath.Join(testlib.RepoDir(f, "golden"), "templates.txt"))
	if err != nil {
		f.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			f.Add(line)
		}
	}
	for _, tt := range parseCases() {
		f.Add(tt.in)
	}
	for _, tt := range parseErrors() {
		f.Add(tt.in)
	}
	for _, in := range []string{
		"{keyword|slice(::0)}", "{keyword|sslice(5:1:-1)}", "{keyword|slice(-9223372036854775808::9223372036854775807)}",
		"{keyword|chop(-1)|chomp(-9223372036854775808)}", "{keyword|chop(9223372036854775807)|sslice(::-9223372036854775808)}",
		"{keyword|filter(not not contains a)}", "{keyword|filter(< 1|x)}", "{keyword|filter(not)}", "{keyword|filter( )}",
		"{keyword|int|float}", "{var:i,inf;nan;-0;1e400}{%i|split(;)|float|int}", "{var:n,> 1}{keyword|filter(%n)|sort}",
		"{var:a,x}{%a|split(;)|appends(%a)|shell_quote|upper|titlecase}", "{title|path.parent.name}",
	} {
		f.Add(in)
	}
	unicodePhoto := examplePhoto()
	unicodePhoto["title"] = strs("h\U000000e9llo")
	unicodePhoto["keyword"] = strs("h\U000000e9llo", "stra\U000000dfe", "\U00000130", "a/b:c", "", "1.5", "-inf", "Holidays")
	renderErrors := []error{ErrUnsupportedField, ErrUnsupportedConstruct, ErrTooManyValues, ErrRender}
	o := RenderOptions{Today: time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)}
	f.Fuzz(func(t *testing.T, s string) {
		tmpl, err := Parse(s)
		if err != nil {
			return
		}
		for _, p := range []Photo{fakePhoto{}, unicodePhoto, &countingPhoto{}} {
			got, unmatched, err := NewRenderer().Render(tmpl, p, o)
			size := 0
			for _, v := range got {
				size += len(v)
			}
			switch {
			case err != nil && !slices.ContainsFunc(renderErrors, func(e error) bool { return errors.Is(err, e) }):
				t.Fatalf("Render(%q): %v wraps none of the errors of Render", s, err)
			case err != nil && strings.Contains(err.Error(), "Holidays") && !strings.Contains(s, "Holidays"):
				t.Fatalf("Render(%q): %v names a value", s, err)
			case len(got) > DefaultMaxResults || size > maxRenderBytes+len(s)*DefaultMaxResults:
				t.Fatalf("Render(%q) = %d strings of %d bytes", s, len(got), size)
			}
			again, unmatchedAgain, errAgain := NewRenderer().Render(tmpl, p, o)
			if !slices.Equal(got, again) || !slices.Equal(unmatched, unmatchedAgain) || fmt.Sprint(err) != fmt.Sprint(errAgain) {
				t.Fatalf("Render(%q) = %.200q, %q, %v, then %.200q, %q, %v", s, got, unmatched, err, again, unmatchedAgain, errAgain)
			}
		}
	})
}
