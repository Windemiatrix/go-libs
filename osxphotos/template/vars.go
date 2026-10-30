package template

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// (?:%%)*(%[\w]+) (phototemplate.py:813); \w of RE2 is ASCII, of Python every script.
var variableRe = regexp.MustCompile(`(%%)*%([\p{L}\p{N}_]+)`)

// maxRenderBytes bounds the strings one Render builds: a variable keeps what a statement built
// and "%v%v" doubles each round; upstream has no bound (phototemplate.py:667, 681, 777-783, 814-831).
const maxRenderBytes = 1 << 24

var errTooLong = fmt.Errorf("%w: more than %d bytes of strings", ErrTooManyValues, maxRenderBytes)

func (s *rendering) variable(name string) ([]string, error) {
	vals, ok := s.variables[name]
	if !ok {
		return nil, fmt.Errorf("%w: variable %q is not defined", ErrRender, name)
	}
	return slices.Clone(vals), nil // [find,replace] changes the values in place
}

// expand_variables_to_str (phototemplate.py:793-837); its re.search matches "" at
// 0, so no variable after the start of a value expands (t402).
func (s *rendering) expand(text, what string) (string, error) {
	values := []string{text}
	for round := 0; ; round++ {
		var next []string
		for _, cur := range values {
			m := variableRe.FindStringSubmatchIndex(cur)
			if m == nil || m[0] != 0 {
				break
			}
			name := cur[m[4]:m[5]]
			vals, err := s.variable(name)
			if err != nil && round > 0 { // the name is part of a value, which may be a keyword
				return "", fmt.Errorf("%w: a value of a variable in %s starts with an undefined %%name", ErrRender, what)
			} else if err != nil {
				return "", err
			} else if len(next)+len(values)*len(vals) > s.o.MaxResults {
				return "", fmt.Errorf("%w: more than %d", ErrTooManyValues, s.o.MaxResults)
			}
			// re.sub(f"(%%)*{var}", r"\g<1>" + var_val) (826), with var_val literal: re.sub reads its backslashes.
			sub := regexp.MustCompile(`(%%)*%` + name)
			for _, v := range values {
				for _, val := range vals {
					if s.bytesLeft -= len(v) + strings.Count(v, "%"+name)*len(val); s.bytesLeft < 0 {
						return "", errTooLong
					}
					next = append(next, sub.ReplaceAllString(v, "${1}"+strings.ReplaceAll(val, "$", "$$")))
				}
			}
		}
		if len(next) == 0 || slices.Equal(next, values) {
			break
		} else if round == maxDepth { // upstream never stops on a value "%v-" of v (814-831)
			return "", fmt.Errorf("%w: variables do not settle after %d expansions", ErrRender, maxDepth)
		}
		values = next
	}
	if len(values) != 1 {
		return "", fmt.Errorf("%w: %s takes one value, not %d", ErrRender, what, len(values))
	}
	return strings.ReplaceAll(values[0], "%%", "%"), nil
}
