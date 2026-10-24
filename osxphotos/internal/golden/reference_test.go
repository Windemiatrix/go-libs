package golden

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// allKeys returns the keys of all rules, sorted.
func allKeys() []string { return slices.Sorted(maps.Keys(rules)) }

// isoOffset returns the UTC offset of an ISO 8601 string of isoformat, in
// seconds, for the log of TestReferenceParses.
func isoOffset(t testing.TB, s string) int {
	t.Helper()
	i := strings.LastIndexAny(s, "+-")
	if i < 19 {
		t.Fatalf("a date without an offset")
	}
	parts := strings.Split(s[i+1:], ":")
	n := 0
	for k, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("offset: %v", err)
		}
		n += v * []int{3600, 60, 1}[k]
	}
	if s[i] == '-' {
		n = -n
	}
	return n
}

// TestReferenceParses: the reference side of every rule parses every object
// of every library, and the data keep the properties the rules rely on.
func TestReferenceParses(t *testing.T) {
	dir := Dir(t)
	epoch := time.Unix(0, 0)
	for _, name := range refLibraries(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ref, err := Load(dir, name)
			if err != nil {
				t.Fatal(err)
			}
			nulls := map[string]int{}
			offsetDiffers, adjusted := 0, 0
			for _, part := range []struct {
				file    string
				objects []Object
				trash   bool
			}{{queryFile, ref.Query, false}, {deletedFile, ref.Deleted, true}} {
				for i, obj := range part.objects {
					for _, key := range allKeys() {
						if _, err := rules[key].ref(obj); err != nil {
							t.Errorf("%s element %d: %v", part.file, i+1, err)
						}
						if isNull(obj[key]) {
							nulls[key]++
						}
					}
					if got, _ := refBool(obj, "intrash"); got != part.trash {
						t.Errorf("%s element %d: intrash = %v, want %v", part.file, i+1, got, part.trash)
					}
					// golden-regen runs with TZ=UTC: the dates of the database are in +00:00.
					for _, key := range []string{"date_added", "date_modified", "date_trashed"} {
						if s, ok := decodeString(obj[key]); ok && !strings.HasSuffix(s, "+00:00") {
							t.Errorf("%s element %d: %s is not in +00:00", part.file, i+1, key)
						}
					}
					// The premise of the orientation rule.
					if adj, _ := refBool(obj, "hasadjustments"); adj {
						adjusted++
						if o, ok := decodeInt(obj["orientation"]); !ok || o != 0 {
							t.Errorf("%s element %d: hasadjustments is true and orientation is not 0", part.file, i+1)
						}
					}
					// The offset of the string `date` is not compared; count where it differs
					// from tzoffset (outside the default date).
					s, _ := decodeString(obj["date"])
					if m, ok := parseMoment(s); ok && !m.Equal(epoch) {
						tz, _ := decodeInt(obj["tzoffset"])
						if int64(isoOffset(t, s)) != tz {
							offsetDiffers++
						}
					}
				}
			}
			var nullKeys []string
			for _, k := range allKeys() {
				if nulls[k] > 0 {
					nullKeys = append(nullKeys, k+"="+strconv.Itoa(nulls[k]))
				}
			}
			t.Logf("%s: %d objects (%d in query.json), date offset differs from tzoffset in %d, hasadjustments in %d; keys with null: %s",
				name, len(ref.Query)+len(ref.Deleted), len(ref.Query), offsetDiffers, adjusted, strings.Join(nullKeys, " "))
		})
	}
}
