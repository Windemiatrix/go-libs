package photoinfo

import "testing"

func TestBurstUnselected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		burst bool
		pick  int
		want  bool
	}{
		{false, 2, false}, // not a burst frame
		{true, 0, false},  // no pick type
		{true, 1, true},
		{true, 2, true},
		{true, 3, true},
		{true, 4, true},
		{true, 6, true},
		{true, 8, false},  // selected
		{true, 10, false}, // selected
		{true, 16, false}, // key frame
		{true, 20, false}, // key frame
		{true, 24, false}, // selected key frame
		{true, 32, true},
		{true, 40, false}, // 32 | selected
		{true, 48, false}, // 32 | key frame
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
