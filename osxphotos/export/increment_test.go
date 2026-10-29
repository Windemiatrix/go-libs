package export

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestStemKey(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, want string }{
		{"IMG_0001.HEIC", "img_0001"},
		{".hidden", ".hidden"},
		{"a.b.c", "a.b"},
		{"a.", "a."},
		{"a", "a"},
		{"Cafe\U00000301.jpg", "caf\U000000e9"},
	}
	for _, tt := range tests {
		if got := StemKey(tt.name); got != tt.want {
			t.Errorf("StemKey(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func directory(files ...string) map[string]bool {
	taken := make(map[string]bool, len(files))
	for _, f := range files {
		taken[StemKey(f)] = true
	}
	return taken
}

func TestIncrementName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc      string
		name      string
		count     int
		files     []string
		want      string
		wantCount int
	}{
		{"free", "foo.jpeg", 0, nil, "foo.jpeg", 0},
		{"numbered names alone do not block the plain one", "foo.jpeg", 0, []string{"foo (1).jpeg"}, "foo.jpeg", 0},
		{"taken", "foo.jpeg", 0, []string{"foo.jpeg"}, "foo (1).jpeg", 1},
		{"taken twice", "foo.jpeg", 0, []string{"foo.jpeg", "foo (1).jpeg"}, "foo (2).jpeg", 2},
		{"start count is free", "foo.jpeg", 3, []string{"foo.jpeg"}, "foo (3).jpeg", 3},
		{"start count is taken", "foo.jpeg", 3, []string{"foo (3).jpeg"}, "foo (4).jpeg", 4},
		{"case and extension do not matter", "IMG_0001.jpeg", 0, []string{"img_0001.HEIC"}, "IMG_0001 (1).jpeg", 1},
		{"another extension blocks", "foo.jpeg", 0, []string{"foo.png"}, "foo (1).jpeg", 1},
		{"NFD name, NFC directory", "Cafe\U00000301.jpg", 0, []string{"Caf\U000000e9.jpg"}, "Caf\U000000e9 (1).jpg", 1},
		{"NFC name, NFD directory", "Caf\U000000e9.jpg", 0, []string{"Cafe\U00000301.jpg"}, "Caf\U000000e9 (1).jpg", 1},
		{"NFD name is returned in NFC", "Cafe\U00000301.jpg", 0, nil, "Caf\U000000e9.jpg", 0},
		{"no extension", "README", 0, []string{"README"}, "README (1)", 1},
		{"leading dot is no extension", ".hidden", 0, []string{".hidden"}, ".hidden (1)", 1},
		{"only the last extension is cut", "a.b.c", 0, []string{"a.b.d"}, "a.b (1).c", 1},
		{"a trailing dot stays in the stem", "a.", 0, []string{"a.", "a.jpg"}, "a. (1)", 1},
	}
	for _, tt := range tests {
		got, n := IncrementName(tt.name, tt.count, directory(tt.files...))
		if got != tt.want || n != tt.wantCount {
			t.Errorf("%s: IncrementName(%q, %d) = %q, %d, want %q, %d", tt.desc, tt.name, tt.count, got, n, tt.want, tt.wantCount)
		}
	}
}

func TestIncrementNameFree(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 2))
	pick := func(s []string) string { return s[rng.IntN(len(s))] }
	stems := []string{"foo", "IMG_0001", "Caf\U000000e9", "x y"}
	exts := []string{".jpeg", ".HEIC", ".png", ".mov", ""}
	spell := func(stem string) string {
		if rng.IntN(2) == 0 {
			stem = strings.ToUpper(stem)
		}
		if rng.IntN(2) == 0 {
			stem = norm.NFD.String(stem)
		}
		return stem + pick(exts)
	}
	numbered := func(stem string, n int) string {
		if n == 0 {
			return stem
		}
		return stem + " (" + strconv.Itoa(n) + ")"
	}

	for range 200 {
		stem, ext := pick(stems), pick(exts)
		name := stem + ext
		k := rng.IntN(6)
		full := rng.IntN(4) == 0
		count := rng.IntN(4)
		if full {
			count = 0
		}
		var files []string
		for n := 0; n <= k; n++ {
			if full || rng.IntN(2) == 0 {
				files = append(files, spell(numbered(stem, n)))
			}
		}
		taken := directory(files...)

		got, n := IncrementName(name, count, taken)
		if taken[StemKey(got)] {
			t.Errorf("IncrementName(%q, %d) = %q, %d: taken by %q", name, count, got, n, files)
		}
		if n < count {
			t.Errorf("IncrementName(%q, %d) = %q, %d: below the start count", name, count, got, n)
		}
		for m := count; m < n; m++ {
			if !taken[StemKey(numbered(stem, m)+ext)] {
				t.Errorf("IncrementName(%q, %d) = %q, %d: %d is free and smaller, files %q", name, count, got, n, m, files)
			}
		}
		if want := norm.NFC.String(numbered(stem, n) + ext); got != want {
			t.Errorf("IncrementName(%q, %d) = %q, %d, want the name %q", name, count, got, n, want)
		}
		if full && n != k+1 {
			t.Errorf("IncrementName(%q, 0) = %q, %d with 0..%d taken, want %d", name, got, n, k, k+1)
		}
	}
}
