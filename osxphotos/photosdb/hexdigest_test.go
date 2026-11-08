package photosdb

import "testing"

func TestHexdigestBlake2b160(t *testing.T) {
	t.Parallel()

	// Published BLAKE2b-160 vectors; hashlib.blake2b(digest_size=20) gives the same.
	tests := map[string]string{
		"":    "3345524abf6bbe1809449224b5972c41790b6cf2",
		"abc": "384264f676f39536840523f284921cdc68b6846b",
	}
	for in, want := range tests {
		if got := hexdigest(in); got != want {
			t.Errorf("hexdigest(%q) = %s, want %s", in, got, want)
		}
	}
}
