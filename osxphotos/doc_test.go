package osxphotos_test

import (
	"os"
	"regexp"
	"testing"
)

// CI and tooling read UPSTREAM.md as plain text, so its format is part of the
// contract.
func TestUpstreamRecord(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("UPSTREAM.md")
	if err != nil {
		t.Fatalf("read UPSTREAM.md: %v", err)
	}

	version := lineValue(t, data, `Upstream version`, `(\d+\.\d+\.\d+)`)
	tag := lineValue(t, data, `Upstream tag`, `(v\d+\.\d+\.\d+)`)
	if want := "v" + version; tag != want {
		t.Errorf("Upstream tag = %q, want %q (derived from Upstream version)", tag, want)
	}
}

func lineValue(t *testing.T, data []byte, key, pattern string) string {
	t.Helper()

	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `: ` + pattern + `$`)
	matches := re.FindAllSubmatch(data, -1)
	if len(matches) != 1 {
		t.Fatalf("UPSTREAM.md: want exactly one line %q, found %d", key+": "+pattern, len(matches))
	}
	return string(matches[0][1])
}
