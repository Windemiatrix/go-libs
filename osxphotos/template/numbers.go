package template

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// values_to_int and values_to_float (phototemplate.py:1725-1740) drop a value
// that float() does not take, and int drops NaN, a ValueError too.
func (s *rendering) numbers(name string, vals []string) ([]string, error) {
	out := vals[:0]
	for _, v := range vals {
		f, ok := pyFloat(v)
		switch {
		case !ok || name == "int" && math.IsNaN(f):
			continue
		case name == "int" && math.IsInf(f, 0): // OverflowError, which upstream does not catch
			return nil, fmt.Errorf("%w: int of an infinite value", ErrRender)
		case name == "int":
			if f = math.Trunc(f); f == 0 {
				f = 0 // int() has no -0
			}
			v = strconv.FormatFloat(f, 'f', 0, 64)
		default:
			v = pyFloatRepr(f)
		}
		if s.bytesLeft -= len(v); s.bytesLeft < 0 {
			return nil, errTooLong
		}
		out = append(out, v)
	}
	return out, nil
}

// str(float) of Python (phototemplate.py:1739), whose exponent bounds and ".0" no format of strconv has.
func pyFloatRepr(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) { // Go has "+Inf", "-Inf" and "NaN"
		return strings.ToLower(strings.TrimPrefix(strconv.FormatFloat(f, 'g', -1, 64), "+"))
	}
	if a := math.Abs(f); a != 0 && (a < 1e-4 || a >= 1e16) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
