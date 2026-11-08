package golden

import (
	"slices"
	"testing"
)

func TestKeysCoverFields(t *testing.T) {
	t.Parallel()
	keys := Keys()
	if !slices.IsSorted(keys) || !slices.Contains(keys, "uuid") {
		t.Errorf("Keys %v must be sorted and hold uuid", keys)
	}
	want := slices.Sorted(slices.Values(fieldsTxt(t)))
	if !slices.Equal(keys, want) {
		t.Errorf("Keys gives %d keys, fields.txt has %d\n got %v\nwant %v", len(keys), len(want), keys, want)
	}
}
