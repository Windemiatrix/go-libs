package template

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var stringTests = map[string]func(v, c string) bool{ // phototemplate.py:734-741
	"contains": strings.Contains, "matches": func(v, c string) bool { return v == c },
	"startswith": strings.HasPrefix, "endswith": strings.HasSuffix,
}

// condition is the conditional of _render_template_string (phototemplate.py:685-763).
func condition(op string, vals, cond []string) (bool, error) {
	switch test, ok := stringTests[op]; {
	case ok: // "|" separates alternatives (727-732)
		alternatives := strings.Split(strings.Join(cond, "|"), "|")
		return len(cond) > 0 && slices.ContainsFunc(alternatives, func(a string) bool {
			return slices.ContainsFunc(vals, func(v string) bool { return test(v, a) })
		}), nil
	case op == "==" || op == "!=":
		return slices.Equal(slices.Sorted(slices.Values(vals)), slices.Sorted(slices.Values(cond))) == (op == "=="), nil
	case len(cond) != 1:
		return false, fmt.Errorf("%w: %s takes one value, not %d", ErrRender, op, len(cond))
	}
	c, okC := pyFloat(cond[0]) // any() parses nothing without values and stops at a match (712-716)
	for _, v := range vals {
		if x, ok := pyFloat(v); !ok || !okC { // the value may be a keyword or a title: not in the error
			return false, fmt.Errorf("%w: %s compares numbers only", ErrRender, op)
		} else if op == "<" && x < c || op == "<=" && x <= c || op == ">" && x > c || op == ">=" && x >= c {
			return true, nil
		}
	}
	return false, nil
}

// float() of Python takes an overflow as infinity and no "x", which only hex floats of strconv have.
func pyFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimFunc(s, isSpace), 64)
	return f, (err == nil || errors.Is(err, strconv.ErrRange)) && !strings.ContainsAny(s, "xX")
}
