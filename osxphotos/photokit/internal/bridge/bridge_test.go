package bridge

import (
	"math"
	"testing"
	"time"
)

func TestTimeoutMS(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want int64
	}{
		{-time.Second, 0},
		{0, 0},
		{time.Nanosecond, 1},
		{1500 * time.Microsecond, 1},
		{5 * time.Minute, 300000},
		{math.MaxInt64, 4611686018427},
	} {
		if got := timeoutMS(tt.d); got != tt.want {
			t.Errorf("timeoutMS(%v) = %d, want %d", tt.d, got, tt.want)
		}
	}
}
