package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// The only check of Parse against the grammar of osxphotos that does not
// depend on Parse itself: parse.json is what PhotoTemplateParser said about
// every line of parse.txt (golden-regen). parse.txt has no line nested 33
// deep: osxphotos accepts it and Parse rejects it on purpose (maxDepth), which
// the literal tests of the parser pin down.
func TestParseGolden(t *testing.T) {
	dir := testlib.RepoDir(t, "golden")

	t.Run("parse.txt", func(t *testing.T) {
		lines := parseGoldenLines(t, filepath.Join(dir, "parse.txt"))
		data, err := os.ReadFile(filepath.Join(dir, "parse.json"))
		if err != nil {
			t.Fatalf("%v: run make golden-fetch", err)
		}
		var oracle map[string]struct {
			Accept bool     `json:"accept"`
			Fields []string `json:"fields"`
		}
		if err := json.Unmarshal(data, &oracle); err != nil {
			t.Fatalf("parse.json: %v", err)
		}
		if len(oracle) != len(lines) {
			t.Fatalf("parse.json has %d keys for %d lines of parse.txt", len(oracle), len(lines))
		}
		accepted, rejected := 0, 0
		for i, line := range lines {
			id := fmt.Sprintf("p%03d", i+1)
			want, ok := oracle[id]
			if !ok {
				t.Fatalf("parse.json has no key %s", id)
			}
			tmpl, err := Parse(line)
			if got := err == nil; got != want.Accept {
				t.Errorf("%s: accept = %v, want %v", id, got, want.Accept)
				continue
			}
			if !want.Accept {
				rejected++
				continue
			}
			accepted++
			if got := tmpl.Fields(); !slices.Equal(got, want.Fields) {
				t.Errorf("%s: fields got %q, want %q", id, got, want.Fields)
			}
		}
		if accepted == 0 || rejected == 0 {
			t.Errorf("%d lines accepted and %d rejected: the oracle must have both", accepted, rejected)
		}
	})

	// golden-regen rendered every one of these with osxphotos, so its grammar
	// accepts them all.
	t.Run("templates.txt", func(t *testing.T) {
		count := 0
		for i, line := range parseGoldenLines(t, filepath.Join(dir, "templates.txt")) {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			count++
			if _, err := Parse(line); err != nil {
				t.Errorf("templates.txt:%d: %v", i+1, err)
			}
		}
		if count == 0 {
			t.Error("templates.txt holds no template")
		}
	})
}

func parseGoldenLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
