package template

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The filters slice, sslice, chop and chomp (phototemplate.py:1106-1119,
// 1162-1168); str is indexed by code points.
func (s *rendering) slice(name, arg string, vals []string) ([]string, error) {
	b, err := sliceArgs(name, arg)
	switch {
	case err != nil:
		return nil, err
	case b[2] != nil && *b[2] == 0 && (name == "slice" || len(vals) > 0): // sslice raises the ValueError on a value only
		return nil, fmt.Errorf("%w: %s step cannot be zero", ErrRender, name)
	case name == "slice":
		return pySlice(vals, b), nil
	}
	return vals, s.each(vals, func(v string) string { return string(pySlice([]rune(v), b)) })
}

// create_slice (phototemplate.py:1703-1722); chop(n) is v[:-n] and chomp(n) is v[n:] (1112, 1119).
func sliceArgs(name, arg string) (b [3]*int, err error) {
	parts := strings.SplitN(arg, ":", len(b)) // a fourth part fails in strconv.Atoi
	if name == "chop" || name == "chomp" {
		parts = []string{arg}
	}
	// int() of Python; beyond 64 bits Atoi clamps, as slice.indices would.
	// UPSTREAM.md has what strconv.Atoi does not take.
	for i, p := range parts {
		if n, err := strconv.Atoi(strings.TrimFunc(p, isSpace)); err == nil || errors.Is(err, strconv.ErrRange) {
			b[i] = &n
		} else if p != "" {
			return b, fmt.Errorf("%w: invalid argument of %s", ErrRender, name)
		}
	}
	if name == "chop" && *b[0] != 0 { // v[:-0] would be "", so upstream keeps the value (1112)
		end := -max(*b[0], -math.MaxInt) // -math.MinInt overflows, and v[:2**63] keeps all of v
		b[0], b[1] = nil, &end
	}
	return b, nil
}

// seq[start:end:step] of Python, by slice.indices. A step beyond n+1 either
// way takes one element too, so i+step cannot overflow.
func pySlice[T any](xs []T, b [3]*int) []T {
	n := len(xs)
	step := 1
	if b[2] != nil {
		step = max(min(*b[2], n+1), -n-1)
	}
	first, last := 0, n
	if step < 0 {
		first, last = n-1, -1
	}
	lo, hi := min(first, last), max(first, last)
	bound := func(p *int, def int) int {
		if p == nil {
			return def
		} else if *p < 0 {
			return max(*p+n, lo)
		}
		return min(*p, hi)
	}
	var out []T
	for i, stop := bound(b[0], first), bound(b[1], last); step > 0 && i < stop || step < 0 && i > stop; i += step {
		out = append(out, xs[i])
	}
	return out
}
