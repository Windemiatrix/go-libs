package template

import (
	"fmt"
	"slices"
	"strings"
)

// filter(x) and filter_predicate (phototemplate.py:1169-1171, 1186-1235), whose
// argument is read once. The operator may come from a variable: no error names it.
func keep(vals []string, args string) ([]string, error) {
	negate := false
	op, rest := splitWord(args)
	for ; op == "not"; op, rest = splitWord(rest) { // a loop, since "not not …" may be long (1193-1195)
		negate = !negate
	}
	alternatives := strings.Split(rest, "|")
	out := vals[:0]
	for _, v := range vals {
		switch {
		case op == "" || rest == "": // per value, as upstream: SyntaxError, or IndexError at args[0] or args[1] (1190-1198)
			return nil, fmt.Errorf("%w: filter takes an operator and a value", ErrRender)
		case !slices.Contains(operators, op): // the operators of a condition are those of upstream's predicate (1214-1233)
			return nil, fmt.Errorf("%w: filter takes an unknown operator", ErrRender)
		}
		ok, err := matchAny(op, v, alternatives)
		if err != nil {
			return nil, err
		} else if ok != negate {
			out = append(out, v)
		}
	}
	return out, nil
}

// For one value and one alternative, the condition of a field is the test of
// filter_predicate; any() stops at the first match (phototemplate.py:1204-1207).
func matchAny(op, v string, alternatives []string) (bool, error) {
	for _, c := range alternatives {
		if ok, err := condition(op, []string{v}, []string{c}); ok || err != nil {
			return ok, err
		}
	}
	return false, nil
}

// str.split(None, 1) of Python.
func splitWord(s string) (word, rest string) {
	s = strings.TrimLeftFunc(s, isSpace)
	if i := strings.IndexFunc(s, isSpace); i >= 0 {
		return s[:i], strings.TrimLeftFunc(s[i:], isSpace)
	}
	return s, ""
}
