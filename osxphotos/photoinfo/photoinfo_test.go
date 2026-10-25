package photoinfo

import "testing"

func TestBurstUnselected(t *testing.T) {
	t.Parallel()

	const unrelatedBit = 32
	tests := []struct {
		name  string
		burst bool
		pick  int
		want  bool
	}{
		{"not a burst frame", false, 2, false},
		{"no pick type", true, 0, false},
		{"bit 1 only", true, 1, true},
		{"bit 2 only", true, 2, true},
		{"bits 1 and 2", true, 3, true},
		{"default pick only", true, BurstPickDefault, true},
		{"default pick and bit 2", true, BurstPickDefault | 2, true},
		{"selected", true, BurstPickSelected, false},
		{"selected and bit 2", true, BurstPickSelected | 2, false},
		{"key", true, BurstPickKey, false},
		{"key and default pick", true, BurstPickKey | BurstPickDefault, false},
		{"key and selected", true, BurstPickKey | BurstPickSelected, false},
		{"unrelated bit only", true, unrelatedBit, true},
		{"unrelated bit and selected", true, unrelatedBit | BurstPickSelected, false},
		{"unrelated bit and key", true, unrelatedBit | BurstPickKey, false},
	}
	for _, tt := range tests {
		p := &PhotoInfo{Burst: tt.burst, BurstPickType: tt.pick}
		if got := p.BurstUnselected(); got != tt.want {
			t.Errorf("%s: PhotoInfo{Burst: %v, BurstPickType: %d}.BurstUnselected() = %v, want %v", tt.name, tt.burst, tt.pick, got, tt.want)
		}
	}

	if (&PhotoInfo{}).BurstUnselected() {
		t.Error("PhotoInfo{}.BurstUnselected() = true, want false")
	}
}
