package template

import (
	"math"
	"testing"
)

func TestFloatString(t *testing.T) {
	t.Parallel()
	sum := 0.1
	sum += 0.2 // at run time: the constant 0.1+0.2 is 0.3 exactly
	for _, tt := range []struct {
		f    float64
		want string
	}{
		{0.5, "0.5"}, {-2.5, "-2.5"}, {3, "3.0"}, {1000, "1000.0"}, {123456789.125, "123456789.125"},
		{1e15, "1000000000000000.0"}, {9999999999999998, "9999999999999998.0"}, {1e16, "1e+16"}, {1.5e300, "1.5e+300"},
		{math.MaxFloat64, "1.7976931348623157e+308"}, {0.1, "0.1"}, {0.0001, "0.0001"}, {1e-5, "1e-05"},
		{-1.5e-7, "-1.5e-07"}, {5e-324, "5e-324"}, {sum, "0.30000000000000004"},
		{0, "0.0"}, {math.Copysign(0, -1), "-0.0"}, {math.NaN(), "nan"}, {math.Inf(1), "inf"}, {math.Inf(-1), "-inf"},
	} {
		if got := FloatString(tt.f); got != tt.want {
			t.Errorf("FloatString(%v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}
