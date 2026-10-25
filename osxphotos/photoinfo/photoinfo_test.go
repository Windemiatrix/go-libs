package photoinfo

import "testing"

func TestBurstUnselected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		burst bool
		pick  int
		want  bool
	}{
		{false, 2, false},
		{true, 0, false},
		{true, 1, true},
		{true, 2, true},
		{true, 3, true},
		{true, 4, true},
		{true, 6, true},
		{true, 8, false},
		{true, 10, false},
		{true, 16, false},
		{true, 20, false},
		{true, 24, false},
		{true, 32, true},
		{true, 40, false},
		{true, 48, false},
	}
	for _, tt := range tests {
		p := &PhotoInfo{Burst: tt.burst, BurstPickType: tt.pick}
		if got := p.BurstUnselected(); got != tt.want {
			t.Errorf("PhotoInfo{Burst: %v, BurstPickType: %d}.BurstUnselected() = %v, want %v", tt.burst, tt.pick, got, tt.want)
		}
	}

	if (&PhotoInfo{}).BurstUnselected() {
		t.Error("PhotoInfo{}.BurstUnselected() = true, want false")
	}
}
