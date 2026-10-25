package golden

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

var (
	tableRow = regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|")
	jsonFile = regexp.MustCompile("(?s)```json\n(.*?)\n```")
	// numberedDecision is a reference to a working decision of the project, which is not public.
	numberedDecision = regexp.MustCompile(`(?i)(решени[еяю]|decision) ?№? ?[0-9]+`)
)

// TestNormalizationDoc keeps NORMALIZATION.md in step with the code: each key
// of fields.txt has one row in the English table and one in the Russian one,
// the example record is a valid record, and the file has nothing that is not
// public.
func TestNormalizationDoc(t *testing.T) {
	t.Parallel()
	raw, err := readFile(filepath.Join(testlib.RepoDir(t, "golden"), "NORMALIZATION.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	en, ru, found := strings.Cut(doc, `<a id="normalization-ru"></a>`)
	if !found {
		t.Fatal("no anchor normalization-ru: the file has no Russian part")
	}
	if !strings.Contains(en, "(#normalization-ru)") {
		t.Error("the English part does not link to the Russian one")
	}

	want := slices.Sorted(slices.Values(fieldsTxt(t)))
	for name, part := range map[string]string{"English": en, "Russian": ru} {
		var keys []string
		for _, m := range tableRow.FindAllStringSubmatch(part, -1) {
			keys = append(keys, m[1])
		}
		if len(keys) != len(want) {
			t.Errorf("%s table has %d rows, fields.txt has %d keys", name, len(keys), len(want))
		}
		slices.Sort(keys)
		if !slices.Equal(keys, want) {
			t.Errorf("%s table keys differ from fields.txt\n got %v\nwant %v", name, keys, want)
		}
	}

	t.Run("the example is a valid record", func(t *testing.T) {
		t.Parallel()
		m := jsonFile.FindStringSubmatch(en)
		if m == nil {
			t.Fatal("no json block in the English part")
		}
		divs, err := loadDivs(t, m[1])
		if err != nil {
			t.Fatalf("the example of NORMALIZATION.md is not a valid divergences.json: %v", err)
		}
		if len(divs) != 1 || divs[0].UUID != "*" {
			t.Errorf("the example has %d records, want one for all objects", len(divs))
		}
	})
	t.Run("nothing private", func(t *testing.T) {
		t.Parallel()
		for _, bad := range []string{"_work", "SEC-", "/Users/", ".env", "ТЗ"} {
			if strings.Contains(doc, bad) {
				t.Errorf("NORMALIZATION.md mentions %q", bad)
			}
		}
		if m := numberedDecision.FindString(doc); m != "" {
			t.Errorf("NORMALIZATION.md refers to a numbered decision: %q", m)
		}
		if uuidLike.MatchString(doc) {
			t.Error("NORMALIZATION.md has a UUID")
		}
	})
	t.Run("every key of the code has a row", func(t *testing.T) {
		t.Parallel()
		for _, k := range allKeys() {
			if !strings.Contains(en, "| `"+k+"` |") || !strings.Contains(ru, "| `"+k+"` |") {
				t.Errorf("no row for %s", k)
			}
		}
	})
}
