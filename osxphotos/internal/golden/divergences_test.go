package golden

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func record(pairs ...string) string {
	m := map[string]string{
		"library":  `"synthetic"`,
		"uuid":     `"00000000-0000-0000-0000-000000000001"`,
		"field":    `"title"`,
		"reason":   `"A reason in English."`,
		"upstream": `"photosdb/photosdb.py:2537-2551"`,
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] == "-" {
			delete(m, pairs[i])
		} else {
			m[pairs[i]] = pairs[i+1]
		}
	}
	var parts []string
	for _, k := range []string{"library", "uuid", "field", "want", "got", "reason", "upstream"} {
		if v, ok := m[k]; ok {
			parts = append(parts, `"`+k+`": `+v)
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func loadDivs(t *testing.T, content string) ([]Divergence, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "divergences.json")
	writeFile(t, path, content)
	return LoadDivergences(path)
}

func TestDivergences(t *testing.T) {
	t.Parallel()
	valid := []struct{ name, content string }{
		{"empty array", `[]`},
		{"empty array with spaces", " \n[ ]\n"},
		{"a record for an object", "[" + record() + "]"},
		{"a record with a line range", "[" + record("upstream", `"a/b_c.py:5-5"`) + "]"},
		{"a record with a line", "[" + record("upstream", `"a.py:5"`) + "]"},
		{"a record with values", "[" + record("want", `"x"`, "got", `""`) + "]"},
		{"a record with only want", "[" + record("want", `[1,2]`) + "]"},
		{"a record for all objects", "[" + record("uuid", `"*"`, "field", `"has_raw"`, "want", "false", "got", "true") + "]"},
		{"all objects with null and 0", "[" + record("uuid", `"*"`, "want", "null", "got", "0") + "]"},
		{"all objects with an object", "[" + record("uuid", `"*"`, "want", `{"a":1,"b":[true,null]}`, "got", `"é"`) + "]"},
		{"a local record", "[" + record("library", `"local"`, "uuid", `"*"`, "want", "true", "got", "false") + "]"},
		{"a local record with numbers", "[" + record("library", `"local"`, "uuid", `"*"`, "field", `"width"`, "want", "-1", "got", "3024") + "]"},
		{"a local record with null", "[" + record("library", `"local"`, "uuid", `"*"`, "want", "null", "got", "1.5e-7") + "]"},
		{"two records of one key with other values", "[" + record("want", `"a"`) + ", " + record("want", `"b"`) + "]"},
		{"two records of one value and other fields", "[" + record() + ", " + record("field", `"width"`) + "]"},
	}
	for _, c := range valid {
		t.Run("valid/"+c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := loadDivs(t, c.content); err != nil {
				t.Error(err)
			}
		})
	}

	invalid := []struct{ name, content, want string }{
		{"empty file", ``, "not one JSON array"},
		{"blank file", " \n", "not one JSON array"},
		{"null", `null`, "not one JSON array"},
		{"object", `{}`, "not one JSON array"},
		{"string", `"x"`, "not one JSON array"},
		{"not JSON", `[`, "not one JSON array"},
		{"data after the array", `[] []`, "not one JSON array"},
		{"garbage after the array", `[]x`, "not one JSON array"},
		{"a record after the array", "[] " + record(), "not one JSON array"},
		{"element is a string", `["SECRET-X"]`, "record 1 is not an object"},
		{"element is null", `[null]`, "record 1: library must be one directory name"},
		{"element is a list", `[[]]`, "record 1 is not an object"},
		{"unknown key", "[" + strings.TrimSuffix(record(), "}") + `, "SECRET-KEY": 1}]`, "record 1 is not an object"},
		{"wrong type", "[" + record("library", `5`) + "]", "record 1 is not an object"},
		{"second record is bad", "[" + record() + ", " + record("field", `"nonsense"`) + "]", "record 2: field is not a key"},

		{"no library", "[" + record("library", "-") + "]", "record 1: library must be one directory name"},
		{"empty library", "[" + record("library", `""`) + "]", "library must be one directory name"},
		{"library with a slash", "[" + record("library", `"a/b"`) + "]", "library must be one directory name"},
		{"library with a backslash", "[" + record("library", `"a\\b"`) + "]", "library must be one directory name"},
		{"library .", "[" + record("library", `"."`) + "]", "library must be one directory name"},
		{"library ..", "[" + record("library", `".."`) + "]", "library must be one directory name"},
		{"empty uuid", "[" + record("uuid", `""`) + "]", "uuid is empty"},
		{"no uuid", "[" + record("uuid", "-") + "]", "uuid is empty"},
		{"unknown field", "[" + record("field", `"SECRET-FIELD"`) + "]", "field is not a key of fields.txt"},
		{"no field", "[" + record("field", "-") + "]", "field is not a key of fields.txt"},
		{"empty reason", "[" + record("reason", `""`) + "]", "reason is empty"},
		{"blank reason", "[" + record("reason", `"  "`) + "]", "reason is empty"},
		{"no reason", "[" + record("reason", "-") + "]", "reason is empty"},
		{"no upstream", "[" + record("upstream", "-") + "]", "upstream is not a path"},
		{"upstream without a line", "[" + record("upstream", `"a.py"`) + "]", "upstream is not a path"},
		{"upstream with a capital", "[" + record("upstream", `"A.py:1"`) + "]", "upstream is not a path"},
		{"upstream not python", "[" + record("upstream", `"a.go:1"`) + "]", "upstream is not a path"},
		{"upstream with a dot path", "[" + record("upstream", `"../a.py:1"`) + "]", "upstream is not a path"},
		{"upstream with an open range", "[" + record("upstream", `"a.py:1-"`) + "]", "upstream is not a path"},
		{"upstream with a space", "[" + record("upstream", `"a.py:1 "`) + "]", "upstream is not a path"},

		{"want out of range", "[" + record("want", `1e999`) + "]", "want or got is a number out of range"},
		{"got out of range", "[" + record("got", `[-1e999]`) + "]", "want or got is a number out of range"},
		{"all objects without values", "[" + record("uuid", `"*"`) + "]", `uuid "*" needs both want and got`},
		{"all objects with want only", "[" + record("uuid", `"*"`, "want", "true") + "]", `uuid "*" needs both want and got`},
		{"all objects with got only", "[" + record("uuid", `"*"`, "got", "true") + "]", `uuid "*" needs both want and got`},
		{"local with a uuid", "[" + record("library", `"local"`, "want", "true", "got", "false") + "]", `only for uuid "*"`},
		{"local without values", "[" + record("library", `"local"`, "uuid", `"*"`) + "]", "needs both want and got"},
		{"local with a string", "[" + record("library", `"local"`, "uuid", `"*"`, "want", `"SECRET-S"`, "got", "1") + "]", "takes true, false, null or a number"},
		{"local with a list", "[" + record("library", `"local"`, "uuid", `"*"`, "want", `[]`, "got", "1") + "]", "takes true, false, null or a number"},
		{"local with an object", "[" + record("library", `"local"`, "uuid", `"*"`, "want", "1", "got", `{}`) + "]", "takes true, false, null or a number"},
		{"a record repeated", "[" + record() + ", " + record() + "]", "record 2 repeats record 1"},
		{"a record repeated with values", "[" + record("want", "1", "got", "2") + ", " + record("want", "1", "got", "2") + "]", "record 2 repeats record 1"},
		{"a record repeated in another form", "[" + record("want", `{"b":1,"a":[2]}`) + ", " + record("want", `{"a": [2.0], "b": 1}`) + "]", "record 2 repeats record 1"},
	}
	for _, c := range invalid {
		t.Run("invalid/"+c.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadDivs(t, c.content)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to contain %q", err, c.want)
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "0000-0000") {
				t.Errorf("the error repeats the content: %v", err)
			}
		})
	}

	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()
		if _, err := LoadDivergences(filepath.Join(t.TempDir(), "SECRET-DIR", "divergences.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("null is not the same as absent", func(t *testing.T) {
		t.Parallel()
		divs, err := loadDivs(t, "["+record("uuid", `"*"`, "want", "null", "got", "0")+", "+record("want", "null")+", "+record("field", `"width"`)+"]")
		if err != nil {
			t.Fatal(err)
		}
		if string(divs[0].Want) != "null" || string(divs[0].Got) != "0" {
			t.Errorf("the first record: want %q got %q, want null and 0", divs[0].Want, divs[0].Got)
		}
		if string(divs[1].Want) != "null" || divs[1].Got != nil {
			t.Errorf("the second record: want %q got %q, want null and nothing", divs[1].Want, divs[1].Got)
		}
		if divs[2].Want != nil || divs[2].Got != nil {
			t.Errorf("the third record has values: %q %q", divs[2].Want, divs[2].Got)
		}
	})
	t.Run("the record is read as written", func(t *testing.T) {
		t.Parallel()
		divs, err := loadDivs(t, "["+record("want", `{"a":[1,"é"]}`, "got", `"x"`)+"]")
		if err != nil {
			t.Fatal(err)
		}
		want := Divergence{
			Library: "synthetic", UUID: "00000000-0000-0000-0000-000000000001", Field: "title",
			Want: []byte(`{"a":[1,"é"]}`), Got: []byte(`"x"`), Reason: "A reason in English.", Upstream: "photosdb/photosdb.py:2537-2551",
		}
		if len(divs) != 1 || !reflect.DeepEqual(divs[0], want) {
			t.Errorf("got %+v, want %+v", divs, want)
		}
	})
	t.Run("want and got are kept in the form of Diff", func(t *testing.T) {
		t.Parallel()
		for _, c := range []struct{ in, want string }{
			{`[1, 2]`, `[1,2]`},
			{`{"b": 1, "a": 2}`, `{"a":2,"b":1}`},
			{`1.0`, `1`},
			{`"` + uesc("0041") + `"`, `"A"`},
			{`"` + uesc("003c") + `&>"`, `"<&>"`},
			{`"x\"y"`, `"x\"y"`},
			{`"` + string(rune(0x2028)) + `"`, `"` + uesc("2028") + `"`},
			{`["é", null]`, `["é",null]`},
		} {
			divs, err := loadDivs(t, "["+record("want", c.in, "got", c.in)+"]")
			if err != nil {
				t.Fatalf("%s: %v", c.in, err)
			}
			if string(divs[0].Want) != c.want || string(divs[0].Got) != c.want {
				t.Errorf("%s: want %s, got %s; want %s", c.in, divs[0].Want, divs[0].Got, c.want)
			}
		}
	})
}

// The tokens that must not be in the record of a local library.
var uuidLike = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// The records of a local library must keep the data of its owner out of the
// public file.
func TestDivergencesFile(t *testing.T) {
	t.Parallel()
	divs, err := LoadDivergences(filepath.Join(testlib.RepoDir(t, "golden"), "divergences.json"))
	if err != nil {
		t.Fatal(err)
	}
	libs := append(testlib.Libraries(t), "local")
	fields := fieldsTxt(t)
	for i, d := range divs {
		if !slices.Contains(libs, d.Library) {
			t.Errorf("record %d: library %q has no reference data", i+1, d.Library)
		}
		if !slices.Contains(fields, d.Field) {
			t.Errorf("record %d: field %q is not in fields.txt", i+1, d.Field)
		}
		if d.Library == "local" {
			for _, s := range []string{d.UUID, string(d.Want), string(d.Got), d.Reason} {
				if uuidLike.MatchString(s) {
					t.Errorf("record %d: a record of a local library has a UUID", i+1)
				}
			}
			if strings.Contains(string(d.Want)+string(d.Got), `"`) {
				t.Errorf("record %d: a record of a local library has a string", i+1)
			}
		}
	}
}

func titleDiffLib() Library {
	lib := fixtureLib()
	lib.Photos[0].Title = ""
	return lib
}

func TestExplain(t *testing.T) {
	t.Parallel()
	title := func(extra ...string) Divergence {
		d := Divergence{Library: "lib", UUID: selectedFrame, Field: "title", Reason: "r", Upstream: "a.py:1"}
		for i := 0; i+1 < len(extra); i += 2 {
			switch extra[i] {
			case "uuid":
				d.UUID = extra[i+1]
			case "library":
				d.Library = extra[i+1]
			case "field":
				d.Field = extra[i+1]
			case "want":
				d.Want = []byte(extra[i+1])
			case "got":
				d.Got = []byte(extra[i+1])
			}
		}
		return d
	}
	cases := []struct {
		name      string
		keys      []string
		divs      []Divergence
		wantDiffs int
		explained map[string]int
		stale     int
	}{
		{"no records", nil, nil, 1, map[string]int{}, 0},
		{"a record by uuid", nil, []Divergence{title()}, 0, map[string]int{"title": 1}, 0},
		{"a record by uuid with the values", nil, []Divergence{title("want", `"Title A"`, "got", `""`)}, 0, map[string]int{"title": 1}, 0},
		{"a record by uuid with want only", nil, []Divergence{title("want", `"Title A"`)}, 0, map[string]int{"title": 1}, 0},
		{"a record by uuid with got only", nil, []Divergence{title("got", `""`)}, 0, map[string]int{"title": 1}, 0},
		{"a record for all objects", nil, []Divergence{title("uuid", "*", "want", `"Title A"`, "got", `""`)}, 0, map[string]int{"title": 1}, 0},
		{"another want", nil, []Divergence{title("want", `"Other"`)}, 1, map[string]int{}, 1},
		{"another got", nil, []Divergence{title("got", `"x"`)}, 1, map[string]int{}, 1},
		{"all objects with another got", nil, []Divergence{title("uuid", "*", "want", `"Title A"`, "got", `"x"`)}, 1, map[string]int{}, 1},
		{"another uuid", nil, []Divergence{title("uuid", emptyPhoto)}, 1, map[string]int{}, 1},
		{"another field", nil, []Divergence{title("field", "width")}, 1, map[string]int{}, 1},
		{"a record of another library is ignored", nil, []Divergence{title("library", "other")}, 1, map[string]int{}, 0},
		{"a record of a key that is not compared is not stale", []string{"title"}, []Divergence{title("field", "width")}, 1, map[string]int{}, 0},
		{"a record of a compared key is stale", []string{"title", "width"}, []Divergence{title("field", "width")}, 1, map[string]int{}, 1},
		{"a record of the uuid is stale even when uuid is not a key", []string{"title"}, []Divergence{title("field", "uuid", "want", "true", "got", "false")}, 1, map[string]int{}, 1},
		{"two records explain one difference", nil, []Divergence{title(), title("uuid", "*", "want", `"Title A"`, "got", `""`)}, 0, map[string]int{"title": 1}, 0},
		{"two records, one of them stale", nil, []Divergence{title(), title("want", `"Other"`)}, 0, map[string]int{"title": 1}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			keys := c.keys
			if keys == nil {
				keys = everyKey()
			}
			rep, err := Compare(fixtureRef(), titleDiffLib(), keys, c.divs)
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Diffs) != c.wantDiffs || !reflect.DeepEqual(rep.Explained, c.explained) || len(rep.Stale) != c.stale {
				t.Errorf("Diffs = %v, Explained = %v, Stale = %v; want %d, %v, %d", rep.Diffs, rep.Explained, rep.Stale, c.wantDiffs, c.explained, c.stale)
			}
			if rep.OK() != (c.wantDiffs == 0 && c.stale == 0) {
				t.Errorf("OK() = %v", rep.OK())
			}
			rev := slices.Clone(c.divs)
			slices.Reverse(rev)
			again, err := Compare(fixtureRef(), titleDiffLib(), keys, rev)
			if err != nil || !reflect.DeepEqual(again, rep) {
				t.Errorf("the report depends on the order of the records")
			}
		})
	}

	t.Run("Explained counts per key", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].Title, lib.Photos[0].Description, lib.Photos[2].Title = "", "", "t"
		divs := []Divergence{
			{Library: "lib", UUID: "*", Field: "title", Want: []byte(`""`), Got: []byte(`"t"`), Reason: "r", Upstream: "a.py:1"},
			{Library: "lib", UUID: selectedFrame, Field: "title", Reason: "r", Upstream: "a.py:1"},
			{Library: "lib", UUID: selectedFrame, Field: "description", Reason: "r", Upstream: "a.py:1"},
		}
		rep := compare(t, fixtureRef(), lib, divs...)
		if !rep.OK() || !reflect.DeepEqual(rep.Explained, map[string]int{"title": 2, "description": 1}) {
			t.Errorf("Explained = %v, Diffs = %v, Stale = %v", rep.Explained, rep.Diffs, rep.Stale)
		}
	})
	t.Run("a stale record is sorted and complete", func(t *testing.T) {
		t.Parallel()
		divs := []Divergence{
			{Library: "lib", UUID: unselectedFrame, Field: "width", Reason: "r2", Upstream: "a.py:1"},
			{Library: "lib", UUID: selectedFrame, Field: "title", Want: []byte(`"x"`), Reason: "r1", Upstream: "a.py:1"},
			{Library: "lib", UUID: selectedFrame, Field: "height", Reason: "r3", Upstream: "a.py:1"},
		}
		rep := compare(t, fixtureRef(), fixtureLib(), divs...)
		if len(rep.Stale) != 3 || rep.Stale[0].Field != "height" || rep.Stale[1].Field != "title" || rep.Stale[2].Field != "width" {
			t.Fatalf("Stale = %+v, want height, title, width", rep.Stale)
		}
		if rep.Stale[1].Reason != "r1" || string(rep.Stale[1].Want) != `"x"` {
			t.Errorf("a stale record is not kept whole: %+v", rep.Stale[1])
		}
	})
	t.Run("the set of objects is explained like a key", func(t *testing.T) {
		t.Parallel()
		lib := fixtureLib()
		lib.Photos[0].IsPhoto = false
		d := Divergence{Library: "lib", UUID: selectedFrame, Field: "uuid", Want: []byte("true"), Got: []byte("false"), Reason: "r", Upstream: "a.py:1"}
		rep, err := Compare(fixtureRef(), lib, nil, []Divergence{d})
		if err != nil || !rep.OK() || rep.Explained["uuid"] != 1 {
			t.Errorf("Compare = %+v, %v; want the difference of uuid explained", rep, err)
		}
	})
}

func TestExplainedRecordForTwoLibraries(t *testing.T) {
	t.Parallel()
	d := Divergence{Library: "other", UUID: selectedFrame, Field: "title", Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), titleDiffLib(), d)
	if len(rep.Diffs) != 1 || len(rep.Stale) != 0 {
		t.Errorf("Diffs = %v, Stale = %v", rep.Diffs, rep.Stale)
	}
}
