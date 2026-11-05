package template

import (
	"strings"
	"testing"
	"unicode/utf8"
)

type sanitizeCase struct {
	in, replacement, want string
}

func checkSanitize(t *testing.T, name string, f func(string, string) string, cases []sanitizeCase) {
	t.Helper()
	for _, tt := range cases {
		got := f(tt.in, tt.replacement)
		if got != tt.want || !utf8.ValidString(got) {
			t.Errorf("%s(%.40q, %q) = %.40q (%d runes), want %.40q (%d runes)", name, tt.in, tt.replacement,
				got, utf8.RuneCountInString(got), tt.want, utf8.RuneCountInString(tt.want))
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	x := func(n int) string { return strings.Repeat("x", n) }
	e := func(n int) string { return strings.Repeat("\U000000e9", n) }
	checkSanitize(t, "SanitizeFilename", SanitizeFilename, []sanitizeCase{
		{"", ":", ""},
		{"a/b", ":", "a:b"},
		{"a/b", "-", "a-b"},
		{"a/b", "", "ab"},
		{"Cafe\U00000301.jpg", ":", "Caf\U000000e9.jpg"},
		// the caller keeps the name inside its directory, as upstream leaves it to the caller
		{"..", ":", ".."},
		{".", ":", "."},
		{"a\x00b", ":", "a\x00b"},
		{"-a\tb", ":", "-a\tb"},
		{x(240), ":", x(240)},
		{x(241), ":", x(240)},
		{x(300), ":", x(240)},
		// path_utils.py:60-63: the stem when it is at least as long as the excess, else the extension
		{e(239) + ".jpg", ":", e(236) + ".jpg"},
		// drop == len(stem): nothing of the stem is left before the "."
		{"ab." + x(239), ":", "." + x(239)},
		// the stem is everything before the last "." (path_utils.py:58-59)
		{x(250) + ".tar.gz", ":", x(237) + ".gz"},
		{"a." + x(300), ":", "a." + x(238)},
		{"." + x(300), ":", "." + x(239)},
		{"a." + x(240), ":", "a." + x(238)},
		{"a.b." + x(240), ":", "a.b." + x(236)},
		// the extension runs out and the stem stays whole: longer than 240, as upstream
		{x(240) + "." + x(240), ":", x(240) + "."},
		{"a.", ":", "a."},
		// NFC first, so the combining mark is one rune of the 240
		{x(239) + "e\U00000301", ":", x(239) + "\U000000e9"},
		// NFC before the replacement (path_utils.py:51-52)
		{"/\U00000301", "e", "e\U00000301"},
		// a str of Python cannot hold invalid UTF-8: the part that is cut is in runes
		{"\xff" + x(240), ":", "\U0000fffd" + x(239)},
	})
}

func TestSanitizeDirname(t *testing.T) {
	t.Parallel()
	x := func(n int) string { return strings.Repeat("x", n) }
	e := func(n int) string { return strings.Repeat("\U000000e9", n) }
	checkSanitize(t, "SanitizeDirname", SanitizeDirname, []sanitizeCase{
		{"", ":", ""},
		{"a/b/c", ":", "a:b:c"},
		{"a/b", "-", "a-b"},
		{"a/b", "", "ab"},
		{"Cafe\U00000301", ":", "Caf\U000000e9"},
		{"..", ":", ".."},
		{".", ":", "."},
		{"a\x00b", ":", "a\x00b"},
		{x(254), ":", x(254)},
		{x(255), ":", x(255)},
		{x(256), ":", x(255)},
		// no extension is kept (path_utils.py:100-102)
		{x(253) + ".jpg", ":", x(253) + ".j"},
		{e(256), ":", e(255)},
		{x(254) + "e\U00000301", ":", x(254) + "\U000000e9"},
		// the replacement before NFC (path_utils.py:96-99)
		{"/\U00000301", "e", "\U000000e9"},
		{"\xff" + x(255), ":", "\U0000fffd" + x(254)},
	})
}
