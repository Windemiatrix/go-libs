package golden

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func allKeys() []string { return slices.Sorted(maps.Keys(rules)) }

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

func TestReferenceParses(t *testing.T) {
	dir := testlib.RepoDir(t, "golden")
	epoch := time.Unix(0, 0)
	for _, name := range testlib.Libraries(t) {
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
					if got, _ := decode[bool](obj, "intrash", false, ""); got != part.trash {
						t.Errorf("%s element %d: intrash = %v, want %v", part.file, i+1, got, part.trash)
					}
					// golden-regen runs with TZ=UTC: the dates of the database are in +00:00.
					for _, key := range []string{"date_added", "date_modified", "date_trashed"} {
						if s, _ := decode[string](obj, key, true, ""); s != "" && !strings.HasSuffix(s, "+00:00") {
							t.Errorf("%s element %d: %s is not in +00:00", part.file, i+1, key)
						}
					}
					// The premise of the orientation rule.
					if adj, _ := decode[bool](obj, "hasadjustments", false, ""); adj {
						adjusted++
						if o, err := decode[int64](obj, "orientation", false, ""); err != nil || o != 0 {
							t.Errorf("%s element %d: hasadjustments is true and orientation is not 0", part.file, i+1)
						}
					}
					// The offset of the string `date` is not compared; count where it differs
					// from tzoffset (outside the default date).
					s, _ := decode[string](obj, "date", false, "")
					if m, ok := parseMoment(s); ok && !m.Equal(epoch) {
						tz, _ := decode[int64](obj, "tzoffset", true, "")
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
